// Package profileupdate protects an IAM principal's global profile pointer
// using the accepted PostgreSQL schema and IAM application authority.
package profileupdate

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const queryTimeout = 2 * time.Second

const checkAuthority = authorization.ApplicationAuthoritySQL + `
 AND has_column_privilege(current_user,'hp_iam.principals','profile_content_ref','UPDATE')
 AND has_column_privilege(current_user,'hp_iam.principals','profile_digest','UPDATE')
 AND NOT has_table_privilege(current_user,'hp_iam.principals','DELETE,TRUNCATE')`

const selectRuntime = `SELECT runtime_id::text,service,generation,boot_id::text,certificate_fingerprint,status
 FROM hp_iam.runtime_registry WHERE runtime_id=$1 AND fence_evidence_ref IS NULL FOR SHARE`

const selectIdentity = `SELECT s.session_id::text,s.principal_id::text,s.epoch,s.csrf_hash,
 s.created_at,s.expires_at,s.idle_expires_at,s.revoked_at IS NOT NULL,p.disabled_at IS NOT NULL,
 p.profile_content_ref::text,p.profile_digest
 FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id) WHERE s.session_hash=$1`

const selectSessionRevocation = `SELECT EXISTS(SELECT 1 FROM hp_iam.revocations
 WHERE scope='SESSION' AND subject_id=$1)`

const changePointer = `UPDATE hp_iam.principals SET profile_content_ref=$2,profile_digest=$3
 WHERE principal_id=$1 AND disabled_at IS NULL AND profile_content_ref=$4 AND profile_digest=$5`

// Store borrows a bounded, caller-owned IAM pool. The owner disables SQL
// argument tracing and provides a trusted clock for session checks.
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

// Identity first uses the established session consumer, then supplies the
// exact current authority tuple to the caller for guarded work.
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
	if _, err := referenceDigest(reference); err != nil {
		return app.Identity{}, err
	}

	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	if !applicationAuthority(bounded, s.pool.QueryRow(bounded, checkAuthority)) {
		return app.Identity{}, app.ErrUnavailable
	}
	result := s.session.Execute(bounded, reference)
	if !result.IsValid() {
		if result.State == "denied" {
			return app.Identity{}, app.ErrUnauthenticated
		}
		return app.Identity{}, app.ErrUnavailable
	}

	tx, err := s.begin(bounded)
	if err != nil {
		return app.Identity{}, err
	}
	defer func() { _ = tx.Rollback(bounded) }()
	current, err := s.current(bounded, tx, reference, caller.Normalized(), local.Normalized(), false)
	return current.identity, err
}

// WithProfile locks the authenticated session, both runtime registrations,
// global revocation state and the principal during the callback. A callback
// that starts a response uses the current pointer while these locks are held.
// A callback that switches the pointer succeeds only after an acknowledged
// commit. Loss of that acknowledgement leaves an uncertain result.
func (s *Store) WithProfile(ctx context.Context, who app.Identity, use func(context.Context, app.ProfileTx) error) error {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil || use == nil {
		return app.ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	tx, err := s.begin(bounded)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(bounded) }()
	current, err := s.current(bounded, tx, who.Reference, who.Caller, who.Local, true)
	if err != nil {
		return err
	}
	if current.identity != who {
		return app.ErrStale
	}
	if current.ref == nil && current.digest == nil {
		return app.ErrNotFound
	}
	if current.ref == nil || current.digest == nil {
		return app.ErrUnavailable
	}
	pointer := app.Pointer{Ref: *current.ref, Digest: *current.digest}
	if !validPointer(pointer) {
		return app.ErrUnavailable
	}
	guarded, cancelSession, err := s.sessionDeadline(bounded, who)
	if err != nil {
		return err
	}
	defer cancelSession()
	work := &profileTx{tx: tx, ctx: guarded, principalID: who.PrincipalID, pointer: pointer}
	if err := use(guarded, work); err != nil {
		return err
	}
	if guarded.Err() != nil || work.attempted && !work.switched {
		return app.ErrUnavailable
	}
	if !work.switched {
		return nil
	}
	commitCtx, cancelCommit, err := s.sessionDeadline(guarded, who)
	if err != nil {
		return err
	}
	defer cancelCommit()
	return classifyCommit(tx.Commit(commitCtx))
}

type profileTx struct {
	tx                  pgx.Tx
	ctx                 context.Context
	principalID         string
	pointer             app.Pointer
	attempted, switched bool
}

func (p *profileTx) Pointer() app.Pointer { return p.pointer }

func (p *profileTx) Switch(ctx context.Context, old, next app.Pointer) error {
	if ctx == nil || ctx.Err() != nil || p.ctx.Err() != nil {
		return app.ErrUnavailable
	}
	if p.attempted || !validPointer(next) || old.Ref == next.Ref || old.Digest == next.Digest {
		return app.ErrInvalid
	}
	if old != p.pointer {
		return app.ErrStale
	}
	p.attempted = true
	bounded, cancel := context.WithCancel(p.ctx)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	tag, err := p.tx.Exec(bounded, changePointer, p.principalID, next.Ref, next.Digest, old.Ref, old.Digest)
	if err != nil || ctx.Err() != nil || bounded.Err() != nil {
		return app.ErrUnavailable
	}
	if tag.RowsAffected() != 1 {
		return app.ErrStale
	}
	p.switched = true
	return nil
}

func validPointer(pointer app.Pointer) bool {
	return runtimeaccess.ValidUUID(pointer.Ref) && strings.ToLower(pointer.Ref) == pointer.Ref &&
		runtimeaccess.ValidFingerprint(pointer.Digest)
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
	var allowed bool
	return row.Scan(&allowed) == nil && ctx.Err() == nil && allowed
}

type snapshot struct {
	identity    app.Identity
	ref, digest *string
}

func (s *Store) current(ctx context.Context, tx pgx.Tx, reference string, caller, local runtimeaccess.Registration, write bool) (snapshot, error) {
	hash, err := referenceDigest(reference)
	if err != nil {
		return snapshot{}, err
	}
	if err := lockRuntimes(ctx, tx, caller, local); err != nil {
		return snapshot{}, err
	}
	// Row locks alone cannot cover a new revocation. SHARE keeps insert and
	// change of revocation rows ordered against the protected operation.
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.revocations IN SHARE MODE`); err != nil {
		return snapshot{}, app.ErrUnavailable
	}
	lock := " FOR SHARE OF s,p"
	if write {
		lock = " FOR SHARE OF s FOR NO KEY UPDATE OF p"
	}
	result := snapshot{identity: app.Identity{Reference: reference, Caller: caller, Local: local}}
	who := &result.identity
	state := sessionpolicy.Snapshot{Found: true}
	var csrf []byte
	var disabled bool
	err = tx.QueryRow(ctx, selectIdentity+lock, hash[:]).Scan(&who.SessionID, &who.PrincipalID, &who.SessionEpoch, &csrf,
		&state.CreatedAt, &state.ExpiresAt, &state.IdleExpiresAt, &state.Revoked, &disabled, &result.ref, &result.digest)
	if errors.Is(err, pgx.ErrNoRows) || disabled {
		return snapshot{}, app.ErrUnauthenticated
	}
	if err != nil || len(csrf) != len(who.CSRFHash) || who.SessionEpoch < 1 || who.SessionEpoch > authorizeaction.MaxWireInteger ||
		!runtimeaccess.ValidUUID(who.SessionID) || !runtimeaccess.ValidUUID(who.PrincipalID) {
		return snapshot{}, app.ErrUnavailable
	}
	var revoked bool
	if err := tx.QueryRow(ctx, selectSessionRevocation, who.SessionID).Scan(&revoked); err != nil {
		return snapshot{}, app.ErrUnavailable
	}
	now, err := s.clock.Now(ctx)
	if err != nil || !now.Usable() || ctx.Err() != nil {
		return snapshot{}, app.ErrUnavailable
	}
	state.Revoked = state.Revoked || revoked
	if !sessionpolicy.Evaluate(state, now).IsValid() {
		return snapshot{}, app.ErrUnauthenticated
	}
	copy(who.CSRFHash[:], csrf)
	who.CreatedAt = state.CreatedAt.UTC()
	who.ExpiresAt = state.ExpiresAt.UTC()
	who.ValidUntil = state.IdleExpiresAt.UTC()
	return result, nil
}

func lockRuntimes(ctx context.Context, tx pgx.Tx, caller, local runtimeaccess.Registration) error {
	if !caller.Valid() || caller.Service != "BFF" {
		return app.ErrDenied
	}
	if !local.Valid() || local.Service != "IAM" || local.RuntimeID == caller.RuntimeID {
		return app.ErrUnavailable
	}
	runtimes := [2]runtimeaccess.Registration{caller, local}
	if local.RuntimeID < caller.RuntimeID {
		runtimes = [2]runtimeaccess.Registration{local, caller}
	}
	for _, registered := range runtimes {
		var actual runtimeaccess.Registration
		var status string
		err := tx.QueryRow(ctx, selectRuntime, registered.RuntimeID).Scan(&actual.RuntimeID, &actual.Service,
			&actual.Generation, &actual.BootID, &actual.CertificateFingerprint, &status)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (actual != registered || status != "OPEN") {
			if registered.Service == "IAM" {
				return app.ErrStale
			}
			return app.ErrDenied
		}
		if err != nil {
			return app.ErrUnavailable
		}
	}
	return nil
}

// The wall clock of the process may differ from the authority's observation.
// Translate remaining session time to a local monotonic deadline.
func (s *Store) sessionDeadline(ctx context.Context, who app.Identity) (context.Context, context.CancelFunc, error) {
	started := time.Now()
	now, err := s.clock.Now(ctx)
	if err != nil || !now.Usable() || ctx.Err() != nil {
		return nil, nil, app.ErrUnavailable
	}
	if now.At.Before(who.CreatedAt) || !now.At.Before(who.ValidUntil) {
		return nil, nil, app.ErrUnauthenticated
	}
	bounded, cancel := context.WithDeadline(ctx, started.Add(who.ValidUntil.Sub(now.At)))
	if bounded.Err() != nil {
		cancel()
		if ctx.Err() != nil {
			return nil, nil, app.ErrUnavailable
		}
		return nil, nil, app.ErrUnauthenticated
	}
	return bounded, cancel, nil
}

func referenceDigest(reference string) ([32]byte, error) {
	var hash [32]byte
	if !runtimeaccess.ValidFingerprint("sha256:" + reference) {
		return hash, app.ErrUnauthenticated
	}
	_, _ = hex.Decode(hash[:], []byte(reference))
	return hash, nil
}

func classifyCommit(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrTxCommitRollback):
		return app.ErrUnavailable
	default:
		return app.ErrUncertain
	}
}

var _ app.Store = (*Store)(nil)
var _ app.ProfileTx = (*profileTx)(nil)
