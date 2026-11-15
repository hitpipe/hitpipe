// Package issuesession creates an authority-owned session only after a trusted,
// one-use login confirmation. It exposes no HTTP endpoint or production verifier.
package issuesession

import (
	"context"
	"time"
)

// Record contains authority-owned identity and hashes only. Raw session and CSRF
// secrets must never enter a Store, SQL arguments or a log.
type Record struct {
	Issuer, Subject, PrincipalID, SessionID string
	SessionHash, CSRFHash                   [32]byte
	CreatedAt, ExpiresAt, IdleExpiresAt     time.Time
	Epoch                                   int64
}

// Persisted is returned only after the transaction committed successfully. A
// Store resolves an existing principal by (Issuer, Subject), never by email.
type Persisted struct{ PrincipalID, SessionID string }

// Store atomically resolves/creates the principal and creates the session.
// An error, including an unknown commit outcome, must return no success. A caller
// does not retry a consumed login proof or infer success from a database error.
type Store interface {
	Create(context.Context, Record) (Persisted, error)
}
