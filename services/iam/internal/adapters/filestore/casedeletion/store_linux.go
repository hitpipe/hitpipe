//go:build linux

// Package casedeletion implements the publisher's local durable registry. The
// publisher alone owns this directory; consumers use its authenticated service.
// Operate on a directory excluded from PostgreSQL/content restoration.
package casedeletion

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
)

const (
	lockName  = "registry.lock"
	stateName = "registry.json"
	maxState  = 4 * 1024 * 1024
)

var (
	ErrUnavailable = errors.New("case deletion registry: source unavailable")
	ErrInvalid     = errors.New("case deletion registry: invalid input")
	ErrInitialized = errors.New("case deletion registry: initialization requires an empty directory")
	ErrUncertain   = errors.New("case deletion registry: publication durability uncertain")
)

type fileID struct {
	device uint64
	inode  uint64
}

type Store struct {
	dir       string
	directory fileID
	lock      fileID
	source    string

	mu          sync.Mutex
	seen        bool
	environment string
	revision    uint64
	digest      [sha256.Size]byte
}

type envelope struct {
	Source    string    `json:"source"`
	LockInode uint64    `json:"lock_inode"`
	State     app.State `json:"state"`
}

// Initialize provisions a new source only in an existing, empty, absolute 0700
// directory owned by the publisher. It must never be used to repair a missing
// source. Incomplete provisioning is left in place for operator inspection.
func Initialize(dir string, initial app.State) (*Store, error) {
	if initial.Validate() != nil || initial.Revision != 1 || len(initial.Workspaces) != 0 {
		return nil, ErrInvalid
	}
	source := randomID()
	directory, dirID, err := openDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	children, err := directory.Readdirnames(1)
	if len(children) != 0 {
		return nil, ErrInitialized
	}
	if !errors.Is(err, io.EOF) {
		return nil, unavailable("inspect new directory")
	}
	lock, err := createFile(directory, lockName)
	if errors.Is(err, syscall.EEXIST) {
		return nil, ErrInitialized
	}
	if err != nil {
		return nil, unavailable("create stable lock")
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := acquire(ctx, lock, true); err != nil {
		return nil, err
	}
	lockID, err := privateFile(lock)
	if err != nil {
		return nil, err
	}
	if _, err := lock.Write([]byte(source)); err != nil || lock.Sync() != nil {
		return nil, unavailable("persist source identity")
	}
	s := &Store{dir: dir, directory: dirID, lock: lockID, source: source}
	if err := s.checkAttached(directory); err != nil {
		return nil, err
	}
	payload, err := s.encode(initial)
	if err != nil {
		return nil, err
	}
	if err := publish(ctx, directory, payload, nil); err != nil {
		return nil, err
	}
	if err := s.observe(initial, payload); err != nil {
		return nil, err
	}
	return s, nil
}

// Open never creates a missing lock or registry, even if the directory exists.
func Open(dir string) (*Store, error) {
	directory, dirID, err := openDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	lock, lockID, err := openRegular(directory, lockName)
	if err != nil {
		return nil, unavailable("open stable lock")
	}
	defer lock.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := acquire(ctx, lock, false); err != nil {
		return nil, err
	}
	source, err := readSource(lock)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, directory: dirID, lock: lockID, source: source}
	if err := s.checkAttached(directory); err != nil {
		return nil, err
	}
	if _, _, _, err := s.current(directory); err != nil {
		return nil, err
	}
	return s, nil
}

// Read holds the shared source lock through the callback, allowing a guarded
// restore/open action without letting a publication pass the same barrier.
// Callbacks must not reenter the store.
func (s *Store) Read(ctx context.Context, fn func(app.State) error) error {
	if fn == nil {
		return ErrInvalid
	}
	return s.withLock(ctx, false, func(dir *os.File) error {
		state, _, _, err := s.current(dir)
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return fn(state)
	})
}

// Change commits a callback's next revision atomically. Identical state is a
// no-op; errors, cancellation before rename, or invalid state publish nothing.
// ErrUncertain means rename succeeded but durability was not confirmed.
func (s *Store) Change(ctx context.Context, fn func(*app.State) error) error {
	if fn == nil {
		return ErrInvalid
	}
	return s.withLock(ctx, true, func(dir *os.File) error {
		state, prior, stateID, err := s.current(dir)
		if err != nil {
			return err
		}
		environment, revision := state.Environment, state.Revision
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(&state); err != nil {
			return err
		}
		next, err := s.encode(state)
		if err != nil {
			return err
		}
		if bytes.Equal(prior, next) {
			return ctx.Err()
		}
		if state.Environment != environment || revision == ^uint64(0) || state.Revision != revision+1 {
			return ErrInvalid
		}
		if err := s.checkAttached(dir); err != nil {
			return err
		}
		if err := publish(ctx, dir, next, &stateID); err != nil {
			return err
		}
		return s.observe(state, next)
	})
}

func (s *Store) withLock(ctx context.Context, exclusive bool, fn func(*os.File) error) error {
	if s == nil || ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, id, err := openDirectory(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	if id != s.directory {
		return unavailable("directory identity changed")
	}
	lock, lockID, err := openRegular(dir, lockName)
	if err != nil {
		return unavailable("open stable lock")
	}
	defer lock.Close()
	if lockID != s.lock {
		return unavailable("lock identity changed")
	}
	if err := acquire(ctx, lock, exclusive); err != nil {
		return err
	}
	source, err := readSource(lock)
	if err != nil || source != s.source {
		return unavailable("source identity changed")
	}
	if err := s.checkAttached(dir); err != nil {
		return err
	}
	return fn(dir)
}

func (s *Store) checkAttached(dir *os.File) error {
	original, id, err := openDirectory(s.dir)
	if err != nil {
		return err
	}
	original.Close()
	if id != s.directory {
		return unavailable("directory identity changed")
	}
	lock, id, err := openRegular(dir, lockName)
	if err != nil {
		return unavailable("stable lock unavailable")
	}
	lock.Close()
	if id != s.lock {
		return unavailable("lock identity changed")
	}
	return nil
}

func (s *Store) current(dir *os.File) (app.State, []byte, fileID, error) {
	var empty app.State
	f, id, err := openRegular(dir, stateName)
	if err != nil {
		return empty, nil, fileID{}, unavailable("open state")
	}
	defer f.Close()
	payload, err := io.ReadAll(io.LimitReader(f, maxState+1))
	if err != nil || len(payload) > maxState {
		return empty, nil, fileID{}, unavailable("read bounded state")
	}
	var record envelope
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&record); err != nil || record.Source != s.source || record.LockInode != s.lock.inode {
		return empty, nil, fileID{}, unavailable("state identity or shape")
	}
	// Only an exact canonical encoding is generated by the publisher. Comparing
	// it rejects duplicate keys, alternate spelling, trailers and opaque fields.
	canonical, err := s.encode(record.State)
	if err != nil || !bytes.Equal(canonical, payload) {
		return empty, nil, fileID{}, unavailable("invalid canonical state")
	}
	// Reconcile a prior rename whose directory fsync had an uncertain result.
	// We never report a state as usable if its data or name cannot be synced.
	if f.Sync() != nil || dir.Sync() != nil {
		return empty, nil, fileID{}, unavailable("confirm current state durability")
	}
	if err := s.observe(record.State, payload); err != nil {
		return empty, nil, fileID{}, err
	}
	return record.State, payload, id, nil
}

func (s *Store) observe(state app.State, payload []byte) error {
	checksum := sha256.Sum256(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen && (state.Environment != s.environment || state.Revision < s.revision ||
		state.Revision == s.revision && checksum != s.digest) {
		return unavailable("previous revision or conflicting state")
	}
	s.seen, s.environment, s.revision, s.digest = true, state.Environment, state.Revision, checksum
	return nil
}

func (s *Store) encode(state app.State) ([]byte, error) {
	if err := state.Validate(); err != nil {
		return nil, ErrInvalid
	}
	data, err := json.Marshal(envelope{Source: s.source, LockInode: s.lock.inode, State: state})
	if err != nil || len(data)+1 > maxState {
		return nil, ErrInvalid
	}
	return append(data, '\n'), nil
}

func publish(ctx context.Context, dir *os.File, payload []byte, expected *fileID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name := ".pending-" + randomID()
	temp, err := createFile(dir, name)
	if err != nil {
		return unavailable("create temporary state")
	}
	defer syscall.Unlinkat(int(dir.Fd()), name)
	n, writeErr := temp.Write(payload)
	syncErr := temp.Sync()
	closeErr := temp.Close()
	if n != len(payload) || writeErr != nil || syncErr != nil || closeErr != nil {
		return unavailable("persist temporary state")
	}
	current, id, err := openRegular(dir, stateName)
	if current != nil {
		current.Close()
	}
	if (expected == nil && !errors.Is(err, syscall.ENOENT)) ||
		(expected != nil && (err != nil || id != *expected)) {
		return unavailable("state node changed before publication")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	fd := int(dir.Fd())
	if err := syscall.Renameat(fd, name, fd, stateName); err != nil {
		return unavailable("install current state")
	}
	if dir.Sync() != nil {
		return ErrUncertain
	}
	return nil
}

func createFile(dir *os.File, name string) (*os.File, error) {
	fd, err := syscall.Openat(int(dir.Fd()), name,
		syscall.O_RDWR|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Fchmod(fd, 0600); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func stat(f *os.File) (syscall.Stat_t, fileID, error) {
	var metadata syscall.Stat_t
	if err := syscall.Fstat(int(f.Fd()), &metadata); err != nil {
		return metadata, fileID{}, unavailable("stat source node")
	}
	return metadata, fileID{device: uint64(metadata.Dev), inode: uint64(metadata.Ino)}, nil
}

func privateFile(f *os.File) (fileID, error) {
	meta, id, err := stat(f)
	if err != nil || meta.Mode != syscall.S_IFREG|0600 || meta.Nlink != 1 || meta.Uid != uint32(os.Geteuid()) {
		return fileID{}, unavailable("non-private or ambiguous source file")
	}
	return id, nil
}

func openRegular(dir *os.File, name string) (*os.File, fileID, error) {
	fd, err := syscall.Openat(int(dir.Fd()), name,
		syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fileID{}, err
	}
	f := os.NewFile(uintptr(fd), name)
	id, err := privateFile(f)
	if err != nil {
		f.Close()
		return nil, fileID{}, err
	}
	return f, id, nil
}

func openDirectory(path string) (*os.File, fileID, error) {
	if path == "/" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, fileID{}, ErrInvalid
	}
	const flags = syscall.O_RDONLY | syscall.O_DIRECTORY | syscall.O_NOFOLLOW | syscall.O_CLOEXEC
	fd, err := syscall.Open("/", flags, 0)
	if err != nil {
		return nil, fileID{}, unavailable("open source root")
	}
	// Refuse symlinks in intermediate components as well as at the final
	// directory, without path-based check/use races between components.
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		next, err := syscall.Openat(fd, part, flags, 0)
		syscall.Close(fd)
		if err != nil {
			return nil, fileID{}, unavailable("open source directory")
		}
		fd = next
	}
	result := os.NewFile(uintptr(fd), path)
	meta, id, err := stat(result)
	if err != nil || meta.Mode != syscall.S_IFDIR|0700 || meta.Uid != uint32(os.Geteuid()) {
		result.Close()
		return nil, fileID{}, unavailable("source directory must be publisher-owned 0700")
	}
	return result, id, nil
}

func acquire(ctx context.Context, lock *os.File, exclusive bool) error {
	how := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		how = syscall.LOCK_EX | syscall.LOCK_NB
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(lock.Fd()), how)
		switch {
		case err == nil:
			return ctx.Err()
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		default:
			return unavailable("acquire stable source lock")
		}
	}
}

func readSource(lock *os.File) (string, error) {
	data, err := io.ReadAll(io.LimitReader(lock, 33))
	if err != nil || len(data) != 32 || strings.ToLower(string(data)) != string(data) {
		return "", unavailable("invalid source identity")
	}
	if _, err := hex.DecodeString(string(data)); err != nil {
		return "", unavailable("invalid source identity")
	}
	return string(data), nil
}

func randomID() string {
	var value [16]byte
	rand.Read(value[:])
	return hex.EncodeToString(value[:])
}

func unavailable(action string) error { return fmt.Errorf("%w: %s", ErrUnavailable, action) }
