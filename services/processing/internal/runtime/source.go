package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

type sourceReaders struct {
	a         *access
	origin    string
	transport *http.Transport
}

func (s sourceReaders) ForCase(ctx context.Context, c preparecase.Case, r preparecase.Request) (evaluatepaidorder.SourceFactsReader, error) {
	w, ok := s.a.stand[r.WorkspaceID]
	if !ok || c.OwnerID != w.config.Owner || c.Policy.ConnectionID != w.config.Source || c.AccountID != w.config.Account || c.Live != (w.config.Mode == "LIVE") {
		return nil, preparecase.ErrHold
	}
	id, err := drafts.NewID()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(protectedTime)
	if !c.EpisodeDeadlineAt.IsZero() && c.EpisodeDeadlineAt.Before(deadline) {
		deadline = c.EpisodeDeadlineAt
	}
	scope := sourcefacts.CallerContext{WorkspaceID: r.WorkspaceID, CaseID: c.CaseID, SessionID: c.SessionID,
		ConnectionID: w.config.Source, AccountID: c.AccountID, RequestID: id, ExpectedEpoch: r.Epoch, DeadlineAt: deadline, LiveMode: &c.Live}
	return sourcefacts.NewReader(s.origin, c.Policy, scope, s.transport, s, limitBody)
}

func (s sourceReaders) Start(ctx context.Context, c sourcefacts.CallerContext, f func() error) error {
	w, ok := s.a.stand[c.WorkspaceID]
	if !ok || c.ConnectionID != w.config.Source || c.AccountID != w.config.Account {
		return preparecase.ErrHold
	}
	return s.a.start(ctx, c.WorkspaceID, w.config.Owner, c.ExpectedEpoch, func(context.Context) error { return f() })
}
