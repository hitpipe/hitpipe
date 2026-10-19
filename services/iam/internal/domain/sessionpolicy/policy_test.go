package sessionpolicy_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

func baseline() sessionpolicy.Snapshot {
	created := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	return sessionpolicy.Snapshot{Found: true, CreatedAt: created,
		ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
}

func observe(at time.Time) sessionpolicy.Observation {
	return sessionpolicy.Observation{At: at, Trusted: true}
}

func TestEvaluateValid(t *testing.T) {
	s := baseline()
	got := sessionpolicy.Evaluate(s, observe(s.IdleExpiresAt.Add(-time.Nanosecond)))
	if got.State != sessionpolicy.StateValid || got.Reason != sessionpolicy.ReasonValid || !got.IsValid() {
		t.Fatalf("expected session_valid before both deadlines, got %+v", got)
	}
}

func TestEvaluateUnknown(t *testing.T) {
	s := baseline()
	s.Found = false
	got := sessionpolicy.Evaluate(s, observe(s.CreatedAt))
	if got.State != sessionpolicy.StateDenied || got.Reason != sessionpolicy.ReasonUnknown || got.IsValid() {
		t.Fatalf("unknown must be denied: %+v", got)
	}
}

func TestEvaluateIdleBoundary(t *testing.T) {
	for _, tc := range []struct {
		name  string
		delta time.Duration
		want  sessionpolicy.Reason
	}{
		{"before", -time.Nanosecond, sessionpolicy.ReasonValid},
		{"equal", 0, sessionpolicy.ReasonIdleExpired},
		{"after", time.Nanosecond, sessionpolicy.ReasonIdleExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := baseline()
			got := sessionpolicy.Evaluate(s, observe(s.IdleExpiresAt.Add(tc.delta)))
			if got.Reason != tc.want || got.IsValid() != (tc.want == sessionpolicy.ReasonValid) {
				t.Fatalf("delta %s: want %s, got %+v", tc.delta, tc.want, got)
			}
		})
	}
}

func TestEvaluateAbsoluteBoundary(t *testing.T) {
	s := baseline()
	s.IdleExpiresAt = s.ExpiresAt
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		got := sessionpolicy.Evaluate(s, observe(s.ExpiresAt.Add(delta)))
		want := sessionpolicy.ReasonAbsoluteExpired
		if delta < 0 {
			want = sessionpolicy.ReasonValid
		}
		if got.Reason != want || got.IsValid() != (delta < 0) {
			t.Fatalf("absolute delta %s: want %s, got %+v", delta, want, got)
		}
	}
}

func TestEvaluateRevoked(t *testing.T) {
	s := baseline()
	s.Revoked = true
	if got := sessionpolicy.Evaluate(s, observe(s.CreatedAt)); got.Reason != sessionpolicy.ReasonRevoked || got.IsValid() {
		t.Fatalf("revocation must override future expiry: %+v", got)
	}
}

func TestEvaluateInvalidChronology(t *testing.T) {
	mutations := map[string]func(*sessionpolicy.Snapshot){
		"zero creation":            func(s *sessionpolicy.Snapshot) { s.CreatedAt = time.Time{} },
		"zero absolute":            func(s *sessionpolicy.Snapshot) { s.ExpiresAt = time.Time{} },
		"zero idle":                func(s *sessionpolicy.Snapshot) { s.IdleExpiresAt = time.Time{} },
		"absolute equals creation": func(s *sessionpolicy.Snapshot) { s.ExpiresAt = s.CreatedAt },
		"idle equals creation":     func(s *sessionpolicy.Snapshot) { s.IdleExpiresAt = s.CreatedAt },
		"idle exceeds absolute":    func(s *sessionpolicy.Snapshot) { s.IdleExpiresAt = s.ExpiresAt.Add(time.Nanosecond) },
		"lifetime above cap":       func(s *sessionpolicy.Snapshot) { s.ExpiresAt = s.ExpiresAt.Add(time.Nanosecond) },
		"far future":               func(s *sessionpolicy.Snapshot) { s.ExpiresAt = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC) },
		"unsupported year":         func(s *sessionpolicy.Snapshot) { s.IdleExpiresAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
	}
	for name, change := range mutations {
		t.Run(name, func(t *testing.T) {
			s := baseline()
			change(&s)
			if got := sessionpolicy.Evaluate(s, observe(baseline().CreatedAt)); got.Reason != sessionpolicy.ReasonInvalidSnapshot || got.IsValid() {
				t.Fatalf("invalid snapshot must be denied: %+v", got)
			}
		})
	}
}

func TestEvaluateClockAndFutureCreation(t *testing.T) {
	s := baseline()
	for _, o := range []sessionpolicy.Observation{
		{At: s.CreatedAt, Trusted: false}, {Trusted: true},
		{At: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), Trusted: true},
	} {
		if got := sessionpolicy.Evaluate(s, o); got.Reason != sessionpolicy.ReasonClockUncertain || got.IsValid() {
			t.Fatalf("bad clock must be denied: %+v", got)
		}
	}
	if got := sessionpolicy.Evaluate(s, observe(s.CreatedAt.Add(-time.Nanosecond))); got.Reason != sessionpolicy.ReasonNotYetValid {
		t.Fatalf("future creation must be denied: %+v", got)
	}
}

func TestEvaluateDeterministicImmutable(t *testing.T) {
	s := baseline()
	original := s
	now := observe(s.CreatedAt)
	want := sessionpolicy.Decision{State: sessionpolicy.StateValid, Reason: sessionpolicy.ReasonValid}
	for range 100 {
		if got := sessionpolicy.Evaluate(s, now); got != want {
			t.Fatalf("unexpected decision: %+v", got)
		}
		if !reflect.DeepEqual(s, original) {
			t.Fatal("input snapshot changed")
		}
	}
	if (sessionpolicy.Decision{}).IsValid() {
		t.Fatal("zero decision must fail closed")
	}
}

func TestEvaluateRenewedIdleAndOffsets(t *testing.T) {
	s := baseline()
	s.IdleExpiresAt = s.CreatedAt.Add(16 * time.Hour)
	now := s.CreatedAt.Add(12 * time.Hour).In(time.FixedZone("fixture-offset", 3*60*60))
	if got := sessionpolicy.Evaluate(s, observe(now)); !got.IsValid() {
		t.Fatalf("authority-owned renewed deadline and equivalent offset must work: %+v", got)
	}
}

func ExampleEvaluate() {
	created := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	s := sessionpolicy.Snapshot{Found: true, CreatedAt: created,
		ExpiresAt: created.Add(24 * time.Hour), IdleExpiresAt: created.Add(8 * time.Hour)}
	d := sessionpolicy.Evaluate(s, sessionpolicy.Observation{At: created, Trusted: true})
	fmt.Println(d.State, d.Reason)
	// Output: valid session_valid
}
