// Package receivesignal composes verified normalization, content and local receipt
// persistence. Success is a local receipt, never an HTTP ACK or Processing ACK.
package receivesignal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/authenticatewebhook"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/contentref"
)

var (
	ErrUnavailable    = errors.New("receipt unavailable")
	ErrAuthentication = normalizesignal.ErrAuthentication
	ErrInvalidEvent   = normalizesignal.ErrInvalidEvent
	ErrContext        = errors.New("connection unavailable")
	ErrConflict       = errors.New("receipt identity conflict")
	ErrIncomplete     = errors.New("receipt incomplete or expired")
	ErrQuota          = errors.New("receipt capacity exceeded")
	ErrUncertain      = errors.New("commit outcome unconfirmed")
)

type Clock interface {
	Now(context.Context) (time.Time, error)
}
type Connection struct {
	Binding       normalizesignal.Binding
	SecurityEpoch int64
}
type Delivery struct {
	Connection Connection
	RawBody    []byte
	Signature  string
}
type Stored struct {
	Signal  normalizesignal.Signal
	Content contentref.Ref
}
type Session interface {
	Lookup(context.Context, Connection, string) (*Stored, error)
	Insert(context.Context, Connection, Stored) error
	References(context.Context, Connection) (map[string]bool, error)
}
type Repository interface {
	WithSession(context.Context, func(Session) error) error
}
type Content interface {
	Put([]byte, time.Time) (contentref.Ref, error)
	Read(contentref.Ref) ([]byte, error)
	Purge(map[string]bool) (int, error)
}
type Consumer struct {
	workspace string
	normalize normalizesignal.Consumer
	clock     Clock
	retention time.Duration
	repo      Repository
	content   Content
}
type authClock struct{ Clock }

func (c authClock) NowUnix(ctx context.Context) (int64, error) {
	t, e := c.Now(ctx)
	if e != nil || !validNow(t) {
		return 0, ErrUnavailable
	}
	return t.Unix(), nil
}
func New(workspace string, secrets authenticatewebhook.SecretResolver, clock Clock, retention time.Duration, repo Repository, content Content) (*Consumer, error) {
	if workspace == "" || missing(clock) || missing(repo) || missing(content) || retention <= 0 || retention > 72*time.Hour || retention%time.Second != 0 {
		return nil, ErrContext
	}
	n, e := normalizesignal.New(secrets, authClock{clock})
	if e != nil {
		return nil, ErrUnavailable
	}
	return &Consumer{workspace: strings.ToLower(workspace), normalize: n, clock: clock, retention: retention, repo: repo, content: content}, nil
}
func validNow(t time.Time) bool { return !t.IsZero() && t.Year() >= 1970 && t.Year() < 9999 }
func newID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", ErrUnavailable
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func receipt(s normalizesignal.Signal) normalizesignal.Receipt {
	first, _ := time.Parse(time.RFC3339Nano, s.FirstReceivedAt)
	expires, _ := time.Parse(time.RFC3339Nano, s.ExpiresAt)
	return normalizesignal.Receipt{MessageID: s.MessageID, ReceiptID: s.ReceiptID, FirstReceivedAt: first, ExpiresAt: expires}
}
func wire(s *normalizesignal.Signal) ([]byte, error) {
	b, e := json.Marshal(s)
	if e != nil {
		return nil, ErrUnavailable
	}
	b, e = jcs.Transform(b)
	if e != nil {
		return nil, ErrUnavailable
	}
	return b, nil
}

// Receive authenticates before any lookup, including duplicates. One generated
// UUIDv4 serves signal_id, receipt_id and message_id; this makes their relation
// explicit using existing keys, without adding a table or hidden payload column.
func (c *Consumer) Receive(ctx context.Context, d Delivery) (*normalizesignal.Signal, error) {
	if c == nil || missing(c.clock) || missing(c.repo) || missing(c.content) || ctx == nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	if len(d.RawBody) == 0 || len(d.RawBody) > normalizesignal.MaxBodyBytes || len(d.Signature) > 2048 {
		return nil, ErrUnavailable
	}
	d.RawBody = append([]byte(nil), d.RawBody...)
	if d.Connection.Binding.Livemode != nil {
		mode := *d.Connection.Binding.Livemode
		d.Connection.Binding.Livemode = &mode
	}
	d.Connection.Binding.WorkspaceID = strings.ToLower(d.Connection.Binding.WorkspaceID)
	d.Connection.Binding.ConnectionID = strings.ToLower(d.Connection.Binding.ConnectionID)
	if d.Connection.Binding.WorkspaceID != c.workspace || d.Connection.SecurityEpoch <= 0 {
		return nil, ErrContext
	}
	now, e := c.clock.Now(ctx)
	if e != nil || !validNow(now) {
		return nil, ErrUnavailable
	}
	now = now.UTC().Truncate(time.Second)
	id, e := newID()
	if e != nil {
		return nil, e
	}
	input := normalizesignal.Input{RawBody: d.RawBody, Signature: d.Signature, Binding: d.Connection.Binding, Receipt: normalizesignal.Receipt{MessageID: id, ReceiptID: id, FirstReceivedAt: now.UTC(), ExpiresAt: now.Add(c.retention).UTC()}}
	candidate, e := c.normalize.Normalize(ctx, input)
	if e != nil {
		switch {
		case errors.Is(e, normalizesignal.ErrAuthentication), errors.Is(e, normalizesignal.ErrInvalidEvent):
			return nil, e
		case errors.Is(e, normalizesignal.ErrContext):
			return nil, ErrContext
		default:
			return nil, ErrUnavailable
		}
	}
	var result *normalizesignal.Signal
	e = c.repo.WithSession(ctx, func(session Session) error {
		previous, e := session.Lookup(ctx, d.Connection, candidate.EventID)
		if e != nil {
			return e
		}
		if previous != nil {
			input.Receipt = receipt(previous.Signal)
			expected, e := c.normalize.Normalize(ctx, input)
			if e != nil {
				return ErrIncomplete
			}
			expectedBytes, e := wire(expected)
			if e != nil {
				return e
			}
			// Both the DB identity and actual stored bytes must agree with the signed
			// event projected using ORIGINAL identity/first age, never caller retry time.
			previousBytes, e := wire(&previous.Signal)
			if e != nil {
				return e
			}
			if !bytes.Equal(expectedBytes, previousBytes) {
				return ErrConflict
			}
			actual, e := c.content.Read(previous.Content)
			if e != nil {
				return ErrIncomplete
			}
			if !bytes.Equal(actual, expectedBytes) {
				return ErrConflict
			}
			result = expected
			return nil
		}
		payload, e := wire(candidate)
		if e != nil {
			return e
		}
		ref, e := c.content.Put(payload, input.Receipt.ExpiresAt)
		if e != nil {
			return ErrUnavailable
		}
		readable, e := c.content.Read(ref)
		if e != nil || !bytes.Equal(readable, payload) {
			return ErrIncomplete
		}
		checked, e := c.clock.Now(ctx)
		if e != nil || !validNow(checked) || !input.Receipt.ExpiresAt.After(checked) {
			return ErrIncomplete
		}
		// Put has completed file/directory fsync. Insert's SQL transaction starts ONLY
		// now; no transaction or SQL row lock was held during file IO.
		if e = session.Insert(ctx, d.Connection, Stored{Signal: *candidate, Content: ref}); e != nil {
			return e
		}
		readable, e = c.content.Read(ref)
		if e != nil || !bytes.Equal(readable, payload) {
			return ErrIncomplete
		}
		result = candidate
		return nil
	})
	if e != nil {
		return nil, e
	}
	if result == nil {
		return nil, ErrUnavailable
	}
	if ctx.Err() != nil {
		return nil, ErrUncertain
	}
	checked, clockErr := c.clock.Now(ctx)
	if clockErr != nil || !validNow(checked) || !receipt(*result).ExpiresAt.After(checked) {
		return nil, ErrIncomplete
	}
	return result, nil
}

// Cleanup is explicit owner maintenance for this workspace-only content root.
// Same repository lease excludes Receive, including other method instances.
// All outbox references, including expired/held/acked ones, are protected.
func (c *Consumer) Cleanup(ctx context.Context, connection Connection) (int, error) {
	if c == nil || missing(c.repo) || missing(c.content) || ctx == nil || connection.Binding.WorkspaceID != c.workspace {
		return 0, ErrContext
	}
	n := 0
	e := c.repo.WithSession(ctx, func(s Session) error {
		refs, e := s.References(ctx, connection)
		if e != nil {
			return e
		}
		n, e = c.content.Purge(refs)
		if e != nil {
			return ErrUnavailable
		}
		return nil
	})
	return n, e
}

func missing(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
