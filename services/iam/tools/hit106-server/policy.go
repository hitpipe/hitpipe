//go:build hit106lab

package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

// The disposable browser lab explicitly uses the existing HIT-68 policy fixture durations.
const (
	labAbsoluteLifetime = 24 * time.Hour
	labIdleLifetime     = 8 * time.Hour
)

func checkPolicy(ctx context.Context) error {
	pool, err := openLabAppPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()

	rows, err := pool.Query(ctx, `SELECT created_at, expires_at, idle_expires_at, revoked_at IS NOT NULL
		FROM hp_iam.sessions`)
	if err != nil {
		return errors.New("read session deadlines from the local IAM PostgreSQL failed")
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		snapshot := sessionpolicy.Snapshot{Found: true}
		if rows.Scan(&snapshot.CreatedAt, &snapshot.ExpiresAt, &snapshot.IdleExpiresAt, &snapshot.Revoked) != nil {
			return errors.New("decode local IAM session deadlines failed")
		}
		if err := checkSnapshot(snapshot); err != nil {
			return err
		}
		count++
	}
	if rows.Err() != nil || count == 0 {
		return errors.New("expected session rows from the browser suite in the local IAM PostgreSQL")
	}
	fmt.Printf("HIT-106 session policy verified PostgreSQL rows=%d absolute=%s idle=%s\n",
		count, labAbsoluteLifetime, labIdleLifetime)
	fmt.Println("Synthetic trusted observations: idle_before=session_valid idle_at=idle_expired absolute_before=idle_expired absolute_at=absolute_expired")
	return nil
}

func checkSnapshot(snapshot sessionpolicy.Snapshot) error {
	if snapshot.ExpiresAt.Sub(snapshot.CreatedAt) != labAbsoluteLifetime ||
		snapshot.IdleExpiresAt.Sub(snapshot.CreatedAt) != labIdleLifetime {
		return errors.New("stored session durations differ from the explicit lab policy")
	}
	checks := []struct {
		name string
		at   time.Time
		want sessionpolicy.Reason
	}{
		{"idle_before", snapshot.IdleExpiresAt.Add(-time.Nanosecond), sessionpolicy.ReasonValid},
		{"idle_at", snapshot.IdleExpiresAt, sessionpolicy.ReasonIdleExpired},
		{"absolute_before", snapshot.ExpiresAt.Add(-time.Nanosecond), sessionpolicy.ReasonIdleExpired},
		{"absolute_at", snapshot.ExpiresAt, sessionpolicy.ReasonAbsoluteExpired},
	}
	for _, check := range checks {
		decision := sessionpolicy.Evaluate(snapshot, sessionpolicy.Observation{At: check.at, Trusted: true})
		if decision.Reason != check.want || decision.IsValid() != (check.want == sessionpolicy.ReasonValid) {
			return fmt.Errorf("unexpected session policy decision at %s", check.name)
		}
	}
	return nil
}
