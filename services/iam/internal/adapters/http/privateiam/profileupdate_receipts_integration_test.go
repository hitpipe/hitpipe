//go:build integration

package privateiam_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

// profileCommandFixture is the test owner's explicit receipt authority. It
// has no process-local receipt state and is not used by production IAM.
type profileCommandFixture struct{ root string }

func newProfileCommandFixture(root string) (*profileCommandFixture, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, app.ErrUnavailable
	}
	dir, err := openProfileReceiptDir(root)
	if err != nil {
		return nil, err
	}
	dir.Close()
	return &profileCommandFixture{root: root}, nil
}

func openProfileReceiptDir(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	dir := os.NewFile(uintptr(fd), path)
	info, err := dir.Stat()
	if err != nil || info.Mode() != os.ModeDir|0700 {
		dir.Close()
		return nil, app.ErrUnavailable
	}
	return dir, nil
}

func receiptFixtureID(id string) bool {
	return runtimeaccess.ValidUUID(id) && id == strings.ToLower(id)
}

func (f *profileCommandFixture) WithCommand(ctx context.Context, principal, request string, use func(context.Context, app.CommandRecord) error) error {
	if f == nil || ctx == nil || ctx.Err() != nil || use == nil || !receiptFixtureID(principal) || !receiptFixtureID(request) {
		return app.ErrUnavailable
	}
	dirPath := filepath.Join(f.root, principal)
	root, err := openProfileReceiptDir(f.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := os.Mkdir(dirPath, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return app.ErrUnavailable
	}
	if root.Sync() != nil {
		return app.ErrUnavailable
	}
	dir, err := openProfileReceiptDir(dirPath)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := lockProfileReceiptFixture(ctx, dir); err != nil {
		return err
	}
	return use(ctx, &fixtureProfileReceipt{dir: dir, name: request + ".json"})
}

func lockProfileReceiptFixture(ctx context.Context, dir *os.File) error {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return ctx.Err()
		} else if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			return app.ErrUnavailable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

type fixtureProfileReceipt struct {
	dir  *os.File
	name string
}

func receiptFixtureValid(r app.Receipt) bool {
	return r.Revision > 1 && r.Revision <= 9007199254740991 && r.Before.Ref != r.After.Ref &&
		receiptFixtureID(r.Before.Ref) && receiptFixtureID(r.After.Ref) &&
		runtimeaccess.ValidFingerprint(r.BodyDigest) &&
		runtimeaccess.ValidFingerprint(r.Before.Digest) && runtimeaccess.ValidFingerprint(r.After.Digest)
}

func (r *fixtureProfileReceipt) Load(ctx context.Context) (app.Receipt, bool, error) {
	if err := ctx.Err(); err != nil {
		return app.Receipt{}, false, err
	}
	fd, err := syscall.Openat(int(r.dir.Fd()), r.name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return app.Receipt{}, false, nil
	}
	if err != nil {
		return app.Receipt{}, false, app.ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "fixture-profile-command")
	defer file.Close()
	const limit = 8192
	info, err := file.Stat()
	if err != nil || info.Mode() != 0600 || info.Size() < 1 || info.Size() > limit {
		return app.Receipt{}, false, app.ErrUnavailable
	}
	bytes, err := io.ReadAll(io.LimitReader(file, limit+1))
	var receipt app.Receipt
	if err != nil || len(bytes) > limit || json.Unmarshal(bytes, &receipt) != nil || !receiptFixtureValid(receipt) {
		return app.Receipt{}, false, app.ErrUnavailable
	}
	// A previous publication may have renamed a synced file but failed to
	// sync this directory. Make the visible entry durable before using it.
	if r.dir.Sync() != nil {
		return app.Receipt{}, false, app.ErrUnavailable
	}
	return receipt, true, ctx.Err()
}

func (r *fixtureProfileReceipt) Prepare(ctx context.Context, want app.Receipt) error {
	if want.Confirmed || !receiptFixtureValid(want) {
		return app.ErrInvalid
	}
	prior, found, err := r.Load(ctx)
	if err != nil {
		return err
	}
	if found {
		prior.Confirmed = false
		if prior != want {
			return app.ErrConflict
		}
		return nil
	}
	return r.persist(ctx, want)
}

func (r *fixtureProfileReceipt) Confirm(ctx context.Context, want app.Receipt) error {
	current, found, err := r.Load(ctx)
	if err != nil {
		return err
	}
	if !found {
		return app.ErrUnavailable
	}
	want.Confirmed = current.Confirmed
	if want != current {
		return app.ErrConflict
	}
	if current.Confirmed {
		return nil
	}
	want.Confirmed = true
	return r.persist(ctx, want)
}

func (r *fixtureProfileReceipt) persist(ctx context.Context, receipt app.Receipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return app.ErrUnavailable
	}
	file, err := os.CreateTemp(r.dir.Name(), ".intent-")
	if err != nil {
		return app.ErrUnavailable
	}
	defer file.Close()
	defer os.Remove(file.Name())
	written, err := file.Write(data)
	if err != nil || written != len(data) || file.Chmod(0600) != nil || file.Sync() != nil {
		return app.ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if syscall.Renameat(int(r.dir.Fd()), filepath.Base(file.Name()), int(r.dir.Fd()), r.name) != nil || r.dir.Sync() != nil {
		return app.ErrUnavailable
	}
	return ctx.Err()
}

func TestProfileCommandFixtureSurvivesReopenAndRefusesAlternateIntent(t *testing.T) {
	const fixtureRootEnv = "HIT125_RECEIPT_RESTART_TEST_ROOT"
	root, restarted := os.LookupEnv(fixtureRootEnv)
	if !restarted {
		root = t.TempDir()
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	const principal = "550e8400-e29b-41d4-a716-446655440001"
	const request = "550e8400-e29b-41d4-a716-446655440002"
	digest := func(symbol string) string { return "sha256:" + strings.Repeat(symbol, 64) }
	want := app.Receipt{
		BodyDigest: digest("1"), Revision: 2,
		Before: app.Pointer{Ref: "550e8400-e29b-41d4-a716-446655440003", Digest: digest("2")},
		After:  app.Pointer{Ref: "550e8400-e29b-41d4-a716-446655440004", Digest: digest("3")},
	}
	withReopened := func(use func(context.Context, app.CommandRecord) error) {
		t.Helper()
		port, err := newProfileCommandFixture(root)
		if err != nil {
			t.Fatal(err)
		}
		if err := port.WithCommand(t.Context(), principal, request, use); err != nil {
			t.Fatal(err)
		}
	}
	verifyConfirmed := func(ctx context.Context, record app.CommandRecord) error {
		confirmed := want
		confirmed.Confirmed = true
		got, exists, err := record.Load(ctx)
		if err != nil || !exists || got != confirmed {
			t.Fatal("confirmed durable intent unavailable after restart")
		}
		return record.Prepare(ctx, want)
	}
	if restarted {
		withReopened(verifyConfirmed)
		return
	}
	withReopened(func(ctx context.Context, record app.CommandRecord) error { return record.Prepare(ctx, want) })
	withReopened(func(ctx context.Context, record app.CommandRecord) error {
		got, exists, err := record.Load(ctx)
		if err != nil || !exists || got != want {
			t.Fatal("prepared durable intent unavailable after reopen")
		}
		foreign := want
		foreign.BodyDigest = digest("4")
		if !errors.Is(record.Confirm(ctx, foreign), app.ErrConflict) {
			t.Fatal("different body confirmed authority")
		}
		return record.Confirm(ctx, want)
	})
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestProfileCommandFixtureSurvivesReopenAndRefusesAlternateIntent$")
	child.Env = append(os.Environ(), fixtureRootEnv+"="+root)
	if err := child.Run(); err != nil {
		t.Fatalf("receipt did not survive a fresh process: %v", err)
	}
}
