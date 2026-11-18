package runtime

import (
	"context"
	"errors"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
)

type caseBoundary struct{ a *access }

func (b caseBoundary) Start(ctx context.Context, s preparecase.Scope, work func(context.Context) error) error {
	w, ok := b.a.stand[s.WorkspaceID]
	if !ok || b.a.stopping.Load() {
		return preparecase.ErrHold
	}
	c := withPermit(ctx)
	p, err := b.a.local(c, s.WorkspaceID, "CASE_READ", &s.CaseID)
	if err != nil || p.epoch != s.Epoch {
		return preparecase.ErrHold
	}
	if s.OwnerID != "" {
		if s.OwnerID != p.principal || s.SourceConnectionID != w.config.Source || s.DestinationConnectionID != w.config.Destination {
			return preparecase.ErrHold
		}
		source, err := b.a.local(c, s.WorkspaceID, "CONNECTOR_READ", &w.config.Source)
		if err != nil || source.epoch != s.Epoch || source.principal != p.principal {
			return preparecase.ErrHold
		}
		destination, err := b.a.local(c, s.WorkspaceID, "CONNECTOR_WRITE", &w.config.Destination)
		if err != nil || destination.epoch != s.Epoch || destination.principal != p.principal {
			return preparecase.ErrHold
		}
		for _, permission := range []permit{p, source} {
			if permission.expires.Before(destination.expires) {
				destination.expires = permission.expires
			}
		}
		scopeOf(c).permit = destination
	}
	return b.a.start(c, s.WorkspaceID, p.principal, s.Epoch, work)
}

func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
	c := withPermit(ctx)
	source := rt.a.stand[workspace].config.Source
	p, err := rt.a.local(c, workspace, "CONNECTOR_READ", &source)
	if err != nil {
		return
	}
	var claim postgres.CaseClaim
	err = rt.a.start(c, workspace, p.principal, p.epoch, func(c context.Context) error {
		if err := rt.caseJobs.Index(c, workspace, 5); err != nil {
			return err
		}
		var err error
		claim, err = rt.caseJobs.Claim(c, workspace, p.epoch)
		return err
	})
	if err != nil {
		rt.logger.Warn("processing case worker could not claim work")
		return
	}
	if claim.CaseID == "" {
		return
	}

	bounded, cancel := context.WithDeadline(ctx, claim.Until())
	defer cancel()
	readers := rt.caseReaders
	readers.claim = claim
	service := preparecase.Service{
		Repository: postgres.ClaimedDecisionStore{
			DecisionStore: postgres.DecisionStore{DB: rt.caseJobs.DB, Content: rt.files},
			Claim:         claim,
		},
		Readers: readers, Renderer: intent.Renderer{}, Content: rt.files,
		Boundary: caseBoundary{rt.a}, Clock: clock{},
	}
	result, err := service.Prepare(bounded, claim.Request)
	switch {
	case errors.Is(err, preparecase.ErrStale):
		rt.logger.Info("processing case worker lost current claim", "case_id", claim.CaseID)
	case errors.Is(err, preparecase.ErrHold), errors.Is(err, drafts.ErrDenied):
		rt.logger.Info("processing case worker held", "case_id", claim.CaseID)
	case err != nil:
		// Errors can include SQL or owner content. Preserve claim until expiry,
		// and leave details out of the product log.
		rt.logger.Warn("processing case worker interrupted", "case_id", claim.CaseID)
	default:
		rt.logger.Info("processing case worker decision", "case_id", result.CaseID, "state", result.State)
	}
}
