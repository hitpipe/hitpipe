package profile

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
)

// SessionSubject discovers only the global gate key and lifetime. Profile
// contents and authorities are read separately inside the gate.
func (r *Reader) SessionSubject(ctx context.Context, reference string) (authorizeaction.Session, error) {
	if r == nil || r.pool == nil || ctx == nil || ctx.Err() != nil {
		return authorizeaction.Session{}, ErrUnavailable
	}
	digest, valid := referenceDigest(reference)
	if !valid {
		return authorizeaction.Session{}, getprofile.ErrUnauthenticated
	}
	var session authorizeaction.Session
	var authorized bool
	err := r.pool.QueryRow(ctx, `SELECT session_id::text,epoch,created_at,expires_at,idle_expires_at,
 revoked_at IS NOT NULL, (`+authorization.ApplicationAuthoritySQL+`)
 FROM hp_iam.sessions WHERE session_hash=$1`, digest[:]).Scan(
		&session.ID, &session.Epoch, &session.Policy.CreatedAt, &session.Policy.ExpiresAt,
		&session.Policy.IdleExpiresAt, &session.Policy.Revoked, &authorized)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return authorizeaction.Session{}, getprofile.ErrUnauthenticated
	case err != nil || !authorized || !validUUID(session.ID) || session.Epoch < 1 || session.Epoch > authorizeaction.MaxWireInteger:
		return authorizeaction.Session{}, ErrUnavailable
	}
	session.Found, session.Policy.Found = true, true
	return session, nil
}

// HoldCurrent pins the current IAM and BFF registrations, session, profile
// pointer and revocations while the caller starts the response. It is called
// at the end of the existing getprofile callback, so the earlier reads do not
// contend with a held connection even when the pool has one connection.
func (r *Reader) HoldCurrent(ctx context.Context, local authorizeaction.Caller,
	binding getprofile.Binding, subject authorizeaction.Session, facts getprofile.Facts) (release func(), err error) {
	digest, valid := referenceDigest(binding.Reference)
	if r == nil || r.pool == nil || ctx == nil || ctx.Err() != nil || !valid || binding.Caller != facts.Caller || subject.ID != facts.SessionID || subject.Epoch != facts.SessionEpoch ||
		len(facts.Revocations) != 0 || local.RuntimeID == facts.Caller.RuntimeID {
		return nil, ErrUnavailable
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	rollback := func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 250*time.Millisecond)
		defer cancel()
		_ = tx.Rollback(cleanup)
	}
	defer func() {
		if release == nil {
			rollback()
		}
	}()
	var safe bool
	if err := tx.QueryRow(ctx, authorization.ApplicationAuthoritySQL).Scan(&safe); err != nil || !safe {
		return nil, ErrUnavailable
	}
	if err := holdRuntimes(ctx, tx, local, facts.Caller); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `LOCK TABLE hp_iam.revocations IN SHARE MODE`); err != nil {
		return nil, ErrUnavailable
	}
	var sessionID, principalID string
	var epoch int64
	var policy sessionpolicy.Snapshot
	var disabled bool
	var ref, contentDigest *string
	err = tx.QueryRow(ctx, selectProfile+` FOR SHARE OF s,p`, digest[:]).Scan(&sessionID, &principalID, &epoch,
		&policy.CreatedAt, &policy.ExpiresAt, &policy.IdleExpiresAt, &policy.Revoked, &disabled, &ref, &contentDigest)
	if err != nil || sessionID != facts.SessionID || principalID != facts.PrincipalID || epoch != facts.SessionEpoch ||
		policy.Revoked || disabled || !policy.CreatedAt.Equal(subject.Policy.CreatedAt) || !policy.ExpiresAt.Equal(subject.Policy.ExpiresAt) ||
		!policy.IdleExpiresAt.Equal(subject.Policy.IdleExpiresAt) || ref == nil || *ref != facts.ProfileRef ||
		contentDigest == nil || *contentDigest != facts.ProfileDigest {
		return nil, ErrUnavailable
	}
	var denied bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_iam.revocations
 WHERE scope='SESSION' AND subject_id=$1)`, sessionID).Scan(&denied); err != nil || denied || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	return rollback, nil
}

func holdRuntimes(ctx context.Context, tx pgx.Tx, local, caller authorizeaction.Caller) error {
	runtimes := [2]authorizeaction.Caller{local, caller}
	if caller.RuntimeID < local.RuntimeID {
		runtimes = [2]authorizeaction.Caller{caller, local}
	}
	for _, expected := range runtimes {
		var actual authorizeaction.Caller
		var status string
		err := tx.QueryRow(ctx, selectRuntime+` AND fence_evidence_ref IS NULL FOR SHARE`, expected.RuntimeID).Scan(
			&actual.RuntimeID, &actual.Service, &actual.BootID, &actual.Generation, &actual.CertificateFingerprint, &status)
		if err != nil || actual != expected || status != "OPEN" {
			return ErrUnavailable
		}
	}
	return nil
}
