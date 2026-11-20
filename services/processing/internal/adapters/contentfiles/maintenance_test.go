package contentfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaintenanceExcludesRuntimeAcrossRenameAndCommit(t *testing.T) {
	p := private(t)
	files, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	atRename := make(chan struct{})
	done := make(chan error, 1)
	release := make(chan struct{})
	defer close(release)
	files.Step = func(step string) error {
		if step == "rename" {
			close(atRename)
			<-release
		}
		return nil
	}
	now := time.Now()
	go func() {
		_, err := files.Write(context.Background(), []byte("fixture"), "HISTORY", fixtureOrigin(), now.Add(time.Minute), now)
		done <- err
	}()
	<-atRename
	if m, err := OpenMaintenance(p); err == nil {
		m.Close()
		t.Error("maintenance reached volume during durable write")
	}
	release <- struct{}{}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if m, err := OpenMaintenance(p); err == nil {
		m.Close()
		t.Error("maintenance reached volume before caller's SQL commit")
	}
	if err = files.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMaintenance(p)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if runtime, err := New(p); err == nil {
		runtime.Close()
		t.Fatal("runtime reached volume during maintenance")
	}
	if n, err := m.Sweep(context.Background(), fixtureOrigin().WorkspaceID, now, 1, func(string) (bool, error) {
		t.Error("unexpired file reached the database deletion check")
		return true, nil
	}); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}

func TestMaintenanceRejectsUnsafeNamesAndRechecksIdentity(t *testing.T) {
	p := private(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	f := open(t, p)
	ref, err := f.Write(context.Background(), []byte("expired fixture"), "HISTORY", fixtureOrigin(), now.Add(time.Second), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	symlink, _ := UUID()
	if err = os.Symlink(outside, filepath.Join(p, symlink)); err != nil {
		t.Fatal(err)
	}
	hardlink, _ := UUID()
	if err = os.Link(outside, filepath.Join(p, hardlink)); err != nil {
		t.Fatal(err)
	}
	m, err := OpenMaintenance(p)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	n, err := m.Sweep(context.Background(), fixtureOrigin().WorkspaceID, now.Add(time.Minute), 10, func(id string) (bool, error) {
		if id != ref.ID {
			t.Fatalf("invalid candidate reached DB check: %s", id)
		}
		if err := os.Rename(filepath.Join(p, id), filepath.Join(p, "fixture-replaced")); err != nil {
			t.Fatal(err)
		}
		return true, os.WriteFile(filepath.Join(p, id), []byte("new inode"), 0600)
	})
	if n != 0 || err == nil {
		t.Fatalf("unsafe sweep: removed=%d err=%v", n, err)
	}
	for path, want := range map[string]string{outside: "outside bytes", filepath.Join(p, ref.ID): "new inode"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("unexpected change: %v %v", string(got), err)
		}
	}
	halt := errors.New("source unavailable")
	if err = os.Rename(filepath.Join(p, "fixture-replaced"), filepath.Join(p, ref.ID)); err != nil {
		t.Fatal(err)
	}
	n, err = m.Sweep(context.Background(), fixtureOrigin().WorkspaceID, now.Add(time.Minute), 1, func(string) (bool, error) {
		return false, halt
	})
	if n != 0 || !errors.Is(err, halt) {
		t.Fatalf("failed SQL guard did not stop unlink: %d %v", n, err)
	}
}
