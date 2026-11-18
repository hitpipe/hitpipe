// Package facts uses only the unchanged hp_intake schema and constrained app role.
package facts

import (
	"context"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

type Repository struct{ pool *pgxpool.Pool }

func New(ctx context.Context, dsn string) (*Repository, error) {
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, app.Fail("database")
	}
	cfg.MaxConns = 10
	cfg.MinConns = 0
	cfg.ConnConfig.RuntimeParams["search_path"] = "hp_intake,pg_catalog"
	p, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, app.Fail("database")
	}
	var allowed bool
	e = p.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT pg_has_role(current_user,'hp_intake_migrator','MEMBER') AND pg_has_role(current_user,'hp_intake_app','MEMBER') FROM pg_roles WHERE rolname=current_user`).Scan(&allowed)
	if e != nil || !allowed {
		p.Close()
		return nil, app.Fail("database")
	}
	return &Repository{p}, nil
}
func (r *Repository) Close() { r.pool.Close() }
func (r *Repository) tx(ctx context.Context, b app.Binding) (pgx.Tx, error) {
	if !sf.UUID(b.WorkspaceID) || b.LiveMode == nil {
		return nil, app.Fail("context")
	}
	tx, e := r.pool.Begin(ctx)
	if e != nil {
		return nil, app.Fail("database")
	}
	_, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true),set_config('statement_timeout','2s',true),set_config('lock_timeout','2s',true),set_config('transaction_timeout','2s',true)`, b.WorkspaceID)
	if e == nil {
		e = connection(ctx, tx, b)
	}
	if e != nil {
		tx.Rollback(ctx)
		return nil, app.Fail("context")
	}
	return tx, nil
}
func connection(ctx context.Context, tx pgx.Tx, b app.Binding) error {
	var ok bool
	e := tx.QueryRow(ctx, `SELECT account_id=$3 AND livemode=$4 AND security_epoch=$5 AND health='READY' AND verified_at IS NOT NULL AND revoked_at IS NULL FROM hp_intake.source_connections WHERE workspace_id=$1 AND connection_id=$2 FOR SHARE`, b.WorkspaceID, b.ConnectionID, b.AccountID, *b.LiveMode, b.Epoch).Scan(&ok)
	if e != nil || !ok {
		return app.Fail("context")
	}
	return nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func (r *Repository) Check(ctx context.Context, b app.Binding) error {
	tx, e := r.tx(ctx, b)
	if e != nil {
		return e
	}
	defer rollback(tx)
	if tx.Commit(ctx) != nil {
		return app.Fail("database")
	}
	return nil
}
func (r *Repository) Save(ctx context.Context, b app.Binding, rec app.Record) error {
	tx, e := r.tx(ctx, b)
	if e != nil {
		return e
	}
	defer rollback(tx)
	f, ref := rec.Facts, rec.Ref
	if f.ConnectionID != b.ConnectionID || f.StripeAccountID != b.AccountID || f.Livemode != *b.LiveMode {
		return app.Fail("context")
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_intake.source_facts(workspace_id,facts_id,case_id,connection_id,facts_ref,facts_bytes,digest,complete,oldest_observed_at,completed_at,valid_until,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, b.WorkspaceID, f.FactsID, f.CaseID, f.ConnectionID, ref.ID, ref.Bytes, ref.Digest, f.Complete, f.OldestObservedAt, f.CompletedAt, f.ValidUntil, ref.ExpiresAt)
	if e != nil {
		return app.Fail("database")
	}
	if tx.Commit(ctx) != nil {
		return app.Fail("database")
	}
	return nil
}
func (r *Repository) Load(ctx context.Context, b app.Binding, id string) (app.Record, error) {
	tx, e := r.tx(ctx, b)
	if e != nil {
		return app.Record{}, e
	}
	defer rollback(tx)
	var rec app.Record
	e = tx.QueryRow(ctx, `SELECT facts_id::text,case_id::text,connection_id::text,facts_ref::text,facts_bytes,digest,complete,oldest_observed_at,completed_at,valid_until,expires_at FROM hp_intake.source_facts WHERE workspace_id=$1 AND facts_id=$2`, b.WorkspaceID, id).Scan(&rec.Facts.FactsID, &rec.Facts.CaseID, &rec.Facts.ConnectionID, &rec.Ref.ID, &rec.Ref.Bytes, &rec.Ref.Digest, &rec.Facts.Complete, &rec.Facts.OldestObservedAt, &rec.Facts.CompletedAt, &rec.Facts.ValidUntil, &rec.Ref.ExpiresAt)
	if e != nil {
		return app.Record{}, app.Fail("database")
	}
	if tx.Commit(ctx) != nil {
		return app.Record{}, app.Fail("database")
	}
	return rec, nil
}
