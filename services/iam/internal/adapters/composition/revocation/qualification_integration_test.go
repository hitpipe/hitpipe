//go:build integration

package revocation_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	barrierhttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation"
	revocationdb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/revocation"
	runtimedb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgxpool"
)

type labDB struct{ admin, app *pgxpool.Pool }

func labDatabaseConfig(cfg *pgxpool.Config, user string) bool {
	return cfg != nil && cfg.ConnConfig.Host == "127.0.0.1" &&
		cfg.ConnConfig.User == user && strings.HasPrefix(cfg.ConnConfig.Database, "hit68_test_") &&
		cfg.ConnConfig.Password == "" && cfg.ConnConfig.TLSConfig == nil && len(cfg.ConnConfig.Fallbacks) == 0
}

func openLabDB(t *testing.T) labDB {
	t.Helper()
	adminDSN, appDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN"), os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("use the disposable PostgreSQL 18.7 environment from tools/check_authorization.py")
	}
	ac, errA := pgxpool.ParseConfig(adminDSN)
	pc, errP := pgxpool.ParseConfig(appDSN)
	if errA != nil || errP != nil || !labDatabaseConfig(ac, "postgres") || !labDatabaseConfig(pc, "hp_hit81_app") ||
		ac.ConnConfig.Database != pc.ConnConfig.Database || ac.ConnConfig.Port != pc.ConnConfig.Port {
		t.Fatal("require a password-free disposable loopback database and dedicated fixture roles; values withheld")
	}
	connect := func(cfg *pgxpool.Config) *pgxpool.Pool {
		cfg.MaxConns, cfg.MinConns = 6, 0
		p, err := pgxpool.NewWithConfig(t.Context(), cfg)
		if err != nil {
			t.Fatal("open the isolated PostgreSQL connection pool")
		}
		t.Cleanup(p.Close)
		return p
	}
	db := labDB{admin: connect(ac), app: connect(pc)}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	var version string
	var leastPrivilege, emptyRegistry bool
	if err := db.app.QueryRow(ctx, `SELECT current_setting('server_version'),
		current_user='hp_hit81_app' AND current_user=session_user AND pg_has_role(current_user,'hp_iam_app','MEMBER')
		AND NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole)),
		NOT EXISTS(SELECT 1 FROM hp_iam.runtime_registry)`).Scan(&version, &leastPrivilege, &emptyRegistry); err != nil ||
		version != "18.7" || !leastPrivilege || !emptyRegistry {
		t.Fatal("expected PostgreSQL 18.7 with least-privilege app and an isolated empty runtime registry")
	}
	t.Logf("postgres=%s app_role=hp_hit81_app registry_empty=%t", version, emptyRegistry)
	return db
}

func fixtureSQL(t *testing.T, p *pgxpool.Pool, q string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if _, err := p.Exec(ctx, q, args...); err != nil {
		t.Fatalf("disposable fixture SQL failed (%x); arguments withheld", sha256.Sum256([]byte(q)))
	}
}

func fixtureUUID(t *testing.T) string {
	t.Helper()
	var v [16]byte
	if _, err := rand.Read(v[:]); err != nil {
		t.Fatal("generate synthetic identity")
	}
	v[6] = (v[6] & 0x0f) | 0x40
	v[8] = (v[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", v[0:4], v[4:6], v[6:8], v[8:10], v[10:16])
}

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	path string
}

func keyAndSerial(t *testing.T) (*ecdsa.PrivateKey, *big.Int) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial, serr := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil || serr != nil {
		t.Fatal("generate ephemeral test certificates")
	}
	return key, serial.Add(serial, big.NewInt(1))
}

func file(t *testing.T, name string, value []byte) {
	t.Helper()
	if err := os.WriteFile(name, value, 0600); err != nil {
		t.Fatal("write isolated fixture file")
	}
}

func newCA(t *testing.T, dir, name string) testCA {
	t.Helper()
	key, serial := keyAndSerial(t)
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-10 * time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal("create isolated root")
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("parse isolated root")
	}
	name = filepath.Join(dir, name+".crt")
	file(t, name, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return testCA{cert: cert, key: key, path: name}
}

func (ca testCA) leaf(t *testing.T, dir, name, ip string, expired bool) (prefix, pin string) {
	t.Helper()
	key, serial := keyAndSerial(t)
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, DNSNames: []string{name + ".hit116.test"}}
	if ip != "" {
		tmpl.IPAddresses = []net.IP{net.ParseIP(ip)}
		tmpl.ExtKeyUsage = append(tmpl.ExtKeyUsage, x509.ExtKeyUsageServerAuth)
	}
	if expired {
		tmpl.NotBefore, tmpl.NotAfter = now.Add(-time.Hour), now.Add(-time.Minute)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal("sign isolated leaf")
	}
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal("encode isolated leaf")
	}
	prefix = filepath.Join(dir, name)
	file(t, prefix+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	file(t, prefix+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}))
	return prefix, fmt.Sprintf("sha256:%x", sha256.Sum256(der))
}

type childReady struct {
	PID      int
	Business string
}

type child struct {
	command *exec.Cmd
	logFile string
	done    chan struct{}
	result  error
}

func launch(t *testing.T, dir string, cfg processConfig) *child {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal("serialize fixture process configuration")
	}
	name := filepath.Join(dir, "config.json")
	file(t, name, raw)
	self, err := os.Executable()
	if err != nil {
		t.Fatal("locate race-enabled test binary for separate processes")
	}
	c := &child{command: exec.CommandContext(t.Context(), self, "-test.run=^TestHIT116Process$"),
		done: make(chan struct{}), logFile: filepath.Join(dir, "process.log")}
	c.command.Env = []string{childFile + "=" + name,
		"HITPIPE_IAM_TEST_APP_DSN=" + os.Getenv("HITPIPE_IAM_TEST_APP_DSN")}
	log, err := os.OpenFile(c.logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatal("open fixture process log")
	}
	c.command.Stdout, c.command.Stderr = log, log
	startErr := c.command.Start()
	_ = log.Close()
	if startErr != nil {
		t.Fatal("start independent fixture process")
	}
	go func() {
		c.result = c.command.Wait()
		close(c.done)
	}()
	t.Cleanup(func() { c.stop(t) })
	return c
}

func (c *child) stop(t *testing.T) {
	t.Helper()
	_ = c.command.Process.Kill()
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		t.Error("fixture process did not stop")
	}
}

func (c *child) await(t *testing.T, dir, name string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	path := filepath.Join(dir, name)
	if err := waitFile(ctx, path); err != nil {
		raw, _ := os.ReadFile(c.logFile)
		t.Fatalf("fixture process did not produce %s: %v\n%s", name, err, raw)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("read fixture process data")
	}
	return raw
}

func (c *child) complete(t *testing.T) {
	t.Helper()
	select {
	case <-c.done:
		if c.result != nil {
			raw, _ := os.ReadFile(c.logFile)
			t.Fatalf("restarted coordinator failed: %v\n%s", c.result, raw)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("restarted coordinator did not complete")
	}
}

func signal(t *testing.T, dir, name string) {
	t.Helper()
	if err := mark(dir, name, nil); err != nil {
		t.Fatal("signal isolated process")
	}
}

func runtimeIdentity(row runtimeaccess.Registration) startgate.Identity {
	return startgate.Identity{RuntimeID: row.RuntimeID, BootID: row.BootID, Generation: uint64(row.Generation)}
}

func ready(t *testing.T, c *child, dir string) childReady {
	t.Helper()
	var data childReady
	if err := json.Unmarshal(c.await(t, dir, "ready"), &data); err != nil || data.PID != c.command.Process.Pid || data.PID == os.Getpid() {
		t.Fatal("fixture did not demonstrate an independent OS process")
	}
	return data
}

func businessStatus(ctx context.Context, url string) (int, error) {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Do(r)
	if err != nil {
		return 0, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	err = resp.Body.Close()
	return resp.StatusCode, err
}

func probe(t *testing.T, target childReady, path string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if got, err := businessStatus(ctx, target.Business+path); err != nil || got != want {
		t.Fatalf("PID %d protected probe %s: got %d want %d; %v", target.PID, path, got, want, err)
	}
}

func newBarrierClient(t *testing.T, leaf, ca string, targets map[string]string) *barrierhttp.Client {
	t.Helper()
	cert, err := readLeaf(leaf)
	if err != nil {
		t.Fatal("read ephemeral client certificate")
	}
	roots, err := readCA(ca)
	if err != nil {
		t.Fatal("read ephemeral server trust roots")
	}
	client, err := barrierhttp.NewClient(cert, roots, targets)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type sendFunc func(context.Context, revocation.Participant, startgate.BarrierCommand) (startgate.BarrierAck, error)

func (f sendFunc) Send(ctx context.Context, target revocation.Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
	return f(ctx, target, cmd)
}

func verifyBeforeSend(ctx context.Context, db labDB, saved revocation.Record, participants []runtimeaccess.Registration) error {
	// A separate database connection must see the committed deny, epoch and
	// complete identity roster before any HTTP send is even attempted.
	var status string
	var epoch, entityEpoch, total, exact int64
	var workspace *string
	var deny bool
	a, b := participants[0], participants[1]
	err := db.admin.QueryRow(ctx, `SELECT r.status,r.epoch,r.workspace_id::text,
		CASE WHEN r.scope='SESSION' THEN (SELECT epoch FROM hp_iam.sessions WHERE session_id=r.subject_id)
			ELSE (SELECT security_epoch FROM hp_iam.workspaces WHERE workspace_id=r.subject_id) END,
		CASE WHEN r.scope='SESSION' THEN (SELECT revoked_at IS NOT NULL FROM hp_iam.sessions WHERE session_id=r.subject_id)
			ELSE (r.workspace_id=r.subject_id) END,
		(SELECT count(*) FROM hp_iam.revocation_participants p WHERE p.revocation_id=r.revocation_id),
		(SELECT count(*) FROM hp_iam.revocation_participants p WHERE p.revocation_id=r.revocation_id
		  AND p.workspace_id IS NOT DISTINCT FROM r.workspace_id
		  AND ((p.runtime_id=$2 AND p.generation=$3 AND p.boot_id=$4)
		    OR (p.runtime_id=$5 AND p.generation=$6 AND p.boot_id=$7)))
		FROM hp_iam.revocations r WHERE r.revocation_id=$1`,
		saved.ID, a.RuntimeID, a.Generation, a.BootID, b.RuntimeID, b.Generation, b.BootID,
	).Scan(&status, &epoch, &workspace, &entityEpoch, &deny, &total, &exact)
	wantWorkspace := saved.Subject.WorkspaceID
	sameWorkspace := workspace == nil && wantWorkspace == nil || workspace != nil && wantWorkspace != nil && *workspace == *wantWorkspace
	if err != nil || !sameWorkspace || status != string(revocation.Pending) || !deny ||
		epoch != int64(saved.Epoch) || entityEpoch != epoch || total != 2 || exact != total {
		return errors.New("network would precede an exact durable PENDING/deny/roster")
	}
	return nil
}

func note(t *testing.T, label string, result revocation.Record) {
	t.Helper()
	type proof struct {
		RuntimeID string     `json:"runtime_id"`
		Boot      string     `json:"boot_id"`
		Gen       uint64     `json:"generation"`
		Epoch     uint64     `json:"applied_epoch"`
		ACK       *time.Time `json:"acknowledged_at"`
	}
	roster := make([]proof, 0, len(result.Participants))
	for _, target := range result.Participants {
		roster = append(roster, proof{target.Identity.RuntimeID, target.Identity.BootID,
			target.Identity.Generation, target.AppliedEpoch, target.AcknowledgedAt})
	}
	encoded, err := json.Marshal(roster)
	if err != nil {
		t.Fatal("render minimized fixture proof")
	}
	t.Logf("%s id=%s scope=%s workspace_set=%t epoch=%d accepted=%s deadline=%s state=%s effective=%v incident=%t deadline_breached=%t missing=%v roster=%s",
		label, result.ID, result.Subject.Scope, result.Subject.WorkspaceID != nil, result.Epoch, result.AcceptedAt.Format(time.RFC3339Nano),
		result.DeadlineAt.Format(time.RFC3339Nano), result.Status, result.EffectiveAt, result.IncidentID != nil,
		result.DeadlineBreachedAt != nil, result.MissingRuntimeIDs(), encoded)
}

func get(t *testing.T, store *revocationdb.Store, id string) revocation.Record {
	t.Helper()
	actual, err := store.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return actual
}

func awaitRecord(t *testing.T, store *revocationdb.Store, id string, match func(revocation.Record) bool) revocation.Record {
	t.Helper()
	start := time.Now()
	for time.Since(start) < 1400*time.Millisecond {
		if record := get(t, store, id); match(record) {
			return record
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected durable partial acknowledgement before the in-flight section is released")
	return revocation.Record{}
}

func afterDeadline(t *testing.T, record revocation.Record) {
	t.Helper()
	timer := time.NewTimer(time.Until(record.DeadlineAt) + 50*time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-t.Context().Done():
		t.Fatal("coordinator lab deadline unexpectedly cancelled")
	}
}

func logWire(t *testing.T, dir, id string, count int) {
	t.Helper()
	bytes, err := os.ReadFile(filepath.Join(dir, "wire-"+id))
	hashes := strings.Fields(string(bytes))
	if err != nil || len(hashes) != count {
		t.Fatalf("expected %d real TLS requests for exact operation, got %d", count, len(hashes))
	}
	for _, actual := range hashes {
		if actual != hashes[0] || len(actual) != 64 {
			t.Fatal("network replay changed the encoded command")
		}
	}
	t.Logf("wire revocation=%s attempts=%d exact_command_sha256=%s", id, count, hashes[0])
}

func TestPostgresRevocationMultiProcess(t *testing.T) {
	db := openLabDB(t)
	ctx := t.Context()
	revDB, err := revocationdb.New(db.app)
	if err != nil {
		t.Fatal(err)
	}
	s1, s2, s3, ws, otherWS, principal := fixtureUUID(t), fixtureUUID(t), fixtureUUID(t), fixtureUUID(t), fixtureUUID(t), fixtureUUID(t)
	sessionOp, workspaceOp, stalledOp := fixtureUUID(t), fixtureUUID(t), fixtureUUID(t)
	sess := startgate.Subject{Scope: startgate.Session, SubjectID: s1}
	workspace := startgate.Subject{Scope: startgate.Workspace, SubjectID: ws, WorkspaceID: &ws}
	stalled := startgate.Subject{Scope: startgate.Session, SubjectID: s3}
	clock := time.Now().UTC()
	ids := []string{s1, s2, s3}
	workspaces := []string{ws, otherWS}
	t.Cleanup(func() {
		for _, id := range []string{sessionOp, workspaceOp, stalledOp} {
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.revocation_participants WHERE revocation_id=$1`, id)
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.revocations WHERE revocation_id=$1`, id)
		}
	})
	fixtureSQL(t, db.admin, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
		VALUES($1,'https://fixture.invalid',$2,$3)`, principal, principal, clock.Add(-2*time.Minute))
	// Subject fixtures are deleted after transactions; the dev container is
	// disposed regardless of the test outcome.
	t.Cleanup(func() {
		for _, id := range ids {
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.sessions WHERE session_id=$1`, id)
		}
		for _, id := range workspaces {
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.workspaces WHERE workspace_id=$1`, id)
		}
		fixtureSQL(t, db.admin, `DELETE FROM hp_iam.principals WHERE principal_id=$1`, principal)
	})
	for _, id := range workspaces {
		fixtureSQL(t, db.admin, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
			VALUES($1,$2,1,7,'ACTIVE',$3,$3)`, id, fixtureUUID(t), clock)
	}
	for _, id := range ids {
		s, csrf := sha256.Sum256([]byte(id+" synthetic IAM")), sha256.Sum256([]byte(id+" synthetic CSRF"))
		fixtureSQL(t, db.admin, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
			VALUES($1,$2,$3,$4,$5,$6,$7,3)`, id, s[:], principal, csrf[:], clock.Add(-time.Minute), clock.Add(time.Hour), clock.Add(30*time.Minute))
	}

	certDir := t.TempDir()
	iamCA, runtimeCA, foreignCA := newCA(t, certDir, "iam-ca"), newCA(t, certDir, "runtime-ca"), newCA(t, certDir, "foreign-ca")
	iamLeaf, iamPin := iamCA.leaf(t, certDir, "iam", "", false)
	foreignLeaf, _ := foreignCA.leaf(t, certDir, "foreign", "", false)
	peerLeaf, _ := iamCA.leaf(t, certDir, "not-iam", "", false)
	expiredLeaf, _ := iamCA.leaf(t, certDir, "expired", "", true)
	addrs := []string{"127.119.116.4", "127.119.116.5"}
	targets := make(map[string]string, 2)
	inventory := make([]runtimeaccess.Registration, 2)
	configs := make([]targetProcess, 2)
	access := func(session, scope string) startgate.StartRequest {
		return startgate.StartRequest{Authorities: []startgate.Authority{
			{Subject: startgate.Subject{Scope: startgate.Session, SubjectID: session, WorkspaceID: &scope}, Epoch: 3, Allowed: true},
			{Subject: startgate.Subject{Scope: startgate.Workspace, SubjectID: scope, WorkspaceID: &scope}, Epoch: 7, Allowed: true},
		}}
	}
	probes := map[string]startgate.StartRequest{
		"/inflight": access(s1, ws), "/session": access(s1, ws), "/session-other-workspace": access(s1, otherWS),
		"/workspace": access(s2, ws), "/unaffected": access(s2, otherWS), "/stalled": access(s3, otherWS),
	}
	runtimeIDs := make([]string, 2)
	// HIT-115's accepted local opening is intentionally IAM-only. These
	// two instances qualify the protocol independently, without claiming
	// that either is an already wired Intake/Processing service.
	for i := range addrs {
		leaf, pin := runtimeCA.leaf(t, certDir, fmt.Sprintf("runtime-%d", i+1), addrs[i], false)
		reg := runtimeaccess.Registration{RuntimeID: fixtureUUID(t), BootID: fixtureUUID(t),
			Generation: 1, Service: "IAM", CertificateFingerprint: pin}
		t.Cleanup(func() {
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.revocation_participants WHERE runtime_id=$1`, reg.RuntimeID)
			fixtureSQL(t, db.admin, `DELETE FROM hp_iam.runtime_registry WHERE runtime_id=$1`, reg.RuntimeID)
		})
		runtimeIDs[i] = reg.RuntimeID
		inventory[i], targets[reg.RuntimeID] = reg, "https://"+net.JoinHostPort(addrs[i], "9443")
		configs[i] = targetProcess{Dir: t.TempDir(), Address: addrs[i], IAMCA: iamCA.path, IAMPin: iamPin,
			Leaf: leaf, Inventory: inventory, Own: reg, Probes: probes}
	}
	configs[0].DropRevocation, configs[1].DelayRevocation = workspaceOp, stalledOp
	regDB, err := runtimedb.New(db.app, inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, reg := range inventory {
		if _, err := regDB.Register(ctx, reg); err != nil {
			t.Fatal(err)
		}
	}
	first := launch(t, configs[0].Dir, processConfig{Target: &configs[0]})
	second := launch(t, configs[1].Dir, processConfig{Target: &configs[1]})
	readyA, readyB := ready(t, first, configs[0].Dir), ready(t, second, configs[1].Dir)
	if readyA.PID == readyB.PID {
		t.Fatal("runtime PIDs must differ")
	}
	t.Logf("coordinator_pid=%d managed_iam_runtime_pids=[%d,%d] real_control_port=9443", os.Getpid(), readyA.PID, readyB.PID)
	for i, c := range []*child{first, second} {
		probe(t, []childReady{readyA, readyB}[i], "/unaffected", http.StatusServiceUnavailable)
		signal(t, configs[i].Dir, "register")
		if string(c.await(t, configs[i].Dir, "registration")) != "open" {
			t.Fatal("first boot didn't perform accepted registration")
		}
		probe(t, []childReady{readyA, readyB}[i], "/unaffected", http.StatusCreated)
	}

	client := newBarrierClient(t, iamLeaf, runtimeCA.path, targets)
	// Reject a foreign CA, a different leaf signed by IAM's own CA, and an
	// expired leaf. These calls contain only synthetic fixture identities.
	expected := revocation.Participant{Identity: runtimeIdentity(inventory[0]), CertificateFingerprint: inventory[0].CertificateFingerprint}
	testCommand := startgate.BarrierCommand{
		TargetRuntimeID: expected.Identity.RuntimeID, Generation: expected.Identity.Generation, BootID: expected.Identity.BootID,
		RevocationID: fixtureUUID(t), Subject: sess, Epoch: 4, DeadlineAt: time.Now().UTC().Add(4 * time.Second),
	}
	for _, leaf := range []string{foreignLeaf, peerLeaf, expiredLeaf} {
		// Use a raw client so even certificates rejected by outbound domain
		// validation are actually presented to the server handshake.
		if rawTLS(t, ctx, leaf, runtimeCA.path, addrs[0]) {
			t.Fatal("private control listener accepted a non-IAM, alternate, or expired certificate")
		}
	}
	wrongPin := expected
	wrongPin.CertificateFingerprint = inventory[1].CertificateFingerprint
	wrongBoot := testCommand
	wrongBoot.BootID = inventory[1].BootID
	wrongBootParticipant := expected
	wrongBootParticipant.Identity.BootID = wrongBoot.BootID
	for _, check := range []struct {
		p   revocation.Participant
		cmd startgate.BarrierCommand
	}{{wrongPin, testCommand}, {wrongBootParticipant, wrongBoot}} {
		var ack startgate.BarrierAck
		timeout, cancel := context.WithTimeout(ctx, time.Second)
		ack, err := client.Send(timeout, check.p, check.cmd)
		cancel()
		if err == nil || ack.RevocationID != "" {
			t.Fatal("wrong pin or command tuple did not fail closed")
		}
	}
	logWire(t, configs[0].Dir, testCommand.RevocationID, 1)
	probe(t, readyA, "/session", http.StatusCreated)
	t.Log("mTLS rejects foreign CA, alternate IAM-CA leaf, expired leaf and wrong runtime pin; wrong boot yields no ACK")

	var sendChecks sync.Map
	coordinator, err := revocation.New(revDB, sendFunc(func(ctx context.Context, target revocation.Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
		rec, err := revDB.Get(ctx, cmd.RevocationID)
		if err != nil {
			return startgate.BarrierAck{}, err
		}
		err = verifyBeforeSend(ctx, db, rec, inventory)
		sendChecks.Store(cmd.RevocationID+"/"+target.Identity.RuntimeID, err == nil)
		if err != nil {
			return startgate.BarrierAck{}, err
		}
		return client.Send(ctx, target, cmd)
	}), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	begin := func(id string, subject startgate.Subject, epoch uint64) revocation.Record {
		t.Helper()
		got, err := coordinator.Begin(ctx, revocation.Intent{ID: id, Subject: subject, ReasonCode: "HIT116_LAB"})
		if err != nil || got.Status != revocation.Pending || got.Epoch != epoch || got.EffectiveAt != nil ||
			len(got.MissingRuntimeIDs()) != 2 || got.DeadlineAt.Sub(got.AcceptedAt) != 5*time.Second {
			t.Fatalf("expected exact committed PENDING and five-second deadline: %v", err)
		}
		if err := verifyBeforeSend(ctx, db, got, inventory); err != nil {
			t.Fatal(err)
		}
		note(t, "durable-before-send", got)
		return got
	}
	drive := func(id string) revocation.Record {
		t.Helper()
		got, err := coordinator.Drive(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	type operationResult struct {
		code int
		err  error
	}
	businessDone := make(chan operationResult, 1)
	go func() {
		code, err := businessStatus(ctx, readyA.Business+"/inflight")
		businessDone <- operationResult{code, err}
	}()
	entered := first.await(t, configs[0].Dir, "inflight-entered")
	session := begin(sessionOp, sess, 4)
	type driveResult struct {
		value revocation.Record
		err   error
	}
	driven := make(chan driveResult, 1)
	go func() {
		value, err := coordinator.Drive(ctx, sessionOp)
		driven <- driveResult{value, err}
	}()
	partial := awaitRecord(t, revDB, sessionOp, func(record revocation.Record) bool {
		return slices.Equal(record.MissingRuntimeIDs(), []string{runtimeIDs[0]})
	})
	if partial.Status != revocation.Pending || partial.EffectiveAt != nil {
		t.Fatal("barrier finished while one runtime still held the critical actual-start section")
	}
	note(t, "parallel-ack-while-other-start-section-held", partial)
	signal(t, configs[0].Dir, "release-start")
	var complete revocation.Record
	select {
	case got := <-driven:
		if got.err != nil || got.value.Status != revocation.Effective || got.value.EffectiveAt == nil ||
			len(got.value.MissingRuntimeIDs()) != 0 || !got.value.EffectiveAt.Before(session.DeadlineAt) {
			t.Fatalf("session required all exact ACKs within five seconds: %v", got.err)
		}
		complete = got.value
	case <-time.After(4 * time.Second):
		t.Fatal("parallel revocation did not finish")
	}
	note(t, "all-ack-session", complete)
	for _, r := range []childReady{readyA, readyB} {
		probe(t, r, "/session", http.StatusForbidden)
		probe(t, r, "/session-other-workspace", http.StatusForbidden)
		probe(t, r, "/unaffected", http.StatusCreated)
	}
	select {
	case <-businessDone:
		t.Fatal("a released actual-start section was confused with completed work")
	default:
	}
	signal(t, configs[0].Dir, "finish-action")
	select {
	case final := <-businessDone:
		if final.err != nil || final.code != http.StatusCreated {
			t.Fatal("a legitimate earlier action could not finish after ACK")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("legitimate earlier action never finished")
	}
	t.Logf("pre-barrier actual start=%s completed after all ACK; new global session starts denied across workspaces", entered)

	lost := begin(workspaceOp, workspace, 8)
	missing := drive(workspaceOp)
	if missing.Status != revocation.Pending || missing.EffectiveAt != nil || !slices.Equal(missing.MissingRuntimeIDs(), runtimeIDs[:1]) {
		t.Fatal("a lost real HTTP response must leave one durable missing ACK")
	}
	note(t, "lost-response-but-one-ack-durable", missing)
	// B still has no local SESSION-to-WORKSPACE leak; isolating the two
	// constraints is observable despite the lost acknowledgement at A.
	for _, r := range []childReady{readyA, readyB} {
		probe(t, r, "/workspace", http.StatusForbidden)
		probe(t, r, "/unaffected", http.StatusCreated)
	}
	droppedWire := first.await(t, configs[0].Dir, "ack-dropped")
	var dropped struct {
		At    time.Time `json:"acknowledged_at"`
		Epoch uint64    `json:"applied_epoch"`
	}
	if err := json.Unmarshal(droppedWire, &dropped); err != nil || !dropped.At.Before(lost.DeadlineAt) || dropped.Epoch != lost.Epoch {
		t.Fatal("could not observe authentic ACK produced before its response was dropped")
	}
	afterDeadline(t, lost)
	recoverFromFreshProcess := func(observe string) *child {
		t.Helper()
		dir := t.TempDir()
		restart := &recoveryProcess{Dir: dir, Leaf: iamLeaf, CAs: runtimeCA.path, Targets: targets, ObserveIncident: observe}
		c := launch(t, dir, processConfig{Recovery: restart})
		c.complete(t)
		if observe != "" {
			var incident revocation.Record
			raw := c.await(t, dir, "before-replay")
			if err := json.Unmarshal(raw, &incident); err != nil || incident.ID != observe {
				t.Fatal("independent recovery process did not record the pending incident before replay")
			}
			note(t, "restart-observed-deadline-incident-before-network", incident)
		}
		t.Logf("coordinator recovery pid=%d database=existing-records", c.command.Process.Pid)
		return c
	}
	recoverFromFreshProcess(workspaceOp)
	recovered := get(t, revDB, workspaceOp)
	if recovered.Status != revocation.Effective || recovered.EffectiveAt == nil || recovered.IncidentID == nil ||
		recovered.DeadlineBreachedAt == nil || len(recovered.MissingRuntimeIDs()) != 0 {
		t.Fatal("on-time ACK recovered after deadline must complete while preserving the incident")
	}
	recoveredOriginal := false
	for _, p := range recovered.Participants {
		if p.Identity.RuntimeID == runtimeIDs[0] && p.AcknowledgedAt != nil && p.AcknowledgedAt.Equal(revocation.NormalizeTime(dropped.At)) {
			recoveredOriginal = true
		}
	}
	if !recoveredOriginal {
		t.Fatal("a fresh late ACK was substituted for the original lost response")
	}
	note(t, "exact-on-time-ack-replayed-after-restart", recovered)
	logWire(t, configs[0].Dir, workspaceOp, 2)
	logWire(t, configs[1].Dir, workspaceOp, 1)

	delayed := begin(stalledOp, stalled, 4)
	startedDelay := time.Now()
	stillPending := drive(stalledOp)
	deliveredAfter := time.Since(startedDelay)
	if stillPending.EffectiveAt != nil || !slices.Equal(stillPending.MissingRuntimeIDs(), runtimeIDs[1:]) {
		t.Fatal("a delayed necessary node must not appear effective")
	}
	second.await(t, configs[1].Dir, "network-delay-armed")
	second.await(t, configs[1].Dir, "network-delay-cancelled")
	t.Logf("real control request cancelled after %s; deadline %s stays unchanged", deliveredAfter.Round(time.Millisecond),
		delayed.DeadlineAt.Format(time.RFC3339Nano))
	afterDeadline(t, delayed)
	recoverFromFreshProcess(stalledOp)
	late := get(t, revDB, stalledOp)
	if late.Status != revocation.Incident || late.EffectiveAt != nil || late.IncidentID == nil ||
		late.DeadlineBreachedAt == nil || !slices.Equal(late.MissingRuntimeIDs(), runtimeIDs[1:]) {
		t.Fatal("late never-applied command manufactured an ACK")
	}
	note(t, "real-network-delay-no-late-ack", late)
	logWire(t, configs[1].Dir, stalledOp, 2)
	probe(t, readyA, "/stalled", http.StatusForbidden)
	probe(t, readyB, "/stalled", http.StatusForbidden)
	// A late command may install a deny, but it cannot assert timely drain.
	second.stop(t)
	fixtureSQL(t, db.admin, `UPDATE hp_iam.runtime_registry SET last_observed_at=now()-interval '30 minutes'
		WHERE runtime_id=$1`, runtimeIDs[1])
	boot := configs[1]
	boot.Dir, boot.Own.BootID, boot.Own.Generation = t.TempDir(), fixtureUUID(t), 2
	boot.Inventory = append([]runtimeaccess.Registration(nil), inventory...)
	boot.Inventory[1] = boot.Own
	b := launch(t, boot.Dir, processConfig{Target: &boot})
	fresh := ready(t, b, boot.Dir)
	if fresh.PID == readyB.PID {
		t.Fatal("expected a genuinely new boot process")
	}
	probe(t, fresh, "/unaffected", http.StatusServiceUnavailable)
	signal(t, boot.Dir, "register")
	if string(b.await(t, boot.Dir, "registration")) != "conflict" {
		t.Fatal("new process may not replace the old tuple without a real fence")
	}
	probe(t, fresh, "/unaffected", http.StatusServiceUnavailable)
	recoverFromFreshProcess(stalledOp)
	afterRestart := get(t, revDB, stalledOp)
	if afterRestart.Status != revocation.Incident || afterRestart.EffectiveAt != nil ||
		!slices.Equal(afterRestart.MissingRuntimeIDs(), runtimeIDs[1:]) {
		t.Fatal("restart/lease expiry improperly acted as an old-boot ACK or fence")
	}
	note(t, "replacement-boot-closed-old-roster-unfulfilled", afterRestart)
	logWire(t, boot.Dir, stalledOp, 1)
	for _, id := range []string{sessionOp, workspaceOp, stalledOp} {
		for _, runtime := range runtimeIDs {
			ok, present := sendChecks.Load(id + "/" + runtime)
			if !present || ok != true {
				t.Fatal("at least one initial network send lacked the separate committed SQL check")
			}
		}
	}
}

// rawTLS verifies handshake rejection independently of the product client's own
// input checks. It does not send a control command on an accepted connection.
func rawTLS(t *testing.T, ctx context.Context, file, ca, ip string) bool {
	t.Helper()
	cert, err := readLeaf(file)
	if err != nil {
		t.Fatal("read independent TLS fixture")
	}
	roots, err := readCA(ca)
	if err != nil {
		t.Fatal("read runtime CA fixture")
	}
	dialer := tls.Dialer{Config: &tls.Config{
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: roots, ServerName: ip,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) { return &cert, nil },
	}}
	budget, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	raw, err := dialer.DialContext(budget, "tcp4", net.JoinHostPort(ip, "9443"))
	if err != nil {
		return false
	}
	defer raw.Close()
	// A TLS 1.3 client's Handshake can return before the server sends its alert;
	// the first authenticated HTTP exchange proves the final acceptance state.
	_ = raw.SetDeadline(time.Now().Add(800 * time.Millisecond))
	if _, err := fmt.Fprintf(raw, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", ip); err != nil {
		return false
	}
	var firstByte [1]byte
	_, err = io.ReadFull(raw, firstByte[:])
	return err == nil
}
