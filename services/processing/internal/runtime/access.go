package runtime

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"sync/atomic"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals"
	"github.com/jackc/pgx/v5"
)

type access struct {
	iam      iamClient
	gate     *startgate.Gate
	app      postgres.Beginner
	stand    map[string]installed
	stopping atomic.Bool
}

func (a *access) user(ctx context.Context, workspace, token, action string, resource *string) (permit, error) {
	if state, err := a.gate.State(ctx); err != nil || state != startgate.Open {
		return permit{}, drafts.ErrUnavailable
	}
	p, err := a.iam.authorize(ctx, workspace, action, token, resource)
	if err != nil {
		return p, err
	}
	// A service process may hold a real IAM permit but still lack an
	// authoritative membership/binding input for this composition.
	w, ok := a.stand[workspace]
	if !ok {
		return permit{}, drafts.ErrUnavailable
	}
	if p.session != w.config.SessionID || p.principal != w.config.Owner {
		return permit{}, drafts.ErrDenied
	}
	p.connections = []string{w.config.Source, w.config.Destination}
	h := scopeOf(ctx)
	if h == nil {
		return permit{}, drafts.ErrUnavailable
	}
	h.permit = p
	return p, nil
}

func (a *access) local(ctx context.Context, workspace, action string, resource *string) (permit, error) {
	w, ok := a.stand[workspace]
	if !ok {
		return permit{}, drafts.ErrUnavailable
	}
	p, err := a.user(ctx, workspace, w.token, action, resource)
	if err == nil && p.role != "OWNER" {
		return permit{}, drafts.ErrDenied
	}
	return p, err
}

func (a *access) start(ctx context.Context, workspace, principal string, epoch int64, f func(context.Context) error) error {
	h := scopeOf(ctx)
	if h == nil || !h.permit.matches(workspace, principal, epoch) {
		return drafts.ErrDenied
	}
	return gated(ctx, a.gate, h.permit, f)
}

func (a *access) readOwn(ctx context.Context, p permit, f func(context.Context, pgx.Tx) error) error {
	return a.start(ctx, p.workspace, p.principal, p.epoch, func(c context.Context) error {
		tx, err := a.app.BeginTx(c, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		if err != nil {
			return ErrUnavailable
		}
		defer tx.Rollback(c)
		if _, err = tx.Exec(c, `SELECT set_config('hp.workspace_id',$1,true)`, p.workspace); err != nil {
			return ErrUnavailable
		}
		if err = f(c, tx); err != nil {
			return err
		}
		if err = tx.Commit(c); err != nil {
			return ErrUnavailable
		}
		return nil
	})
}

type draftsAccess struct{ a *access }

func (d draftsAccess) Authorize(ctx context.Context, r drafts.Access) (drafts.Grant, error) {
	action := "CONFIG_READ"
	var id *string
	mutation := false
	switch r.Kind {
	case drafts.Get, lifecycle.List, lifecycle.Snapshot:
	case drafts.Save, drafts.Check, drafts.Rehearse:
		action, mutation = "CONFIG_WRITE", true
	case "activateOperation", lifecycle.Change:
		action, mutation = "ACTIVATE", true
	default:
		return drafts.Grant{}, drafts.ErrDenied
	}
	if r.DraftID != "" {
		id = &r.DraftID
	} else if r.OperationID != "" {
		id = &r.OperationID
	}
	p, err := d.a.user(ctx, r.WorkspaceID, r.Session, action, id)
	if err != nil {
		return drafts.Grant{}, err
	}
	verified := false
	if mutation {
		w := d.a.stand[p.workspace]
		h := sha256.Sum256([]byte(r.CSRF))
		verified = subtle.ConstantTimeCompare(h[:], w.csrfHash[:]) == 1
		if !verified {
			return drafts.Grant{}, drafts.ErrDenied
		}
	}
	return drafts.Grant{WorkspaceID: p.workspace, PrincipalID: p.principal, SessionID: p.session,
		Role: p.role, Epoch: p.epoch, ExpiresAt: p.expires, Allowed: true, CSRFVerified: verified}, nil
}

func (d draftsAccess) Start(ctx context.Context, g drafts.Grant, _ string, f func(context.Context) error) error {
	return d.a.start(ctx, g.WorkspaceID, g.PrincipalID, g.Epoch, f)
}

func (d draftsAccess) Check(ctx context.Context, g drafts.Grant, q drafts.BindingQuery) (drafts.BindingProof, error) {
	w, ok := d.a.stand[g.WorkspaceID]
	c := w.config
	if !ok || g.PrincipalID != c.Owner || g.SessionID != c.SessionID || q.SourceConnectionID != c.Source || q.DestinationConnectionID != c.Destination ||
		q.Mode != c.Mode || q.PriceAlias != c.PriceAlias || q.ListAlias != c.ListAlias || q.AssigneeAlias != c.AssigneeAlias {
		return drafts.BindingProof{}, drafts.ErrDenied
	}
	// Lifecycle invokes this port inside its boundary. Other callers acquire the
	// same gate later before persisting/issuing a protected response.
	if err := ctx.Err(); err != nil {
		return drafts.BindingProof{}, err
	}
	now := time.Now().UTC()
	if !now.Before(g.ExpiresAt) {
		return drafts.BindingProof{}, drafts.ErrDenied
	}
	return drafts.BindingProof{WorkspaceID: g.WorkspaceID, PrincipalID: g.PrincipalID, Epoch: g.Epoch, Granted: true,
		CheckedAt: now, ExpiresAt: g.ExpiresAt, SourceConnectionID: c.Source, DestinationConnectionID: c.Destination,
		Mode: c.Mode, PriceAlias: c.PriceAlias, ListAlias: c.ListAlias, AssigneeAlias: c.AssigneeAlias, SourceAccountID: c.Account,
		DestinationWorkspaceID: c.DestinationWorkspace, PriceID: c.Price, ListID: c.List, AssigneeID: c.Assignee, ServiceLabel: c.Label}, nil
}

type casesAccess struct{ a *access }

func (r casesAccess) Authorize(ctx context.Context, q readcases.Request) (readcases.Grant, error) {
	var id *string
	if q.CaseID != "" {
		id = &q.CaseID
	}
	p, err := r.a.user(ctx, q.WorkspaceID, q.Session, "CASE_READ", id)
	if err != nil {
		switch {
		case errors.Is(err, drafts.ErrUnauthenticated):
			return readcases.Grant{}, readcases.ErrUnauthenticated
		case errors.Is(err, drafts.ErrDenied):
			return readcases.Grant{}, readcases.ErrDenied
		default:
			return readcases.Grant{}, readcases.ErrUnavailable
		}
	}
	g := readcases.Grant{WorkspaceID: p.workspace, PrincipalID: p.principal, Epoch: p.epoch, ExpiresAt: p.expires}
	if q.Kind == readcases.Detail {
		permission, connection, e := r.a.recoveryPermit(withPermit(ctx), q.WorkspaceID, q.CaseID, q.Session)
		if e == nil && permission.epoch == p.epoch && permission.principal == p.principal {
			g.RecoverConnection = connection
			g.RecoverUntil = permission.expires
		}
	}
	return g, nil
}

func (r casesAccess) Start(ctx context.Context, g readcases.Grant, _ string, f func(context.Context) error) error {
	err := r.a.start(ctx, g.WorkspaceID, g.PrincipalID, g.Epoch, f)
	if errors.Is(err, drafts.ErrDenied) {
		return readcases.ErrDenied
	}
	return err
}

type intakeAccess struct{ a *access }

func (i intakeAccess) Establish(ctx context.Context, id receivesignals.Identity) (receivesignals.Grant, error) {
	w, ok := i.a.stand[id.WorkspaceID]
	c := w.config
	if !ok || id.ConnectionID != c.Source || id.AccountID != c.Account || id.Live != (c.Mode == "LIVE") {
		return receivesignals.Grant{}, admission.ErrNotAuthorized
	}
	p, err := i.a.local(ctx, id.WorkspaceID, "CONNECTOR_READ", &c.Source)
	if err != nil {
		return receivesignals.Grant{}, admission.ErrNotAuthorized
	}
	ownerCtx := admission.EstablishedContext{WorkspaceID: c.ID, ConnectionID: c.Source, AccountID: c.Account,
		Live: c.Mode == "LIVE", AuthorityVerified: true}
	err = i.a.readOwn(ctx, p, func(cctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(cctx, `SELECT o.operation_id::text,o.revision FROM hp_processing.operations o
 JOIN hp_processing.accepted_configs a ON (a.workspace_id,a.operation_id,a.config_id)=(o.workspace_id,o.operation_id,o.selected_config_id)
 WHERE o.workspace_id=$1 AND o.owner_user_id=$2 AND a.canonical_config->>'source_connection_id'=$3
 AND a.canonical_config->>'destination_connection_id'=$4 AND a.canonical_config->>'mode'=$5
 AND o.state IN ('ACTIVE','DRAINING','PAUSED')
 ORDER BY CASE o.state WHEN 'ACTIVE' THEN 0 WHEN 'DRAINING' THEN 1 ELSE 2 END,o.first_activation_at DESC LIMIT 1`,
			c.ID, c.Owner, c.Source, c.Destination, c.Mode).Scan(&ownerCtx.OperationID, &ownerCtx.ExpectedRevision)
		if errors.Is(err, pgx.ErrNoRows) {
			return admission.ErrNotFound
		}
		return err
	})
	if errors.Is(err, drafts.ErrDenied) {
		err = admission.ErrNotAuthorized
	}
	if err != nil {
		return receivesignals.Grant{}, err
	}
	return receivesignals.Grant{Context: ownerCtx, PrincipalID: p.principal, Epoch: p.epoch, ExpiresAt: p.expires, Allowed: true}, nil
}

func (i intakeAccess) Start(ctx context.Context, g receivesignals.Grant, _ string, f func(context.Context) error) error {
	err := i.a.start(ctx, g.Context.WorkspaceID, g.PrincipalID, g.Epoch, f)
	if errors.Is(err, drafts.ErrDenied) {
		return admission.ErrNotAuthorized
	}
	return err
}

type observationAccess struct{ a *access }

func (o observationAccess) Authorize(ctx context.Context, q observations.Access) (observations.Grant, error) {
	w, ok := o.a.stand[q.WorkspaceID]
	if !ok {
		return observations.Grant{}, observations.ErrDenied
	}
	p, err := o.a.local(ctx, q.WorkspaceID, "CONNECTOR_READ", &w.config.Destination)
	if err != nil {
		return observations.Grant{}, observations.ErrDenied
	}
	out := observations.Grant{WorkspaceID: p.workspace, PrincipalID: p.principal, Epoch: p.epoch, ExpiresAt: p.expires,
		ConnectionID: w.config.Destination, Allowed: true}
	err = o.a.readOwn(ctx, p, func(c context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(c, `SELECT k.operation_id::text,k.owner_user_id::text
 FROM hp_processing.case_identity k JOIN hp_processing.accepted_configs a
 ON (k.workspace_id,k.operation_id,k.config_id)=(a.workspace_id,a.operation_id,a.config_id)
 JOIN hp_processing.episodes e ON (e.workspace_id,e.case_id)=(k.workspace_id,k.case_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2 AND k.owner_user_id=$3 AND k.lineage_id=$4 AND e.episode_id=$5
 AND a.canonical_config->>'destination_connection_id'=$6`,
			q.WorkspaceID, q.CaseID, p.principal, q.LineageID, q.EpisodeID, w.config.Destination).Scan(&out.OperationID, &out.OwnerID)
		if errors.Is(err, pgx.ErrNoRows) {
			return observations.ErrNotFound
		}
		return err
	})
	if errors.Is(err, drafts.ErrDenied) {
		err = observations.ErrDenied
	}
	return out, err
}

func (o observationAccess) Start(ctx context.Context, g observations.Grant, _ string, f func(context.Context) error) error {
	err := o.a.start(ctx, g.WorkspaceID, g.PrincipalID, g.Epoch, f)
	if errors.Is(err, drafts.ErrDenied) {
		return observations.ErrDenied
	}
	return err
}

type dispatchAccess struct{ a *access }

func (d dispatchAccess) Start(ctx context.Context, c dispatchintent.Claim, f func(context.Context) error) error {
	if c.MessageID == "" && d.a.stopping.Load() {
		return dispatchintent.ErrHeld
	}
	w, ok := d.a.stand[c.WorkspaceID]
	if !ok || c.OwnerID != "" && c.OwnerID != w.config.Owner || c.ConnectionID != "" && c.ConnectionID != w.config.Destination {
		return dispatchintent.ErrHeld
	}
	err := d.a.start(ctx, c.WorkspaceID, w.config.Owner, c.Epoch, f)
	if errors.Is(err, drafts.ErrDenied) {
		return dispatchintent.ErrHeld
	}
	return err
}
