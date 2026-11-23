package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
)

const runtimeTestToken = "test-only-delivery-token"
const runtimeHealth = "/internal/v1/health/delivery"

func TestDeliveryProcessCycle(t *testing.T) {
	fixtureBin, normalBin := os.Getenv("HIT133_FIXTURE_BIN"), os.Getenv("HIT133_NORMAL_BIN")
	if fixtureBin == "" || normalBin == "" {
		t.Skip("run tests/run_hit91.ps1 -RuntimeRevision <sha>")
	}
	f := setup(t)
	raw, intent, snapshot := runtimeIntent(t)
	fp := sha256.Sum256([]byte(runtimeTestToken))
	if _, err := f.admin.Exec(t.Context(), `INSERT INTO hp_delivery.destination_connections
		(workspace_id,connection_id,external_workspace_id,security_epoch,token_fingerprint,health)
		VALUES($1,$2,$3,$4,$5,'HEALTHY')`, intent.WorkspaceID, intent.ConnectionID, intent.DestinationWorkspace, intent.Epoch, fp[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(t.Context(), `INSERT INTO hp_delivery.credential_budget
		VALUES ($1,clock_timestamp(),12,0,clock_timestamp())`, fp[:]); err != nil {
		t.Fatal(err)
	}

	processingCA, opsCA, serviceCA, providerCA := newReadCA(t), newReadCA(t), newReadCA(t), newReadCA(t)
	dir := t.TempDir()
	saveRuntimeCA(t, dir, "processing-client-ca", processingCA)
	saveRuntimeCA(t, dir, "processing-server-ca", processingCA)
	saveRuntimeCA(t, dir, "ops-client-ca", opsCA)
	saveRuntimeCA(t, dir, "clickup-fixture-ca", providerCA)
	saveRuntimeCert(t, dir, "delivery-server", serviceCA.issue(t, "127.0.0.1", x509.ExtKeyUsageServerAuth))
	saveRuntimeCert(t, dir, "delivery-client", serviceCA.issue(t, "delivery.internal", x509.ExtKeyUsageClientAuth))
	ops := opsCA.issue(t, "ops.internal", x509.ExtKeyUsageClientAuth)
	caller := processingCA.issue(t, "processing.internal", x509.ExtKeyUsageClientAuth)
	auth := Authorization{Case: intent.CaseID}
	targetBody := targetJSON(t, auth, "fixture-task", "")
	var creates, gets atomic.Int64
	firstRead := make(chan struct{})
	clickup := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != runtimeTestToken || r.TLS.Version != tls.VersionTLS13 || r.URL.RawQuery != "" {
			t.Error("invalid ClickUp transport or credential")
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/list/456/task":
			creates.Add(1)
			var sent struct {
				Name, Description string
				Assignees         []int64
			}
			d := json.NewDecoder(r.Body)
			d.DisallowUnknownFields()
			if r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Idempotency-Key") != "" ||
				d.Decode(&sent) != nil || sent.Name != fixtureTitle ||
				sent.Description != canonicalTestBlock(intent.CaseID) ||
				len(sent.Assignees) != 1 || fmt.Sprint(sent.Assignees[0]) != exactAssignee {
				t.Error("invalid ClickUp create payload")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"id":"fixture-task"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/task/fixture-task":
			if gets.Add(1) == 1 {
				close(firstRead)
				<-r.Context().Done()
				return
			}
			_, _ = w.Write(targetBody)
		default:
			t.Error("unexpected ClickUp method or route")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	clickup.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{providerCA.issue(t, "127.0.0.1", x509.ExtKeyUsageServerAuth)}}
	clickup.StartTLS()
	t.Cleanup(clickup.Close)

	var recovered atomic.Bool
	var mu sync.Mutex
	var first []byte
	var firstID uuid.UUID
	var posts, applications int
	processing := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		var observation intake.TaskObservation
		requestID, requestErr := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil || requestErr != nil || requestID == uuid.Nil || r.Method != http.MethodPost || r.URL.Path != applyPath ||
			r.Header.Get("Content-Type") != "application/json" || r.TLS.Version != tls.VersionTLS13 ||
			len(r.TLS.VerifiedChains) == 0 || r.TLS.VerifiedChains[0][0].VerifyHostname("delivery.internal") != nil ||
			json.Unmarshal(body, &observation) != nil || observation.Workspace != intent.WorkspaceID ||
			observation.Case != intent.CaseID || observation.Lineage != intent.LineageID || observation.Episode != intent.EpisodeID ||
			observation.Knowledge != "RESULT_VERIFIED" || !observation.Complete || observation.EvidenceRef == nil ||
			observation.TaskID == nil || *observation.TaskID != "fixture-task" {
			t.Error("invalid applyTaskObservation request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		digest := observation.MessageDigest
		wire, err := intake.EncodeObservation(&observation)
		if err != nil || !bytes.Equal(wire, body) || digest != observation.MessageDigest {
			t.Error("noncanonical Processing observation")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		posts++
		if first == nil {
			first, firstID = body, observation.MessageID
		} else if !bytes.Equal(body, first) || observation.MessageID != firstID {
			t.Error("Processing received a changed message after outage")
			w.WriteHeader(http.StatusConflict)
			return
		}
		if !recovered.Load() {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		disposition := "DUPLICATE"
		if applications == 0 {
			applications++
			disposition = "APPLIED"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]any{"message_id": firstID, "disposition": disposition})
	}))
	processing.TLS = &tls.Config{
		MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: serviceCA.roots,
		Certificates: []tls.Certificate{processingCA.issue(t, "127.0.0.1", x509.ExtKeyUsageServerAuth)},
	}
	processing.StartTLS()
	t.Cleanup(processing.Close)
	manifestPath := filepath.Join(dir, "manifest.json")
	manifest, err := json.Marshal(map[string]any{"Snapshot": snapshot, "Token": runtimeTestToken, "ClickUp": clickup.URL})
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, manifestPath, manifest)
	business, health := runtimeAddress(t), runtimeAddress(t)
	env := map[string]string{
		"DELIVERY_APP_DSN": os.Getenv("HIT91_TEST_APP_DSN"), "DELIVERY_WORKER_DSN": os.Getenv("HIT92_TEST_WORKER_DSN"),
		"DELIVERY_CONTENT_DIR": f.root, "DELIVERY_TLS_DIR": dir, "DELIVERY_BUSINESS_ADDR": business,
		"DELIVERY_HEALTH_ADDR": health, "DELIVERY_PROCESSING_ORIGIN": processing.URL,
		"DELIVERY_PROFILE": "TEST", "DELIVERY_FIXTURE_MANIFEST": manifestPath,
	}
	refuses(t, normalBin, env, nil, "obligatory runtime adapters")
	refuses(t, fixtureBin, env, map[string]string{"DELIVERY_APP_DSN": ""}, "configuration is missing")
	refuses(t, fixtureBin, env, map[string]string{"DELIVERY_CONTENT_DIR": filepath.Join(dir, "missing")}, "private content directory unavailable")
	refuses(t, fixtureBin, env, map[string]string{"DELIVERY_PROFILE": "STANDARD"}, "requires explicit TEST profile")
	if creates.Load() != 0 {
		t.Fatal("failed start produced a provider write")
	}

	opsHTTP := runtimeHTTP(t, serviceCA, ops)
	businessHTTP := runtimeHTTP(t, serviceCA, caller)
	healthCode := func() (int, map[string]string) {
		code, body, _ := runtimeRequest(opsHTTP, http.MethodGet, "https://"+health+runtimeHealth, nil)
		var m map[string]string
		_ = json.Unmarshal(body, &m)
		return code, m
	}
	waitHealthy := func(wanted int) {
		t.Helper()
		runtimeEventually(t, fmt.Sprintf("health HTTP %d", wanted), func() bool {
			code, m := healthCode()
			if code != wanted {
				return false
			}
			if wanted == http.StatusOK {
				_, err := time.Parse(time.RFC3339Nano, m["checked_at"])
				return len(m) == 5 && m["status"] == "READY" && m["service"] == "delivery" &&
					m["protocol_version"] == "1.0" && m["build_revision"] == os.Getenv("HIT133_REVISION") && err == nil
			}
			return m["code"] == "DELIVERY_CLOSED"
		})
	}
	send := func(wanted int) intake.Receipt {
		t.Helper()
		code, body, err := runtimeRequest(businessHTTP, http.MethodPost, "https://"+business+"/internal/v1/delivery-intents", raw)
		var receipt intake.Receipt
		if err != nil || code != wanted {
			t.Fatalf("DeliveryIntent HTTP=%d want=%d, err=%v", code, wanted, err)
		}
		if wanted == http.StatusAccepted && (json.Unmarshal(body, &receipt) != nil ||
			receipt.MessageID != intent.MessageID || receipt.Disposition != "APPLIED" || receipt.RecordID == uuid.Nil) {
			t.Fatal("invalid durable receipt")
		}
		return receipt
	}

	stop := runtimeStart(t, fixtureBin, env)
	waitHealthy(http.StatusOK)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	waitHealthy(http.StatusServiceUnavailable)
	send(http.StatusServiceUnavailable)
	if creates.Load() != 0 {
		t.Fatal("unavailable authority produced a ClickUp write")
	}
	writeRuntimeFile(t, manifestPath, manifest)
	waitHealthy(http.StatusOK)
	receipt := send(http.StatusAccepted)
	if dup := send(http.StatusAccepted); receipt != dup {
		t.Fatal("same durable receipt changed on duplicate intent")
	}
	select {
	case <-firstRead:
	case <-time.After(12 * time.Second):
		t.Fatal("no readback after durable ClickUp identity")
	}
	stop()
	if creates.Load() != 1 || gets.Load() != 1 {
		t.Fatal("unexpected provider counts at process crash")
	}
	var state, knownID string
	var fenced bool
	if err := f.admin.QueryRow(t.Context(), `SELECT state,known_task_id,ever_dispatch_fenced
		FROM hp_delivery.effect_lineage WHERE workspace_id=$1 AND lineage_id=$2`, intent.WorkspaceID, intent.LineageID).
		Scan(&state, &knownID, &fenced); err != nil || state != "RESULT_ID_OBSERVED" || knownID != "fixture-task" || !fenced {
		t.Fatalf("durable create identity not confirmed after crash: %s %t %v", state, fenced, err)
	}

	stop = runtimeStart(t, fixtureBin, env)
	waitHealthy(http.StatusOK)
	if dup := send(http.StatusAccepted); receipt != dup {
		t.Fatal("restart changed the receipt")
	}
	var messageID uuid.UUID
	outbox := func(want string) bool {
		var state, digest string
		var deliveryCount int
		err := f.admin.QueryRow(t.Context(), `SELECT message_id,state,digest,delivery_count FROM hp_delivery.outbox
			WHERE workspace_id=$1 AND kind='TASK_OBSERVATION'`, intent.WorkspaceID).Scan(&messageID, &state, &digest, &deliveryCount)
		return err == nil && state == want && deliveryCount > 0 && strings.HasPrefix(digest, "sha256:")
	}
	runtimeEventually(t, "durable PENDING observation after HTTP 503", func() bool { return outbox("PENDING") })
	pendingID := messageID
	stop()
	recovered.Store(true)
	runtimeStart(t, fixtureBin, env)
	waitHealthy(http.StatusOK)
	runtimeEventually(t, "existing publisher ACK", func() bool { return outbox("ACKED") })
	mu.Lock()
	sent, applied, original := posts, applications, firstID
	mu.Unlock()
	if messageID != pendingID || messageID != original || sent < 2 || applied != 1 || creates.Load() != 1 || gets.Load() != 2 {
		t.Fatalf("bad replay: Processing sends=%d applied=%d ClickUp writes=%d GETs=%d", sent, applied, creates.Load(), gets.Load())
	}
	var inboxCount, attempts, observations, jobs int
	err = f.admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM hp_delivery.inbox WHERE workspace_id=$1),
		(SELECT count(*) FROM hp_delivery.attempts WHERE workspace_id=$1),
		(SELECT count(*) FROM hp_delivery.effect_observations WHERE workspace_id=$1 AND knowledge='RESULT_VERIFIED'),
		(SELECT count(*) FROM hp_delivery.job_index WHERE workspace_id=$1)`, intent.WorkspaceID).
		Scan(&inboxCount, &attempts, &observations, &jobs)
	if err != nil || inboxCount != 1 || attempts != 1 || observations != 1 || jobs != 0 {
		t.Fatalf("unexpected durable state: inbox=%d attempts=%d verified=%d due=%d err=%v", inboxCount, attempts, observations, jobs, err)
	}
	t.Logf("mTLS health READY/503 verified; one intent/receipt, one create after two process kills, GETs=%d", gets.Load())
	t.Logf("Processing 503 before recovery; identical TaskObservation, deliveries=%d applications=%d outbox=ACKED", sent, applied)
}

func runtimeIntent(t *testing.T) ([]byte, intake.Intent, intake.Snapshot) {
	t.Helper()
	s := intake.Snapshot{
		Workspace: uuid.New(), Case: uuid.New(), Operation: uuid.New(), Config: uuid.New(),
		Lineage: uuid.New(), Episode: uuid.New(), Connection: uuid.New(), SourceFacts: uuid.New(),
		Epoch: 1, ConfigDigest: "sha256:" + strings.Repeat("a", 64), FactsDigest: "sha256:" + strings.Repeat("b", 64),
		FactsUntil: time.Now().UTC().Truncate(time.Microsecond).Add(2 * time.Minute), Deadline: time.Now().UTC().Truncate(time.Microsecond).Add(3 * time.Minute),
		DestinationWorkspace: "fixture-workspace", List: "456", Assignee: exactAssignee,
	}
	fields := map[string]any{
		"schema_version": "1.0", "rendering_version": "handoff-block-v1", "connection_epoch": s.Epoch,
		"config_digest": s.ConfigDigest, "source_facts_digest": s.FactsDigest, "facts_valid_until": s.FactsUntil, "deadline_at": s.Deadline,
		"destination_workspace_id": s.DestinationWorkspace, "target_list_id": s.List, "assignee_id": s.Assignee,
		"title": fixtureTitle, "service_label": "Example", "source_session_id": "fixture-session", "order_reference": "order",
		"case_marker": "fixture-case", "rendered_handoff_block": canonicalTestBlock(s.Case), "context": []any{},
		"message_id": uuid.New(), "workspace_id": s.Workspace, "case_id": s.Case, "operation_id": s.Operation,
		"config_id": s.Config, "lineage_id": s.Lineage, "episode_id": s.Episode, "connection_id": s.Connection, "source_facts_id": s.SourceFacts,
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatal(err)
	}
	fields["message_digest"] = checksum(canonical)
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := intake.DecodeIntent(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, intent, s
}

func writeRuntimeFile(t *testing.T, path string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func saveRuntimeCA(t *testing.T, dir, name string, ca *testCerts) {
	t.Helper()
	writeRuntimeFile(t, filepath.Join(dir, name+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.cert.Raw}))
}

func saveRuntimeCert(t *testing.T, dir, name string, cert tls.Certificate) {
	t.Helper()
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, filepath.Join(dir, name+".pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}))
	writeRuntimeFile(t, filepath.Join(dir, name+".key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
}

func runtimeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func runtimeHTTP(t *testing.T, ca *testCerts, cert tls.Certificate) *http.Client {
	t.Helper()
	c := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.roots, Certificates: []tls.Certificate{cert}},
		DisableKeepAlives: true,
	}}
	t.Cleanup(c.CloseIdleConnections)
	return c
}

func runtimeRequest(c *http.Client, method, url string, body []byte) (int, []byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-Request-ID", uuid.NewString())
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	return res.StatusCode, raw, err
}

func runtimeEnv(env, overrides map[string]string) []string {
	all := os.Environ()
	for k, v := range env {
		if replacement, ok := overrides[k]; ok {
			v = replacement
		}
		all = append(all, k+"="+v)
	}
	return all
}

func refuses(t *testing.T, path string, env, overrides map[string]string, reason string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path)
	cmd.Env = runtimeEnv(env, overrides)
	raw, err := cmd.CombinedOutput()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 1 || !strings.Contains(string(raw), reason) {
		t.Fatalf("expected closed startup (%s), err=%v output=%s", reason, err, raw)
	}
	t.Logf("closed startup exit=1 (%s)", reason)
}

func runtimeStart(t *testing.T, path string, env map[string]string) func() {
	t.Helper()
	cmd := exec.Command(path)
	cmd.Env = runtimeEnv(env, nil)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stop := sync.OnceFunc(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Logf("Delivery process log: %s", strings.TrimSpace(output.String()))
	})
	t.Cleanup(stop)
	return stop
}

func runtimeEventually(t *testing.T, want string, match func() bool) {
	t.Helper()
	until := time.Now().Add(14 * time.Second)
	for time.Now().Before(until) {
		if match() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("not observed: %s", want)
}
