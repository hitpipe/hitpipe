package receivesignals

import (
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"testing"
	"time"
)

const ws = "11111111-1111-4111-8111-111111111111"
const connection = "22222222-2222-4222-8222-222222222222"
const operation = "33333333-3333-4333-8333-333333333333"
const message = "44444444-4444-4444-8444-444444444444"

type decoder struct{ calls int }

func (d *decoder) Decode(b []byte) (admission.Signal, error) {
	d.calls++
	if string(b) != "untouched original bytes" {
		return admission.Signal{}, admission.ErrInvalidSignal
	}
	return admission.Signal{MessageID: message, WorkspaceID: ws, ConnectionID: connection, AccountID: "acct"}, nil
}

type contexts struct{ grant Grant }

func (c *contexts) Establish(_ context.Context, q Identity) (Grant, error) {
	if q.WorkspaceID != ws || q.ConnectionID != connection || q.AccountID != "acct" {
		return Grant{}, admission.ErrNotAuthorized
	}
	return c.grant, nil
}

type gate struct {
	mode  string
	calls []string
}

func (g *gate) Start(c context.Context, _ Grant, p string, f func(context.Context) error) error {
	g.calls = append(g.calls, p)
	switch g.mode {
	case "deny":
		return admission.ErrNotAuthorized
	case "no_call":
		return nil
	case "nil_context":
		return f(nil)
	case "cancel":
		cc, cancel := context.WithCancel(c)
		cancel()
		return f(cc)
	}
	return f(c)
}

type clock struct{ at time.Time }

func (c clock) Now() time.Time { return c.at }

type store struct {
	calls int
	err   error
}

func (s *store) Accept(c context.Context, v admission.Signal, a admission.EstablishedContext) (admission.Receipt, error) {
	s.calls++
	if c.Err() != nil || !a.AuthorityVerified || v.MessageID != message {
		return admission.Receipt{}, admission.ErrNotAuthorized
	}
	if s.err != nil {
		return admission.Receipt{}, s.err
	}
	return admission.Receipt{MessageID: message, Disposition: "APPLIED", RecordID: operation}, nil
}
func fixture() (Service, *decoder, *contexts, *gate, *store) {
	now := time.Now()
	d := &decoder{}
	c := &contexts{Grant{Context: admission.EstablishedContext{WorkspaceID: ws, ConnectionID: connection, AccountID: "acct", OperationID: operation, ExpectedRevision: 1, AuthorityVerified: true}, PrincipalID: ws, Epoch: 7, ExpiresAt: now.Add(time.Hour), Allowed: true}}
	g := &gate{}
	r := &store{}
	return Service{c, g, clock{now}, d, r}, d, c, g, r
}
func TestOneRawDecodeCommitThenProtectedReceipt(t *testing.T) {
	s, d, _, g, r := fixture()
	emitted := 0
	if e := s.Execute(context.Background(), []byte("untouched original bytes"), func(a admission.Receipt) error {
		emitted++
		if r.calls != 1 || len(g.calls) != 2 || g.calls[1] != "INTAKE_RECEIPT_RESPONSE_START" || a.MessageID != message {
			t.Fatal("receipt before commit/current gate")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if d.calls != 1 || emitted != 1 {
		t.Fatal("redecoded or duplicate emission")
	}
}
func TestNoAcknowledgementOnUnknownAuthorityAndFailedGates(t *testing.T) {
	for _, mode := range []string{"deny", "no_call", "nil_context", "cancel", "expired", "unverified", "pending", "wrong_binding", "zero_epoch", "commit_error"} {
		t.Run(mode, func(t *testing.T) {
			s, _, c, g, r := fixture()
			switch mode {
			case "expired":
				c.grant.ExpiresAt = time.Time{}
			case "unverified":
				c.grant.Context.AuthorityVerified = false
			case "pending":
				c.grant.RevocationPending = true
			case "wrong_binding":
				c.grant.Context.ConnectionID = operation
			case "zero_epoch":
				c.grant.Epoch = 0
			case "commit_error":
				r.err = errors.New("actual store failed")
			default:
				g.mode = mode
			}
			e := s.Execute(context.Background(), []byte("untouched original bytes"), func(admission.Receipt) error { t.Fatal("failed operation emitted receipt"); return nil })
			if e == nil {
				t.Fatal("accepted unknown boundary")
			}
		})
	}
}
func TestMissingAndTypedNilPortsStayClosed(t *testing.T) {
	for _, mode := range []string{"contexts", "boundary", "clock", "decoder", "store"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, _, _ := fixture()
			switch mode {
			case "contexts":
				var x *contexts
				s.Contexts = x
			case "boundary":
				var x *gate
				s.Boundary = x
			case "clock":
				s.Clock = nil
			case "decoder":
				var x *decoder
				s.Decoder = x
			case "store":
				var x *store
				s.Store = x
			}
			if s.Ready() || s.Execute(context.Background(), nil, func(admission.Receipt) error { t.Fatal("missing port emitted"); return nil }) == nil {
				t.Fatal("permissive fallback")
			}
		})
	}
}
