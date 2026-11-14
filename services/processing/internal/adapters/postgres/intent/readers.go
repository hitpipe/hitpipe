package intent

import (
	"context"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

// Readers composes HIT-77's existing guarded HTTPS reader, without duplicating its
// transport/Decode. Origin, mTLS transport, owner gate, clock, deadline and private
// buffer bound must all be explicitly configured. No production credentials or
// permissive boundary are provided. The Boundary must be the same runtime owner
// instance used by preparecase.Service, not an independent cached permission.
type Readers struct {
	Origin           string
	Transport        *http.Transport
	Boundary         preparecase.Boundary
	Clock            preparecase.Clock
	ReadDeadline     time.Duration
	MaxResponseBytes int64
}

func (f Readers) ForCase(ctx context.Context, c preparecase.Case, r preparecase.Request) (evaluatepaidorder.SourceFactsReader, error) {
	if f.Boundary == nil || f.Clock == nil || f.Transport == nil || f.ReadDeadline < time.Second || f.ReadDeadline > 10*time.Second || f.MaxResponseBytes <= 0 || r.CaseID != c.CaseID || r.WorkspaceID != c.WorkspaceID || r.Epoch < 1 {
		return nil, preparecase.ErrHold
	}
	now := f.Clock.Now()
	if now.IsZero() {
		return nil, preparecase.ErrHold
	}
	deadline := now.Add(f.ReadDeadline)
	if !c.EpisodeDeadlineAt.IsZero() && c.EpisodeDeadlineAt.Before(deadline) {
		deadline = c.EpisodeDeadlineAt
	}
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	caller := sourcefacts.CallerContext{WorkspaceID: c.WorkspaceID, CaseID: c.CaseID, SessionID: c.SessionID, ConnectionID: c.Mapping.SourceConnectionID, AccountID: c.AccountID, RequestID: c.CaseID, ExpectedEpoch: r.Epoch, DeadlineAt: deadline, LiveMode: &c.Live}
	return sourcefacts.NewReader(f.Origin, c.Policy, caller, f.Transport, sharedSourceBoundary{f.Boundary, c.Scope(r), caller}, f.MaxResponseBytes)
}

type sharedSourceBoundary struct {
	boundary preparecase.Boundary
	scope    preparecase.Scope
	expected sourcefacts.CallerContext
}

func (g sharedSourceBoundary) Start(ctx context.Context, c sourcefacts.CallerContext, start func() error) error {
	e := g.expected
	if c.LiveMode == nil || e.LiveMode == nil || c.WorkspaceID != e.WorkspaceID || c.CaseID != e.CaseID || c.SessionID != e.SessionID || c.ConnectionID != e.ConnectionID || c.AccountID != e.AccountID || *c.LiveMode != *e.LiveMode || c.ExpectedEpoch != e.ExpectedEpoch || !c.DeadlineAt.Equal(e.DeadlineAt) {
		return preparecase.ErrHold
	}
	called := false
	err := g.boundary.Start(ctx, g.scope, func(ctx context.Context) error {
		if called || ctx == nil || ctx.Err() != nil {
			return preparecase.ErrHold
		}
		called = true
		return start()
	})
	if err != nil {
		return err
	}
	if !called {
		return preparecase.ErrHold
	}
	return nil
}
