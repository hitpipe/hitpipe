package contentfiles

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

func private(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "content")
	if e := os.Mkdir(p, 0700); e != nil {
		t.Fatal(e)
	}
	canonical, e := filepath.EvalSymlinks(p)
	if e != nil {
		t.Fatal(e)
	}
	return canonical
}
func open(t *testing.T, p string) *Store {
	t.Helper()
	s, e := New(p)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func TestDurableContentAndRestart(t *testing.T) {
	p := private(t)
	s := open(t, p)
	now := time.Now().UTC()
	var steps []string
	s.Step = func(name string) error { steps = append(steps, name); return nil }
	ref, e := s.Write(context.Background(), []byte(`{"synthetic":true}`), "DELIVERY_INTENT", now.Add(time.Minute), now)
	if e != nil {
		t.Fatal(e)
	}
	if want := []string{"write", "file-fsync", "rename", "directory-fsync"}; fmtJSON(steps) != fmtJSON(want) {
		t.Fatal(steps)
	}
	if ref.ExpiresAt.Nanosecond()%1000 != 0 {
		t.Fatal("expiry incompatible with SQL precision")
	}
	info, e := os.Stat(filepath.Join(p, ref.ID))
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("unsafe file permissions")
	}
	b, e := open(t, p).Read(context.Background(), ref, now)
	if e != nil || string(b) != `{"synthetic":true}` {
		t.Fatal("reopen lost durable content", e)
	}
}
func fmtJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func TestFaultStepsNeverReturnAcceptedRef(t *testing.T) {
	for _, step := range []string{"write", "file-fsync", "rename", "directory-fsync"} {
		t.Run(step, func(t *testing.T) {
			p := private(t)
			s := open(t, p)
			now := time.Now()
			s.Step = func(n string) error {
				if n == step {
					return errors.New("injected storage fault")
				}
				return nil
			}
			ref, e := s.Write(context.Background(), []byte(`"synthetic"`), "HISTORY", now.Add(time.Second), now)
			if e == nil || ref != (preparecase.Ref{}) {
				t.Fatal("fault positively acknowledged")
			}
			restarted := open(t, p)
			if e = restarted.PurgeExpired(now.Add(2 * time.Second)); e != nil {
				t.Fatal(e)
			}
			entries, e := os.ReadDir(p)
			if e != nil || len(entries) != 0 {
				t.Fatal("expired fault orphan remains", entries, e)
			}
		})
	}
}
func TestExpiryCorruptionTraversalAndSymlink(t *testing.T) {
	p := private(t)
	s := open(t, p)
	now := time.Now()
	ctx := context.Background()
	ref, e := s.Write(ctx, []byte(`"synthetic"`), "CASE_CONTEXT", now.Add(time.Minute), now)
	if e != nil {
		t.Fatal(e)
	}
	forged := ref
	forged.ID = "../outside"
	if _, e = s.Read(ctx, forged, now); e == nil {
		t.Fatal("traversal accepted")
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("not content"), 0600)
	os.Remove(filepath.Join(p, ref.ID))
	os.Symlink(outside, filepath.Join(p, ref.ID))
	if _, e = s.Read(ctx, ref, now); e == nil {
		t.Fatal("symlink followed")
	}
	os.Remove(filepath.Join(p, ref.ID))
	ref, e = s.Write(ctx, []byte(`"synthetic"`), "CASE_CONTEXT", now.Add(time.Minute), now)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(filepath.Join(p, ref.ID))
	raw[len(raw)-2] ^= 1
	os.WriteFile(filepath.Join(p, ref.ID), raw, 0600)
	if _, e = s.Read(ctx, ref, now); e == nil {
		t.Fatal("corrupt digest accepted")
	}
	// Intact protected expiry metadata still bounds a corrupt body without SQL.
	_ = s.PurgeExpired(now.Add(2 * time.Minute))
	if _, e = os.Stat(filepath.Join(p, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("expired corrupt body retained")
	}
	ref, e = s.Write(ctx, []byte("x"), "DELIVERY_INTENT", now.Add(time.Second), now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Read(ctx, ref, now.Add(time.Second)); e == nil {
		t.Fatal("expiry equality allowed")
	}
	if _, e = os.Stat(filepath.Join(p, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("expired content still exists")
	}
	wide := filepath.Join(t.TempDir(), "wide")
	os.Mkdir(wide, 0755)
	canonicalWide, _ := filepath.EvalSymlinks(wide)
	if _, e = New(canonicalWide); e == nil {
		t.Fatal("public directory accepted")
	}
}
func TestUncertainCommitOrphanRetainedUntilExpiry(t *testing.T) {
	p := private(t)
	s := open(t, p)
	now := time.Now()
	ref, e := s.Write(context.Background(), []byte("x"), "DELIVERY_INTENT", now.Add(time.Minute), now)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.PurgeExpired(now); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(p, ref.ID)); e != nil {
		t.Fatal("unexpired possibly referenced file was deleted")
	}
}
func TestContentProcessRestart(t *testing.T) {
	if os.Getenv("HIT79_FILE_HELPER") == "1" {
		s, e := New(os.Getenv("HIT79_FILE_ROOT"))
		if e != nil {
			os.Exit(2)
		}
		now := time.Now()
		r, e := s.Write(context.Background(), []byte(`"synthetic"`), "HISTORY", now.Add(time.Minute), now)
		if e != nil {
			os.Exit(3)
		}
		json.NewEncoder(os.Stdout).Encode(r)
		os.Exit(0)
	}
	p := private(t)
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(binary, "-test.run=^TestContentProcessRestart$")
	cmd.Env = append(os.Environ(), "HIT79_FILE_HELPER=1", "HIT79_FILE_ROOT="+p)
	b, e := cmd.Output()
	if e != nil {
		t.Fatal(e)
	}
	var ref preparecase.Ref
	if e = json.Unmarshal(b, &ref); e != nil {
		t.Fatal(e)
	}
	got, e := open(t, p).Read(context.Background(), ref, time.Now())
	if e != nil || string(got) != `"synthetic"` {
		t.Fatal("process restart lost content", e)
	}
}

func TestCrashBetweenFileSteps(t *testing.T) {
	if step := os.Getenv("HIT79_CRASH_STEP"); step != "" {
		s, e := New(os.Getenv("HIT79_FILE_ROOT"))
		if e != nil {
			os.Exit(41)
		}
		s.Step = func(n string) error {
			if n == step {
				os.Exit(23)
			}
			return nil
		}
		now := time.Now()
		_, _ = s.Write(context.Background(), []byte(`"synthetic"`), "HISTORY", now.Add(time.Minute), now)
		os.Exit(42)
	}
	for _, step := range []string{"write", "file-fsync", "rename", "directory-fsync"} {
		t.Run(step, func(t *testing.T) {
			p := private(t)
			exe, e := os.Executable()
			if e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(exe, "-test.run=^TestCrashBetweenFileSteps$")
			cmd.Env = append(os.Environ(), "HIT79_CRASH_STEP="+step, "HIT79_FILE_ROOT="+p)
			e = cmd.Run()
			var exit *exec.ExitError
			if !errors.As(e, &exit) || exit.ExitCode() != 23 {
				t.Fatal("target process crash not reached", e)
			}
			files, e := os.ReadDir(p)
			if e != nil || len(files) != 1 {
				t.Fatal("unacknowledged crash file missing", files, e)
			}
			s := open(t, p)
			if e = s.PurgeExpired(time.Now().Add(2 * time.Minute)); e != nil {
				t.Fatal(e)
			}
			files, _ = os.ReadDir(p)
			if len(files) != 0 {
				t.Fatal("orphan survived hard expiry")
			}
		})
	}
}
