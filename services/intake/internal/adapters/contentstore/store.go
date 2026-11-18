// Package contentstore owns isolated bounded content files, not SQL receipts.
package contentstore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrStore = errors.New("content unavailable")

const MaxBytes = 65536 // Intake source_facts/outbox bounded content, not raw webhook.
type Ref struct {
	ID        string
	Digest    string
	Bytes     int
	ExpiresAt time.Time
}

// Store must own its root exclusively. References, expiry and referenced-ID
// snapshots come from an authoritative owner; this adapter does not invent them.
type Store struct {
	mu    sync.Mutex
	root  *os.Root
	now   func() time.Time
	fault func(string) error
}

func Open(path string, now func() time.Time) (*Store, error) {
	if now == nil {
		return nil, ErrStore
	}
	absolute, e := filepath.Abs(path)
	if e != nil {
		return nil, ErrStore
	}
	// Reject symlinks in the configured path; os.Root then pins the opened directory.
	for p := absolute; ; p = filepath.Dir(p) {
		st, e := os.Lstat(p)
		if e != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return nil, ErrStore
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	st, e := os.Stat(absolute)
	if e != nil || st.Mode().Perm() != 0700 {
		return nil, ErrStore
	}
	r, e := os.OpenRoot(absolute)
	if e != nil {
		return nil, ErrStore
	}
	d, err := r.Open(".")
	if err != nil {
		r.Close()
		return nil, ErrStore
	}
	opened, err := d.Stat()
	d.Close()
	if err != nil || !os.SameFile(st, opened) || opened.Mode().Perm() != 0700 {
		r.Close()
		return nil, ErrStore
	}
	return &Store{root: r, now: now}, nil
}
func (s *Store) Close() error { s.mu.Lock(); defer s.mu.Unlock(); return s.root.Close() }
func (s *Store) step(name string) error {
	if s.fault != nil {
		return s.fault(name)
	}
	return nil
}
func valid(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return id[14] == '4' && strings.ContainsRune("89ab", rune(id[19]))
}
func name() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrStore
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}
func (s *Store) syncDir() error {
	d, e := s.root.Open(".")
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}

// Put returns a reference ONLY after file sync, rename and directory sync.
// A failure after rename may leave a complete orphan; never retry as if absent.
func (s *Store) Put(data []byte, expires time.Time) (Ref, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(data) == 0 || len(data) > MaxBytes || expires.IsZero() || !expires.After(s.now()) || expires.Sub(s.now()) > 90*24*time.Hour {
		return Ref{}, ErrStore
	}
	id, e := name()
	if e != nil {
		return Ref{}, ErrStore
	}
	tmp := ".tmp-" + id
	f, e := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return Ref{}, ErrStore
	}
	defer f.Close()
	// Failed writes intentionally remain recognizable orphans for explicit purge.
	if s.step("write") != nil {
		return Ref{}, ErrStore
	}
	n, e := f.Write(data)
	if e != nil || n != len(data) {
		return Ref{}, ErrStore
	}
	if s.step("file_sync") != nil || f.Sync() != nil {
		return Ref{}, ErrStore
	}
	if e = f.Close(); e != nil {
		return Ref{}, ErrStore
	}
	if s.step("rename") != nil {
		return Ref{}, ErrStore
	}
	if e = s.root.Rename(tmp, id); e != nil {
		return Ref{}, ErrStore
	}
	if s.step("directory_sync") != nil || s.syncDir() != nil {
		return Ref{}, ErrStore
	}
	h := sha256.Sum256(data)
	return Ref{ID: id, Digest: "sha256:" + hex.EncodeToString(h[:]), Bytes: len(data), ExpiresAt: expires}, nil
}
func (s *Store) Read(ref Ref) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(ref.ID) || ref.Bytes < 1 || ref.Bytes > MaxBytes || !ref.ExpiresAt.After(s.now()) {
		return nil, ErrStore
	}
	f, e := s.root.OpenFile(ref.ID, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, ErrStore
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() != int64(ref.Bytes) {
		return nil, ErrStore
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if e != nil || len(b) != ref.Bytes {
		return nil, ErrStore
	}
	h := sha256.Sum256(b)
	if ref.Digest != "sha256:"+hex.EncodeToString(h[:]) || !ref.ExpiresAt.After(s.now()) {
		return nil, ErrStore
	}
	return b, nil
}

// Purge removes only adapter-named files absent from a complete authoritative
// reference snapshot. Caller must exclude concurrent external publishers; this
// mutex serializes this instance only. No SQL/global orphan truth is claimed.
func (s *Store) Purge(referenced map[string]bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if referenced == nil {
		return 0, ErrStore
	}
	for id := range referenced {
		if !valid(id) {
			return 0, ErrStore
		}
	}
	d, e := s.root.Open(".")
	if e != nil {
		return 0, ErrStore
	}
	entries, e := d.ReadDir(-1)
	d.Close()
	if e != nil {
		return 0, ErrStore
	}
	n := 0
	for _, entry := range entries {
		id := entry.Name()
		temp := strings.HasPrefix(id, ".tmp-") && valid(strings.TrimPrefix(id, ".tmp-"))
		if !valid(id) && !temp {
			continue
		}
		if referenced[id] {
			continue
		}
		st, e := s.root.Lstat(id)
		if e != nil || !st.Mode().IsRegular() {
			return n, ErrStore
		}
		if s.root.Remove(id) != nil {
			return n, ErrStore
		}
		n++
	}
	if n > 0 && s.syncDir() != nil {
		return n, ErrStore
	}
	return n, nil
}
