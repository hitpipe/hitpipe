package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

var ErrPurgeScope = errors.New("processing purge role, schema or workspace rejected")

type PurgeResult struct {
	Held    int `json:"held"`
	Removed int `json:"removed"`
}

type PurgeStore struct {
	DB    Beginner
	Files *contentfiles.Maintenance
}

func (s PurgeStore) begin(ctx context.Context, workspace string) (pgx.Tx, error) {
	tx, err := scoped(ctx, s.DB, workspace)
	if err != nil {
		return nil, err
	}
	var allowed bool
	err = tx.QueryRow(ctx, `SELECT current_setting('server_version_num')='180007'
 AND current_user=session_user AND NOT rolsuper AND NOT rolbypassrls AND NOT rolcreaterole
 AND NOT rolcreatedb AND NOT rolreplication AND pg_has_role(current_user,'hp_processing_purge','USAGE')
 AND NOT EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname <> current_user
   AND r.rolname <> 'hp_processing_purge' AND pg_has_role(current_user,r.oid,'MEMBER'))
 AND row_security_active('hp_processing.drafts') AND row_security_active('hp_processing.case_content')
 AND row_security_active('hp_processing.safety_history') AND row_security_active('hp_processing.history_content')
 AND row_security_active('hp_processing.outbox')
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_processing.schema_version)
 FROM pg_roles WHERE rolname=current_user`).Scan(&allowed)
	if err != nil || !allowed {
		_ = tx.Rollback(ctx)
		return nil, ErrPurgeScope
	}
	return tx, nil
}

func (s PurgeStore) Run(ctx context.Context, workspace string, limit int) (PurgeResult, error) {
	var result PurgeResult
	if !readcases.UUID(workspace) || limit < 1 || limit > contentfiles.MaxPurgeBatch || s.DB == nil || s.Files == nil {
		return result, ErrPurgeScope
	}
	tx, err := s.begin(ctx, workspace)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	var cutoff time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&cutoff); err != nil {
		return result, err
	}
	tag, err := tx.Exec(ctx, `WITH expired AS (
 SELECT workspace_id,message_id FROM hp_processing.outbox
 WHERE workspace_id=$1 AND expires_at<=$2 AND state IN ('PENDING','CLAIMED')
 ORDER BY expires_at,message_id LIMIT $3 FOR UPDATE SKIP LOCKED
)
UPDATE hp_processing.outbox o SET state='HELD',claim_token=NULL,lease_until=NULL
FROM expired e WHERE (o.workspace_id,o.message_id)=(e.workspace_id,e.message_id)`, workspace, cutoff, limit)
	if err != nil {
		return result, err
	}
	// An interrupted hold commit must never be reported as success or followed
	// by file deletion. A fresh run can resolve the durable state from SQL.
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Held = int(tag.RowsAffected())
	result.Removed, err = s.remove(ctx, workspace, cutoff, limit)
	return result, err
}

func (s PurgeStore) remove(ctx context.Context, workspace string, cutoff time.Time, limit int) (int, error) {
	tx, err := s.begin(ctx, workspace)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	// The volume lease excludes Processing writers across file and SQL commit.
	// This additionally excludes SQL writes to all known direct ref sources
	// while checking and unlinking. Prefer a busy error over waiting.
	_, err = tx.Exec(ctx, `LOCK TABLE hp_processing.case_content, hp_processing.drafts,
 hp_processing.history_content, hp_processing.outbox, hp_processing.safety_history IN SHARE MODE NOWAIT`)
	if err != nil {
		return 0, err
	}
	removed, err := s.Files.Sweep(ctx, workspace, cutoff, limit, func(id string) (bool, error) {
		var unprotected bool
		err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
 SELECT 1 FROM hp_processing.case_content WHERE workspace_id=$1 AND value_ref=$2 AND expires_at>$3
 UNION ALL
 SELECT 1 FROM hp_processing.drafts WHERE workspace_id=$1 AND prose_ref=$2
 AND (prose_expires_at IS NULL OR prose_expires_at>$3 OR expires_at>$3)
 UNION ALL
 SELECT 1 FROM hp_processing.history_content WHERE workspace_id=$1 AND detail_ref=$2 AND expires_at>$3
 UNION ALL
 SELECT 1 FROM hp_processing.outbox WHERE workspace_id=$1 AND payload_ref=$2
 AND (expires_at>$3 OR state IN ('PENDING','CLAIMED'))
 UNION ALL
 SELECT 1 FROM hp_processing.safety_history h WHERE h.workspace_id=$1 AND h.proof_ref=$2
 AND NOT EXISTS(SELECT 1 FROM hp_processing.history_content c
   WHERE (c.workspace_id,c.entry_id)=(h.workspace_id,h.entry_id)
   AND c.detail_ref=h.proof_ref AND c.expires_at<=$3)
)`, workspace, id, cutoff).Scan(&unprotected)
		return unprotected, err
	})
	if err != nil {
		return removed, err
	}
	return removed, tx.Commit(ctx)
}
