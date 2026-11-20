// Package contentfiles implements owner-only durable files. SQL must store refs,
// not the envelopes below. No garbage collector guesses an unknown SQL outcome.
package contentfiles

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
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

const maxBody = 65536

// Store owns an existing private directory. Step is optional fault injection;
// it can only fail a write, never authorize an action or override an IO error.
type Store struct {
	mu    sync.Mutex
	root  *os.Root
	lease *os.File
	Step  func(string) error
}
type header struct {
	Origin  preparecase.Origin `json:"origin"`
	Version int                `json:"version"`
	Ref     preparecase.Ref    `json:"ref"`
}

func New(path string) (*Store, error) {
	return openStore(path, syscall.LOCK_SH)
}

func openStore(path string, mode int) (*Store, error) {
	absolute, e := filepath.Abs(path)
	if e != nil {
		return nil, preparecase.ErrContent
	}
	resolved, e := filepath.EvalSymlinks(absolute)
	if e != nil || resolved != absolute {
		return nil, preparecase.ErrContent
	}
	root, e := os.OpenRoot(absolute)
	if e != nil {
		return nil, preparecase.ErrContent
	}
	f, e := root.Open(".")
	if e != nil {
		root.Close()
		return nil, preparecase.ErrContent
	}
	stat, e := f.Stat()
	if e != nil || !stat.IsDir() || stat.Mode().Perm() != 0700 {
		f.Close()
		root.Close()
		return nil, preparecase.ErrContent
	}
	// The stable directory inode is shared for the lifetime of any runtime
	// using it, including the commit after Write returns its new reference.
	if e = syscall.Flock(int(f.Fd()), mode|syscall.LOCK_NB); e != nil {
		f.Close()
		root.Close()
		return nil, fmt.Errorf("content directory unavailable: %w", e)
	}
	return &Store{root: root, lease: f}, nil
}
func (s *Store) Close() error { return errors.Join(s.root.Close(), s.lease.Close()) }
func UUID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func hash(b []byte) string { sum := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(sum[:]) }
func (s *Store) step(ctx context.Context, name string) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.Step != nil {
		return s.Step(name)
	}
	return nil
}
func (s *Store) syncDir() error {
	f, e := s.root.Open(".")
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func validClass(c string) bool {
	return c == "DELIVERY_INTENT" || c == "DELIVERY_QUERY" || c == "HISTORY" || c == "CASE_CONTEXT" || c == "DRAFT_DATA"
}
func (s *Store) Write(ctx context.Context, b []byte, class string, origin preparecase.Origin, expiry, now time.Time) (preparecase.Ref, error) {
	zero := preparecase.Ref{}
	// Persist exactly the precision representable by PostgreSQL timestamptz.
	// Round down, never extend a caller's hard expiry or a source-proof deadline.
	expiry = expiry.UTC().Truncate(time.Microsecond)
	if s == nil || s.root == nil || ctx == nil || ctx.Err() != nil || !validClass(class) || !validOrigin(origin) || len(b) < 1 || len(b) > maxBody || now.IsZero() || !expiry.After(now) || expiry.After(now.Add(90*24*time.Hour)) {
		return zero, preparecase.ErrContent
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id, e := UUID()
	if e != nil {
		return zero, e
	}
	if _, e := s.root.Lstat(id); !os.IsNotExist(e) {
		return zero, preparecase.ErrContent
	}
	ref := preparecase.Ref{ID: id, Digest: hash(b), Class: class, Bytes: len(b), ExpiresAt: expiry.UTC()}
	h, e := json.Marshal(header{Origin: origin, Version: 1, Ref: ref})
	if e != nil {
		return zero, e
	}
	tmp := id + ".pending"
	f, e := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return zero, preparecase.ErrContent
	}
	// This write cannot have a committed SQL ref: no ref is returned until the
	// complete durable sequence. A renamed orphan is retained for expiry purge.
	renamed := false
	defer func() {
		f.Close()
		if !renamed {
			s.root.Remove(tmp)
		}
	}()
	if _, e = f.Write(append(append(h, '\n'), b...)); e != nil {
		return zero, e
	}
	if e = s.step(ctx, "write"); e != nil {
		return zero, e
	}
	if e = f.Sync(); e != nil {
		return zero, e
	}
	if e = s.step(ctx, "file-fsync"); e != nil {
		return zero, e
	}
	if e = f.Close(); e != nil {
		return zero, e
	}
	if e = s.root.Rename(tmp, id); e != nil {
		return zero, e
	}
	renamed = true
	if e = s.step(ctx, "rename"); e != nil {
		return zero, e
	}
	if e = s.syncDir(); e != nil {
		return zero, e
	}
	if e = s.step(ctx, "directory-fsync"); e != nil {
		return zero, e
	}
	return ref, nil
}
func (s *Store) read(id string) (header, []byte, error) {
	zero := header{}
	if !idPattern.MatchString(strings.TrimSuffix(id, ".pending")) {
		return zero, nil, preparecase.ErrContent
	}
	f, e := s.root.OpenFile(id, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return zero, nil, preparecase.ErrContent
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > maxBody+2048 {
		return zero, nil, preparecase.ErrContent
	}
	raw, e := io.ReadAll(io.LimitReader(f, maxBody+2049))
	if e != nil {
		return zero, nil, preparecase.ErrContent
	}
	n := bytes.IndexByte(raw, '\n')
	if n < 0 || n > 2048 {
		return zero, nil, preparecase.ErrContent
	}
	var h header
	d := json.NewDecoder(bytes.NewReader(raw[:n]))
	d.DisallowUnknownFields()
	if d.Decode(&h) != nil || h.Version != 1 || !validOrigin(h.Origin) || h.Ref.ID != strings.TrimSuffix(id, ".pending") || !validClass(h.Ref.Class) || h.Ref.ExpiresAt.IsZero() {
		return zero, nil, preparecase.ErrContent
	}
	b := raw[n+1:]
	if len(b) != h.Ref.Bytes || len(b) == 0 || len(b) > maxBody || h.Ref.Digest != hash(b) {
		return h, nil, preparecase.ErrContent
	}
	return h, b, nil
}
func (s *Store) Read(ctx context.Context, ref preparecase.Ref, now time.Time) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || now.IsZero() || !idPattern.MatchString(ref.ID) {
		return nil, preparecase.ErrContent
	}
	h, b, e := s.read(ref.ID)
	if e != nil {
		return nil, e
	}
	if h.Ref.Digest != ref.Digest || h.Ref.Class != ref.Class || h.Ref.Bytes != ref.Bytes || !h.Ref.ExpiresAt.Equal(ref.ExpiresAt) {
		return nil, preparecase.ErrContent
	}
	if !now.Before(h.Ref.ExpiresAt) {
		return nil, preparecase.ErrContent
	}
	return b, nil
}

var originID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validOrigin(o preparecase.Origin) bool {
	return originID.MatchString(o.WorkspaceID) && originID.MatchString(o.CaseID) && o.Revision > 0
}
