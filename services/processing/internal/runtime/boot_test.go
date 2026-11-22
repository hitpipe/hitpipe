package runtime

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupBootBindsProvisioningAndClaimsOnce(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	c := Config{Identity: "11111111-1111-4111-8111-111111111111", Generation: 5,
		BootIdentity: filepath.Join(dir, "identity")}
	boot := "22222222-2222-4222-8222-222222222222"
	leaf := []byte("synthetic client leaf")
	record := fmt.Sprintf("%s\n%d\n%s\nsha256:%x\n", c.Identity, c.Generation, boot, sha256.Sum256(leaf))
	if err := os.WriteFile(c.BootIdentity, []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	wrong := c
	wrong.Generation++
	if _, err := startupBoot(wrong, leaf); !errors.Is(err, ErrConfig) {
		t.Fatalf("wrong generation was admitted: %v", err)
	}
	if _, err := startupBoot(c, []byte("different leaf")); !errors.Is(err, ErrConfig) {
		t.Fatalf("wrong certificate was admitted: %v", err)
	}
	link := c
	link.BootIdentity = filepath.Join(dir, "link")
	if err := os.Symlink(c.BootIdentity, link.BootIdentity); err != nil {
		t.Fatal(err)
	}
	if _, err := startupBoot(link, leaf); !errors.Is(err, ErrConfig) {
		t.Fatalf("symlink was admitted: %v", err)
	}
	if err := os.Chmod(c.BootIdentity, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := startupBoot(c, leaf); !errors.Is(err, ErrConfig) {
		t.Fatalf("shared file was admitted: %v", err)
	}
	if err := os.Chmod(c.BootIdentity, 0600); err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		got string
		err error
	}
	started := make(chan outcome, 2)
	for range 2 {
		go func() {
			id, err := startupBoot(c, leaf)
			started <- outcome{id, err}
		}()
	}
	success := 0
	for range 2 {
		o := <-started
		if o.err == nil && o.got == boot {
			success++
		} else if !errors.Is(o.err, ErrConfig) {
			t.Fatalf("unexpected claim: %v", o.err)
		}
	}
	if success != 1 {
		t.Fatalf("boot was claimed %d times", success)
	}
}
