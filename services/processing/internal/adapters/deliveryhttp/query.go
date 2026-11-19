package deliveryhttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
)

type QueryCodec struct{}

func (QueryCodec) Decode(b []byte, c dispatchintent.Claim, now time.Time) (dispatchintent.Message, error) {
	var p recheck.Packet
	q := &p.Query
	if c.Ref.Class != recheck.Kind || draftjson.Hash(b) != c.Digest || json.Unmarshal(b, &p) != nil ||
		q.WorkspaceID != c.WorkspaceID || q.CaseID != c.CaseID || q.LineageID != c.LineageID || q.NativeTaskID == nil ||
		c.NativeTaskID == "" || *q.NativeTaskID != c.NativeTaskID || q.ExpectedConnectionEpoch != c.Epoch ||
		p.EpisodeID != c.EpisodeID || p.OperationID != c.OperationID || p.ConfigID != c.ConfigID || p.ConfigDigest != c.ConfigDigest ||
		p.ConnectionID != c.ConnectionID || p.OwnerID != c.OwnerID || p.Revision != c.Revision ||
		!q.DeadlineAt.Equal(c.DeadlineAt) || !now.Before(q.DeadlineAt) {
		return dispatchintent.Message{}, dispatchintent.ErrHeld
	}
	wire, err := json.Marshal(q)
	if err != nil || draftjson.Validate("DeliveryQuery", wire) != nil {
		return dispatchintent.Message{}, dispatchintent.ErrHeld
	}
	return dispatchintent.Message{Body: wire, FactsUntil: c.Ref.ExpiresAt, Deadline: q.DeadlineAt}, nil
}

type QuerySender struct {
	Transport *Sender
	Apply     func(context.Context, dispatchintent.Claim, []byte) (observations.Receipt, error)
}

func (s QuerySender) Send(ctx context.Context, c dispatchintent.Claim, m dispatchintent.Message, b dispatchintent.Boundary) (dispatchintent.Receipt, error) {
	if s.Transport == nil || s.Apply == nil || c.Ref.Class != recheck.Kind {
		return dispatchintent.Receipt{}, dispatchintent.ErrHeld
	}
	out, raw, err := s.Transport.exchange(ctx, c, m, b, "/internal/v1/delivery-query", http.StatusOK)
	hold := func() (dispatchintent.Receipt, error) {
		out.MessageID, out.Disposition = c.MessageID, "HELD"
		return out, nil
	}
	if err != nil {
		if errors.Is(err, dispatchintent.ErrStale) || ctx.Err() == nil && errors.Is(err, dispatchintent.ErrHeld) {
			return hold()
		}
		return out, err
	}
	if out.HTTPStatus != http.StatusOK {
		if out.HTTPStatus == http.StatusTooManyRequests || out.HTTPStatus == http.StatusServiceUnavailable {
			return out, nil
		}
		return hold()
	}
	o, err := (observationjson.Codec{}).Decode(raw)
	now := s.Transport.clock.Now()
	if err != nil || o.WorkspaceID != c.WorkspaceID || o.CaseID != c.CaseID || o.LineageID != c.LineageID ||
		o.EpisodeID != c.EpisodeID || o.TaskID == nil || *o.TaskID != c.NativeTaskID ||
		o.ObservedAt.Before(c.AcceptedAt) || o.ObservedAt.After(now) || !now.Before(c.DeadlineAt) {
		return hold()
	}
	receipt, err := s.Apply(ctx, c, raw)
	if err != nil {
		if errors.Is(err, observations.ErrUnavailable) || ctx.Err() != nil {
			return out, err
		}
		return hold()
	}
	if receipt.MessageID != o.MessageID || receipt.Disposition != "APPLIED" {
		return hold()
	}
	out.MessageID, out.Disposition = c.MessageID, "APPLIED"
	return out, nil
}
