package profilefiles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

const ref = "550e8400-e29b-41d4-a716-446655440001"
const valid = `{"user_id":"550e8400-e29b-41d4-a716-446655440002","display_name":"Студент","timezone":"local custom zone","revision":1}`

func digest(data string) string {
	sum := sha256.Sum256([]byte(data))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func fixture(t *testing.T, data string) (string, *Reader) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ref+".json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	r, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return dir, r
}

func TestProfileContentStrictFormatAndExactDigest(t *testing.T) {
	_, reader := fixture(t, valid)
	profile, err := reader.Read(context.Background(), ref, digest(valid))
	if err != nil || profile.DisplayName != "Студент" || profile.Timezone != "local custom zone" || profile.Revision != 1 {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if _, err := reader.Read(context.Background(), ref, digest(valid+" ")); !errors.Is(err, ErrUnavailable) {
		t.Fatal("wrong digest accepted")
	}
	for name, data := range map[string]string{
		"unknown":        strings.Replace(valid, `"revision":1`, `"revision":1,"role":"ADMIN"`, 1),
		"duplicate":      strings.Replace(valid, `"revision":1`, `"revision":1,"revision":2`, 1),
		"null":           strings.Replace(valid, `"revision":1`, `"revision":null`, 1),
		"missing":        strings.Replace(valid, `,"revision":1`, "", 1),
		"fraction":       strings.Replace(valid, `"revision":1`, `"revision":1.5`, 1),
		"unsafe integer": strings.Replace(valid, `"revision":1`, `"revision":9007199254740992`, 1),
		"empty":          strings.Replace(valid, `Студент`, "", 1),
		"long name":      strings.Replace(valid, `Студент`, strings.Repeat("я", 121), 1),
		"long zone":      strings.Replace(valid, `local custom zone`, strings.Repeat("я", 81), 1),
		"invalid UUID":   strings.Replace(valid, `550e8400-e29b-41d4-a716-446655440002`, "../escape", 1),
		"invalid UTF8":   strings.Replace(valid, `Студент`, string([]byte{255}), 1),
		"trailing":       valid + "{}",
		"oversize":       valid + strings.Repeat(" ", MaxBytes),
	} {
		t.Run(name, func(t *testing.T) {
			_, r := fixture(t, data)
			if _, err := r.Read(context.Background(), ref, digest(data)); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("invalid content accepted: %v", err)
			}
		})
	}
}

func TestProfileContentFilesystemBoundary(t *testing.T) {
	for _, mode := range []string{"symlink", "hardlink", "directory", "fifo", "public file", "public root", "missing"} {
		t.Run(mode, func(t *testing.T) {
			dir, r := fixture(t, valid)
			path := filepath.Join(dir, ref+".json")
			switch mode {
			case "symlink":
				outside := filepath.Join(t.TempDir(), "private.json")
				os.WriteFile(outside, []byte(valid), 0600)
				os.Remove(path)
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(path, filepath.Join(dir, "other")); err != nil {
					t.Fatal(err)
				}
			case "directory":
				os.Remove(path)
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				os.Remove(path)
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "public file":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "public root":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "missing":
				os.Remove(path)
			}
			if _, err := r.Read(context.Background(), ref, digest(valid)); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("unsafe file accepted: %v", err)
			}
		})
	}
	dir, r := fixture(t, valid)
	for _, value := range []string{"../" + ref, ref + "/child", strings.ToUpper(ref), "00000000-0000-0000-0000-000000000000"} {
		if _, err := r.Read(context.Background(), value, digest(valid)); !errors.Is(err, ErrUnavailable) {
			t.Fatal("unsafe ref accepted")
		}
	}
	link := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := New(link); !errors.Is(err, ErrConfiguration) {
		t.Fatal("symlink root accepted")
	}
	if _, err := New(filepath.Join(link, "child")); !errors.Is(err, ErrConfiguration) {
		t.Fatal("symlink ancestor accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Read(ctx, ref, digest(valid)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	r.Close()
	if _, err := r.Read(context.Background(), ref, digest(valid)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("closed reader accepted")
	}
}

func TestProfileContentNoCachedFallbackAfterChange(t *testing.T) {
	dir, r := fixture(t, valid)
	if _, err := r.Read(context.Background(), ref, digest(valid)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ref+".json"), []byte(strings.Replace(valid, "Студент", "Другой", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(context.Background(), ref, digest(valid)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("stale content fallback")
	}
	if err := os.Remove(filepath.Join(dir, ref+".json")); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Read(context.Background(), ref, digest(valid)); !errors.Is(err, ErrUnavailable) {
		t.Fatal("deleted content fallback")
	}
}

func TestProfileContentRejectsUnpairedUnicodeWithoutRepairingIt(t *testing.T) {
	for _, value := range []string{`\uD800`, `\uDC00`, `\uD83D\u0041`, `\uD83D\\uDE00`} {
		for _, field := range []string{"display_name", "timezone"} {
			data := strings.Replace(valid, `"Студент"`, `"`+value+`"`, 1)
			if field == "timezone" {
				data = strings.Replace(valid, `"local custom zone"`, `"`+value+`"`, 1)
			}
			_, reader := fixture(t, data)
			if _, err := reader.Read(context.Background(), ref, digest(data)); !errors.Is(err, ErrUnavailable) {
				t.Fatal("unpaired UTF-16 escape silently repaired")
			}
		}
	}
	for _, tc := range []struct{ value, want string }{
		{`\uD83D\uDE00`, "😀"},
		{`\\uD800`, `\uD800`},
		{`\uFFFD`, "�"},
	} {
		data := strings.Replace(valid, `"Студент"`, `"`+tc.value+`"`, 1)
		_, reader := fixture(t, data)
		profile, err := reader.Read(context.Background(), ref, digest(data))
		if err != nil || profile.DisplayName != tc.want {
			t.Fatal("valid Unicode pair/literal escape rejected")
		}
	}
}
