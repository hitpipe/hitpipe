// Package postgres contains Processing-owned transactional storage primitives.
package postgres

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
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
func record(ctx context.Context, tx pgx.Tx, s admission.Signal, operation, config, owner string) (recordResult, error) {
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
	tag, e = tx.Exec(ctx, `INSERT INTO hp_processing.case_identity
 (workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge)
 VALUES($1,$2,$3,$4,$5,$6,$7,clock_timestamp(),$8,'RECEIVED',1,'UNESTABLISHED')
 ON CONFLICT(workspace_id,source_account_id,livemode,session_id) DO NOTHING`, s.WorkspaceID, id, operation, config, s.AccountID, s.Live, s.SessionID, owner)
	if e != nil {
		return zero, e
	}
	if tag.RowsAffected() == 1 {
		history, e := newID()
		if e != nil {
			return zero, e
		}
		_, e = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state,proof_ref)
  VALUES($1,$2,$3,'SYSTEM',clock_timestamp(),'SIGNAL_ADMITTED','RECEIVED',$4)`, s.WorkspaceID, history, id, s.ReceiptID)
		if e != nil {
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
