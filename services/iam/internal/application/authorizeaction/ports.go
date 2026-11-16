// Package authorizeaction composes local, current IAM authorization. It exposes
// no listener and implements neither the distributed revocation coordinator nor
// a permissive production caller, resource-owner or start-gate adapter.
package authorizeaction

import (
	"context"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

// Request is the accepted AuthorizeRequest projection. ResourceID nil represents
// its required JSON null, not an omitted public field. Wire parsing is external.
type Request struct {
	WorkspaceID string  `json:"workspace_id"`
	Action      string  `json:"action"`
	ResourceID  *string `json:"resource_id"`
	RuntimeID   string  `json:"runtime_id"`
}

// Authorization is an OAS response only when Execute returns a nil error. An
// error returns its zero value, which must not be serialized as an epoch=0 reply.
type Authorization struct {
	Allowed           bool      `json:"allowed"`
	Epoch             int64     `json:"epoch"`
	CheckedAt         time.Time `json:"checked_at"`
	RevocationPending bool      `json:"revocation_pending"`
	PrincipalID       string    `json:"principal_id,omitempty"`
	SessionID         string    `json:"session_id,omitempty"`
	WorkspaceID       string    `json:"workspace_id,omitempty"`
	Role              string    `json:"role,omitempty"`
}

// ReferenceDeriver is wired to the existing sessionlookup.Reference function at
// composition time. A raw opaque secret never reaches Reader or StartGate.
type ReferenceDeriver interface{ Reference(string) (string, error) }
type ReferenceFunc func(string) (string, error)

func (f ReferenceFunc) Reference(secret string) (string, error) { return f(secret) }

// Caller must come from a verified service transport/process identity authority,
// never browser-supplied user/role/certificate headers.
type Caller struct {
	Service, RuntimeID, BootID, CertificateFingerprint string
	Generation                                         int64
}
type CallerAuthority interface {
	Current(context.Context) (Caller, error)
}

type ReadRequest struct{ Reference, WorkspaceID, RuntimeID, BindingGrantID string }
type Session struct {
	Found           bool
	ID, PrincipalID string
	Epoch           int64
	Policy          sessionpolicy.Snapshot
}
type Principal struct {
	Found    bool
	ID       string
	Disabled bool
}
type Workspace struct {
	Found         bool
	ID, Lifecycle string
	Epoch         int64
}
type Membership struct {
	Found                                  bool
	WorkspaceID, PrincipalID, Role, Status string
	Revision                               int64
	Revoked                                bool
}
type Runtime struct {
	Found  bool
	Caller Caller
	Status string
}

// BindingGrantID is the IAM external_bindings.grant_id, not a foreign service's
// connection_id, case_id or operation_id. Only a trusted owner may resolve it.
type Binding struct {
	Found                        bool
	GrantID, WorkspaceID, Status string
	Epoch                        int64
}
type Revocation struct {
	Scope, SubjectID, WorkspaceID, Status string
	Epoch                                 int64
}
type Snapshot struct {
	Session     Session
	Principal   Principal
	Workspace   Workspace
	Membership  Membership
	Runtime     Runtime
	Binding     Binding
	Revocations []Revocation
}

// Reader obtains one current IAM-owned projection in a fresh bounded read-only
// transaction. It must not cache grants or read another service's database.
type Reader interface {
	Read(context.Context, ReadRequest) (Snapshot, error)
}
type Actor struct{ PrincipalID, SessionID, WorkspaceID, Role string }

// Proof attests exact resource ownership and the retained operation's semantics.
// A known UUID, client operation name or a broad role is not such a proof.
// Verified attests ALL necessary current conditions and applicable grants for the
// exact retained operation, including required source/target authority. An empty
// BindingGrantID means the trusted owner established that no local IAM binding
// applies; it must never mean unknown, unresolved or omitted required authority.
// In particular, ACTIVATE/CONFIG with unknown necessary source/target grants must
// return an error or Verified=false rather than escape the connection deny filter.
type Proof struct {
	Verified                                                           bool
	PrincipalID, WorkspaceID                                           string
	ResourceID                                                         *string
	Action, Operation, RecoveryAction, LifecycleAction, BindingGrantID string
}

// ResourceAuthority must establish the complete Proof under the admitted scope.
// It must fail closed when any required owner fact/grant cannot be established;
// successful parsing, an empty binding field or an OWNER role is insufficient.
type ResourceAuthority interface {
	Prove(context.Context, Actor, Request) (Proof, error)
}

// ScopeEpochs preserves separately owned epochs. MembershipRevision is a row
// revision, never a fabricated membership epoch. Binding fields are empty/zero
// when no authoritative IAM binding is applicable to the exact operation.
type ScopeEpochs struct {
	PrincipalID, SessionID, WorkspaceID, BindingGrantID            string
	SessionEpoch, WorkspaceEpoch, BindingEpoch, MembershipRevision int64
	Revocations                                                    []Revocation
}
type GateBinding struct {
	Caller    Caller
	Request   Request
	Reference string
}

// Admission is valid only during Within. Confirm checks the separate, exact
// current scope facts under the same gate, then supplies the authority's explicit
// workspace-scoped wire epoch. Historical EFFECTIVE records require an explicit
// current-grant proof accounting for the deny overlay; a numeric comparison
// alone is not that proof. Implementations must not max/merge scope epochs.
type Admission interface {
	Binding() GateBinding
	Confirm(context.Context, ScopeEpochs) (int64, error)
}

// StartGate must hold the actual-start critical section through callback SQL and
// decision creation. It invokes the callback exactly once with the same context
// and binding, and succeeds only if callback and final gate checks succeeded.
// Unknown, CLOSED, SYNCING, FENCED and stale scope observations fail closed.
type StartGate interface {
	Within(context.Context, GateBinding, func(context.Context, Admission) error) error
}
