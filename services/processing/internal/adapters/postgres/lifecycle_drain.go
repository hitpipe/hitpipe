package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

// Drain settles the one draining SINGLE operation in an explicit workspace.
// The trusted composition repeatedly calls it for its currently serving
// workspaces, including after a restart. Clocks never move the saved deadline.
// New admissions already acquire this operation row lock and require ACTIVE.
func (l LifecycleStore) Drain(ctx context.Context, workspace string) error {
	if !drafts.UUID(workspace) {
		return drafts.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := l.Drafts.tx(ctx, workspace)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var operation string
	var revision int64
	var pauseAt, deadline, now time.Time
	err = tx.QueryRow(ctx, `SELECT operation_id::text,revision,pause_accepted_at,drain_deadline_at,clock_timestamp()
 FROM hp_processing.operations WHERE workspace_id=$1 AND state='DRAINING' FOR UPDATE`, workspace).Scan(&operation, &revision, &pauseAt, &deadline, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	now = now.UTC().Truncate(time.Microsecond)
	if t := l.Drafts.Clock.Now().UTC().Truncate(time.Microsecond); t.After(now) {
		now = t
	}
	if !deadline.Equal(pauseAt.Add(lifecycle.DrainLimit)) || now.Before(pauseAt) || revision >= 9007199254740991 {
		return drafts.ErrUnavailable
	}
	// A confirmed result, exclusion, or explicit unresolved status is already
	// readable. Close its previous episode, without changing known observations.
	if _, err = tx.Exec(ctx, `UPDATE hp_processing.episodes e SET completed_at=$4
 FROM hp_processing.case_identity k WHERE (k.workspace_id,k.case_id)=(e.workspace_id,e.case_id)
 AND k.workspace_id=$1 AND k.operation_id=$2 AND k.admitted_at<=$3 AND e.completed_at IS NULL
 AND k.state IN ('CONFIRMED','EXCLUDED','UNKNOWN','ATTENTION','CLOSED_WITHOUT_CONFIRMATION')`,
		workspace, operation, pauseAt, now); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT k.case_id::text,k.state,k.revision,k.outcome_knowledge,k.ever_dispatch_fenced,
 (SELECT e.episode_id::text FROM hp_processing.episodes e WHERE (e.workspace_id,e.case_id)=(k.workspace_id,k.case_id)
 AND e.completed_at IS NULL ORDER BY e.started_at,e.episode_id LIMIT 1)
 FROM hp_processing.case_identity k WHERE k.workspace_id=$1 AND k.operation_id=$2 AND k.admitted_at<=$3
 AND k.state IN ('RECEIVED','WAITING_DATA','WORKING') AND ($4::timestamptz>=$5::timestamptz OR EXISTS(
 SELECT 1 FROM hp_processing.episodes e WHERE (e.workspace_id,e.case_id)=(k.workspace_id,k.case_id)
 AND e.completed_at IS NULL AND e.deadline_at<=$4::timestamptz))
 ORDER BY k.case_id FOR UPDATE OF k`, workspace, operation, pauseAt, now, deadline)
	if err != nil {
		return err
	}
	type item struct {
		id, state, outcome string
		revision           int64
		fenced             bool
		episode            *string
	}
	var due []item
	for rows.Next() {
		var x item
		if err = rows.Scan(&x.id, &x.state, &x.revision, &x.outcome, &x.fenced, &x.episode); err != nil {
			rows.Close()
			return err
		}
		due = append(due, x)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, x := range due {
		if x.revision >= 9007199254740991 {
			return drafts.ErrUnavailable
		}
		d := preparecase.Decision{At: now, Reason: "PAUSE_DRAIN_UNFINISHED", State: "ATTENTION"}
		if x.fenced || x.outcome == "POSSIBLE_EFFECT" || x.outcome == "TARGET_ID_KNOWN" {
			d.State = "UNKNOWN"
		}
		if x.episode != nil {
			d.EpisodeID = *x.episode
		}
		// Use the same envelope and history content writer as normal decisions.
		raw, err := json.Marshal(struct {
			CaseID   string
			Revision int64
			Decision preparecase.Decision
		}{x.id, x.revision, d})
		if err != nil {
			return err
		}
		d.Evidence, err = l.Drafts.Files.Write(ctx, raw, "HISTORY", origin(workspace, x.id, x.revision), now.Add(90*24*time.Hour), now)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET state=$3,revision=revision+1 WHERE workspace_id=$1 AND case_id=$2 AND revision=$4`,
			workspace, x.id, d.State, x.revision); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE hp_processing.episodes SET completed_at=$3 WHERE workspace_id=$1 AND case_id=$2 AND completed_at IS NULL`,
			workspace, x.id, now); err != nil {
			return err
		}
		if err = appendDecisionHistory(ctx, tx, workspace, x.id, x.state, d); err != nil {
			return err
		}
	}
	var open bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.case_identity
 WHERE workspace_id=$1 AND operation_id=$2 AND admitted_at<=$3 AND state IN ('RECEIVED','WAITING_DATA','WORKING'))`,
		workspace, operation, pauseAt).Scan(&open); err != nil {
		return err
	}
	if !open {
		if _, err = tx.Exec(ctx, `UPDATE hp_processing.operations SET state='PAUSED',revision=revision+1 WHERE workspace_id=$1 AND operation_id=$2 AND revision=$3`,
			workspace, operation, revision); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RunDrain runs with the existing owner's serving roster and retries transient
// errors on the next tick. The caller must make failures operationally visible.
func (l LifecycleStore) RunDrain(ctx context.Context, serving func(context.Context) ([]string, error), report func(error)) error {
	if ctx == nil || serving == nil || report == nil {
		return drafts.ErrInvalid
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		workspaces, err := serving(ctx)
		if err != nil {
			report(err)
		}
		for _, workspace := range workspaces {
			if err = l.Drain(ctx, workspace); err != nil {
				report(err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
