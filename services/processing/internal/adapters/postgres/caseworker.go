package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

type CaseJobStore struct{ DB Beginner }

type CaseClaim struct {
	preparecase.Request
	token, operation, config string
	owner                    string
	operationRevision        int64
	signalAt, until          time.Time
}

func (c CaseClaim) Until() time.Time { return c.until }

const caseWork = `
 FROM hp_processing.case_identity k
 JOIN hp_processing.operations o USING(workspace_id,operation_id)
 JOIN LATERAL (
   SELECT max(i.received_at) AS at FROM hp_processing.inbox i
   WHERE i.workspace_id=k.workspace_id AND i.record_id=k.case_id AND i.disposition='APPLIED'
 ) signal ON signal.at IS NOT NULL
`

const eligibleCase = `
 k.workspace_id=$1 AND o.state='ACTIVE' AND o.selected_config_id=k.config_id
 AND o.owner_user_id=k.owner_user_id
 AND k.lineage_id IS NULL AND k.known_task_id IS NULL AND NOT k.ever_dispatch_fenced
 AND k.outcome_knowledge IN ('UNESTABLISHED','NO_EFFECT')
 AND (k.state='RECEIVED' OR (k.state='WAITING_DATA' AND signal.at > COALESCE((
   SELECT max(h.at) FROM hp_processing.safety_history h
   WHERE h.workspace_id=k.workspace_id AND h.case_id=k.case_id
     AND h.old_state IN ('RECEIVED','WAITING_DATA') AND h.new_state='WAITING_DATA'
 ),'-infinity')))
 AND NOT EXISTS (
   SELECT 1 FROM hp_processing.episodes e WHERE e.workspace_id=k.workspace_id AND e.case_id=k.case_id
   AND (e.completed_at IS NOT NULL OR e.deadline_at<=clock_timestamp())
 )
`

// Index reconstructs scheduling metadata from the committed case and inbox. The
// HTTP role does not write job_index. An interrupted index operation loses no
// work, and an unsuccessful decision is woken only by a distinct applied signal.
func (s CaseJobStore) Index(ctx context.Context, workspace string, limit int) error {
	if limit < 1 || limit > 100 {
		return preparecase.ErrHold
	}
	tx, err := (DispatchStore{DB: s.DB}).tx(ctx, workspace)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `WITH obsolete AS (
 SELECT j.job_id FROM hp_processing.job_index j
 LEFT JOIN hp_processing.case_identity k ON (k.workspace_id,k.case_id)=(j.workspace_id,j.job_id)
 WHERE j.workspace_id=$1 AND j.kind='CASE_PREPARE'
 AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
 AND (k.case_id IS NULL OR k.state NOT IN ('RECEIVED','WAITING_DATA') OR k.lineage_id IS NOT NULL
   OR k.known_task_id IS NOT NULL OR k.ever_dispatch_fenced)
 ORDER BY j.due_at,j.job_id LIMIT $2 FOR UPDATE OF j SKIP LOCKED
 ) DELETE FROM hp_processing.job_index j USING obsolete WHERE j.job_id=obsolete.job_id`, workspace, limit)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.job_index(job_id,workspace_id,kind,due_at)
 SELECT k.case_id,k.workspace_id,'CASE_PREPARE',signal.at`+caseWork+`
 WHERE `+eligibleCase+`
 AND NOT EXISTS (SELECT 1 FROM hp_processing.job_index j WHERE j.job_id=k.case_id)
 ORDER BY signal.at,k.case_id LIMIT $2 ON CONFLICT(job_id) DO NOTHING`, workspace, limit)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Claim pins the operation revision and latest applied inbox event. A competing
// worker skips this row. No SQL transaction survives a successful Claim return.
func (s CaseJobStore) Claim(ctx context.Context, workspace string, epoch int64) (CaseClaim, error) {
	claim := CaseClaim{Request: preparecase.Request{WorkspaceID: workspace, Epoch: epoch}}
	if epoch < 1 {
		return CaseClaim{}, preparecase.ErrHold
	}
	var err error
	if claim.token, err = newID(); err != nil {
		return CaseClaim{}, err
	}
	tx, err := (DispatchStore{DB: s.DB}).tx(ctx, workspace)
	if err != nil {
		return CaseClaim{}, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `SELECT k.case_id::text,k.revision,k.operation_id::text,k.config_id::text,k.owner_user_id::text,o.revision,signal.at`+caseWork+`
 JOIN hp_processing.job_index j ON (j.workspace_id,j.job_id)=(k.workspace_id,k.case_id)
 WHERE `+eligibleCase+`
 AND j.kind='CASE_PREPARE' AND j.due_at<=clock_timestamp()
 AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
 ORDER BY j.due_at,j.job_id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`, workspace).Scan(
		&claim.CaseID, &claim.ExpectedRevision, &claim.operation, &claim.config, &claim.owner, &claim.operationRevision, &claim.signalAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CaseClaim{}, nil
	}
	if err != nil {
		return CaseClaim{}, err
	}
	err = tx.QueryRow(ctx, `UPDATE hp_processing.job_index SET claim_token=$3,lease_until=clock_timestamp()+interval '10 seconds'
 WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_PREPARE' RETURNING lease_until`, workspace, claim.CaseID, claim.token).Scan(&claim.until)
	if err != nil {
		return CaseClaim{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CaseClaim{}, err
	}
	return claim, nil
}

func (c CaseClaim) check(ctx context.Context, tx pgx.Tx, lock bool) error {
	query := `SELECT 1 FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_PREPARE'
 AND claim_token=$3 AND lease_until=$4 AND lease_until>clock_timestamp()`
	if lock {
		query += ` FOR UPDATE`
	}
	var present int
	err := tx.QueryRow(ctx, query, c.WorkspaceID, c.CaseID, c.token, c.until).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return preparecase.ErrStale
	}
	return err
}

func (c CaseClaim) checkCase(ctx context.Context, tx pgx.Tx) error {
	var present int
	err := tx.QueryRow(ctx, `SELECT 1`+caseWork+` WHERE `+eligibleCase+`
 AND k.case_id=$2 AND k.revision=$3 AND k.operation_id=$4 AND k.config_id=$5
 AND k.owner_user_id=$6 AND o.revision=$7 AND signal.at=$8`,
		c.WorkspaceID, c.CaseID, c.ExpectedRevision, c.operation, c.config, c.owner, c.operationRevision, c.signalAt).Scan(&present)
	if errors.Is(err, pgx.ErrNoRows) {
		return preparecase.ErrStale
	}
	return err
}

// CheckStart rereads current SQL state before an actual source network start.
// It commits before the caller initiates I/O. The shared runtime gate orders
// that start with PAUSE/revocation, and commit independently rechecks the lease.
func (s CaseJobStore) CheckStart(ctx context.Context, c CaseClaim) error {
	tx, err := scoped(ctx, s.DB, c.WorkspaceID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = c.checkCase(ctx, tx); err != nil {
		return err
	}
	if err = c.check(ctx, tx, false); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (c CaseClaim) complete(ctx context.Context, tx pgx.Tx) error {
	tag, err := tx.Exec(ctx, `DELETE FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_PREPARE'
 AND claim_token=$3 AND lease_until=$4 AND lease_until>clock_timestamp()`, c.WorkspaceID, c.CaseID, c.token, c.until)
	if err == nil && tag.RowsAffected() != 1 {
		return preparecase.ErrStale
	}
	return err
}

// ClaimedDecisionStore adapts the established financial transaction to a single
// lease. Clearing the index row and writing the decision are one SQL commit.
type ClaimedDecisionStore struct {
	DecisionStore
	Claim CaseClaim
}

func (s ClaimedDecisionStore) Load(ctx context.Context, r preparecase.Request) (preparecase.Case, error) {
	if r != s.Claim.Request {
		return preparecase.Case{}, preparecase.ErrStale
	}
	tx, err := scoped(ctx, s.DB, r.WorkspaceID)
	if err != nil {
		return preparecase.Case{}, err
	}
	defer tx.Rollback(ctx)
	if err = s.Claim.check(ctx, tx, false); err != nil {
		return preparecase.Case{}, err
	}
	if err = s.Claim.checkCase(ctx, tx); err != nil {
		return preparecase.Case{}, err
	}
	c, err := loadCase(ctx, tx, r, false)
	if err != nil {
		return preparecase.Case{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return preparecase.Case{}, err
	}
	return c, nil
}

func (s ClaimedDecisionStore) Commit(ctx context.Context, c preparecase.Case, d preparecase.Decision, now time.Time) (preparecase.Result, error) {
	if c.Request != s.Claim.Request {
		return preparecase.Result{}, preparecase.ErrStale
	}
	return s.DecisionStore.commit(ctx, c, d, now, &s.Claim)
}
