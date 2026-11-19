// Package contentfacts scopes the unchanged HIT-74 store by checked workspace.
package contentfacts

import (
	"context"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	mu     sync.Mutex
	root   *os.Root
	path   string
	now    func() time.Time
	stores map[string]*contentstore.Store
	closed bool
}

func New(base string, now func() time.Time) (*Store, error) {
	s, e := contentstore.Open(base, now)
	if e != nil {
		return nil, app.Fail("content")
	}
	s.Close()
	r, e := os.OpenRoot(base)
	if e != nil {
		return nil, app.Fail("content")
	}
	return &Store{root: r, path: base, now: now, stores: map[string]*contentstore.Store{}}, nil
}
func (s *Store) workspace(b app.Binding) (*contentstore.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || !sf.UUID(b.WorkspaceID) {
		return nil, app.Fail("content")
	}
	if v := s.stores[b.WorkspaceID]; v != nil {
		return v, nil
	}
	if e := s.root.Mkdir(b.WorkspaceID, 0700); e != nil && !os.IsExist(e) {
		return nil, app.Fail("content")
	}
	v, e := contentstore.Open(filepath.Join(s.path, b.WorkspaceID), s.now)
	if e != nil {
		return nil, app.Fail("content")
	}
	s.stores[b.WorkspaceID] = v
	return v, nil
}
func (s *Store) Put(ctx context.Context, b app.Binding, data []byte, expires time.Time) (contentstore.Ref, error) {
	if ctx.Err() != nil {
		return contentstore.Ref{}, app.Fail("cancelled")
	}
	v, e := s.workspace(b)
	if e != nil {
		return contentstore.Ref{}, e
	}
	return v.Put(data, expires)
}
func (s *Store) Read(ctx context.Context, b app.Binding, ref contentstore.Ref) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, app.Fail("cancelled")
	}
	v, e := s.workspace(b)
	if e != nil {
		return nil, e
	}
	return v.Read(ref)
}
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for _, v := range s.stores {
		v.Close()
	}
	s.root.Close()
}
