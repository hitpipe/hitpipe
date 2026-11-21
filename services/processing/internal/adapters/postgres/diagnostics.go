package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
)

const DiagnosticSampleLimit = 100

// QueueSample is a bounded count of stored rows within one existing worker
// scope. Nullable ages distinguish absent rows from a measured zero.
type QueueSample struct {
	InboxRows, OutboxRows, JobRows, CaseRows                                  int
	InboxHeld, OutboxPending, OutboxClaimed, OutboxHeld, OutboxAcked, JobsDue int
	ExpiredJobLeases, ExpiredOutboxLeases, WaitingData, Attention, Unknown    int
	InboxAge, OutboxAge, OutboxDueLag, JobDueLag                              *int64
	ExpiredJobID, ExpiredOutboxID                                             string
}

// SampleQueues is called inside the runtime's authorized workspace start.
// job_index has no RLS, so its explicit workspace predicate is essential.
func (d DispatchStore) SampleQueues(ctx context.Context, workspace string) (QueueSample, error) {
	var sample QueueSample
	tx, err := d.txWithMode(ctx, workspace, pgx.ReadOnly)
	if err != nil {
		return sample, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `WITH stamp AS (SELECT statement_timestamp() AS now),
 inbox AS (SELECT received_at,disposition FROM hp_processing.inbox
   WHERE workspace_id=$1 ORDER BY message_id LIMIT $2),
 outbox AS (SELECT message_id,created_at,next_attempt_at,state,lease_until FROM hp_processing.outbox
   WHERE workspace_id=$1 ORDER BY message_id LIMIT $2),
 jobs AS (SELECT job_id,due_at,lease_until,claim_token IS NOT NULL AS has_claim FROM hp_processing.job_index
   WHERE workspace_id=$1 ORDER BY due_at,job_id LIMIT $2),
 cases AS (SELECT state FROM hp_processing.case_identity
   WHERE workspace_id=$1 ORDER BY case_id LIMIT $2)
 SELECT
 (SELECT count(*) FROM inbox),
 (SELECT count(*) FROM outbox),
 (SELECT count(*) FROM jobs),
 (SELECT count(*) FROM cases),
 (SELECT count(*) FROM inbox WHERE disposition='HELD'),
 (SELECT count(*) FROM outbox WHERE state='PENDING'),
 (SELECT count(*) FROM outbox WHERE state='CLAIMED'),
 (SELECT count(*) FROM outbox WHERE state='HELD'),
 (SELECT count(*) FROM outbox WHERE state='ACKED'),
 (SELECT count(*) FROM jobs WHERE due_at<=now),
 (SELECT count(*) FROM jobs WHERE has_claim AND lease_until<=now),
 (SELECT count(*) FROM outbox WHERE state='CLAIMED' AND lease_until<=now),
 (SELECT count(*) FROM cases WHERE state='WAITING_DATA'),
 (SELECT count(*) FROM cases WHERE state='ATTENTION'),
 (SELECT count(*) FROM cases WHERE state='UNKNOWN'),
 (SELECT (extract(epoch FROM now-least(now,min(received_at)))*1000)::bigint
  FROM inbox WHERE isfinite(received_at) HAVING count(*)>0),
 (SELECT (extract(epoch FROM now-least(now,min(created_at)))*1000)::bigint
  FROM outbox WHERE isfinite(created_at) AND state IN ('PENDING','CLAIMED','HELD') HAVING count(*)>0),
 (SELECT (extract(epoch FROM now-min(next_attempt_at))*1000)::bigint
  FROM outbox WHERE isfinite(next_attempt_at) AND next_attempt_at<=now AND state IN ('PENDING','CLAIMED') HAVING count(*)>0),
 (SELECT (extract(epoch FROM now-min(due_at))*1000)::bigint
  FROM jobs WHERE isfinite(due_at) AND due_at<=now HAVING count(*)>0),
 COALESCE((SELECT job_id::text FROM jobs WHERE has_claim AND lease_until<=now ORDER BY job_id LIMIT 1),''),
 COALESCE((SELECT message_id::text FROM outbox WHERE state='CLAIMED' AND lease_until<=now ORDER BY message_id LIMIT 1),'')
 FROM stamp`, workspace, DiagnosticSampleLimit).Scan(
		&sample.InboxRows, &sample.OutboxRows, &sample.JobRows, &sample.CaseRows,
		&sample.InboxHeld, &sample.OutboxPending, &sample.OutboxClaimed, &sample.OutboxHeld, &sample.OutboxAcked, &sample.JobsDue,
		&sample.ExpiredJobLeases, &sample.ExpiredOutboxLeases, &sample.WaitingData, &sample.Attention, &sample.Unknown,
		&sample.InboxAge, &sample.OutboxAge, &sample.OutboxDueLag, &sample.JobDueLag, &sample.ExpiredJobID, &sample.ExpiredOutboxID)
	if err != nil {
		return QueueSample{}, err
	}
	return sample, tx.Commit(ctx)
}
