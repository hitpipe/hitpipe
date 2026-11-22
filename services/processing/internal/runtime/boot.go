package runtime

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

// startupBoot consumes a provisioned incarnation. IAM still has to authenticate
// registration and return a fresh OPEN snapshot before protected work can start.
func startupBoot(c Config, clientLeaf []byte) (string, error) {
	if c.BootIdentity == "" {
		boot, err := drafts.NewID()
		if err != nil {
			return "", ErrUnavailable
		}
		return boot, nil
	}

	dir := filepath.Dir(c.BootIdentity)
	if resolved, err := filepath.EvalSymlinks(dir); err != nil || resolved != dir {
		return "", ErrConfig
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", ErrConfig
	}
	defer root.Close()
	parent, err := root.Open(".")
	if err != nil {
		return "", ErrConfig
	}
	defer parent.Close()
	info, err := parent.Stat()
	if err != nil || !info.IsDir() || !ownedMode(info, 0700) {
		return "", ErrConfig
	}
	name := filepath.Base(c.BootIdentity)
	info, err = root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !ownedMode(info, 0600) || info.Size() > 256 {
		return "", ErrConfig
	}
	file, err := root.Open(name)
	if err != nil {
		return "", ErrConfig
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return "", ErrConfig
	}
	raw, err := io.ReadAll(io.LimitReader(file, 257))
	if err != nil || len(raw) > 256 {
		return "", ErrConfig
	}
	// Exact, LF-terminated records avoid duplicate keys or alternate spellings.
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 5 || lines[4] != "" || lines[0] != c.Identity ||
		lines[1] != strconv.FormatUint(c.Generation, 10) ||
		!drafts.UUID(lines[2]) || lines[3] != fmt.Sprintf("sha256:%x", sha256.Sum256(clientLeaf)) {
		return "", ErrConfig
	}
	boot := lines[2]
	if boot == "00000000-0000-0000-0000-000000000000" {
		return "", ErrConfig
	}
	claim, err := root.OpenFile(".boot-used-"+boot, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return "", ErrConfig
		}
		return "", ErrUnavailable
	}
	if err = claim.Sync(); err != nil {
		claim.Close()
		return "", ErrUnavailable
	}
	if claim.Close() != nil || parent.Sync() != nil {
		return "", ErrUnavailable
	}
	return boot, nil
}

func ownedMode(info os.FileInfo, permissions os.FileMode) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && info.Mode().Perm() == permissions
}
