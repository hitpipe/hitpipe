package authenticatesession_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

type lookupFunc func(context.Context, string) (sessionpolicy.Snapshot, error)

func (f lookupFunc) Lookup(c context.Context, r string) (sessionpolicy.Snapshot, error) {
	return f(c, r)
}

type clockFunc func(context.Context) (sessionpolicy.Observation, error)

func (f clockFunc) Now(c context.Context) (sessionpolicy.Observation, error) { return f(c) }

type nilLookup struct{}

func (*nilLookup) Lookup(context.Context, string) (sessionpolicy.Snapshot, error) {
	panic("typed nil called")
}

type nilClock struct{}

func (*nilClock) Now(context.Context) (sessionpolicy.Observation, error) { panic("typed nil called") }

func input() (sessionpolicy.Snapshot, sessionpolicy.Observation) {
	t := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	return sessionpolicy.Snapshot{Found: true, CreatedAt: t, ExpiresAt: t.Add(24 * time.Hour), IdleExpiresAt: t.Add(8 * time.Hour)}, sessionpolicy.Observation{At: t.Add(7 * time.Hour), Trusted: true}
}

func TestDependenciesAndInvalidRequests(t *testing.T) {
	snap, obs := input()
	lookups, clocks := 0, 0
	l := lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) { lookups++; return snap, nil })
	c := clockFunc(func(context.Context) (sessionpolicy.Observation, error) { clocks++; return obs, nil })
	var typedLookup *nilLookup
	var typedClock *nilClock
	var typedFunc lookupFunc
	for _, pair := range []struct {
		l authenticatesession.SessionLookup
		c authenticatesession.Clock
	}{{nil, c}, {l, nil}, {typedLookup, c}, {l, typedClock}, {typedFunc, c}} {
		if _, err := authenticatesession.New(pair.l, pair.c); err == nil {
			t.Fatal("nil dependency accepted")
		}
	}
	var zero authenticatesession.Service
	if r := zero.Execute(context.Background(), "a"); r.IsValid() || r.Reason != "dependencies_unavailable" {
		t.Fatal(r)
	}
	s, err := authenticatesession.New(l, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"", "_a", "-a", "a/b", "a.b", "a b", "я", strings.Repeat("a", 129)} {
		if r := s.Execute(context.Background(), ref); r.Reason != "invalid_request" || r.IsValid() {
			t.Fatal(ref, r)
		}
	}
	if r := s.Execute(nil, "a"); r.Reason != "invalid_request" {
		t.Fatal(r)
	}
	if lookups != 0 || clocks != 0 {
		t.Fatal("invalid request invoked dependencies")
	}
	for _, ref := range []string{"a", "0_A-z", strings.Repeat("a", 128)} {
		if r := s.Execute(context.Background(), ref); !r.IsValid() {
			t.Fatal(r)
		}
	}
	if (authenticatesession.Result{}).IsValid() || (authenticatesession.Result{State: "valid", Reason: "clock_unavailable"}).IsValid() {
		t.Fatal("default/mixed result allowed")
	}
}

func TestPortErrorsDoNotAllowOrLeak(t *testing.T) {
	snap, obs := input()
	for _, kind := range []string{"lookup", "clock", "untrusted", "zero clock"} {
		t.Run(kind, func(t *testing.T) {
			clocks := 0
			l := lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) {
				if kind == "lookup" {
					return snap, errors.New("SECRET_LOOKUP_PATH")
				}
				return snap, nil
			})
			c := clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
				clocks++
				if kind == "clock" {
					return obs, errors.New("SECRET_CLOCK")
				}
				if kind == "untrusted" {
					obs.Trusted = false
				}
				if kind == "zero clock" {
					obs.At = time.Time{}
				}
				return obs, nil
			})
			s, _ := authenticatesession.New(l, c)
			r := s.Execute(context.Background(), "internal")
			want := "clock_unavailable"
			if kind == "lookup" {
				want = "lookup_unavailable"
				if clocks != 0 {
					t.Fatal("clock called after lookup error")
				}
			}
			if r.State != "unavailable" || r.Reason != want || r.IsValid() || strings.Contains(r.Reason, "SECRET") {
				t.Fatal(r)
			}
		})
	}
}

func TestClockReadAfterLookupAndCancellation(t *testing.T) {
	snap, obs := input()
	for _, phase := range []string{"none", "before", "lookup", "clock"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			order := ""
			l := lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) {
				order += "L"
				if phase == "lookup" {
					cancel()
				}
				return snap, nil
			})
			c := clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
				order += "C"
				if order != "LC" {
					t.Fatal("clock read before lookup")
				}
				if phase == "clock" {
					cancel()
				}
				return obs, nil
			})
			s, _ := authenticatesession.New(l, c)
			if phase == "before" {
				cancel()
			}
			r := s.Execute(ctx, "a")
			if phase == "none" {
				if !r.IsValid() || order != "LC" {
					t.Fatal(r, order)
				}
			} else if r.Reason != "request_cancelled" || r.IsValid() {
				t.Fatal(r)
			}
			if phase == "before" && order != "" || phase == "lookup" && order != "L" {
				t.Fatal("cancelled dependency invoked", order)
			}
		})
	}
	// A delayed lookup crosses the idle boundary. A cached earlier now would allow.
	now := obs.At
	l := lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) {
		now = snap.IdleExpiresAt
		return snap, nil
	})
	c := clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
		return sessionpolicy.Observation{At: now, Trusted: true}, nil
	})
	s, _ := authenticatesession.New(l, c)
	if r := s.Execute(context.Background(), "a"); r.IsValid() || r.Reason != "idle_expired" {
		t.Fatal("stale observation", r)
	}
}
