package session

import "errors"

var (
	// ErrSessionNotFound is returned when no session row exists for the given id.
	ErrSessionNotFound = errors.New("session not found")
	// ErrSessionRevoked is returned when the session row exists but revoked_at is set.
	ErrSessionRevoked = errors.New("session revoked")
	// ErrSessionExpired is returned when the session is past expires_at.
	ErrSessionExpired = errors.New("session expired")
	// ErrSessionIdleExpired is returned when the session is past idle_expires_at.
	ErrSessionIdleExpired = errors.New("session idle expired")
)
