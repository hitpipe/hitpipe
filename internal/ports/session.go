// Package ports defines application-level port interfaces.
package ports

import (
	"context"

	"github.com/dkar-dev/hitpipe/internal/domain/session"
	"github.com/google/uuid"
)

// SessionLookup is a read-only port for reading IAM sessions from PostgreSQL.
// Implementations must not create, extend, or modify sessions. The port
// intentionally exposes only a read method; write operations (create,
// revoke, extend) belong to other bounded contexts and are out of scope
// for this adapter.
type SessionLookup interface {
	// FindByID returns the session with the given id, or ErrSessionNotFound
	// when no such row exists. The returned session is a read-only snapshot;
	// callers must not mutate it.
	FindByID(ctx context.Context, id uuid.UUID) (*session.Session, error)
}
