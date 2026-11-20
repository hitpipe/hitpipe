package intakeruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	web "github.com/hitpipe/hitpipe/services/intake/internal/adapters/http/stripewebhook"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	facts "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
	out "github.com/hitpipe/hitpipe/services/intake/internal/application/sendoutbox"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
)

const (
	Workspace      = "11111111-1111-4111-8111-111111111111"
	Connection     = "22222222-2222-4222-8222-222222222222"
	Case           = "33333333-3333-4333-8333-333333333333"
	Account        = "acct_synthetic"
	Session        = "cs_synthetic"
	Endpoint       = "synthetic"
	Version        = "2026-10-28.endive"
	ProcessingName = "processing.fixture"
)

type sandbox struct {
	ctx          context.Context
	gate         chan struct{}
	signing, key []byte
}

func (s *sandbox) work(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		return out.ErrDenied
	case <-s.ctx.Done():
		return out.ErrDenied
	case s.gate <- struct{}{}:
	}
	defer func() { <-s.gate }()
	if s.ctx.Err() != nil || ctx.Err() != nil {
		return out.ErrDenied
	}
	return fn(ctx)
}
func loadSecret(dir, name string) ([]byte, error) {
	p := filepath.Join(dir, name)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() < 16 || info.Size() > 512 {
		return nil, ErrConfig
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) < 16 || len(b) > 512 {
		return nil, ErrConfig
	}
	return b, nil
}
func (s *sandbox) connection() receivesignal.Connection {
	mode := false
	return receivesignal.Connection{SecurityEpoch: 1, Binding: normalizesignal.Binding{
		InstallationID: Endpoint, WorkspaceID: Workspace, ConnectionID: Connection, StripeAccountID: Account,
		Livemode: &mode, APIVersion: Version, AccountScope: normalizesignal.ConnectedAccount,
	}}
}
func (s *sandbox) Lookup(ctx context.Context, id string) (web.Installation, error) {
	if ctx.Err() != nil || s.ctx.Err() != nil {
		return web.Installation{}, web.ErrDenied
	}
	if id != Endpoint {
		return web.Installation{}, web.ErrNotFound
	}
	return web.Installation{EndpointID: Endpoint, Connection: s.connection(), Revision: 1, Ready: true}, nil
}
func (s *sandbox) Resolve(ctx context.Context, id string) ([]byte, error) {
	if id != Endpoint || ctx.Err() != nil || s.ctx.Err() != nil {
		return nil, ErrClosed
	}
	return s.signing, nil
}

type webhookGate struct{ s *sandbox }

func (g webhookGate) Start(ctx context.Context, i web.Installation, run func() error) error {
	w := g.s.connection()
	b := i.Connection.Binding
	if i.EndpointID != Endpoint || i.Revision != 1 || !i.Ready || i.Connection.SecurityEpoch != 1 ||
		b.Livemode == nil || *b.Livemode {
		return web.ErrDenied
	}
	w.Binding.Livemode, b.Livemode = nil, nil
	if w.Binding != b {
		return web.ErrDenied
	}
	return g.s.work(ctx, func(context.Context) error { return run() })
}

func binding() facts.Binding {
	mode := false
	return facts.Binding{WorkspaceID: Workspace, ConnectionID: Connection, AccountID: Account, CredentialRef: "sandbox-read",
		APIVersion: Version, Revision: "sandbox-1", Epoch: 1, LiveMode: &mode, Ready: true}
}
func (s *sandbox) Current(ctx context.Context, q sf.Request) (facts.Binding, error) {
	if s.ctx.Err() != nil || ctx.Err() != nil || q.WorkspaceID != Workspace || q.ConnectionID != Connection ||
		q.CaseID != Case || q.SessionID != Session || q.ExpectedEpoch != 1 {
		return facts.Binding{}, facts.Fail("context")
	}
	return binding(), nil
}
func (s *sandbox) Start(ctx context.Context, b facts.Binding, run func() error) error {
	if !b.Same(binding()) {
		return facts.Fail("context")
	}
	err := s.work(ctx, func(context.Context) error { return run() })
	if errors.Is(err, out.ErrDenied) {
		return facts.Fail("context")
	}
	return err
}
func (s *sandbox) ReadKey(ctx context.Context, b facts.Binding) ([]byte, error) {
	if ctx.Err() != nil || s.ctx.Err() != nil || !b.Same(binding()) {
		return nil, facts.Fail("credential")
	}
	return s.key, nil
}

var scope = out.Scope{Workspace: Workspace, Connection: Connection, Account: Account, Epoch: 1}

type senderGate struct{ s *sandbox }

func (g senderGate) Start(ctx context.Context, b out.Scope, run func(context.Context) error) error {
	if b != scope {
		return out.ErrDenied
	}
	return g.s.work(ctx, run)
}

type content struct{ store *contentstore.Store }

func (c content) Put(ctx context.Context, b facts.Binding, raw []byte, expires time.Time) (contentstore.Ref, error) {
	if ctx.Err() != nil || !b.Same(binding()) {
		return contentstore.Ref{}, facts.Fail("content")
	}
	return c.store.Put(raw, expires)
}
func (c content) Read(ctx context.Context, b facts.Binding, ref contentstore.Ref) ([]byte, error) {
	if ctx.Err() != nil || !b.Same(binding()) {
		return nil, facts.Fail("content")
	}
	return c.store.Read(ref)
}

type clock struct{}

func (clock) Now(ctx context.Context) (time.Time, error) { return time.Now(), ctx.Err() }
