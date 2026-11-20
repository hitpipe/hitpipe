package runtime

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
)

type recoveryAccess struct{ a *access }

func (a *access) recoveryPermit(ctx context.Context, ws, caseID, session string) (permit, string, error) {
	owner, err := a.user(ctx, ws, session, "CASE_RECOVER", &caseID)
	if err != nil {
		return permit{}, "", err
	}
	if owner.role != "OWNER" {
		return permit{}, "", drafts.ErrDenied
	}
	destination := a.stand[ws].config.Destination
	read, err := a.user(ctx, ws, session, "CONNECTOR_READ", &destination)
	if err != nil {
		return permit{}, "", err
	}
	if read.epoch != owner.epoch || read.principal != owner.principal || read.session != owner.session || read.role != "OWNER" {
		return permit{}, "", drafts.ErrDenied
	}
	if owner.expires.Before(read.expires) {
		read.expires = owner.expires
	}
	scopeOf(ctx).permit = read
	return read, destination, nil
}

func (a recoveryAccess) Authorize(ctx context.Context, q recheck.Request) (recheck.Grant, error) {
	p, destination, err := a.a.recoveryPermit(ctx, q.WorkspaceID, q.CaseID, q.Session)
	if err != nil {
		return recheck.Grant{}, err
	}
	w := a.a.stand[q.WorkspaceID]
	h := sha256.Sum256([]byte(q.CSRF))
	if subtle.ConstantTimeCompare(h[:], w.csrfHash[:]) != 1 {
		return recheck.Grant{}, drafts.ErrDenied
	}
	return recheck.Grant{WorkspaceID: p.workspace, OwnerID: p.principal, ConnectionID: destination, Epoch: p.epoch, ExpiresAt: p.expires}, nil
}

func (a recoveryAccess) Start(ctx context.Context, c dispatchintent.Claim, f func(context.Context) error) error {
	return a.a.start(ctx, c.WorkspaceID, c.OwnerID, c.Epoch, f)
}

// Initial metadata polling uses connector read authority. A scoped claim
// refreshes CASE_RECOVER and the destination read grant at each actual start.
type queryAccess struct{ a *access }

func (a queryAccess) Start(ctx context.Context, c dispatchintent.Claim, f func(context.Context) error) error {
	w, known := a.a.stand[c.WorkspaceID]
	if !known {
		return dispatchintent.ErrHeld
	}
	if c.CaseID == "" {
		if a.a.stopping.Load() {
			return dispatchintent.ErrHeld
		}
		return a.a.start(ctx, c.WorkspaceID, w.config.Owner, c.Epoch, f)
	}
	if c.OwnerID != w.config.Owner || c.ConnectionID != w.config.Destination {
		return dispatchintent.ErrHeld
	}
	x := withPermit(ctx)
	p, _, err := a.a.recoveryPermit(x, c.WorkspaceID, c.CaseID, w.token)
	if err != nil || p.epoch != c.Epoch {
		return dispatchintent.ErrHeld
	}
	return a.a.start(x, c.WorkspaceID, c.OwnerID, c.Epoch, f)
}

func (rt *Runtime) advanceRecheck(ctx context.Context, workspace string) {
	c := withPermit(ctx)
	connection := rt.a.stand[workspace].config.Destination
	p, err := rt.a.local(c, workspace, "CONNECTOR_READ", &connection)
	if err != nil {
		return
	}
	out, err := rt.queries.RunScope(c, dispatchintent.Request{WorkspaceID: workspace, Epoch: p.epoch})
	rt.diag.dispatchFault(recheckInterrupted, workspace, err)
	for _, v := range out {
		rt.logger.Info("processing recheck outbox transition", "request_id", v.MessageID, "state", v.State, "disposition", v.Disposition)
	}
}

type queryObservationAuthority struct {
	observations.Authority
	claim dispatchintent.Claim
}

func (a queryObservationAuthority) Authorize(ctx context.Context, q observations.Access) (observations.Grant, error) {
	g, err := a.Authority.Authorize(ctx, q)
	if err == nil && (g.Epoch != a.claim.Epoch || g.OwnerID != a.claim.OwnerID || g.ConnectionID != a.claim.ConnectionID || g.OperationID != a.claim.OperationID) {
		return observations.Grant{}, observations.ErrDenied
	}
	return g, err
}

func (a queryAccess) apply(observe observations.Service) func(context.Context, dispatchintent.Claim, []byte) (observations.Receipt, error) {
	return func(ctx context.Context, c dispatchintent.Claim, raw []byte) (observations.Receipt, error) {
		var receipt observations.Receipt
		w := a.a.stand[c.WorkspaceID]
		ctx = withPermit(ctx)
		p, destination, err := a.a.recoveryPermit(ctx, c.WorkspaceID, c.CaseID, w.token)
		if err != nil || p.epoch != c.Epoch || p.principal != c.OwnerID || destination != c.ConnectionID {
			return receipt, observations.ErrDenied
		}
		service := observe
		service.Authority = queryObservationAuthority{observe.Authority, c}
		err = service.Execute(ctx, raw, func(r observations.Receipt) error { receipt = r; return nil })
		return receipt, err
	}
}
