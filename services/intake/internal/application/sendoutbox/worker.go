// Package sendoutbox delivers previously persisted Intake signals.
package sendoutbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/contentref"
)

var (
	ErrUnavailable = errors.New("outbox unavailable")
	ErrDenied      = errors.New("outbox authority denied")
	ErrStale       = errors.New("outbox attempt no longer current")
)

type Scope struct {
	Workspace, Connection, Account string
	Livemode                       bool
	Epoch                          int64
}

type Attempt struct {
	Signal  normalizesignal.Signal
	Content contentref.Ref
	Token   string
	Until   time.Time
	Number  int
}

type Outcome string

const (
	Acked   Outcome = "ACKED"
	Held    Outcome = "HELD"
	Pending Outcome = "PENDING"
)

type Repository interface {
	Claim(context.Context, Scope, time.Duration, time.Time) (Attempt, bool, error)
	Live(context.Context, Scope, Attempt) error
	Finish(context.Context, Scope, Attempt, Outcome, time.Time) error
}

// Gate validates current tenant, connection and epoch against owner authority.
// It must invoke the callback once, synchronously, under the actual-start
// barrier. Unknown or revoked scope must fail closed. Callbacks are bounded.
type Gate interface {
	Start(context.Context, Scope, func(context.Context) error) error
}

type Content interface {
	Read(contentref.Ref) ([]byte, error)
}

type Result struct {
	Disposition string
	Retry       bool
	RetryAfter  time.Time
}

type Transport interface {
	Send(ctx context.Context, requestID, messageID string, raw []byte, start func(context.Context, func(context.Context) error) error) (Result, error)
}

type Policy struct {
	Batch, Attempts                         int
	Lease, Timeout, BaseBackoff, MaxBackoff time.Duration
}

type Worker struct {
	workspace string
	repo      Repository
	gate      Gate
	content   Content
	transport Transport
	policy    Policy
}

func New(workspace string, repo Repository, gate Gate, content Content, transport Transport, policy Policy) (*Worker, error) {
	if !ValidUUID(workspace) || missing(repo) || missing(gate) || missing(content) || missing(transport) ||
		policy.Batch <= 0 || policy.Batch > 1000 || policy.Attempts <= 0 || policy.Attempts >= 1<<31-1 ||
		policy.Timeout <= 0 || policy.Timeout > 2*time.Second || policy.Lease < time.Millisecond ||
		policy.Lease <= policy.Timeout || policy.Lease > 72*time.Hour ||
		policy.BaseBackoff < time.Millisecond || policy.MaxBackoff < policy.BaseBackoff || policy.MaxBackoff > 72*time.Hour {
		return nil, ErrUnavailable
	}
	return &Worker{workspace, repo, gate, content, transport, policy}, nil
}

type Summary struct{ Acked, Held, Retried int }

// Run is one bounded batch for an explicitly chosen owner scope. Claims start
// individually so a queued item never consumes another item's lease.
func (w *Worker) Run(ctx context.Context, s Scope) (Summary, error) {
	var result Summary
	if !ValidScope(s) || s.Workspace != w.workspace {
		return result, ErrDenied
	}
	cutoff := time.Now()
	for range w.policy.Batch {
		var a Attempt
		var claimed bool
		if err := w.guard(ctx, s, func(c context.Context) error {
			var err error
			a, claimed, err = w.repo.Claim(c, s, w.policy.Lease, cutoff)
			return err
		}); err != nil {
			return result, err
		}
		if !claimed {
			return result, nil
		}
		state, due := w.deliver(ctx, s, a)
		if ctx.Err() != nil {
			return result, ErrUnavailable
		}
		finish, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := w.repo.Finish(finish, s, a, state, due)
		cancel()
		if err != nil {
			return result, err
		}
		switch state {
		case Acked:
			result.Acked++
		case Held:
			result.Held++
		case Pending:
			result.Retried++
		}
	}
	return result, nil
}

func (w *Worker) guard(ctx context.Context, s Scope, action func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var called, repeated bool
	var result error
	err := w.gate.Start(bounded, s, func(context.Context) error {
		if called {
			repeated = true
			return ErrDenied
		}
		if bounded.Err() != nil {
			return ErrDenied
		}
		called = true
		result = action(bounded)
		return result
	})
	if repeated || !called {
		return ErrDenied
	}
	if result != nil {
		return result
	}
	if err != nil {
		return ErrDenied
	}
	return nil
}

func (w *Worker) deliver(ctx context.Context, s Scope, a Attempt) (Outcome, time.Time) {
	now := time.Now()
	if a.Number > w.policy.Attempts || !a.Content.ExpiresAt.After(now) {
		return Held, time.Time{}
	}
	deadline := now.Add(w.policy.Timeout)
	if a.Until.Before(deadline) {
		deadline = a.Until
	}
	if a.Content.ExpiresAt.Before(deadline) {
		deadline = a.Content.ExpiresAt
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var raw []byte
	err := w.guard(ctx, s, func(c context.Context) error {
		if e := w.repo.Live(c, s, a); e != nil {
			return e
		}
		b, e := w.content.Read(a.Content)
		if e != nil || !validWire(b, s, a) {
			return ErrDenied
		}
		raw = b
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrUnavailable) {
			return w.retry(a, time.Time{})
		}
		return Held, time.Time{}
	}
	start := func(c context.Context, action func(context.Context) error) error {
		return w.guard(c, s, func(guarded context.Context) error {
			if err := w.repo.Live(guarded, s, a); err != nil {
				return err
			}
			if guarded.Err() != nil || !time.Now().Before(a.Content.ExpiresAt) {
				return ErrStale
			}
			return action(guarded)
		})
	}
	res, err := w.transport.Send(ctx, a.Token, a.Signal.MessageID, raw, start)
	switch {
	case err == nil && (res.Disposition == "APPLIED" || res.Disposition == "DUPLICATE" || res.Disposition == "HELD"):
		return Acked, time.Time{}
	case errors.Is(err, ErrDenied), errors.Is(err, ErrStale), !res.Retry, a.Number >= w.policy.Attempts:
		return Held, time.Time{}
	}
	return w.retry(a, res.RetryAfter)
}

func (w *Worker) retry(a Attempt, earliest time.Time) (Outcome, time.Time) {
	if a.Number >= w.policy.Attempts {
		return Held, time.Time{}
	}
	due := time.Now().Add(backoff(w.policy, a.Number))
	if earliest.After(due) {
		due = earliest
	}
	if !due.Before(a.Content.ExpiresAt) {
		return Held, time.Time{}
	}
	return Pending, due
}

func backoff(p Policy, number int) time.Duration {
	delay := p.BaseBackoff
	for n := 1; n < number && delay < p.MaxBackoff; n++ {
		if delay > p.MaxBackoff/2 {
			delay = p.MaxBackoff
			break
		}
		delay *= 2
	}
	// Equal jitter avoids immediate retries and never exceeds the maximum.
	half := delay / 2
	jitter, err := rand.Int(rand.Reader, big.NewInt(int64(delay-half)+1))
	if err != nil {
		return delay
	}
	return half + time.Duration(jitter.Int64())
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidUUID(v string) bool { return uuid.MatchString(v) }
func ValidScope(s Scope) bool {
	return ValidUUID(s.Workspace) && ValidUUID(s.Connection) && opaque(s.Account) && s.Epoch > 0
}
func opaque(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= 255
}

func validWire(raw []byte, s Scope, a Attempt) bool {
	if len(raw) != a.Content.Bytes || len(raw) == 0 || len(raw) > 65536 || !utf8.Valid(raw) ||
		!ValidUUID(a.Signal.MessageID) || !ValidUUID(a.Signal.ReceiptID) || !ValidUUID(a.Content.ID) ||
		a.Signal.WorkspaceID != s.Workspace || a.Signal.ConnectionID != s.Connection ||
		a.Signal.StripeAccountID != s.Account || a.Signal.Livemode != s.Livemode ||
		a.Signal.SchemaVersion != "1.0" || !opaque(a.Signal.EventID) || !opaque(a.Signal.SessionID) {
		return false
	}
	switch a.Signal.EventType {
	case "checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed":
	default:
		return false
	}
	first, err1 := time.Parse(time.RFC3339Nano, a.Signal.FirstReceivedAt)
	expires, err2 := time.Parse(time.RFC3339Nano, a.Signal.ExpiresAt)
	_, err3 := time.Parse(time.RFC3339Nano, a.Signal.SourceCreatedAt)
	if err1 != nil || err2 != nil || err3 != nil || !expires.Equal(a.Content.ExpiresAt) ||
		!expires.After(first) || expires.Sub(first) > 72*time.Hour ||
		"sha256:"+sha(raw) != a.Content.Digest {
		return false
	}
	j, err := jcs.Transform(raw)
	if err != nil || !bytes.Equal(j, raw) {
		return false
	}
	// The expected signal comes from the scoped SQL projection. Equality with
	// its complete canonical encoding rejects missing, null and extra fields.
	b, err := json.Marshal(a.Signal)
	if err != nil {
		return false
	}
	j, err = jcs.Transform(b)
	if err != nil || !bytes.Equal(j, raw) {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(j, &fields) != nil {
		return false
	}
	delete(fields, "message_digest")
	b, err = json.Marshal(fields)
	if err != nil {
		return false
	}
	j, err = jcs.Transform(b)
	return err == nil && a.Signal.MessageDigest == "sha256:"+sha(j)
}
func sha(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
func missing(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Chan, reflect.Map, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}
