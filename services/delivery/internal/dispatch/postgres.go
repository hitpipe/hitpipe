package dispatch

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) (*Postgres, error) {
	if pool == nil || pool.Config().MaxConns > 10 {
		return nil, ErrUnwired
	}
	return &Postgres{pool: pool}, nil
}

func (p *Postgres) scoped(ctx context.Context, job Job, run func(pgx.Tx) error) error {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return ErrUnwired
	}
	defer tx.Rollback(ctx)
	var worker bool
	if err := tx.QueryRow(ctx, "SELECT current_user='hp_delivery_worker'").Scan(&worker); err != nil || !worker {
		return ErrUnwired
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true),set_config('synchronous_commit','on',true)", job.Workspace.String()); err != nil {
		return ErrUnwired
	}
	if err := run(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return ErrUnwired
	}
	return nil
}

func (p *Postgres) load(ctx context.Context, job Job) (saved, error) {
	var r saved
	a := &r.auth
	a.Workspace, a.Attempt = job.Workspace, job.Attempt
	err := p.scoped(ctx, job, func(tx pgx.Tx) error {
		var state string
		var fenced bool
		var epoch int64
		var revoked bool
		var fingerprint []byte
		err := tx.QueryRow(ctx, `
			SELECT l.case_id,l.config_id,l.connection_id,l.lineage_id,a.episode_id,
			   a.connection_epoch,a.source_facts_digest,a.facts_valid_until,a.episode_deadline_at,
			   c.external_workspace_id,c.token_fingerprint,c.security_epoch,c.revoked_at IS NOT NULL,
			   l.state,l.ever_dispatch_fenced,
			   f.rendered_payload_ref,f.rendered_payload_sha256,f.rendered_payload_bytes,f.expires_at
			FROM hp_delivery.attempts a
			JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id)
			JOIN hp_delivery.destination_connections c USING(workspace_id,connection_id)
			JOIN hp_delivery.intent_content f USING(workspace_id,attempt_id)
			JOIN hp_delivery.inbox i ON i.workspace_id=a.workspace_id AND i.message_id=a.command_message_id
			   AND i.record_id=a.attempt_id AND i.disposition='APPLIED'
			WHERE a.workspace_id=$1 AND a.attempt_id=$2 AND a.attempt_number=1`, job.Workspace, job.Attempt).
			Scan(&a.Case, &a.Config, &a.Connection, &a.Lineage, &a.Episode, &a.Epoch, &a.FactsDigest, &a.FactsUntil,
				&a.Deadline, &a.TargetWorkspace, &fingerprint, &epoch, &revoked, &state, &fenced,
				&r.ref.ID, &r.ref.SHA256, &r.ref.Size, &r.ref.Expires)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHeld
		}
		if err != nil {
			return ErrUnwired
		}
		if fenced || state != "PREPARED" {
			return ErrFenced
		}
		if revoked || epoch != a.Epoch || len(fingerprint) != len(r.fingerprint) {
			return ErrHeld
		}
		copy(r.fingerprint[:], fingerprint)
		return nil
	})
	return r, err
}

func (p *Postgres) fence(ctx context.Context, r saved) error {
	a := r.auth
	return p.scoped(ctx, Job{a.Workspace, a.Attempt}, func(tx pgx.Tx) error {
		row, err := tx.Exec(ctx, `
			UPDATE hp_delivery.effect_lineage
			SET state='DISPATCHING',ever_dispatch_fenced=true,updated_at=clock_timestamp()
			WHERE workspace_id=$1 AND lineage_id=$2 AND state='PREPARED' AND NOT ever_dispatch_fenced`, a.Workspace, a.Lineage)
		if err != nil {
			return ErrUnwired
		}
		if row.RowsAffected() != 1 {
			return ErrFenced
		}
		row, err = tx.Exec(ctx, `
			UPDATE hp_delivery.attempts SET dispatch_fenced_at=clock_timestamp()
			WHERE workspace_id=$1 AND attempt_id=$2 AND lineage_id=$3 AND attempt_number=1
			  AND dispatch_fenced_at IS NULL AND network_start_at IS NULL
			  AND clock_timestamp()<facts_valid_until AND clock_timestamp()<episode_deadline_at`, a.Workspace, a.Attempt, a.Lineage)
		if err != nil {
			return ErrUnwired
		}
		if row.RowsAffected() != 1 {
			return ErrHeld
		}
		return nil
	})
}

func (p *Postgres) markStart(ctx context.Context, r saved) error {
	a := r.auth
	return p.scoped(ctx, Job{a.Workspace, a.Attempt}, func(tx pgx.Tx) error {
		var ok int
		err := tx.QueryRow(ctx, `
			SELECT 1 FROM hp_delivery.destination_connections
			WHERE workspace_id=$1 AND connection_id=$2 AND revoked_at IS NULL
			  AND security_epoch=$3 AND token_fingerprint=$4 AND external_workspace_id=$5
			FOR SHARE`, a.Workspace, a.Connection, a.Epoch, r.fingerprint[:], a.TargetWorkspace).Scan(&ok)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrHeld
		}
		if err != nil {
			return ErrUnwired
		}
		budget, err := tx.Exec(ctx, `
			UPDATE hp_delivery.credential_budget SET used_count=used_count+1
			WHERE token_fingerprint=$1 AND used_count<limit_count AND next_eligible_at<=clock_timestamp()`, r.fingerprint[:])
		if err != nil {
			return ErrUnwired
		}
		if budget.RowsAffected() != 1 {
			return ErrHeld
		}
		row, err := tx.Exec(ctx, `
			UPDATE hp_delivery.attempts a SET network_start_at=clock_timestamp()
			FROM hp_delivery.effect_lineage l, hp_delivery.intent_content f
			WHERE a.workspace_id=$1 AND a.attempt_id=$2 AND a.connection_epoch=$3
			  AND l.workspace_id=a.workspace_id AND l.lineage_id=a.lineage_id
			  AND l.state='DISPATCHING' AND l.ever_dispatch_fenced
			  AND f.workspace_id=a.workspace_id AND f.attempt_id=a.attempt_id
			  AND f.rendered_payload_ref=$4 AND f.rendered_payload_sha256=$5 AND clock_timestamp()<f.expires_at
			  AND a.dispatch_fenced_at IS NOT NULL AND a.network_start_at IS NULL
			  AND clock_timestamp()<a.facts_valid_until AND clock_timestamp()<a.episode_deadline_at`,
			a.Workspace, a.Attempt, a.Epoch, r.ref.ID, r.ref.SHA256)
		if err != nil {
			return ErrUnwired
		}
		if row.RowsAffected() != 1 {
			return ErrHeld
		}
		return nil
	})
}

func (p *Postgres) finish(ctx context.Context, r saved, o outcome) error {
	a := r.auth
	state, knowledge := "AMBIGUOUS", "POSSIBLE_EFFECT"
	proof := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(o.code)))
	if o.result.Disposition == TargetIDObserved {
		state, knowledge, proof = "RESULT_ID_OBSERVED", "TARGET_ID_OBSERVED", o.proof
	}
	return p.scoped(ctx, Job{a.Workspace, a.Attempt}, func(tx pgx.Tx) error {
		row, err := tx.Exec(ctx, `
			UPDATE hp_delivery.effect_lineage SET state=$3,known_task_id=NULLIF($4,''),updated_at=clock_timestamp()
			WHERE workspace_id=$1 AND lineage_id=$2 AND state='DISPATCHING' AND ever_dispatch_fenced`,
			a.Workspace, a.Lineage, state, o.result.TaskID)
		if err != nil {
			return ErrUnwired
		}
		if row.RowsAffected() != 1 {
			return ErrFenced
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO hp_delivery.effect_observations(workspace_id,observation_id,lineage_id,attempt_id,knowledge,
			   observed_at,proof_code,proof_digest,known_task_id)
			VALUES($1,gen_random_uuid(),$2,$3,$4,clock_timestamp(),$5,$6,NULLIF($7,''))`,
			a.Workspace, a.Lineage, a.Attempt, knowledge, o.code, proof, o.result.TaskID); err != nil {
			return ErrUnwired
		}
		return nil
	})
}
