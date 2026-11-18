// Package observations applies authenticated owner observations, never creates an
// effect, reads a provider, or treats an ID/transport receipt as confirmation.
package observations

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrInvalid     = errors.New("INVALID_OBSERVATION")
	ErrDenied      = errors.New("OBSERVATION_DENIED")
	ErrNotFound    = errors.New("NOT_FOUND")
	ErrConflict    = errors.New("OBSERVATION_CONFLICT")
	ErrStale       = errors.New("OBSERVATION_STALE")
	ErrUnavailable = errors.New("OBSERVATION_UNAVAILABLE")
)
var uuid = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func UUID(s string) bool { return uuid.MatchString(s) }
func Missing(x any) bool {
	if x == nil {
		return true
	}
	v := reflect.ValueOf(x)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

type Access struct{ WorkspaceID, CaseID, LineageID, EpisodeID string }
type Grant struct {
	WorkspaceID, PrincipalID, OwnerID, OperationID, ConnectionID string
	Epoch                                                        int64
	ExpiresAt                                                    time.Time
	Allowed, RevocationPending                                   bool
}
type Authority interface {
	Authorize(context.Context, Access) (Grant, error)
}

// Start is the runtime's shared, synchronous actual-start gate. No runtime
// authority is supplied by message fields, headers, or this local package.
type Boundary interface {
	Start(context.Context, Grant, string, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Codec interface {
	Decode([]byte) (TaskObservation, error)
}
type Snapshot struct {
	WorkspaceID, CaseID, OperationID, ConfigID, OwnerID, ConnectionID, LineageID, EpisodeID string
	State, Outcome                                                                          string
	Revision                                                                                int64
	KnownTaskID                                                                             *string
	LastConfirmedAt                                                                         *time.Time
	LatestObservationAt, EpisodeStartedAt                                                   time.Time
	Fenced                                                                                  bool
	Replay                                                                                  *Receipt
}
type Change struct {
	State, Outcome, Reason string
	KnownTaskID            *string
	Fenced                 bool
	LastConfirmedAt        *time.Time
	Disposition            string
}
type Repository interface {
	Load(context.Context, TaskObservation, Grant) (Snapshot, error)
	Commit(context.Context, TaskObservation, Grant, Snapshot, Change) (Receipt, error)
}
type Service struct {
	Repository Repository
	Authority  Authority
	Boundary   Boundary
	Clock      Clock
	Codec      Codec
}

func (s Service) Ready() bool {
	return !Missing(s.Repository) && !Missing(s.Authority) && !Missing(s.Boundary) && !Missing(s.Clock) && !Missing(s.Codec)
}
func (s Service) Execute(ctx context.Context, body []byte, emit func(Receipt) error) error {
	if ctx == nil || ctx.Err() != nil || !s.Ready() || emit == nil {
		return ErrUnavailable
	}
	o, e := s.Codec.Decode(body)
	if e != nil {
		return e
	}
	grant, e := s.Authority.Authorize(ctx, Access{o.WorkspaceID, o.CaseID, o.LineageID, o.EpisodeID})
	if e != nil {
		return e
	}
	valid := func() bool {
		return grant.Allowed && !grant.RevocationPending && strings.EqualFold(grant.WorkspaceID, o.WorkspaceID) && UUID(grant.PrincipalID) && UUID(grant.OwnerID) && UUID(grant.OperationID) && UUID(grant.ConnectionID) && grant.Epoch > 0 && grant.Epoch <= 9007199254740991 && !s.Clock.Now().IsZero() && grant.ExpiresAt.After(s.Clock.Now())
	}
	if !valid() {
		return ErrDenied
	}
	protected := func(phase string, work func(context.Context) error) error {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		var called, active atomic.Bool
		active.Store(true)
		defer active.Store(false)
		var actionErr error
		e := s.Boundary.Start(bounded, grant, phase, func(c context.Context) error {
			if !active.Load() || !called.CompareAndSwap(false, true) || c == nil || c.Err() != nil || bounded.Err() != nil || !valid() {
				return ErrDenied
			}
			w, stop := context.WithCancel(bounded)
			defer stop()
			end := context.AfterFunc(c, stop)
			defer end()
			actionErr = work(w)
			return actionErr
		})
		if actionErr != nil {
			return actionErr
		}
		if e != nil {
			return e
		}
		if !called.Load() || bounded.Err() != nil {
			return ErrDenied
		}
		return nil
	}
	var snap Snapshot
	if e = protected("OBSERVATION_SQL_CONTENT_READ", func(c context.Context) error { var e error; snap, e = s.Repository.Load(c, o, grant); return e }); e != nil {
		return e
	}
	var receipt Receipt
	if snap.Replay != nil {
		receipt = *snap.Replay
	} else {
		now := s.Clock.Now()
		if o.ObservedAt.IsZero() || o.ObservedAt.After(now) || o.ObservedAt.Before(snap.EpisodeStartedAt) {
			return ErrInvalid
		}
		change := Decide(snap, o, now)
		if e = protected("OBSERVATION_COMMIT", func(c context.Context) error {
			var e error
			receipt, e = s.Repository.Commit(c, o, grant, snap, change)
			return e
		}); e != nil {
			return e
		}
	}
	return protected("OBSERVATION_RESPONSE_START", func(c context.Context) error {
		if c.Err() != nil {
			return c.Err()
		}
		return emit(receipt)
	})
}
func allTrue(o TaskObservation) bool {
	for _, b := range []*bool{o.WorkspaceMatches, o.ListMatches, o.AssigneeMatches, o.TitleMatches, o.MarkerMatches, o.ContextMatches} {
		if b == nil || !*b {
			return false
		}
	}
	return true
}
func anyFalse(o TaskObservation) bool {
	for _, b := range []*bool{o.WorkspaceMatches, o.ListMatches, o.AssigneeMatches, o.TitleMatches, o.MarkerMatches, o.ContextMatches} {
		if b != nil && !*b {
			return true
		}
	}
	return false
}
func noMatches(o TaskObservation) bool {
	return o.WorkspaceMatches == nil && o.ListMatches == nil && o.AssigneeMatches == nil && o.TitleMatches == nil && o.MarkerMatches == nil && o.ContextMatches == nil
}
func hasID(s *string) bool { return s != nil && *s != "" }

// This is an explicit product-state decision table, not an enum confidence rank.
// Evidence is the authenticated Delivery owner's assertion/reference, never an
// independently fetched proof body. Expired/missing evidence cannot confirm.
func Decide(s Snapshot, o TaskObservation, now time.Time) Change {
	c := Change{State: s.State, Outcome: s.Outcome, KnownTaskID: s.KnownTaskID, Fenced: s.Fenced, LastConfirmedAt: s.LastConfirmedAt, Reason: "DELIVERY_OBSERVATION_HELD", Disposition: "HELD"}
	if !s.LatestObservationAt.IsZero() && !o.ObservedAt.UTC().Truncate(time.Microsecond).After(s.LatestObservationAt) {
		c.Reason = "DELIVERY_OBSERVATION_LATE"
		return c
	}
	if s.State == "EXCLUDED" || s.State == "CLOSED_WITHOUT_CONFIRMATION" {
		return c
	}
	if hasID(s.KnownTaskID) && hasID(o.TaskID) && *s.KnownTaskID != *o.TaskID {
		c.Reason = "DELIVERY_IDENTITY_CONFLICT"
		return c
	}
	evidence := o.EvidenceRef != nil && o.EvidenceExpiresAt != nil && o.EvidenceExpiresAt.After(now) && o.EvidenceExpiresAt.After(o.ObservedAt)
	confirm := o.Knowledge == "RESULT_VERIFIED" && o.Complete && allTrue(o) && hasID(o.TaskID) && evidence
	mismatch := o.Knowledge == "RESULT_MISMATCH" && o.Complete && anyFalse(o) && hasID(o.TaskID) && evidence
	switch {
	case confirm:
		c.State = "CONFIRMED"
		c.Outcome = "RESULT_VERIFIED"
		at := o.ObservedAt.UTC().Truncate(time.Microsecond)
		c.LastConfirmedAt = &at
	case mismatch:
		c.State = "ATTENTION"
		c.Outcome = "RESULT_MISMATCH"
	case o.Knowledge == "TARGET_ID_OBSERVED" && hasID(o.TaskID):
		c.State = "UNKNOWN"
		if c.Outcome != "RESULT_VERIFIED" && c.Outcome != "RESULT_MISMATCH" {
			c.Outcome = "TARGET_ID_KNOWN"
		}
	case o.Knowledge == "POSSIBLE_EFFECT" || o.Knowledge == "INCOMPLETE":
		c.State = "UNKNOWN"
		if !hasID(c.KnownTaskID) && c.Outcome != "RESULT_VERIFIED" && c.Outcome != "RESULT_MISMATCH" {
			c.Outcome = "POSSIBLE_EFFECT"
		}
	case o.Knowledge == "KNOWN_NO_EFFECT" && o.Complete && !hasID(o.TaskID) && !hasID(s.KnownTaskID) && noMatches(o) && evidence && s.Outcome != "RESULT_VERIFIED" && s.Outcome != "RESULT_MISMATCH":
		c.State = "ATTENTION"
		c.Outcome = "NO_EFFECT"
	default:
		return c
	}
	if hasID(o.TaskID) && c.KnownTaskID == nil {
		v := *o.TaskID
		c.KnownTaskID = &v
		if c.Outcome == "POSSIBLE_EFFECT" || c.Outcome == "NO_EFFECT" || c.Outcome == "UNESTABLISHED" {
			c.Outcome = "TARGET_ID_KNOWN"
		}
	}
	// Neither NO_EFFECT nor any retry clears an existing dispatch fence. All
	// observations for a prepared lineage keep that lineage unavailable to create.
	c.Fenced = true
	c.Disposition = "APPLIED"
	c.Reason = "DELIVERY_" + o.Knowledge
	return c
}
