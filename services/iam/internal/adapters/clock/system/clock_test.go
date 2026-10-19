package system_test

import (
	"context"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/clock/system"
	"testing"
	"time"
)

func TestSystemClockRequiresAttestation(t *testing.T) {
	var nilClock *system.Clock
	for _, clock := range []*system.Clock{nilClock, system.New(nil), system.New(func(context.Context, time.Time) bool { return false })} {
		if o, e := clock.Now(context.Background()); e == nil || o.Trusted {
			t.Fatal("clock trusted without attestation")
		}
	}
	// A synthetic probe is an adapter unit test, not evidence of host health.
	clock := system.New(func(_ context.Context, at time.Time) bool { return !at.IsZero() })
	if o, e := clock.Now(context.Background()); e != nil || !o.Usable() {
		t.Fatal("synthetic attestation not used")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if o, e := clock.Now(ctx); e == nil || o.Trusted {
		t.Fatal("cancelled time trusted")
	}
	if o, e := clock.Now(nil); e == nil || o.Trusted {
		t.Fatal("nil context trusted")
	}
	ctx, cancel = context.WithCancel(context.Background())
	clock = system.New(func(context.Context, time.Time) bool { cancel(); return true })
	if o, e := clock.Now(ctx); e == nil || o.Trusted {
		t.Fatal("probe cancellation ignored")
	}
}
