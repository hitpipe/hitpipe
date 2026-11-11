// Package session implements the opaque HitPipe session domain model.
// Sessions are owned by IAM and stored in hp_iam.sessions. This package
// defines the domain aggregate and its validity rules; it does not perform
// any I/O.
package session

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session is an opaque HitPipe session read from the IAM PostgreSQL
// sessions table. All fields come from a single SELECT row; the adapter
// layer never writes to this table.
type Session struct {
	// ID is the public session identifier (uuid).
	ID uuid.UUID
	// PrincipalID is the owning principal (user) identifier.
	PrincipalID uuid.UUID
	// SessionHash is the opaque session secret stored in the database.
	// It is never returned to callers outside the persistence layer.
	SessionHash []byte
	// CSRFHash is the opaque CSRF protection value stored in the database.
	// It is never returned to callers outside the persistence layer.
	CSRFHash []byte
	// CreatedAt is when the session was created.
	CreatedAt time.Time
	// ExpiresAt is the absolute expiry of the session.
	ExpiresAt time.Time
	// IdleExpiresAt is the idle expiry of the session (<= ExpiresAt).
	IdleExpiresAt time.Time
	// RevokedAt is set when the session has been revoked; nil means active.
	RevokedAt *time.Time
	// Epoch is the security epoch of the session (must be > 0).
	Epoch int64
}

// IsRevoked returns true when revoked_at is set.
func (s *Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

// IsExpired returns true when now is past ExpiresAt.
func (s *Session) IsExpired(now time.Time) bool {
	return now.After(s.ExpiresAt)
}

// IsIdleExpired returns true when now is past IdleExpiresAt.
func (s *Session) IsIdleExpired(now time.Time) bool {
	return now.After(s.IdleExpiresAt)
}

// Valid returns nil when the session is active: not revoked, not expired,
// and not idle-expired at the given now. Errors wrap the sentinel errors
// from this package so callers can use errors.Is for classification.
func (s *Session) Valid(now time.Time) error {
	if s.IsRevoked() {
		return fmt.Errorf("session %s is revoked: %w", s.ID, ErrSessionRevoked)
	}
	if s.IsExpired(now) {
		return fmt.Errorf("session %s is expired: %w", s.ID, ErrSessionExpired)
	}
	if s.IsIdleExpired(now) {
		return fmt.Errorf("session %s is idle-expired: %w", s.ID, ErrSessionIdleExpired)
	}
	return nil
}
