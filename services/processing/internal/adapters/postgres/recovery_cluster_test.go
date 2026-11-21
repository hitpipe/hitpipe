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
