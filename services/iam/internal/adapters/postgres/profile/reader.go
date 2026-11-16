// Package profile reads the session owner's current global IAM profile pointer.
// Workspace selectors, membership and browser identity never choose its actor.
package profile

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConfiguration  = errors.New("profile_reader_configuration_invalid")
	ErrInvalidRequest = errors.New("profile_reader_request_invalid")
	ErrUnavailable    = errors.New("profile_reader_unavailable")
)

const queryTimeout = 2 * time.Second
const selectProfile = `SELECT s.session_id::text,s.principal_id::text,s.epoch,
 s.created_at,s.expires_at,s.idle_expires_at,s.revoked_at IS NOT NULL,
 p.disabled_at IS NOT NULL,p.profile_content_ref::text,p.profile_digest
 FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id) WHERE s.session_hash=$1`
const selectRuntime = `SELECT runtime_id::text,service,boot_id::text,generation,certificate_fingerprint,status
 FROM hp_iam.runtime_registry WHERE runtime_id=$1`

// A matching session deny cannot disappear merely because workspace_id is NULL
// or has an unknown relation. Its exact scope is preserved for fail-closed checks.
const selectSessionRevocations = `SELECT scope,subject_id::text,coalesce(workspace_id::text,''),epoch,status
 FROM hp_iam.revocations WHERE scope='SESSION' AND subject_id=$1 ORDER BY epoch,revocation_id`

// Reader borrows the caller's bounded IAM application pool. It owns no profile
// cache or pool lifetime. SQL-argument logging and tracing must remain disabled.
type Reader struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Reader, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 {
		return nil, ErrConfiguration
	}
	return &Reader{pool: pool}, nil
}

// Read returns one fresh, bounded, read-only consistent projection. The caller
// identity comes from a verified transport/process port; this row is an authority
// observation which the application compares exactly under its actual-response
// gate. This method itself grants no protected response and writes no user row.
func (r *Reader) Read(ctx context.Context, reference string, caller authorizeaction.Caller) (getprofile.Snapshot, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return getprofile.Snapshot{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return getprofile.Snapshot{}, err
	}
	digest, ok := referenceDigest(reference)
	if !ok || !validUUID(caller.RuntimeID) {
		return getprofile.Snapshot{}, ErrInvalidRequest
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	failed := func() (getprofile.Snapshot, error) {
		if err := bounded.Err(); err != nil {
			return getprofile.Snapshot{}, err
		}
		return getprofile.Snapshot{}, ErrUnavailable
	}
	tx, err := r.pool.BeginTx(bounded, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return failed()
	}
	defer func() { _ = tx.Rollback(bounded) }()
	var safe bool
	if err := tx.QueryRow(bounded, authorization.ApplicationAuthoritySQL).Scan(&safe); err != nil || !safe {
		if bounded.Err() != nil {
			return failed()
		}
		return getprofile.Snapshot{}, ErrConfiguration
	}
	var snapshot getprofile.Snapshot
	var ref, digestText *string
	err = tx.QueryRow(bounded, selectProfile, digest[:]).Scan(&snapshot.Session.ID, &snapshot.Session.PrincipalID, &snapshot.Session.Epoch,
		&snapshot.Session.Policy.CreatedAt, &snapshot.Session.Policy.ExpiresAt, &snapshot.Session.Policy.IdleExpiresAt, &snapshot.Session.Policy.Revoked,
		&snapshot.Principal.Disabled, &ref, &digestText)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(bounded); err != nil {
			return failed()
		}
		if bounded.Err() != nil {
			return failed()
		}
		return snapshot, nil
	}
	if err != nil {
		return failed()
	}
	snapshot.Session.Found, snapshot.Session.Policy.Found, snapshot.Principal.Found = true, true, true
	snapshot.Principal.ID = snapshot.Session.PrincipalID
	// Keep a missing value absent. Never fabricate a default pointer/digest or
	// normalize malformed stored metadata into a successful profile.
	if ref != nil {
		snapshot.ProfileRef = *ref
	}
	if digestText != nil {
		snapshot.ProfileDigest = *digestText
	}
	err = tx.QueryRow(bounded, selectRuntime, strings.ToLower(caller.RuntimeID)).Scan(&snapshot.Runtime.Caller.RuntimeID, &snapshot.Runtime.Caller.Service, &snapshot.Runtime.Caller.BootID, &snapshot.Runtime.Caller.Generation, &snapshot.Runtime.Caller.CertificateFingerprint, &snapshot.Runtime.Status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return failed()
	}
	snapshot.Runtime.Found = err == nil
	rows, err := tx.Query(bounded, selectSessionRevocations, snapshot.Session.ID)
	if err != nil {
		return failed()
	}
	for rows.Next() {
		var revoke authorizeaction.Revocation
		if err := rows.Scan(&revoke.Scope, &revoke.SubjectID, &revoke.WorkspaceID, &revoke.Epoch, &revoke.Status); err != nil {
			rows.Close()
			return failed()
		}
		snapshot.Revocations = append(snapshot.Revocations, revoke)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return failed()
	}
	if err := tx.Commit(bounded); err != nil {
		return failed()
	}
	if bounded.Err() != nil {
		return failed()
	}
	return snapshot, nil
}
func referenceDigest(value string) ([32]byte, bool) {
	var digest [32]byte
	if len(value) != hex.EncodedLen(len(digest)) {
		return digest, false
	}
	for _, c := range []byte(value) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return digest, false
		}
	}
	_, err := hex.Decode(digest[:], []byte(value))
	return digest, err == nil
}
func validUUID(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for n, c := range []byte(value) {
		if n == 8 || n == 13 || n == 18 || n == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

var _ getprofile.Reader = (*Reader)(nil)
