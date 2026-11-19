// Package readcases owns bounded read-only case projections. It grants no write
// command and never interprets financial eligibility as an external result.
package readcases

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts"
)

var (
	ErrInvalid         = errors.New("INVALID_REQUEST")
	ErrUnauthenticated = errors.New("UNAUTHENTICATED")
	ErrDenied          = errors.New("READ_DENIED")
	ErrNotFound        = errors.New("NOT_FOUND")
	ErrUnavailable     = errors.New("READ_UNAVAILABLE")
	ErrExpired         = errors.New("CONTENT_EXPIRED")
	ErrFutureHistory   = errors.New(casetimeouts.HistoryInvalid)
)

type Kind byte

const (
	List Kind = iota + 1
	Detail
	History
)

type Request struct {
	Kind                                        Kind
	WorkspaceID, CaseID, Session, State, Cursor string
	Limit                                       int
}

// Grant is obtained afresh from the injected IAM owner port, never from browser
// identity/role/epoch headers. Scope is checked again before every protected start.
type Grant struct {
	WorkspaceID, PrincipalID string
	Epoch                    int64
	ExpiresAt                time.Time
}
type Authority interface {
	Authorize(context.Context, Request) (Grant, error)
}
type Boundary interface {
	Start(context.Context, Grant, string, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Repository interface {
	Read(context.Context, Request, Grant) (any, error)
}
type Service struct {
	Authority  Authority
	Boundary   Boundary
	Clock      Clock
	Repository Repository
}

func (s Service) Execute(ctx context.Context, r Request, emit func(any) error) error {
	if ctx == nil || ctx.Err() != nil || s.Authority == nil || s.Boundary == nil || s.Clock == nil || s.Repository == nil || emit == nil {
		return ErrUnavailable
	}
	if !UUID(r.WorkspaceID) || r.Kind < List || r.Kind > History || (r.Kind != List && !UUID(r.CaseID)) || r.Limit < 1 || r.Limit > 100 || (r.State != "" && !State(r.State)) {
		return ErrInvalid
	}
	if r.Session == "" {
		return ErrUnauthenticated
	}
	grant, e := s.Authority.Authorize(ctx, r)
	if e != nil {
		return e
	}
	valid := func() bool {
		return grant.WorkspaceID == r.WorkspaceID && UUID(grant.PrincipalID) && grant.Epoch > 0 && grant.Epoch <= 9007199254740991 && grant.ExpiresAt.After(s.Clock.Now())
	}
	if !valid() {
		return ErrDenied
	}
	var view any
	protected := func(phase string, work func(context.Context) error) error {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return s.Boundary.Start(bounded, grant, phase, func(c context.Context) error {
			if c.Err() != nil || !valid() {
				return ErrDenied
			}
			return work(c)
		})
	}
	if e = protected("SQL_CONTENT_READ", func(c context.Context) error { var e error; view, e = s.Repository.Read(c, r, grant); return e }); e != nil {
		return e
	}
	// Nothing is emitted before the second shared actual-start boundary. A revoke
	// after SQL commit must prevent the protected response from starting.
	return protected("RESPONSE_START", func(c context.Context) error {
		if c.Err() != nil {
			return c.Err()
		}
		return emit(view)
	})
}

type CaseSummary struct {
	CaseID           string     `json:"case_id"`
	OperationID      string     `json:"operation_id"`
	ConfigID         string     `json:"config_id"`
	ConfigDigest     string     `json:"config_digest"`
	State            string     `json:"state"`
	Revision         int64      `json:"revision"`
	OwnerUserID      string     `json:"owner_user_id"`
	OrderLabel       string     `json:"order_label"`
	ReceivedAt       time.Time  `json:"received_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	NextAction       string     `json:"next_action"`
	DueAt            *time.Time `json:"due_at"`
	TargetURL        *string    `json:"target_url"`
	OutcomeKnowledge string     `json:"outcome_knowledge"`
}
type Cases struct {
	Items      []CaseSummary `json:"items"`
	NextCursor *string       `json:"next_cursor"`
	SnapshotAt time.Time     `json:"snapshot_at"`
}
type ContextField struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	AddedAt   time.Time `json:"added_at"`
	ExpiresAt time.Time `json:"expires_at"`
}
type CaseDetail struct {
	Case                CaseSummary    `json:"case"`
	Context             []ContextField `json:"context"`
	ContextComplete     bool           `json:"context_complete"`
	ContextExpired      bool           `json:"context_expired"`
	LastObservationAt   *time.Time     `json:"last_observation_at"`
	ObservationComplete bool           `json:"observation_complete"`
	AvailableActions    []string       `json:"available_actions"`
	HistoryCursor       *string        `json:"history_cursor"`
}
type HistoryEntry struct {
	EntryID     string    `json:"entry_id"`
	CaseID      string    `json:"case_id"`
	At          time.Time `json:"at"`
	ActorKind   string    `json:"actor_kind"`
	ActorID     *string   `json:"actor_id"`
	OldState    *string   `json:"old_state"`
	NewState    string    `json:"new_state"`
	ReasonCode  string    `json:"reason_code"`
	EvidenceRef *string   `json:"evidence_ref"`
	EpisodeID   *string   `json:"episode_id"`
	ExpiresAt   time.Time `json:"expires_at"`
}
type HistoryView struct {
	Items      []HistoryEntry `json:"items"`
	NextCursor *string        `json:"next_cursor"`
}

type Labels struct {
	OrderLabel, NextAction string
	Actions                []string
}
type Record struct {
	Summary         CaseSummary
	Context         []ContextField
	RequiredKeys    []string
	ContextExpired  bool
	LatestReason    string
	Config          []byte
	KnownTaskID     *string
	DispatchFenced  bool
	LastConfirmedAt *time.Time
}
