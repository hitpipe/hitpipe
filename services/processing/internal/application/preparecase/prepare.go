package preparecase

import (
	"context"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"time"
)

type Service struct {
	Repository Repository
	Content    Content
	Readers    Readers
	Renderer   Renderer
	Boundary   Boundary
	Clock      Clock
}

func (s Service) protected(ctx context.Context, scope Scope, work func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	called := false
	e := s.Boundary.Start(bounded, scope, func(c context.Context) error {
		if called || bounded.Err() != nil || c == nil || c.Err() != nil {
			return ErrHold
		}
		called = true
		return work(bounded)
	})
	if e != nil {
		return e
	}
	if !called || bounded.Err() != nil {
		return ErrHold
	}
	return nil
}

type captured struct {
	r evaluatepaidorder.SourceFactsReader
	s evaluatepaidorder.Snapshot
}

func (c *captured) Read(ctx context.Context, q evaluatepaidorder.Query) (evaluatepaidorder.Snapshot, error) {
	s, e := c.r.Read(ctx, q)
	c.s = s
	return s, e
}

// Prepare reuses the existing evaluator through its existing reader port. A VALID
// result is only financial knowledge; the final protected repository transaction
// rechecks every local pin and freshness before accepting one intent.
func (s Service) Prepare(ctx context.Context, r Request) (Result, error) {
	zero := Result{}
	if ctx == nil || ctx.Err() != nil || r.Epoch < 1 || r.ExpectedRevision < 1 || r.WorkspaceID == "" || r.CaseID == "" || s.Repository == nil || s.Content == nil || s.Readers == nil || s.Renderer == nil || s.Boundary == nil || s.Clock == nil {
		return zero, ErrHold
	}
	var c Case
	if e := s.protected(ctx, Scope{Request: r}, func(ctx context.Context) error { var e error; c, e = s.Repository.Load(ctx, r); return e }); e != nil {
		return zero, e
	}
	now := s.Clock.Now()
	if now.IsZero() {
		return zero, ErrHold
	}
	if c.OutboxRef != nil {
		// Revalidate content on retry/restart. Missing/expired content is not recreated
		// as a fresh permission or new message. SQL safety identity remains intact.
		if _, e := s.Content.Read(ctx, *c.OutboxRef, now); e != nil {
			return zero, e
		}
		return Result{c.CaseID, c.State, "PENDING_OUTBOX", c.MessageID, c.Revision}, nil
	}
	if c.OperationState != "ACTIVE" || c.Revision != r.ExpectedRevision || c.Fenced || c.LineageID != "" || (c.Outcome != "UNESTABLISHED" && c.Outcome != "NO_EFFECT") || (c.State != "RECEIVED" && c.State != "WAITING_DATA") {
		return zero, ErrHold
	}
	reader, e := s.Readers.ForCase(ctx, c, r)
	if e != nil || reader == nil {
		return zero, ErrHold
	}
	capture := &captured{r: reader}
	financial := evaluatepaidorder.EvaluatePaidOrder(ctx, evaluatepaidorder.Request{Policy: c.Policy, CaseID: c.CaseID, SessionID: c.SessionID, Now: now}, capture)
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	d := Decision{Financial: financial, Reason: financial.Reason, State: "WAITING_DATA", Snapshot: capture.s, At: s.Clock.Now()}
	if financial.Status == eligibility.Ineligible && financial.Reason != "PAYMENT_STATUS" && financial.Reason != "SOURCE_SCOPE" {
		d.State = "EXCLUDED"
	}
	values := map[string]string{}
	fields := []ContextField{}
	if financial.Status == eligibility.Valid {
		for _, key := range c.Mapping.RequiredKeys {
			found := false
			for _, v := range c.Context {
				if v.Key == key {
					b, e := s.Content.Read(ctx, v.Ref, d.At)
					if e != nil {
						d.Reason = "CONTEXT_UNAVAILABLE"
						break
					}
					var text string
					if json.Unmarshal(b, &text) != nil {
						d.Reason = "CONTEXT_UNAVAILABLE"
						break
					}
					values[key] = text
					fields = append(fields, ContextField{AddedAt: v.AddedAt, ExpiresAt: v.Ref.ExpiresAt, Key: key, Value: text})
					found = true
					break
				}
			}
			if !found {
				d.Reason = "CONTEXT_UNAVAILABLE"
				break
			}
		}
		if len(values) != len(c.Mapping.RequiredKeys) {
			d.Reason = "CONTEXT_UNAVAILABLE"
		}
		if len(values) == len(c.Mapping.RequiredKeys) {
			payload, e := s.Renderer.Build(c, d, values, fields)
			if e != nil {
				d.Reason = "RENDERING_OR_PROOF_INVALID"
			} else {
				expiry := payload.DeadlineAt
				if capture.s.Facts.ValidUntil.Before(expiry) {
					expiry = capture.s.Facts.ValidUntil
				}
				ref, e := s.Content.Write(ctx, payload.Body, "DELIVERY_INTENT", expiry, d.At)
				if e != nil {
					return zero, e
				}
				d.State = "WORKING"
				d.Intent = &ref
				d.EpisodeID = payload.EpisodeID
				d.LineageID = payload.LineageID
				d.MessageID = payload.MessageID
				d.MessageDigest = payload.Digest
				d.DeadlineAt = payload.DeadlineAt
			}
		}
	}
	// Detailed decision/proof is expiring owner content, never SQL JSON. These are
	// recorded observations at At, not a claim that the proof remains fresh later.
	evidence, e := json.Marshal(struct {
		CaseID   string
		Revision int64
		OldState string
		Decision Decision
	}{c.CaseID, c.Revision, c.State, d})
	if e != nil {
		return zero, e
	}
	d.Evidence, e = s.Content.Write(ctx, evidence, "HISTORY", d.At.Add(90*24*time.Hour), d.At)
	if e != nil {
		return zero, e
	}
	var result Result
	e = s.protected(ctx, c.Scope(r), func(ctx context.Context) error {
		var e error
		result, e = s.Repository.Commit(ctx, c, d, s.Clock.Now())
		return e
	})
	if e != nil {
		return zero, e
	}
	return result, nil
}
