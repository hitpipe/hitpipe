package intake

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ContentRef struct {
	ID      uuid.UUID
	SHA256  string
	Size    int
	Expires time.Time
}

type ContentStore interface {
	Put(data []byte, expires time.Time) (ContentRef, error)
}

type Postgres struct {
	pool    *pgxpool.Pool
	content ContentStore
}

func NewPostgres(pool *pgxpool.Pool, content ContentStore) (*Postgres, error) {
	if pool == nil || content == nil || pool.Config().MaxConns > 10 {
		return nil, ErrUnavailable
	}
	return &Postgres{pool: pool, content: content}, nil
}

func (p *Postgres) Accept(ctx context.Context, i Intent, now func() time.Time) (Receipt, error) {
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `SELECT set_config('hp.workspace_id', $1, true),
	    set_config('synchronous_commit', 'on', true), set_config('TimeZone', 'UTC', true)`, i.WorkspaceID.String()); err != nil {
		return Receipt{}, ErrUnavailable
	}

	var external string
	var epoch int64
	var revoked bool
	err = tx.QueryRow(ctx, `
		SELECT external_workspace_id, security_epoch, revoked_at IS NOT NULL
		FROM hp_delivery.destination_connections WHERE workspace_id=$1 AND connection_id=$2
		FOR SHARE`, i.WorkspaceID, i.ConnectionID).Scan(&external, &epoch, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, ErrDenied
	}
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if revoked || epoch != i.Epoch || external != i.DestinationWorkspace {
		return Receipt{}, ErrDenied
	}

	attempt, err := uuid.NewRandom()
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	inserted, err := tx.Exec(ctx, `
		INSERT INTO hp_delivery.inbox (workspace_id,message_id,digest,disposition,record_id)
		VALUES ($1,$2,$3,'APPLIED',$4)
		ON CONFLICT (workspace_id,message_id) DO NOTHING`,
		i.WorkspaceID, i.MessageID, i.MessageDigest, attempt)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if inserted.RowsAffected() == 0 {
		receipt, err := readReceipt(ctx, tx, i)
		if err != nil {
			return Receipt{}, err
		}
		if tx.Commit(ctx) != nil {
			return Receipt{}, ErrUnavailable
		}
		return receipt, nil
	}

	created := now().UTC().Truncate(time.Microsecond)
	contentExpiry := created.Add(90 * 24 * time.Hour)
	if !i.ContentUntil.IsZero() && i.ContentUntil.Before(contentExpiry) {
		contentExpiry = i.ContentUntil.UTC().Truncate(time.Microsecond)
	}
	if !fresh(i, created, contentExpiry) {
		return Receipt{}, ErrStale
	}
	lineage, err := tx.Exec(ctx, `
		INSERT INTO hp_delivery.effect_lineage (workspace_id,lineage_id,case_id,connection_id,effect_kind,config_id,state,updated_at)
		VALUES ($1,$2,$3,$4,'CREATE_TASK',$5,'PREPARED',$6) ON CONFLICT DO NOTHING`,
		i.WorkspaceID, i.LineageID, i.CaseID, i.ConnectionID, i.ConfigID, created)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if lineage.RowsAffected() != 1 {
		return Receipt{}, ErrConflict
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO hp_delivery.attempts (workspace_id,attempt_id,lineage_id,episode_id,attempt_number,command_message_id,
		    source_facts_digest,facts_valid_until,episode_deadline_at,connection_epoch,prepared_at)
		VALUES ($1,$2,$3,$4,1,$5,$6,$7,$8,$9,$10)`,
		i.WorkspaceID, attempt, i.LineageID, i.EpisodeID, i.MessageID,
		i.FactsDigest, i.FactsUntil, i.Deadline, i.Epoch, created)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}

	ref, err := p.content.Put(i.Content, contentExpiry)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if ref.ID == uuid.Nil || ref.SHA256 != digest(i.Content) ||
		ref.Size != len(i.Content) || !ref.Expires.Equal(contentExpiry) {
		return Receipt{}, ErrUnavailable
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO hp_delivery.intent_content (workspace_id,attempt_id,rendered_payload_ref,rendered_payload_sha256,
		    rendered_payload_bytes,created_at,expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		i.WorkspaceID, attempt, ref.ID, ref.SHA256, ref.Size, created, ref.Expires)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if ctx.Err() != nil {
		return Receipt{}, ErrUnavailable
	}
	if !fresh(i, now(), contentExpiry) {
		return Receipt{}, ErrStale
	}
	// When Commit cannot confirm the outcome, never remove the file. The
	// server might have committed the reference before the connection broke.
	if tx.Commit(ctx) != nil {
		return Receipt{}, ErrUnavailable
	}
	return Receipt{MessageID: i.MessageID, Disposition: "APPLIED", RecordID: attempt}, nil
}

func fresh(i Intent, now, contentExpiry time.Time) bool {
	return now.Before(i.FactsUntil) && now.Before(i.Deadline) && now.Before(contentExpiry)
}

func readReceipt(ctx context.Context, tx pgx.Tx, i Intent) (Receipt, error) {
	var sum, disposition string
	var record uuid.NullUUID
	err := tx.QueryRow(ctx, `
		SELECT digest,disposition,record_id FROM hp_delivery.inbox
		WHERE workspace_id=$1 AND message_id=$2`, i.WorkspaceID, i.MessageID).Scan(&sum, &disposition, &record)
	if err != nil {
		return Receipt{}, ErrUnavailable
	}
	if sum != i.MessageDigest || disposition != "APPLIED" || !record.Valid {
		return Receipt{}, ErrConflict
	}
	return Receipt{MessageID: i.MessageID, Disposition: disposition, RecordID: record.UUID}, nil
}
