package intake

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrDenied      = errors.New("delivery scope denied")
	ErrStale       = errors.New("delivery facts or deadline expired")
	ErrConflict    = errors.New("delivery identity conflict")
	ErrUnavailable = errors.New("delivery admission unavailable")
	ErrNotFound    = errors.New("delivery resource not found")
	ErrQuota       = errors.New("delivery quota exceeded")
)

// Snapshot comes from an independent, current source of authority. A copied
// command or a successful client certificate alone cannot provide authority.
type Snapshot struct {
	Workspace, Case, Operation, Config, Lineage, Episode uuid.UUID
	Connection, SourceFacts                              uuid.UUID
	Epoch                                                int64
	ConfigDigest, FactsDigest                            string
	FactsUntil, Deadline                                 time.Time
	DestinationWorkspace, List, Assignee                 string
}

func (s Snapshot) matches(i Intent) bool {
	return s.Workspace == i.WorkspaceID && s.Case == i.CaseID &&
		s.Operation == i.OperationID && s.Config == i.ConfigID &&
		s.Lineage == i.LineageID && s.Episode == i.EpisodeID &&
		s.Connection == i.ConnectionID && s.Epoch == i.Epoch &&
		s.SourceFacts == i.SourceFactsID && s.ConfigDigest == i.ConfigDigest &&
		s.FactsDigest == i.FactsDigest && s.FactsUntil.Equal(i.FactsUntil) &&
		s.Deadline.Equal(i.Deadline) && s.DestinationWorkspace == i.DestinationWorkspace &&
		s.List == i.List && s.Assignee == i.Assignee
}

type Authority interface {
	Current(ctx context.Context, workspace, caseID uuid.UUID) (Snapshot, error)
}

// StartGate runs a bounded callback only while the runtime and workspace epoch
// permit protected starts. The service supplies no implementation or default.
type StartGate interface {
	WithOpen(ctx context.Context, workspace uuid.UUID, epoch int64, admit func() error) error
}

type Ledger interface {
	Accept(ctx context.Context, intent Intent, now func() time.Time) (Receipt, error)
}

type Receipt struct {
	MessageID   uuid.UUID `json:"message_id"`
	Disposition string    `json:"disposition"`
	RecordID    uuid.UUID `json:"record_id"`
}

type Service struct {
	authority Authority
	gate      StartGate
	ledger    Ledger
}

func New(authority Authority, gate StartGate, ledger Ledger) (*Service, error) {
	if authority == nil || gate == nil || ledger == nil {
		return nil, ErrUnavailable
	}
	return &Service{authority: authority, gate: gate, ledger: ledger}, nil
}

func (s *Service) Accept(ctx context.Context, intent Intent) (Receipt, error) {
	// Resolve external authority before taking a gate or any SQL lock.
	current, err := s.authority.Current(ctx, intent.WorkspaceID, intent.CaseID)
	if err != nil {
		return Receipt{}, err
	}
	if !current.matches(intent) {
		return Receipt{}, ErrDenied
	}
	var receipt Receipt
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = s.gate.WithOpen(ctx, current.Workspace, current.Epoch, func() error {
		var err error
		receipt, err = s.ledger.Accept(ctx, intent, time.Now)
		return err
	})
	if err != nil {
		return Receipt{}, err
	}
	if receipt.RecordID == uuid.Nil {
		return Receipt{}, ErrUnavailable
	}
	return receipt, nil
}
