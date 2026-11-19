// Package outbox uses only the existing Intake worker role and scoped tables.
package outbox

import (
	"context"
	"errors"
	"time"

	app "github.com/hitpipe/hitpipe/services/intake/internal/application/sendoutbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Repository, error) {
	if pool == nil || pool.Config().MaxConns > 10 {
		return nil, app.ErrUnavailable
	}
	return &Repository{pool}, nil
}

// Each callback ends before the caller does any filesystem or network work.
func (r *Repository) scoped(ctx context.Context, s app.Scope, fn func(context.Context, pgx.Tx) error) error {
	if !app.ValidScope(s) {
		return app.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return app.ErrUnavailable
	}
	defer func() {
		c, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		_ = tx.Rollback(c)
	}()
	var authorized bool
	err = tx.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls
		AND pg_has_role(current_user,'hp_intake_worker','member')
		AND NOT pg_has_role(current_user,'hp_intake_migrator','member')
		AND NOT pg_has_role(current_user,'hp_intake_purge','member')
		FROM pg_roles WHERE rolname=current_user`).Scan(&authorized)
	if err != nil {
		return app.ErrUnavailable
	}
	if !authorized {
		return app.ErrDenied
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true),
		set_config('statement_timeout','2000',true),set_config('lock_timeout','2000',true),
		set_config('transaction_timeout','2000',true)`, s.Workspace); err != nil {
		return app.ErrUnavailable
	}
	if err = fn(ctx, tx); err != nil {
		return err
	}
	if tx.Commit(ctx) != nil {
		return app.ErrUnavailable
	}
	return nil
}

func (r *Repository) Claim(ctx context.Context, s app.Scope, lease time.Duration, cutoff time.Time) (app.Attempt, bool, error) {
	var a app.Attempt
	var found bool
	if lease < time.Millisecond || lease > 72*time.Hour || cutoff.IsZero() {
		return a, false, app.ErrUnavailable
	}
	err := r.scoped(ctx, s, func(ctx context.Context, tx pgx.Tx) error {
		var created, first, expires time.Time
		err := tx.QueryRow(ctx, `
			WITH picked AS (
				SELECT o.workspace_id,o.message_id,s.connection_id,s.signal_id,s.account_id,
					s.livemode,s.session_id,s.event_id,s.event_type,s.source_created_at,
					s.first_received_at,s.expires_at
				FROM hp_intake.outbox o
				JOIN hp_intake.signals s ON (s.workspace_id,s.signal_id)=(o.workspace_id,o.message_id)
				JOIN hp_intake.source_connections c ON (c.workspace_id,c.connection_id)=(s.workspace_id,s.connection_id)
				WHERE o.workspace_id=$1 AND s.connection_id=$2 AND c.account_id=$3 AND c.livemode=$4
					AND s.account_id=$3 AND s.livemode=$4 AND c.security_epoch=$5
					AND c.health='READY' AND c.revoked_at IS NULL
					AND o.kind='NORMALIZED_SIGNAL' AND o.next_attempt_at<=LEAST(clock_timestamp(),$7::timestamptz)
					AND (o.state='PENDING' OR (o.state='CLAIMED' AND o.lease_until<=clock_timestamp()))
					AND o.delivery_count<2147483647
				ORDER BY o.next_attempt_at,o.message_id LIMIT 1 FOR UPDATE OF o SKIP LOCKED
			)
			UPDATE hp_intake.outbox o SET state='CLAIMED',claim_token=gen_random_uuid(),
				lease_until=clock_timestamp()+($6::bigint*interval '1 millisecond'),delivery_count=o.delivery_count+1
			FROM picked p WHERE (o.workspace_id,o.message_id)=(p.workspace_id,p.message_id)
			RETURNING o.workspace_id::text,o.message_id::text,p.signal_id::text,p.connection_id::text,
				p.account_id,p.livemode,p.session_id,p.event_id,p.event_type,p.source_created_at,
				p.first_received_at,p.expires_at,o.digest,o.payload_ref::text,o.payload_sha256,
				o.payload_bytes,o.expires_at,o.claim_token::text,o.lease_until,o.delivery_count
		`, s.Workspace, s.Connection, s.Account, s.Livemode, s.Epoch, lease.Milliseconds(), cutoff).Scan(
			&a.Signal.WorkspaceID, &a.Signal.MessageID, &a.Signal.ReceiptID, &a.Signal.ConnectionID,
			&a.Signal.StripeAccountID, &a.Signal.Livemode, &a.Signal.SessionID, &a.Signal.EventID, &a.Signal.EventType,
			&created, &first, &expires, &a.Signal.MessageDigest, &a.Content.ID, &a.Content.Digest,
			&a.Content.Bytes, &a.Content.ExpiresAt, &a.Token, &a.Until, &a.Number)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return app.ErrUnavailable
		}
		a.Signal.SchemaVersion = "1.0"
		a.Signal.SourceCreatedAt = wireTime(created)
		a.Signal.FirstReceivedAt = wireTime(first)
		a.Signal.ExpiresAt = wireTime(expires)
		found = true
		return nil
	})
	if err != nil {
		return app.Attempt{}, false, err
	}
	return a, found, nil
}

func (r *Repository) Live(ctx context.Context, s app.Scope, a app.Attempt) error {
	if !validAttempt(s, a) {
		return app.ErrStale
	}
	return r.scoped(ctx, s, func(ctx context.Context, tx pgx.Tx) error {
		var ok bool
		err := tx.QueryRow(ctx, `
			SELECT true FROM hp_intake.outbox o
			JOIN hp_intake.signals s ON (s.workspace_id,s.signal_id)=(o.workspace_id,o.message_id)
			JOIN hp_intake.source_connections c ON (c.workspace_id,c.connection_id)=(s.workspace_id,s.connection_id)
			WHERE o.workspace_id=$1 AND s.connection_id=$2 AND c.account_id=$3 AND c.livemode=$4
				AND s.account_id=$3 AND s.livemode=$4 AND c.security_epoch=$5
				AND c.health='READY' AND c.revoked_at IS NULL AND s.state<>'EXPIRED'
				AND o.message_id=$6 AND o.state='CLAIMED' AND o.claim_token=$7
				AND o.lease_until>clock_timestamp() AND o.expires_at>clock_timestamp()
				AND o.payload_ref=$8 AND o.payload_sha256=$9 AND o.payload_bytes=$10 AND o.digest=$11
				AND o.kind='NORMALIZED_SIGNAL' AND o.created_at=s.first_received_at
				AND o.expires_at=s.expires_at AND o.expires_at=$12 AND o.payload_bytes=s.serialized_bytes
		`, s.Workspace, s.Connection, s.Account, s.Livemode, s.Epoch, a.Signal.MessageID, a.Token,
			a.Content.ID, a.Content.Digest, a.Content.Bytes, a.Signal.MessageDigest, a.Content.ExpiresAt).Scan(&ok)
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrStale
		}
		if err != nil {
			return app.ErrUnavailable
		}
		return nil
	})
}

func (r *Repository) Finish(ctx context.Context, s app.Scope, a app.Attempt, state app.Outcome, due time.Time) error {
	if !validAttempt(s, a) {
		return app.ErrStale
	}
	switch state {
	case app.Acked, app.Held:
	case app.Pending:
		if due.IsZero() || !due.Before(a.Content.ExpiresAt) {
			return app.ErrStale
		}
	default:
		return app.ErrUnavailable
	}
	return r.scoped(ctx, s, func(ctx context.Context, tx pgx.Tx) error {
		// Finishing an already-started attempt only records its bounded outcome.
		// It authorizes no new read or network start. The lease is checked again
		// by PostgreSQL under the UPDATE row lock.
		tag, err := tx.Exec(ctx, `
			UPDATE hp_intake.outbox o SET state=$4,claim_token=NULL,lease_until=NULL,
				next_attempt_at=CASE WHEN $4='PENDING' THEN $5 ELSE o.next_attempt_at END
			WHERE o.workspace_id=$1 AND o.message_id=$2 AND o.state='CLAIMED' AND o.claim_token=$3
				AND o.lease_until>clock_timestamp() AND o.kind='NORMALIZED_SIGNAL'
				AND ($4<>'PENDING' OR $5<o.expires_at)
				AND o.payload_ref=$7 AND o.payload_sha256=$8 AND o.payload_bytes=$9 AND o.digest=$10
				AND o.expires_at=$11 AND EXISTS (SELECT 1 FROM hp_intake.signals s
					WHERE (s.workspace_id,s.signal_id)=(o.workspace_id,o.message_id) AND s.connection_id=$6)
		`, s.Workspace, a.Signal.MessageID, a.Token, string(state), due, s.Connection,
			a.Content.ID, a.Content.Digest, a.Content.Bytes, a.Signal.MessageDigest, a.Content.ExpiresAt)
		if err != nil {
			return app.ErrUnavailable
		}
		if tag.RowsAffected() != 1 {
			return app.ErrStale
		}
		return nil
	})
}

func validAttempt(s app.Scope, a app.Attempt) bool {
	return app.ValidScope(s) && app.ValidUUID(a.Token) && app.ValidUUID(a.Signal.MessageID) &&
		a.Signal.WorkspaceID == s.Workspace && a.Signal.ConnectionID == s.Connection &&
		a.Signal.StripeAccountID == s.Account && a.Signal.Livemode == s.Livemode
}

func wireTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
