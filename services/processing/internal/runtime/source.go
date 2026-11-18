package runtime

import (
	"context"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

type sourceReaders struct {
	a         *access
	origin    string
	transport *http.Transport
	jobs      postgres.CaseJobStore
	claim     postgres.CaseClaim
}

func (s sourceReaders) ForCase(ctx context.Context, c preparecase.Case, r preparecase.Request) (evaluatepaidorder.SourceFactsReader, error) {
	w, ok := s.a.stand[r.WorkspaceID]
	if !ok || r != s.claim.Request || c.OwnerID != w.config.Owner || c.Policy.ConnectionID != w.config.Source ||
		c.Mapping.DestinationConnectionID != w.config.Destination || c.Mapping.DestinationWorkspaceID != w.config.DestinationWorkspace ||
		c.Mapping.PriceID != w.config.Price || c.Mapping.ListID != w.config.List || c.Mapping.AssigneeID != w.config.Assignee ||
		c.AccountID != w.config.Account || c.Live != (w.config.Mode == "LIVE") {
		return nil, preparecase.ErrHold
	}
	id, err := drafts.NewID()
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(protectedTime)
	if s.claim.Until().Before(deadline) {
		deadline = s.claim.Until()
	}
	if !c.EpisodeDeadlineAt.IsZero() && c.EpisodeDeadlineAt.Before(deadline) {
		deadline = c.EpisodeDeadlineAt
	}
	scope := sourcefacts.CallerContext{WorkspaceID: r.WorkspaceID, CaseID: c.CaseID, SessionID: c.SessionID,
		ConnectionID: w.config.Source, AccountID: c.AccountID, RequestID: id, ExpectedEpoch: r.Epoch, DeadlineAt: deadline, LiveMode: &c.Live}
	return sourcefacts.NewReader(s.origin, c.Policy, scope, s.transport, s, limitBody)
}

func (s sourceReaders) Start(ctx context.Context, c sourcefacts.CallerContext, f func() error) error {
	w, ok := s.a.stand[c.WorkspaceID]
	if !ok || s.a.stopping.Load() || c.WorkspaceID != s.claim.WorkspaceID || c.CaseID != s.claim.CaseID ||
		c.ExpectedEpoch != s.claim.Epoch || c.ConnectionID != w.config.Source || c.AccountID != w.config.Account {
		return preparecase.ErrHold
	}
	request := withPermit(ctx)
	p, err := s.a.local(request, c.WorkspaceID, "CONNECTOR_READ", &c.ConnectionID)
	if err != nil || p.epoch != c.ExpectedEpoch {
		return preparecase.ErrHold
	}
	return s.a.start(request, c.WorkspaceID, w.config.Owner, c.ExpectedEpoch, func(x context.Context) error {
		if err := s.jobs.CheckStart(x, s.claim); err != nil {
			return err
		}
		if x.Err() != nil || !time.Now().Before(s.claim.Until()) {
			return preparecase.ErrStale
		}
		return f()
	})
}
