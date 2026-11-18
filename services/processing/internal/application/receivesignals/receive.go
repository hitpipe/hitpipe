// Package receivesignals composes trusted Intake admission ports. It grants no
// authority from a message, TLS certificate, browser header or local default.
package receivesignals

import (
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"reflect"
	"regexp"
	"sync/atomic"
	"time"
)

var ErrUnavailable = errors.New("SIGNAL_UNAVAILABLE")
var id = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func UUID(s string) bool { return id.MatchString(s) }

type Identity struct {
	WorkspaceID, ConnectionID, AccountID string
	Live                                 bool
}
type Grant struct {
	Context                    admission.EstablishedContext
	PrincipalID                string
	Epoch                      int64
	ExpiresAt                  time.Time
	Allowed, RevocationPending bool
}

// ContextSource must resolve hints using trusted operation/installation and
// current authority data. Any protected lookup in that implementation must use
// the same current-epoch/actual-start gate as admission. No port is optional.
// The controlled test source is not a runtime IAM client.
type ContextSource interface {
	Establish(context.Context, Identity) (Grant, error)
}

// Boundary is the runtime's shared synchronous current-epoch/actual-start gate.
// It must cover the entire SQL/content operation and separately response start.
type Boundary interface {
	Start(context.Context, Grant, string, func(context.Context) error) error
}
type Clock interface{ Now() time.Time }
type Service struct {
	Contexts ContextSource
	Boundary Boundary
	Clock    Clock
	Decoder  admission.Decoder
	Store    admission.Store
}

func missing(x any) bool {
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
func (s Service) Ready() bool {
	return !missing(s.Contexts) && !missing(s.Boundary) && !missing(s.Clock) && !missing(s.Decoder) && !missing(s.Store)
}

type protectedStore struct {
	run func(context.Context, admission.Signal, admission.EstablishedContext) (admission.Receipt, error)
}

func (p protectedStore) Accept(c context.Context, v admission.Signal, e admission.EstablishedContext) (admission.Receipt, error) {
	return p.run(c, v, e)
}
func (s Service) Execute(ctx context.Context, raw []byte, emit func(admission.Receipt) error) error {
	if ctx == nil || ctx.Err() != nil || !s.Ready() || emit == nil {
		return ErrUnavailable
	}
	var grant Grant
	valid := func() bool {
		c := grant.Context
		now := s.Clock.Now()
		return grant.Allowed && !grant.RevocationPending && UUID(grant.PrincipalID) && UUID(c.WorkspaceID) && UUID(c.ConnectionID) && UUID(c.OperationID) && c.AccountID != "" && c.AuthorityVerified && c.ExpectedRevision > 0 && c.ExpectedRevision <= 9007199254740991 && grant.Epoch > 0 && grant.Epoch <= 9007199254740991 && !now.IsZero() && grant.ExpiresAt.After(now)
	}
	protect := func(phase string, work func(context.Context) error) error {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		var called, active atomic.Bool
		active.Store(true)
		defer active.Store(false)
		var actionErr error
		e := s.Boundary.Start(bounded, grant, phase, func(c context.Context) error {
			if !active.Load() || !called.CompareAndSwap(false, true) || c == nil || c.Err() != nil || bounded.Err() != nil || !valid() {
				return admission.ErrNotAuthorized
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
			return admission.ErrNotAuthorized
		}
		return nil
	}
	store := protectedStore{func(c context.Context, v admission.Signal, a admission.EstablishedContext) (receipt admission.Receipt, e error) {
		e = protect("INTAKE_ADMISSION_SQL_CONTENT_COMMIT", func(w context.Context) error { var e error; receipt, e = s.Store.Accept(w, v, a); return e })
		return
	}}
	receipt, e := admission.AdmitWithContext(ctx, raw, s.Decoder, store, func(c context.Context, v admission.Signal) (admission.EstablishedContext, error) {
		bounded, cancel := context.WithTimeout(c, 2*time.Second)
		defer cancel()
		var e error
		grant, e = s.Contexts.Establish(bounded, Identity{v.WorkspaceID, v.ConnectionID, v.AccountID, v.Live})
		if e != nil {
			return admission.EstablishedContext{}, e
		}
		if bounded.Err() != nil || !valid() {
			return admission.EstablishedContext{}, admission.ErrNotAuthorized
		}
		return grant.Context, nil
	})
	if e != nil {
		return e
	}
	return protect("INTAKE_RECEIPT_RESPONSE_START", func(c context.Context) error {
		if c.Err() != nil {
			return c.Err()
		}
		return emit(receipt)
	})
}
