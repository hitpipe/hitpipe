//go:build integration

package migrations

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Each migration scenario owns a separate database inside the disposable runner.
// Never derive test databases from a general application connection.
func migrationDatabase(t *testing.T) (context.Context, *pgx.Conn, func() *pgx.Conn) {
	t.Helper()
	adminDSN, migratorDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_MIGRATOR_DSN")
	if adminDSN == "" || migratorDSN == "" {
		t.Skip("disposable PostgreSQL runner is required")
	}
	adminCfg, err := pgx.ParseConfig(adminDSN)
	if err != nil || adminCfg.Host != "127.0.0.1" || !strings.HasPrefix(adminCfg.Database, "hit68_test_") {
		t.Fatal("refusing a non-runner PostgreSQL connection")
	}
	migrationCfg, err := pgx.ParseConfig(migratorDSN)
	if err != nil || migrationCfg.Host != adminCfg.Host || migrationCfg.Port != adminCfg.Port || migrationCfg.Database != adminCfg.Database {
		t.Fatal("migrator must belong to the same disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal("cannot connect to disposable administrator")
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "hit81_test_migration_" + hex.EncodeToString(suffix[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatalf("create isolated migration database: %v", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("remove isolated migration database: %v", err)
		}
		_ = admin.Close(cleanup)
	})
	if _, err := admin.Exec(ctx, "GRANT CREATE ON DATABASE "+identifier+" TO hp_iam_migrator"); err != nil {
		t.Fatal(err)
	}
	adminCfg.Database = name
	local, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close(context.Background()) })
	migrationCfg.Database = name
	connect := func() *pgx.Conn {
		t.Helper()
		conn, err := pgx.ConnectConfig(ctx, migrationCfg.Copy())
		if err != nil {
			t.Fatal("connect isolated migrator")
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	return ctx, local, connect
}

func TestIAMSchemaMigrationRepeatOnFreshConnection(t *testing.T) {
	ctx, admin, connect := migrationDatabase(t)
	first := connect()
	result, err := Apply(ctx, first)
	if err != nil || result.State != "installed" || result.Version != 1 || result.SQLSHA256 != SQLSHA256 {
		t.Fatalf("initial migration: %+v %v", result, err)
	}
	var installed time.Time
	if err := first.QueryRow(ctx, "SELECT installed_at FROM hp_iam.schema_version").Scan(&installed); err != nil {
		t.Fatal(err)
	}
	_ = first.Close(ctx)
	second := connect()
	result, err = Apply(ctx, second)
	if err != nil || result.State != "already_installed" {
		t.Fatalf("repeat migration: %+v %v", result, err)
	}
	var unchanged time.Time
	if err := admin.QueryRow(ctx, "SELECT installed_at FROM hp_iam.schema_version").Scan(&unchanged); err != nil || !unchanged.Equal(installed) {
		t.Fatal("repeat recreated the version record")
	}
}

func TestIAMSchemaMigrationAtomicDDLFailure(t *testing.T) {
	ctx, admin, connect := migrationDatabase(t)
	// Fail after earlier tables and the version row have been created. The exact
	// migration bytes remain unchanged; the test-only trigger belongs to this DB.
	_, err := admin.Exec(ctx, `CREATE FUNCTION public.hit81_reject_sessions() RETURNS event_trigger
		LANGUAGE plpgsql AS $$ BEGIN
		IF EXISTS(SELECT 1 FROM pg_event_trigger_ddl_commands() WHERE object_identity='hp_iam.sessions') THEN
			RAISE EXCEPTION 'HIT81_SYNTHETIC_DDL_FAILURE';
		END IF; END $$;
		CREATE EVENT TRIGGER hit81_reject_sessions ON ddl_command_end
		WHEN TAG IN ('CREATE TABLE') EXECUTE FUNCTION public.hit81_reject_sessions()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, connect()); !errors.Is(err, ErrSQL) {
		t.Fatalf("expected exact SQL failure, got %v", err)
	}
	var absent bool
	if err := admin.QueryRow(ctx, "SELECT to_regnamespace('hp_iam') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("failed migration left partially committed schema")
	}
	if _, err := admin.Exec(ctx, "DROP EVENT TRIGGER hit81_reject_sessions; DROP FUNCTION public.hit81_reject_sessions()"); err != nil {
		t.Fatal(err)
	}
	if result, err := Apply(ctx, connect()); err != nil || result.State != "installed" {
		t.Fatalf("clean retry after rolled back DDL: %+v %v", result, err)
	}
}

func TestIAMSchemaMigrationRefusesInvalidExistingState(t *testing.T) {
	for _, scenario := range []string{"foreign_owner", "partial_schema", "unknown_version", "missing_rls", "weakened_policy", "widened_grant"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, admin, connect := migrationDatabase(t)
			switch scenario {
			case "foreign_owner":
				if _, err := admin.Exec(ctx, "CREATE SCHEMA hp_iam"); err != nil {
					t.Fatal(err)
				}
			case "partial_schema":
				if _, err := admin.Exec(ctx, "CREATE SCHEMA hp_iam AUTHORIZATION hp_iam_migrator"); err != nil {
					t.Fatal(err)
				}
			default:
				if _, err := Apply(ctx, connect()); err != nil {
					t.Fatal(err)
				}
				if scenario == "unknown_version" {
					_, err := admin.Exec(ctx, "ALTER TABLE hp_iam.schema_version DROP CONSTRAINT schema_version_version_check; UPDATE hp_iam.schema_version SET version=2")
					if err != nil {
						t.Fatal(err)
					}
				} else {
					statement := map[string]string{
						"missing_rls":     "ALTER TABLE hp_iam.workspaces DISABLE ROW LEVEL SECURITY",
						"weakened_policy": "ALTER POLICY tenant ON hp_iam.workspaces USING(true) WITH CHECK(true)",
						"widened_grant":   "GRANT DELETE ON hp_iam.audit_minimal TO hp_iam_app",
					}[scenario]
					if _, err := admin.Exec(ctx, statement); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := Apply(ctx, connect()); !errors.Is(err, ErrSchema) {
				t.Fatalf("unsafe existing state was not refused: %v", err)
			}
		})
	}
}

func TestIAMSchemaMigrationRefusesRuntimeAndOpenTransaction(t *testing.T) {
	ctx, _, connect := migrationDatabase(t)
	owner := connect()
	if _, err := owner.Exec(ctx, "BEGIN"); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, owner); !errors.Is(err, ErrConnection) {
		t.Fatalf("accepted an outer transaction: %v", err)
	}
	if _, err := owner.Exec(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	runtimeCfg, err := pgx.ParseConfig(os.Getenv("HITPIPE_IAM_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	runtimeCfg.Database = owner.Config().Database
	runtime, err := pgx.ConnectConfig(ctx, runtimeCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close(ctx)
	if _, err := Apply(ctx, runtime); !errors.Is(err, ErrRole) {
		t.Fatalf("runtime was allowed to migrate: %v", err)
	}
}

func TestIAMSchemaMigrationRejectsUnsafeDefaultPrivileges(t *testing.T) {
	ctx, admin, connect := migrationDatabase(t)
	if _, err := admin.Exec(ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE hp_iam_migrator GRANT DELETE ON TABLES TO hp_iam_app"); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, connect()); !errors.Is(err, ErrDefaults) {
		t.Fatalf("unsafe defaults were allowed: %v", err)
	}
	var absent bool
	if err := admin.QueryRow(ctx, "SELECT to_regnamespace('hp_iam') IS NULL").Scan(&absent); err != nil || !absent {
		t.Fatal("unsafe defaults caused schema changes")
	}
}

func TestIAMSchemaMigrationConcurrentInstall(t *testing.T) {
	ctx, _, connect := migrationDatabase(t)
	connections := []*pgx.Conn{connect(), connect()}
	results := make(chan Result, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for _, conn := range connections {
		workers.Add(1)
		go func(c *pgx.Conn) {
			defer workers.Done()
			r, err := Apply(ctx, c)
			results <- r
			errors <- err
		}(conn)
	}
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	states := map[string]int{}
	for result := range results {
		states[result.State]++
	}
	if states["installed"] != 1 || states["already_installed"] != 1 {
		t.Fatalf("concurrent install was not serialized: %v", states)
	}
}
