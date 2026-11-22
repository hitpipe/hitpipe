package postgres

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/jackc/pgx/v5"
)

type iamTrace struct {
	Message    string `json:"msg"`
	Status     int    `json:"status"`
	RuntimeID  string `json:"runtime_id"`
	BootID     string `json:"boot_id"`
	Generation int64  `json:"generation"`
	Gate       string `json:"gate"`
}

func awaitIAMTrace(t *testing.T, path string, want func(iamTrace) bool) iamTrace {
	t.Helper()
	until := time.Now().Add(6 * time.Second)
	for time.Now().Before(until) {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range bytes.Split(raw, []byte{'\n'}) {
			var event iamTrace
			if json.Unmarshal(line, &event) == nil && want(event) {
				return event
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	raw, _ := os.ReadFile(path)
	t.Fatalf("actual IAM observation missing: %s", raw)
	return iamTrace{}
}

func postIAMFixture(t *testing.T, st *processStand, cert tls.Certificate, path, session string, body any) (int, []byte, error) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig: &tls.Config{RootCAs: st.pki.Root, Certificates: []tls.Certificate{cert},
			MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
	}}
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://127.0.0.2:8443"+path, bytes.NewReader(marshalProcess(t, body)))
	if err != nil {
		t.Fatal(err)
	}
	id, _ := newID()
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Content-Type", "application/json")
	if session != "" {
		req.Header.Set("X-HitPipe-Session", session)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func seedIAMWorkspace(t *testing.T, st *processStand, ws string) {
	t.Helper()
	db, err := pgx.Connect(t.Context(), "host=/var/run/postgresql user=hp_hit146_iam_app dbname=hit146_iam sslmode=disable")
	if err != nil {
		t.Fatal("open disposable IAM fixture database")
	}
	defer db.Close(ctx)
	tx, err := db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	session, csrf := sha256.Sum256([]byte(st.token)), sha256.Sum256([]byte(st.csrf))
	for _, s := range []struct {
		sql  string
		args []any
	}{
		{"SELECT set_config('hp.workspace_id',$1,true)", []any{ws}},
		{"INSERT INTO hp_iam.principals(principal_id,oidc_issuer,oidc_subject,created_at) VALUES($1,'https://test.invalid','synthetic',now())", []any{owner}},
		{"INSERT INTO hp_iam.sessions(session_id,session_hash,principal_id,csrf_hash,created_at,expires_at,idle_expires_at,epoch) VALUES($1,$2,$3,$4,now(),now()+interval '1 hour',now()+interval '30 minutes',7)", []any{st.session, session[:], owner, csrf[:]}},
		{"INSERT INTO hp_iam.workspaces(workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at) VALUES($1,$2,1,7,'ACTIVE',now())", []any{ws, config}},
		{"INSERT INTO hp_iam.memberships(workspace_id,principal_id,role,status,revision,accepted_at) VALUES($1,$2,'OWNER','ACTIVE',1,now())", []any{ws, owner}},
		{"INSERT INTO hp_iam.external_bindings(workspace_id,grant_id,provider,provider_account_id,livemode,pack,status,security_epoch,created_at,verified_at) VALUES($1,$2,'CLICKUP','synthetic',false,'PAID_ORDER_TASK_V1','VERIFIED',7,now(),now())", []any{ws, destination}},
	} {
		if _, err := tx.Exec(t.Context(), s.sql, s.args...); err != nil {
			t.Fatal("seed disposable IAM owner, workspace and session")
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal("commit IAM fixture")
	}
}

func TestRealIAMRegistrationBoundary(t *testing.T) {
	processing, iam := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT146_IAM_BIN")
	if os.Getenv("HIT78_ISOLATED_PG") != "1" || processing == "" || iam == "" {
		t.Skip("requires the isolated IAM and Processing runner")
	}
	f := prepared(t, []string{})
	result, err := f.service.Prepare(ctx, f.request)
	if err != nil || result.Disposition != "PENDING_OUTBOX" {
		t.Fatal("seed one pending synthetic delivery")
	}
	ws := f.request.WorkspaceID
	st := createProcessStandWithIAM(t, processing, f.path, map[string]string{ws: "acct_synthetic"}, false)
	seedIAMWorkspace(t, st, ws)

	// The fixed binding belongs to TEST provisioning, never to the incoming
	// registration. Both processes receive it through their trusted local inputs.
	boot, _ := newID()
	iamRuntime, _ := newID()
	iamBoot, _ := newID()
	revokeID, _ := newID()
	missingACK, _ := newID()
	leaf := st.pki.Issue(t, "iam.test", net.ParseIP("127.0.0.2"), true, false)
	cert, key := st.keypair("real-iam", leaf)
	iamClient := st.pki.Leaf(t, "iam.test", false, false)
	clientCert, clientKey := st.keypair("real-iam-client", iamClient)
	iamFingerprint := sha256.Sum256(iamClient.Certificate[0])
	procCert, err := tls.LoadX509KeyPair(filepath.Join(st.dir, "processing-client.pem"), filepath.Join(st.dir, "processing-client.key"))
	if err != nil {
		t.Fatal(err)
	}
	controlCert, err := tls.LoadX509KeyPair(filepath.Join(st.dir, "processing.pem"), filepath.Join(st.dir, "processing.key"))
	if err != nil {
		t.Fatal(err)
	}
	controlPin := fmt.Sprintf("sha256:%x", sha256.Sum256(controlCert.Certificate[0]))
	if controlPin == st.cert {
		t.Fatal("registration and control pins must differ")
	}
	registration := drafts.RuntimeRegistration{RuntimeID: st.identity, Service: "PROCESSING", Generation: 2, BootID: boot, CertificateFingerprint: st.cert}
	raw, err := os.ReadFile(st.config)
	var options map[string]any
	if err != nil || json.Unmarshal(raw, &options) != nil {
		t.Fatal("read synthetic Processing configuration")
	}
	options["generation"] = registration.Generation
	st.config = st.json("real-processing.json", options)
	tracePath := filepath.Join(st.dir, "iam-trace.jsonl")
	trace, err := os.Create(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	defer trace.Close()
	var binding map[string]any
	if err = json.Unmarshal(marshalProcess(t, registration), &binding); err != nil {
		t.Fatal(err)
	}
	binding["dns_name"] = "processing-client.test"
	cfg := st.json("iam-process.json", map[string]any{
		"IAM": map[string]any{"runtime_id": iamRuntime, "boot_id": iamBoot, "service": "IAM", "generation": 2,
			"dns_name": "iam.test", "certificate_fingerprint": "sha256:" + hex.EncodeToString(iamFingerprint[:])},
		"Processing": binding, "CA": filepath.Join(st.dir, "ca.pem"), "Certificate": cert, "Key": key,
		"ClientCertificate": clientCert, "ClientKey": clientKey,
		"Workspace": ws, "Revocation": revokeID, "MissingACK": missingACK, "Dir": st.dir,
		"Owner": owner, "Session": st.session, "Destination": destination, "ProcessingControlPin": controlPin,
	})
	command := exec.Command(iam, cfg)
	command.Stdout, command.Stderr = trace, trace
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		<-done
		log, _ := os.ReadFile(tracePath)
		t.Logf("actual IAM process trace:\n%s", log)
	})
	awaitIAMTrace(t, tracePath, func(e iamTrace) bool { return e.Message == "iam-listening" })

	const register = "/internal/v1/runtime/register"
	history := "/api/v1/workspaces/" + ws + "/cases/" + f.request.CaseID + "/history"
	headers := map[string]string{"X-HitPipe-Session": st.token}
	unbound := st.boot("unbound")
	st.pollHealth(unbound, "CLOSED")
	rejected := awaitIAMTrace(t, tracePath, func(e iamTrace) bool {
		return e.Message == "iam-register" && e.Status == 403 && e.RuntimeID == st.identity &&
			e.Generation == 2 && e.BootID != "" && e.BootID != boot
	})
	st.must("bff.test", "8443", "GET", history, nil, headers, 503)
	if err := unbound.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-unbound.done
	t.Logf("HTTP 403 for unprovisioned boot %s, protected HTTP 503 while CLOSED", rejected.BootID)

	// The first registered incarnation follows an unadmitted process. No
	// old admitted runtime is replaced without a fence.
	provisionDir := filepath.Join(st.dir, "boot-provision")
	if err := os.Mkdir(provisionDir, 0700); err != nil {
		t.Fatal(err)
	}
	provision := st.file("boot-provision/identity", []byte(fmt.Sprintf("%s\n%d\n%s\n%s\n",
		registration.RuntimeID, registration.Generation, registration.BootID, registration.CertificateFingerprint)))
	options["boot_identity_file"] = provision
	st.config = st.json("real-processing.json", options)
	st.waitDelivery.Store(true)
	p := st.boot("provisioned")
	st.pollHealth(p, "CLOSED")
	awaitIAMTrace(t, tracePath, func(e iamTrace) bool {
		return e.Message == "iam-register" && e.Status == 200 && e.BootID == boot && e.Gate == "SYNCING"
	})
	st.must("bff.test", "8443", "GET", history, nil, headers, 503)
	if st.deliveryCount.Load() != 0 {
		t.Fatal("a CLOSED process started Delivery")
	}
	code, payload, err := postIAMFixture(t, st, procCert, register, "", registration)
	var current drafts.RuntimeSnapshot
	if err != nil || code != 200 || draftjson.Validate("RuntimeSnapshot", payload) != nil ||
		json.Unmarshal(payload, &current) != nil || current.Gate != "SYNCING" || current.BootID != boot ||
		current.Generation != 2 || current.RuntimeID != st.identity {
		t.Fatalf("expected matching actual SYNCING snapshot, code=%d: %v", code, err)
	}
	t.Logf("actual IAM SYNCING snapshot %s", payload)

	gate, err := startgate.New(startgate.Identity{RuntimeID: current.RuntimeID, BootID: current.BootID, Generation: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err = gate.ApplySnapshot(ctx, startgate.RuntimeSnapshot{RuntimeID: current.RuntimeID, BootID: current.BootID,
		Generation: uint64(current.Generation), SnapshotRevision: uint64(current.SnapshotRevision), Gate: startgate.State(current.Gate)}); !errors.Is(err, startgate.ErrIdentity) {
		t.Fatal("current generation accepted the genuine old IAM snapshot", err)
	}
	old := registration
	old.Generation--
	code, _, err = postIAMFixture(t, st, procCert, register, "", old)
	if err != nil || code != 403 {
		t.Fatal("old generation registration did not receive 403", code, err)
	}
	foreign := st.pki.Leaf(t, "bff.test", false, false)
	if code, _, err = postIAMFixture(t, st, foreign, register, "", registration); err == nil || code != 0 {
		t.Fatal("wrong service principal received an HTTP response", code)
	}
	t.Log("old generation: HTTP 403 and stale snapshot ErrIdentity. Wrong principal: mTLS rejected.")
	st.file("open-iam", nil)
	awaitIAMTrace(t, tracePath, func(e iamTrace) bool {
		return e.Message == "iam-register" && e.Status == 200 && e.BootID == boot && e.Gate == "OPEN"
	})
	st.pollHealth(p, "READY")
	health := st.must("ops.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 200)
	st.must("bff.test", "8443", "GET", history, nil, headers, 200)
	t.Logf("SYNCING → genuine IAM OPEN → protected HTTP 200; OPS %s", health)

	select {
	case message := <-st.deliverySeen:
		if message != result.MessageID {
			t.Fatal("unexpected synthetic message", message)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("genuine IAM permit did not allow the first Delivery start")
	}
	t.Logf("synthetic Delivery accepted first HTTP start %s and held its response", result.MessageID)
	st.file("revoke", nil)
	if err := waitIAMFile(t, st.dir, "effective-observed", 5*time.Second); err != nil {
		t.Fatal(err)
	}
	st.must("bff.test", "8443", "GET", history, nil, headers, 403)
	t.Log("protected request after effective revocation: HTTP 403")
	select {
	case msg := <-st.deliverySeen:
		t.Fatal("old epoch started a new HTTP request", msg)
	case <-time.After(2300 * time.Millisecond):
	}
	state, _, _, attempts := (sendFixture{f: f, message: result.MessageID}).outbox(t)
	if state != "CLAIMED" || attempts != 1 || st.deliveryCount.Load() != 1 {
		t.Fatal("in-flight request unexpectedly finalized, retried or repeated", state, attempts, st.deliveryCount.Load())
	}
	t.Logf("held request after ACK and timeout: outbox=%s attempts=%d Delivery_HTTP_starts=%d", state, attempts, st.deliveryCount.Load())
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-p.done
	replay := st.boot("reused-provision")
	select {
	case err := <-replay.done:
		if err == nil {
			t.Fatal("a reused provision started successfully")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a reused provision did not fail before registration")
	}
	st.file("revoke-without-processing", nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("real IAM coordinator failed its missing-ACK verification", err)
		}
	case <-time.After(9 * time.Second):
		t.Fatal("real IAM missing-ACK verification timed out")
	}
	t.Log("reused provision rejected, absent Processing remained missing, IAM recorded INCIDENT")
}

func waitIAMFile(t *testing.T, dir, name string, limit time.Duration) error {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("IAM observation missing: %s", name)
}
