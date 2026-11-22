package profilefiles

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
)

// WriteProfile publishes an immutable version in the format used by Read.
// The returned pointer can be linked by IAM only after publication is durable.
// Database rollback or an unconfirmed result does not authorize file deletion.
func (r *Reader) WriteProfile(ctx context.Context, profile getprofile.Profile) (app.Pointer, error) {
	if ctx == nil || r == nil {
		return app.Pointer{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return app.Pointer{}, err
	}
	data, err := json.Marshal(profile)
	if err != nil || len(data) > MaxBytes {
		return app.Pointer{}, ErrUnavailable
	}
	check, err := decode(data)
	if err != nil || check != profile {
		return app.Pointer{}, ErrUnavailable
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.dir == nil || !private(r.dir, true) {
		return app.Pointer{}, ErrUnavailable
	}
	dir, err := lockProfileDirectory(ctx, r.dir)
	if err != nil {
		return app.Pointer{}, err
	}
	defer dir.Close()
	ref := newProfileRef()
	if err := publishProfile(ctx, dir, ref, data); err != nil {
		return app.Pointer{}, err
	}
	sum := sha256.Sum256(data)
	return app.Pointer{Ref: ref, Digest: "sha256:" + hex.EncodeToString(sum[:])}, nil
}

func newProfileRef() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	value := hex.EncodeToString(id[:])
	return value[:8] + "-" + value[8:12] + "-" + value[12:16] + "-" + value[16:20] + "-" + value[20:]
}

func lockProfileDirectory(ctx context.Context, pinned *os.File) (*os.File, error) {
	// Open a distinct file description, so competing writers using the same
	// Reader or different Reader instances take the same advisory lock.
	fd, err := syscall.Openat(int(pinned.Fd()), ".", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	dir := os.NewFile(uintptr(fd), "iam-profile-publication")
	retry := time.NewTicker(5 * time.Millisecond)
	defer retry.Stop()
	for {
		if err := ctx.Err(); err != nil {
			dir.Close()
			return nil, err
		}
		err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil && private(dir, true) {
			return dir, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			dir.Close()
			return nil, ErrUnavailable
		}
		select {
		case <-ctx.Done():
			dir.Close()
			return nil, ctx.Err()
		case <-retry.C:
		}
	}
}

// publishProfile requires the exclusive publication lock. The old reader
// opens only finalized UUID.json names, even while the writer is staging data.
func publishProfile(ctx context.Context, dir *os.File, ref string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dirFD := int(dir.Fd())
	pending, name := ".pending-"+ref, ref+".json"
	flags := syscall.O_WRONLY | syscall.O_CREAT | syscall.O_EXCL | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
	fd, err := syscall.Openat(dirFD, pending, flags, 0600)
	if err != nil {
		return ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "iam-profile-staged")
	defer file.Close()
	published := false
	defer func() {
		if !published {
			_ = syscall.Unlinkat(dirFD, pending)
		}
	}()
	if file.Chmod(0600) != nil {
		return ErrUnavailable
	}
	n, err := file.Write(data)
	if err != nil || n != len(data) || !private(file, false) || file.Sync() != nil || !private(dir, true) {
		return ErrUnavailable
	}
	// Every IAM writer checks and publishes under the directory lock. An
	// existing name of any kind is never replaced, even on UUID collision.
	dest, err := syscall.Openat(dirFD, name, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err == nil {
		_ = syscall.Close(dest)
		return ErrUnavailable
	}
	if !errors.Is(err, syscall.ENOENT) {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if syscall.Renameat(dirFD, pending, dirFD, name) != nil {
		return ErrUnavailable
	}
	published = true
	if dir.Sync() != nil || !private(dir, true) || !private(file, false) {
		return ErrUnavailable
	}
	return ctx.Err()
}
