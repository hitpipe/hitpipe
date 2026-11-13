// Package sessionlookup reads the existing IAM session projection from PostgreSQL.
// It neither issues sessions nor grants business permissions.
package sessionlookup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConfiguration    = errors.New("session_lookup_configuration_invalid")
	ErrInvalidReference = errors.New("session_lookup_reference_invalid")
	ErrUnavailable      = errors.New("session_lookup_unavailable")
)

const queryTimeout = 2 * time.Second

// Only the fields owned by the existing SessionLookup port leave the database.
// A public session UUID is not a session secret and must never select a session.
const selectSnapshot = `SELECT created_at, expires_at, idle_expires_at, revoked_at IS NOT NULL
FROM hp_iam.sessions WHERE session_hash = $1`

// Reference hashes the exact opaque cookie/header value for the internal port.
// It does not trim, decode or otherwise normalize that value. The length bounds
// come from the accepted X-HitPipe-Session schema; entropy is the issuer's duty.
// A future IAM transport must derive this reference itself, never accept a
// browser-supplied digest or public session_id in its place. No transport is wired here.
func Reference(secret string) (string, error) {
	if len(secret) < 43 || len(secret) > 128 {
		return "", ErrInvalidReference
	}
	digest := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(digest[:]), nil
}

// Lookup borrows a caller-owned IAM pool. It owns no cache or mutable session state.
// The caller supplies the IAM database/role and disables logging of SQL arguments.
type Lookup struct {
	pool *pgxpool.Pool
}

// New requires a configured bounded pool; the accepted cohort uses default 5,
// maximum 10 connections per Go service. It does not connect or open another pool.
func New(pool *pgxpool.Pool) (*Lookup, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 {
		return nil, ErrConfiguration
	}
	return &Lookup{pool: pool}, nil
}

// Lookup implements the existing consumer-owned port. reference is the internal
// lowercase SHA-256 reference returned by Reference, not a UUID or raw cookie.
// Each call makes a fresh parameterized SELECT outside any retained transaction.
// Missing rows are Found=false. Errors/cancellation always return a zero snapshot;
// database messages, the secret and its reference never enter returned errors.
func (l *Lookup) Lookup(ctx context.Context, reference string) (sessionpolicy.Snapshot, error) {
	if l == nil || l.pool == nil || ctx == nil {
		return sessionpolicy.Snapshot{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return sessionpolicy.Snapshot{}, err
	}
	if len(reference) != sha256.Size*2 {
		return sessionpolicy.Snapshot{}, ErrInvalidReference
	}
	digest, err := hex.DecodeString(reference)
	if err != nil || hex.EncodeToString(digest) != reference {
		return sessionpolicy.Snapshot{}, ErrInvalidReference
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	var snapshot sessionpolicy.Snapshot
	err = l.pool.QueryRow(bounded, selectSnapshot, digest).Scan(
		&snapshot.CreatedAt, &snapshot.ExpiresAt, &snapshot.IdleExpiresAt, &snapshot.Revoked,
	)
	if cancelled := bounded.Err(); cancelled != nil {
		return sessionpolicy.Snapshot{}, cancelled
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionpolicy.Snapshot{}, nil
	}
	if err != nil {
		return sessionpolicy.Snapshot{}, ErrUnavailable
	}
	snapshot.Found = true
	return snapshot, nil
}

var _ authenticatesession.SessionLookup = (*Lookup)(nil)
