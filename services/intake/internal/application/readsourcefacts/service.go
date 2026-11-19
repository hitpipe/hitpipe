// Package readsourcefacts composes explicitly established authority, provider
// reads, the original Intake schema and HIT-74 content. No permissive defaults.
package readsourcefacts

import (
	"bytes"
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"reflect"
	"sync/atomic"
	"time"
)

type Failure struct{ Code string }

func (e *Failure) Error() string { return e.Code }
func Fail(code string) error     { return &Failure{code} }

type Binding struct {
	WorkspaceID, ConnectionID, AccountID, CredentialRef, APIVersion, Revision string
	Epoch                                                                     int64
	LiveMode                                                                  *bool
	Ready                                                                     bool
}

func (b Binding) Clone() Binding {
	if b.LiveMode != nil {
		v := *b.LiveMode
		b.LiveMode = &v
	}
	return b
}
func (b Binding) Same(c Binding) bool {
	return b.LiveMode != nil && c.LiveMode != nil && b.WorkspaceID == c.WorkspaceID && b.ConnectionID == c.ConnectionID && b.AccountID == c.AccountID && b.Epoch == c.Epoch && *b.LiveMode == *c.LiveMode && b.Revision == c.Revision && b.CredentialRef == c.CredentialRef && b.APIVersion == c.APIVersion && b.Ready == c.Ready
}

// Authority resolves checked workspace/installation/grant state. Start must
// synchronously execute at most once under the shared current epoch/start gate.
// It cannot retain callbacks or acknowledge a barrier before old starts stop.
// Production wiring of this port is a separate authority/runtime task.
type Authority interface {
	Current(context.Context, sf.Request) (Binding, error)
	Start(context.Context, Binding, func() error) error
}
type Credentials interface {
	ReadKey(context.Context, Binding) ([]byte, error)
}
type Guard interface {
	Check(context.Context) error
	Start(context.Context, func() error) error
}
type Provider interface {
	Read(context.Context, sf.Request, Binding, Guard) (sf.Facts, error)
}
type Record struct {
	Facts sf.Facts
	Ref   contentstore.Ref
}
type Repository interface {
	Check(context.Context, Binding) error
	Save(context.Context, Binding, Record) error
	Load(context.Context, Binding, string) (Record, error)
}
type Content interface {
	Put(context.Context, Binding, []byte, time.Time) (contentstore.Ref, error)
	Read(context.Context, Binding, contentstore.Ref) ([]byte, error)
}
type Clock func() time.Time
type Service struct {
	authority Authority
	repo      Repository
	content   Content
	provider  Provider
	now       Clock
	retention time.Duration
}

func missing(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	return (r.Kind() == reflect.Pointer || r.Kind() == reflect.Func || r.Kind() == reflect.Interface || (r.Kind() == reflect.Map || r.Kind() == reflect.Slice || r.Kind() == reflect.Chan)) && r.IsNil()
}
func New(a Authority, r Repository, c Content, p Provider, now Clock, retention time.Duration) (*Service, error) {
	if missing(a) || missing(r) || missing(c) || missing(p) || now == nil || retention <= 0 || retention > 90*24*time.Hour {
		return nil, Fail("dependency")
	}
	return &Service{a, r, c, p, now, retention}, nil
}
func (s *Service) bind(ctx context.Context, q sf.Request) (Binding, error) {
	if ctx.Err() != nil || !q.DeadlineAt.After(s.now()) || !sf.UUID(q.WorkspaceID) || !sf.UUID(q.ConnectionID) || !sf.UUID(q.CaseID) || !sf.Opaque(q.SessionID) || q.ExpectedEpoch < 1 || q.ExpectedEpoch > 9007199254740991 {
		return Binding{}, Fail("request")
	}
	b, e := s.authority.Current(ctx, q)
	if e != nil || !b.Ready || b.LiveMode == nil || b.WorkspaceID != q.WorkspaceID || b.ConnectionID != q.ConnectionID || b.Epoch != q.ExpectedEpoch || !sf.Opaque(b.AccountID) || b.CredentialRef == "" || b.Revision == "" || b.APIVersion == "" {
		return Binding{}, Fail("context")
	}
	return b.Clone(), nil
}

type bound struct {
	s *Service
	q sf.Request
	b Binding
}

func (g bound) Start(ctx context.Context, action func() error) error {
	if ctx.Err() != nil {
		return Fail("cancelled")
	}
	b, e := g.s.bind(ctx, g.q)
	if e != nil || !g.b.Same(b) {
		return Fail("context")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var called atomic.Bool
	var active atomic.Bool
	active.Store(true)
	var actionErr error
	e = g.s.authority.Start(ctx, g.b.Clone(), func() error {
		if !active.Load() || !called.CompareAndSwap(false, true) || ctx.Err() != nil {
			return Fail("context")
		}
		actionErr = action()
		return actionErr
	})
	active.Store(false)
	if actionErr != nil {
		return actionErr
	}
	if e != nil || !called.Load() || ctx.Err() != nil {
		return Fail("context")
	}
	return nil
}
func (g bound) Check(ctx context.Context) error {
	return g.Start(ctx, func() error { return g.s.repo.Check(ctx, g.b) })
}

type Result struct {
	Body    []byte
	Facts   sf.Facts
	binding Binding
	request sf.Request
}

func (s *Service) Read(ctx context.Context, q sf.Request) (Result, error) {
	ctx, cancel := context.WithDeadline(ctx, q.DeadlineAt)
	defer cancel()
	b, e := s.bind(ctx, q)
	if e != nil {
		return Result{}, e
	}
	g := bound{s, q, b}
	if e = g.Check(ctx); e != nil {
		return Result{}, e
	}
	f, e := s.provider.Read(ctx, q, b, g)
	if e != nil {
		return Result{}, e
	}
	if e = g.Check(ctx); e != nil {
		return Result{}, e
	}
	f.FactsID, e = sf.NewID()
	if e != nil {
		return Result{}, Fail("unavailable")
	}
	f.CaseID = q.CaseID
	f.ConnectionID = q.ConnectionID
	f.SessionID = q.SessionID
	f.StripeAccountID = b.AccountID
	f.Livemode = *b.LiveMode
	f.CompletedAt = s.now().UTC().Truncate(time.Microsecond)
	if f.OldestObservedAt.IsZero() || f.CompletedAt.Before(f.OldestObservedAt) {
		return Result{}, Fail("unavailable")
	}
	f.ValidUntil = f.OldestObservedAt.Add(10 * time.Second)
	if q.DeadlineAt.Before(f.ValidUntil) {
		f.ValidUntil = q.DeadlineAt
	}
	f.ValidUntil = f.ValidUntil.Truncate(time.Microsecond)
	if !f.ValidUntil.After(s.now()) {
		return Result{}, Fail("expired")
	}
	f.Digest, e = sf.FactsDigest(f)
	if e != nil {
		return Result{}, Fail("unavailable")
	}
	data, e := sf.Encode(f)
	if e != nil {
		return Result{}, Fail("unavailable")
	}
	ref, e := s.content.Put(ctx, b, data, f.CompletedAt.Add(s.retention))
	if e != nil {
		return Result{}, Fail("content")
	}
	actual, e := s.content.Read(ctx, b, ref)
	if e != nil || !bytes.Equal(actual, data) {
		return Result{}, Fail("content")
	}
	if e = g.Check(ctx); e != nil {
		return Result{}, e
	}
	e = g.Start(ctx, func() error { return s.repo.Save(ctx, b, Record{f, ref}) })
	if e != nil {
		return Result{}, e
	}
	rec, e := s.Load(ctx, q, f.FactsID)
	if e != nil || rec.Facts.Digest != f.Digest || !bytes.Equal(rec.Body, data) {
		return Result{}, Fail("unavailable")
	}
	return rec, nil
}
func (s *Service) Load(ctx context.Context, q sf.Request, id string) (Result, error) {
	if !sf.UUID(id) {
		return Result{}, Fail("request")
	}
	b, e := s.bind(ctx, q)
	if e != nil {
		return Result{}, e
	}
	g := bound{s, q, b}
	var rec Record
	e = g.Start(ctx, func() error { var e error; rec, e = s.repo.Load(ctx, b, id); return e })
	if e != nil {
		return Result{}, Fail("unavailable")
	}
	m := rec.Facts // SQL metadata only; payload is never stored in SQL.
	if m.CaseID != q.CaseID || m.ConnectionID != q.ConnectionID || !m.ValidUntil.After(s.now()) || m.ValidUntil.After(q.DeadlineAt) {
		return Result{}, Fail("expired")
	}
	data, e := s.content.Read(ctx, b, rec.Ref)
	if e != nil {
		return Result{}, Fail("content")
	}
	f, e := sf.Decode(data)
	if e != nil || f.FactsID != id || f.FactsID != m.FactsID || f.CaseID != m.CaseID || f.ConnectionID != m.ConnectionID || f.Complete != m.Complete || !f.OldestObservedAt.Equal(m.OldestObservedAt) || !f.CompletedAt.Equal(m.CompletedAt) || !f.ValidUntil.Equal(m.ValidUntil) || f.SessionID != q.SessionID || f.StripeAccountID != b.AccountID || f.Livemode != *b.LiveMode {
		return Result{}, Fail("content")
	}
	if e = g.Check(ctx); e != nil {
		return Result{}, e
	}
	return Result{data, f, b, q}, nil
}

// Respond applies the same current gate to the actual protected response start.
func (s *Service) Respond(ctx context.Context, r Result, write func() error) error {
	if !r.Facts.ValidUntil.After(s.now()) {
		return Fail("expired")
	}
	g := bound{s, r.request, r.binding}
	if e := g.Check(ctx); e != nil {
		return e
	}
	return g.Start(ctx, func() error {
		if !r.Facts.ValidUntil.After(s.now()) {
			return Fail("expired")
		}
		return write()
	})
}
func Code(e error) string {
	var f *Failure
	if errors.As(e, &f) {
		return f.Code
	}
	return "unavailable"
}
