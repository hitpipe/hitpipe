# Полные исходники HIT-128 на c426dc400a4bec03fafba72e2a5780c9d85af1e2

Ниже байты четырёх добавленных файлов из проверенного коммита. Номер первой строки каждого блока — 1. Изменённые строки существующего runtime_process_test.go входят в отдельно зарегистрированный exact-source.diff. Патч содержит все пять изменённых файлов.

SHA-256 патча `e09ecf6812fb3bce67a6c4ba9ddf05cabd923f84c82c1572b33e95110c21fbd1`. Применение через `git apply --cached --unidiff-zero` к индексу базы `19831026d635dd74d4b54dd4acd93e53319c182e` завершилось с кодом 0 и дало Git tree `ac69c7a9cce91c70fdbfe31a38ab725927defdff`, равный tree проверенного коммита.

## services/processing/scripts/recovery/verify_recovery.py

SHA-256 `9898ac32f0b9eea496e6e23ab88a8e0ee8b43b1f49f77830a531f6a67a3f11c1`, 4394 байт.

~~~~python
#!/usr/bin/env python3
"""Qualify Processing recovery in two private PostgreSQL clusters."""
import argparse
import hashlib
import json
import pathlib
import tempfile
import sys

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parents[1] / "postgres"))
import verify


class Run(verify.Run):
    def __init__(self, output, go, binaries):
        super().__init__(output)
        self.name = self.name.replace("hit76-", "hit128-", 1)
        self.db = "hit78"
        self.go = str(go.resolve())
        self.binaries = pathlib.Path(binaries)
        self.script_sha256 = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()

    def command(self, label, argv, *args, **kwargs):
        if label == "container-create":
            argv = ["hitpipe.task=HIT-128" if a == "hitpipe.task=HIT-76" else a for a in argv]
            argv[2:2] = ["--mount", f"type=bind,source={self.binaries},target=/test,readonly"]
        return super().command(label, argv, *args, **kwargs)

    def checks(self):
        go = ["env", "GOTOOLCHAIN=local", "GOWORK=off", self.go]
        version = self.command("go-version", [*go, "version"]).stdout.split()
        if version[2] != b"go1.27.2":
            raise RuntimeError("the runner requires Go 1.27.2")
        go += ["-C", str(verify.MODULE)]
        self.command("go-race", [*go, "test", "-race", "-count=1", "./..."])
        self.command("go-vet", [*go, "vet", "./..."])
        arch = self.command("container-arch", ["docker", "exec", self.container, "uname", "-m"])
        target = {"aarch64": "arm64", "x86_64": "amd64"}[arch.stdout.decode().strip()]
        linux = ["env", "GOOS=linux", "GOARCH=" + target, "CGO_ENABLED=0", *go]
        hashes = {}
        for name in ("processing", "processing-purge"):
            path = self.binaries / name
            self.command("build-" + name, [*linux, "build", "-trimpath", "-ldflags",
                         "-X main.revision=" + self.source, "-o", str(path), "./cmd/" + name])
            hashes[name] = hashlib.sha256(path.read_bytes()).hexdigest()
        path = self.binaries / "storage.test"
        self.command("build-recovery-tests",
                     [*linux, "test", "-c", "-o", str(path), "./internal/adapters/postgres"])
        hashes[path.name] = hashlib.sha256(path.read_bytes()).hexdigest()
        (self.output / "binaries.json").write_text(json.dumps({
            "source_sha": self.source, "platform": "linux/" + target, "sha256": hashes,
        }, indent=2) + "\n")
        self.command("recovery-process-tests", [
            "docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT128_RECOVERY_STAND=1",
            "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge",
            self.container, "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=110s",
            "-test.run=^TestProcessingRecoveryOwnedStand$",
        ])
        self.command("predecessor-process-tests", [
            "docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing",
            "-e", "HIT126_PURGE_BIN=/test/processing-purge", self.container, "/test/storage.test",
            "-test.v", "-test.count=1", "-test.timeout=145s",
            "-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$",
        ])

    def main(self):
        if self.tree_status:
            raise RuntimeError("use a clean checkout and an output directory outside it")
        super().main()
        path = self.output / "summary.json"
        summary = json.loads(path.read_text())
        summary.update(task="HIT-128", scope="Two real PostgreSQL 18.7 clusters inside one disposable, "
                       "network-none stand. Isolated Linux Processing and purge, controlled mTLS neighbors. "
                       "Selective in-memory SQL recovery only; no expiring file copy or operational RTO.")
        path.write_text(json.dumps(summary, indent=2) + "\n")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--go", type=pathlib.Path, required=True)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="hit128-build-") as binaries:
        Run(args.output.resolve(), args.go, binaries).main()
~~~~

## services/processing/internal/adapters/postgres/recovery_sql_test.go

SHA-256 `de27f8cc4330ee20521c9aa082abb9db87f34cbe5c4e869883b3850e706e6bab`, 5255 байт.

~~~~go
package postgres

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// These projections are the local qualification inventory. They deliberately
// omit drafts, notifications, runtime leases and the rebuildable job index.
// Content columns are opaque refs and their original integrity/expiry metadata.
type recoveryTable struct {
	name, columns, read string
}

var recoveryTables = []recoveryTable{
	{"operations", "workspace_id,operation_id,owner_user_id,revision,state,first_activation_at,selected_config_id,pause_accepted_at,drain_deadline_at", ""},
	{"accepted_configs", "workspace_id,config_id,operation_id,schema_version,evaluator_version,canonical_config,digest,accepted_by,accepted_at,rehearsal_digest", ""},
	{"case_identity", "workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,ever_dispatch_fenced,outcome_knowledge,lineage_id,known_task_id,last_confirmed_at", ""},
	{"episodes", "workspace_id,episode_id,case_id,started_at,deadline_at,automatic_writes,qualified_manual_command_id,completed_at", ""},
	{"case_content", "workspace_id,content_id,case_id,field_key,value_ref,value_sha256,serialized_value_bytes,author_user_id,added_at,expires_at", ""},
	{"safety_history", "workspace_id,entry_id,case_id,actor_kind,actor_id,at,reason_code,old_state,new_state,proof_ref", ""},
	{"history_content", "workspace_id,entry_id,detail_ref,detail_sha256,detail_bytes,created_at,expires_at", ""},
	{"user_commands", "workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at", ""},
	{"inbox", "workspace_id,message_id,digest,disposition,record_id,received_at", ""},
	{
		"outbox",
		"workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at,state,claim_token,lease_until,delivery_count",
		"workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at," +
			"CASE WHEN state IN ('PENDING','CLAIMED') THEN 'HELD' ELSE state END,NULL::uuid,NULL::timestamptz,delivery_count",
	},
}

const recoveryRowLimit = 1000 // bounded, local fixture only

type recoveryCopy struct {
	table recoveryTable
	rows  [][]any
}

func captureRecovery(t *testing.T, source *pgx.Conn, workspace string) []recoveryCopy {
	t.Helper()
	tx, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", workspace); err != nil {
		t.Fatal(err)
	}
	result := make([]recoveryCopy, 0, len(recoveryTables))
	for _, table := range recoveryTables {
		projection := table.read
		if projection == "" {
			projection = table.columns
		}
		// Tables and projections above are fixed in the test. RLS permits only
		// the explicit scope set for this transaction.
		rows, err := tx.Query(ctx, "SELECT "+projection+" FROM hp_processing."+table.name+" LIMIT $1", recoveryRowLimit+1)
		if err != nil {
			t.Fatalf("%s: %v", table.name, err)
		}
		data, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]any, error) {
			return row.Values()
		})
		if err != nil || len(data) > recoveryRowLimit {
			t.Fatalf("%s: scope exceeds the local qualification or cannot be read (%v)", table.name, err)
		}
		result = append(result, recoveryCopy{table, data})
		t.Logf("scoped SELECT hp_processing.%s: rows=%d", table.name, len(data))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result
}

var errRecoveryNotEmpty = errors.New("target workspace is not empty")

func applyRecovery(target *pgx.Conn, workspace string, archive []recoveryCopy) error {
	tx, err := target.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(128, 1)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", workspace); err != nil {
		return err
	}
	// The restore is only for the new isolated target. Refuse a second import
	// or a target that already has live safety/expiry state in this scope.
	for _, table := range recoveryTables {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hp_processing."+table.name+")").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w: %s", errRecoveryNotEmpty, table.name)
		}
	}
	// Insert through ordinary RLS and table constraints, in a single
	// transaction. The selected_config FK is already deferred by the schema.
	for _, part := range archive {
		table := part.table
		markers := make([]string, len(strings.Split(table.columns, ",")))
		for i := range markers {
			markers[i] = "$" + strconv.Itoa(i+1)
		}
		insert := "INSERT INTO hp_processing." + table.name + " (" + table.columns + ") VALUES (" + strings.Join(markers, ",") + ")"
		for _, row := range part.rows {
			tag, err := tx.Exec(ctx, insert, row...)
			if err != nil {
				return fmt.Errorf("restore %s: %w", table.name, err)
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("restore %s: row not inserted", table.name)
			}
		}
	}
	return tx.Commit(ctx)
}
~~~~

## services/processing/internal/adapters/postgres/recovery_cluster_test.go

SHA-256 `ee2b27da000b2c5b90cfa3bdb2524107fcdd052d34d7eb30bd0b8a8709dc1f46`, 5593 байт.

~~~~go
package postgres

import (
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const recoveryHost = "127.0.0.5"
const recoveryPort = "55432"
const recoveryDB = "hit128-restore"

type recoveryCluster struct {
	socket, ca string
}

func (c recoveryCluster) unixDSN(role, database string) string {
	return "host=" + c.socket + " port=" + recoveryPort + " dbname=" + database + " sslmode=disable user=" + role
}

func (c recoveryCluster) tlsDSN(role string) string {
	return "host=" + recoveryHost + " port=" + recoveryPort + " dbname=" + recoveryDB +
		" sslmode=verify-full sslrootcert=" + c.ca + " user=" + role
}

func recoveryCommand(t *testing.T, cmd *exec.Cmd) []byte {
	t.Helper()
	start := cmd.String()
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", start, err, raw)
	}
	t.Logf("exit=0 %s", start)
	return raw
}

func createRecoveryCluster(t *testing.T, stand *processStand) recoveryCluster {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "hit128-target-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	mustWrite := func(name string, raw []byte) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	server := stand.pki.Issue(t, "postgres-restore.test", net.ParseIP(recoveryHost), true, false)
	cert := mustWrite("server.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate[0]}))
	_, key := stand.keypair("postgres-restore", server)
	keyBytes, err := os.ReadFile(key)
	if err != nil {
		t.Fatal(err)
	}
	private := mustWrite("server.key", keyBytes)
	ca := stand.file("pg-restore-ca.pem", stand.pki.CAPEM())
	recoveryCommand(t, exec.Command("chown", "-R", "postgres:postgres", root))
	pgBin := "/usr/lib/postgresql/18/bin"
	run := func(binary string, args ...string) []byte {
		t.Helper()
		return recoveryCommand(t, exec.Command("runuser", append([]string{"-u", "postgres", "--", filepath.Join(pgBin, binary)}, args...)...))
	}
	data := filepath.Join(root, "data")
	run("initdb", "-D", data, "--auth-local=trust", "--auth-host=trust")
	options := "-k " + root + " -p " + recoveryPort + " -h " + recoveryHost +
		" -c fsync=on -c full_page_writes=on -c synchronous_commit=on -c ssl=on" +
		" -c ssl_cert_file=" + cert + " -c ssl_key_file=" + private
	run("pg_ctl", "-D", data, "-l", filepath.Join(root, "postgres.log"), "-o", options, "-w", "start")
	t.Cleanup(func() { run("pg_ctl", "-D", data, "-m", "fast", "-w", "stop") })
	// Both clusters use the same pinned distribution and migration. The second
	// has a freshly initialized data directory, WAL and system identifier.
	cluster := recoveryCluster{socket: root, ca: ca}
	psql := func(user, database string, args ...string) []byte {
		t.Helper()
		conn := cluster.unixDSN(user, database)
		return recoveryCommand(t, exec.Command("psql", append([]string{"-X", "-v", "ON_ERROR_STOP=1", "-d", conn}, args...)...))
	}
	psql("postgres", "postgres", "-f", "/migrations/roles.sql")
	db := pgx.Identifier{recoveryDB}.Sanitize()
	psql("postgres", "postgres", "-c", "CREATE DATABASE "+db+" OWNER hp_processing_owner")
	psql("postgres", "postgres", "-c", "REVOKE ALL ON DATABASE "+db+" FROM PUBLIC; GRANT CONNECT ON DATABASE "+db+" TO hp_processing_app,hp_processing_migrator,hp_processing_worker,hp_processing_purge")
	psql("hp_processing_migrator", recoveryDB, "-f", "/migrations/apply.sql")

	check := "SELECT system_identifier FROM pg_control_system()"
	sourceID := recoveryCommand(t, exec.Command("psql", "-XAt", "-U", "postgres", "-d", "postgres", "-c", check))
	targetID := psql("postgres", "postgres", "-At", "-c", check)
	if strings.TrimSpace(string(sourceID)) == strings.TrimSpace(string(targetID)) {
		t.Fatal("target must be a separately initialized PostgreSQL cluster")
	}
	t.Logf("different cluster system identifiers: source=%s target=%s",
		strings.TrimSpace(string(sourceID)), strings.TrimSpace(string(targetID)))
	return cluster
}

func (c recoveryCluster) connect(t *testing.T, role string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, c.tlsDSN(role))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	var version, fsync, writes, sync string
	err = conn.QueryRow(ctx, "SELECT current_setting('server_version_num'),current_setting('fsync'),current_setting('full_page_writes'),current_setting('synchronous_commit')").Scan(&version, &fsync, &writes, &sync)
	if err != nil || version != "180007" || fsync != "on" || writes != "on" || sync != "on" {
		t.Fatal("unexpected target PostgreSQL settings", version, fsync, writes, sync, err)
	}
	secured, ok := conn.PgConn().Conn().(*tls.Conn)
	if !ok || len(secured.ConnectionState().VerifiedChains) == 0 {
		t.Fatal("target PostgreSQL identity not verified over TLS")
	}
	t.Log("target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified")
	return conn
}

func (c recoveryCluster) runtimeConfig(t *testing.T, stand *processStand) {
	t.Helper()
	raw, err := os.ReadFile(stand.config)
	if err != nil {
		t.Fatal(err)
	}
	var conf map[string]any
	if err = json.Unmarshal(raw, &conf); err != nil {
		t.Fatal(err)
	}
	conf["database_name"] = recoveryDB
	conf["application_dsn_file"] = stand.file("restore-app.dsn", []byte(c.tlsDSN("hp_processing_app")))
	conf["worker_dsn_file"] = stand.file("restore-worker.dsn", []byte(c.tlsDSN("hp_processing_worker")))
	stand.config = stand.json("restore-processing.json", conf)
}
~~~~

## services/processing/internal/adapters/postgres/recovery_process_test.go

SHA-256 `8abbaf0d1b427b046704be9dbad0610f89aa7441043a00e3f0d401634156853a`, 19401 байт.

~~~~go
package postgres

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

func recoverExpiredDetail(t *testing.T, tx pgx.Tx, files *contentfiles.Store, ws, cid, entry string) preparecase.Ref {
	t.Helper()
	at := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Hour)
	expiry := at.Add(time.Hour)
	ref, err := files.Write(ctx, []byte(`{"controlled":true}`), "HISTORY",
		preparecase.Origin{WorkspaceID: ws, CaseID: cid, Revision: 1}, expiry, at)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history
 (workspace_id,entry_id,case_id,actor_kind,at,reason_code,old_state,new_state,proof_ref)
 VALUES($1,$2,$3,'SYSTEM',$4,'RECOVERY_EXPIRED_FIXTURE','UNKNOWN','UNKNOWN',$5)`, ws, entry, cid, at, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		ws, entry, ref.ID, ref.Digest, ref.Bytes, at, ref.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestRecoveryCrashChild(t *testing.T) {
	if os.Getenv("HIT128_RECOVERY_STAND") != "1" || os.Getenv("HIT128_CRASH_CHILD") != "1" {
		t.Skip("the crash is only run by the disposable recovery stand")
	}
	if len(os.Args) < 5 {
		t.Fatal("missing controlled crash arguments")
	}
	args := os.Args[len(os.Args)-5:]
	path, ws, cid, entry, marker := args[0], args[1], args[2], args[3], args[4]
	files, err := contentfiles.New(path)
	if err != nil {
		t.Fatal(err)
	}
	tx := begin(t, connect(t), ws)
	ref := recoverExpiredDetail(t, tx, files, ws, cid, entry)
	// The file and directory have been fsynced; the SQL transaction remains
	// open and never reaches COMMIT. The parent checks both sides of this cut.
	if err = os.WriteFile(marker, []byte(ref.ID), 0600); err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	t.Fatal("SIGKILL was not delivered")
}

func recoveringCrash(t *testing.T, files, ws, cid string, app *pgx.Conn) string {
	t.Helper()
	entry, _ := newID()
	path := filepath.Join(t.TempDir(), "orphan-ref")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryCrashChild$", "--", files, ws, cid, entry, path)
	cmd.Env = append(os.Environ(), "HIT128_CRASH_CHILD=1")
	raw, err := cmd.CombinedOutput()
	var failure *exec.ExitError
	if !errors.As(err, &failure) || failure.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("child did not reach the commit boundary: %v\n%s", err, raw)
	}
	id, err := os.ReadFile(path)
	if err != nil || !drafts.UUID(string(id)) {
		t.Fatal("orphan marker missing", err)
	}
	path = filepath.Join(files, string(id))
	if _, err = os.Stat(path); err != nil {
		t.Fatal("durable orphan did not survive SIGKILL", err)
	}
	tx := begin(t, app, ws)
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM hp_processing.history_content WHERE entry_id=$1)
 + (SELECT count(*) FROM hp_processing.safety_history WHERE entry_id=$1)`, entry).Scan(&count); err != nil || count != 0 {
		t.Fatal("uncommitted SQL survived the process kill", count, err)
	}
	t.Log("SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent")
	return path
}

func stopRecoveryProcess(t *testing.T, p *processingChild) {
	t.Helper()
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var e *exec.ExitError
	if err := <-p.done; !errors.As(err, &e) || e.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatal("expected SIGKILL when stopping Processing", err)
	}
	t.Log("Processing stopped by SIGKILL")
}

type recoveryIdentity struct {
	Operation, Config, Digest, Case, Lineage, Target, State, Knowledge string
	Fence                                                              bool
	Cases, Commands, Inbox, Intents, Queries                           int
}

func restoredIdentity(t *testing.T, db *pgx.Conn, ws, cid string) recoveryIdentity {
	t.Helper()
	tx := begin(t, db, ws)
	defer tx.Rollback(ctx)
	var s recoveryIdentity
	err := tx.QueryRow(ctx, `SELECT k.operation_id::text,k.config_id::text,c.digest,k.case_id::text,
 k.lineage_id::text,k.known_task_id,k.state,k.outcome_knowledge,k.ever_dispatch_fenced,
 (SELECT count(*) FROM hp_processing.case_identity),
 (SELECT count(*) FROM hp_processing.user_commands),
 (SELECT count(*) FROM hp_processing.inbox),
 (SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_INTENT'),
 (SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_QUERY')
 FROM hp_processing.case_identity k JOIN hp_processing.accepted_configs c USING(workspace_id,config_id)
 WHERE k.case_id=$1`, cid).Scan(&s.Operation, &s.Config, &s.Digest, &s.Case, &s.Lineage, &s.Target,
		&s.State, &s.Knowledge, &s.Fence, &s.Cases, &s.Commands, &s.Inbox, &s.Intents, &s.Queries)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type recoveryOutbox struct {
	state    string
	attempts int
	lease    bool
	expiry   time.Time
}

func recoveredOutbox(t *testing.T, db *pgx.Conn, ws, id string) recoveryOutbox {
	t.Helper()
	tx := begin(t, db, ws)
	defer tx.Rollback(ctx)
	var q recoveryOutbox
	err := tx.QueryRow(ctx, `SELECT state,delivery_count,claim_token IS NOT NULL OR lease_until IS NOT NULL,expires_at
 FROM hp_processing.outbox WHERE message_id=$1`, id).Scan(&q.state, &q.attempts, &q.lease, &q.expiry)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func (s *processStand) recoveryHealth(t *testing.T, p *processingChild, label, state string) {
	t.Helper()
	code, raw := s.pollHealth(p, state)
	t.Logf("%s OPS mTLS HTTP %d %s", label, code, bytes.TrimSpace(raw))
}

func (s *processStand) recoveryWaitRegistration(t *testing.T, previous int64) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for s.registrations.Load() <= previous {
		if time.Now().After(until) {
			t.Fatal("stand IAM did not receive the registration")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The IAM handler published the response. Allow its client to apply it
	// before the test reads Health, rather than asserting against a race.
	time.Sleep(100 * time.Millisecond)
}

func TestProcessingRecoveryOwnedStand(t *testing.T) {
	binary, purge := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT126_PURGE_BIN")
	if os.Getenv("HIT128_RECOVERY_STAND") != "1" || binary == "" || purge == "" {
		t.Skip("requires the private HIT-128 PostgreSQL stand and built commands")
	}
	started := time.Now()
	f := newObservation(t)
	ws, cid := f.request.WorkspaceID, f.request.CaseID
	app := connect(t)
	other := prepared(t, []string{}).request
	stand := createProcessStand(t, binary, f.path, map[string]string{ws: "acct_synthetic", other.WorkspaceID: "acct_synthetic"})
	stand.waitDelivery.Store(true)
	stand.queryReply = func(w http.ResponseWriter, r *http.Request) {
		// Observe a real outbound query without returning an observation/ACK.
		<-r.Context().Done()
	}
	proc := stand.boot("recovery-source.log")
	stand.recoveryHealth(t, proc, "original", "CLOSED")
	stand.iamOpen.Store(true)
	stand.recoveryHealth(t, proc, "original", "READY")
	select {
	case got := <-stand.deliverySeen:
		if got != f.lineage {
			t.Fatal("unexpected first create request")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original create did not reach the controlled peer")
	}
	// Check the actual Intake route for the already known payment session.
	// This new signal creates an inbox receipt on the original case.
	eventAt := time.Now().UTC()
	signal := intakeWire(t, ws, func(wire map[string]any) {
		wire["source_created_at"] = eventAt.Add(-time.Hour).Format(time.RFC3339Nano)
		wire["first_received_at"] = eventAt.Format(time.RFC3339Nano)
		wire["expires_at"] = eventAt.Add(time.Hour).Format(time.RFC3339Nano)
	})
	admitted := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", signal, nil, 202)
	var intakeRecord struct {
		RecordID string `json:"record_id"`
	}
	if json.Unmarshal(admitted, &intakeRecord) != nil || intakeRecord.RecordID != cid {
		t.Fatal("duplicate payment session did not resolve to the original case")
	}

	// Its HTTP response is still withheld. Inject a separately authenticated
	// known-target observation, then a real user command for read-only recheck.
	obs := observationWire(t, f.observation(t, "TARGET_ID_OBSERVED"))
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", obs, nil, 202)
	_, _, revision, _, _, _ := f.stored(t)
	commandID, _ := newID()
	command := recheckCommand(t, commandID, revision)
	caseURL := "/api/v1/workspaces/" + ws + "/cases/" + cid
	headers := map[string]string{"X-HitPipe-Session": stand.token}
	stand.command(caseURL+"/recovery", command, commandID, 202)
	stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)

	// Crash one file/SQL writer, then add separate committed metadata for an
	// expired detail. Both file types must survive export without being copied.
	orphan := recoveringCrash(t, f.path, ws, cid, app)
	expiredEntry, _ := newID()
	tx := begin(t, app, ws)
	expired := recoverExpiredDetail(t, tx, f.files, ws, cid, expiredEntry)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	stand.bootMu.Lock()
	boot := stand.current
	stand.bootMu.Unlock()
	revocation, _ := newID()
	barrier := drafts.BarrierCommand{TargetRuntimeID: stand.identity, BootID: boot.BootID, Generation: boot.Generation,
		RevocationID: revocation, WorkspaceID: &other.WorkspaceID, Scope: "WORKSPACE", SubjectID: other.WorkspaceID, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	ack := stand.must("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", marshalProcess(t, barrier), nil, 200)
	if draftjson.Validate("BarrierAck", ack) != nil {
		t.Fatal("invalid actual barrier acknowledgement")
	}
	stand.bootMu.Lock()
	stand.denials = append(stand.denials, drafts.DenyEpochSnapshot{WorkspaceID: &other.WorkspaceID, Scope: "WORKSPACE", SubjectID: other.WorkspaceID,
		Epoch: 8, RevocationID: revocation, AcceptedAt: time.Now().Add(-time.Second).UTC()})
	stand.bootMu.Unlock()
	stopRecoveryProcess(t, proc)
	stand.iamStale.Store(true)
	cold := stand.boot("recovery-same-db.log")
	stand.recoveryHealth(t, cold, "cold restart, same database", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	stopRecoveryProcess(t, cold)

	before := restoredIdentity(t, app, ws, cid)
	if !before.Fence || before.State != "UNKNOWN" || before.Knowledge != "TARGET_ID_KNOWN" ||
		before.Cases != 1 || before.Commands != 1 || before.Inbox != 3 || before.Intents != 1 || before.Queries != 1 {
		t.Fatal("unexpected source safety summary", before.Fence, before.State, before.Knowledge,
			before.Cases, before.Commands, before.Inbox, before.Intents, before.Queries)
	}
	first := recoveredOutbox(t, app, ws, f.lineage)
	if first.state != "CLAIMED" || !first.lease || first.attempts != 1 {
		t.Fatal("original effect did not reach the interrupted transport state", first)
	}
	tx = begin(t, app, ws)
	oldClaim := dispatchintent.Claim{Request: dispatchintent.Request{WorkspaceID: ws, Epoch: 7}, MessageID: f.lineage}
	if err := tx.QueryRow(ctx, `SELECT claim_token::text,lease_until FROM hp_processing.outbox WHERE message_id=$1`,
		f.lineage).Scan(&oldClaim.Token, &oldClaim.LeaseUntil); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	archive := captureRecovery(t, app, ws)
	stand.t.Logf("source intent state=%s lease=%t attempts=%d; known identity and fence persisted", first.state, first.lease, first.attempts)

	cluster := createRecoveryCluster(t, stand)
	target := cluster.connect(t, "hp_processing_app")
	if err := applyRecovery(target, ws, archive); err != nil {
		t.Fatal(err)
	}
	if err := applyRecovery(target, ws, archive); !errors.Is(err, errRecoveryNotEmpty) {
		t.Fatal("repeat restore must refuse non-empty target scope", err)
	}
	if err := applyRecovery(target, other.WorkspaceID, archive); err == nil {
		t.Fatal("RLS accepted a restore into a foreign workspace")
	}
	tx = begin(t, target, other.WorkspaceID)
	var foreign int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing.operations").Scan(&foreign); err != nil || foreign != 0 {
		t.Fatal("foreign restore was not atomic", foreign, err)
	}
	tx.Rollback(ctx)
	if after := restoredIdentity(t, target, ws, cid); before != after {
		t.Fatal("restored identity, command, inbox, outbox or fence differs")
	}
	held := recoveredOutbox(t, target, ws, f.lineage)
	if held.state != "HELD" || held.lease || held.attempts != first.attempts || !held.expiry.Equal(first.expiry) {
		t.Fatal("restore promoted or rearmed unfinished create", held)
	}
	query := recoveredOutbox(t, target, ws, commandID)
	if query.state != "HELD" || query.lease {
		t.Fatal("restore promoted unfinished recheck", query)
	}
	worker := DispatchStore{DB: cluster.connect(t, "hp_processing_worker")}
	if err := worker.Check(ctx, oldClaim); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("restored worker accepted an old job/claim or returned an unexpected error", err)
	}
	t.Log("restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back")

	cluster.runtimeConfig(t, stand)
	normal := stand.bootWith("recovery-normal-closed.log", "-config", stand.config)
	stand.recoveryHealth(t, normal, "ordinary configuration without authoritative stand inputs", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	stopRecoveryProcess(t, normal)

	previous := stand.registrations.Load()
	restored := stand.boot("recovery-target.log")
	stand.recoveryWaitRegistration(t, previous)
	stand.recoveryHealth(t, restored, "target with stale boot snapshot", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	stand.registerGeneration.Store(2)
	stand.iamStale.Store(false)
	stand.recoveryWaitRegistration(t, stand.registrations.Load())
	stand.recoveryHealth(t, restored, "target with wrong generation", "CLOSED")
	stand.registerGeneration.Store(0)
	stand.recoveryHealth(t, restored, "target with fresh boot and generation", "READY")
	revokedURL := "/api/v1/workspaces/" + other.WorkspaceID + "/cases/" + other.CaseID
	stand.must("bff.test", "8443", "GET", revokedURL, nil, headers, 403)
	stand.authorizationEpoch.Store(9)
	stand.must("bff.test", "8443", "GET", revokedURL, nil, headers, 403)
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 200)
	stand.command(caseURL+"/recovery", command, commandID, 202)
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", obs, nil, 202)
	replayed := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", signal, nil, 202)
	if !bytes.Equal(replayed, admitted) {
		t.Fatal("restored Intake replay lost the original receipt")
	}
	t.Log("fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded")
	history := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)
	var h readcases.HistoryView
	if json.Unmarshal(history, &h) != nil || len(h.Items) < 2 {
		t.Fatal("nonempty restored history unavailable")
	}
	expiredMetadata, valid := false, false
	for _, item := range h.Items {
		if item.EntryID == expiredEntry {
			expiredMetadata = item.EvidenceRef != nil && *item.EvidenceRef == expired.ID && item.ExpiresAt.Equal(expired.ExpiresAt)
		} else if item.EvidenceRef != nil {
			valid = true
		}
	}
	if !expiredMetadata || !valid {
		t.Fatal("did not retain live refs and expired metadata distinctly", expiredMetadata, valid)
	}
	stopRecoveryProcess(t, restored)

	tx = begin(t, target, ws)
	var activeRef string
	if err := tx.QueryRow(ctx, `SELECT detail_ref::text FROM hp_processing.history_content
 WHERE entry_id<>$1 AND expires_at>clock_timestamp() ORDER BY created_at LIMIT 1`, expiredEntry).Scan(&activeRef); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	activePath := filepath.Join(f.path, activeRef)
	saved := activePath + ".recovery-test"
	if err := os.Rename(activePath, saved); err != nil {
		t.Fatal(err)
	}
	fault := stand.boot("recovery-missing-content.log")
	stand.recoveryHealth(t, fault, "target, one missing detail", "READY")
	missing := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 503)
	if err := os.WriteFile(activePath, []byte("deliberately damaged synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	corrupt := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 503)
	if !bytes.Contains(missing, []byte("READ_UNAVAILABLE")) || !bytes.Contains(corrupt, []byte("READ_UNAVAILABLE")) {
		t.Fatal("unexpected missing or corrupt detail result")
	}
	t.Log("live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE")
	stopRecoveryProcess(t, fault)
	if err := os.Rename(saved, activePath); err != nil {
		t.Fatal(err)
	}
	if err := f.files.Close(); err != nil {
		t.Fatal(err)
	}
	purgeCmd := exec.Command(purge, "--local-stand", "--content-dir", f.path, "--workspace", ws, "--limit", "100")
	purgeCmd.Env = append(os.Environ(), "HITPIPE_PURGE_DSN="+cluster.tlsDSN("hp_processing_purge"))
	raw := recoveryCommand(t, purgeCmd)
	if !json.Valid(raw) {
		t.Fatal("unexpected cleanup output")
	}
	t.Logf("purge result: %s", strings.TrimSpace(string(raw)))
	for _, path := range []string{orphan, filepath.Join(f.path, expired.ID)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("expired file was not physically removed", err)
		}
	}
	if err := applyRecovery(target, ws, archive); !errors.Is(err, errRecoveryNotEmpty) {
		t.Fatal("post-purge replay should still refuse a nonempty target", err)
	}
	if _, err := os.Stat(filepath.Join(f.path, expired.ID)); !os.IsNotExist(err) {
		t.Fatal("restore resurrected an already removed file", err)
	}
	afterPurge := stand.boot("recovery-after-purge.log")
	stand.recoveryHealth(t, afterPurge, "target after expiry cleanup", "READY")
	still := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)
	var afterHistory readcases.HistoryView
	if json.Unmarshal(still, &afterHistory) != nil || !reflect.DeepEqual(h.Items, afterHistory.Items) {
		t.Fatal("expired file removal changed retained history metadata")
	}
	stopRecoveryProcess(t, afterPurge)
	t.Log("purge removed expired bytes while preserving the exact public history metadata and expiry")
	if after := restoredIdentity(t, target, ws, cid); before != after ||
		stand.deliveryCount.Load() != 1 || recoveredOutbox(t, target, ws, f.lineage) != held {
		t.Fatal("replay or purge changed safety/identity or repeated the external effect")
	}
	t.Logf("RECOVERY PASS: live provider create calls=1, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=%d", time.Since(started).Milliseconds())
}
~~~~
