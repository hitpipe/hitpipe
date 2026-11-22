// Package profileupdate implements the private update of one's own profile.
package profileupdate

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var (
	ErrInvalid         = errors.New("profile update: invalid request")
	ErrUnauthenticated = errors.New("profile update: session not established")
	ErrDenied          = errors.New("profile update: authority denied")
	ErrNotFound        = errors.New("profile update: profile absent")
	ErrConflict        = errors.New("profile update: conflicting command")
	ErrStale           = errors.New("profile update: current revision changed")
	ErrUnavailable     = errors.New("profile update: dependency unavailable")
	ErrUncertain       = errors.New("profile update: commit outcome uncertain")
)

type Command struct {
	RequestID, DisplayName, Timezone string
	ExpectedRevision                 int64
}

type Identity struct {
	Reference, SessionID, PrincipalID string
	SessionEpoch                      int64
	CreatedAt, ExpiresAt, ValidUntil  time.Time
	CSRFHash                          [32]byte
	Caller, Local                     runtimeaccess.Registration
}

type Pointer struct{ Ref, Digest string }

// ProfileTx is limited to the locked and authorized principal for this call.
type ProfileTx interface {
	Pointer() Pointer
	Switch(context.Context, Pointer, Pointer) error
}

type Store interface {
	Identity(context.Context, string, runtimeaccess.Registration, runtimeaccess.Registration) (Identity, error)
	// WithProfile holds current identity and profile facts through use. It
	// commits only a successful Switch and distinguishes uncertain commits.
	WithProfile(context.Context, Identity, func(context.Context, ProfileTx) error) error
}

type Content interface {
	getprofile.Content
	WriteProfile(context.Context, getprofile.Profile) (Pointer, error)
}

type Gate interface {
	Local() runtimeaccess.Registration
	Caller(context.Context, runtimeaccess.Peer) (runtimeaccess.Registration, error)
	Within(context.Context, runtimeaccess.Peer, Identity, func(context.Context) error) error
}

// Receipt contains only implementation references and command identity. Its
// authority is scoped to a single global principal, with no workspace key.
type Receipt struct {
	BodyDigest    string
	Before, After Pointer
	Revision      int64
	Confirmed     bool
}

type CommandRecord interface {
	Load(context.Context) (Receipt, bool, error)
	// Prepare durably fixes the intent, before the SQL pointer can change.
	Prepare(context.Context, Receipt) error
	// Confirm durably promotes the exact intent after the SQL result is known.
	Confirm(context.Context, Receipt) error
}

// CommandAuthority holds exclusive command access for the owner throughout
// use, including reconciliation. An unknown or unconfigured authority denies.
type CommandAuthority interface {
	WithCommand(context.Context, string, string, func(context.Context, CommandRecord) error) error
}
