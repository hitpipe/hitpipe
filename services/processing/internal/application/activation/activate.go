// Package activation owns only the explicit acceptance of a checked draft.
// It has no financial execution, provider-write or pause/resume port.
package activation

import (
	"context"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"sync/atomic"
	"time"
)

const Kind drafts.Kind = "activateOperation"

type Request struct {
	WorkspaceID, Session, CSRF, RequestID string
	IfMatch                               *int64
	Body                                  []byte
}
type Input struct {
	Command drafts.ActivationCommand
	Digest  string
}
type Snapshot struct {
	RehearsalReadinessID, RehearsalFinalDigest  string
	RehearsalBinding                            *drafts.BindingProof
	Draft                                       drafts.Snapshot
	Check                                       drafts.ReadinessCheck
	Rehearsal                                   drafts.RehearsalResult
	Examples                                    []drafts.ScenarioExample
	Binding                                     drafts.BindingProof
	RehearsalDigest                             string
	OperationID, OperationOwner, OperationState string
	OperationRevision                           int64
	FirstActivation                             *time.Time
	Replay                                      []byte
}
type Repository interface {
	Load(context.Context, Request, Input, drafts.Grant) (Snapshot, error)
	Commit(context.Context, Request, Input, drafts.Grant, Snapshot, drafts.BindingProof) ([]byte, error)
}
type Codec interface {
	Decode([]byte) (Input, error)
	ValidateResponse([]byte) error
}
type Service struct {
	Repository Repository
	Authority  drafts.Authority
	Boundary   drafts.Boundary
	Bindings   drafts.Bindings
	Clock      drafts.Clock
	Codec      Codec
}

func (s Service) Ready() bool {
	return !drafts.Missing(s.Repository) && !drafts.Missing(s.Authority) && !drafts.Missing(s.Boundary) && !drafts.Missing(s.Bindings) && !drafts.Missing(s.Clock) && !drafts.Missing(s.Codec)
}
func (s Service) Execute(ctx context.Context, r Request, emit func([]byte, int) error) error {
	if ctx == nil || ctx.Err() != nil || !s.Ready() || emit == nil {
		return drafts.ErrUnavailable
	}
	if !drafts.UUID(r.WorkspaceID) || !drafts.UUID(r.RequestID) || len(r.CSRF) < 32 || len(r.CSRF) > 128 {
		return drafts.ErrInvalid
	}
	if r.Session == "" {
		return drafts.ErrUnauthenticated
	}
	in, e := s.Codec.Decode(r.Body)
	if e != nil {
		return e
	}
	if in.Command.Command.RequestID != r.RequestID {
		return drafts.ErrInvalid
	}
	if r.IfMatch != nil && (in.Command.Command.ExpectedRevision == nil || *r.IfMatch != *in.Command.Command.ExpectedRevision) {
		return drafts.ErrStale
	}
	g, e := s.Authority.Authorize(ctx, drafts.Access{Kind: Kind, WorkspaceID: r.WorkspaceID, DraftID: in.Command.DraftID, Session: r.Session, CSRF: r.CSRF})
	if e != nil {
		return e
	}
	valid := func() bool {
		now := s.Clock.Now()
		return !now.IsZero() && g.Allowed && !g.RevocationPending && g.CSRFVerified && g.Role == "OWNER" && g.WorkspaceID == r.WorkspaceID && drafts.UUID(g.PrincipalID) && drafts.UUID(g.SessionID) && g.Epoch > 0 && g.Epoch <= 9007199254740991 && now.Before(g.ExpiresAt)
	}
	if !valid() {
		return drafts.ErrDenied
	}
	protected := func(phase string, action func(context.Context) error) error {
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		var called, active atomic.Bool
		active.Store(true)
		defer active.Store(false)
		var inner error
		e := s.Boundary.Start(c, g, phase, func(b context.Context) error {
			if !active.Load() || !called.CompareAndSwap(false, true) || b == nil || b.Err() != nil || c.Err() != nil || !valid() {
				return drafts.ErrDenied
			}
			w, stop := context.WithCancel(c)
			defer stop()
			end := context.AfterFunc(b, stop)
			defer end()
			inner = action(w)
			return inner
		})
		if inner != nil {
			return inner
		}
		if e != nil {
			return e
		}
		if !called.Load() || c.Err() != nil {
			return drafts.ErrDenied
		}
		return nil
	}
	var snap Snapshot
	if e = protected("ACTIVATION_SQL_CONTENT_READ", func(c context.Context) error { var e error; snap, e = s.Repository.Load(c, r, in, g); return e }); e != nil {
		return e
	}
	body := snap.Replay
	if body == nil {
		c := snap.Draft.Config
		if e = CheckSaved(in, snap, g, s.Clock.Now()); e != nil {
			return e
		}
		call, cancel := context.WithTimeout(ctx, 2*time.Second)
		p, e := s.Bindings.Check(call, g, drafts.BindingQuery{SourceConnectionID: *c.SourceConnectionID, DestinationConnectionID: *c.DestinationConnectionID, Mode: *c.Mode, PriceAlias: *c.PriceAlias, ListAlias: *c.ListAlias, AssigneeAlias: *c.AssigneeAlias})
		cancel()
		if e != nil {
			return drafts.ErrUnavailable
		}
		if !FreshBinding(snap, g, p, s.Clock.Now()) {
			if !p.Granted || p.WorkspaceID != g.WorkspaceID || p.PrincipalID != g.PrincipalID {
				return drafts.ErrDenied
			}
			return drafts.ErrConflict
		}
		if e = protected("ACTIVATION_COMMAND_COMMIT", func(c context.Context) error {
			var e error
			body, e = s.Repository.Commit(c, r, in, g, snap, p)
			return e
		}); e != nil {
			return e
		}
	}
	if s.Codec.ValidateResponse(body) != nil {
		return drafts.ErrUnavailable
	}
	return protected("ACTIVATION_RESPONSE_START", func(c context.Context) error {
		if c.Err() != nil {
			return c.Err()
		}
		return emit(body, 201)
	})
}
func CheckSaved(in Input, s Snapshot, g drafts.Grant, now time.Time) error {
	c, a := s.Draft.Config, in.Command
	if !s.Draft.Exists {
		return drafts.ErrNotFound
	}
	if c.Revision != a.DraftRevision {
		return drafts.ErrStale
	}
	if s.OperationID == "" {
		if a.Command.ExpectedRevision != nil {
			return drafts.ErrStale
		}
	} else {
		if s.OperationOwner != g.PrincipalID {
			return drafts.ErrDenied
		}
		if a.Command.ExpectedRevision == nil || *a.Command.ExpectedRevision != s.OperationRevision {
			return drafts.ErrStale
		}
		// Activation is not a resume, reconfiguration or archive command.
		if s.OperationState != "DRAFT" && s.OperationState != "PREPARED" {
			return drafts.ErrConflict
		}
	}
	if !a.ConfirmActivate || a.AcceptanceTextVersion != "managed-operation-v1" {
		return drafts.ErrInvalid
	}
	if c.SourceConnectionID == nil || c.DestinationConnectionID == nil || c.Mode == nil || c.PriceAlias == nil || c.ListAlias == nil || c.AssigneeAlias == nil || c.ServiceLabel == nil || c.TitlePattern == nil || c.RequiredContextKeys == nil {
		return drafts.ErrConflict
	}
	ck, rr := s.Check, s.Rehearsal
	if ck.CheckID != a.ReadinessCheckID || !ck.Passed || len(ck.Issues) != 0 || ck.DraftID != a.DraftID || ck.DraftRevision != a.DraftRevision || ck.ConfigDigest != s.Draft.ConfigDigest || ck.CheckedAt.IsZero() || ck.CheckedAt.After(now) || !now.Before(ck.ExpiresAt) || !now.Before(s.Draft.ConfigExpiry) {
		return drafts.ErrConflict
	}
	if ck.ActivationPreview == nil || ck.ActivationPreview.AcceptedConfigDigest != a.AcceptedConfigDigest || s.RehearsalReadinessID != ck.CheckID || s.RehearsalFinalDigest != a.AcceptedConfigDigest || s.RehearsalBinding == nil || !drafts.SameBinding(s.Binding, *s.RehearsalBinding) {
		return drafts.ErrConflict
	}
	if rr.RehearsalID != a.RehearsalID || rr.DraftID != a.DraftID || rr.DraftRevision != a.DraftRevision || rr.ConfigDigest != s.Draft.ConfigDigest || rr.LiveRequestsMade != 0 || len(s.Examples) == 0 || len(s.Examples) > 10 || len(rr.Results) != len(s.Examples) {
		return drafts.ErrConflict
	}
	for i, ex := range s.Examples {
		if rr.Results[i] != drafts.RehearseCanonical(c.DraftID, ck.ActivationPreview.CanonicalConfig, ex, now) {
			return drafts.ErrConflict
		}
	}
	if s.Binding.Epoch != g.Epoch || s.Binding.PrincipalID != g.PrincipalID || s.Binding.WorkspaceID != g.WorkspaceID {
		return drafts.ErrConflict
	}
	return nil
}
func FreshBinding(s Snapshot, g drafts.Grant, p drafts.BindingProof, now time.Time) bool {
	return drafts.SameBinding(s.Binding, p) && !p.CheckedAt.Before(s.Draft.LoadedAt) && !p.CheckedAt.After(now) && now.Before(p.ExpiresAt)
}
