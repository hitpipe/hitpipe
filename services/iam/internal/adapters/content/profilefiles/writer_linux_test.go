package profilefiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
)

func TestProfileWriterPublishesExactCompatibleVersion(t *testing.T) {
	dir, writer := fixture(t, valid)
	current, err := writer.Read(t.Context(), ref, digest(valid))
	if err != nil {
		t.Fatal(err)
	}
	next := current
	next.DisplayName, next.Timezone, next.Revision = "Тестовое имя", "local / synthetic zone", 2
	ptr, err := writer.WriteProfile(t.Context(), next)
	if err != nil {
		t.Fatal(err)
	}
	if !canonicalUUID(ptr.Ref) || ptr.Ref == ref || ptr.Ref[14] != '4' || !strings.ContainsRune("89ab", rune(ptr.Ref[19])) {
		t.Fatal("writer did not publish a fresh opaque v4 reference")
	}
	filename := filepath.Join(dir, ptr.Ref+".json")
	info, err := os.Stat(filename)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("profile publication is absent or its mode is unsafe")
	}
	raw, err := os.ReadFile(filename)
	if err != nil || ptr.Digest != digest(string(raw)) {
		t.Fatal("pointer digest does not bind the exact immutable bytes")
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got, err := reader.Read(t.Context(), ptr.Ref, ptr.Digest)
	if err != nil || got != next {
		t.Fatal("reopened accepted reader did not recover exact profile")
	}
	got, err = reader.Read(t.Context(), ref, digest(valid))
	if err != nil || got != current {
		t.Fatal("previous version is no longer readable")
	}
}

func TestProfileWriterRefusesInvalidVersionsAndCannotReplaceExistingFiles(t *testing.T) {
	dir, writer := fixture(t, valid)
	previous, err := writer.Read(t.Context(), ref, digest(valid))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []getprofile.Profile{
		{UserID: previous.UserID, DisplayName: string([]byte{255}), Timezone: "custom", Revision: 2},
		{UserID: previous.UserID, DisplayName: "synthetic", Timezone: strings.Repeat("a", 81), Revision: 2},
		{UserID: "../foreign", DisplayName: "synthetic", Timezone: "custom", Revision: 2},
		{UserID: previous.UserID, DisplayName: "synthetic", Timezone: "custom", Revision: 0},
	} {
		if ptr, err := writer.WriteProfile(t.Context(), p); !errors.Is(err, ErrUnavailable) || ptr != (app.Pointer{}) {
			t.Fatal("invalid profile produced a writeable pointer")
		}
	}
	locked, err := lockProfileDirectory(t.Context(), writer.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	if err := publishProfile(t.Context(), locked, ref, []byte("replacement")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("version publication allowed replacement")
	}
	symlinkRef := "550e8400-e29b-41d4-a716-446655440005"
	if err := os.Symlink(ref+".json", filepath.Join(dir, symlinkRef+".json")); err != nil {
		t.Fatal(err)
	}
	if err := publishProfile(t.Context(), locked, symlinkRef, []byte("replacement")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("version publication allowed symbolic-link replacement")
	}
	names, err := os.ReadDir(dir)
	if err != nil || len(names) != 2 {
		t.Fatal("failed publication left a staged version")
	}
	got, err := writer.Read(t.Context(), ref, digest(valid))
	if err != nil || got != previous {
		t.Fatal("failed publication changed previously accepted bytes")
	}
}

func TestProfileWriterSerializesPublicationAndKeepsOldReaderAvailable(t *testing.T) {
	dir, writer := fixture(t, valid)
	second, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	previous, err := writer.Read(t.Context(), ref, digest(valid))
	if err != nil {
		t.Fatal(err)
	}
	locked, err := lockProfileDirectory(t.Context(), writer.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Millisecond)
	defer cancel()
	if ptr, err := second.WriteProfile(ctx, previous); !errors.Is(err, context.DeadlineExceeded) || ptr != (app.Pointer{}) {
		t.Fatal("a blocked writer ignored the actual publication lock or its deadline")
	}
	got, err := second.Read(t.Context(), ref, digest(valid))
	if err != nil || got != previous {
		t.Fatal("blocked writer prevented pre-existing profile read")
	}
	if err := locked.Close(); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		ptr app.Pointer
		err error
	}
	results := make(chan outcome, 2)
	for _, handle := range []*Reader{writer, second} {
		go func() {
			ptr, err := handle.WriteProfile(t.Context(), previous)
			results <- outcome{ptr, err}
		}()
	}
	first, other := <-results, <-results
	if first.err != nil || other.err != nil || first.ptr.Ref == other.ptr.Ref {
		t.Fatal("parallel in-scope publications did not receive separate immutable references")
	}
	for _, result := range []outcome{first, other} {
		p, err := second.Read(t.Context(), result.ptr.Ref, result.ptr.Digest)
		if err != nil || p != previous {
			t.Fatal("concurrent publication exposed an incomplete version")
		}
	}
}
