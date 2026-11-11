package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dkar-dev/hitpipe/internal/domain/session"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionRepository is a read-only PostgreSQL adapter for SessionLookup.
// It executes only SELECT statements against hp_iam.sessions and never
// creates, extends, or modifies sessions. All write operations (create,
// revoke, extend) belong to other bounded contexts and are out of scope
// for this adapter.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository creates a read-only SessionRepository backed by pool.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// FindByID returns the session with the given id, or ErrSessionNotFound
// when no such row exists. The result is a read-only snapshot.
//
// Read-only guarantee: this method issues a single SELECT and never issues
// INSERT/UPDATE/DELETE against hp_iam.sessions.
func (r *SessionRepository) FindByID(ctx context.Context, id uuid.UUID) (*session.Session, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT session_id, principal_id, session_hash, csrf_hash,
		       created_at, expires_at, idle_expires_at, revoked_at, epoch
		FROM hp_iam.sessions
		WHERE session_id = $1
	`, id)

	var s session.Session
	var revokedAt *time.Time

	err := row.Scan(
		&s.ID,
		&s.PrincipalID,
		&s.SessionHash,
		&s.CSRFHash,
		&s.CreatedAt,
		&s.ExpiresAt,
		&s.IdleExpiresAt,
		&revokedAt,
		&s.Epoch,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, session.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to scan session row: %w", err)
	}

	s.RevokedAt = revokedAt
	return &s, nil
}
