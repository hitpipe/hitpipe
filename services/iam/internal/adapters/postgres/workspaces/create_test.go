package workspaces

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
)

type offsetClock struct{ observed time.Time }

func (clock offsetClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: clock.observed, Trusted: true}, nil
}

func TestSessionDeadlineUsesTrustedRemainingTime(t *testing.T) {
	trusted := time.Now().Add(-6 * time.Hour)
	store := Store{clock: offsetClock{observed: trusted}}
	guarded, cancel, err := store.sessionDeadline(context.Background(), app.Identity{ValidUntil: trusted.Add(2 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	deadline, ok := guarded.Deadline()
	if got := time.Until(deadline); !ok || got <= 0 || got > 2*time.Second {
		t.Fatal("session deadline did not follow remaining trusted lifetime")
	}
}

func TestCommitOutcomePreservesUncertainty(t *testing.T) {
	for _, outcome := range []struct {
		driverError error
		want        error
	}{
		{nil, nil},
		{pgx.ErrTxCommitRollback, app.ErrUnavailable},
		{context.DeadlineExceeded, app.ErrUncertain},
		{errors.New("disconnected after sending COMMIT"), app.ErrUncertain},
	} {
		if got := classifyCommit(outcome.driverError); !errors.Is(got, outcome.want) {
			t.Fatalf("unexpected commit outcome: %v", got)
		}
	}
}
