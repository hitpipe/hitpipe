//go:build linux

package casedeletion

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
)

const fixtureWorkspace = "ef031c28-65fc-40c0-84f3-721aa2583b3b"

func emptyState(t *testing.T) app.State {
	t.Helper()
	state, err := app.NewState("test")
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func fixtureStore(t *testing.T) (string, *Store) {
	t.Helper()
	dir := t.TempDir()
	// Go creates t.TempDir ancestors with differing permissions. The
	// dedicated final publisher directory has a strict mode.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := Initialize(dir, emptyState(t))
	if err != nil {
		t.Fatal(err)
	}
	return dir, s
}

func publishWorkspace(s *app.State) error {
	s.Revision++
	s.Workspaces[fixtureWorkspace] = app.Workspace{
		Generation: 1,
		Revision:   s.Revision,
		Decisions:  make(map[string]app.Decision),
	}
	return nil
}

func replaceFile(t *testing.T, dir, name string, payload []byte) {
	t.Helper()
	tmp, err := os.CreateTemp(dir, "replacement-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, dir, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func expectUnavailable(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Read(t.Context(), func(app.State) error {
		t.Error("invalid registry invoked read callback")
		return nil
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Read = %v; want unavailable", err)
	}
	if err := s.Change(t.Context(), func(*app.State) error {
		t.Error("invalid registry invoked update callback")
		return nil
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Change = %v; want unavailable", err)
	}
}

func TestRegistryCommitAndFreshOpen(t *testing.T) {
	dir, s := fixtureStore(t)
	before := readFile(t, dir, stateName)
	if _, err := Initialize(dir, emptyState(t)); !errors.Is(err, ErrInitialized) {
		t.Fatalf("duplicate initialization: %v", err)
	}
	t.Run("nonempty directory seed refused", func(t *testing.T) {
		seed := emptyState(t)
		_ = publishWorkspace(&seed)
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Initialize(dir, seed); !errors.Is(err, ErrInvalid) {
			t.Fatalf("previous state as initialization seed: %v", err)
		}
		nodes, err := os.ReadDir(dir)
		if err != nil || len(nodes) != 0 {
			t.Fatal("invalid seed provisioned source")
		}
	})
	aborted := errors.New("caller rejected")
	err := s.Change(t.Context(), func(st *app.State) error {
		_ = publishWorkspace(st)
		return aborted
	})
	if !errors.Is(err, aborted) || !bytes.Equal(readFile(t, dir, stateName), before) {
		t.Fatal("rejected transaction published a state")
	}
	if err := s.Change(t.Context(), func(st *app.State) error {
		st.Revision += 2
		return nil
	}); !errors.Is(err, ErrInvalid) || !bytes.Equal(readFile(t, dir, stateName), before) {
		t.Fatal("nonconsecutive revision published a state")
	}
	if err := s.Change(t.Context(), func(st *app.State) error {
		st.Environment = "other-test"
		st.Revision++
		return nil
	}); !errors.Is(err, ErrInvalid) || !bytes.Equal(readFile(t, dir, stateName), before) {
		t.Fatal("publisher environment change published a state")
	}
	if err := s.Change(t.Context(), publishWorkspace); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, dir, stateName)
	if bytes.Equal(after, before) {
		t.Fatal("successful transaction did not publish state")
	}
	fresh, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = fresh.Read(t.Context(), func(st app.State) error {
		ws, exists := st.Workspaces[fixtureWorkspace]
		if !exists || st.Revision != 2 || ws.Revision != 2 || ws.Generation != 1 {
			return errors.New("fresh publisher did not read committed state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Change(t.Context(), func(*app.State) error { return nil }); err != nil ||
		!bytes.Equal(readFile(t, dir, stateName), after) {
		t.Fatalf("no-op changed committed state: %v", err)
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 2 {
		t.Fatalf("unexpected publisher state nodes: %v, %v", children, err)
	}
}

func TestRegistryRefusesMissingAmbiguousAndSubstitutedNodes(t *testing.T) {
	t.Run("absent source never initialized by Open", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(dir); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("empty Open: %v", err)
		}
		children, err := os.ReadDir(dir)
		if err != nil || len(children) != 0 {
			t.Fatal("Open provisioned files")
		}
	})
	t.Run("link to directory", func(t *testing.T) {
		dir, _ := fixtureStore(t)
		link := filepath.Join(t.TempDir(), "another-root")
		if err := os.Symlink(filepath.Dir(dir), link); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(filepath.Join(link, filepath.Base(dir))); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("symlinked parent Open: %v", err)
		}
	})
	for _, test := range []struct {
		name     string
		modifier func(*testing.T, string)
	}{
		{"absent active JSON", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, stateName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink active JSON", func(t *testing.T, dir string) {
			old := filepath.Join(dir, "backup")
			if err := os.Rename(filepath.Join(dir, stateName), old); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(old, filepath.Join(dir, stateName)); err != nil {
				t.Fatal(err)
			}
		}},
		{"hardlink active JSON", func(t *testing.T, dir string) {
			if err := os.Link(filepath.Join(dir, stateName), filepath.Join(dir, "backup")); err != nil {
				t.Fatal(err)
			}
		}},
		{"wrong permissions", func(t *testing.T, dir string) {
			if err := os.Chmod(filepath.Join(dir, stateName), 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"FIFO active JSON", func(t *testing.T, dir string) {
			target := filepath.Join(dir, stateName)
			if err := os.Remove(target); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(target, 0600); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate JSON", func(t *testing.T, dir string) {
			old := readFile(t, dir, stateName)
			payload := bytes.Replace(old, []byte(`"revision":1`), []byte(`"revision":1,"revision":1`), 1)
			replaceFile(t, dir, stateName, payload)
		}},
		{"trailing JSON", func(t *testing.T, dir string) {
			replaceFile(t, dir, stateName, append(readFile(t, dir, stateName), []byte("{}\n")...))
		}},
		{"oversized state", func(t *testing.T, dir string) {
			replaceFile(t, dir, stateName, bytes.Repeat([]byte{' '}, maxState+1))
		}},
		{"different source's active JSON", func(t *testing.T, dir string) {
			other, _ := fixtureStore(t)
			replaceFile(t, dir, stateName, readFile(t, other, stateName))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, s := fixtureStore(t)
			test.modifier(t, dir)
			expectUnavailable(t, s)
			if _, err := Open(dir); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("fresh Open = %v; want unavailable", err)
			}
		})
	}
	t.Run("same-content lock replaced", func(t *testing.T) {
		dir, s := fixtureStore(t)
		replaceFile(t, dir, lockName, readFile(t, dir, lockName))
		expectUnavailable(t, s)
		if _, err := Open(dir); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("fresh publisher accepted replacement lock: %v", err)
		}
	})
	t.Run("same-incarnation rollback detected", func(t *testing.T) {
		dir, s := fixtureStore(t)
		saved := readFile(t, dir, stateName)
		if err := s.Change(t.Context(), publishWorkspace); err != nil {
			t.Fatal(err)
		}
		replaceFile(t, dir, stateName, saved)
		expectUnavailable(t, s)
	})
	t.Run("same revision competing contents detected", func(t *testing.T) {
		dir, s := fixtureStore(t)
		alternate, err := app.NewState("other-test")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := s.encode(alternate)
		if err != nil {
			t.Fatal(err)
		}
		replaceFile(t, dir, stateName, raw)
		expectUnavailable(t, s)
	})
}

func TestRegistryReadBarrierAcrossProcesses(t *testing.T) {
	dir, s := fixtureStore(t)
	unlock := make(chan struct{})
	var unblock sync.Once
	release := func() { unblock.Do(func() { close(unlock) }) }
	defer release()
	locked := make(chan struct{})
	reader := make(chan error, 1)
	go func() {
		reader <- s.Read(t.Context(), func(state app.State) error {
			close(locked)
			<-unlock
			if state.Revision != 1 {
				return errors.New("restore guarded wrong revision")
			}
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-reader:
		t.Fatalf("restore failed to acquire barrier: %v", err)
	case <-time.After(4 * time.Second):
		t.Fatal("restore did not start")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRegistryWriterChild$")
	cmd.Env = []string{"HIT154_TEST_REGISTRY_DIR=" + dir, "HIT154_TEST_REGISTRY_SOURCE=" + s.source}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	type line struct {
		text  string
		ended bool
	}
	output := make(chan line, 4)
	go func() {
		buf := bufio.NewScanner(stdout)
		for buf.Scan() {
			select {
			case output <- line{text: buf.Text()}:
			case <-ctx.Done():
				return
			}
		}
		select {
		case output <- line{ended: true}:
		case <-ctx.Done():
		}
	}()
	defer func() {
		release()
		if cmd.ProcessState == nil {
			cancel()
			_ = cmd.Wait()
		}
	}()
	next := func() line {
		select {
		case m := <-output:
			return m
		case <-ctx.Done():
			t.Fatalf("child unresponsive: %v", ctx.Err())
			return line{}
		}
	}
	if first := next(); first.ended || first.text != "writer ready" {
		t.Fatalf("writer did not reach barrier: %+v", first)
	}
	waiterCtx, stop := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer stop()
	if err := s.Change(waiterCtx, func(*app.State) error {
		t.Error("update ran in same process while restore held reader lock")
		return nil
	}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("did not honor timeout waiting for shared restore guard: %v", err)
	}
	select {
	case item := <-output:
		t.Fatalf("writer obtained guard before restore opened: %+v", item)
	default:
	}
	release()
	if err := <-reader; err != nil {
		t.Fatal(err)
	}
	if got := next(); got.ended || got.text != "writer published revision 2" {
		t.Fatalf("writer did not publish after release: %+v", got)
	}
	for item := next(); !item.ended; item = next() {
		t.Log(item.text)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := s.Read(t.Context(), func(state app.State) error {
		if state.Revision != 2 || state.Workspaces[fixtureWorkspace].Generation != 1 {
			return errors.New("resident publisher missed external committed revision")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Log("shared restore read prevented a second process from publishing until after guarded open")
}

func TestRegistryWriterChild(t *testing.T) {
	dir := os.Getenv("HIT154_TEST_REGISTRY_DIR")
	if dir == "" {
		return
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.source != os.Getenv("HIT154_TEST_REGISTRY_SOURCE") {
		t.Fatal("separate writer is limited to its synthetic source")
	}
	fmt.Println("writer ready")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := s.Change(ctx, func(state *app.State) error {
		if state.Environment != "test" || state.Revision != 1 || len(state.Workspaces) != 0 {
			return errors.New("test writer refused non-fixture store")
		}
		return publishWorkspace(state)
	}); err != nil {
		t.Fatal(err)
	}
	fmt.Println("writer published revision 2")
}
