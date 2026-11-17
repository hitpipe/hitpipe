// Package drafts owns preparation only. None of its ports can activate an
// operation, create a case/intent or call a provider.
package drafts

import (
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"time"
)

var (
	ErrInvalid         = errors.New("INVALID_REQUEST")
	ErrUnauthenticated = errors.New("UNAUTHENTICATED")
	ErrDenied          = errors.New("DRAFT_ACCESS_DENIED")
	ErrNotFound        = errors.New("DRAFT_NOT_FOUND")
	ErrConflict        = errors.New("COMMAND_CONFLICT")
	ErrStale           = errors.New("REVISION_PRECONDITION_FAILED")
	ErrUnavailable     = errors.New("DRAFT_UNAVAILABLE")
)

type Kind string

const (
	Save     Kind = "saveDraft"
	Get      Kind = "getDraft"
	Check    Kind = "checkDraftReadiness"
	Rehearse Kind = "rehearseDraft"
)

type Request struct {
	Kind                                Kind
	WorkspaceID, DraftID, Session, CSRF string
	IfMatch                             *int64
	Body                                []byte
}
type Input struct {
	ReadinessCheckID string
	Command          CommandMeta
	Config           DraftConfig
	DraftID          string
	DraftRevision    int64
	Examples         []ScenarioExample
	Digest           string
}
type Grant struct {
	WorkspaceID, PrincipalID, SessionID, Role string
	Epoch                                     int64
	ExpiresAt                                 time.Time
	Allowed, RevocationPending, CSRFVerified  bool
}
type Access struct {
	Kind                                Kind
	WorkspaceID, DraftID, Session, CSRF string
}
type Authority interface {
	Authorize(context.Context, Access) (Grant, error)
}

// Boundary is the runtime's same current session/grant/epoch actual-start gate
// across SQL/content and response. It executes synchronously once or denies.
type Boundary interface {
	Start(context.Context, Grant, string, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Codec interface {
	Decode(Kind, []byte) (Input, error)
	Encode(Kind, any) ([]byte, error)
	ValidateResponse(Kind, []byte) error
	ConfigDigest(DraftConfig) (string, error)
	Preview(DraftConfig, BindingProof) (ActivationPreview, error)
}
type Ref struct {
	preparecase.Ref
	Revision int64
}

// FileStore reuses existing fsync/rename owner content, including trusted-pinned
// root reads whose SQL metadata intentionally has no payload byte-count column.
type FileStore interface {
	Write(context.Context, []byte, string, preparecase.Origin, time.Time, time.Time) (preparecase.Ref, error)
	ReadDraft(context.Context, Ref, preparecase.Origin, time.Time) ([]byte, error)
	ReadDraftRoot(context.Context, string, string, time.Time, preparecase.Origin, time.Time) ([]byte, error)
}

// Root and CommandNode are private durable reference envelopes, not another
// product configuration/wire format. Config bytes are the accepted DraftConfig.
type Root struct {
	Format               string
	WorkspaceID, DraftID string
	Revision             int64
	Config               Ref
	ConfigDigest         string
	Tail                 *Ref
}
type CommandNode struct {
	ReadinessCheckID     string `json:",omitempty"`
	AcceptedConfigDigest string `json:",omitempty"`
	// Private expiring evidence needed to verify rehearsal meaning at activation.
	Examples                                         []ScenarioExample `json:",omitempty"`
	OperationID                                      string            `json:",omitempty"`
	OperationRevision                                int64             `json:",omitempty"`
	AuthorityEpoch                                   int64
	BindingCheck                                     *BindingProof
	Format                                           string
	WorkspaceID, DraftID, RequestID, ActorID, Digest string
	Kind                                             Kind
	ExpectedRevision                                 *int64
	ResultRevision                                   int64
	ConfigDigest                                     string
	Response                                         Ref
	Previous                                         *Ref
}
type CheckEvidence struct {
	Check ReadinessCheck
	Proof BindingProof
}
type Snapshot struct {
	SelectedCheck                     *CheckEvidence
	Exists                            bool
	Config                            DraftConfig
	ConfigDigest                      string
	ConfigExpiry, UpdatedAt, LoadedAt time.Time
	Root                              Root
	Replay                            []byte
}
type Candidate struct {
	ReadinessCheckID     string
	AcceptedConfigDigest string
	BindingCheck         *BindingProof
	Config               DraftConfig
	ConfigDigest         string
	ContentExpiry        time.Time
	Response             []byte
	PositiveValidUntil   *time.Time
}
type Repository interface {
	Load(context.Context, Request, Input, Grant) (Snapshot, error)
	Commit(context.Context, Request, Input, Grant, Snapshot, Candidate) ([]byte, error)
}
type BindingProof struct {
	WorkspaceID, PrincipalID, SourceConnectionID, DestinationConnectionID, Mode, PriceAlias, ListAlias, AssigneeAlias string
	SourceAccountID, DestinationWorkspaceID, PriceID, ListID, AssigneeID, ServiceLabel                                string
	Epoch                                                                                                             int64
	Granted                                                                                                           bool
	CheckedAt, ExpiresAt                                                                                              time.Time
}

// Bindings is an explicit current authoritative owner lookup. Its implementation
// must resolve/check the exact supplied aliases and connection scope, not trust
// a BFF/browser claim or a local copy of another service's database.
type BindingQuery struct{ SourceConnectionID, DestinationConnectionID, Mode, PriceAlias, ListAlias, AssigneeAlias string }
type Bindings interface {
	Check(context.Context, Grant, BindingQuery) (BindingProof, error)
}
type Policy struct {
	ContentTTL, CheckTTL time.Duration
	ReplayNodes          int
}

func (p Policy) Valid() bool {
	return p.ContentTTL > 0 && p.ContentTTL <= 90*24*time.Hour && p.CheckTTL > 0 && p.CheckTTL <= p.ContentTTL && p.ReplayNodes > 0
}
