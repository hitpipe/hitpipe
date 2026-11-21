// Package migrations installs the unchanged accepted IAM schema using a dedicated owner.
package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

//go:embed schema/001_iam.sql
var files embed.FS

const SQLSHA256 = "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"

// SchemaSQL returns a copy: callers cannot mutate the embedded migration.
func SchemaSQL() []byte {
	data, _ := files.ReadFile("schema/001_iam.sql")
	return data
}

var (
	ErrConnection = errors.New("IAM_MIGRATION_CONNECTION_INVALID")
	ErrRole       = errors.New("IAM_MIGRATION_ROLE_UNSAFE")
	ErrDefaults   = errors.New("IAM_MIGRATION_DEFAULT_PRIVILEGES_UNSAFE")
	ErrSchema     = errors.New("IAM_MIGRATION_SCHEMA_STATE_INVALID")
	ErrSQL        = errors.New("IAM_MIGRATION_SQL_FAILED")
	ErrLock       = errors.New("IAM_MIGRATION_LOCK_FAILED")
)

type Result struct {
	State     string `json:"state"`
	Version   int    `json:"version"`
	SQLSHA256 string `json:"sql_sha256"`
}

// Apply needs a fresh connection whose current role is hp_iam_migrator. Bootstrap
// is an explicitly separate administrator operation. No runtime login may own DDL.
// The SQL already contains BEGIN/COMMIT, so it must not run inside a pgx transaction.
func Apply(ctx context.Context, conn *pgx.Conn) (Result, error) {
	if conn == nil || conn.IsClosed() || conn.PgConn().TxStatus() != 'I' {
		return Result{}, ErrConnection
	}
	data := SchemaSQL()
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != SQLSHA256 {
		return Result{}, ErrSQL
	}
	if err := checkRoles(ctx, conn); err != nil {
		return Result{}, err
	}
	// Advisory locks are scoped to this database and released if the connection dies.
	const lockKey int64 = 0x485049414d000001
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockKey); err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = conn.Close(cleanup)
		return Result{}, ErrLock
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if conn.PgConn().TxStatus() != 'I' {
			if _, err := conn.Exec(cleanup, "ROLLBACK"); err != nil {
				_ = conn.Close(cleanup)
				return
			}
		}
		if _, err := conn.Exec(cleanup, "SELECT pg_advisory_unlock($1)", lockKey); err != nil {
			_ = conn.Close(cleanup)
		}
	}()
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='hp_iam')").Scan(&exists); err != nil {
		return Result{}, ErrSchema
	}
	if exists {
		if err := checkInstalled(ctx, conn); err != nil {
			return Result{}, err
		}
		return Result{State: "already_installed", Version: 1, SQLSHA256: SQLSHA256}, nil
	}
	// Refuse preconfigured default grants rather than letting them silently widen
	// the privileges of tables created by the exact accepted SQL.
	var defaults bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_default_acl
		WHERE defaclrole=(SELECT oid FROM pg_roles WHERE rolname=current_user))`).Scan(&defaults); err != nil || defaults {
		return Result{}, ErrDefaults
	}
	// PgConn simple-query executes the byte-for-byte file, including its own
	// transaction boundaries. A failed statement is rolled back before returning.
	if _, err := conn.PgConn().Exec(ctx, string(data)).ReadAll(); err != nil {
		return Result{}, ErrSQL
	}
	if err := checkInstalled(ctx, conn); err != nil {
		return Result{}, err
	}
	return Result{State: "installed", Version: 1, SQLSHA256: SQLSHA256}, nil
}

func checkRoles(ctx context.Context, conn *pgx.Conn) error {
	var safe bool
	err := conn.QueryRow(ctx, `SELECT current_user='hp_iam_migrator'
		AND NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname IN(current_user,session_user)
			AND (rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls))
		AND (SELECT count(*)=4 FROM pg_roles WHERE rolname IN
			('hp_iam_migrator','hp_iam_app','hp_iam_worker','hp_iam_purge')
			AND NOT (rolcanlogin OR rolsuper OR rolcreatedb OR rolcreaterole OR rolreplication OR rolbypassrls))
		AND NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.member
			WHERE r.rolname IN('hp_iam_migrator','hp_iam_app','hp_iam_worker','hp_iam_purge'))
		AND NOT EXISTS(SELECT 1 FROM pg_auth_members m JOIN pg_roles r ON r.oid=m.roleid
			WHERE m.member=(SELECT oid FROM pg_roles WHERE rolname=session_user)
			AND r.rolname<>'hp_iam_migrator')`).Scan(&safe)
	if err != nil || !safe {
		return ErrRole
	}
	return nil
}

func checkInstalled(ctx context.Context, conn *pgx.Conn) error {
	var safe bool
	err := conn.QueryRow(ctx, `SELECT n.nspowner=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator')
		AND (SELECT count(*)=17 FROM pg_class c WHERE c.relnamespace=n.oid AND c.relkind IN('r','p'))
		AND NOT EXISTS(SELECT 1 FROM pg_class c WHERE c.relnamespace=n.oid
			AND c.relowner<>(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator'))
		AND (SELECT count(*)=17 FROM pg_class c WHERE c.relnamespace=n.oid AND c.relkind='r'
			AND c.relname=ANY(ARRAY['schema_version','principals','sessions','workspaces','memberships',
			'owner_transfers','external_bindings','revocations','runtime_registry','revocation_participants',
			'deletion_overlays','deletion_receipts','support_grants','command_receipts','audit_minimal','outbox','job_index']))
		AND (SELECT count(*)=10 FROM pg_class c WHERE c.relnamespace=n.oid AND c.relrowsecurity AND c.relforcerowsecurity
			AND c.relname=ANY(ARRAY['workspaces','memberships','owner_transfers','external_bindings','deletion_overlays',
			'deletion_receipts','support_grants','command_receipts','audit_minimal','outbox']))
		FROM pg_namespace n WHERE n.nspname='hp_iam'`).Scan(&safe)
	if err != nil || !safe {
		return ErrSchema
	}
	var count, version int
	if err := conn.QueryRow(ctx, "SELECT count(*),coalesce(max(version),0) FROM hp_iam.schema_version").Scan(&count, &version); err != nil || count != 1 || version != 1 {
		return ErrSchema
	}
	return checkPermissions(ctx, conn)
}
