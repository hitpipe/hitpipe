package postgres

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/jackc/pgx/v5"
)

func TestDiagnosticSampleBoundedAndScoped(t *testing.T) {
	app := connect(t)
	workspace, _ := newID()
	foreign, _ := newID()
	worker := DispatchStore{DB: dispatchTestDB{}}
	insert := func(ws string, count int, age time.Duration) {
		t.Helper()
		tx := begin(t, app, ws)
		defer tx.Rollback(ctx)
		for range count {
			id, _ := newID()
			if _, err := tx.Exec(ctx, `INSERT INTO hp_processing.inbox(workspace_id,message_id,digest,disposition,received_at)
 VALUES($1,$2,$3,'HELD',$4)`, ws, id, "sha256:"+strings.Repeat("b", 64), time.Now().Add(-age)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	insert(workspace, DiagnosticSampleLimit+7, 10*time.Second)
	insert(foreign, 1, 20*time.Hour)
	id, _ := newID()
	tx, err := worker.tx(ctx, foreign)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO hp_processing.job_index(job_id,workspace_id,kind,due_at,claim_token,lease_until)
 VALUES($1,$2,'DELIVERY_INTENT',clock_timestamp()-interval '12 hours',$1,clock_timestamp()-interval '11 hours')`, id, foreign); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := worker.SampleQueues(ctx, workspace)
	if err != nil || s.InboxRows != DiagnosticSampleLimit || s.InboxHeld != DiagnosticSampleLimit ||
		s.InboxAge == nil || *s.InboxAge < 9_000 || *s.InboxAge > 30_000 || s.JobRows != 0 ||
		s.ExpiredJobLeases != 0 || s.OutboxAge != nil || s.JobDueLag != nil {
		t.Fatalf("wrong scoped bounded sample %+v %v", s, err)
	}
	s, err = worker.SampleQueues(ctx, foreign)
	if err != nil || s.InboxRows != 1 || s.InboxAge == nil || *s.InboxAge < 70_000_000 ||
		s.JobRows != 1 || s.ExpiredJobLeases != 1 || s.JobDueLag == nil || *s.JobDueLag < 43_000_000 {
		t.Fatalf("foreign workspace did not obtain its own metadata %+v %v", s, err)
	}
	if _, err = (DispatchStore{DB: app}).SampleQueues(ctx, workspace); err == nil {
		t.Fatal("app could read the worker job diagnostics")
	}
}

type logEvent map[string]any

func (e logEvent) count(group, field string) float64 {
	g, _ := e[group].(map[string]any)
	value, _ := g[field].(float64)
	return value
}

func readDiagnosticEvents(path string) (out []logEvent) {
	raw, _ := os.ReadFile(path)
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		var e logEvent
		if json.Unmarshal(line, &e) == nil {
			out = append(out, e)
		}
	}
	return
}

func awaitDiagnostic(t *testing.T, p *processingChild, what string, matches func(logEvent) bool) logEvent {
	t.Helper()
	deadline := time.Now().Add(9 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range readDiagnosticEvents(p.logPath) {
			if matches(e) {
				return e
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("missing diagnostic %s: %v", what, readDiagnosticEvents(p.logPath))
	return nil
}

const diagnosticHealthPath = "/internal/v1/health/processing"

type diagnosticHTTPResponse struct {
	Scenario string `json:"scenario"`
	Method   string `json:"method"`
	Path     string `json:"path"`
	Peer     string `json:"peer"`
	Status   int    `json:"http_status"`
	Body     string `json:"response_body"`
}

// Validate the bytes received from the process and exercise both directions
// of the closed schema. The modified copies are never captured as responses.
func checkDiagnosticHealth(t *testing.T, raw []byte, stage string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if draftjson.Validate("Health", raw) != nil || json.Unmarshal(raw, &fields) != nil ||
		len(fields) != 5 || string(fields["status"]) != `"`+stage+`"` ||
		string(fields["service"]) != `"PROCESSING"` {
		t.Fatal("technical health changed", string(raw))
	}
	for _, key := range []string{"status", "service", "build_revision", "protocol_version", "checked_at"} {
		value := fields[key]
		delete(fields, key)
		missing := marshalProcess(t, fields)
		if draftjson.Validate("Health", missing) == nil {
			t.Fatal("schema allowed missing required property", key)
		}
		fields[key] = value
	}
	fields["confirmed_delivery"] = json.RawMessage("true")
	if draftjson.Validate("Health", marshalProcess(t, fields)) == nil {
		t.Fatal("schema allowed an extra property")
	}
}

func TestProcessingDiagnosticsOwnedStand(t *testing.T) {
	binary, purge := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT126_PURGE_BIN")
	if binary == "" || purge == "" {
		t.Skip("requires the owned Processing PostgreSQL and purge/runtime binaries")
	}
	f, revision := knownFixture(t)
	workspace, caseID := f.request.WorkspaceID, f.request.CaseID
	stand := createProcessStand(t, binary, f.path, map[string]string{workspace: "acct_synthetic"})
	stand.queryReply = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }
	var httpLog bytes.Buffer
	capture := func(scenario, route, peer string, status int, body []byte) {
		t.Helper()
		row := diagnosticHTTPResponse{
			Scenario: scenario, Method: http.MethodGet, Path: route,
			Peer: peer, Status: status, Body: string(body),
		}
		if err := json.NewEncoder(&httpLog).Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	captureHealth := func(process *processingChild, scenario, stage string) {
		t.Helper()
		status, raw := stand.pollHealth(process, stage)
		checkDiagnosticHealth(t, raw, stage)
		capture(scenario, diagnosticHealthPath, "OPS", status, raw)
	}

	// Keep a known expired lease from the earlier intent. The UNKNOWN case
	// cannot claim it. The real runtime owns the metadata visible in this scope.
	worker := DispatchStore{DB: dispatchTestDB{}}
	tx, err := worker.tx(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO hp_processing.job_index(job_id,workspace_id,kind,due_at,claim_token,lease_until)
 VALUES($1,$2,'DELIVERY_INTENT',clock_timestamp()-interval'5 seconds',$1,clock_timestamp()-interval'1 second')`, f.lineage, workspace); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	proc := stand.boot("diagnostics-before-restart.log")
	captureHealth(proc, "closed_before_fresh_iam_snapshot", "CLOSED")
	awaitDiagnostic(t, proc, "closed barrier", func(e logEvent) bool {
		return e["msg"] == "processing barrier diagnostic" && e["barrier"] == "CLOSED"
	})
	stand.iamOpen.Store(true)
	captureHealth(proc, "ready_after_gate_open", "READY")
	awaitDiagnostic(t, proc, "open barrier", func(e logEvent) bool {
		return e["msg"] == "processing barrier diagnostic" && e["barrier"] == "OPEN"
	})
	q := awaitDiagnostic(t, proc, "pending intent, expired job, UNKNOWN and positive ages", func(e logEvent) bool {
		return e["workspace_id"] == workspace && e["msg"] == "processing queue sample" &&
			e.count("outbox", "pending") >= 1 && e.count("inbox", "max_age_ms") > 0 &&
			e.count("outbox", "max_due_lag_ms") > 0 && e.count("jobs", "max_due_lag_ms") > 0 &&
			e.count("jobs", "expired_leases") == 1 && e["expired_job_id"] == f.lineage && e.count("cases", "unknown") == 1
	})
	if q["sample_limit"] != float64(DiagnosticSampleLimit) {
		t.Fatal("wrong reported sample cap", q)
	}
	awaitDiagnostic(t, proc, "expired lease reason", func(e logEvent) bool { return e["reason"] == "lease_expired" })

	request, _ := newID()
	casePath := "/api/v1/workspaces/" + workspace + "/cases/" + caseID
	stand.command(casePath+"/recovery", recheckCommand(t, request, revision), request, 202)
	waitRecheckOutbox(t, workspace, request, "HELD", 1, 5*time.Second)
	awaitDiagnostic(t, proc, "real held query", func(e logEvent) bool {
		return e["msg"] == "processing recheck outbox transition" && e["request_id"] == request && e["state"] == "HELD"
	})
	held := awaitDiagnostic(t, proc, "outbox held snapshot", func(e logEvent) bool {
		return e["msg"] == "processing queue sample" && e.count("outbox", "held") >= 1
	})
	if held.count("outbox", "rows") <= q.count("outbox", "rows") || held.count("jobs", "rows") != q.count("jobs", "rows") {
		t.Fatal("held outbox did not retain the measured independent job population", q, held)
	}
	// Keep the unresolved intent and stop its reindexing. Let a real sample
	// attest the committed change before deleting the now-obsolete job entry.
	tx, err = worker.tx(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE hp_processing.outbox SET state='HELD' WHERE workspace_id=$1 AND message_id=$2`, workspace, f.lineage); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	awaitDiagnostic(t, proc, "both outbox rows held", func(e logEvent) bool {
		return e["msg"] == "processing queue sample" && e.count("outbox", "held") == 2
	})
	tx, err = worker.tx(ctx, workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `DELETE FROM hp_processing.job_index WHERE workspace_id=$1 AND job_id=$2`, workspace, f.lineage); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	awaitDiagnostic(t, proc, "job backlog changed after commit", func(e logEvent) bool {
		return e["msg"] == "processing queue sample" && e.count("jobs", "expired_leases") == 0 &&
			e.count("outbox", "held") == 2 && e.count("jobs", "rows") < held.count("jobs", "rows")
	})

	observed := f.observation(t, "RESULT_MISMATCH")
	no := false
	observed.ListMatches = &no
	observed.ProofCode = "HIT127_PRIVATE_OBSERVATION_BODY"
	observed.ObservedAt = time.Now().UTC().Add(-700 * time.Millisecond)
	body := observationWire(t, observed)
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", body, nil, 202)
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", body, nil, 202)
	evt := awaitDiagnostic(t, proc, "committed observation", func(e logEvent) bool {
		return e["msg"] == "processing observation committed" && e["message_id"] == observed.MessageID
	})
	if age, ok := evt["observation_age_ms"].(float64); !ok || age < 700 || age > 5000 ||
		evt["effect_id"] != f.lineage || evt["case_id"] != caseID || evt["case_state"] != "ATTENTION" {
		t.Fatal("wrong observation metric", evt)
	}
	awaitDiagnostic(t, proc, "persisted ATTENTION", func(e logEvent) bool {
		return e["msg"] == "processing queue sample" && e.count("cases", "attention") == 1 &&
			e.count("cases", "unknown") == 0
	})

	captureHealth(proc, "ready_with_held_outbox_and_attention_case", "READY")
	deniedStatus, denied, err := stand.call("delivery.test", "9444", http.MethodGet, diagnosticHealthPath, nil, nil)
	if err != nil || deniedStatus != 403 || draftjson.Validate("Error", denied) != nil {
		t.Fatal("non-OPS health response", deniedStatus, err)
	}
	capture("non_ops_peer_denied", diagnosticHealthPath, "DELIVERY", deniedStatus, denied)

	purgeCmd := exec.Command(purge, "--local-stand", "--content-dir", f.path, "--workspace", workspace)
	purgeCmd.Env = append(os.Environ(), "HITPIPE_PURGE_DSN=host=/var/run/postgresql dbname=hit78 sslmode=disable user=hp_processing_purge")
	purgeLog, err := purgeCmd.CombinedOutput()
	var purgeEvent logEvent
	if err == nil || json.Unmarshal(purgeLog, &purgeEvent) != nil || purgeEvent["reason"] != "purge_volume_unavailable" {
		t.Fatalf("expected structured purge failure: %s %v", purgeLog, err)
	}

	// An absent authenticated file causes a real HTTP fault and local log.
	app := connect(t)
	tx = begin(t, app, workspace)
	var detail string
	if err = tx.QueryRow(ctx, `SELECT c.detail_ref::text FROM hp_processing.history_content c
 JOIN hp_processing.inbox i ON (i.workspace_id,i.record_id)=(c.workspace_id,c.entry_id)
 WHERE i.workspace_id=$1 AND i.message_id=$2`, workspace, observed.MessageID).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if err = os.Remove(filepath.Join(f.path, detail)); err != nil {
		t.Fatal(err)
	}
	status, unavailable, err := stand.call("bff.test", "8443", http.MethodGet, casePath+"/history", nil,
		map[string]string{"X-HitPipe-Session": stand.token, "X-CSRF-Token": stand.csrf})
	if err != nil || status != 503 || draftjson.Validate("Error", unavailable) != nil {
		t.Fatal("history response after file failure", status, err)
	}
	capture("owned_file_unavailable", "/api/v1/workspaces/{workspaceId}/cases/{caseId}/history", "BFF", status, unavailable)
	awaitDiagnostic(t, proc, "content file failure", func(e logEvent) bool { return e["reason"] == "content_unavailable" })

	// PostgreSQL rejects worker job reads on the disposable database. Both
	// the index rebuild and the separate sample fail at the actual SQL path.
	admin, err := pgx.Connect(ctx, "host=/var/run/postgresql user=postgres dbname=hit78 sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	const restoreJobs = "GRANT SELECT ON hp_processing.job_index TO hp_processing_worker"
	if _, err = admin.Exec(ctx, "REVOKE SELECT ON hp_processing.job_index FROM hp_processing_worker"); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, restoreJobs)
	if err = proc.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-proc.done
	restart := stand.boot("diagnostics-after-restart.log")
	captureHealth(restart, "ready_after_process_restart", "READY")
	awaitDiagnostic(t, restart, "PostgreSQL failure", func(e logEvent) bool { return e["reason"] == "postgres_unavailable" })
	awaitDiagnostic(t, restart, "job index recovery failure", func(e logEvent) bool { return e["reason"] == "job_recovery_failed" })
	if _, err = admin.Exec(ctx, restoreJobs); err != nil {
		t.Fatal(err)
	}
	awaitDiagnostic(t, restart, "restored durable sample", func(e logEvent) bool {
		return e["msg"] == "processing queue sample" && e.count("outbox", "held") == 2 && e.count("cases", "attention") == 1
	})
	// Fault the technical worker connection on this disposable database.
	// Restore the role before the remaining suite uses the container.
	if _, err = admin.Exec(ctx, "ALTER ROLE hp_processing_worker NOLOGIN"); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec(ctx, "ALTER ROLE hp_processing_worker LOGIN")
	if _, err = admin.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity
 WHERE usename='hp_processing_worker' AND application_name='hitpipe-processing'`); err != nil {
		t.Fatal(err)
	}
	captureHealth(restart, "degraded_on_worker_postgres_connection_failure", "DEGRADED")
	if _, err = admin.Exec(ctx, "ALTER ROLE hp_processing_worker LOGIN"); err != nil {
		t.Fatal(err)
	}
	captureHealth(restart, "ready_after_worker_postgres_reconnect", "READY")
	if err = restart.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-restart.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("diagnostics worker prevented orderly shutdown")
	}
	var runtimeLog []byte
	for _, p := range []*processingChild{proc, restart} {
		raw, err := os.ReadFile(p.logPath)
		if err != nil {
			t.Fatal(err)
		}
		runtimeLog = append(runtimeLog, raw...)
	}
	for _, stream := range [][]byte{runtimeLog, purgeLog, httpLog.Bytes()} {
		lower := bytes.ToLower(stream)
		for _, marker := range []string{stand.token, stand.csrf, stand.session, "acct_synthetic", "86synthetic99",
			"hit127_private_observation_body", "private_key", "payload", "csrf", "host=/var/run", "session_id",
			"authorization", "set-cookie", "password", "secret", "api_key", "access_token", "bearer ",
			"-----begin ", "sk_live_", "sk_test_", "whsec_"} {
			if bytes.Contains(lower, []byte(strings.ToLower(marker))) {
				t.Fatal("a diagnostic or HTTP capture contained a forbidden field or fixture value")
			}
		}
	}
	if n := bytes.Count(runtimeLog, []byte(`"msg":"processing observation committed"`)); n != 1 {
		t.Fatal("replay or restart duplicated observation event", n)
	}
	if destination := os.Getenv("HIT127_CAPTURE_FILE"); destination != "" {
		if err := os.WriteFile(destination, append(runtimeLog, purgeLog...), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if destination := os.Getenv("HIT127_HTTP_CAPTURE_FILE"); destination != "" {
		if err := os.WriteFile(destination, httpLog.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("HTTP PASS: captured actual Health CLOSED/READY/DEGRADED/recovered and mTLS denial; missing each required field and an extra property were rejected by the pinned validator")
	t.Logf("Observed live inbox age %.0f ms, outbox due lag %.0f ms, job due lag %.0f ms and committed observation age %.0f ms.",
		q.count("inbox", "max_age_ms"), q.count("outbox", "max_due_lag_ms"), q.count("jobs", "max_due_lag_ms"), evt["observation_age_ms"])
	t.Log("PROCESS PASS: CLOSED/OPEN and exact Health, actual HELD/UNKNOWN, changed outbox/jobs, observation replay, expired lease, content and PostgreSQL faults, purge reason, restart and log privacy")
}
