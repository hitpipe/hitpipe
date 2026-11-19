package postgres

import (
	"context"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
	"github.com/jackc/pgx/v5"
)

// RecheckDispatch uses the same outbox and job leases as HIT-98, with a
// separate message kind and the saved RECHECK ledger as the source of identity.
type RecheckDispatch struct{ DispatchStore }

func (d RecheckDispatch) Index(ctx context.Context, r dispatchintent.Request, n int) error {
	return d.index(ctx, r, n, recheck.Kind)
}
func (d RecheckDispatch) Jobs(ctx context.Context, w string, n int) ([]dispatchintent.Job, error) {
	return d.jobs(ctx, w, n, recheck.Kind)
}
func (d RecheckDispatch) Claim(ctx context.Context, r dispatchintent.Request, id string, lease time.Duration) (dispatchintent.Claim, error) {
	return d.claim(ctx, r, id, lease, recheckLoad)
}
func (d RecheckDispatch) Check(ctx context.Context, c dispatchintent.Claim) error {
	return d.check(ctx, c, recheckLoad)
}
func (d RecheckDispatch) Finish(ctx context.Context, c dispatchintent.Claim, state string, next time.Time) error {
	return d.finish(ctx, c, state, next, recheckLoad)
}

func recheckLoad(ctx context.Context, tx pgx.Tx, r dispatchintent.Request, id string, lock bool) (dispatchRow, error) {
	x := dispatchRow{c: dispatchintent.Claim{Request: r, MessageID: id}}
	c := &x.c
	var token *string
	var lease *time.Time
	query := `SELECT o.payload_ref::text,o.payload_sha256,o.payload_bytes,o.expires_at,o.digest,o.claim_token::text,o.lease_until,
 o.delivery_count,o.state,o.next_attempt_at,u.aggregate_id::text,u.actor_id::text,u.expected_revision,u.recorded_at,
 j.claim_token::text,j.lease_until,clock_timestamp()
 FROM hp_processing.outbox o JOIN hp_processing.job_index j ON (j.workspace_id,j.job_id,j.kind)=(o.workspace_id,o.message_id,o.kind)
 JOIN hp_processing.user_commands u ON (u.workspace_id,u.request_id)=(o.workspace_id,o.message_id)
 WHERE o.workspace_id=$1 AND o.message_id=$2 AND o.kind=$3 AND u.disposition=$4`
	if lock {
		query += ` FOR UPDATE OF o,j`
	}
	err := tx.QueryRow(ctx, query, r.WorkspaceID, id, recheck.Kind, recheckAccepted).Scan(&c.Ref.ID, &c.Ref.Digest, &c.Ref.Bytes,
		&c.Ref.ExpiresAt, &c.Digest, &token, &lease, &c.Attempts, &x.state, &x.next, &c.CaseID, &c.OwnerID, &c.Revision, &c.AcceptedAt,
		&x.jobToken, &x.jobLease, &x.now)
	if err != nil {
		return x, err
	}
	c.Ref.Class = recheck.Kind
	c.Ref.ExpiresAt = c.Ref.ExpiresAt.UTC()
	c.AcceptedAt = c.AcceptedAt.UTC()
	c.DeadlineAt = c.Ref.ExpiresAt
	if token != nil {
		c.Token = *token
	}
	if lease != nil {
		c.LeaseUntil = lease.UTC()
	}
	k, err := readKnownCase(ctx, tx, r.WorkspaceID, c.CaseID, c.OwnerID, false)
	if err != nil {
		return x, err
	}
	c.OperationID, c.ConfigID, c.ConfigDigest, c.ConnectionID = k.OperationID, k.ConfigID, k.digest, k.ConnectionID
	c.EpisodeID, c.LineageID = k.EpisodeID, k.LineageID
	if k.KnownTaskID != nil {
		c.NativeTaskID = *k.KnownTaskID
	}
	x.current = k.Revision == c.Revision && k.eligible(c.ConnectionID)
	return x, nil
}
