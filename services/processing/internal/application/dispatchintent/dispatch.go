// Package dispatchintent delivers an existing immutable internal message. A
// durable Delivery receipt is never an observation of an external task.
package dispatchintent

import (
	"context"
	"crypto/rand"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"math/big"
	"reflect"
	"sync/atomic"
	"time"
)

var ErrHeld = errors.New("DELIVERY_SEND_HELD")
var ErrStale = errors.New("DELIVERY_LEASE_STALE")

type Request struct {
	WorkspaceID string
	Epoch       int64
}
type Job struct{ WorkspaceID, MessageID string }
type Claim struct {
	Request
	MessageID, Token, CaseID, OperationID, ConfigID, ConfigDigest, OwnerID, ConnectionID, EpisodeID, LineageID, Digest string
	Revision                                                                                                           int64
	NativeTaskID                                                                                                       string
	AcceptedAt                                                                                                         time.Time
	Ref                                                                                                                preparecase.Ref
	LeaseUntil, DeadlineAt                                                                                             time.Time
	Attempts                                                                                                           int
}

// Boundary is the SAME runtime-owned current authority/binding/epoch/start gate
// used for scoped SQL and actual network starts. It must execute work once,
// synchronously, or deny without invoking it. No production default is supplied.
type Boundary interface {
	Start(context.Context, Claim, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Repository interface {
	Index(context.Context, Request, int) error
	Jobs(context.Context, string, int) ([]Job, error) // metadata only, separate worker transaction
	Claim(context.Context, Request, string, time.Duration) (Claim, error)
	Check(context.Context, Claim) error
	Finish(context.Context, Claim, string, time.Time) error
}
type Message struct {
	Body                 []byte
	FactsUntil, Deadline time.Time
}
type Decoder interface {
	Decode([]byte, Claim, time.Time) (Message, error)
}
type Receipt struct {
	MessageID, Disposition string
	HTTPStatus             int
	RetryAfter             time.Time
}
type Sender interface {
	Send(context.Context, Claim, Message, Boundary) (Receipt, error)
}
type Policy struct {
	Batch, MaxAttempts                             int
	Lease, RequestTimeout, BaseBackoff, MaxBackoff time.Duration
}

func (p Policy) Valid() bool {
	return p.Batch > 0 && p.Batch <= 100 && p.MaxAttempts > 0 && p.MaxAttempts <= 1000 && p.Lease > 0 && p.RequestTimeout > 0 && p.RequestTimeout < p.Lease && p.BaseBackoff > 0 && p.MaxBackoff >= p.BaseBackoff
}

type Service struct {
	Repository Repository
	Content    preparecase.Content
	Decoder    Decoder
	Sender     Sender
	Boundary   Boundary
	Clock      Clock
	Policy     Policy
	Jitter     func(time.Duration) (time.Duration, error)
}
type Result struct{ MessageID, State, Disposition string }

// Protected checks that an absent or non-invoking gate cannot authorize work.
func Protected(ctx context.Context, b Boundary, c Claim, work func(context.Context) error) error {
	if ctx == nil || Missing(b) || ctx.Err() != nil {
		return ErrHeld
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var called, active atomic.Bool
	active.Store(true)
	defer active.Store(false)
	var actionErr error
	err := b.Start(ctx, c, func(g context.Context) error {
		if !active.Load() || !called.CompareAndSwap(false, true) || g == nil || g.Err() != nil || ctx.Err() != nil {
			return ErrHeld
		}
		w, cancel := context.WithCancel(ctx)
		defer cancel()
		stop := context.AfterFunc(g, cancel)
		defer stop()
		actionErr = work(w)
		return actionErr
	})
	if actionErr != nil {
		return actionErr
	}
	if err != nil || !called.Load() || ctx.Err() != nil {
		return ErrHeld
	}
	return nil
}
func (s Service) ready() bool {
	return !Missing(s.Repository) && !Missing(s.Content) && !Missing(s.Decoder) && !Missing(s.Sender) && !Missing(s.Boundary) && !Missing(s.Clock) && s.Jitter != nil && s.Policy.Valid()
}

// RunScope indexes only a current authorized workspace. New tenants are not
// discovered by scanning RLS payload tables or by a technical superuser.
func (s Service) RunScope(ctx context.Context, r Request) ([]Result, error) {
	if !s.ready() || r.WorkspaceID == "" || r.Epoch < 1 {
		return nil, ErrHeld
	}
	if err := Protected(ctx, s.Boundary, Claim{Request: r}, func(c context.Context) error { return s.Repository.Index(c, r, s.Policy.Batch) }); err != nil {
		return nil, err
	}
	jobs, err := s.Repository.Jobs(ctx, r.WorkspaceID, s.Policy.Batch)
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, j := range jobs {
		if j.WorkspaceID != r.WorkspaceID {
			continue
		}
		v, e := s.SendOne(ctx, r, j.MessageID)
		if e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s Service) SendOne(ctx context.Context, r Request, id string) (Result, error) {
	zero := Result{}
	if !s.ready() {
		return zero, ErrHeld
	}
	var c Claim
	err := Protected(ctx, s.Boundary, Claim{Request: r}, func(x context.Context) error {
		var e error
		c, e = s.Repository.Claim(x, r, id, s.Policy.Lease)
		return e
	})
	if err != nil {
		return zero, err
	}
	finish := func(state, reason string, next time.Time) (Result, error) {
		e := Protected(ctx, s.Boundary, c, func(x context.Context) error { return s.Repository.Finish(x, c, state, next) })
		if e != nil {
			return zero, e
		}
		return Result{c.MessageID, state, reason}, nil
	}
	now := s.Clock.Now()
	if now.IsZero() || !now.Before(c.Ref.ExpiresAt) || !now.Before(c.DeadlineAt) || c.Attempts > s.Policy.MaxAttempts {
		return finish("HELD", "EXPIRED_OR_ATTEMPT_BOUND", now)
	}
	body, err := s.Content.Read(ctx, c.Ref, now)
	if err != nil {
		return finish("HELD", "CONTENT_UNAVAILABLE", now)
	}
	m, err := s.Decoder.Decode(body, c, now)
	if err != nil {
		return finish("HELD", "MESSAGE_INVALID", now)
	}
	end := now.Add(s.Policy.RequestTimeout)
	for _, t := range []time.Time{c.LeaseUntil, c.Ref.ExpiresAt, m.FactsUntil, m.Deadline} {
		if t.Before(end) {
			end = t
		}
	}
	networkCtx, cancel := context.WithDeadline(ctx, end)
	defer cancel()
	receipt, sendErr := s.Sender.Send(networkCtx, c, m, freshStart{s, c, m})
	now = s.Clock.Now()
	if sendErr == nil && receipt.MessageID == c.MessageID {
		switch receipt.Disposition {
		case "APPLIED", "DUPLICATE":
			return finish("ACKED", receipt.Disposition, now)
		case "HELD", "CONFLICT":
			return finish("HELD", receipt.Disposition, now)
		}
	}
	// Retrying an INTERNAL immutable message is at-least-once delivery, never a
	// decision to issue another ClickUp create. No new financial proof is minted.
	status := receipt.HTTPStatus
	if c.Attempts >= s.Policy.MaxAttempts || !now.Before(c.Ref.ExpiresAt) || !now.Before(m.FactsUntil) || !now.Before(m.Deadline) || status == 400 || status == 401 || status == 403 || status == 404 || status == 409 || status == 412 || status == 413 || (status >= 300 && status < 400) {
		return finish("HELD", "RETRY_UNSAFE_OR_EXHAUSTED", now)
	}
	delay := s.Policy.BaseBackoff
	for i := 1; i < c.Attempts && delay < s.Policy.MaxBackoff; i++ {
		if delay > s.Policy.MaxBackoff/2 {
			delay = s.Policy.MaxBackoff
			break
		}
		delay *= 2
	}
	if delay > s.Policy.MaxBackoff {
		delay = s.Policy.MaxBackoff
	}
	jitter, e := s.Jitter(delay)
	if e != nil || jitter < 0 || jitter > delay {
		return finish("HELD", "RETRY_CONFIGURATION", now)
	}
	// jitter is in [0, delay], keeping the deployment backoff ceiling bounded.
	next := now.Add(delay/2 + jitter/2)
	if receipt.RetryAfter.After(next) {
		next = receipt.RetryAfter
	}
	for _, t := range []time.Time{c.Ref.ExpiresAt, m.FactsUntil, m.Deadline} {
		if !next.Before(t) {
			return finish("HELD", "RETRY_EXCEEDS_ORIGINAL_EXPIRY", now)
		}
	}
	return finish("PENDING", "RETRY_SAME_MESSAGE", next)
}

type freshStart struct {
	s Service
	c Claim
	m Message
}

func (g freshStart) Start(ctx context.Context, c Claim, work func(context.Context) error) error {
	if c != g.c {
		return ErrHeld
	}
	return Protected(ctx, g.s.Boundary, c, func(x context.Context) error {
		now := g.s.Clock.Now()
		if now.IsZero() || !now.Before(g.m.FactsUntil) || !now.Before(g.m.Deadline) || !now.Before(c.Ref.ExpiresAt) || !now.Before(c.LeaseUntil) {
			return ErrHeld
		}
		// Check commits its short read-only SQL transaction before socket IO starts.
		if e := g.s.Repository.Check(x, c); e != nil {
			return e
		}
		return work(x)
	})
}

// Missing also rejects typed nil dependencies at composition boundaries.
func Missing(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}

// CryptoJitter supplies bounded deployment backoff entropy. It does not grant
// authority or extend source/episode/content expiry. Entropy failure holds work.
func CryptoJitter(ceiling time.Duration) (time.Duration, error) {
	if ceiling <= 0 {
		return 0, ErrHeld
	}
	n, e := rand.Int(rand.Reader, new(big.Int).Add(big.NewInt(int64(ceiling)), big.NewInt(1)))
	if e != nil {
		return 0, ErrHeld
	}
	return time.Duration(n.Int64()), nil
}
