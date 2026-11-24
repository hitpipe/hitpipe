package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/jackc/pgx/v5"
)

const intakeReader = "host=/var/run/postgresql dbname=intake_test sslmode=disable user=hp_intake_app"

func (s *backend) queueProofPath() string { return filepath.Join(s.state, "runtime", "queue.json") }

func (s *backend) possibleIngress() string {
	return filepath.Join(s.state, "runtime", "possible-ingress")
}

func (s *backend) removeQueueProof() error {
	err := os.Remove(s.queueProofPath())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Processing's shared reservation fences the final resume commit against
// publication and invalidation. Queue queries and the webhook itself run under
// the supervisor's queueMu so the cross-process lock is never held over I/O
// except for the short atomic write/removal of the proof.
func (s *backend) withQueueWriter(f func() error) error {
	path := filepath.Join(s.state, "runtime", "queue.lock")
	lock, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.New("invalid school queue lock")
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	return f()
}

func (s *backend) revokeQueueProof() error {
	if _, err := os.Lstat(s.queueProofPath()); os.IsNotExist(err) {
		return nil
	}
	return s.withQueueWriter(s.removeQueueProof)
}

func (s *backend) queueRead(ctx context.Context, dsn string, read func(pgx.Tx) error) error {
	db, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close(context.Background())
	tx, err := db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, s.workspaceID); err != nil {
		return err
	}
	return read(tx)
}

func (s *backend) refreshQueueProof(ctx context.Context) error {
	if _, err := os.Lstat(s.browserJobPath()); !os.IsNotExist(err) {
		return s.revokeQueueProof()
	}
	// A timed-out webhook may still commit. Check the durable ingress marker
	// until that exact event receives an unambiguous idempotent acknowledgement.
	if _, err := os.Lstat(s.possibleIngress()); !os.IsNotExist(err) {
		return s.revokeQueueProof()
	}
	// Intake is read before Processing. Once the scoped durable outbox is empty,
	// a prior send is either durable in Processing or has no future send. New
	// school events cannot enter while this reservation is held.
	var pending bool
	var scope = lifecycle.QueueScope{WorkspaceID: s.workspaceID}
	err := s.queueRead(ctx, intakeReader, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM hp_intake.outbox WHERE workspace_id=$1 AND state<>'ACKED'
		) OR EXISTS (
			SELECT 1 FROM hp_intake.signals WHERE workspace_id=$1 AND connection_id=$2 AND state IN ('HELD','UNRESOLVED')
		)`, s.workspaceID, sourceID).Scan(&pending)
	})
	if err != nil {
		return err
	}
	if pending {
		return s.revokeQueueProof()
	}
	err = s.queueRead(ctx, processingApp, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT o.operation_id::text, a.digest, o.first_activation_at, o.pause_accepted_at, (
			EXISTS(SELECT 1 FROM hp_processing.inbox i WHERE i.workspace_id=o.workspace_id AND i.disposition IN ('HELD','CONFLICT')) OR
			EXISTS(SELECT 1 FROM hp_processing.outbox q WHERE q.workspace_id=o.workspace_id AND q.state<>'ACKED')
		)
		FROM hp_processing.operations o
		JOIN hp_processing.accepted_configs a
		  ON (a.workspace_id,a.operation_id,a.config_id)=(o.workspace_id,o.operation_id,o.selected_config_id)
		WHERE o.workspace_id=$1 AND o.state='PAUSED' AND o.pause_accepted_at IS NOT NULL
		ORDER BY o.first_activation_at DESC LIMIT 1`, s.workspaceID).
			Scan(&scope.OperationID, &scope.ConfigDigest, &scope.FirstActivationAt, &scope.PauseAcceptedAt, &pending)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return err
	}
	if pending || scope.OperationID == "" {
		return s.revokeQueueProof()
	}
	now := time.Now().UTC()
	var current lifecycle.QueueEvidence
	raw, err := os.ReadFile(s.queueProofPath())
	if err == nil && json.Unmarshal(raw, &current) == nil && current.Valid(scope, now) && current.SignalCount == 0 && len(current.Gaps) == 0 {
		return nil
	}
	return s.withQueueWriter(func() error {
		return writeJSON(s.queueProofPath(), lifecycle.QueueEvidence{
			Scope: scope, Generation: mustUUID(), Complete: true, Gaps: []string{},
			ObservedAt: now, ExpiresAt: now.Add(5 * time.Minute),
		})
	})
}

func (s *backend) refreshQueueWhenPaused(ctx context.Context, status proof, statusErr error) {
	if statusErr != nil || !s.queueMu.TryLock() {
		return
	}
	defer s.queueMu.Unlock()
	if status.OperationState != "PAUSED" {
		_ = s.revokeQueueProof()
		return
	}
	check, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	// On a temporary failure an existing proof keeps its original generation
	// and expiry. The successful read established all earlier work was terminal,
	// and the only new ingress must first revoke the proof under the shared fence.
	// A first proof always requires a successful read of both databases.
	_ = s.refreshQueueProof(check)
}
