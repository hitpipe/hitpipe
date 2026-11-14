package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/jackc/pgx/v5"
)

type Beginner interface {
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}

// Store requires an explicitly configured Processing application-role connection
// source. It never creates databases, installs bindings or activates operations.
type Store struct{ DB Beginner }

func (db Store) Accept(ctx context.Context, s admission.Signal, c admission.EstablishedContext) (admission.Receipt, error) {
	zero := admission.Receipt{}
	if e := c.Check(s); e != nil {
		return zero, e
	}
	if db.DB == nil {
		return zero, admission.ErrNotAdmissible
	}
	tx, e := db.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if e != nil {
		return zero, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, c.WorkspaceID); e != nil {
		return zero, e
	}
	// A previously committed message retains its original result after lifecycle
	// changes. No new permission or business state is created on this path.
	var digest, disposition, recordID string
	e = tx.QueryRow(ctx, `SELECT digest,disposition,record_id::text FROM hp_processing.inbox WHERE workspace_id=$1 AND message_id=$2`, s.WorkspaceID, s.MessageID).Scan(&digest, &disposition, &recordID)
	if e == nil {
		if digest != s.Digest {
			return zero, admission.ErrConflict
		}
		if e = tx.Commit(ctx); e != nil {
			return zero, e
		}
		return admission.Receipt{MessageID: s.MessageID, Disposition: disposition, RecordID: recordID}, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return zero, e
	}
	// Lock the operation against concurrent lifecycle/CAS updates. Accepted config
	// is immutable to the application role; both FKs are enforced by the schema.
	var state, config, owner string
	var revision int64
	var first *time.Time
	e = tx.QueryRow(ctx, `SELECT o.state,o.selected_config_id::text,o.owner_user_id::text,o.revision,o.first_activation_at
 FROM hp_processing.operations o JOIN hp_processing.accepted_configs a
 ON (a.workspace_id,a.operation_id,a.config_id)=(o.workspace_id,o.operation_id,o.selected_config_id)
 WHERE o.workspace_id=$1 AND o.operation_id=$2 FOR UPDATE OF o`, c.WorkspaceID, c.OperationID).Scan(&state, &config, &owner, &revision, &first)
	if errors.Is(e, pgx.ErrNoRows) {
		return zero, admission.ErrNotAdmissible
	}
	if e != nil {
		return zero, e
	}
	// source_created_at is Session.created from Event.data.object.created, NOT
	// Event.created: Lead decision HIT-78 comment 12171 / HIT-73 comment 12172.
	if state != "ACTIVE" || first == nil || revision != c.ExpectedRevision || s.SourceCreatedAt.Before(*first) {
		return zero, admission.ErrNotAdmissible
	}
	r, e := record(ctx, tx, s, c.OperationID, config, owner)
	if e != nil {
		return zero, e
	}
	if e = tx.Commit(ctx); e != nil {
		return zero, e
	}
	return admission.Receipt{MessageID: s.MessageID, Disposition: r.Disposition, RecordID: r.RecordID}, nil
}
