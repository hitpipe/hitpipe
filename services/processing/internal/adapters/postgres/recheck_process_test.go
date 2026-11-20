package postgres

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

type queryTestPlan struct {
	attempts int
	wire     []byte
	reply    func(http.ResponseWriter, *http.Request, int)
}

func TestProcessingRecheckOwnedStand(t *testing.T) {
	binary := os.Getenv("HIT111_PROCESSING_BIN")
	if binary == "" {
		t.Skip("requires isolated PG18.7 and executable")
	}
	f, rev := knownFixture(t)
	unknown := prepared(t, []string{})
	ws, caseID := f.request.WorkspaceID, f.request.CaseID
	stand := createProcessStand(t, binary, f.path, map[string]string{ws: "acct_synthetic", unknown.request.WorkspaceID: "acct_synthetic"})
	stand.manualCases = map[string]bool{unknown.request.WorkspaceID: true}
	var mu sync.Mutex
	plans := map[string]*queryTestPlan{}
	stand.queryReply = func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(r.Body, 4097))
		var q drafts.DeliveryQuery
		if r.Method != "POST" || err != nil || len(raw) > 4096 || draftjson.Validate("DeliveryQuery", raw) != nil ||
			json.Unmarshal(raw, &q) != nil || q.WorkspaceID != ws || q.CaseID != caseID || q.LineageID != f.lineage ||
			q.NativeTaskID == nil || *q.NativeTaskID != "86synthetic99" || q.ExpectedConnectionEpoch != 7 ||
			!q.DeadlineAt.After(time.Now()) || time.Until(q.DeadlineAt) > 30*time.Second {
			t.Errorf("bad query request: %s %s", r.Method, raw)
			w.WriteHeader(400)
			return
		}
		id := r.Header.Get("X-Request-ID")
		mu.Lock()
		plan := plans[id]
		if plan == nil {
			mu.Unlock()
			t.Errorf("unexpected query %s", id)
			w.WriteHeader(400)
			return
		}
		plan.attempts++
		n := plan.attempts
		if n == 1 {
			plan.wire = bytes.Clone(raw)
		}
		same := bytes.Equal(raw, plan.wire)
		mu.Unlock()
		if !same {
			t.Error("retry changed query")
			w.WriteHeader(409)
			return
		}
		plan.reply(w, r, n)
	}
	proc := stand.boot("recheck-initial.log")
	stand.pollHealth(proc, "CLOSED")
	stand.iamOpen.Store(true)
	stand.pollHealth(proc, "READY")
	detailURL := "/api/v1/workspaces/" + ws + "/cases/" + caseID
	recoveryURL := detailURL + "/recovery"
	headers := func(id string) map[string]string {
		return map[string]string{"X-HitPipe-Session": stand.token, "X-CSRF-Token": stand.csrf, "X-Request-ID": id}
	}
	view := func() (v readcases.CaseDetail) {
		t.Helper()
		raw := stand.must("bff.test", "8443", "GET", detailURL, nil, headers(""), 200)
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return
	}
	if v := view(); v.Case.Revision != rev || v.Case.State != "UNKNOWN" || v.Case.TargetURL != nil ||
		len(v.AvailableActions) != 1 || v.AvailableActions[0] != "RECHECK" {
		t.Fatalf("initial case: %+v", v)
	}
	post := func(url string, c drafts.RecoveryCommand, h map[string]string, status int) []byte {
		t.Helper()
		return stand.must("bff.test", "8443", "POST", url, marshalProcess(t, c), h, status)
	}
	success := func(c drafts.RecoveryCommand) readcases.CaseDetail {
		t.Helper()
		var v readcases.CaseDetail
		if err := json.Unmarshal(post(recoveryURL, c, headers(c.Command.RequestID), 202), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	id, _ := newID()
	valid := recheckCommand(t, id, rev)
	body := marshalProcess(t, valid)
	stand.must("delivery.test", "8443", "POST", recoveryURL, body, headers(id), 403)
	h := headers(id)
	delete(h, "X-HitPipe-Session")
	post(recoveryURL, valid, h, 401)
	h = headers(id)
	h["X-CSRF-Token"] = "bad-bad-bad-bad-bad-bad-bad-bad-bad-bad"
	post(recoveryURL, valid, h, 403)
	stand.denyRecover.Store(true)
	post(recoveryURL, valid, headers(id), 403)
	if len(view().AvailableActions) != 0 {
		t.Fatal("read advertised denied action")
	}
	stand.denyRecover.Store(false)
	for _, action := range []string{"BIND_TASK", "CLOSE_WITHOUT_CONFIRMATION", "CONTINUE_SAFE", "RECONSIDER", "REOPEN_CLOSED"} {
		x := valid
		x.Action = action
		post(recoveryURL, x, headers(id), 400)
	}
	x := valid
	x.NativeTaskID = draftString("another")
	post(recoveryURL, x, headers(id), 409)
	x = recheckCommand(t, id, rev+1)
	post(recoveryURL, x, headers(id), 412)
	h = headers(id)
	h["If-Match"] = "99"
	post(recoveryURL, valid, h, 400)
	post("/api/v1/workspaces/"+ws+"/cases/"+unknown.request.CaseID+"/recovery", valid, headers(id), 404)
	post("/api/v1/workspaces/"+unknown.request.WorkspaceID+"/cases/"+unknown.request.CaseID+"/recovery",
		recheckCommand(t, id, 1), headers(id), 409)
	recheckCounts(t, ws, 0)

	reply := func(w http.ResponseWriter, o observations.TaskObservation) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(observationWire(t, o))
	}
	install := func(id string, f func(http.ResponseWriter, *http.Request, int)) {
		mu.Lock()
		defer mu.Unlock()
		plans[id] = &queryTestPlan{reply: f}
	}
	attempts := func(id string) int {
		mu.Lock()
		defer mu.Unlock()
		return plans[id].attempts
	}
	untilHTTP := func(ch <-chan struct{}, label string) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(6 * time.Second):
			t.Fatal(label)
		}
	}
	firstStarted := make(chan struct{})
	install(id, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			close(firstStarted)
			<-r.Context().Done()
			return
		}
		reply(w, f.observation(t, "RESULT_VERIFIED"))
	})
	accepted := success(valid)
	if accepted.Case.State != "UNKNOWN" || accepted.Case.OutcomeKnowledge != "TARGET_ID_KNOWN" ||
		accepted.Case.Revision != rev || accepted.Case.TargetURL != nil {
		t.Fatalf("202 confirmed a provider result: %+v", accepted)
	}
	untilHTTP(firstStarted, "query did not start")
	again := success(valid)
	if again.Case.State != "UNKNOWN" || attempts(id) != 1 {
		t.Fatal("early replay changed result")
	}
	x = valid
	x.Reason = "different body"
	post(recoveryURL, x, headers(id), 409)
	recheckCounts(t, ws, 1)
	if err := proc.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-proc.done
	restarted := stand.boot("recheck-restart.log")
	stand.pollHealth(restarted, "READY")
	success(valid)
	waitRecheckOutbox(t, ws, id, "ACKED", 2, 14*time.Second)
	if v := view(); v.Case.State != "CONFIRMED" || v.Case.OutcomeKnowledge != "RESULT_VERIFIED" ||
		v.Case.TargetURL == nil || v.Case.Revision != rev+1 || !v.ObservationComplete {
		t.Fatalf("HIT-99 result missing: %+v", v)
	}
	recheckCounts(t, ws, 1)

	for n, scenario := range []string{"wrong_task", "wrong_lineage", "old_reply", "incomplete_wire", "provider_timeout", "429_then_503"} {
		id, _ := newID()
		before := view()
		install(id, func(w http.ResponseWriter, r *http.Request, attempt int) {
			o := f.observation(t, "RESULT_VERIFIED")
			switch scenario {
			case "wrong_task":
				o.TaskID = draftString("not-the-known-task")
			case "wrong_lineage":
				o.LineageID = owner
			case "old_reply":
				o.ObservedAt = time.Now().Add(-time.Minute)
			case "incomplete_wire":
				o.TaskID = nil
			case "provider_timeout":
				<-r.Context().Done()
				return
			case "429_then_503":
				if attempt == 1 {
					w.Header().Set("Retry-After", "1")
					w.WriteHeader(429)
					return
				}
				if attempt == 2 {
					w.WriteHeader(503)
					return
				}
				o.Knowledge = "INCOMPLETE"
				o.Complete = false
				o.EvidenceRef, o.EvidenceExpiresAt = nil, nil
			}
			reply(w, o)
		})
		success(recheckCommand(t, id, before.Case.Revision))
		state, wantAttempts := "HELD", 1
		if scenario == "provider_timeout" {
			wantAttempts = 3
		}
		if scenario == "429_then_503" {
			state, wantAttempts = "ACKED", 3
		}
		waitRecheckOutbox(t, ws, id, state, wantAttempts, 11*time.Second)
		after := view()
		if scenario == "429_then_503" {
			if after.Case.State != "UNKNOWN" || after.Case.OutcomeKnowledge != "RESULT_VERIFIED" ||
				after.Case.Revision != before.Case.Revision+1 || after.Case.TargetURL != nil || after.ObservationComplete {
				t.Fatalf("%s: %+v", scenario, after)
			}
		} else if after.Case.Revision != before.Case.Revision || after.Case.State != before.Case.State ||
			after.Case.OutcomeKnowledge != before.Case.OutcomeKnowledge {
			t.Fatalf("%s changed case: %+v", scenario, after)
		}
		recheckCounts(t, ws, n+2)
	}

	// The competing observation uses the ordinary inbound mTLS route while
	// the older query response is still on the wire.
	id, _ = newID()
	started, release := make(chan struct{}), make(chan struct{})
	install(id, func(w http.ResponseWriter, r *http.Request, _ int) {
		o := f.observation(t, "RESULT_VERIFIED")
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		reply(w, o)
	})
	success(recheckCommand(t, id, view().Case.Revision))
	untilHTTP(started, "racing query not started")
	mismatch := f.observation(t, "RESULT_MISMATCH")
	no := false
	mismatch.ListMatches = &no
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", observationWire(t, mismatch), nil, 202)
	close(release)
	waitRecheckOutbox(t, ws, id, "HELD", 1, 6*time.Second)
	v := view()
	if v.Case.State != "ATTENTION" || v.Case.OutcomeKnowledge != "RESULT_MISMATCH" || v.Case.TargetURL != nil {
		t.Fatalf("older observation changed the current knowledge: %+v", v)
	}
	raw := stand.must("bff.test", "8443", "GET", detailURL+"/history", nil, headers(""), 200)
	if !bytes.Contains(raw, []byte("RECHECK_ACCEPTED")) || !bytes.Contains(raw, []byte("DELIVERY_OBSERVATION_LATE")) {
		t.Fatal("history is missing command or outcome")
	}

	// A revoke between TLS ClientHello and the first HTTP write must prevent
	// the queried endpoint from being called, even on an established lease.
	stand.pauseHandshake.Store(true)
	id, _ = newID()
	install(id, func(w http.ResponseWriter, r *http.Request, _ int) {
		t.Error("old query passed revoked actual-start boundary")
		w.WriteHeader(500)
	})
	success(recheckCommand(t, id, v.Case.Revision))
	untilHTTP(stand.handshakeEntered, "query TLS never started")
	stand.bootMu.Lock()
	boot := stand.current
	stand.bootMu.Unlock()
	revokeID, _ := newID()
	barrier := drafts.BarrierCommand{TargetRuntimeID: stand.identity, BootID: boot.BootID, Generation: boot.Generation,
		RevocationID: revokeID, WorkspaceID: &ws, Scope: "WORKSPACE", SubjectID: ws, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	ack := stand.must("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", marshalProcess(t, barrier), nil, 200)
	if draftjson.Validate("BarrierAck", ack) != nil {
		t.Fatal(string(ack))
	}
	close(stand.handshakeProceed)
	time.Sleep(350 * time.Millisecond)
	if attempts(id) != 0 || stand.deliveryCount.Load() != 0 {
		t.Fatal("unexpected HTTP query or create")
	}
	caseState, knowledge, _, task, fenced, _ := f.stored(t)
	if caseState != "ATTENTION" || knowledge != "RESULT_MISMATCH" || !fenced || task == nil || *task != "86synthetic99" {
		t.Fatal(caseState, knowledge, task, fenced)
	}
	t.Log("PROCESS PASS: recoverCase 202 and HIT-99, access/CSRF/mTLS, one queued exact query, SIGKILL/restart, bounded timeout/429/503, incomplete and mismatched responses, late racing observation, revoked raw write and no Create")
}

func waitRecheckOutbox(t *testing.T, ws, id, wanted string, wantCount int, limit time.Duration) {
	t.Helper()
	c := connect(t)
	defer c.Close(ctx)
	deadline := time.Now().Add(limit)
	for {
		tx := begin(t, c, ws)
		var state string
		var n int
		err := tx.QueryRow(ctx, `SELECT state,delivery_count FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2 AND kind='DELIVERY_QUERY'`, ws, id).Scan(&state, &n)
		tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state == wanted && n == wantCount {
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("query %s: %s %d want %s %d", id, state, n, wanted, wantCount)
		}
		time.Sleep(45 * time.Millisecond)
	}
}
