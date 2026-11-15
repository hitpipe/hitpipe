//go:build integration

package migrations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// These are database-boundary tests, not proof of a user's business authority:
// trusted IAM code can choose hp.workspace_id, so it must authorize the caller first.
var tenantTables = []string{
	"workspaces", "memberships", "owner_transfers", "external_bindings",
	"deletion_overlays", "deletion_receipts", "support_grants",
	"command_receipts", "audit_minimal", "outbox",
}

var iamGlobalTables = []string{
	"schema_version", "principals", "sessions", "revocations", "runtime_registry", "revocation_participants",
}

type schemaDB struct {
	admin *pgx.Conn
	cfg   *pgx.ConnConfig
	ctx   context.Context
}

func schemaDatabase(t *testing.T) schemaDB {
	t.Helper()
	adminDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN")
	if adminDSN == "" {
		t.Skip("run tools/check_iam_schema.py for isolated PostgreSQL schema tests")
	}
	cfg, err := pgx.ParseConfig(adminDSN)
	if err != nil || cfg.Host != "127.0.0.1" ||
		(!strings.HasPrefix(cfg.Database, "hit81_test_") && !strings.HasPrefix(cfg.Database, "hit68_test_")) || cfg.User != "postgres" {
		t.Fatal("schema tests require the isolated loopback fixture database and its administrator")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("could not connect to isolated fixture database; connection values are withheld")
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	var user, session string
	if err := admin.QueryRow(ctx, "SELECT current_user,session_user").Scan(&user, &session); err != nil || user != "postgres" || session != "postgres" {
		t.Fatal("fixture connection must have the expected local administrator identity")
	}
	return schemaDB{admin: admin, cfg: cfg, ctx: ctx}
}

func (db schemaDB) runtime(t *testing.T, kind string) *pgx.Conn {
	t.Helper()
	env, user := "HITPIPE_IAM_TEST_"+strings.ToUpper(kind)+"_DSN", "hp_hit81_"+kind
	cfg, err := pgx.ParseConfig(os.Getenv(env))
	if err != nil || os.Getenv(env) == "" || cfg.Host != db.cfg.Host || cfg.Port != db.cfg.Port ||
		cfg.Database != db.cfg.Database || cfg.User != user {
		t.Fatal("runtime tests require a dedicated nonprivileged login in the same isolated database")
	}
	conn, err := pgx.ConnectConfig(db.ctx, cfg)
	if err != nil {
		t.Fatal("could not connect to isolated runtime; connection values are withheld")
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	var current, session string
	if err := conn.QueryRow(db.ctx, "SELECT current_user,session_user").Scan(&current, &session); err != nil || current != user || session != user {
		t.Fatal("runtime checks must execute through the actual runtime login, without administrator SET ROLE")
	}
	return conn
}

func schemaUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal("synthetic fixture random source unavailable")
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func schemaExec(t *testing.T, ctx context.Context, q interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, sql string, args ...any) pgconn.CommandTag {
	t.Helper()
	tag, err := q.Exec(ctx, sql, args...)
	if err != nil {
		t.Fatalf("isolated database statement failed: %v", err)
	}
	return tag
}

func schemaTx(t *testing.T, db schemaDB, conn *pgx.Conn) pgx.Tx {
	t.Helper()
	tx, err := conn.Begin(db.ctx)
	if err != nil {
		t.Fatal("could not begin isolated transaction")
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}

func schemaWorkspace(t *testing.T, db schemaDB, tx pgx.Tx, workspace string) {
	t.Helper()
	schemaExec(t, db.ctx, tx, "SELECT set_config('hp.workspace_id',$1,true)", workspace)
}

func expectSchemaError(t *testing.T, db schemaDB, tx pgx.Tx, state, sql string, args ...any) {
	t.Helper()
	probe, err := tx.Begin(db.ctx) // SAVEPOINT keeps one expected failure from hiding later checks.
	if err != nil {
		t.Fatal("could not begin isolated failure probe")
	}
	_, execErr := probe.Exec(db.ctx, sql, args...)
	if err := probe.Rollback(db.ctx); err != nil {
		t.Fatal("could not recover expected failure probe")
	}
	var pgErr *pgconn.PgError
	if !errors.As(execErr, &pgErr) || pgErr.Code != state {
		t.Fatalf("expected PostgreSQL SQLSTATE %s; got %v", state, execErr)
	}
}

type schemaFixture struct {
	workspace  [2]string
	principal  [2]string
	session    string
	runtime    string
	boot       string
	revocation string
	created    time.Time
	digest     [32]byte
}

func tenantInsert(t *testing.T, f schemaFixture, table, workspace string) (string, []any) {
	t.Helper()
	id, ref := schemaUUID(t), schemaUUID(t)
	prefix := "INSERT INTO hp_iam." + table
	switch table {
	case "workspaces":
		return prefix + "(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at) VALUES($1,$2,1,1,'ACTIVE',$3)", []any{workspace, ref, f.created}
	case "memberships":
		return prefix + "(workspace_id,principal_id,role,status,revision) VALUES($1,$2,'OWNER','ACTIVE',1)", []any{workspace, f.principal[0]}
	case "owner_transfers":
		return prefix + "(workspace_id,transfer_id,from_principal_id,to_principal_id,expected_workspace_revision,created_at,expires_at) VALUES($1,$2,$3,$4,1,$5,$6)", []any{workspace, id, f.principal[0], f.principal[1], f.created, f.created.Add(time.Hour)}
	case "external_bindings":
		return prefix + "(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at) VALUES($1,$2,'STRIPE',$3,false,'PAID_ORDER_TASK_V1','VERIFIED',1,$4)", []any{workspace, id, "synthetic-" + id, f.created}
	case "deletion_overlays":
		return prefix + "(workspace_id,deletion_id,confirmed_at,active_deadline_at,backup_deadline_at,status) VALUES($1,$2,$3,$4,$5,'CLOSING')", []any{workspace, id, f.created, f.created.Add(7 * 24 * time.Hour), f.created.Add(35 * 24 * time.Hour)}
	case "deletion_receipts":
		return prefix + "(workspace_id,service,receipt_digest) VALUES($1,'IAM',$2)", []any{workspace, "sha256:" + strings.Repeat("a", 64)}
	case "support_grants":
		return prefix + "(workspace_id,grant_id,support_principal_id,authorized_by,action_code,resource_id,created_at,expires_at) VALUES($1,$2,$3,$4,'synthetic.inspect',$5,$6,$7)", []any{workspace, id, f.principal[1], f.principal[0], ref, f.created, f.created.Add(time.Hour)}
	case "command_receipts":
		return prefix + "(workspace_id,actor_id,request_id,body_digest,result_id,result_kind,created_at) VALUES($1,$2,$3,$4,$5,'SYNTHETIC',$6)", []any{workspace, f.principal[0], id, "sha256:" + strings.Repeat("b", 64), ref, f.created}
	case "audit_minimal":
		return prefix + "(workspace_id,entry_id,actor_id,action_code,resource_id,at,reason_code) VALUES($1,$2,$3,'synthetic.inspect',$4,$5,'SYNTHETIC')", []any{workspace, id, f.principal[0], ref, f.created}
	case "outbox":
		return prefix + "(workspace_id,message_id,digest,kind,payload_content_ref,payload_expires_at,created_at,state,next_attempt_at) VALUES($1,$2,$3,'SYNTHETIC',$4,$5,$6,'PENDING',$6)", []any{workspace, id, "sha256:" + strings.Repeat("c", 64), ref, f.created.Add(time.Hour), f.created}
	default:
		t.Fatal("unlisted canonical tenant table")
		return "", nil
	}
}

func seedSchema(t *testing.T, db schemaDB) schemaFixture {
	t.Helper()
	f := schemaFixture{
		workspace: [2]string{schemaUUID(t), schemaUUID(t)}, principal: [2]string{schemaUUID(t), schemaUUID(t)},
		session: schemaUUID(t), runtime: schemaUUID(t), boot: schemaUUID(t), revocation: schemaUUID(t),
		created: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
	}
	f.digest = sha256.Sum256([]byte("SYNTHETIC-HIT81-" + f.session))
	// Cleanup is scoped to this fixture's UUIDs, including sessions created by its tests.
	// Administrator is used only for setup/cleanup, never for a permission assertion.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanup, err := db.admin.Begin(ctx)
		if err != nil {
			t.Error("fixture cleanup could not begin")
			return
		}
		defer cleanup.Rollback(ctx)
		for _, table := range []string{"owner_transfers", "support_grants", "deletion_receipts", "deletion_overlays", "memberships", "external_bindings", "command_receipts", "audit_minimal", "outbox", "job_index", "workspaces"} {
			if _, err := cleanup.Exec(ctx, "DELETE FROM hp_iam."+table+" WHERE workspace_id IN($1,$2)", f.workspace[0], f.workspace[1]); err != nil {
				t.Errorf("could not clean up owned %s fixtures", table)
				return
			}
		}
		for _, query := range []struct {
			sql  string
			args []any
		}{
			{"DELETE FROM hp_iam.revocation_participants WHERE revocation_id=$1", []any{f.revocation}},
			{"DELETE FROM hp_iam.revocations WHERE revocation_id=$1", []any{f.revocation}},
			{"DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1", []any{f.runtime}},
			{"DELETE FROM hp_iam.sessions WHERE principal_id IN($1,$2)", []any{f.principal[0], f.principal[1]}},
			{"DELETE FROM hp_iam.principals WHERE principal_id IN($1,$2)", []any{f.principal[0], f.principal[1]}},
		} {
			if _, err := cleanup.Exec(ctx, query.sql, query.args...); err != nil {
				t.Error("could not clean up owned global IAM fixtures")
				return
			}
		}
		if err := cleanup.Commit(ctx); err != nil {
			t.Error("fixture cleanup could not commit")
		}
	})
	tx := schemaTx(t, db, db.admin)
	schemaExec(t, db.ctx, tx, "SET LOCAL ROLE hp_iam_migrator")
	for _, id := range f.principal {
		schemaExec(t, db.ctx, tx, "INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://issuer.invalid',$2,$3)", id, id, f.created)
	}
	schemaExec(t, db.ctx, tx, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$2,$4,$5,$6,1)`, f.session, f.digest[:], f.principal[0], f.created, f.created.Add(time.Hour), f.created.Add(30*time.Minute))
	for _, workspace := range f.workspace {
		schemaWorkspace(t, db, tx, workspace)
		for _, table := range tenantTables {
			sql, args := tenantInsert(t, f, table, workspace)
			schemaExec(t, db.ctx, tx, sql, args...)
			if table == "memberships" {
				schemaExec(t, db.ctx, tx, "INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision) VALUES($1,$2,'OPERATOR','ACTIVE',1)", workspace, f.principal[1])
			}
		}
	}
	schemaExec(t, db.ctx, tx, `INSERT INTO hp_iam.runtime_registry(runtime_id,service,generation,boot_id,certificate_fingerprint,status,registered_at,last_observed_at)
VALUES($1,'IAM',1,$2,$3,'CLOSED',$4,$4)`, f.runtime, f.boot, "sha256:"+strings.Repeat("d", 64), f.created)
	schemaExec(t, db.ctx, tx, `INSERT INTO hp_iam.revocations(revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
VALUES($1,'SESSION',$2,1,$3,$4,'PENDING','SYNTHETIC')`, f.revocation, f.session, f.created, f.created.Add(5*time.Second))
	schemaExec(t, db.ctx, tx, `INSERT INTO hp_iam.revocation_participants(revocation_id,runtime_id,generation,boot_id) VALUES($1,$2,1,$3)`, f.revocation, f.runtime, f.boot)
	if err := tx.Commit(db.ctx); err != nil {
		t.Fatal("could not commit canonical fixture as the schema owner")
	}
	return f
}

func TestIAMSchemaRoleSeparationAndOwnership(t *testing.T) {
	db := schemaDatabase(t)
	for _, role := range []string{"hp_iam_migrator", "hp_iam_app", "hp_iam_worker", "hp_iam_purge"} {
		var login, elevated bool
		if err := db.admin.QueryRow(db.ctx, `SELECT rolcanlogin,(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication) FROM pg_roles WHERE rolname=$1`, role).Scan(&login, &elevated); err != nil || login || elevated {
			t.Fatalf("group/owner role %s must be NOLOGIN without administrative or RLS-bypass capabilities", role)
		}
	}
	for _, kind := range []string{"app", "worker", "purge"} {
		t.Run(kind, func(t *testing.T) {
			conn := db.runtime(t, kind)
			var elevated, login, inherits, owner, schemaUsage, schemaCreate bool
			var memberships []string
			if err := conn.QueryRow(db.ctx, `SELECT (rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication),rolcanlogin,rolinherit,
pg_has_role(current_user,'hp_iam_migrator','MEMBER'),has_schema_privilege(current_user,'hp_iam','USAGE'),has_schema_privilege(current_user,'hp_iam','CREATE')
FROM pg_roles WHERE rolname=current_user`).Scan(&elevated, &login, &inherits, &owner, &schemaUsage, &schemaCreate); err != nil || elevated || !login || !inherits || owner || !schemaUsage || schemaCreate {
				t.Fatal("runtime login has unexpected identity, schema ownership, inheritance or administrative rights")
			}
			if err := conn.QueryRow(db.ctx, `SELECT COALESCE(array_agg(parent.rolname::text ORDER BY parent.rolname),ARRAY[]::text[]) FROM pg_auth_members m
JOIN pg_roles child ON child.oid=m.member JOIN pg_roles parent ON parent.oid=m.roleid WHERE child.rolname=current_user`).Scan(&memberships); err != nil || !reflect.DeepEqual(memberships, []string{"hp_iam_" + kind}) {
				t.Fatal("runtime login must inherit exactly its own IAM group")
			}
		})
	}
	var owned, foreign int
	if err := db.admin.QueryRow(db.ctx, `SELECT count(*),count(*) FILTER(WHERE pg_get_userbyid(c.relowner)<>'hp_iam_migrator')
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='hp_iam' AND c.relkind='r'`).Scan(&owned, &foreign); err != nil || owned != 17 || foreign != 0 {
		t.Fatal("all 17 canonical IAM tables must be owned by the dedicated migrator")
	}
}

func TestIAMSchemaRuntimePrivilegeMatrix(t *testing.T) {
	db := schemaDatabase(t)
	tables := append(append(append([]string{}, tenantTables...), iamGlobalTables...), "job_index")
	for _, kind := range []string{"app", "worker", "purge"} {
		conn := db.runtime(t, kind)
		for _, table := range tables {
			var got [4]bool // SELECT, INSERT, UPDATE, DELETE.
			if err := conn.QueryRow(db.ctx, `SELECT has_table_privilege(current_user,$1,'SELECT'),has_table_privilege(current_user,$1,'INSERT'),
has_table_privilege(current_user,$1,'UPDATE'),has_table_privilege(current_user,$1,'DELETE')`, "hp_iam."+table).Scan(&got[0], &got[1], &got[2], &got[3]); err != nil {
				t.Fatal("could not inspect actual runtime table privileges")
			}
			want := [4]bool{true, true, true, false}
			if kind == "purge" {
				want = [4]bool{true, false, false, true}
			}
			if table == "audit_minimal" || table == "command_receipts" {
				want[2] = false
			}
			if table == "schema_version" {
				want = [4]bool{true, false, false, false}
			}
			if table == "job_index" {
				want = [4]bool{}
				if kind != "app" {
					want = [4]bool{true, true, true, true}
				}
			}
			if got != want {
				t.Fatalf("%s has unexpected SELECT/INSERT/UPDATE/DELETE rights on %s: got %v, want %v", kind, table, got, want)
			}
		}
	}
}

func TestIAMSchemaTenantRLSReadAndWriteIsolation(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	for _, table := range tenantTables {
		var enabled, forced bool
		if err := db.admin.QueryRow(db.ctx, "SELECT relrowsecurity,relforcerowsecurity FROM pg_class WHERE oid=$1::regclass", "hp_iam."+table).Scan(&enabled, &forced); err != nil || !enabled || !forced {
			t.Fatalf("%s must enable and force row-level security, including its owner", table)
		}
		for _, kind := range []string{"app", "worker", "purge"} {
			t.Run(table+"/"+kind, func(t *testing.T) {
				conn := db.runtime(t, kind)
				tx := schemaTx(t, db, conn)
				for _, workspace := range []string{"", f.workspace[0], f.workspace[1]} {
					schemaWorkspace(t, db, tx, workspace)
					var count, foreign int
					if err := tx.QueryRow(db.ctx, "SELECT count(*),count(*) FILTER(WHERE workspace_id::text<>$1) FROM hp_iam."+table+" WHERE workspace_id IN($2,$3)", workspace, f.workspace[0], f.workspace[1]).Scan(&count, &foreign); err != nil {
						t.Fatal("could not read tenant fixture under the actual runtime role")
					}
					want := 1
					if table == "memberships" {
						want = 2
					}
					if workspace == "" {
						want = 0
					}
					if count != want || foreign != 0 {
						t.Fatalf("%s disclosed missing/foreign workspace rows", table)
					}
				}
				schemaWorkspace(t, db, tx, f.workspace[0])
				if kind == "purge" {
					tag := schemaExec(t, db.ctx, tx, "DELETE FROM hp_iam."+table+" WHERE workspace_id=$1", f.workspace[1])
					if tag.RowsAffected() != 0 {
						t.Fatal("purge could delete another workspace's rows")
					}
					return
				}
				schemaWorkspace(t, db, tx, "")
				missingContextInsert, missingContextArgs := tenantInsert(t, f, table, f.workspace[0])
				expectSchemaError(t, db, tx, "42501", missingContextInsert, missingContextArgs...)
				schemaWorkspace(t, db, tx, f.workspace[0])
				insert, args := tenantInsert(t, f, table, f.workspace[1])
				expectSchemaError(t, db, tx, "42501", insert, args...)
				if table == "audit_minimal" || table == "command_receipts" {
					expectSchemaError(t, db, tx, "42501", "UPDATE hp_iam."+table+" SET workspace_id=workspace_id WHERE workspace_id=$1", f.workspace[0])
					return
				}
				tag := schemaExec(t, db.ctx, tx, "UPDATE hp_iam."+table+" SET workspace_id=workspace_id WHERE workspace_id=$1", f.workspace[1])
				if tag.RowsAffected() != 0 {
					t.Fatal("runtime could update a foreign workspace row")
				}
				tag = schemaExec(t, db.ctx, tx, "UPDATE hp_iam."+table+" SET workspace_id=workspace_id WHERE workspace_id=$1", f.workspace[0])
				if tag.RowsAffected() == 0 {
					t.Fatal("authorized tenant update was unexpectedly hidden")
				}
				expectSchemaError(t, db, tx, "42501", "UPDATE hp_iam."+table+" SET workspace_id=$1 WHERE workspace_id=$2", f.workspace[1], f.workspace[0])
			})
		}
	}
}

func TestIAMSchemaOwnerCannotBypassForcedRLS(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	tx := schemaTx(t, db, db.admin)
	schemaExec(t, db.ctx, tx, "SET LOCAL ROLE hp_iam_migrator")
	schemaWorkspace(t, db, tx, "")
	for _, table := range tenantTables {
		var count int
		if err := tx.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam."+table+" WHERE workspace_id IN($1,$2)", f.workspace[0], f.workspace[1]).Scan(&count); err != nil || count != 0 {
			t.Fatalf("schema owner bypassed forced tenant RLS on %s", table)
		}
	}
}

func TestIAMSchemaSetLocalDoesNotLeakAcrossTransactions(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	conn := db.runtime(t, "app")
	for _, commit := range []bool{true, false} {
		tx := schemaTx(t, db, conn)
		schemaWorkspace(t, db, tx, f.workspace[0])
		var visible int
		if err := tx.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id=$1", f.workspace[0]).Scan(&visible); err != nil || visible != 1 {
			t.Fatal("first transaction did not see its selected workspace")
		}
		var err error
		if commit {
			err = tx.Commit(db.ctx)
		} else {
			err = tx.Rollback(db.ctx)
		}
		if err != nil {
			t.Fatal("could not finish first runtime transaction")
		}
		next := schemaTx(t, db, conn) // The same physical connection, not a fresh pool checkout.
		var workspace string
		if err := next.QueryRow(db.ctx, "SELECT COALESCE(NULLIF(current_setting('hp.workspace_id',true),''),'')").Scan(&workspace); err != nil || workspace != "" {
			t.Fatal("SET LOCAL leaked workspace selection into the next transaction")
		}
		if err := next.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam.workspaces WHERE workspace_id IN($1,$2)", f.workspace[0], f.workspace[1]).Scan(&visible); err != nil || visible != 0 {
			t.Fatal("the next request inherited the preceding request's tenant rows")
		}
		if err := next.Rollback(db.ctx); err != nil {
			t.Fatal("could not close fresh runtime transaction")
		}
	}
}

func TestIAMSchemaGlobalTablesAreIAMAuthorityWithoutTenantFiction(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	for _, table := range iamGlobalTables {
		var enabled, forced bool
		if err := db.admin.QueryRow(db.ctx, "SELECT relrowsecurity,relforcerowsecurity FROM pg_class WHERE oid=$1::regclass", "hp_iam."+table).Scan(&enabled, &forced); err != nil || enabled || forced {
			t.Fatalf("global IAM table %s must not pretend a workspace GUC supplies IAM business authority", table)
		}
	}
	predicates := map[string]struct {
		where string
		id    any
	}{
		"schema_version": {"version=$1", 1}, "principals": {"principal_id=$1", f.principal[0]},
		"sessions": {"session_id=$1", f.session}, "revocations": {"revocation_id=$1 AND workspace_id IS NULL", f.revocation},
		"runtime_registry": {"runtime_id=$1", f.runtime}, "revocation_participants": {"revocation_id=$1 AND workspace_id IS NULL", f.revocation},
	}
	for _, kind := range []string{"app", "worker", "purge"} {
		conn := db.runtime(t, kind)
		tx := schemaTx(t, db, conn)
		for _, workspace := range []string{"", f.workspace[0], f.workspace[1]} {
			schemaWorkspace(t, db, tx, workspace)
			for _, table := range iamGlobalTables {
				predicate := predicates[table]
				var count int
				if err := tx.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam."+table+" WHERE "+predicate.where, predicate.id).Scan(&count); err != nil || count != 1 {
					t.Fatalf("global IAM table %s was incorrectly filtered by workspace selection", table)
				}
			}
		}
	}
}

func TestIAMSchemaJobIndexIsWorkerAndPurgeOnly(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	app := schemaTx(t, db, db.runtime(t, "app"))
	for _, sql := range []string{
		"SELECT * FROM hp_iam.job_index", "INSERT INTO hp_iam.job_index(job_id,workspace_id,kind,due_at) SELECT gen_random_uuid(),gen_random_uuid(),'SYNTHETIC',clock_timestamp()",
		"UPDATE hp_iam.job_index SET kind='SYNTHETIC'", "DELETE FROM hp_iam.job_index",
	} {
		expectSchemaError(t, db, app, "42501", sql)
	}
	for _, kind := range []string{"worker", "purge"} {
		t.Run(kind, func(t *testing.T) {
			tx := schemaTx(t, db, db.runtime(t, kind))
			id, token := schemaUUID(t), schemaUUID(t)
			schemaExec(t, db.ctx, tx, "INSERT INTO hp_iam.job_index(job_id,workspace_id,kind,due_at) VALUES($1,$2,'SYNTHETIC',$3)", id, f.workspace[0], f.created)
			var workspace string
			if err := tx.QueryRow(db.ctx, "SELECT workspace_id::text FROM hp_iam.job_index WHERE job_id=$1", id).Scan(&workspace); err != nil || workspace != f.workspace[0] {
				t.Fatal("coordinator role could not read its global job index entry without tenant GUC")
			}
			if schemaExec(t, db.ctx, tx, "UPDATE hp_iam.job_index SET claim_token=$1 WHERE job_id=$2", token, id).RowsAffected() != 1 {
				t.Fatal("coordinator role could not claim its job entry")
			}
			if schemaExec(t, db.ctx, tx, "DELETE FROM hp_iam.job_index WHERE job_id=$1", id).RowsAffected() != 1 {
				t.Fatal("coordinator role could not remove its job entry")
			}
		})
	}
}

func TestIAMSchemaGlobalUniquenessAndForeignKeys(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	tx := schemaTx(t, db, db.runtime(t, "app"))
	expectSchemaError(t, db, tx, "23505", "INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://issuer.invalid',$2,$3)", schemaUUID(t), f.principal[0], f.created)
	// Subject identifiers are scoped to their issuer, not globally unique alone.
	schemaExec(t, db.ctx, tx, "UPDATE hp_iam.principals SET oidc_issuer='https://other-issuer.invalid',oidc_subject=$1 WHERE principal_id=$2", f.principal[0], f.principal[1])
	expectSchemaError(t, db, tx, "23505", `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$2,$4,$5,$5,1)`, schemaUUID(t), f.digest[:], f.principal[1], f.created, f.created.Add(time.Hour))
	expectSchemaError(t, db, tx, "23503", `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$2,$4,$5,$5,1)`, schemaUUID(t), sha256Bytes(schemaUUID(t)), schemaUUID(t), f.created, f.created.Add(time.Hour))
	// The same external account/pack/mode cannot be reserved by another workspace.
	var providerAccount string
	schemaWorkspace(t, db, tx, f.workspace[0])
	if err := tx.QueryRow(db.ctx, "SELECT provider_account_id FROM hp_iam.external_bindings WHERE workspace_id=$1", f.workspace[0]).Scan(&providerAccount); err != nil {
		t.Fatal("could not read synthetic external account")
	}
	schemaWorkspace(t, db, tx, f.workspace[1])
	expectSchemaError(t, db, tx, "23505", `INSERT INTO hp_iam.external_bindings(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at)
VALUES($1,$2,'STRIPE',$3,false,'PAID_ORDER_TASK_V1','RESERVED',1,$4)`, f.workspace[1], schemaUUID(t), providerAccount, f.created)
	// livemode is deliberately part of the canonical uniqueness key.
	schemaExec(t, db.ctx, tx, `INSERT INTO hp_iam.external_bindings(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at)
VALUES($1,$2,'STRIPE',$3,true,'PAID_ORDER_TASK_V1','RESERVED',1,$4)`, f.workspace[1], schemaUUID(t), providerAccount, f.created)
}

func sha256Bytes(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }

func TestIAMSchemaSessionAndRevocationConstraints(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	tx := schemaTx(t, db, db.runtime(t, "app"))
	for _, change := range []string{
		"session_hash=decode('00','hex')", "csrf_hash=decode('00','hex')", "epoch=0",
		"expires_at=created_at", "idle_expires_at=expires_at+interval '1 microsecond'",
	} {
		t.Run("session/"+change, func(t *testing.T) {
			expectSchemaError(t, db, tx, "23514", "UPDATE hp_iam.sessions SET "+change+" WHERE session_id=$1", f.session)
		})
	}
	for _, change := range []string{
		"epoch=0", "deadline_at=accepted_at+interval '6 seconds'", "status='EFFECTIVE'",
		"effective_at=accepted_at", "scope='UNSUPPORTED'", "status='UNSUPPORTED'",
	} {
		t.Run("revocation/"+change, func(t *testing.T) {
			expectSchemaError(t, db, tx, "23514", "UPDATE hp_iam.revocations SET "+change+" WHERE revocation_id=$1", f.revocation)
		})
	}
	expectSchemaError(t, db, tx, "23514", "UPDATE hp_iam.revocation_participants SET acknowledged_at=$1 WHERE revocation_id=$2", f.created, f.revocation)
	schemaExec(t, db.ctx, tx, "UPDATE hp_iam.revocations SET status='EFFECTIVE',effective_at=$1 WHERE revocation_id=$2", f.created.Add(time.Second), f.revocation)
	schemaExec(t, db.ctx, tx, "UPDATE hp_iam.revocation_participants SET acknowledged_at=$1,applied_epoch=1 WHERE revocation_id=$2", f.created.Add(time.Second), f.revocation)
}

func TestIAMSchemaAppendOnlyAuditAndCommandReceipts(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	for _, kind := range []string{"app", "worker"} {
		for _, table := range []string{"audit_minimal", "command_receipts"} {
			t.Run(kind+"/"+table, func(t *testing.T) {
				tx := schemaTx(t, db, db.runtime(t, kind))
				schemaWorkspace(t, db, tx, f.workspace[0])
				insert, args := tenantInsert(t, f, table, f.workspace[0])
				schemaExec(t, db.ctx, tx, insert, args...)
				expectSchemaError(t, db, tx, "42501", "UPDATE hp_iam."+table+" SET workspace_id=workspace_id WHERE workspace_id=$1", f.workspace[0])
				expectSchemaError(t, db, tx, "42501", "DELETE FROM hp_iam."+table+" WHERE workspace_id=$1", f.workspace[0])
			})
		}
	}
	// The retention/purge authority has a separate, tenant-scoped DELETE grant.
	tx := schemaTx(t, db, db.runtime(t, "purge"))
	schemaWorkspace(t, db, tx, f.workspace[0])
	for _, table := range []string{"audit_minimal", "command_receipts"} {
		if schemaExec(t, db.ctx, tx, "DELETE FROM hp_iam."+table+" WHERE workspace_id=$1", f.workspace[0]).RowsAffected() != 1 {
			t.Fatal("purge did not retain its explicit canonical retention authority")
		}
		insert, args := tenantInsert(t, f, table, f.workspace[0])
		expectSchemaError(t, db, tx, "42501", insert, args...)
	}
}

func TestIAMSchemaCommitIsDurableAndFailedTransactionLeavesNoPartialWrite(t *testing.T) {
	db := schemaDatabase(t)
	f := seedSchema(t, db)
	// COMMIT durability here is the live PostgreSQL boundary: crash/recovery or
	// production storage-loss qualification is a separate operational check.
	var fsync, fullPageWrites, synchronousCommit string
	if err := db.admin.QueryRow(db.ctx, `SELECT current_setting('fsync'),current_setting('full_page_writes'),current_setting('synchronous_commit')`).Scan(&fsync, &fullPageWrites, &synchronousCommit); err != nil || fsync != "on" || fullPageWrites != "on" || synchronousCommit != "on" {
		t.Fatal("commit test requires synchronous WAL durability settings")
	}
	var unlogged int
	if err := db.admin.QueryRow(db.ctx, `SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='hp_iam' AND c.relkind='r' AND c.relpersistence<>'p'`).Scan(&unlogged); err != nil || unlogged != 0 {
		t.Fatal("canonical IAM tables must use permanent logged storage")
	}
	writer := db.runtime(t, "app")
	committedID, rolledBackID := schemaUUID(t), schemaUUID(t)
	insert := `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
VALUES($1,$2,$3,$2,$4,$5,$5,1)`
	tx := schemaTx(t, db, writer)
	schemaExec(t, db.ctx, tx, insert, committedID, sha256Bytes(committedID), f.principal[0], f.created, f.created.Add(time.Hour))
	if err := tx.Commit(db.ctx); err != nil {
		t.Fatal("valid runtime transaction did not commit")
	}
	reader := db.runtime(t, "app") // A distinct physical connection reads the committed row.
	var count int
	if err := reader.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam.sessions WHERE session_id=$1", committedID).Scan(&count); err != nil || count != 1 {
		t.Fatal("fresh connection did not observe committed IAM state")
	}
	failed := schemaTx(t, db, writer)
	schemaExec(t, db.ctx, failed, insert, rolledBackID, sha256Bytes(rolledBackID), f.principal[0], f.created, f.created.Add(time.Hour))
	_, err := failed.Exec(db.ctx, "UPDATE hp_iam.sessions SET epoch=0 WHERE session_id=$1", rolledBackID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatal("controlled second statement did not fail its canonical constraint")
	}
	if err := failed.Commit(db.ctx); !errors.Is(err, pgx.ErrTxCommitRollback) {
		t.Fatal("an aborted multi-statement transaction unexpectedly committed")
	}
	if err := reader.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam.sessions WHERE session_id=$1", rolledBackID).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed transaction leaked its successful first statement")
	}
	if err := reader.QueryRow(db.ctx, "SELECT count(*) FROM hp_iam.sessions WHERE session_id=$1", committedID).Scan(&count); err != nil || count != 1 {
		t.Fatal("rollback damaged an earlier independently committed transaction")
	}
}
