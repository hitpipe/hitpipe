package dispatchintent

import (
	"context"
	"testing"
	"time"
)

type nonInvokingGate struct{}

func (nonInvokingGate) Start(context.Context, Claim, func(context.Context) error) error { return nil }

type nilGate struct{}

func (*nilGate) Start(context.Context, Claim, func(context.Context) error) error {
	panic("typed nil invoked")
}
func TestAbsentAndNonInvokingAuthorityNeverStart(t *testing.T) {
	var missing *nilGate
	for _, g := range []Boundary{nil, missing, nonInvokingGate{}} {
		called := false
		if e := Protected(context.Background(), g, Claim{}, func(context.Context) error { called = true; return nil }); e == nil || called {
			t.Fatal("absent current authority authorized start")
		}
	}
}
func TestBackoffEntropyBounds(t *testing.T) {
	for range 32 {
		d, e := CryptoJitter(time.Millisecond)
		if e != nil || d < 0 || d > time.Millisecond {
			t.Fatal(d, e)
		}
	}
	if _, e := CryptoJitter(0); e == nil {
		t.Fatal("missing jitter bound")
	}
}
