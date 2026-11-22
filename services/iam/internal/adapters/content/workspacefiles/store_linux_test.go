package workspacefiles_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/content/workspacefiles"
)

const (
	workspace = "c1dd99ef-2481-4be0-b207-dd2f35371dee"
	other     = "874c42b6-5c32-4c68-87e8-2efa59a8d0d0"
	ref       = "dbab791d-e1a7-4598-bfdb-22522be826dc"
	name      = ref + ".json"
	pending   = ".pending-" + ref
	data      = `{"name":"Команда"}`
	changed   = `{"name":"Другое пространство"}`
)

func fixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "owner")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func openStore(t *testing.T, root string) *workspacefiles.Store {
	t.Helper()
	store, err := workspacefiles.New(root)
	if err != nil {
		t.Fatal("owner store failed to open")
	}
	t.Cleanup(func() {
		if store.Close() != nil {
			t.Error("owner store failed to close")
		}
	})
	return store
}

func requireContent(t *testing.T, store *workspacefiles.Store, id, expected string) {
	t.Helper()
	if actual, err := store.Read(t.Context(), id, ref); err != nil || actual != expected {
		t.Fatal("stored content did not round-trip exactly")
	}
}

func TestPublishReopenAndWorkspaceIsolation(t *testing.T) {
	root := fixture(t)
	store := openStore(t, root)
	if err := store.Put(t.Context(), workspace, ref, data); err != nil {
		t.Fatal("initial publication failed")
	}
	if _, err := store.Read(t.Context(), other, ref); !errors.Is(err, workspacefiles.ErrUnavailable) {
		t.Fatal("the same reference in another workspace disclosed content")
	}
	if err := store.Put(t.Context(), other, ref, changed); err != nil {
		t.Fatal("the same reference in a distinct workspace conflicted")
	}
	if err := store.Close(); err != nil {
		t.Fatal("owner store failed to close")
	}
	store = openStore(t, root)
	if err := store.Put(t.Context(), workspace, ref, data); err != nil {
		t.Fatal("exact retry failed after reopening the store")
	}
	if err := store.Put(t.Context(), workspace, ref, changed); !errors.Is(err, workspacefiles.ErrConflict) {
		t.Fatal("conflicting content replaced an existing reference")
	}
	requireContent(t, store, workspace, data)
	requireContent(t, store, other, changed)
	for _, id := range []string{workspace, other} {
		dir := filepath.Join(root, id)
		info, err := os.Stat(dir)
		if err != nil || info.Mode() != os.ModeDir|0700 {
			t.Fatal("workspace scope did not have exact owner-only permissions")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 || entries[0].Name() != name {
			t.Fatal("publication left an unexpected path in the owner directory")
		}
		info, err = entries[0].Info()
		if err != nil || info.Mode() != 0600 {
			t.Fatal("workspace object did not have exact owner-only permissions")
		}
	}
}

func TestConcurrentPublicationAcrossStoreInstances(t *testing.T) {
	root := fixture(t)
	stores := []*workspacefiles.Store{openStore(t, root), openStore(t, root)}
	inputs := []string{data, changed}
	type attempt struct {
		value string
		err   error
	}
	const workers = 12
	start := make(chan struct{})
	results := make(chan attempt, workers)
	var group sync.WaitGroup
	for i := range workers {
		group.Go(func() {
			<-start
			value := inputs[i%len(inputs)]
			err := stores[i%len(stores)].Put(t.Context(), workspace, ref, value)
			results <- attempt{value: value, err: err}
		})
	}
	close(start)
	group.Wait()
	close(results)
	winner, err := stores[0].Read(t.Context(), workspace, ref)
	if err != nil || winner != data && winner != changed {
		t.Fatal("concurrent publication lost both values")
	}
	for result := range results {
		if result.value == winner && result.err == nil || result.value != winner && errors.Is(result.err, workspacefiles.ErrConflict) {
			continue
		}
		t.Fatal("concurrent publication did not preserve exact idempotency and conflict")
	}
}

func TestRecoverInterruptedPublish(t *testing.T) {
	for _, scenario := range []string{"unpublished partial file", "published before directory sync"} {
		t.Run(scenario, func(t *testing.T) {
			root := fixture(t)
			dir := filepath.Join(root, workspace)
			if err := os.Mkdir(dir, 0700); err != nil {
				t.Fatal(err)
			}
			staged := data
			published := scenario == "published before directory sync"
			if !published {
				staged = "partial"
			}
			stagedPath := filepath.Join(dir, pending)
			if err := os.WriteFile(stagedPath, []byte(staged), 0600); err != nil {
				t.Fatal(err)
			}
			if published {
				if err := os.Rename(stagedPath, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			store := openStore(t, root)
			if _, err := store.Read(t.Context(), workspace, ref); !published && !errors.Is(err, workspacefiles.ErrUnavailable) {
				t.Fatal("read returned an incomplete publication")
			}
			if published {
				if err := store.Put(t.Context(), workspace, ref, changed); !errors.Is(err, workspacefiles.ErrConflict) {
					t.Fatal("recovery allowed a mismatch against the published bytes")
				}
			}
			if err := store.Put(t.Context(), workspace, ref, data); err != nil {
				t.Fatal("matching retry could not recover interrupted publication")
			}
			requireContent(t, store, workspace, data)
			if _, err := os.Lstat(stagedPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("successful retry left a pending file")
			}
		})
	}
}

func TestFileAndDirectorySafety(t *testing.T) {
	for _, damage := range []string{"symlink", "hard link", "fifo", "directory", "public file", "public scope", "public root", "oversize file"} {
		t.Run(damage, func(t *testing.T) {
			root := fixture(t)
			store := openStore(t, root)
			if err := store.Put(t.Context(), workspace, ref, data); err != nil {
				t.Fatal("fixture publication failed")
			}
			dir := filepath.Join(root, workspace)
			file := filepath.Join(dir, name)
			var err error
			switch damage {
			case "symlink", "fifo", "directory":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				switch damage {
				case "symlink":
					outside := filepath.Join(t.TempDir(), "outside")
					if err := os.WriteFile(outside, []byte(data), 0600); err != nil {
						t.Fatal(err)
					}
					err = os.Symlink(outside, file)
				case "fifo":
					err = syscall.Mkfifo(file, 0600)
				case "directory":
					err = os.Mkdir(file, 0700)
				}
			case "hard link":
				err = os.Link(file, filepath.Join(dir, "unrelated"))
			case "public file":
				err = os.Chmod(file, 0644)
			case "public scope":
				err = os.Chmod(dir, 0755)
			case "public root":
				err = os.Chmod(root, 0755)
			case "oversize file":
				err = os.WriteFile(file, []byte(strings.Repeat("x", 32*1024+1)), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.Read(t.Context(), workspace, ref); !errors.Is(err, workspacefiles.ErrUnavailable) {
				t.Fatal("unsafe stored content was readable")
			}
			if err = store.Put(t.Context(), workspace, ref, data); !errors.Is(err, workspacefiles.ErrUnavailable) {
				t.Fatal("unsafe stored content was accepted as a valid publication")
			}
		})
	}
}

func TestRejectCrossScopeSymlinkAndBadInputs(t *testing.T) {
	root := fixture(t)
	store := openStore(t, root)
	if err := store.Put(t.Context(), workspace, ref, data); err != nil {
		t.Fatal("fixture publication failed")
	}
	if err := os.Symlink(filepath.Join(root, workspace), filepath.Join(root, other)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(t.Context(), other, ref); !errors.Is(err, workspacefiles.ErrUnavailable) {
		t.Fatal("workspace symlink gave cross-scope access")
	}
	if err := store.Put(t.Context(), other, ref, changed); !errors.Is(err, workspacefiles.ErrUnavailable) {
		t.Fatal("workspace symlink gave cross-scope write access")
	}
	link := filepath.Join(t.TempDir(), "owner-link")
	if err := os.Symlink(filepath.Dir(root), link); err != nil {
		t.Fatal(err)
	}
	if opened, err := workspacefiles.New(filepath.Join(link, filepath.Base(root))); !errors.Is(err, workspacefiles.ErrConfiguration) {
		_ = opened.Close()
		t.Fatal("root resolution followed a symlink path component")
	}
	for _, invalid := range []string{"../" + ref, strings.ToUpper(ref), "00000000-0000-0000-0000-000000000000"} {
		if err := store.Put(t.Context(), workspace, invalid, data); !errors.Is(err, workspacefiles.ErrUnavailable) {
			t.Fatal("invalid content reference was accepted")
		}
	}
	for _, invalid := range []string{"", strings.Repeat("x", 32*1024+1)} {
		if err := store.Put(t.Context(), workspace, ref, invalid); !errors.Is(err, workspacefiles.ErrUnavailable) {
			t.Fatal("unbounded content was accepted")
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.Read(cancelled, workspace, ref); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled read did not fail closed")
	}
	if err := store.Put(cancelled, workspace, ref, changed); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled write did not fail closed")
	}
	requireContent(t, store, workspace, data)
}
