package contentstore

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func setup(t *testing.T) (*Store, string) {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if e := os.Chmod(p, 0700); e != nil {
		t.Fatal(e)
	}
	s, e := Open(p, func() time.Time { return time.Unix(1800000000, 0) })
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s, p
}
func expiry() time.Time { return time.Unix(1800003600, 0) }
func TestRoundtripPermissionsOrderingExpiry(t *testing.T) {
	s, p := setup(t)
	var steps []string
	s.fault = func(x string) error { steps = append(steps, x); return nil }
	r, e := s.Put([]byte("synthetic"), expiry())
	if e != nil {
		t.Fatal(e)
	}
	if !valid(r.ID) {
		t.Fatal("name")
	}
	if len(steps) != 4 || steps[0] != "write" || steps[1] != "file_sync" || steps[2] != "rename" || steps[3] != "directory_sync" {
		t.Fatal(steps)
	}
	st, _ := os.Stat(filepath.Join(p, r.ID))
	if st.Mode().Perm() != 0600 {
		t.Fatal("mode")
	}
	b, e := s.Read(r)
	if e != nil || string(b) != "synthetic" {
		t.Fatal(e)
	}
	r2, e := s.Put(b, expiry())
	if e != nil || r.ID == r2.ID {
		t.Fatal("random identity")
	}
	s.now = func() time.Time { return expiry() }
	if b, e := s.Read(r); e == nil || b != nil {
		t.Fatal("expired read")
	}
}
func TestInterruptedStagesExposeActualFilesAndPurge(t *testing.T) {
	for _, stage := range []string{"write", "file_sync", "rename", "directory_sync"} {
		t.Run(stage, func(t *testing.T) {
			s, p := setup(t)
			s.fault = func(x string) error {
				if x == stage {
					return errors.New("synthetic fault")
				}
				return nil
			}
			r, e := s.Put([]byte("synthetic"), expiry())
			if e == nil || r != (Ref{}) {
				t.Fatal("false success")
			}
			files, e := os.ReadDir(p)
			if e != nil || len(files) != 1 {
				t.Fatal("actual residual files", files, e)
			}
			b, e := os.ReadFile(filepath.Join(p, files[0].Name()))
			if e != nil {
				t.Fatal(e)
			}
			if stage == "write" {
				if len(b) != 0 {
					t.Fatal("unexpected bytes")
				}
			} else if string(b) != "synthetic" {
				t.Fatal("unexpected residual")
			}
			s.fault = nil
			if n, e := s.Purge(map[string]bool{}); e != nil || n != 1 {
				t.Fatal(n, e)
			}
			files, _ = os.ReadDir(p)
			if len(files) != 0 {
				t.Fatal("orphan retained")
			}
		})
	}
}
func TestTraversalSymlinkAndTamper(t *testing.T) {
	s, p := setup(t)
	r, e := s.Put([]byte("synthetic"), expiry())
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"../outside", "/etc/passwd", "", ".tmp-" + r.ID} {
		bad := r
		bad.ID = id
		if _, e := s.Read(bad); e == nil {
			t.Fatal("path accepted")
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("outside"), 0600)
	os.Remove(filepath.Join(p, r.ID))
	if e = os.Symlink(outside, filepath.Join(p, r.ID)); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(r); e == nil {
		t.Fatal("symlink read")
	}
	if _, e = s.Purge(map[string]bool{}); e == nil {
		t.Fatal("symlink purge accepted")
	}
	b, _ := os.ReadFile(outside)
	if string(b) != "outside" {
		t.Fatal("outside changed")
	}
	os.Remove(filepath.Join(p, r.ID))
	os.WriteFile(filepath.Join(p, r.ID), []byte("different"), 0600)
	if _, e = s.Read(r); e == nil {
		t.Fatal("digest bypass")
	}
}
func TestRootAndBoundsAndReferenceSnapshot(t *testing.T) {
	s, p := setup(t)
	if _, e := Open(p, nil); e == nil {
		t.Fatal("unknown clock")
	}
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(p, link)
	if x, e := Open(link, s.now); e == nil {
		x.Close()
		t.Fatal("symlink root")
	}
	os.Chmod(p, 0755)
	if x, e := Open(p, s.now); e == nil {
		x.Close()
		t.Fatal("permissive root")
	}
	os.Chmod(p, 0700)
	for _, data := range [][]byte{nil, bytes.Repeat([]byte("a"), MaxBytes+1)} {
		if _, e := s.Put(data, expiry()); e == nil {
			t.Fatal("size")
		}
	}
	r, e := s.Put(bytes.Repeat([]byte("a"), MaxBytes), expiry())
	if e != nil {
		t.Fatal(e)
	}
	if n, e := s.Purge(map[string]bool{r.ID: true}); n != 0 || e != nil {
		t.Fatal("linked deleted")
	}
	if _, e = s.Purge(nil); e == nil {
		t.Fatal("unknown snapshot")
	}
	if _, e = s.Put([]byte("a"), s.now()); e == nil {
		t.Fatal("expiry")
	}
	os.Chmod(filepath.Join(p, r.ID), 0644)
	if _, e = s.Read(r); e == nil {
		t.Fatal("permissive file")
	}
}

func TestProcessExitBeforeCommitStages(t *testing.T) {
	if p := os.Getenv("HIT74_CHILD_ROOT"); p != "" {
		s, e := Open(p, func() time.Time { return time.Unix(1800000000, 0) })
		if e != nil {
			os.Exit(3)
		}
		s.fault = func(stage string) error {
			if stage == os.Getenv("HIT74_CHILD_STAGE") {
				os.Exit(23)
			}
			return nil
		}
		s.Put([]byte("synthetic"), expiry())
		os.Exit(4)
	}
	for _, stage := range []string{"write", "file_sync", "rename", "directory_sync"} {
		t.Run(stage, func(t *testing.T) {
			s, p := setup(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestProcessExitBeforeCommitStages$")
			cmd.Env = append(os.Environ(), "HIT74_CHILD_ROOT="+p, "HIT74_CHILD_STAGE="+stage)
			e := cmd.Run()
			if e == nil {
				t.Fatal("child did not stop")
			}
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 23 {
				t.Fatal("unexpected child exit", e)
			}
			files, e := os.ReadDir(p)
			if e != nil || len(files) != 1 {
				t.Fatal("residual", files, e)
			}
			b, e := os.ReadFile(filepath.Join(p, files[0].Name()))
			if e != nil {
				t.Fatal(e)
			}
			if stage == "write" {
				if len(b) != 0 {
					t.Fatal("bytes")
				}
			} else if string(b) != "synthetic" {
				t.Fatal("bytes")
			}
			if n, e := s.Purge(map[string]bool{}); n != 1 || e != nil {
				t.Fatal("purge", n, e)
			}
		})
	}
}
func TestClosedFilesystemFails(t *testing.T) {
	s, _ := setup(t)
	s.Close()
	if r, e := s.Put([]byte("synthetic"), expiry()); e == nil || r != (Ref{}) {
		t.Fatal("closed filesystem accepted")
	}
}
