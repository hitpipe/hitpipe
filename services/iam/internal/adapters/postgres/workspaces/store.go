// Package workspaces reads and writes the current IAM-owned workspace facts
// through the ordinary IAM application role.
package workspaces

import (
	"context"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const queryTimeout = 2 * time.Second

// Check the shared installation guard and the RLS flags of the tenant tables
// used by this adapter. Workspace selectors never stand in for membership.
const checkAuthority = authorization.ApplicationAuthoritySQL + `
 AND (SELECT count(*)=5 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='hp_iam' AND c.relrowsecurity AND c.relforcerowsecurity
        AND c.relname IN('workspaces','memberships','command_receipts','audit_minimal','outbox'))`

const selectIdentity = `SELECT s.session_id::text,s.principal_id::text,s.epoch,s.csrf_hash,
 s.created_at,s.expires_at,s.idle_expires_at,s.revoked_at IS NOT NULL,p.disabled_at IS NOT NULL
 FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id)
 WHERE s.session_hash=$1 FOR SHARE OF s,p`

const selectCaller = `SELECT runtime_id::text,service,generation,boot_id::text,certificate_fingerprint,status
 FROM hp_iam.runtime_registry WHERE runtime_id=$1 AND fence_evidence_ref IS NULL FOR SHARE`

const selectWorkspace = `SELECT w.workspace_id::text,w.name_content_ref::text,m.role,w.lifecycle,
 w.revision,w.security_epoch,m.revision
 FROM hp_iam.workspaces w JOIN hp_iam.memberships m USING(workspace_id)
 WHERE w.workspace_id=$1 AND m.principal_id=$2 AND w.lifecycle IN ('ACTIVE','DELETING','DELETED')
   AND m.status='ACTIVE' AND m.revoked_at IS NULL
   AND NOT EXISTS(SELECT 1 FROM hp_iam.revocations r
                  WHERE (r.scope='WORKSPACE' AND (r.workspace_id=w.workspace_id OR r.subject_id=w.workspace_id))
                     OR (r.scope='MEMBERSHIP' AND (r.workspace_id=w.workspace_id OR r.workspace_id IS NULL)))
 FOR SHARE OF w,m`

// Store borrows the IAM application pool. The owner configures the pool with
// no tracing of SQL arguments and a separate lifecycle from this adapter.
type Store struct {
	pool    *pgxpool.Pool
	session authenticatesession.Service
	clock   authenticatesession.Clock
}

func New(pool *pgxpool.Pool, clock authenticatesession.Clock) (*Store, error) {
	lookup, err := sessionlookup.New(pool)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	session, err := authenticatesession.New(lookup, clock)
	if err != nil {
		return nil, app.ErrUnavailable
	}
	return &Store{pool: pool, session: session, clock: clock}, nil
}

func (s *Store) Identity(ctx context.Context, reference string, caller, local runtimeaccess.Registration) (app.Identity, error) {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil {
		return app.Identity{}, app.ErrUnavailable
	}
	if !caller.Valid() || caller.Service != "BFF" {
		return app.Identity{}, app.ErrDenied
	}
	if !local.Valid() || local.Service != "IAM" {
		return app.Identity{}, app.ErrUnavailable
	}
	if _, err := parseReference(reference); err != nil {
		return app.Identity{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	if !applicationAuthority(ctx, s.pool.QueryRow(ctx, checkAuthority)) {
		return app.Identity{}, app.ErrUnavailable
	}
	session := s.session.Execute(ctx, reference)
	if !session.IsValid() {
		if session.State == "denied" {
			return app.Identity{}, app.ErrUnauthenticated
		}
		return app.Identity{}, app.ErrUnavailable
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return app.Identity{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	return s.currentIdentity(ctx, tx, reference, caller.Normalized(), local.Normalized())
}

func (s *Store) WithRows(ctx context.Context, who app.Identity, ids []string, use func(context.Context, []app.Row) error) error {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil || use == nil {
		return app.ErrUnavailable
	}
	candidates := make([]string, 0, len(ids))
	for _, id := range ids {
		if !runtimeaccess.ValidUUID(id) {
			return app.ErrInvalid
		}
		candidates = append(candidates, strings.ToLower(id))
	}
	sort.Strings(candidates)
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := s.confirmIdentity(ctx, tx, who); err != nil {
		return err
	}
	result := make([]app.Row, 0, len(candidates))
	for i, id := range candidates {
		if i > 0 && candidates[i-1] == id {
			continue
		}
		if err := selectScope(ctx, tx, id); err != nil {
			return err
		}
		row, found, err := readWorkspace(ctx, tx, who.PrincipalID, id)
		if err != nil {
			return err
		}
		if found {
			result = append(result, row)
		}
	}
	guarded, cancelSession, err := s.sessionDeadline(ctx, who)
	if err != nil {
		return err
	}
	defer cancelSession()
	// Row locks and the current revocation set remain held while the caller
	// checks owner content and starts the response under its existing gate.
	return use(guarded, result)
}

func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, app.ErrUnavailable
	}
	if !applicationAuthority(ctx, tx.QueryRow(ctx, checkAuthority)) {
		_ = tx.Rollback(ctx)
		return nil, app.ErrUnavailable
	}
	return tx, nil
}

func applicationAuthority(ctx context.Context, row pgx.Row) bool {
	var approved bool
	return row.Scan(&approved) == nil && ctx.Err() == nil && approved
}

func (s *Store) currentIdentity(ctx context.Context, tx pgx.Tx, reference string, caller, local runtimeaccess.Registration) (app.Identity, error) {
	digest, err := parseReference(reference)
	if err != nil {
		return app.Identity{}, err
	}
	if !caller.Valid() || caller.Service != "BFF" {
		return app.Identity{}, app.ErrDenied
	}
	if !local.Valid() || local.Service != "IAM" || local.RuntimeID == caller.RuntimeID {
		return app.Identity{}, app.ErrUnavailable
	}
	runtimes := [2]runtimeaccess.Registration{caller, local}
	if local.RuntimeID < caller.RuntimeID {
		runtimes = [2]runtimeaccess.Registration{local, caller}
	}
	for _, registration := range runtimes {
		var actual runtimeaccess.Registration
		var status string
		err = tx.QueryRow(ctx, selectCaller, registration.RuntimeID).Scan(
			&actual.RuntimeID, &actual.Service, &actual.Generation, &actual.BootID, &actual.CertificateFingerprint, &status)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (actual != registration || status != "OPEN") {
			if registration.Service == "IAM" {
				return app.Identity{}, app.ErrStale
			}
			return app.Identity{}, app.ErrDenied
		}
		if err != nil {
			return app.Identity{}, app.ErrUnavailable
		}
	}
	// Prevent a newly inserted denial slipping between the last SQL check
	// and the protected SQL effect or actual response start.
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.revocations IN SHARE MODE`); err != nil {
		return app.Identity{}, app.ErrUnavailable
	}
	who := app.Identity{Reference: reference, Caller: caller, Local: local}
	var state sessionpolicy.Snapshot
	var csrfHash []byte
	var disabled bool
	err = tx.QueryRow(ctx, selectIdentity, digest[:]).Scan(
		&who.SessionID, &who.PrincipalID, &who.SessionEpoch, &csrfHash,
		&state.CreatedAt, &state.ExpiresAt, &state.IdleExpiresAt, &state.Revoked, &disabled)
	if errors.Is(err, pgx.ErrNoRows) || disabled {
		return app.Identity{}, app.ErrUnauthenticated
	}
	if err != nil || len(csrfHash) != len(who.CSRFHash) || !positiveWire(who.SessionEpoch) ||
		!runtimeaccess.ValidUUID(who.SessionID) || !runtimeaccess.ValidUUID(who.PrincipalID) {
		return app.Identity{}, app.ErrUnavailable
	}
	var revoked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_iam.revocations WHERE scope='SESSION' AND subject_id=$1)`, who.SessionID).Scan(&revoked)
	if err != nil {
		return app.Identity{}, app.ErrUnavailable
	}
	observation, err := s.clock.Now(ctx)
	if err != nil || !observation.Usable() || ctx.Err() != nil {
		return app.Identity{}, app.ErrUnavailable
	}
	state.Found = true
	state.Revoked = state.Revoked || revoked
	if !sessionpolicy.Evaluate(state, observation).IsValid() {
		return app.Identity{}, app.ErrUnauthenticated
	}
	copy(who.CSRFHash[:], csrfHash)
	who.CreatedAt = state.CreatedAt.UTC()
	who.ExpiresAt = state.ExpiresAt.UTC()
	who.ValidUntil = state.IdleExpiresAt.UTC()
	return who, nil
}

func (s *Store) confirmIdentity(ctx context.Context, tx pgx.Tx, expected app.Identity) (app.Identity, error) {
	current, err := s.currentIdentity(ctx, tx, expected.Reference, expected.Caller, expected.Local)
	if err != nil {
		return app.Identity{}, err
	}
	if current != expected {
		return app.Identity{}, app.ErrStale
	}
	return current, nil
}

func (s *Store) liveNow(ctx context.Context, who app.Identity) (time.Time, error) {
	observation, err := s.clock.Now(ctx)
	if err != nil || !observation.Usable() || ctx.Err() != nil {
		return time.Time{}, app.ErrUnavailable
	}
	if !observation.At.Before(who.ValidUntil) {
		return time.Time{}, app.ErrUnauthenticated
	}
	return observation.At.UTC(), nil
}

// Translate the remaining trusted session lifetime to a local monotonic
// deadline. Counting from before the clock query also bounds sampling latency.
func (s *Store) sessionDeadline(ctx context.Context, who app.Identity) (context.Context, context.CancelFunc, error) {
	started := time.Now()
	observed, err := s.liveNow(ctx, who)
	if err != nil {
		return nil, nil, err
	}
	guarded, cancel := context.WithDeadline(ctx, started.Add(who.ValidUntil.Sub(observed)))
	if guarded.Err() != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, nil, app.ErrUnavailable
		}
		return nil, nil, app.ErrUnauthenticated
	}
	return guarded, cancel, nil
}

func parseReference(reference string) ([32]byte, error) {
	var hash [32]byte
	if !runtimeaccess.ValidFingerprint("sha256:" + reference) {
		return hash, app.ErrUnauthenticated
	}
	_, _ = hex.Decode(hash[:], []byte(reference))
	return hash, nil
}

func positiveWire(value int64) bool {
	return value > 0 && value <= authorizeaction.MaxWireInteger
}

func selectScope(ctx context.Context, tx pgx.Tx, id string) error {
	if _, err := tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, id); err != nil {
		return app.ErrUnavailable
	}
	return nil
}

func readWorkspace(ctx context.Context, tx pgx.Tx, principal, workspace string) (app.Row, bool, error) {
	var row app.Row
	err := tx.QueryRow(ctx, selectWorkspace, workspace, principal).Scan(
		&row.WorkspaceID, &row.NameRef, &row.Role, &row.Lifecycle, &row.Revision, &row.SecurityEpoch, &row.MembershipRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.Row{}, false, nil
	}
	if err != nil {
		return app.Row{}, false, app.ErrUnavailable
	}
	if !runtimeaccess.ValidUUID(row.NameRef) || !positiveWire(row.Revision) ||
		!positiveWire(row.SecurityEpoch) || !positiveWire(row.MembershipRevision) {
		return app.Row{}, false, nil
	}
	switch row.Role {
	case "OWNER", "OPERATOR", "READER":
		return row, true, nil
	default:
		return app.Row{}, false, nil
	}
}

var _ app.Store = (*Store)(nil)
