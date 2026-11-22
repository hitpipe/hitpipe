// Package casedeletion binds the maintenance procedure to current IAM authority
// on the admitted IAM database. It accepts no ownership claim from the caller.
package casedeletion

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ownerBudget = 2 * time.Second

// OwnerAuthority borrows the existing restricted IAM application pool (no SQL
// argument tracing). The callback performs a bounded local registry commit,
// with no network or database calls. User consent and target authenticity are
// separate from this check of current identity and role.
type OwnerAuthority struct {
	pool  *pgxpool.Pool
	clock authenticatesession.Clock
}

func New(pool *pgxpool.Pool, clock authenticatesession.Clock) (*OwnerAuthority, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 || clock == nil {
		return nil, app.ErrUnavailable
	}
	// Reuse the accepted constructor's typed-nil clock check.
	if _, err := authenticatesession.New(lockedSession{}, clock); err != nil {
		return nil, app.ErrUnavailable
	}
	return &OwnerAuthority{pool: pool, clock: clock}, nil
}

// WithCurrentOwner observes the accepted session policy and all scoped owner
// facts under row locks. It holds the locks and the deny-insert exclusion until
// the local commit callback finishes. This is a bounded current authorization
// decision, never a reusable lease.
func (a *OwnerAuthority) WithCurrentOwner(ctx context.Context, input app.OwnerCheck, commit func(context.Context, app.OwnerIdentity) error) error {
	if a == nil || a.pool == nil || a.clock == nil {
		return app.ErrUnavailable
	}
	if ctx == nil || commit == nil || !app.ValidID(input.WorkspaceID) {
		return app.ErrInvalid
	}
	reference, err := sessionlookup.Reference(input.Session)
	if err != nil {
		return app.ErrUnauthorized
	}
	digest, err := hex.DecodeString(reference)
	if err != nil {
		return app.ErrUnavailable
	}
	work, cancel := context.WithTimeout(ctx, ownerBudget)
	defer cancel()
	tx, err := a.pool.BeginTx(work, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return unavailable(work)
	}
	defer func() {
		// pgx does not retain transaction cancellation on the backend. Release
		// locks even when the operation deadline or caller context already ended.
		cleanup, stop := context.WithTimeout(context.Background(), ownerBudget)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()

	var allowed bool
	if err := tx.QueryRow(work, checkApplicationAuthority).Scan(&allowed); err != nil || !allowed {
		return unavailable(work)
	}
	// The existing FORCE RLS selector is a boundary, not authorization. The
	// joined row and role are still checked below. Keep this setting local to
	// the transaction; a borrowed pool connection must not retain it.
	if _, err := tx.Exec(work, `SELECT pg_catalog.set_config('hp.workspace_id',$1,true)`, input.WorkspaceID); err != nil {
		return unavailable(work)
	}
	// Keep a server-side idle bound in addition to the 2s application budget,
	// but do not let a shorter preconfigured timeout release locks mid-commit.
	if _, err := tx.Exec(work, `SET LOCAL idle_in_transaction_session_timeout = '3s'`); err != nil {
		return unavailable(work)
	}
	var (
		sessionID, principalID, workspaceID, lifecycle, role, status string
		sessionEpoch, workspaceEpoch, membershipRevision             int64
		principalDisabled, membershipRevoked                         bool
		accepted                                                     *time.Time
		snapshot                                                     sessionpolicy.Snapshot
	)
	err = tx.QueryRow(work, lockOwner, digest, input.WorkspaceID).Scan(
		&sessionID, &principalID, &workspaceID,
		&sessionEpoch, &workspaceEpoch, &membershipRevision,
		&snapshot.CreatedAt, &snapshot.ExpiresAt, &snapshot.IdleExpiresAt, &snapshot.Revoked,
		&principalDisabled, &lifecycle, &role, &status, &membershipRevoked, &accepted,
	)
	if work.Err() != nil {
		return work.Err()
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return app.ErrUnauthorized
	}
	if err != nil {
		return app.ErrUnavailable
	}
	if sessionEpoch < 1 || sessionEpoch > authorizeaction.MaxWireInteger ||
		workspaceEpoch < 1 || workspaceEpoch > authorizeaction.MaxWireInteger ||
		membershipRevision < 1 || membershipRevision > authorizeaction.MaxWireInteger ||
		!app.ValidID(sessionID) || !app.ValidID(principalID) {
		return app.ErrUnavailable
	}
	if principalDisabled || lifecycle != "ACTIVE" || role != "OWNER" || status != "ACTIVE" ||
		membershipRevoked || accepted == nil || workspaceID != input.WorkspaceID {
		return app.ErrUnauthorized
	}

	// A row lock cannot protect the absence of a deny. In this small admitted
	// IAM scope cooperate with actual revocation writes using a short SHARE
	// lock. NOWAIT rejects an overlapping writer instead of waiting with the
	// owner rows held. Once locked, the ReadCommitted statement sees the latest
	// committed denials, including an insert after the owner query began.
	if _, err := tx.Exec(work, `LOCK TABLE hp_iam.revocations IN SHARE MODE NOWAIT`); err != nil {
		return unavailable(work)
	}
	var blocked bool
	if err := tx.QueryRow(work, currentDenials, sessionID, workspaceID).Scan(&blocked); err != nil {
		return unavailable(work)
	}
	if blocked {
		return app.ErrUnauthorized
	}

	// Anchor the bounded grant to monotonic time no later than the actual
	// trusted observation. Clock probing and policy evaluation must not extend it.
	clockStart := time.Now()
	observed, err := a.clock.Now(work)
	if err != nil || !observed.Usable() || work.Err() != nil {
		return unavailable(work)
	}
	if accepted.After(observed.At) || accepted.IsZero() || accepted.UTC().Year() < 1 {
		return app.ErrUnauthorized
	}
	snapshot.Found = true
	authenticator, err := authenticatesession.New(lockedSession{reference, snapshot}, observedClock{observed})
	if err != nil {
		return app.ErrUnavailable
	}
	result := authenticator.Execute(work, reference)
	if !result.IsValid() {
		if result.State == "unavailable" {
			return unavailable(work)
		}
		return app.ErrUnauthorized
	}
	window := min(snapshot.ExpiresAt.Sub(observed.At), snapshot.IdleExpiresAt.Sub(observed.At))
	if window <= 0 {
		return app.ErrUnauthorized
	}
	// The callback checks this context under the file lock and immediately
	// before publication. If the session expires sooner than the normal bound,
	// no subsequent owner decision can use this observation.
	publish, stop := context.WithDeadline(work, clockStart.Add(window))
	defer stop()
	if err := publish.Err(); err != nil {
		return err
	}
	return commit(publish, app.OwnerIdentity{PrincipalID: principalID})
}

type lockedSession struct {
	reference string
	snapshot  sessionpolicy.Snapshot
}

func (s lockedSession) Lookup(ctx context.Context, reference string) (sessionpolicy.Snapshot, error) {
	if ctx.Err() != nil || reference != s.reference {
		return sessionpolicy.Snapshot{}, app.ErrUnavailable
	}
	return s.snapshot, nil
}

type observedClock struct{ observed sessionpolicy.Observation }

func (c observedClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	return c.observed, ctx.Err()
}

func unavailable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return app.ErrUnavailable
}

var _ app.OwnerAuthority = (*OwnerAuthority)(nil)
