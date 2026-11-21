package runtime

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

const sampleInterval = 2 * time.Second
const sampleBudget = 350 * time.Millisecond

type diagnosticFault uint8

const (
	sqlUnavailable diagnosticFault = iota
	contentUnavailable
	jobRecoveryFailed
	jobClaimFailed
	leaseExpired
	leaseStale
	dispatchHeld
	outboxInterrupted
	recheckInterrupted
	timerInterrupted
	barrierDenied
	faultCount
)

var faultNames = [faultCount]string{
	sqlUnavailable:     "postgres_unavailable",
	contentUnavailable: "content_unavailable",
	jobRecoveryFailed:  "job_recovery_failed",
	jobClaimFailed:     "job_claim_failed",
	leaseExpired:       "lease_expired",
	leaseStale:         "lease_stale",
	dispatchHeld:       "dispatch_held",
	outboxInterrupted:  "outbox_interrupted",
	recheckInterrupted: "recheck_interrupted",
	timerInterrupted:   "timer_interrupted",
	barrierDenied:      "barrier_denied",
}

type diagnostics struct {
	logger             *slog.Logger
	mu                 sync.Mutex
	last               [faultCount]time.Time
	observationWindow  time.Time
	observationCount   int
	observationSkipped int
}

func (d *diagnostics) fault(code diagnosticFault, workspace, caseID, messageID string) {
	now := time.Now()
	d.mu.Lock()
	if now.Sub(d.last[code]) < sampleInterval {
		d.mu.Unlock()
		return
	}
	d.last[code] = now
	d.mu.Unlock()
	attrs := []slog.Attr{slog.String("reason", faultNames[code])}
	attrs = appendOpaque(attrs, "workspace_id", workspace)
	attrs = appendOpaque(attrs, "case_id", caseID)
	attrs = appendOpaque(attrs, "message_id", messageID)
	d.logger.LogAttrs(context.Background(), slog.LevelWarn, "processing diagnostic fault", attrs...)
}

func appendOpaque(attrs []slog.Attr, key, id string) []slog.Attr {
	if drafts.UUID(id) {
		return append(attrs, slog.String(key, id))
	}
	return attrs
}

func (d *diagnostics) observation(c postgres.CommittedObservation) {
	const maxPerInterval = 16
	var skipped int
	now := time.Now()
	d.mu.Lock()
	if now.Sub(d.observationWindow) >= sampleInterval {
		skipped = d.observationSkipped
		d.observationWindow = now
		d.observationCount, d.observationSkipped = 0, 0
	}
	if d.observationCount >= maxPerInterval {
		d.observationSkipped++
		d.mu.Unlock()
		return
	}
	d.observationCount++
	d.mu.Unlock()
	attrs := []slog.Attr{slog.Int64("observation_age_ms", c.Age.Milliseconds()),
		slog.String("disposition", c.Disposition), slog.String("case_state", c.State),
		slog.Int("skipped_previous_window", skipped)}
	attrs = appendOpaque(attrs, "workspace_id", c.WorkspaceID)
	attrs = appendOpaque(attrs, "case_id", c.CaseID)
	attrs = appendOpaque(attrs, "message_id", c.MessageID)
	attrs = appendOpaque(attrs, "effect_id", c.EffectID)
	d.logger.LogAttrs(context.Background(), slog.LevelInfo, "processing observation committed", attrs...)
}

func (rt *Runtime) watchDiagnostics(ctx context.Context, stop <-chan struct{}) {
	scopes := make([]string, 0, len(rt.a.stand))
	for workspace := range rt.a.stand {
		scopes = append(scopes, workspace)
	}
	slices.Sort(scopes)
	tick := time.NewTimer(sampleInterval)
	defer tick.Stop()
	var lastGate string
	for {
		gate := rt.sampleGate(ctx)
		if gate != lastGate {
			rt.logger.Info("processing barrier diagnostic", "barrier", gate)
			lastGate = gate
		}
		if gate == string(startgate.Open) {
			for _, workspace := range scopes {
				rt.sampleScope(ctx, workspace)
			}
		}
		tick.Reset(sampleInterval)
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (rt *Runtime) sampleGate(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, sampleBudget)
	defer cancel()
	state, err := rt.a.gate.State(ctx)
	if err != nil {
		return "UNAVAILABLE"
	}
	switch state {
	case startgate.Open, startgate.Closed, startgate.Syncing:
		return string(state)
	default:
		return "UNAVAILABLE"
	}
}

func (rt *Runtime) sampleScope(ctx context.Context, workspace string) {
	ctx, cancel := context.WithTimeout(withPermit(ctx), sampleBudget)
	defer cancel()
	// Reuse the configured owner scope and the existing worker role. There
	// are no global tenant scans, new permissions, locks or network actions
	// inside the SQL snapshot.
	p, err := rt.a.local(ctx, workspace, "ACTIVATE", nil)
	if err != nil {
		rt.diag.fault(barrierDenied, workspace, "", "")
		return
	}
	var sample postgres.QueueSample
	var sampleErr error
	err = rt.a.start(ctx, workspace, p.principal, p.epoch, func(c context.Context) error {
		sample, sampleErr = (postgres.DispatchStore{DB: rt.caseJobs.DB}).SampleQueues(c, workspace)
		return sampleErr
	})
	if err != nil {
		if sampleErr != nil {
			rt.diag.fault(sqlUnavailable, workspace, "", "")
		} else {
			rt.diag.fault(barrierDenied, workspace, "", "")
		}
		return
	}
	if sample.ExpiredJobLeases != 0 || sample.ExpiredOutboxLeases != 0 {
		rt.diag.fault(leaseExpired, workspace, "", sample.ExpiredOutboxID)
	}
	rt.diag.sample(workspace, sample)
}

func (d *diagnostics) sample(workspace string, s postgres.QueueSample) {
	const limit = postgres.DiagnosticSampleLimit
	attrs := []slog.Attr{
		slog.Int("sample_limit", limit), slog.Int64("interval_ms", sampleInterval.Milliseconds()),
		slog.Group("inbox", "rows", s.InboxRows, "cap_reached", s.InboxRows == limit, "held", s.InboxHeld,
			"max_age_ms", s.InboxAge),
		slog.Group("outbox", "rows", s.OutboxRows, "cap_reached", s.OutboxRows == limit, "pending", s.OutboxPending,
			"claimed", s.OutboxClaimed, "held", s.OutboxHeld, "acked", s.OutboxAcked, "expired_leases", s.ExpiredOutboxLeases,
			"max_age_ms", s.OutboxAge, "max_due_lag_ms", s.OutboxDueLag),
		slog.Group("jobs", "rows", s.JobRows, "cap_reached", s.JobRows == limit, "due", s.JobsDue,
			"expired_leases", s.ExpiredJobLeases, "max_due_lag_ms", s.JobDueLag),
		slog.Group("cases", "rows", s.CaseRows, "cap_reached", s.CaseRows == limit, "waiting_data", s.WaitingData,
			"attention", s.Attention, "unknown", s.Unknown),
	}
	attrs = appendOpaque(attrs, "workspace_id", workspace)
	attrs = appendOpaque(attrs, "expired_job_id", s.ExpiredJobID)
	attrs = appendOpaque(attrs, "expired_message_id", s.ExpiredOutboxID)
	d.logger.LogAttrs(context.Background(), slog.LevelInfo, "processing queue sample", attrs...)
}

func (d *diagnostics) dispatchFault(code diagnosticFault, workspace string, err error) {
	switch {
	case err == nil:
	case errors.Is(err, dispatchintent.ErrStale):
		d.fault(leaseStale, workspace, "", "")
	case errors.Is(err, dispatchintent.ErrHeld):
		d.fault(dispatchHeld, workspace, "", "")
	case errors.Is(err, drafts.ErrDenied):
		d.fault(barrierDenied, workspace, "", "")
	default:
		d.fault(code, workspace, "", "")
	}
}
