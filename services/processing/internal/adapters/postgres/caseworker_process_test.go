package postgres

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

type controlledFacts struct {
	activation time.Time
	mode       string
}

type sourceCycle struct {
	workspace, session, caseID string
}

func TestProcessingCaseWorkerOwnedStand(t *testing.T) {
	binary := os.Getenv("HIT111_PROCESSING_BIN")
	if binary == "" {
		t.Skip("requires owned PostgreSQL and built Processing executable")
	}
	f := newDraftFixtureBase(t)
	pauseScope := newDraftFixtureBase(t)
	unknownBinding := prepared(t, []string{})
	stand := createProcessStand(t, binary, f.path, map[string]string{
		f.workspace: "acct_controlled", pauseScope.workspace: "acct_controlled",
		unknownBinding.request.WorkspaceID: "acct_other_binding",
	})
	var modes sync.Map
	reads := make(chan sourceCycle, 32)
	release := make(chan struct{})
	stand.intakeReply = func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			WorkspaceID   string    `json:"workspace_id"`
			SessionID     string    `json:"session_id"`
			CaseID        string    `json:"case_id"`
			ConnectionID  string    `json:"connection_id"`
			ExpectedEpoch int64     `json:"expected_epoch"`
			Deadline      time.Time `json:"deadline_at"`
		}
		if r.URL.Path != "/internal/v1/source-facts/read" || r.Method != http.MethodPost ||
			json.NewDecoder(r.Body).Decode(&in) != nil || in.ConnectionID != config || in.ExpectedEpoch != 7 ||
			!drafts.UUID(in.CaseID) || !drafts.UUID(r.Header.Get("X-Request-ID")) || !time.Now().Before(in.Deadline) {
			w.WriteHeader(400)
			return
		}
		reads <- sourceCycle{in.WorkspaceID, in.SessionID, in.CaseID}
		v, ok := modes.Load(in.SessionID)
		if !ok {
			w.WriteHeader(400)
			return
		}
		facts := v.(controlledFacts)
		if facts.mode == "unavailable" {
			w.WriteHeader(503)
			return
		}
		if facts.mode == "hold" {
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		c := preparecase.Case{Request: preparecase.Request{CaseID: in.CaseID}, Mapping: preparecase.Mapping{SourceConnectionID: config},
			SessionID: in.SessionID, AccountID: "acct_controlled", ActivationAt: facts.activation}
		reply := sourceWire(t, c, func(m map[string]any) {
			// Complete the observation during this HTTP request, so evaluating
			// with a pre-read clock would wrongly refuse these authentic facts.
			now := time.Now().UTC()
			m["session_created_at"] = facts.activation.Format(time.RFC3339Nano)
			m["oldest_observed_at"] = now.Add(-time.Second).Format(time.RFC3339Nano)
			m["completed_at"] = now.Format(time.RFC3339Nano)
			m["valid_until"] = now.Add(8 * time.Second).Format(time.RFC3339Nano)
			switch facts.mode {
			case "incomplete":
				m["complete"] = false
			case "unpaid":
				m["payment_status"] = "unpaid"
			case "old":
				m["session_created_at"] = facts.activation.Add(-time.Second).Format(time.RFC3339Nano)
			}
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}
	start := stand.boot("case-worker-initial.log")
	stand.pollHealth(start, "CLOSED")
	stand.iamOpen.Store(true)
	stand.pollHealth(start, "READY")
	openConfig := f.config()
	openConfig.RequiredContextKeys = &[]string{}
	_, operation := stand.activateWithConfig(f, openConfig)
	pauseConfig := pauseScope.config()
	pauseConfig.RequiredContextKeys = &[]string{}
	_, pauseOp := stand.activateWithConfig(pauseScope, pauseConfig)
	signal := func(scope draftFixture, session, event string, created time.Time) []byte {
		return intakeWire(t, scope.workspace, func(m map[string]any) {
			m["stripe_account_id"] = "acct_controlled"
			m["session_id"] = session
			m["event_type"] = event
			m["source_created_at"] = created.UTC().Format(time.RFC3339Nano)
			m["first_received_at"] = time.Now().UTC().Format(time.RFC3339Nano)
			m["expires_at"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		})
	}
	accept := func(body []byte) (string, []byte) {
		reply := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", body, nil, 202)
		var r struct {
			RecordID string `json:"record_id"`
		}
		if json.Unmarshal(reply, &r) != nil || !drafts.UUID(r.RecordID) {
			t.Fatal(string(reply))
		}
		return r.RecordID, reply
	}
	awaitRead := func(workspace, session, caseID string) {
		t.Helper()
		select {
		case read := <-reads:
			if read != (sourceCycle{workspace, session, caseID}) {
				t.Fatalf("unexpected Intake start: %+v", read)
			}
		case <-time.After(13 * time.Second):
			t.Fatal("Processing did not reach controlled Intake", session)
		}
	}
	assertQuiet := func() {
		t.Helper()
		select {
		case read := <-reads:
			t.Fatal("unscheduled read", read)
		case <-time.After(450 * time.Millisecond):
		}
	}
	setMode := func(session, mode string) { modes.Store(session, controlledFacts{*operation.FirstActivationAt, mode}) }
	completed := "checkout.session.completed"
	payment := "checkout.session.async_payment_succeeded"
	activateAt := *operation.FirstActivationAt
	stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals",
		signal(f, "cs_before", payment, activateAt.Add(-time.Second)), nil, 409)
	assertQuiet()
	waitWorkerCase(t, unknownBinding.request.WorkspaceID, unknownBinding.request.CaseID, "RECEIVED", "SIGNAL_ADMITTED")
	assertCaseWorkerAttempted(t, unknownBinding.request)
	assertQuiet()
	decisionCounts(t, unknownBinding.request.WorkspaceID, 0, 1, 0)

	for _, tc := range []struct{ mode, reason, state string }{
		{"unavailable", "READER_UNAVAILABLE", "WAITING_DATA"},
		{"incomplete", "UNKNOWN_FACT", "WAITING_DATA"},
		{"unpaid", "PAYMENT_STATUS", "WAITING_DATA"},
		{"old", "BEFORE_ACTIVATION", "EXCLUDED"},
	} {
		session := "cs_" + tc.mode
		setMode(session, tc.mode)
		initial := signal(f, session, completed, activateAt)
		id, receipt := accept(initial)
		awaitRead(f.workspace, session, id)
		waitWorkerCase(t, f.workspace, id, tc.state, tc.reason)
		if again, replay := accept(initial); again != id || !bytes.Equal(receipt, replay) {
			t.Fatal("lost ACK did not reproduce original receipt")
		}
		assertQuiet()
	}

	session := "cs_unpaid"
	setMode(session, "hold")
	late := signal(f, session, payment, activateAt)
	caseID, lateAck := accept(late)
	awaitRead(f.workspace, session, caseID)
	// A second durable admission proceeds while Intake is waiting for a reply,
	// so it cannot be waiting on a lock taken by the financial reader.
	id, _ := accept(signal(f, session, payment, activateAt))
	if id != caseID {
		t.Fatal("late signals for one Session opened multiple cases")
	}
	if err := start.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-start.done
	setMode(session, "paid")
	restarted := stand.boot("case-worker-restart.log")
	stand.pollHealth(restarted, "READY")
	if again, ack := accept(late); again != caseID || !bytes.Equal(lateAck, ack) {
		t.Fatal("restart changed the late admission receipt")
	}
	awaitRead(f.workspace, session, caseID)
	waitWorkerCase(t, f.workspace, caseID, "WORKING", "FINANCIAL_ELIGIBILITY_ONLY")
	var message string
	select {
	case message = <-stand.deliverySeen:
	case <-time.After(6 * time.Second):
		t.Fatal("automatic paid case did not reach the established sender")
	}
	assertWorkerDelivery(t, f.workspace, caseID, message)
	accept(signal(f, session, payment, activateAt))
	assertQuiet()
	if stand.deliveryCount.Load() != 1 {
		t.Fatal("committed lineage was dispatched again")
	}

	// Pause during a read: commit must see the changed operation even though
	// the response is paid.
	modes.Store("cs_pause", controlledFacts{*pauseOp.FirstActivationAt, "hold"})
	pauseID, _ := accept(signal(pauseScope, "cs_pause", completed, *pauseOp.FirstActivationAt))
	awaitRead(pauseScope.workspace, "cs_pause", pauseID)
	path := "/api/v1/workspaces/" + pauseScope.workspace + "/operations/" + pauseOp.OperationID + "/lifecycle"
	command := drafts.LifecycleCommand{Command: pauseScope.meta(&pauseOp.Revision), Action: lifecycle.Pause}
	body := stand.command(path, command, command.Command.RequestID, 202)
	var draining drafts.Operation
	if json.Unmarshal(body, &draining) != nil || draining.State != "DRAINING" {
		t.Fatal(string(body))
	}
	close(release)
	assertQuiet()
	waitWorkerCase(t, pauseScope.workspace, pauseID, "RECEIVED", "SIGNAL_ADMITTED")
	decisionCounts(t, pauseScope.workspace, 0, 1, 0)
	stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals",
		signal(pauseScope, "cs_during_pause", completed, *pauseOp.FirstActivationAt), nil, 409)

	// Revoke the other workspace after ClientHello but before the source
	// request. The actual raw write must still check the shared gate.
	stand.pauseSourceHandshake.Store(true)
	setMode("cs_revoke", "paid")
	revokeCase, _ := accept(signal(f, "cs_revoke", completed, activateAt))
	select {
	case <-stand.handshakeEntered:
	case <-time.After(4 * time.Second):
		t.Fatal("source TLS did not reach controlled handshake")
	}
	stand.bootMu.Lock()
	boot := stand.current
	stand.bootMu.Unlock()
	revokeID, _ := newID()
	commandBarrier := drafts.BarrierCommand{TargetRuntimeID: stand.identity, BootID: boot.BootID, Generation: boot.Generation,
		RevocationID: revokeID, WorkspaceID: &f.workspace, Scope: "WORKSPACE", SubjectID: f.workspace, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	reply := stand.must("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", marshalProcess(t, commandBarrier), nil, 200)
	if draftjson.Validate("BarrierAck", reply) != nil {
		t.Fatal(string(reply))
	}
	close(stand.handshakeProceed)
	assertQuiet()
	state, _, count := readWorkerCase(t, f.workspace, revokeCase)
	if state == "WORKING" || count != 0 || stand.deliveryCount.Load() != 1 {
		t.Fatal("revoked case created an intent", state, count)
	}
	t.Log("PROCESS PASS: owned mTLS admission and SourceFacts, fresh proof clock, incomplete/unpaid/unavailable, late payment and kill/restart, ACK dedup, one delivery receipt without confirmation, pause during read and raw-write barrier")
}

func assertCaseWorkerAttempted(t *testing.T, r preparecase.Request) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		tx, err := (dispatchTestDB{}).BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var claimed bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.job_index
 WHERE workspace_id=$1 AND job_id=$2 AND kind='CASE_PREPARE' AND claim_token IS NOT NULL)`, r.WorkspaceID, r.CaseID).Scan(&claimed)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if claimed {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatal("test never exercised the worker's binding check")
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func readWorkerCase(t *testing.T, workspace, id string) (string, string, int) {
	t.Helper()
	c := connect(t)
	defer c.Close(ctx)
	tx := begin(t, c, workspace)
	defer tx.Rollback(ctx)
	var state, reason string
	var outbox int
	err := tx.QueryRow(ctx, `SELECT k.state,
 (SELECT reason_code FROM hp_processing.safety_history WHERE workspace_id=k.workspace_id AND case_id=k.case_id ORDER BY at DESC LIMIT 1),
 (SELECT count(*) FROM hp_processing.outbox WHERE workspace_id=k.workspace_id AND message_id=k.lineage_id)
 FROM hp_processing.case_identity k WHERE k.workspace_id=$1 AND k.case_id=$2`, workspace, id).Scan(&state, &reason, &outbox)
	if err != nil {
		t.Fatal(err)
	}
	return state, reason, outbox
}

func waitWorkerCase(t *testing.T, ws, id, state, reason string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for {
		gotState, gotReason, n := readWorkerCase(t, ws, id)
		if gotState == state && gotReason == reason && (n == 1) == (state == "WORKING") {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("want %s/%s, got %s/%s (%d intents)", state, reason, gotState, gotReason, n)
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func assertWorkerDelivery(t *testing.T, workspace, caseID, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		c := connect(t)
		tx := begin(t, c, workspace)
		var outbox, lineage, state string
		var n, episode int
		err := tx.QueryRow(ctx, `SELECT o.state,k.lineage_id::text,k.state,o.delivery_count,
 (SELECT count(*) FROM hp_processing.episodes WHERE workspace_id=k.workspace_id AND case_id=k.case_id)
 FROM hp_processing.case_identity k JOIN hp_processing.outbox o ON o.workspace_id=k.workspace_id AND o.message_id=k.lineage_id
 WHERE k.workspace_id=$1 AND k.case_id=$2`, workspace, caseID).Scan(&outbox, &lineage, &state, &n, &episode)
		tx.Rollback(ctx)
		c.Close(ctx)
		if err != nil || lineage != message || state != "WORKING" || n != 1 || episode != 1 {
			t.Fatalf("delivery identity/outcome: %s %s %s %d %d %v", outbox, lineage, state, n, episode, err)
		}
		if outbox == "ACKED" {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatal("Delivery receipt was not durably recorded")
		}
		time.Sleep(40 * time.Millisecond)
	}
}
