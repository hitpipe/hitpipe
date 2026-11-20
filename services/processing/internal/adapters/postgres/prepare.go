package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"github.com/jackc/pgx/v5"
)

type DecisionStore struct {
	DB      Beginner
	Content preparecase.Content
}

func scoped(ctx context.Context, db Beginner, ws string) (pgx.Tx, error) {
	return scopedWithMode(ctx, db, ws, "")
}

func scopedWithMode(ctx context.Context, db Beginner, ws string, mode pgx.TxAccessMode) (pgx.Tx, error) {
	tx, e := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: mode})
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, ws); e != nil {
		tx.Rollback(ctx)
		return nil, e
	}
	return tx, nil
}
func loadCase(ctx context.Context, tx pgx.Tx, r preparecase.Request, lock bool) (preparecase.Case, error) {
	c := preparecase.Case{Request: r}
	var config []byte
	var schema, evaluator string
	var activation *time.Time
	var lineage, knownTask *string
	query := `SELECT k.case_id::text,k.operation_id::text,k.config_id::text,k.owner_user_id::text,k.session_id,k.source_account_id,k.livemode,k.state,k.outcome_knowledge,k.revision,k.ever_dispatch_fenced,k.lineage_id::text,k.known_task_id,k.admitted_at,o.revision,o.state,o.first_activation_at,a.digest,a.canonical_config,a.schema_version,a.evaluator_version
 FROM hp_processing.case_identity k JOIN hp_processing.operations o ON (k.workspace_id,k.operation_id)=(o.workspace_id,o.operation_id)
 JOIN hp_processing.accepted_configs a ON (k.workspace_id,k.operation_id,k.config_id)=(a.workspace_id,a.operation_id,a.config_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2`
	if lock {
		query += ` FOR UPDATE OF k,o`
	}
	e := tx.QueryRow(ctx, query, r.WorkspaceID, r.CaseID).Scan(&c.CaseID, &c.OperationID, &c.ConfigID, &c.OwnerID, &c.SessionID, &c.AccountID, &c.Live, &c.State, &c.Outcome, &c.Revision, &c.Fenced, &lineage, &knownTask, &c.AdmittedAt, &c.OperationRevision, &c.OperationState, &activation, &c.ConfigDigest, &config, &schema, &evaluator)
	if e != nil {
		return c, preparecase.ErrHold
	}
	if activation == nil {
		return c, preparecase.ErrHold
	}
	c.ActivationAt = *activation
	if knownTask != nil {
		c.Fenced = true
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(config, &m) != nil {
		return c, preparecase.ErrHold
	}
	str := func(k string) string { var s string; json.Unmarshal(m[k], &s); return s }
	c.Mapping = preparecase.Mapping{SourceConnectionID: str("source_connection_id"), DestinationConnectionID: str("destination_connection_id"), DestinationWorkspaceID: str("destination_workspace_id"), ListID: str("list_id"), AssigneeID: str("assignee_id"), PriceID: str("price_id"), ServiceLabel: str("service_label"), TitlePattern: str("title_pattern")}
	if json.Unmarshal(m["required_context_keys"], &c.Mapping.RequiredKeys) != nil || c.Mapping.RequiredKeys == nil {
		return c, preparecase.ErrHold
	}
	if str("pack_id") != "PAID_ORDER_TASK_V1" || (str("mode") != "TEST" && str("mode") != "LIVE") || (str("mode") == "LIVE") != c.Live {
		return c, preparecase.ErrHold
	}
	canonical, e := jcs.Transform(config)
	if e != nil || intent.Digest(canonical) != c.ConfigDigest {
		return c, preparecase.ErrHold
	}
	live := c.Live
	c.Policy = eligibility.Policy{ConfigID: c.ConfigID, SchemaVersion: schema, EvaluatorVersion: evaluator, AccountID: c.AccountID, ConnectionID: c.Mapping.SourceConnectionID, LiveMode: &live, PriceID: c.Mapping.PriceID, ActivationAt: c.ActivationAt}
	if !eligibility.ValidatePolicy(c.Policy) {
		return c, preparecase.ErrHold
	}
	rows, e := tx.Query(ctx, `SELECT field_key,value_ref::text,value_sha256,serialized_value_bytes,added_at,expires_at FROM hp_processing.case_content WHERE workspace_id=$1 AND case_id=$2 ORDER BY field_key`, r.WorkspaceID, r.CaseID)
	if e != nil {
		return c, e
	}
	for rows.Next() {
		var v preparecase.ContextRef
		v.Ref.Class = "CASE_CONTEXT"
		if e = rows.Scan(&v.Key, &v.Ref.ID, &v.Ref.Digest, &v.Ref.Bytes, &v.AddedAt, &v.Ref.ExpiresAt); e != nil {
			rows.Close()
			return c, e
		}
		c.Context = append(c.Context, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return c, e
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2)`, r.WorkspaceID, r.CaseID).Scan(&c.HasEpisodes); e != nil {
		return c, e
	}
	e = tx.QueryRow(ctx, `SELECT episode_id::text,started_at,deadline_at FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2 AND completed_at IS NULL ORDER BY started_at LIMIT 1`, r.WorkspaceID, r.CaseID).Scan(&c.EpisodeID, &c.EpisodeStartedAt, &c.EpisodeDeadlineAt)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return c, e
	}
	if lineage != nil {
		c.LineageID = *lineage
		c.MessageID = *lineage
		ref := preparecase.Ref{Class: "DELIVERY_INTENT"}
		var state string
		e = tx.QueryRow(ctx, `SELECT payload_ref::text,payload_sha256,payload_bytes,expires_at,digest,state FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2 AND kind='DELIVERY_INTENT'`, r.WorkspaceID, *lineage).Scan(&ref.ID, &ref.Digest, &ref.Bytes, &ref.ExpiresAt, &c.MessageDigest, &state)
		if e != nil || state != "PENDING" {
			return c, preparecase.ErrHold
		}
		c.OutboxRef = &ref
	}
	return c, nil
}
func (db DecisionStore) Load(ctx context.Context, r preparecase.Request) (preparecase.Case, error) {
	if db.DB == nil {
		return preparecase.Case{}, preparecase.ErrHold
	}
	tx, e := scoped(ctx, db.DB, r.WorkspaceID)
	if e != nil {
		return preparecase.Case{}, e
	}
	defer tx.Rollback(ctx)
	c, e := loadCase(ctx, tx, r, false)
	if e != nil {
		return c, e
	}
	if e = tx.Commit(ctx); e != nil {
		return preparecase.Case{}, e
	}
	return c, nil
}
func (db DecisionStore) Commit(ctx context.Context, expected preparecase.Case, d preparecase.Decision, now time.Time) (preparecase.Result, error) {
	return db.commit(ctx, expected, d, now, nil)
}

func (db DecisionStore) commit(ctx context.Context, expected preparecase.Case, d preparecase.Decision, now time.Time, claim *CaseClaim) (preparecase.Result, error) {
	zero := preparecase.Result{}
	if db.DB == nil || db.Content == nil || now.IsZero() || d.At.IsZero() || d.At.After(now) || d.Evidence.Class != "HISTORY" {
		return zero, preparecase.ErrHold
	}
	if _, e := db.Content.Read(ctx, d.Evidence, now); e != nil {
		return zero, e
	}
	var payload preparecase.DeliveryIntent
	if d.Intent != nil {
		if d.Intent.Class != "DELIVERY_INTENT" {
			return zero, preparecase.ErrHold
		}
		b, e := db.Content.Read(ctx, *d.Intent, now)
		if e != nil {
			return zero, e
		}
		decoder := json.NewDecoder(bytes.NewReader(b))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&payload) != nil {
			return zero, preparecase.ErrHold
		}
		var m map[string]json.RawMessage
		if json.Unmarshal(b, &m) != nil {
			return zero, preparecase.ErrHold
		}
		delete(m, "message_digest")
		unsigned, _ := json.Marshal(m)
		canonical, e := jcs.Transform(unsigned)
		if e != nil || intent.Digest(canonical) != payload.MessageDigest || payload.MessageDigest != d.MessageDigest || payload.MessageId != d.MessageID || payload.LineageId != d.LineageID || payload.MessageId != payload.LineageId || payload.EpisodeId != d.EpisodeID || payload.CaseId != expected.CaseID || payload.ConfigId != expected.ConfigID || payload.ConfigDigest != expected.ConfigDigest || payload.OperationId != expected.OperationID || payload.WorkspaceId != expected.WorkspaceID || payload.ConnectionEpoch != expected.Epoch || payload.ConnectionId != expected.Mapping.DestinationConnectionID || d.State != "WORKING" {
			return zero, preparecase.ErrHold
		}
	}
	// Recheck the original required content before entering the SQL transaction.
	for _, key := range expected.Mapping.RequiredKeys {
		found := false
		for _, v := range expected.Context {
			if v.Key == key {
				if _, e := db.Content.Read(ctx, v.Ref, now); e != nil && d.Intent != nil {
					return zero, e
				}
				found = true
				break
			}
		}
		if !found && d.Intent != nil {
			return zero, preparecase.ErrHold
		}
	}
	tx, e := scoped(ctx, db.DB, expected.WorkspaceID)
	if e != nil {
		return zero, e
	}
	defer tx.Rollback(ctx)
	if claim != nil {
		if e = claim.check(ctx, tx, true); e != nil {
			return zero, e
		}
	}
	current, e := loadCase(ctx, tx, expected.Request, true)
	if e != nil {
		return zero, e
	}
	if current.Revision != expected.Revision || current.Revision != expected.ExpectedRevision || current.OperationRevision != expected.OperationRevision || current.ConfigID != expected.ConfigID || current.ConfigDigest != expected.ConfigDigest || current.OwnerID != expected.OwnerID || current.State != expected.State || current.LineageID != "" || current.Fenced || (current.Outcome != "UNESTABLISHED" && current.Outcome != "NO_EFFECT") {
		return zero, preparecase.ErrStale
	}
	// Admission serializes through the same operation row. A payment signal
	// committed while facts were read must get its own current decision.
	if claim != nil {
		if d.At.Before(claim.signalAt) {
			return zero, preparecase.ErrStale
		}
		if e = claim.checkCase(ctx, tx); e != nil {
			return zero, e
		}
	}
	var operation string
	var dbNow time.Time
	if e = tx.QueryRow(ctx, `SELECT state,clock_timestamp() FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id=$2`, current.WorkspaceID, current.OperationID).Scan(&operation, &dbNow); e != nil {
		return zero, e
	}
	if operation != "ACTIVE" || ctx.Err() != nil || (current.State != "RECEIVED" && current.State != "WAITING_DATA") || !dbNow.Before(d.Evidence.ExpiresAt) {
		return zero, preparecase.ErrHold
	}
	// The gate owns current authority/epoch throughout this bounded SQL callback.
	// Source financial proof is checked again after obtaining the local row lock.
	if d.Intent != nil {
		for _, key := range current.Mapping.RequiredKeys {
			var oldRef, newRef *preparecase.ContextRef
			for i := range expected.Context {
				if expected.Context[i].Key == key {
					oldRef = &expected.Context[i]
				}
			}
			for i := range current.Context {
				if current.Context[i].Key == key {
					newRef = &current.Context[i]
				}
			}
			if oldRef == nil || newRef == nil || oldRef.Ref.ID != newRef.Ref.ID || oldRef.Ref.Digest != newRef.Ref.Digest || oldRef.Ref.Bytes != newRef.Ref.Bytes || !oldRef.Ref.ExpiresAt.Equal(newRef.Ref.ExpiresAt) || !oldRef.AddedAt.Equal(newRef.AddedAt) || !dbNow.Before(newRef.Ref.ExpiresAt) {
				return zero, preparecase.ErrStale
			}
		}
		if eligibility.Evaluate(current.Policy, d.Snapshot.Facts, dbNow).Status != eligibility.Valid || !dbNow.Before(d.Intent.ExpiresAt) || !dbNow.Before(d.DeadlineAt) || !payload.FactsValidUntil.Equal(d.Snapshot.Facts.ValidUntil) || !payload.DeadlineAt.Equal(d.DeadlineAt) {
			return zero, preparecase.ErrHold
		}
		if current.EpisodeID != "" {
			if d.EpisodeID != current.EpisodeID || !d.DeadlineAt.Equal(current.EpisodeDeadlineAt) {
				return zero, preparecase.ErrStale
			}
		} else {
			if current.HasEpisodes || !d.DeadlineAt.Equal(d.At.Add(300*time.Second)) {
				return zero, preparecase.ErrHold
			}
			_, e = tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at) VALUES($1,$2,$3,$4,$5)`, current.WorkspaceID, d.EpisodeID, current.CaseID, d.At, d.DeadlineAt)
			if e != nil {
				return zero, e
			}
		}
		_, e = tx.Exec(ctx, `INSERT INTO hp_processing.outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,expires_at) VALUES($1,$2,'DELIVERY_INTENT',$3,$4,$5,$6,$7)`, current.WorkspaceID, d.MessageID, d.Intent.ID, d.Intent.Digest, d.Intent.Bytes, d.MessageDigest, d.Intent.ExpiresAt)
		if e != nil {
			return zero, e
		}
	}
	if d.State != "WORKING" && d.State != "WAITING_DATA" && d.State != "EXCLUDED" {
		return zero, preparecase.ErrHold
	}
	var lineage any
	if d.Intent != nil {
		lineage = d.LineageID
	}
	tag, e := tx.Exec(ctx, `UPDATE hp_processing.case_identity SET state=$3,revision=revision+1,lineage_id=$4 WHERE workspace_id=$1 AND case_id=$2 AND revision=$5 AND lineage_id IS NULL AND NOT ever_dispatch_fenced`, current.WorkspaceID, current.CaseID, d.State, lineage, current.Revision)
	if e != nil {
		return zero, e
	}
	if tag.RowsAffected() != 1 {
		return zero, preparecase.ErrStale
	}
	if e = appendDecisionHistory(ctx, tx, current.WorkspaceID, current.CaseID, current.State, d); e != nil {
		return zero, e
	}
	if claim != nil {
		if e = claim.complete(ctx, tx); e != nil {
			return zero, e
		}
	}
	if ctx.Err() != nil {
		return zero, ctx.Err()
	}
	if e = tx.Commit(ctx); e != nil {
		return zero, e
	}
	result := preparecase.Result{CaseID: current.CaseID, State: d.State, Disposition: "DECISION_RECORDED", Revision: current.Revision + 1}
	if d.Intent != nil {
		result.Disposition = "PENDING_OUTBOX"
		result.MessageID = d.MessageID
	}
	return result, nil
}

func appendDecisionHistory(ctx context.Context, tx pgx.Tx, workspace, caseID, oldState string, d preparecase.Decision) error {
	history, e := newID()
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,old_state,new_state) VALUES($1,$2,$3,'SYSTEM',$4,$5,$6,$7)`, workspace, history, caseID, d.At, d.Reason, oldState, d.State)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,$5,$6,$7)`, workspace, history, d.Evidence.ID, d.Evidence.Digest, d.Evidence.Bytes, d.At, d.Evidence.ExpiresAt)
	if e != nil {
		return e
	}
	return nil
}
