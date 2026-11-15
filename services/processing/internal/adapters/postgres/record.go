// Package postgres contains Processing-owned transactional storage primitives.
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

var errConflict = admission.ErrConflict

// recordResult is staged, private state. Only Store.Accept may acknowledge it
// after successful commit and checks of the established context/local operation.
type recordResult struct{ Disposition, RecordID string }

func newID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

// record stages persistence in the caller-owned, workspace-scoped transaction.
// PRECONDITIONS: validated signal, established binding/authority, selected and
// locked accepted operation/config, and independently verified activation boundary.
// It never commits and its result MUST NOT be acknowledged before successful
// caller commit. This private primitive is not a substitute for those checks.
func record(ctx context.Context, tx pgx.Tx, s admission.Signal, operation, config, owner string, content preparecase.Content, retention time.Duration, operationRevision int64) (recordResult, error) {
	zero := recordResult{}
	tag, e := tx.Exec(ctx, `INSERT INTO hp_processing.inbox(workspace_id,message_id,digest,disposition)
 VALUES($1,$2,$3,'APPLIED') ON CONFLICT(workspace_id,message_id) DO NOTHING`, s.WorkspaceID, s.MessageID, s.Digest)
	if e != nil {
		return zero, e
	}
	if tag.RowsAffected() == 0 {
		var digest string
		var r recordResult
		e = tx.QueryRow(ctx, `SELECT digest,disposition,record_id::text FROM hp_processing.inbox WHERE workspace_id=$1 AND message_id=$2`, s.WorkspaceID, s.MessageID).Scan(&digest, &r.Disposition, &r.RecordID)
		if e != nil {
			return zero, e
		}
		if digest != s.Digest {
			return zero, errConflict
		}
		return r, nil
	}
	id, e := newID()
	if e != nil {
		return zero, e
	}
	if content == nil || retention <= 0 || retention > 90*24*time.Hour {
		return zero, admission.ErrNotAdmissible
	}
	var at time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return zero, e
	}
	at = at.UTC()
	tag, e = tx.Exec(ctx, `INSERT INTO hp_processing.case_identity
 (workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge)
 VALUES($1,$2,$3,$4,$5,$6,$7,$9,$8,'RECEIVED',1,'UNESTABLISHED')
 ON CONFLICT(workspace_id,source_account_id,livemode,session_id) DO NOTHING`, s.WorkspaceID, id, operation, config, s.AccountID, s.Live, s.SessionID, owner, at)
	if e != nil {
		return zero, e
	}
	if tag.RowsAffected() == 1 {
		history, e := newID()
		if e != nil {
			return zero, e
		}
		// Same HISTORY envelope/content protocol as HIT-79. The one actual DB time
		// is used for admission, typed event, content creation and fixed expiry.
		body, e := json.Marshal(struct {
			CaseID            string
			Revision          int64
			OldState          string
			Scope             preparecase.Scope
			OperationRevision int64
			Decision          preparecase.Decision
		}{id, 1, "", preparecase.Scope{Request: preparecase.Request{WorkspaceID: s.WorkspaceID, CaseID: id}, OperationID: operation, ConfigID: config, SourceConnectionID: s.ConnectionID, OwnerID: owner}, operationRevision, preparecase.Decision{Reason: "SIGNAL_ADMITTED", State: "RECEIVED", At: at}})
		if e != nil {
			return zero, e
		}
		ref, e := content.Write(ctx, body, "HISTORY", preparecase.Origin{WorkspaceID: s.WorkspaceID, CaseID: id, Revision: 1}, at.Add(retention), at)
		if e != nil {
			return zero, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state,proof_ref)
  VALUES($1,$2,$3,'SYSTEM',$5,'SIGNAL_ADMITTED','RECEIVED',$4)`, s.WorkspaceID, history, id, s.ReceiptID, at)
		if e != nil {
			return zero, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO hp_processing.history_content(workspace_id,entry_id,detail_ref,detail_sha256,detail_bytes,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, s.WorkspaceID, history, ref.ID, ref.Digest, ref.Bytes, at, ref.ExpiresAt); e != nil {
			return zero, e
		}
	} else {
		// READ COMMITTED sees the winning concurrent INSERT after ON CONFLICT waits.
		e = tx.QueryRow(ctx, `SELECT case_id::text FROM hp_processing.case_identity
  WHERE workspace_id=$1 AND source_account_id=$2 AND livemode=$3 AND session_id=$4`, s.WorkspaceID, s.AccountID, s.Live, s.SessionID).Scan(&id)
		if e != nil {
			return zero, e
		}
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.inbox SET record_id=$3 WHERE workspace_id=$1 AND message_id=$2`, s.WorkspaceID, s.MessageID, id)
	if e != nil {
		return zero, e
	}
	return recordResult{"APPLIED", id}, nil
}
