package postgres

import (
	"context"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/jackc/pgx/v5"
	"time"
)

// DispatchStore uses only the owner worker role. No migration, role expansion or
// cross-service connection is performed here. Every payload read is in a NEW
// transaction with SET LOCAL after the application establishes the workspace.
type DispatchStore struct{ DB Beginner }

func (d DispatchStore) tx(ctx context.Context, ws string) (pgx.Tx, error) {
	if d.DB == nil {
		return nil, dispatchintent.ErrHeld
	}
	tx, e := scoped(ctx, d.DB, ws)
	if e != nil {
		return nil, e
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcreaterole AND NOT rolcreatedb AND pg_has_role(current_user,'hp_processing_worker','member') FROM pg_roles WHERE rolname=current_user`).Scan(&valid)
	if e != nil || !valid {
		tx.Rollback(ctx)
		return nil, dispatchintent.ErrHeld
	}
	return tx, nil
}
func (d DispatchStore) Index(ctx context.Context, r dispatchintent.Request, limit int) error {
	return d.index(ctx, r, limit, "DELIVERY_INTENT")
}
func (d DispatchStore) index(ctx context.Context, r dispatchintent.Request, limit int, kind string) error {
	if r.WorkspaceID == "" || r.Epoch < 1 || limit < 1 || limit > 100 {
		return dispatchintent.ErrHeld
	}
	tx, e := d.tx(ctx, r.WorkspaceID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT message_id::text,next_attempt_at FROM hp_processing.outbox WHERE workspace_id=$1 AND kind=$3 AND state IN('PENDING','CLAIMED') ORDER BY next_attempt_at,message_id LIMIT $2`, r.WorkspaceID, limit, kind)
	if e != nil {
		return e
	}
	type job struct {
		id string
		at time.Time
	}
	jobs := []job{}
	for rows.Next() {
		var j job
		if e = rows.Scan(&j.id, &j.at); e != nil {
			rows.Close()
			return e
		}
		jobs = append(jobs, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, j := range jobs {
		tag, e := tx.Exec(ctx, `INSERT INTO hp_processing.job_index(job_id,workspace_id,kind,due_at) VALUES($1,$2,$4,$3) ON CONFLICT(job_id) DO UPDATE SET due_at=EXCLUDED.due_at WHERE job_index.workspace_id=EXCLUDED.workspace_id AND job_index.kind=EXCLUDED.kind`, j.id, r.WorkspaceID, j.at, kind)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return dispatchintent.ErrHeld
		}
	}
	return tx.Commit(ctx)
}
func (d DispatchStore) Jobs(ctx context.Context, workspace string, limit int) ([]dispatchintent.Job, error) {
	return d.jobs(ctx, workspace, limit, "DELIVERY_INTENT")
}
func (d DispatchStore) jobs(ctx context.Context, workspace string, limit int, kind string) ([]dispatchintent.Job, error) {
	if limit < 1 || limit > 100 {
		return nil, dispatchintent.ErrHeld
	}
	tx, e := d.tx(ctx, workspace)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rows, e := tx.Query(ctx, `SELECT j.workspace_id::text,j.job_id::text FROM hp_processing.job_index j
 WHERE ($2='' OR j.workspace_id::text=$2) AND j.kind=$3 AND j.due_at<=clock_timestamp()
 AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
 AND ($2='' OR EXISTS(SELECT 1 FROM hp_processing.outbox o WHERE o.workspace_id=j.workspace_id AND o.message_id=j.job_id
   AND o.kind=j.kind AND o.state IN('PENDING','CLAIMED')))
 ORDER BY j.due_at,j.job_id LIMIT $1`, limit, workspace, kind)
	if e != nil {
		return nil, e
	}
	out := []dispatchintent.Job{}
	for rows.Next() {
		var j dispatchintent.Job
		if e = rows.Scan(&j.WorkspaceID, &j.MessageID); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, j)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return out, nil
}

type dispatchRow struct {
	c        dispatchintent.Claim
	state    string
	next     time.Time
	now      time.Time
	jobToken *string
	jobLease *time.Time
	current  bool
}

func dispatchLoad(ctx context.Context, tx pgx.Tx, r dispatchintent.Request, id string, lock bool) (dispatchRow, error) {
	x := dispatchRow{c: dispatchintent.Claim{Request: r, MessageID: id}, current: true}
	c := &x.c
	var token *string
	var lease *time.Time
	var cfg []byte
	var op, state string
	var completed *time.Time
	q := `SELECT o.payload_ref::text,o.payload_sha256,o.payload_bytes,o.expires_at,o.digest,o.claim_token::text,o.lease_until,o.delivery_count,o.state,o.next_attempt_at,k.case_id::text,k.operation_id::text,k.config_id::text,k.owner_user_id::text,k.revision,k.lineage_id::text,k.state,a.digest,a.canonical_config,e.episode_id::text,e.deadline_at,e.completed_at,p.state,j.claim_token::text,j.lease_until,clock_timestamp()
 FROM hp_processing.outbox o JOIN hp_processing.job_index j ON (j.workspace_id,j.job_id)=(o.workspace_id,o.message_id) AND j.kind='DELIVERY_INTENT'
 JOIN hp_processing.case_identity k ON k.workspace_id=o.workspace_id AND k.lineage_id=o.message_id
 JOIN hp_processing.operations p ON (p.workspace_id,p.operation_id)=(k.workspace_id,k.operation_id)
 JOIN hp_processing.accepted_configs a ON (a.workspace_id,a.operation_id,a.config_id)=(k.workspace_id,k.operation_id,k.config_id)
 JOIN hp_processing.episodes e ON (e.workspace_id,e.case_id)=(k.workspace_id,k.case_id) AND e.completed_at IS NULL
 WHERE o.workspace_id=$1 AND o.message_id=$2 AND o.kind='DELIVERY_INTENT'`
	if lock {
		q += ` FOR UPDATE OF o,j`
	}
	err := tx.QueryRow(ctx, q, r.WorkspaceID, id).Scan(&c.Ref.ID, &c.Ref.Digest, &c.Ref.Bytes, &c.Ref.ExpiresAt, &c.Digest, &token, &lease, &c.Attempts, &x.state, &x.next, &c.CaseID, &c.OperationID, &c.ConfigID, &c.OwnerID, &c.Revision, &c.LineageID, &state, &c.ConfigDigest, &cfg, &c.EpisodeID, &c.DeadlineAt, &completed, &op, &x.jobToken, &x.jobLease, &x.now)
	if err != nil {
		return x, err
	}
	c.Ref.Class = "DELIVERY_INTENT"
	c.Ref.ExpiresAt = c.Ref.ExpiresAt.UTC()
	c.DeadlineAt = c.DeadlineAt.UTC()
	if token != nil {
		c.Token = *token
	}
	if lease != nil {
		c.LeaseUntil = lease.UTC()
	}
	canonical, err := jcs.Transform(cfg)
	if err != nil || intent.Digest(canonical) != c.ConfigDigest || state != "WORKING" || op != "ACTIVE" || completed != nil {
		return x, dispatchintent.ErrHeld
	}
	var config map[string]json.RawMessage
	if json.Unmarshal(cfg, &config) != nil || json.Unmarshal(config["destination_connection_id"], &c.ConnectionID) != nil || c.ConnectionID == "" || c.LineageID != id {
		return x, dispatchintent.ErrHeld
	}
	return x, nil
}

type dispatchLoader func(context.Context, pgx.Tx, dispatchintent.Request, string, bool) (dispatchRow, error)

func (d DispatchStore) Claim(ctx context.Context, r dispatchintent.Request, id string, lease time.Duration) (dispatchintent.Claim, error) {
	return d.claim(ctx, r, id, lease, dispatchLoad)
}
func (d DispatchStore) claim(ctx context.Context, r dispatchintent.Request, id string, lease time.Duration, load dispatchLoader) (dispatchintent.Claim, error) {
	zero := dispatchintent.Claim{}
	if r.WorkspaceID == "" || id == "" || r.Epoch < 1 || lease <= 0 {
		return zero, dispatchintent.ErrHeld
	}
	tx, e := d.tx(ctx, r.WorkspaceID)
	if e != nil {
		return zero, e
	}
	defer tx.Rollback(ctx)
	x, e := load(ctx, tx, r, id, true)
	if e != nil {
		return zero, e
	}
	if x.next.After(x.now) || (x.state != "PENDING" && x.state != "CLAIMED") || (x.state == "CLAIMED" && x.c.LeaseUntil.After(x.now)) || (x.jobLease != nil && x.jobLease.After(x.now)) {
		return zero, dispatchintent.ErrStale
	}
	token, e := contentfiles.UUID()
	if e != nil {
		return zero, e
	}
	until := x.now.Add(lease).UTC().Truncate(time.Microsecond)
	_, e = tx.Exec(ctx, `UPDATE hp_processing.outbox SET state='CLAIMED',claim_token=$3,lease_until=$4,delivery_count=delivery_count+1 WHERE workspace_id=$1 AND message_id=$2`, r.WorkspaceID, id, token, until)
	if e != nil {
		return zero, e
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.job_index SET claim_token=$3,lease_until=$4 WHERE workspace_id=$1 AND job_id=$2`, r.WorkspaceID, id, token, until)
	if e != nil {
		return zero, e
	}
	if e = tx.Commit(ctx); e != nil {
		return zero, e
	}
	x.c.Token = token
	x.c.LeaseUntil = until
	x.c.Attempts++
	return x.c, nil
}
func owned(x dispatchRow, c dispatchintent.Claim) bool {
	return x.state == "CLAIMED" && x.c == c && x.now.Before(c.LeaseUntil) && x.jobToken != nil && *x.jobToken == c.Token && x.jobLease != nil && x.jobLease.Equal(c.LeaseUntil)
}
func (d DispatchStore) Check(ctx context.Context, c dispatchintent.Claim) error {
	return d.check(ctx, c, dispatchLoad)
}
func (d DispatchStore) check(ctx context.Context, c dispatchintent.Claim, load dispatchLoader) error {
	tx, e := d.tx(ctx, c.WorkspaceID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	x, e := load(ctx, tx, c.Request, c.MessageID, false)
	if e != nil {
		return e
	}
	if !owned(x, c) || !x.current || !x.now.Before(c.Ref.ExpiresAt) || !x.now.Before(c.DeadlineAt) {
		return dispatchintent.ErrStale
	}
	return tx.Commit(ctx)
}
func (d DispatchStore) Finish(ctx context.Context, c dispatchintent.Claim, state string, next time.Time) error {
	return d.finish(ctx, c, state, next, dispatchLoad)
}
func (d DispatchStore) finish(ctx context.Context, c dispatchintent.Claim, state string, next time.Time, load dispatchLoader) error {
	if state != "ACKED" && state != "HELD" && state != "PENDING" {
		return dispatchintent.ErrHeld
	}
	tx, e := d.tx(ctx, c.WorkspaceID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	x, e := load(ctx, tx, c.Request, c.MessageID, true)
	if e != nil {
		return e
	}
	if !owned(x, c) {
		return dispatchintent.ErrStale
	}
	if state == "PENDING" && (!next.After(x.now) || !next.Before(c.Ref.ExpiresAt) || !next.Before(c.DeadlineAt)) {
		return dispatchintent.ErrHeld
	}
	if next.IsZero() {
		next = x.now
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.outbox SET state=$3,next_attempt_at=$4,claim_token=NULL,lease_until=NULL WHERE workspace_id=$1 AND message_id=$2`, c.WorkspaceID, c.MessageID, state, next)
	if e != nil {
		return e
	}
	if state == "PENDING" {
		_, e = tx.Exec(ctx, `UPDATE hp_processing.job_index SET due_at=$3,claim_token=NULL,lease_until=NULL WHERE workspace_id=$1 AND job_id=$2`, c.WorkspaceID, c.MessageID, next)
	} else {
		_, e = tx.Exec(ctx, `DELETE FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2`, c.WorkspaceID, c.MessageID)
	}
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
