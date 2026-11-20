// Package receipt implements the Intake-owned PostgreSQL port, using exact DDL.
package receipt

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const leaseKey int64 = 0x4850494e54414b45

// All receipt publishers/cleanup in this dedicated Intake DB use this lease.
// It serializes baseline publication across processes, not a SQL transaction.
// Other writers must obey the same owner protocol. No throughput claim is made.
type Repository struct {
	pool  *pgxpool.Pool
	fault func(string) error
}
type session struct {
	repo *Repository
	conn *pgxpool.Conn
}

func New(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil || pool.Config().MaxConns > 10 {
		return nil, receivesignal.ErrUnavailable
	}
	return &Repository{pool: pool}, nil
}
func (r *Repository) WithSession(ctx context.Context, fn func(receivesignal.Session) error) error {
	if r == nil || r.pool == nil || ctx == nil || ctx.Err() != nil {
		return receivesignal.ErrUnavailable
	}
	leaseCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, e := r.pool.Acquire(leaseCtx)
	if e != nil {
		return receivesignal.ErrUnavailable
	}
	defer conn.Release()
	var ok bool
	e = conn.QueryRow(leaseCtx, `SELECT NOT rolsuper AND NOT rolbypassrls AND pg_has_role(current_user,'hp_intake_app','member') AND NOT pg_has_role(current_user,'hp_intake_migrator','member') FROM pg_roles WHERE rolname=current_user`).Scan(&ok)
	if e != nil || !ok {
		return receivesignal.ErrContext
	}
	if _, e = conn.Exec(leaseCtx, `SELECT pg_advisory_lock($1)`, leaseKey); e != nil {
		conn.Hijack().Close(context.Background())
		return receivesignal.ErrUnavailable
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var unlocked bool
		if e := conn.QueryRow(clean, `SELECT pg_advisory_unlock($1)`, leaseKey).Scan(&unlocked); e != nil || !unlocked {
			conn.Hijack().Close(clean)
		}
	}()
	return fn(&session{repo: r, conn: conn})
}
func (s *session) begin(ctx context.Context, c receivesignal.Connection) (pgx.Tx, error) {
	tx, e := s.conn.Begin(ctx)
	if e != nil {
		return nil, receivesignal.ErrUnavailable
	}
	fail := func() (pgx.Tx, error) { rollback(tx); return nil, receivesignal.ErrContext }
	if _, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true),set_config('statement_timeout','2000',true),set_config('lock_timeout','2000',true),set_config('transaction_timeout','2000',true)`, c.Binding.WorkspaceID); e != nil {
		return fail()
	}
	if c.Binding.Livemode == nil || c.SecurityEpoch <= 0 {
		return fail()
	}
	var found bool
	e = tx.QueryRow(ctx, `SELECT true FROM hp_intake.source_connections WHERE workspace_id=$1 AND connection_id=$2 AND account_id=$3 AND livemode=$4 AND security_epoch=$5 AND health='READY' AND revoked_at IS NULL AND verified_at IS NOT NULL FOR SHARE`, c.Binding.WorkspaceID, c.Binding.ConnectionID, c.Binding.StripeAccountID, *c.Binding.Livemode, c.SecurityEpoch).Scan(&found)
	if e != nil || !found {
		return fail()
	}
	return tx, nil
}
func (s *session) Lookup(ctx context.Context, c receivesignal.Connection, event string) (*receivesignal.Stored, error) {
	tx, e := s.begin(ctx, c)
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	var x receivesignal.Stored
	var first, expires, created time.Time
	var state string
	e = tx.QueryRow(ctx, `SELECT s.signal_id::text,s.workspace_id::text,s.connection_id::text,s.event_id,s.event_type,s.session_id,s.account_id,s.livemode,s.source_created_at,s.first_received_at,s.expires_at,o.message_id::text,o.digest,o.payload_ref::text,o.payload_sha256,o.payload_bytes,s.state FROM hp_intake.signals s LEFT JOIN hp_intake.outbox o ON o.workspace_id=s.workspace_id AND o.message_id=s.signal_id AND o.kind='NORMALIZED_SIGNAL' AND o.created_at=s.first_received_at AND o.expires_at=s.expires_at AND o.payload_bytes=s.serialized_bytes WHERE s.workspace_id=$1 AND s.connection_id=$2 AND s.event_id=$3`, c.Binding.WorkspaceID, c.Binding.ConnectionID, event).Scan(&x.Signal.ReceiptID, &x.Signal.WorkspaceID, &x.Signal.ConnectionID, &x.Signal.EventID, &x.Signal.EventType, &x.Signal.SessionID, &x.Signal.StripeAccountID, &x.Signal.Livemode, &created, &first, &expires, &x.Signal.MessageID, &x.Signal.MessageDigest, &x.Content.ID, &x.Content.Digest, &x.Content.Bytes, &state)
	if errors.Is(e, pgx.ErrNoRows) {
		if e = tx.Commit(ctx); e != nil {
			return nil, receivesignal.ErrUnavailable
		}
		return nil, nil
	}
	if e != nil {
		return nil, receivesignal.ErrIncomplete
	}
	if state == "EXPIRED" {
		return nil, receivesignal.ErrIncomplete
	}
	x.Signal.SchemaVersion = "1.0"
	x.Signal.SourceCreatedAt = created.UTC().Format(time.RFC3339Nano)
	x.Signal.FirstReceivedAt = first.UTC().Format(time.RFC3339Nano)
	x.Signal.ExpiresAt = expires.UTC().Format(time.RFC3339Nano)
	x.Content.ExpiresAt = expires
	if e = tx.Commit(ctx); e != nil {
		return nil, receivesignal.ErrUnavailable
	}
	return &x, nil
}
func (s *session) Insert(ctx context.Context, c receivesignal.Connection, x receivesignal.Stored) error {
	tx, e := s.begin(ctx, c)
	if e != nil {
		return e
	}
	defer rollback(tx)
	sig := x.Signal
	_, e = tx.Exec(ctx, `INSERT INTO hp_intake.signals(workspace_id,signal_id,connection_id,event_id,event_type,session_id,account_id,livemode,source_created_at,first_received_at,expires_at,serialized_bytes,state,reason_code) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'RECEIVED','VERIFIED_SIGNAL')`, sig.WorkspaceID, sig.ReceiptID, sig.ConnectionID, sig.EventID, sig.EventType, sig.SessionID, sig.StripeAccountID, sig.Livemode, parse(sig.SourceCreatedAt), parse(sig.FirstReceivedAt), parse(sig.ExpiresAt), x.Content.Bytes)
	if e != nil {
		return safe(e)
	}
	if s.repo.fault != nil && s.repo.fault("after_signal") != nil {
		return receivesignal.ErrUnavailable
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_intake.outbox(workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at) VALUES($1,$2,'NORMALIZED_SIGNAL',$3,$4,$5,$6,$7,$7,$8)`, sig.WorkspaceID, sig.MessageID, x.Content.ID, x.Content.Digest, x.Content.Bytes, sig.MessageDigest, parse(sig.FirstReceivedAt), parse(sig.ExpiresAt))
	if e != nil {
		return safe(e)
	}
	if s.repo.fault != nil && s.repo.fault("before_commit") != nil {
		return receivesignal.ErrUnavailable
	}
	if e = tx.Commit(ctx); e != nil {
		return receivesignal.ErrUncertain
	}
	if s.repo.fault != nil && s.repo.fault("after_commit") != nil {
		return receivesignal.ErrUncertain
	}
	return nil
}
func parse(s string) time.Time { t, _ := time.Parse(time.RFC3339Nano, s); return t }
func safe(e error) error {
	var p *pgconn.PgError
	if errors.As(e, &p) {
		if p.Code == "23514" && p.TableName == "ingress_quota" {
			return receivesignal.ErrQuota
		}
		if p.Code == "23505" {
			return receivesignal.ErrConflict
		}
	}
	return receivesignal.ErrUnavailable
}
func (s *session) References(ctx context.Context, c receivesignal.Connection) (map[string]bool, error) {
	tx, e := s.begin(ctx, c)
	if e != nil {
		return nil, e
	}
	defer rollback(tx)
	rows, e := tx.Query(ctx, `SELECT payload_ref::text FROM hp_intake.outbox WHERE workspace_id=$1`, c.Binding.WorkspaceID)
	if e != nil {
		return nil, receivesignal.ErrUnavailable
	}
	refs := map[string]bool{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			return nil, receivesignal.ErrUnavailable
		}
		refs[id] = true
	}
	if rows.Err() != nil {
		return nil, receivesignal.ErrUnavailable
	}
	rows.Close()
	if tx.Commit(ctx) != nil {
		return nil, receivesignal.ErrUnavailable
	}
	return refs, nil
}

func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
