// Package sessionissuer persists a verified login's principal and session in the
// existing IAM schema. It exposes no public authentication or authorization API.
package sessionissuer

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConfiguration = errors.New("session_issuer_configuration_invalid")
	ErrInvalidRecord = errors.New("session_issuer_record_invalid")
	ErrDenied        = errors.New("session_issuer_denied")
	ErrUnavailable   = errors.New("session_issuer_unavailable")
)

const queryTimeout = 2 * time.Second

// The runtime login inherits only the IAM application role. An owner, worker,
// purge role, SET ROLE impersonation, elevated login or foreign database is not
// an application authority. This checks actual server state on every connection,
// not a DSN username or an administrator's SET ROLE simulation.
const checkAuthority = `SELECT current_user=session_user
 AND (SELECT rolcanlogin AND NOT(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication)
      FROM pg_roles WHERE rolname=current_user)
 AND pg_has_role(current_user,'hp_iam_app','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_worker','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_purge','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_migrator','MEMBER')
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname=current_user)
                AND roleid<>(SELECT oid FROM pg_roles WHERE rolname='hp_iam_app'))
 AND (SELECT count(*)=4 FROM pg_roles WHERE rolname IN('hp_iam_app','hp_iam_worker','hp_iam_purge','hp_iam_migrator')
      AND NOT(rolcanlogin OR rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication))
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_app'))
 AND NOT has_database_privilege(current_user,current_database(),'CREATE')
 AND NOT has_schema_privilege(current_user,'hp_iam','CREATE')
 AND EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='hp_iam' AND nspowner=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator'))
 AND (SELECT count(*)=17 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='hp_iam' AND c.relkind='r' AND c.relowner=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator'))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
                WHERE n.nspname='hp_iam' AND (c.relkind IN('p','f') OR c.relkind='r' AND c.relowner<>(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator')))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
                WHERE c.relkind IN('r','p','f') AND n.nspname NOT IN('hp_iam','pg_catalog','information_schema')
                AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%')
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_iam.schema_version)
 AND has_table_privilege(current_user,'hp_iam.principals','SELECT')
 AND has_table_privilege(current_user,'hp_iam.principals','INSERT')
 AND has_table_privilege(current_user,'hp_iam.principals','UPDATE')
 AND has_table_privilege(current_user,'hp_iam.sessions','SELECT')
 AND has_table_privilege(current_user,'hp_iam.sessions','INSERT')
 AND has_table_privilege(current_user,'hp_iam.sessions','UPDATE')
 AND has_table_privilege(current_user,'hp_iam.revocations','SELECT')
 AND NOT has_table_privilege(current_user,'hp_iam.principals','DELETE,TRUNCATE')
 AND NOT has_table_privilege(current_user,'hp_iam.sessions','DELETE,TRUNCATE')`

// The unique (issuer, subject) index serializes concurrent issuers. The no-op
// update locks an existing principal until commit without changing its profile,
// original ID or creation time; it also handles a concurrent first login safely.
const resolvePrincipal = `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
 VALUES($1,$2,$3,$4) ON CONFLICT(oidc_issuer,oidc_subject)
 DO UPDATE SET oidc_subject=EXCLUDED.oidc_subject
 RETURNING principal_id::text,disabled_at IS NOT NULL`

const pendingSessionRevoke = `SELECT EXISTS(SELECT 1 FROM hp_iam.revocations r
 JOIN hp_iam.sessions s ON s.session_id=r.subject_id
 WHERE r.scope='SESSION' AND r.status IN('PENDING','INCIDENT') AND s.principal_id=$1)`

const insertSession = `INSERT INTO hp_iam.sessions
 (session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`

// transaction is deliberately narrow so integration tests can inject an actual
// commit followed by a lost acknowledgement. It is not a success bypass.
type transaction interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

// Issuer borrows the caller's bounded IAM pool and never closes it. The caller
// owns transport security and must not install SQL-argument tracing/logging.
type Issuer struct {
	pool  *pgxpool.Pool
	begin func(context.Context) (transaction, error)
}

// New performs no I/O and opens no additional pool. Authority is checked inside
// each Create transaction, so pooled connections cannot bypass a changed role.
func New(pool *pgxpool.Pool) (*Issuer, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 {
		return nil, ErrConfiguration
	}
	return &Issuer{pool: pool, begin: func(ctx context.Context) (transaction, error) {
		return pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	}}, nil
}

// Create has one two-second budget including pool acquisition, authority checks,
// principal locking, session insertion and commit. A commit error may mean the
// commit happened: no success is returned and the consumed login is not retried.
// The trusted application guard owns rotation/distributed revocation admission;
// this adapter additionally rejects disabled principals and existing pending or
// incident SESSION revocations. It does not change epochs or revoke state.
func (i *Issuer) Create(ctx context.Context, record issuesession.Record) (issuesession.Persisted, error) {
	if i == nil || i.pool == nil || i.begin == nil || ctx == nil {
		return issuesession.Persisted{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return issuesession.Persisted{}, err
	}
	if !validRecord(record) {
		return issuesession.Persisted{}, ErrInvalidRecord
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	failed := func() (issuesession.Persisted, error) {
		if err := bounded.Err(); err != nil {
			return issuesession.Persisted{}, err
		}
		return issuesession.Persisted{}, ErrUnavailable
	}
	tx, err := i.begin(bounded)
	if err != nil {
		return failed()
	}
	// pgx closes a connection if rollback cannot run after cancellation. The same
	// bounded context avoids extending an expired operation with a cleanup wait.
	defer func() { _ = tx.Rollback(bounded) }()
	var authorized bool
	if err := tx.QueryRow(bounded, checkAuthority).Scan(&authorized); err != nil || !authorized {
		if bounded.Err() != nil {
			return failed()
		}
		return issuesession.Persisted{}, ErrConfiguration
	}
	var principal string
	var disabled bool
	if err := tx.QueryRow(bounded, resolvePrincipal, record.PrincipalID, record.Issuer, record.Subject, record.CreatedAt).Scan(&principal, &disabled); err != nil {
		return failed()
	}
	if disabled {
		return issuesession.Persisted{}, ErrDenied
	}
	var pending bool
	if err := tx.QueryRow(bounded, pendingSessionRevoke, principal).Scan(&pending); err != nil {
		return failed()
	}
	if pending {
		return issuesession.Persisted{}, ErrDenied
	}
	if _, err := tx.Exec(bounded, insertSession, record.SessionID, record.SessionHash[:], principal, record.CSRFHash[:], record.CreatedAt, record.ExpiresAt, record.IdleExpiresAt, record.Epoch); err != nil {
		return failed()
	}
	if err := tx.Commit(bounded); err != nil {
		return failed()
	}
	if bounded.Err() != nil {
		return failed()
	}
	return issuesession.Persisted{PrincipalID: principal, SessionID: record.SessionID}, nil
}

func validRecord(r issuesession.Record) bool {
	text := func(value string, limit int) bool {
		return len(value) > 0 && len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
	}
	return text(r.Issuer, 2048) && text(r.Subject, 255) && canonicalUUID(r.PrincipalID) && canonicalUUID(r.SessionID) &&
		r.SessionHash != [32]byte{} && r.CSRFHash != [32]byte{} && r.SessionHash != r.CSRFHash &&
		r.Epoch > 0 && sessionpolicy.Evaluate(sessionpolicy.Snapshot{Found: true, CreatedAt: r.CreatedAt, ExpiresAt: r.ExpiresAt, IdleExpiresAt: r.IdleExpiresAt}, sessionpolicy.Observation{At: r.CreatedAt, Trusted: true}).IsValid()
}

func canonicalUUID(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for index, c := range []byte(value) {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

var _ issuesession.Store = (*Issuer)(nil)
