//go:build integration

package privateiam_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	wired "github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgxpool"
)

type labDatabase struct {
	admin, app *pgxpool.Pool
}

func openLabDatabase(t *testing.T) labDatabase {
	t.Helper()
	adminDSN := os.Getenv("HITPIPE_IAM_TEST_ADMIN_DSN")
	appDSN := os.Getenv("HITPIPE_IAM_TEST_APP_DSN")
	if adminDSN == "" || appDSN == "" {
		t.Skip("run tools/check_authorization.py in an isolated lab to provide PostgreSQL 18.7 roles")
	}
	adminConfig, adminError := pgxpool.ParseConfig(adminDSN)
	appConfig, appError := pgxpool.ParseConfig(appDSN)
	if adminError != nil || appError != nil {
		t.Fatal("invalid isolated PostgreSQL configuration; values withheld")
	}
	for _, config := range []*pgxpool.Config{adminConfig, appConfig} {
		if config.ConnConfig.Host != "127.0.0.1" || !strings.HasPrefix(config.ConnConfig.Database, "hit68_test_") {
			t.Fatal("HIT-115 integration requires a disposable loopback PostgreSQL database")
		}
		config.MaxConns, config.MinConns = 5, 0
	}
	if adminConfig.ConnConfig.User != "postgres" || appConfig.ConnConfig.User != "hp_hit81_app" ||
		adminConfig.ConnConfig.Database != appConfig.ConnConfig.Database || adminConfig.ConnConfig.Port != appConfig.ConnConfig.Port {
		t.Fatal("the fixture administrator and runtime application must use separate roles in the same test database")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, adminConfig)
	if err != nil {
		t.Fatal("could not open the isolated fixture administrator pool")
	}
	t.Cleanup(admin.Close)
	app, err := pgxpool.NewWithConfig(ctx, appConfig)
	if err != nil {
		t.Fatal("could not open the isolated IAM application pool")
	}
	t.Cleanup(app.Close)
	var acceptedVersion, applicationRole bool
	err = app.QueryRow(ctx, `SELECT current_setting('server_version_num') = '180007',
 current_user = 'hp_hit81_app' AND current_user = session_user
 AND pg_has_role(current_user,'hp_iam_app','MEMBER')
 AND NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname=current_user AND (rolsuper OR rolbypassrls OR rolcreaterole))`).Scan(&acceptedVersion, &applicationRole)
	if err != nil || !acceptedVersion || !applicationRole {
		t.Fatal("the actual PostgreSQL server version or least-privilege application role was not verified")
	}
	return labDatabase{admin: admin, app: app}
}

func labUUID() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	value[6], value[8] = value[6]&0x0f|0x40, value[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[:4], value[4:6], value[6:8], value[8:10], value[10:])
}

func sqlExec(t *testing.T, pool *pgxpool.Pool, statement string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, statement, args...); err != nil {
		t.Fatalf("isolated fixture statement failed (sha256:%x); SQL arguments withheld", sha256.Sum256([]byte(statement)))
	}
}

type labData struct {
	now                                  time.Time
	workspace, otherWorkspace, principal string
	session, opaqueSession, resource     string
	localRuntime, callerRuntime          string
	localBoot, callerBoot                string
}

func seedLabData(t *testing.T, db labDatabase) labData {
	t.Helper()
	f := labData{
		now:       time.Now().UTC().Truncate(time.Microsecond),
		workspace: labUUID(), otherWorkspace: labUUID(), principal: labUUID(),
		session: labUUID(), resource: labUUID(),
		localRuntime: labUUID(), callerRuntime: labUUID(), localBoot: labUUID(), callerBoot: labUUID(),
	}
	var secret [32]byte
	_, _ = rand.Read(secret[:])
	f.opaqueSession = base64.RawURLEncoding.EncodeToString(secret[:])
	digest := sha256.Sum256([]byte(f.opaqueSession))
	csrf := sha256.Sum256([]byte("hit115-fixture-csrf-" + labUUID()))
	// Register before seeding so partial setup is also removed on failure.
	t.Cleanup(func() {
		for _, step := range []struct {
			sql  string
			args []any
		}{
			{`DELETE FROM hp_iam.revocation_participants WHERE runtime_id IN ($1,$2)`, []any{f.localRuntime, f.callerRuntime}},
			{`DELETE FROM hp_iam.revocations WHERE workspace_id IN ($1,$2) OR subject_id=$3`, []any{f.workspace, f.otherWorkspace, f.session}},
			{`DELETE FROM hp_iam.runtime_registry WHERE runtime_id IN ($1,$2)`, []any{f.localRuntime, f.callerRuntime}},
			{`DELETE FROM hp_iam.memberships WHERE principal_id=$1`, []any{f.principal}},
			{`DELETE FROM hp_iam.workspaces WHERE workspace_id IN ($1,$2)`, []any{f.workspace, f.otherWorkspace}},
			{`DELETE FROM hp_iam.sessions WHERE session_id=$1`, []any{f.session}},
			{`DELETE FROM hp_iam.principals WHERE principal_id=$1`, []any{f.principal}},
		} {
			sqlExec(t, db.admin, step.sql, step.args...)
		}
	})
	sqlExec(t, db.admin, `INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at)
 VALUES($1::uuid,'https://fixture.invalid',$2::text,$3)`, f.principal, f.principal, f.now.Add(-time.Minute))
	sqlExec(t, db.admin, `INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch)
 VALUES($1,$2,$3,$4,$5,$6,$7,3)`, f.session, digest[:], f.principal, csrf[:], f.now.Add(-time.Minute), f.now.Add(time.Hour), f.now.Add(30*time.Minute))
	for _, workspace := range []string{f.workspace, f.otherWorkspace} {
		sqlExec(t, db.admin, `INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
 VALUES($1,$2,1,7,'ACTIVE',$3,$3)`, workspace, labUUID(), f.now)
	}
	sqlExec(t, db.admin, `INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at)
 VALUES($1,$2,'OWNER','ACTIVE',1,$3)`, f.workspace, f.principal, f.now)
	return f
}

type labClock struct{ at time.Time }

func (c labClock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	if err := ctx.Err(); err != nil {
		return sessionpolicy.Observation{}, err
	}
	return sessionpolicy.Observation{At: c.at, Trusted: true}, nil
}

type labOwner struct {
	data labData
	wait func(context.Context) error
}

func (o labOwner) Prove(ctx context.Context, actor authorizeaction.Actor, request authorizeaction.Request) (authorizeaction.Proof, error) {
	if o.wait != nil {
		if err := o.wait(ctx); err != nil {
			return authorizeaction.Proof{}, err
		}
	}
	if actor.PrincipalID != o.data.principal || actor.SessionID != o.data.session || actor.WorkspaceID != o.data.workspace ||
		request.WorkspaceID != o.data.workspace || request.Action != "CASE_READ" || request.ResourceID == nil || *request.ResourceID != o.data.resource {
		return authorizeaction.Proof{}, nil
	}
	return authorizeaction.Proof{Verified: true, PrincipalID: actor.PrincipalID, WorkspaceID: actor.WorkspaceID,
		ResourceID: request.ResourceID, Action: request.Action, Operation: "getCase"}, nil
}

type labCA struct {
	issuer *x509.Certificate
	key    *ecdsa.PrivateKey
	roots  *x509.CertPool
}

func newLabCA(t *testing.T) labCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("could not generate isolated CA key")
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT-115 isolated CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal("could not sign isolated CA certificate")
	}
	issuer, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("could not load isolated CA certificate")
	}
	roots := x509.NewCertPool()
	roots.AddCert(issuer)
	return labCA{issuer: issuer, key: key, roots: roots}
}

func (c labCA) leaf(t *testing.T, dns string, usage x509.ExtKeyUsage, expired bool) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal("could not generate isolated TLS peer key")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil || serial.Sign() == 0 {
		t.Fatal("could not generate isolated TLS certificate identity")
	}
	now := time.Now()
	cert := &x509.Certificate{SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(30 * time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	if dns != "" {
		cert.DNSNames = []string{dns}
	}
	if usage == x509.ExtKeyUsageServerAuth {
		cert.IPAddresses = []net.IP{net.IPv4(127, 0, 0, 1)}
	}
	if expired {
		cert.NotBefore, cert.NotAfter = now.Add(-time.Hour), now.Add(-time.Minute)
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, c.issuer, &key.PublicKey, c.key)
	if err != nil {
		t.Fatal("could not sign isolated TLS peer certificate")
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func fingerprint(cert tls.Certificate) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(cert.Certificate[0]))
}

const (
	registerPath  = "/internal/v1/runtime/register"
	authorizePath = "/internal/v1/authorize"
	localDNS      = "iam.hit115.test"
	remoteDNS     = "bff.hit115.test"
)

type labPeers struct {
	ca               labCA
	server, iam, bff tls.Certificate
	local, remote    runtimeaccess.Binding
}

func newLabPeers(t *testing.T, f labData) labPeers {
	t.Helper()
	ca := newLabCA(t)
	server := ca.leaf(t, "", x509.ExtKeyUsageServerAuth, false)
	iam := ca.leaf(t, localDNS, x509.ExtKeyUsageClientAuth, false)
	bff := ca.leaf(t, remoteDNS, x509.ExtKeyUsageClientAuth, false)
	return labPeers{
		ca: ca, server: server, iam: iam, bff: bff,
		local: runtimeaccess.Binding{DNSName: localDNS, Registration: runtimeaccess.Registration{
			RuntimeID: f.localRuntime, Service: "IAM", Generation: 1, BootID: f.localBoot, CertificateFingerprint: fingerprint(iam),
		}},
		remote: runtimeaccess.Binding{DNSName: remoteDNS, Registration: runtimeaccess.Registration{
			RuntimeID: f.callerRuntime, Service: "BFF", Generation: 1, BootID: f.callerBoot, CertificateFingerprint: fingerprint(bff),
		}},
	}
}

func serveLab(t *testing.T, service *wired.Server, peers labPeers, inventory []runtimeaccess.Binding) string {
	t.Helper()
	handler, err := transport.New(service)
	if err != nil {
		t.Fatal("could not construct the private IAM handler")
	}
	allow := make([]runtimeaccess.Peer, 0, len(inventory))
	for _, peer := range inventory {
		allow = append(allow, runtimeaccess.Peer{DNSName: peer.DNSName, CertificateFingerprint: peer.CertificateFingerprint})
	}
	config, err := transport.TLSConfig(peers.server, peers.ca.roots, allow)
	if err != nil {
		t.Fatal("could not construct the private IAM mTLS configuration")
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = config
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	t.Cleanup(server.Close)
	return server.URL
}

func labClient(t *testing.T, ca labCA, certificates ...tls.Certificate) *http.Client {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, RootCAs: ca.roots, Certificates: certificates,
	}}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

type result struct {
	status int
	body   []byte
	err    error
}

func post(ctx context.Context, client *http.Client, url string, body any, opaque string) result {
	data, err := json.Marshal(body)
	if err != nil {
		return result{err: err}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return result{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", labUUID())
	if opaque != "" {
		request.Header.Set("X-HitPipe-Session", opaque)
	}
	response, err := client.Do(request)
	if err != nil {
		return result{err: err}
	}
	defer response.Body.Close()
	read, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return result{status: response.StatusCode, body: read, err: err}
}

func assertDenied(t *testing.T, r result, forbidden string) {
	t.Helper()
	if r.err != nil || r.status < 400 || (r.status >= 500 && r.status != http.StatusServiceUnavailable) {
		t.Fatalf("expected a bounded HTTP refusal; HTTP status=%d", r.status)
	}
	var body struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal(r.body, &body) != nil || body.Code == "" || body.Message == "" || !runtimeaccess.ValidUUID(body.RequestID) ||
		forbidden != "" && bytes.Contains(r.body, []byte(forbidden)) || bytes.Contains(r.body, []byte(`"allowed":true`)) {
		t.Fatal("refusal did not use the contract error envelope or leaked authority")
	}
}

func assertRegistration(t *testing.T, r result, binding runtimeaccess.Binding) runtimeaccess.Snapshot {
	t.Helper()
	var snapshot runtimeaccess.Snapshot
	if r.err != nil || r.status != http.StatusOK || json.Unmarshal(r.body, &snapshot) != nil ||
		snapshot.RuntimeID != binding.RuntimeID || snapshot.BootID != binding.BootID || snapshot.Generation != binding.Generation ||
		snapshot.Revision < 1 || snapshot.Denials == nil {
		t.Fatalf("expected matching runtime snapshot; HTTP status=%d", r.status)
	}
	return snapshot
}

func authorizeRequest(f labData, runtime string) authorizeaction.Request {
	return authorizeaction.Request{WorkspaceID: f.workspace, RuntimeID: runtime, Action: "CASE_READ", ResourceID: &f.resource}
}

func assertAllowed(t *testing.T, r result, f labData, role string) {
	t.Helper()
	var reply authorizeaction.Authorization
	if r.err != nil || r.status != http.StatusOK || json.Unmarshal(r.body, &reply) != nil || !reply.Allowed ||
		reply.SessionID != f.session || reply.PrincipalID != f.principal || reply.WorkspaceID != f.workspace ||
		reply.Epoch != 7 || reply.Role != role || reply.RevocationPending || !reply.CheckedAt.Equal(f.now) ||
		bytes.Contains(r.body, []byte(f.opaqueSession)) {
		t.Fatalf("expected the actual session, principal, membership, and workspace authority; HTTP status=%d", r.status)
	}
}

func newLabApp(t *testing.T, db labDatabase, f labData, local runtimeaccess.Binding, inventory []runtimeaccess.Binding, owner labOwner) *wired.Server {
	t.Helper()
	service, err := wired.New(db.app, labClock{f.now}, owner, local.Registration, inventory)
	if err != nil {
		t.Fatal("could not compose private IAM with the real PostgreSQL pool")
	}
	return service
}

func checkRuntimeRow(t *testing.T, db labDatabase, peer runtimeaccess.Binding, status string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var rows int
	if err := db.admin.QueryRow(ctx, `SELECT count(*) FROM hp_iam.runtime_registry WHERE runtime_id=$1 AND service=$2
 AND generation=$3 AND boot_id=$4 AND certificate_fingerprint=$5 AND status=$6`,
		peer.RuntimeID, peer.Service, peer.Generation, peer.BootID, peer.CertificateFingerprint, status).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("expected exactly one persisted %s runtime with the confirmed identity", status)
	}
}

func TestPostgresPrivateIAMEndToEnd(t *testing.T) {
	db := openLabDatabase(t)
	f := seedLabData(t, db)
	peers := newLabPeers(t, f)
	inventory := []runtimeaccess.Binding{peers.local, peers.remote}
	service := newLabApp(t, db, f, peers.local, inventory, labOwner{data: f})
	base := serveLab(t, service, peers, inventory)
	iam, bff := labClient(t, peers.ca, peers.iam), labClient(t, peers.ca, peers.bff)
	req := authorizeRequest(f, peers.local.RuntimeID)
	ctx := t.Context()
	assertDenied(t, post(ctx, iam, base+authorizePath, req, f.opaqueSession), f.opaqueSession)

	initial := assertRegistration(t, post(ctx, iam, base+registerPath, peers.local.Registration, ""), peers.local)
	replay := assertRegistration(t, post(ctx, iam, base+registerPath, peers.local.Registration, ""), peers.local)
	if initial.Gate == "OPEN" || replay.Gate != initial.Gate || replay.Revision < initial.Revision {
		t.Fatal("repeat runtime registration opened or regressed an unapplied snapshot")
	}
	checkRuntimeRow(t, db, peers.local, initial.Gate)
	remote := assertRegistration(t, post(ctx, bff, base+registerPath, peers.remote.Registration, ""), peers.remote)
	if remote.Gate == "OPEN" {
		t.Fatal("remote registration invented proof of gate application")
	}
	checkRuntimeRow(t, db, peers.remote, remote.Gate)
	wrongBoot := peers.local.Registration
	wrongBoot.BootID = labUUID()
	assertDenied(t, post(ctx, iam, base+registerPath, wrongBoot, ""), f.opaqueSession)
	assertDenied(t, post(ctx, bff, base+registerPath, peers.local.Registration, ""), f.opaqueSession)
	assertDenied(t, post(ctx, iam, base+authorizePath, req, f.opaqueSession), f.opaqueSession)
	if err := service.Start(ctx); err != nil {
		t.Fatal("verified local IAM boot could not apply and persist its fresh snapshot")
	}
	checkRuntimeRow(t, db, peers.local, "OPEN")
	assertAllowed(t, post(ctx, iam, base+authorizePath, req, f.opaqueSession), f, "OWNER")

	assertDenied(t, post(ctx, iam, base+authorizePath, req, ""), f.opaqueSession)
	assertDenied(t, post(ctx, bff, base+authorizePath, req, f.opaqueSession), f.opaqueSession)
	remoteRequest := authorizeRequest(f, peers.remote.RuntimeID)
	assertDenied(t, post(ctx, bff, base+authorizePath, remoteRequest, f.opaqueSession), f.opaqueSession)

	remoteIdentity := startgate.Identity{
		RuntimeID: peers.remote.RuntimeID, BootID: peers.remote.BootID, Generation: uint64(peers.remote.Generation),
	}
	remoteGate, err := startgate.New(remoteIdentity)
	if err != nil {
		t.Fatal(err)
	}
	opened := assertRegistration(t, post(ctx, bff, base+registerPath, peers.remote.Registration, ""), peers.remote)
	if opened.Gate != "OPEN" || opened.Revision <= remote.Revision || len(opened.Denials) != 0 {
		t.Fatal("the enrolled remote boot did not receive a fresh OPEN snapshot for this empty fixture")
	}
	if state, err := remoteGate.State(ctx); err != nil || state != startgate.Closed {
		t.Fatal("issuing an OPEN snapshot cannot open the remote gate by itself")
	}
	if err := remoteGate.ApplySnapshot(ctx, startgate.RuntimeSnapshot{
		RuntimeID: opened.RuntimeID, BootID: opened.BootID, Generation: uint64(opened.Generation),
		SnapshotRevision: uint64(opened.Revision), Gate: startgate.State(opened.Gate),
	}); err != nil {
		t.Fatal(err)
	}
	if state, err := remoteGate.State(ctx); err != nil || state != startgate.Open {
		t.Fatal("the exact remote boot could not apply its OPEN snapshot")
	}
	checkRuntimeRow(t, db, peers.remote, "OPEN")
	assertAllowed(t, post(ctx, bff, base+authorizePath, remoteRequest, f.opaqueSession), f, "OWNER")
	wrongRemoteBoot := peers.remote.Registration
	wrongRemoteBoot.BootID = labUUID()
	assertDenied(t, post(ctx, bff, base+registerPath, wrongRemoteBoot, ""), f.opaqueSession)
	foreign := req
	foreign.WorkspaceID = f.otherWorkspace
	assertDenied(t, post(ctx, iam, base+authorizePath, foreign, f.opaqueSession), f.opaqueSession)
	foreign = req
	unknownResource := labUUID()
	foreign.ResourceID = &unknownResource
	assertDenied(t, post(ctx, iam, base+authorizePath, foreign, f.opaqueSession), f.opaqueSession)
	deferred := req
	deferred.Action = "CASE_CONTEXT_WRITE"
	assertDenied(t, post(ctx, iam, base+authorizePath, deferred, f.opaqueSession), f.opaqueSession)
	sqlExec(t, db.admin, `UPDATE hp_iam.memberships SET role='READER',revision=2 WHERE workspace_id=$1 AND principal_id=$2`, f.workspace, f.principal)
	assertAllowed(t, post(ctx, iam, base+authorizePath, req, f.opaqueSession), f, "READER")

	for _, bad := range [][]tls.Certificate{
		nil,
		{peers.ca.leaf(t, "unregistered.hit115.test", x509.ExtKeyUsageClientAuth, false)},
		{peers.ca.leaf(t, localDNS, x509.ExtKeyUsageClientAuth, false)},
		{peers.ca.leaf(t, localDNS, x509.ExtKeyUsageClientAuth, true)},
		{newLabCA(t).leaf(t, localDNS, x509.ExtKeyUsageClientAuth, false)},
	} {
		got := post(ctx, labClient(t, peers.ca, bad...), base+authorizePath, req, f.opaqueSession)
		if got.err == nil || got.status == http.StatusOK {
			t.Fatal("missing, expired or non-inventory TLS client identity did not fail handshake")
		}
	}
}

func TestPostgresPrivateIAMNormalListener(t *testing.T) {
	db := openLabDatabase(t)
	f := seedLabData(t, db)
	peers := newLabPeers(t, f)
	service := newLabApp(t, db, f, peers.local, []runtimeaccess.Binding{peers.local}, labOwner{data: f})

	const address = "127.0.0.1:8443"
	check, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatal("the dedicated private IAM fixture address is already in use")
	}
	if err := check.Close(); err != nil {
		t.Fatal("could not release the isolated fixture address")
	}
	listenContext, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	var listenError error
	go func() {
		defer close(stopped)
		listenError = service.Listen(listenContext, address, peers.server, peers.ca.roots)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Error("the private fixture listener remained active after cancellation")
		}
	})
	client := labClient(t, peers.ca, peers.iam)
	base := "https://" + address
	readyContext, stopWaiting := context.WithTimeout(t.Context(), 4*time.Second)
	defer stopWaiting()
	for {
		registration := post(readyContext, client, base+registerPath, peers.local.Registration, "")
		if registration.err == nil {
			if snapshot := assertRegistration(t, registration, peers.local); snapshot.Gate == "OPEN" {
				t.Fatal("a bootstrap request to the normal listener opened the gate")
			}
			break
		}
		select {
		case <-stopped:
			t.Fatal("the normal listener exited during bootstrap")
		case <-readyContext.Done():
			t.Fatal("the normal listener did not accept a verified bootstrap request")
		case <-time.After(30 * time.Millisecond):
		}
	}
	req := authorizeRequest(f, peers.local.RuntimeID)
	assertDenied(t, post(t.Context(), client, base+authorizePath, req, f.opaqueSession), f.opaqueSession)
	if err := service.Start(t.Context()); err != nil {
		t.Fatal("the normal listener could not open the exact registered local boot")
	}
	assertAllowed(t, post(t.Context(), client, base+authorizePath, req, f.opaqueSession), f, "OWNER")
	cancel()
	waitSignal(t, stopped, "normally listening loopback listener exited after cancellation")
	if listenError != nil {
		t.Fatal("context cancellation caused an unexpected private listener error")
	}
	if conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(t.Context(), "tcp4", address); err == nil {
		conn.Close()
		t.Fatal("the private listener still accepted a connection after cancellation")
	}
}

func TestPostgresPrivateIAMRestartAndPoolFailure(t *testing.T) {
	db := openLabDatabase(t)
	f := seedLabData(t, db)
	peers := newLabPeers(t, f)
	inventory := []runtimeaccess.Binding{peers.local}
	first := newLabApp(t, db, f, peers.local, inventory, labOwner{data: f})
	ctx := t.Context()
	if err := first.Start(ctx); err != nil {
		t.Fatal("first local boot could not start")
	}
	checkRuntimeRow(t, db, peers.local, "OPEN")
	client := labClient(t, peers.ca, peers.iam)
	activeURL := serveLab(t, first, peers, inventory)
	req := authorizeRequest(f, peers.local.RuntimeID)
	assertAllowed(t, post(ctx, client, activeURL+authorizePath, req, f.opaqueSession), f, "OWNER")

	duplicate := newLabApp(t, db, f, peers.local, inventory, labOwner{data: f})
	duplicateURL := serveLab(t, duplicate, peers, inventory)
	assertDenied(t, post(ctx, client, duplicateURL+authorizePath, req, f.opaqueSession), f.opaqueSession)
	if err := duplicate.Start(ctx); err == nil {
		t.Fatal("new process reused an existing OPEN boot")
	}
	assertDenied(t, post(ctx, client, duplicateURL+authorizePath, req, f.opaqueSession), f.opaqueSession)

	reboot, rebootPeer := peers.local, peers.ca.leaf(t, localDNS, x509.ExtKeyUsageClientAuth, false)
	reboot.BootID, reboot.Generation, reboot.CertificateFingerprint = labUUID(), 2, fingerprint(rebootPeer)
	newInventory := []runtimeaccess.Binding{reboot}
	second := newLabApp(t, db, f, reboot, newInventory, labOwner{data: f})
	secondURL := serveLab(t, second, peers, newInventory)
	secondClient := labClient(t, peers.ca, rebootPeer)
	assertDenied(t, post(ctx, secondClient, secondURL+authorizePath, req, f.opaqueSession), f.opaqueSession)
	assertDenied(t, post(ctx, secondClient, secondURL+registerPath, reboot.Registration, ""), f.opaqueSession)
	if err := second.Start(ctx); err == nil {
		t.Fatal("a higher generation silently displaced an unfenced runtime")
	}
	checkRuntimeRow(t, db, peers.local, "OPEN")

	db.app.Close()
	assertDenied(t, post(ctx, client, activeURL+authorizePath, req, f.opaqueSession), f.opaqueSession)
	assertDenied(t, post(ctx, client, activeURL+registerPath, peers.local.Registration, ""), f.opaqueSession)
	checkRuntimeRow(t, db, peers.local, "OPEN")
}

type proofPause struct {
	entered, release, returned chan struct{}
	remaining                  atomic.Int32
}

func newProofPause(after int32) *proofPause {
	pause := &proofPause{entered: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
	pause.remaining.Store(after)
	return pause
}

func waitSignal(t *testing.T, signal <-chan struct{}, reason string) {
	t.Helper()
	select {
	case <-signal:
	case <-t.Context().Done():
		t.Fatal("integration test canceled before expected stage: " + reason)
	case <-time.After(5 * time.Second):
		t.Fatal("integration wait exceeded its bound: " + reason)
	}
}

func awaitResult(t *testing.T, results <-chan result) result {
	t.Helper()
	select {
	case r := <-results:
		return r
	case <-t.Context().Done():
		t.Fatal("integration test canceled before the HTTP result")
	case <-time.After(5 * time.Second):
		t.Fatal("private HTTPS request did not return within its deadline")
	}
	return result{}
}

func TestPostgresPrivateIAMCancelConcurrencyAndCurrentDenials(t *testing.T) {
	db := openLabDatabase(t)
	f := seedLabData(t, db)
	peers := newLabPeers(t, f)
	inventory := []runtimeaccess.Binding{peers.local}
	var next atomic.Pointer[proofPause]
	owner := labOwner{data: f, wait: func(ctx context.Context) error {
		pause := next.Load()
		if pause == nil || pause.remaining.Add(-1) != 0 || !next.CompareAndSwap(pause, nil) {
			return nil
		}
		close(pause.entered)
		defer close(pause.returned)
		select {
		case <-pause.release:
			return ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		}
	}}
	service := newLabApp(t, db, f, peers.local, inventory, owner)
	ctx := t.Context()
	if err := service.Start(ctx); err != nil {
		t.Fatal("could not start registered private IAM fixture")
	}
	base := serveLab(t, service, peers, inventory)
	client := labClient(t, peers.ca, peers.iam)
	req := authorizeRequest(f, peers.local.RuntimeID)
	assertAllowed(t, post(ctx, client, base+authorizePath, req, f.opaqueSession), f, "OWNER")

	canceled := newProofPause(1)
	next.Store(canceled)
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan result, 1)
	go func() { done <- post(callCtx, client, base+authorizePath, req, f.opaqueSession) }()
	waitSignal(t, canceled.entered, "authorize reached the owner proof")
	cancel()
	if r := awaitResult(t, done); r.err == nil {
		assertDenied(t, r, f.opaqueSession)
	}
	waitSignal(t, canceled.returned, "canceled owner proof returned inside the gate")

	// Concurrent requests share one bounded gate. A superseded request may
	// fail closed, so also verify that a subsequent serialized request works.
	const parallel = 6
	var group sync.WaitGroup
	responses := make(chan result, parallel)
	for range parallel {
		group.Go(func() { responses <- post(ctx, client, base+authorizePath, req, f.opaqueSession) })
	}
	group.Wait()
	close(responses)
	for response := range responses {
		if response.status == http.StatusOK {
			assertAllowed(t, response, f, "OWNER")
		} else {
			assertDenied(t, response, f.opaqueSession)
		}
	}
	assertAllowed(t, post(ctx, client, base+authorizePath, req, f.opaqueSession), f, "OWNER")

	// The first owner check discovers the applicable subjects. Pause the
	// second check inside the section that would start the actual response.
	change := newProofPause(2)
	next.Store(change)
	go func() { done <- post(ctx, client, base+authorizePath, req, f.opaqueSession) }()
	waitSignal(t, change.entered, "authorize reached the final owner proof before the epoch change")
	revocation := labUUID()
	sqlExec(t, db.admin, `WITH changed AS (
 UPDATE hp_iam.workspaces SET security_epoch=8 WHERE workspace_id=$1 RETURNING workspace_id
) INSERT INTO hp_iam.revocations(workspace_id,revocation_id,scope,subject_id,epoch,accepted_at,deadline_at,status,reason_code)
 SELECT workspace_id,$2,'WORKSPACE',workspace_id,8,$3,$4,'PENDING','hit115_fixture' FROM changed`,
		f.workspace, revocation, f.now, f.now.Add(5*time.Second))
	close(change.release)
	assertDenied(t, awaitResult(t, done), f.opaqueSession)
	assertDenied(t, post(ctx, client, base+authorizePath, req, f.opaqueSession), f.opaqueSession)
	snapshot := assertRegistration(t, post(ctx, client, base+registerPath, peers.local.Registration, ""), peers.local)
	found := false
	for _, denial := range snapshot.Denials {
		if denial.RevocationID == revocation && denial.Scope == "WORKSPACE" && denial.WorkspaceID != nil &&
			*denial.WorkspaceID == f.workspace && denial.SubjectID == f.workspace && denial.Epoch == 8 &&
			denial.AcceptedAt.Equal(f.now) && denial.EffectiveAt == nil {
			found = true
		}
	}
	if !found {
		t.Fatal("registration snapshot omitted the committed current deny and epoch")
	}
}
