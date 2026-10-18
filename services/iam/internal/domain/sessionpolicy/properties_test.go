package sessionpolicy_test

import (
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"testing"
	"time"
)

// Added after the actual M07 survivor: callers can construct a value whose
// positive state and reason disagree, even though Evaluate never emits it.
func TestDecisionRejectsMixedStateAndReason(t *testing.T) {
	for _, d := range []sessionpolicy.Decision{
		{State: sessionpolicy.StateValid, Reason: sessionpolicy.ReasonUnknown},
		{State: sessionpolicy.StateDenied, Reason: sessionpolicy.ReasonValid},
		{State: sessionpolicy.StateValid}, {Reason: sessionpolicy.ReasonValid}, {},
	} {
		if d.IsValid() {
			t.Fatalf("incomplete/mixed decision allowed: %+v", d)
		}
	}
}

func TestDecisionTablePrecedence(t *testing.T) {
	s := baseline()
	now := observe(s.ExpiresAt)
	s.Found = false
	s.Revoked = true
	s.CreatedAt = time.Time{}
	if d := sessionpolicy.Evaluate(s, sessionpolicy.Observation{}); d.Reason != sessionpolicy.ReasonClockUncertain {
		t.Fatal(d)
	}
	if d := sessionpolicy.Evaluate(s, now); d.Reason != sessionpolicy.ReasonUnknown {
		t.Fatal(d)
	}
	s.Found = true
	if d := sessionpolicy.Evaluate(s, now); d.Reason != sessionpolicy.ReasonRevoked {
		t.Fatal(d)
	}
	s.Revoked = false
	if d := sessionpolicy.Evaluate(s, now); d.Reason != sessionpolicy.ReasonInvalidSnapshot {
		t.Fatal(d)
	}
	s = baseline()
	if d := sessionpolicy.Evaluate(s, now); d.Reason != sessionpolicy.ReasonAbsoluteExpired {
		t.Fatal(d)
	}
}

// FuzzValidityInvariants checks logical implications independent of safe reason
// spelling. Seeds run in ordinary CI; bounded fuzzing is a separate actual run.
func FuzzValidityInvariants(f *testing.F) {
	for _, v := range []int64{-1, 0, 1, 8 * int64(time.Hour), 24 * int64(time.Hour)} {
		f.Add(v, true, false, true)
	}
	f.Fuzz(func(t *testing.T, delta int64, found, revoked, trusted bool) {
		s := baseline()
		s.Found = found
		s.Revoked = revoked
		o := sessionpolicy.Observation{At: s.CreatedAt.Add(time.Duration(delta)), Trusted: trusted}
		d := sessionpolicy.Evaluate(s, o)
		if d.IsValid() && (!found || revoked || !trusted || o.At.Before(s.CreatedAt) || !o.At.Before(s.IdleExpiresAt) || !o.At.Before(s.ExpiresAt)) {
			t.Fatal("invalid session allowed", d)
		}
		if d != sessionpolicy.Evaluate(s, o) {
			t.Fatal("nondeterministic")
		}
	})
}
