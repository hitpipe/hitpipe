package dispatch

import (
	"context"
	"errors"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type known struct {
	job                                          Job
	caseID, lineage, episode, connection, config uuid.UUID
	task, workspace                              string
	epoch                                        int64
	fingerprint                                  [32]byte
	ref                                          intake.ContentRef
}

func (p *Postgres) known(ctx context.Context, q intake.Query, auth Authorization) (known, error) {
	k := known{job: Job{q.Workspace, auth.Attempt}, caseID: q.Case, lineage: q.Lineage, epoch: q.Epoch}
	err := p.scoped(ctx, k.job, func(tx pgx.Tx) error {
		var fp []byte
		err := tx.QueryRow(ctx, `
			SELECT l.connection_id,l.config_id,a.episode_id,l.known_task_id,c.external_workspace_id,
			       c.token_fingerprint,f.rendered_payload_ref,f.rendered_payload_sha256,f.rendered_payload_bytes,f.expires_at
			FROM hp_delivery.effect_lineage l
			JOIN hp_delivery.attempts a USING(workspace_id,lineage_id)
			JOIN hp_delivery.destination_connections c USING(workspace_id,connection_id)
			JOIN hp_delivery.intent_content f USING(workspace_id,attempt_id)
			JOIN hp_delivery.inbox i ON i.workspace_id=a.workspace_id AND i.message_id=a.command_message_id
			  AND i.record_id=a.attempt_id AND i.disposition='APPLIED'
			WHERE l.workspace_id=$1 AND l.case_id=$2 AND l.lineage_id=$3 AND a.attempt_id=$4
			  AND l.ever_dispatch_fenced AND l.state IN('RESULT_ID_OBSERVED','TARGET_OBSERVED')
			  AND l.known_task_id IS NOT NULL AND a.network_start_at IS NOT NULL
			  AND c.revoked_at IS NULL AND c.security_epoch=$5 AND a.connection_epoch=$5
			`, q.Workspace, q.Case, q.Lineage, auth.Attempt, q.Epoch).
			Scan(&k.connection, &k.config, &k.episode, &k.task, &k.workspace, &fp, &k.ref.ID, &k.ref.SHA256, &k.ref.Size, &k.ref.Expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return intake.ErrNotFound
		}
		if err != nil || len(fp) != 32 {
			return intake.ErrUnavailable
		}
		copy(k.fingerprint[:], fp)
		if q.TaskID != "" && q.TaskID != k.task {
			return intake.ErrConflict
		}
		return nil
	})
	return k, err
}

// lockKnown rechecks the same immutable target and installed epoch under the
// start gate. The transaction is always closed before any HTTP transmission.
func lockKnown(ctx context.Context, tx pgx.Tx, k known) error {
	var one int
	err := tx.QueryRow(ctx, `
		SELECT 1 FROM hp_delivery.effect_lineage l
		JOIN hp_delivery.attempts a USING(workspace_id,lineage_id)
		JOIN hp_delivery.destination_connections c USING(workspace_id,connection_id)
		JOIN hp_delivery.intent_content f USING(workspace_id,attempt_id)
		WHERE l.workspace_id=$1 AND l.case_id=$2 AND l.lineage_id=$3 AND a.attempt_id=$4
		  AND l.known_task_id=$5 AND l.ever_dispatch_fenced AND l.state IN('RESULT_ID_OBSERVED','TARGET_OBSERVED')
		  AND c.security_epoch=$6 AND c.revoked_at IS NULL AND c.token_fingerprint=$7
		  AND f.rendered_payload_ref=$8 AND f.rendered_payload_sha256=$9 AND f.expires_at>clock_timestamp()
		FOR SHARE OF c`, k.job.Workspace, k.caseID, k.lineage, k.job.Attempt, k.task, k.epoch, k.fingerprint[:], k.ref.ID, k.ref.SHA256).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return intake.ErrDenied
	}
	if err != nil {
		return intake.ErrUnavailable
	}
	return nil
}

func (p *Postgres) chargeRead(ctx context.Context, k known) error {
	return p.scoped(ctx, k.job, func(tx pgx.Tx) error {
		if err := lockKnown(ctx, tx, k); err != nil {
			return err
		}
		r, err := tx.Exec(ctx, `UPDATE hp_delivery.credential_budget SET used_count=used_count+1
			WHERE token_fingerprint=$1 AND used_count<limit_count AND next_eligible_at<=clock_timestamp()`, k.fingerprint[:])
		if err != nil {
			return intake.ErrUnavailable
		}
		if r.RowsAffected() != 1 {
			return errReadBudget
		}
		return nil
	})
}

func (p *Postgres) saveReadback(ctx context.Context, k known, o intake.TaskObservation, attributes *intake.ContentRef,
	wire intake.ContentRef, created time.Time) error {
	return p.scoped(ctx, k.job, func(tx pgx.Tx) error {
		if err := lockKnown(ctx, tx, k); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO hp_delivery.effect_observations(workspace_id,observation_id,lineage_id,attempt_id,
			 knowledge,observed_at,proof_code,proof_digest,known_task_id)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, o.Workspace, o.Observation, o.Lineage, k.job.Attempt,
			o.Knowledge, o.ObservedAt, o.ProofCode, o.ProofDigest, k.task)
		if err != nil {
			return intake.ErrUnavailable
		}
		if attributes != nil {
			_, err = tx.Exec(ctx, `INSERT INTO hp_delivery.observation_content(workspace_id,observation_id,actual_attributes_ref,
				actual_attributes_sha256,actual_attributes_bytes,expected_attributes_digest,created_at,expires_at)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.Workspace, o.Observation, attributes.ID, attributes.SHA256,
				attributes.Size, k.ref.SHA256, created, attributes.Expires)
			if err != nil {
				return intake.ErrUnavailable
			}
		}
		if o.Complete {
			r, err := tx.Exec(ctx, `UPDATE hp_delivery.effect_lineage SET state='TARGET_OBSERVED',updated_at=clock_timestamp()
			WHERE workspace_id=$1 AND lineage_id=$2 AND known_task_id=$3 AND ever_dispatch_fenced
			  AND state IN('RESULT_ID_OBSERVED','TARGET_OBSERVED')`, o.Workspace, o.Lineage, k.task)
			if err != nil || r.RowsAffected() != 1 {
				return intake.ErrUnavailable
			}
		}
		_, err = tx.Exec(ctx, `INSERT INTO hp_delivery.outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,
			payload_bytes,digest,created_at,expires_at) VALUES($1,$2,'TASK_OBSERVATION',$3,$4,$5,$6,$7,$8)`,
			o.Workspace, o.MessageID, wire.ID, wire.SHA256, wire.Size, o.MessageDigest, created, wire.Expires)
		if err != nil {
			return intake.ErrUnavailable
		}
		_, err = tx.Exec(ctx, `INSERT INTO hp_delivery.job_index(job_id,workspace_id,kind,due_at)
			VALUES ($1,$2,'TASK_OBSERVATION',clock_timestamp())`, o.MessageID, o.Workspace)
		if err != nil {
			return intake.ErrUnavailable
		}
		return nil
	})
}
