// Package workspaces implements only the private owner list/create methods.
package workspaces

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var (
	ErrInvalid         = errors.New("workspaces: invalid request")
	ErrUnauthenticated = errors.New("workspaces: session not established")
	ErrDenied          = errors.New("workspaces: authority not established")
	ErrConflict        = errors.New("workspaces: conflicting command")
	ErrStale           = errors.New("workspaces: current state cannot be established")
	ErrUnavailable     = errors.New("workspaces: dependency unavailable")
	ErrUncertain       = errors.New("workspaces: commit outcome uncertain")
)

type Identity struct {
	Reference, SessionID, PrincipalID string
	SessionEpoch                      int64
	CreatedAt, ExpiresAt, ValidUntil  time.Time
	CSRFHash                          [32]byte
	Caller, Local                     runtimeaccess.Registration
}

type Row struct {
	WorkspaceID, NameRef, Role, Lifecycle       string
	Revision, SecurityEpoch, MembershipRevision int64
}

type Workspace struct {
	WorkspaceID   string `json:"workspace_id"`
	Name          string `json:"name"`
	Role          string `json:"role"`
	Lifecycle     string `json:"lifecycle"`
	Revision      int64  `json:"revision"`
	SecurityEpoch int64  `json:"security_epoch"`
}

// IsPublicLifecycle reports whether the value is an accepted Workspace lifecycle.
func IsPublicLifecycle(value string) bool {
	switch value {
	case "ACTIVE", "DELETING", "DELETED":
		return true
	default:
		return false
	}
}

type Command struct {
	RequestID        string
	Name             string
	ExpectedRevision *int64
}

// Grant is an explicit owner-approved admission, never a browser input. It
// must durably resolve the same (principal, request) to a genuine workspace.
// Creation cannot infer activation, TTL or event vocabulary from a name.
type Grant struct {
	PrincipalID, RequestID, WorkspaceID, Lifecycle string
	AuditAction, AuditReason, MessageKind          string
	OwnerConfirmedAt                               time.Time
	PayloadExpiresAt                               time.Time
}

type Change struct {
	WorkspaceID, RequestID, BodyDigest string
	AuditAction, AuditReason           string
	MessageKind, Lifecycle             string
	OwnerConfirmedAt, PayloadExpiresAt time.Time
}

type Material struct {
	NameRef, AuditID, MessageID, MessageRef, MessageDigest string
}

type Store interface {
	Identity(context.Context, string, runtimeaccess.Registration, runtimeaccess.Registration) (Identity, error)
	// WithRows freshly checks the current session and exact eligible scopes,
	// and holds authoritative facts through the callback's actual start.
	WithRows(context.Context, Identity, []string, func(context.Context, []Row) error) error
	// Create invokes prepare only for a new command before linking content.
	Create(context.Context, Identity, Change, func(context.Context) (Material, error)) (Row, error)
}

type Content interface {
	// Put creates exactly this immutable owner object durably, or verifies an
	// exact existing retry. An error does not authorize a SQL reference.
	Put(context.Context, string, string, string) error
	Read(context.Context, string, string) (string, error)
}

type Candidates interface {
	WorkspaceIDs(context.Context, string) ([]string, error)
}

type Onboarding interface {
	// WithGrant keeps exact, current onboarding admission through its callback.
	// A timeout, missing authority or non-representable lifecycle stays closed.
	WithGrant(context.Context, string, string, func(context.Context, Grant) error) error
}

type Gate interface {
	Local() runtimeaccess.Registration
	Caller(context.Context, runtimeaccess.Peer) (runtimeaccess.Registration, error)
	Within(context.Context, runtimeaccess.Peer, Identity, []Row, func(context.Context) error) error
}
