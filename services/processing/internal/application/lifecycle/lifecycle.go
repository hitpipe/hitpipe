// Package lifecycle coordinates Processing's existing operation and command state.
package lifecycle

import (
	"context"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

const (
	List     drafts.Kind = "listOperations"
	Change   drafts.Kind = "changeOperationLifecycle"
	Snapshot drafts.Kind = "getResumeSnapshot"

	Pause   = "PAUSE"
	Prepare = "PREPARE_RESUME"
	Accept  = "ACCEPT_RESUME"
	Process = "PROCESS_ELIGIBLE"
	Discard = "DISCARD_WITH_GAP"

	DrainLimit = 300 * time.Second
)

type Request struct {
	Kind                                               drafts.Kind
	WorkspaceID, OperationID, Session, CSRF, RequestID string
	IfMatch                                            *int64
	Body                                               []byte
}

type Input struct {
	drafts.LifecycleCommand
	Digest string
}

type QueueScope struct {
	WorkspaceID, OperationID, ConfigDigest string
	PauseAcceptedAt, FirstActivationAt     time.Time
}

type QueueEvidence struct {
	Scope                        QueueScope
	Generation                   string
	Complete                     bool
	SignalCount, SerializedBytes int64
	OldestReceivedAt             *time.Time
	Gaps                         []string
	ObservedAt, ExpiresAt        time.Time
}

func (e QueueEvidence) Valid(scope QueueScope, now time.Time) bool {
	if e.Scope.WorkspaceID != scope.WorkspaceID || e.Scope.OperationID != scope.OperationID ||
		e.Scope.ConfigDigest != scope.ConfigDigest || !e.Scope.PauseAcceptedAt.Equal(scope.PauseAcceptedAt) ||
		!e.Scope.FirstActivationAt.Equal(scope.FirstActivationAt) || !e.Complete || !drafts.UUID(e.Generation) ||
		e.Gaps == nil || e.SignalCount < 0 || e.SignalCount > 1000 ||
		e.SerializedBytes < 0 || e.SerializedBytes > 16<<20 ||
		e.ObservedAt.Before(scope.PauseAcceptedAt) || e.ObservedAt.After(now) || !now.Before(e.ExpiresAt) ||
		e.ExpiresAt.After(e.ObservedAt.Add(72*time.Hour)) {
		return false
	}
	if e.SignalCount == 0 {
		return e.SerializedBytes == 0 && e.OldestReceivedAt == nil
	}
	return e.SerializedBytes >= e.SignalCount && e.OldestReceivedAt != nil &&
		!e.OldestReceivedAt.After(e.ObservedAt) && now.Before(e.OldestReceivedAt.Add(72*time.Hour)) &&
		!e.ExpiresAt.After(e.OldestReceivedAt.Add(72*time.Hour))
}

// Queue is the authoritative owner's bounded interface. WithCurrent allows
// exactly one synchronous callback under a reservation for the exact snapshot.
// A changed generation must be denied. The callback publishes the owner's
// durable disposition receipt; the reservation itself must not discard or
// release held signals, including when a database COMMIT has an unknown outcome.
type Queue interface {
	Capture(context.Context, drafts.Grant, QueueScope) (QueueEvidence, error)
	WithCurrent(context.Context, drafts.Grant, QueueEvidence, func(context.Context) error) error
}

type SavedSnapshot struct {
	View     drafts.ResumeSnapshot
	Evidence QueueEvidence
	ActorID  string
	Epoch    int64
}

type State struct {
	Operation drafts.Operation
	Scope     QueueScope
	Binding   drafts.BindingProof
	Snapshot  *SavedSnapshot
	Replay    []byte
}

type Candidate struct {
	Queue   *QueueEvidence
	Binding *drafts.BindingProof
}

type Repository interface {
	List(context.Context, Request, drafts.Grant) (drafts.Operations, error)
	Load(context.Context, Request, Input, drafts.Grant) (State, error)
	Commit(context.Context, Request, Input, drafts.Grant, State, Candidate) ([]byte, error)
}

type Codec interface {
	DecodeLifecycle([]byte) (Input, error)
	EncodeLifecycle(drafts.Kind, any) ([]byte, error)
}

type Service struct {
	Repository Repository
	Authority  drafts.Authority
	Boundary   drafts.Boundary
	Bindings   drafts.Bindings
	Queue      Queue
	Clock      drafts.Clock
	Codec      Codec
}

func (s Service) Ready() bool {
	return !drafts.Missing(s.Repository) && !drafts.Missing(s.Authority) &&
		!drafts.Missing(s.Boundary) && !drafts.Missing(s.Bindings) &&
		!drafts.Missing(s.Queue) && !drafts.Missing(s.Clock) && !drafts.Missing(s.Codec)
}

func (s Service) Execute(ctx context.Context, r Request, emit func([]byte, int) error) error {
	if !s.Ready() || ctx == nil || ctx.Err() != nil || emit == nil {
		return drafts.ErrUnavailable
	}
	if !drafts.UUID(r.WorkspaceID) || (r.Kind != List && r.Kind != Snapshot && r.Kind != Change) ||
		(r.Kind != List && !drafts.UUID(r.OperationID)) {
		return drafts.ErrInvalid
	}
	if r.Session == "" || len(r.Session) > 4096 {
		return drafts.ErrUnauthenticated
	}
	var in Input
	if r.Kind == Change {
		if len(r.CSRF) < 32 || len(r.CSRF) > 128 || !drafts.UUID(r.RequestID) {
			return drafts.ErrInvalid
		}
		var err error
		in, err = s.Codec.DecodeLifecycle(r.Body)
		if err != nil {
			return err
		}
		if in.Command.RequestID != r.RequestID {
			return drafts.ErrInvalid
		}
		if in.Command.ExpectedRevision == nil || r.IfMatch != nil && *in.Command.ExpectedRevision != *r.IfMatch {
			return drafts.ErrStale
		}
	}
	g, err := s.Authority.Authorize(ctx, drafts.Access{Kind: r.Kind, WorkspaceID: r.WorkspaceID, OperationID: r.OperationID, Session: r.Session, CSRF: r.CSRF})
	if err != nil {
		return err
	}
	valid := func() bool {
		now := s.Clock.Now()
		role := g.Role == "OWNER" || r.Kind == List && (g.Role == "OPERATOR" || g.Role == "READER")
		return role && !now.IsZero() && now.Before(g.ExpiresAt) && g.Allowed && !g.RevocationPending &&
			g.WorkspaceID == r.WorkspaceID && drafts.UUID(g.PrincipalID) && drafts.UUID(g.SessionID) &&
			g.Epoch > 0 && g.Epoch <= 9007199254740991 && (r.Kind != Change || g.CSRFVerified)
	}
	if !valid() {
		return drafts.ErrDenied
	}
	protect := func(phase string, f func(context.Context) error) error {
		return drafts.Protect(ctx, s.Boundary, g, phase, valid, f)
	}
	var body []byte
	if r.Kind == List {
		err = protect("LIFECYCLE_SQL_CONTENT_READ", func(c context.Context) error {
			v, err := s.Repository.List(c, r, g)
			if err != nil {
				return err
			}
			body, err = s.Codec.EncodeLifecycle(List, v)
			return err
		})
		if err != nil {
			return err
		}
	} else {
		var current State
		err = protect("LIFECYCLE_OPERATION_READ", func(c context.Context) error {
			var e error
			current, e = s.Repository.Load(c, r, in, g)
			return e
		})
		if err != nil {
			return err
		}
		switch {
		case r.Kind == Snapshot:
			if current.Snapshot == nil {
				return drafts.ErrNotFound
			}
			if !SnapshotValid(current, g, s.Clock.Now()) {
				return drafts.ErrConflict
			}
			body, err = s.Codec.EncodeLifecycle(Snapshot, current.Snapshot.View)
		case current.Replay != nil:
			body = current.Replay
		default:
			body, err = s.change(ctx, r, in, g, current, protect)
		}
		if err != nil {
			return err
		}
	}
	status := 200
	if r.Kind == Change {
		status = 202
	}
	return protect("LIFECYCLE_RESPONSE_START", func(context.Context) error { return emit(body, status) })
}

func (s Service) change(ctx context.Context, r Request, in Input, g drafts.Grant, current State,
	protect func(string, func(context.Context) error) error) ([]byte, error) {
	if err := Check(current, in, g, s.Clock.Now()); err != nil {
		return nil, err
	}
	var c Candidate
	if in.Action != Pause {
		err := protect("LIFECYCLE_CURRENT_BINDINGS", func(ctx context.Context) error {
			p := current.Binding
			proof, err := s.Bindings.Check(ctx, g, drafts.BindingQuery{
				SourceConnectionID: p.SourceConnectionID, DestinationConnectionID: p.DestinationConnectionID,
				Mode: p.Mode, PriceAlias: p.PriceAlias, ListAlias: p.ListAlias, AssigneeAlias: p.AssigneeAlias,
			})
			if err != nil {
				return drafts.ErrUnavailable
			}
			if !proof.Granted || proof.WorkspaceID != g.WorkspaceID || proof.PrincipalID != g.PrincipalID {
				return drafts.ErrDenied
			}
			if !BindingValid(p, proof, g, s.Clock.Now()) {
				return drafts.ErrConflict
			}
			c.Binding = &proof
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	if in.Action == Prepare {
		err := protect("LIFECYCLE_QUEUE_SNAPSHOT", func(ctx context.Context) error {
			e, err := s.Queue.Capture(ctx, g, current.Scope)
			if err != nil || !e.Valid(current.Scope, s.Clock.Now()) {
				return drafts.ErrUnavailable
			}
			c.Queue = &e
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var body []byte
	commit := func(cctx context.Context) error {
		var err error
		body, err = s.Repository.Commit(cctx, r, in, g, current, c)
		return err
	}
	if in.Action == Accept {
		err := protect("LIFECYCLE_QUEUE_RESERVATION", func(bounded context.Context) error {
			e := current.Snapshot.Evidence
			// Apply the same start guard to the reserved callback, even if a
			// misconfigured adapter calls it late or more than once.
			reservation := reservationGate{s.Queue, e}
			return drafts.Protect(bounded, reservation, g, "LIFECYCLE_COMMIT", func() bool {
				return SnapshotValid(current, g, s.Clock.Now())
			}, commit)
		})
		return body, err
	}
	err := protect("LIFECYCLE_COMMIT", commit)
	return body, err
}

type reservationGate struct {
	Queue
	evidence QueueEvidence
}

func (g reservationGate) Start(ctx context.Context, grant drafts.Grant, _ string, callback func(context.Context) error) error {
	return g.WithCurrent(ctx, grant, g.evidence, callback)
}

func BindingValid(saved, current drafts.BindingProof, g drafts.Grant, now time.Time) bool {
	saved.Epoch = g.Epoch // current epoch is checked at the boundary, not read from an old activation
	saved.PrincipalID = g.PrincipalID
	return current.Granted && current.WorkspaceID == g.WorkspaceID && current.PrincipalID == g.PrincipalID &&
		current.Epoch == g.Epoch && !current.CheckedAt.IsZero() && !current.CheckedAt.After(now) &&
		now.Before(current.ExpiresAt) && drafts.SameBinding(saved, current)
}

func SnapshotValid(s State, g drafts.Grant, now time.Time) bool {
	p := s.Snapshot
	return p != nil && s.Operation.State == "PAUSED" && p.ActorID == g.PrincipalID && p.Epoch == g.Epoch &&
		p.View.OperationID == s.Operation.OperationID && p.View.Revision == s.Operation.Revision &&
		drafts.UUID(p.View.SnapshotID) && !p.View.CreatedAt.After(now) && now.Before(p.View.ExpiresAt) &&
		p.Evidence.Valid(s.Scope, now)
}

func Check(s State, in Input, g drafts.Grant, now time.Time) error {
	if in.Command.ExpectedRevision == nil || s.Operation.Revision != *in.Command.ExpectedRevision {
		return drafts.ErrStale
	}
	switch in.Action {
	case Pause:
		if s.Operation.State == "ACTIVE" {
			return nil
		}
	case Prepare:
		if s.Operation.State == "PAUSED" {
			return nil
		}
	case Accept:
		if SnapshotValid(s, g, now) && in.ResumeSnapshotID != nil && s.Snapshot.View.SnapshotID == *in.ResumeSnapshotID {
			return nil
		}
	}
	return drafts.ErrConflict
}

// ResumeDecision is a readback of the Processing commit for the exact held
// generation. Only its owner can apply it. Automatic release still uses the
// existing admission check (bound operation, revision and first activation).
type ResumeDecision struct {
	Scope                                          QueueScope
	RequestID, SnapshotID, Generation, Disposition string
	ResultRevision, SignalCount, SerializedBytes   int64
	OldestReceivedAt                               *time.Time
	Gaps                                           []string
	ObservedAt                                     time.Time
}
