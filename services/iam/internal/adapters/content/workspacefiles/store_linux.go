// Package workspacefiles stores immutable IAM-owned workspace objects on Linux.
package workspacefiles

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var (
	ErrConfiguration = errors.New("workspace_content_configuration_invalid")
	ErrUnavailable   = errors.New("workspace_content_unavailable")
	ErrConflict      = errors.New("workspace_content_conflict")
)

const maxBytes = 32 * 1024
const directoryFlags = syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
const fileFlags = syscall.O_RDONLY | syscall.O_NONBLOCK | syscall.O_NOFOLLOW | syscall.O_CLOEXEC

type Store struct {
	mu   sync.RWMutex
	root *os.File
}

// New pins a pre-existing owner-only directory. No path component may be a
// symbolic link. The service must provide a local filesystem that supports
// directory fsync and advisory locks.
func New(root string) (*Store, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == "/" {
		return nil, ErrConfiguration
	}
	fd, err := syscall.Open("/", directoryFlags, 0)
	if err != nil {
		return nil, ErrConfiguration
	}
	for _, component := range strings.Split(strings.TrimPrefix(root, "/"), "/") {
		next, openErr := syscall.Openat(fd, component, directoryFlags, 0)
		_ = syscall.Close(fd)
		if openErr != nil {
			return nil, ErrConfiguration
		}
		fd = next
	}
	dir := os.NewFile(uintptr(fd), "iam-workspace-content")
	if !privateDir(dir) {
		_ = dir.Close()
		return nil, ErrConfiguration
	}
	return &Store{root: dir}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	if err != nil {
		return ErrUnavailable
	}
	return nil
}

// Put publishes the exact bytes in their workspace scope without replacement.
// A nil result means the file and both directory entries were synchronized.
// Failed or unconfirmed publication never authorizes a database reference.
func (s *Store) Put(ctx context.Context, workspace, ref, raw string) error {
	if !s.validRequest(ctx, workspace, ref) || len(raw) == 0 || len(raw) > maxBytes {
		return ErrUnavailable
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir, err := s.workspaceDir(ctx, workspace, true)
	if err != nil {
		return err
	}
	defer dir.Close()
	name, pending := ref+".json", ".pending-"+ref
	if found, err := acceptPublished(ctx, dir, name, raw); found || err != nil {
		if err != nil {
			return err
		}
		return s.syncDirectories(ctx, dir)
	}
	// A pending file without a published name belongs to a failed or
	// interrupted attempt. It is safe to remove under the same workspace lock.
	if err := removeUnpublished(dir, pending); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fd, err := syscall.Openat(int(dir.Fd()), pending, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return ErrUnavailable
	}
	file := os.NewFile(uintptr(fd), "iam-workspace-pending")
	defer file.Close()
	defer func() { _ = syscall.Unlinkat(int(dir.Fd()), pending) }()
	if file.Chmod(0600) != nil {
		return ErrUnavailable
	}
	if n, err := file.WriteString(raw); err != nil || n != len(raw) || file.Sync() != nil {
		return ErrUnavailable
	}
	if _, ok := privateFile(file); !ok {
		return ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// The exclusive per-workspace flock serializes every writer that has
	// access to this dedicated owner directory. Check the destination again
	// before atomically renaming the synced file, without following links.
	if existing, err := openFile(dir, name); !errors.Is(err, syscall.ENOENT) {
		if existing != nil {
			_ = existing.Close()
		}
		return ErrUnavailable
	}
	if syscall.Renameat(int(dir.Fd()), pending, int(dir.Fd()), name) != nil {
		return ErrUnavailable
	}
	if _, ok := privateFile(file); !ok {
		return ErrUnavailable
	}
	return s.syncDirectories(ctx, dir)
}

// Read returns the exact bytes of one regular, owner-only, singly linked file.
// The application verifies the content's envelope and authority separately.
func (s *Store) Read(ctx context.Context, workspace, ref string) (string, error) {
	if !s.validRequest(ctx, workspace, ref) {
		return "", ErrUnavailable
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	dir, err := s.workspaceDir(ctx, workspace, false)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	file, err := openFile(dir, ref+".json")
	if err != nil {
		return "", ErrUnavailable
	}
	defer file.Close()
	return readFile(ctx, file)
}

func (s *Store) validRequest(ctx context.Context, workspace, ref string) bool {
	return s != nil && ctx != nil && validID(workspace) && validID(ref)
}

func validID(id string) bool {
	return runtimeaccess.ValidUUID(id) && strings.ToLower(id) == id
}

func (s *Store) workspaceDir(ctx context.Context, workspace string, forWrite bool) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !privateDir(s.root) {
		return nil, ErrUnavailable
	}
	rootFD := int(s.root.Fd())
	lock := syscall.LOCK_SH
	if forWrite {
		if err := syscall.Mkdirat(rootFD, workspace, 0700); err != nil && !errors.Is(err, syscall.EEXIST) {
			return nil, ErrUnavailable
		}
		lock = syscall.LOCK_EX
	}
	fd, err := syscall.Openat(rootFD, workspace, directoryFlags, 0)
	if err != nil {
		return nil, ErrUnavailable
	}
	dir := os.NewFile(uintptr(fd), "iam-workspace-scope")
	if err := lockDir(ctx, dir, lock); err != nil {
		_ = dir.Close()
		return nil, err
	}
	if !privateDir(dir) {
		_ = dir.Close()
		return nil, ErrUnavailable
	}
	return dir, nil
}

// Each caller opens a separate directory description. flock therefore also
// serializes a concurrent publication from another Store or service process.
func lockDir(ctx context.Context, dir *os.File, mode int) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	err := syscall.Flock(int(dir.Fd()), mode|syscall.LOCK_NB)
	if err == nil {
		return ctx.Err()
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EINTR) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			err = syscall.Flock(int(dir.Fd()), mode|syscall.LOCK_NB)
			if err == nil {
				return ctx.Err()
			}
		}
	}
	return ErrUnavailable
}

func (s *Store) syncDirectories(ctx context.Context, dir *os.File) error {
	if !privateDir(dir) || !privateDir(s.root) || dir.Sync() != nil || s.root.Sync() != nil {
		return ErrUnavailable
	}
	return ctx.Err()
}

func privateDir(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil || info.Mode() != os.ModeDir|0700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid())
}

func privateRegular(file *os.File) (os.FileInfo, bool) {
	info, err := file.Stat()
	if err != nil || info.Mode() != 0600 {
		return nil, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return info, ok && stat.Uid == uint32(os.Geteuid()) && stat.Nlink == 1
}

func privateFile(file *os.File) (os.FileInfo, bool) {
	info, ok := privateRegular(file)
	return info, ok && info.Size() > 0 && info.Size() <= maxBytes
}

func openFile(dir *os.File, name string) (*os.File, error) {
	fd, err := syscall.Openat(int(dir.Fd()), name, fileFlags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), "iam-workspace-object"), nil
}

func readFile(ctx context.Context, file *os.File) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, ok := privateFile(file)
	if !ok {
		return "", ErrUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(data) > maxBytes || int64(len(data)) != info.Size() {
		return "", ErrUnavailable
	}
	current, ok := privateFile(file)
	if !ok || current.Size() != info.Size() || current.ModTime() != info.ModTime() {
		return "", ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(data), nil
}

func acceptPublished(ctx context.Context, dir *os.File, name, raw string) (bool, error) {
	file, err := openFile(dir, name)
	if errors.Is(err, syscall.ENOENT) {
		return false, nil
	}
	if err != nil {
		return true, ErrUnavailable
	}
	defer file.Close()
	data, err := readFile(ctx, file)
	if err != nil {
		return true, err
	}
	if data != raw {
		return true, ErrConflict
	}
	// Even a retry after uncertain directory fsync must prove durability.
	if file.Sync() != nil {
		return true, ErrUnavailable
	}
	if _, ok := privateFile(file); !ok {
		return true, ErrUnavailable
	}
	return true, nil
}

func removeUnpublished(dir *os.File, pending string) error {
	staged, err := openFile(dir, pending)
	if errors.Is(err, syscall.ENOENT) {
		return nil
	}
	if err != nil {
		return ErrUnavailable
	}
	_, ok := privateRegular(staged)
	_ = staged.Close()
	if !ok || syscall.Unlinkat(int(dir.Fd()), pending) != nil {
		return ErrUnavailable
	}
	return nil
}
