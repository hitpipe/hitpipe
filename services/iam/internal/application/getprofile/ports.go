// Package getprofile composes one protected read of the session owner's global
// profile. It creates no workspace grant and exposes no permissive runtime gate.
package getprofile

import (
	"context"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
)

// Profile is the exact accepted response. UserID is checked against, and set
// from, the current server session principal rather than browser claims.
type Profile struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Timezone    string `json:"timezone"`
	Revision    int64  `json:"revision"`
}

type Snapshot struct {
	Session       authorizeaction.Session
	Principal     authorizeaction.Principal
	Runtime       authorizeaction.Runtime
	ProfileRef    string
	ProfileDigest string
	Revocations   []authorizeaction.Revocation
}

// Reader returns one fresh IAM-owned SESSION/principal/runtime snapshot; no
// workspace selector or browser user ID establishes identity for this read.
type Reader interface {
	Read(context.Context, string, authorizeaction.Caller) (Snapshot, error)
}

// Content reads exactly the supplied opaque owner reference, verifies its exact
// bytes against the digest and strictly decodes Profile. It neither logs content
// nor keeps a shared profile cache. The private encoding is documented locally.
type Content interface {
	Read(context.Context, string, string) (Profile, error)
}

type Binding struct {
	Caller    authorizeaction.Caller
	Reference string
}

type Facts struct {
	SessionID, PrincipalID, ProfileRef, ProfileDigest string
	SessionEpoch                                      int64
	Caller                                            authorizeaction.Caller
	Revocations                                       []authorizeaction.Revocation
}

// Admission confirms exact current SESSION, principal/profile and runtime facts
// under the same actual-response-start gate. Unknown/old epochs fail closed;
// workspace or membership epochs are not invented for the global profile.
type Admission interface {
	Binding() Binding
	Confirm(context.Context, Facts) error
}

// StartAdmission lets a composed listener hold authoritative SQL facts during
// the final response start. The read-only Admission contract also remains
// available to existing callers that hold those facts across all of Within.
type StartAdmission interface {
	Admission
	Start(context.Context, Facts, func(context.Context) error) error
}

// Gate holds the critical section through fresh SQL, content verification and
// actual response start. It checks eligibility before invoking its callback,
// exactly once, with the bound caller/reference and context. A later I/O failure
// cannot undo an already-started response; callers must never send a second
// response or retry the protected read after its callback has emitted bytes.
// Runtime CLOSED/SYNCING/FENCED, unknown authority and pending deny cannot start.
type Gate interface {
	Within(context.Context, Binding, func(context.Context, Admission) error) error
}
