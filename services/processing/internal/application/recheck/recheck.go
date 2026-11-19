// Package recheck accepts a read-only recovery command for an established task.
package recheck

import (
	"context"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

const Kind = "DELIVERY_QUERY"

type Request struct {
	WorkspaceID, CaseID, Session, CSRF, RequestID string
	IfMatch                                       *int64
	Body                                          []byte
}

type Input struct {
	Command drafts.RecoveryCommand
	Digest  string
}

type Grant struct {
	WorkspaceID, OwnerID, ConnectionID string
	Epoch                              int64
	ExpiresAt                          time.Time
}

type Authority interface {
	Authorize(context.Context, Request) (Grant, error)
}

type Repository interface {
	Accept(context.Context, Request, Input, Grant) (readcases.CaseDetail, error)
}

type Codec interface {
	Decode([]byte) (Input, error)
}

type Service struct {
	Repository Repository
	Authority  Authority
	Boundary   dispatchintent.Boundary
	Codec      Codec
	Clock      dispatchintent.Clock
}

func (s Service) Ready() bool {
	return !dispatchintent.Missing(s.Repository) && !dispatchintent.Missing(s.Authority) &&
		!dispatchintent.Missing(s.Boundary) && !dispatchintent.Missing(s.Codec) && !dispatchintent.Missing(s.Clock)
}

func (s Service) Execute(ctx context.Context, r Request, emit func(readcases.CaseDetail) error) error {
	if ctx == nil || ctx.Err() != nil || !s.Ready() || emit == nil {
		return drafts.ErrUnavailable
	}
	if !drafts.UUID(r.WorkspaceID) || !drafts.UUID(r.CaseID) || !drafts.UUID(r.RequestID) {
		return drafts.ErrInvalid
	}
	in, err := s.Codec.Decode(r.Body)
	if err != nil {
		return err
	}
	meta := in.Command.Command
	if in.Command.Action != "RECHECK" || meta.RequestID != r.RequestID || meta.ExpectedRevision == nil ||
		(r.IfMatch != nil && *r.IfMatch != *meta.ExpectedRevision) {
		return drafts.ErrInvalid
	}
	if r.Session == "" {
		return drafts.ErrUnauthenticated
	}
	g, err := s.Authority.Authorize(ctx, r)
	if err != nil {
		return err
	}
	if g.WorkspaceID != r.WorkspaceID || !drafts.UUID(g.OwnerID) || !drafts.UUID(g.ConnectionID) ||
		g.Epoch < 1 || g.Epoch > 9007199254740991 || !g.ExpiresAt.After(s.Clock.Now()) {
		return drafts.ErrDenied
	}
	c := dispatchintent.Claim{Request: dispatchintent.Request{WorkspaceID: g.WorkspaceID, Epoch: g.Epoch},
		OwnerID: g.OwnerID, CaseID: r.CaseID, ConnectionID: g.ConnectionID}
	return dispatchintent.Protected(ctx, s.Boundary, c, func(ctx context.Context) error {
		view, err := s.Repository.Accept(ctx, r, in, g)
		if err != nil {
			return err
		}
		if ctx.Err() != nil || !g.ExpiresAt.After(s.Clock.Now()) {
			return drafts.ErrDenied
		}
		return emit(view)
	})
}

// Packet is expiring, Processing-owned data. Only the exact Query is sent.
// The other pins bind the queued request to the previously observed effect.
type Packet struct {
	Query                                                                 drafts.DeliveryQuery
	EpisodeID, OperationID, ConfigID, ConfigDigest, ConnectionID, OwnerID string
	Revision                                                              int64
}
