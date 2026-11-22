package issuesession

import (
	"context"
	"errors"
	"testing"
	"time"
)

type startFunc func(context.Context, string, int64, func(context.Context) error) error

func (f startFunc) Within(ctx context.Context, id string, epoch int64, use func(context.Context) error) error {
	return f(ctx, id, epoch, use)
}

func TestAdmittedIssuanceBindsCommitAndResponseToOneExactStart(t *testing.T) {
	f := newFixture(t)
	var within, published bool
	var sessionID string
	admission := startFunc(func(ctx context.Context, id string, epoch int64, use func(context.Context) error) error {
		if !validUUID(id) || epoch != f.decision.Epoch || f.storeCalls != 0 {
			t.Fatal("the gate received an incorrect session or received it after SQL")
		}
		sessionID = id
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		within = true
		defer func() { within = false }()
		return use(ctx)
	})
	f.afterStore = func() {
		if !within || f.record.SessionID != sessionID {
			t.Fatal("SQL ran outside the admitted session start")
		}
	}
	err := f.service.ExecuteAtStart(context.Background(), "opaque-proof", admission, func(_ context.Context, result Result) error {
		if !within || f.storeCalls != 1 || f.guardCalls != 3 || !result.IsIssued() || result.SessionID != sessionID {
			t.Fatal("result started outside the held, committed and current admission")
		}
		published = true
		return nil
	})
	if err != nil || !published {
		t.Fatal("admitted issuance failed")
	}
}

func TestAdmittedIssuanceRejectsAuthorityChangedWhileWaiting(t *testing.T) {
	f := newFixture(t)
	admission := startFunc(func(ctx context.Context, _ string, _ int64, use func(context.Context) error) error {
		f.decision.Epoch++
		return use(ctx)
	})
	err := f.service.ExecuteAtStart(context.Background(), "opaque-proof", admission, func(context.Context, Result) error {
		t.Fatal("a stale decision reached response start")
		return nil
	})
	if !errors.Is(err, ErrIssuanceDenied) || f.storeCalls != 0 {
		t.Fatal("a stale decision reached SQL or was not rejected")
	}
}
