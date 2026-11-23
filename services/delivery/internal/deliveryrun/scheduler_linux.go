package deliveryrun

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/dispatch"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const batchSize = 8

type scheduler struct {
	pool      *pgxpool.Pool
	files     *intake.Directory
	worker    *dispatch.Worker
	query     *dispatch.QueryService
	publisher *dispatch.Publisher
	gate      intake.StartGate
	scopes    func(context.Context) ([]Scope, error)
	ready     *atomic.Bool
	cursors   map[uuid.UUID]uuid.UUID
}

func (s *scheduler) state(value bool) {
	if s.ready.Swap(value) != value {
		if value {
			logState("READY")
		} else {
			logState("CLOSED")
		}
	}
}

func (s *scheduler) run(ctx context.Context, owner *pgxpool.Conn, app *pgxpool.Pool) error {
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()
	defer s.state(false)
	logState("CLOSED")
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		check, stop := context.WithTimeout(ctx, 2*time.Second)
		if err := owner.Conn().Ping(check); err != nil {
			stop()
			return errors.New("closed: scheduler database session lost")
		}
		scopes, err := s.scopes(check)
		healthy := err == nil && validScopes(scopes) && databaseReady(check, app, "hp_delivery_app") == nil &&
			databaseReady(check, s.pool, "hp_delivery_worker") == nil && s.files.Check() == nil
		if healthy {
			for _, scope := range scopes {
				if s.gate.WithOpen(check, scope.Workspace, scope.Epoch, func() error { return nil }) != nil {
					healthy = false
					break
				}
			}
		}
		stop()
		s.state(healthy)
		if healthy {
			for ws := range s.cursors {
				found := false
				for _, scope := range scopes {
					found = found || ws == scope.Workspace
				}
				if !found {
					delete(s.cursors, ws)
				}
			}
			for _, scope := range scopes {
				if err := s.cycle(ctx, scope); err != nil {
					s.state(false)
					break
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func validScopes(scopes []Scope) bool {
	if len(scopes) == 0 || len(scopes) > 64 {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(scopes))
	for _, scope := range scopes {
		if scope.Workspace == uuid.Nil || scope.Epoch < 1 || seen[scope.Workspace] {
			return false
		}
		seen[scope.Workspace] = true
	}
	return true
}

type pending struct {
	attempt, caseID, lineage uuid.UUID
	state                    string
}

func (s *scheduler) cycle(ctx context.Context, scope Scope) error {
	// The gate admits the short protected SQL scan; release transaction and
	// gate before dispatching. Each reused worker checks its own actual start.
	read, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var work []pending
	var sends []uuid.UUID
	err := s.gate.WithOpen(read, scope.Workspace, scope.Epoch, func() error {
		var err error
		work, sends, err = s.scan(read, scope)
		return err
	})
	cancel()
	if err != nil {
		return err
	}
	if len(work) == 0 {
		delete(s.cursors, scope.Workspace)
	} else {
		s.cursors[scope.Workspace] = work[len(work)-1].attempt
	}
	for _, job := range work {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if job.state == "PREPARED" {
			_, err := s.worker.Run(ctx, dispatch.Job{Workspace: scope.Workspace, Attempt: job.attempt})
			if errors.Is(err, dispatch.ErrUnwired) {
				return err
			}
			continue
		}
		_, err := s.query.QueryKnownTask(ctx, intake.Query{
			Workspace: scope.Workspace, Case: job.caseID, Lineage: job.lineage,
			Epoch: scope.Epoch, Deadline: time.Now().Add(8 * time.Second),
		})
		if errors.Is(err, intake.ErrUnavailable) {
			return err
		}
	}
	for _, message := range sends {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Publisher owns the SQL lease, backoff, ACK and immutable replay.
		// A receiver outage is recoverable and does not erase the observation.
		_ = s.publisher.Publish(ctx, scope.Workspace, message, scope.Epoch)
	}
	return nil
}

func (s *scheduler) scan(ctx context.Context, scope Scope) ([]pending, []uuid.UUID, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", scope.Workspace.String()); err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `SELECT a.attempt_id,l.case_id,l.lineage_id,l.state
		FROM hp_delivery.attempts a
		JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id)
		JOIN hp_delivery.intent_content f USING(workspace_id,attempt_id)
		JOIN hp_delivery.destination_connections c USING(workspace_id,connection_id)
		WHERE a.workspace_id=$1 AND a.attempt_id>$2 AND a.attempt_number=1
		  AND c.revoked_at IS NULL AND c.security_epoch=$3 AND a.connection_epoch=$3
		  AND f.expires_at>clock_timestamp()
		  AND ((l.state='PREPARED' AND NOT l.ever_dispatch_fenced
		    AND a.facts_valid_until>clock_timestamp() AND a.episode_deadline_at>clock_timestamp())
		  OR (l.state='RESULT_ID_OBSERVED' AND l.ever_dispatch_fenced
		    AND a.network_start_at IS NOT NULL AND l.known_task_id IS NOT NULL
		    AND NOT EXISTS(SELECT 1 FROM hp_delivery.effect_observations o
		      WHERE o.workspace_id=a.workspace_id AND o.lineage_id=a.lineage_id
		        AND o.attempt_id=a.attempt_id AND o.knowledge<>'TARGET_ID_OBSERVED')))
		ORDER BY a.attempt_id LIMIT $4`, scope.Workspace, s.cursors[scope.Workspace], scope.Epoch, batchSize)
	if err != nil {
		return nil, nil, err
	}
	work, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var p pending
		err := row.Scan(&p.attempt, &p.caseID, &p.lineage, &p.state)
		return p, err
	})
	if err != nil {
		return nil, nil, err
	}
	rows, err = tx.Query(ctx, `SELECT j.job_id
		FROM hp_delivery.job_index j
		WHERE j.workspace_id=$1 AND j.kind='TASK_OBSERVATION' AND j.due_at<=clock_timestamp()
		ORDER BY j.due_at,j.job_id LIMIT $2`, scope.Workspace, batchSize)
	if err != nil {
		return nil, nil, err
	}
	sends, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, nil, err
	}
	return work, sends, tx.Commit(ctx)
}
