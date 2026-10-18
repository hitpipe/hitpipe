// Package authenticatesession orchestrates a session check through consumer-owned
// ports. It authenticates only a session snapshot; business grants are separate.
package authenticatesession

import (
	"context"
	"errors"
	"reflect"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

// SessionLookup returns the current minimal snapshot from its authority. An
// absent session is Found=false, not an infrastructure error. Implementations
// must honor cancellation and must not imply business authorization.
type SessionLookup interface {
	Lookup(context.Context, string) (sessionpolicy.Snapshot, error)
}

// Clock returns an observation whose trust is established by the adapter's
// environment. A readable host clock alone does not establish trust.
type Clock interface {
	Now(context.Context) (sessionpolicy.Observation, error)
}

// Result has bounded state/reason only. Adapter errors and references never
// escape this boundary. The zero result is never valid.
type Result struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// IsValid explicitly requires the complete positive result; it grants no action.
func (r Result) IsValid() bool { return r.State == "valid" && r.Reason == "session_valid" }

// Service owns no mutable state; injected dependencies remain caller-owned and
// must be safe for their intended concurrency/lifetime. New checks dependencies.
type Service struct {
	lookup SessionLookup
	clock  Clock
}

func absent(v any) bool {
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

// New rejects unconfigured and typed-nil dependencies without invoking them.
func New(lookup SessionLookup, clock Clock) (Service, error) {
	if absent(lookup) || absent(clock) {
		return Service{}, errors.New("dependencies_unavailable")
	}
	return Service{lookup: lookup, clock: clock}, nil
}

func validReference(ref string) bool {
	if len(ref) < 1 || len(ref) > 128 {
		return false
	}
	for i, c := range []byte(ref) {
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum && (i == 0 || c != '_' && c != '-') {
			return false
		}
	}
	return true
}

// Execute obtains the snapshot before reading the clock so lookup latency does
// not freeze an earlier now. Errors, cancellation and untrusted time fail closed.
// No store mutation, session renewal or protected business effect is performed.
func (s Service) Execute(ctx context.Context, reference string) Result {
	unavailable := func(reason string) Result { return Result{State: "unavailable", Reason: reason} }
	if absent(s.lookup) || absent(s.clock) {
		return unavailable("dependencies_unavailable")
	}
	if ctx == nil || !validReference(reference) {
		return Result{State: "denied", Reason: "invalid_request"}
	}
	if ctx.Err() != nil {
		return unavailable("request_cancelled")
	}
	snapshot, err := s.lookup.Lookup(ctx, reference)
	if ctx.Err() != nil {
		return unavailable("request_cancelled")
	}
	if err != nil {
		return unavailable("lookup_unavailable")
	}
	observation, err := s.clock.Now(ctx)
	if ctx.Err() != nil {
		return unavailable("request_cancelled")
	}
	if err != nil || !observation.Usable() {
		return unavailable("clock_unavailable")
	}
	decision := sessionpolicy.Evaluate(snapshot, observation)
	return Result{State: string(decision.State), Reason: string(decision.Reason)}
}
