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

func TestProcessingDiagnosticsOwnedStand(t *testing.T) {
	binary, purge := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT126_PURGE_BIN")
	if binary == "" || purge == "" {
		t.Skip("requires the owned Processing PostgreSQL and purge/runtime binaries")
	}
	f, revision := knownFixture(t)
	workspace, caseID := f.request.WorkspaceID, f.request.CaseID
	stand := createProcessStand(t, binary, f.path, map[string]string{workspace: "acct_synthetic"})
	stand.queryReply = func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }

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
	stand.pollHealth(proc, "CLOSED")
	awaitDiagnostic(t, proc, "closed barrier", func(e logEvent) bool {
		return e["msg"] == "processing barrier diagnostic" && e["barrier"] == "CLOSED"
	})
	stand.iamOpen.Store(true)
	stand.pollHealth(proc, "READY")
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

	health := stand.must("ops.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 200)
	var fields logEvent
	if draftjson.Validate("Health", health) != nil || json.Unmarshal(health, &fields) != nil ||
		len(fields) != 5 || fields["status"] != "READY" {
		t.Fatal("technical health changed", string(health))
	}
	stand.must("delivery.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 403)

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
	stand.must("bff.test", "8443", "GET", casePath+"/history", nil, map[string]string{"X-HitPipe-Session": stand.token, "X-CSRF-Token": stand.csrf}, 503)
	awaitDiagnostic(t, proc, "content file failure", func(e logEvent) bool { return e["reason"] == "content_unavailable" })

	// PostgreSQL refuses two real worker operations while job_index is locked.
	// The restarted process recovers its diagnostics from durable SQL after
	// rollback, without copying samples or counting an old HTTP response.
	admin, err := pgx.Connect(ctx, "host=/var/run/postgresql user=postgres dbname=hit78 sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	lock, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, `SET LOCAL lock_timeout='2s'; LOCK TABLE hp_processing.job_index IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	if err = proc.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-proc.done
	restart := stand.boot("diagnostics-after-restart.log")
	stand.pollHealth(restart, "READY")
	awaitDiagnostic(t, restart, "PostgreSQL failure", func(e logEvent) bool { return e["reason"] == "postgres_unavailable" })
	awaitDiagnostic(t, restart, "job index recovery failure", func(e logEvent) bool { return e["reason"] == "job_recovery_failed" })
	if err = lock.Rollback(ctx); err != nil {
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
	stand.pollHealth(restart, "DEGRADED")
	if _, err = admin.Exec(ctx, "ALTER ROLE hp_processing_worker LOGIN"); err != nil {
		t.Fatal(err)
	}
	stand.pollHealth(restart, "READY")
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
	for _, marker := range []string{stand.token, stand.csrf, stand.session, "acct_synthetic", "86synthetic99",
		"HIT127_PRIVATE_OBSERVATION_BODY", "private_key", "payload", "csrf", "host=/var/run", "session_id"} {
		if bytes.Contains(runtimeLog, []byte(marker)) || bytes.Contains(purgeLog, []byte(marker)) {
			t.Fatalf("diagnostics exposed a forbidden fixture marker or field: %q", marker)
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
	t.Logf("Observed live inbox age %.0f ms, outbox due lag %.0f ms, job due lag %.0f ms and committed observation age %.0f ms.",
		q.count("inbox", "max_age_ms"), q.count("outbox", "max_due_lag_ms"), q.count("jobs", "max_due_lag_ms"), evt["observation_age_ms"])
	t.Log("PROCESS PASS: CLOSED/OPEN and exact Health, actual HELD/UNKNOWN, changed outbox/jobs, observation replay, expired lease, content and PostgreSQL faults, purge reason, restart and log privacy")
}
