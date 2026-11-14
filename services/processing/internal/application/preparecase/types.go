// Package preparecase composes local financial knowledge and a pending outbox.
// It never sends a Delivery request or reports a confirmed external effect.
package preparecase

import (
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"time"
)

var ErrHold = errors.New("PREPARATION_HELD")
var ErrStale = errors.New("STALE_CASE_OR_OPERATION")
var ErrContent = errors.New("CONTENT_UNAVAILABLE")

type Request struct {
	WorkspaceID, CaseID     string
	ExpectedRevision, Epoch int64
}
type Scope struct {
	Request
	OperationID, ConfigID, SourceConnectionID, DestinationConnectionID string
}

// Boundary must synchronously run start once under current authority/binding/epoch
// or deny without calling it. No default or runtime IAM implementation is supplied.
// Only protected SQL runs inside its callback, with a maximum two-second deadline.
type Boundary interface {
	Start(context.Context, Scope, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Ref struct {
	ID, Digest, Class string
	Bytes             int
	ExpiresAt         time.Time
}
type Content interface {
	Write(context.Context, []byte, string, time.Time, time.Time) (Ref, error)
	Read(context.Context, Ref, time.Time) ([]byte, error)
}
type ContextRef struct {
	Key     string
	Ref     Ref
	AddedAt time.Time
}
type Mapping struct {
	SourceConnectionID, DestinationConnectionID, DestinationWorkspaceID, ListID, AssigneeID, PriceID, ServiceLabel, TitlePattern string
	RequiredKeys                                                                                                                 []string
}
type Case struct {
	OperationState                      string
	EpisodeID                           string
	EpisodeStartedAt, EpisodeDeadlineAt time.Time
	HasEpisodes                         bool
	Request
	OperationID, ConfigID, ConfigDigest, OwnerID, SessionID, AccountID, State, Outcome string
	Live, Fenced                                                                       bool
	Revision, OperationRevision                                                        int64
	ActivationAt, AdmittedAt                                                           time.Time
	Policy                                                                             eligibility.Policy
	Mapping                                                                            Mapping
	Context                                                                            []ContextRef
	LineageID, MessageID, MessageDigest                                                string
	OutboxRef                                                                          *Ref
}

func (c Case) Scope(r Request) Scope {
	return Scope{r, c.OperationID, c.ConfigID, c.Mapping.SourceConnectionID, c.Mapping.DestinationConnectionID}
}

type Decision struct {
	Financial                                      eligibility.Result
	Reason, State                                  string
	Snapshot                                       evaluatepaidorder.Snapshot
	At                                             time.Time
	EpisodeID, LineageID, MessageID, MessageDigest string
	DeadlineAt                                     time.Time
	Evidence                                       Ref
	Intent                                         *Ref
}
type Result struct {
	CaseID, State, Disposition, MessageID string
	Revision                              int64
}
type Repository interface {
	Load(context.Context, Request) (Case, error)
	Commit(context.Context, Case, Decision, time.Time) (Result, error)
}

// Readers binds the existing SourceFactsReader to the pinned case/policy and
// expected epoch. Its owner controls HTTPS/authenticity and actual network start.
type Readers interface {
	ForCase(context.Context, Case, Request) (evaluatepaidorder.SourceFactsReader, error)
}
type Payload struct {
	Body                                    []byte
	EpisodeID, LineageID, MessageID, Digest string
	DeadlineAt                              time.Time
}
type Renderer interface {
	Build(Case, Decision, map[string]string, []ContextField) (Payload, error)
}
