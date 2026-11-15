package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Controlled owner ports only, never a production grant or Delivery readback.
type observationAuthority struct {
	mu     sync.Mutex
	grant  observations.Grant
	denied bool
}

func (a *observationAuthority) Authorize(c context.Context, q observations.Access) (observations.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.denied || q.WorkspaceID != a.grant.WorkspaceID {
		return observations.Grant{}, observations.ErrDenied
	}
	return a.grant, nil
}

type observationGate struct {
	mu             sync.Mutex
	epoch          int64
	closed, noCall bool
	denyPhase      string
}

func (g *observationGate) Start(c context.Context, a observations.Grant, phase string, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed || g.epoch != a.Epoch || g.denyPhase == phase || c.Err() != nil {
		return observations.ErrDenied
	}
	if g.noCall {
		return nil
	}
	return work(c)
}

type observationFixture struct {
	preparedFixture
	repo                  ObservationStore
	service               observations.Service
	authority             *observationAuthority
	gate                  *observationGate
	pki                   *testfixtures.PKI
	client                *http.Client
	url, lineage, episode string
}

func newObservation(t *testing.T) observationFixture {
	t.Helper()
	f := prepared(t, []string{})
	result, e := f.service.Prepare(ctx, f.request)
	if e != nil {
		t.Fatal(e)
	}
	pool := newReadPool(t, 3)
	repo := ObservationStore{pool, f.files, time.Hour}
	authority := &observationAuthority{grant: observations.Grant{WorkspaceID: f.request.WorkspaceID, PrincipalID: owner, OwnerID: owner, OperationID: op, ConnectionID: destination, Epoch: 7, ExpiresAt: time.Now().Add(time.Hour), Allowed: true}}
	gate := &observationGate{epoch: 7}
	pki := testfixtures.NewPKI(t)
	service := observations.Service{Repository: repo, Authority: authority, Boundary: gate, Clock: testClock{time.Now}, Codec: observationjson.Codec{}}
	fixture := observationFixture{preparedFixture: f, repo: repo, service: service, authority: authority, gate: gate, pki: pki, lineage: result.MessageID}
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	if e = tx.QueryRow(ctx, `SELECT episode_id::text FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2`, f.request.WorkspaceID, f.request.CaseID).Scan(&fixture.episode); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	fixture.start(t)
	return fixture
}
func (f *observationFixture) start(t *testing.T) {
	t.Helper()
	server, e := observationhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, f.service, "delivery.test", 1<<16)
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", server.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go server.ServeTLS(l, "", "")
	t.Cleanup(func() { server.Close(); l.Close() })
	f.url = "https://" + l.Addr().String()
	tr := f.pki.Transport(t)
	tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, false)}
	f.client = &http.Client{Transport: tr, Timeout: 3 * time.Second}
	t.Cleanup(f.client.CloseIdleConnections)
}
func (f observationFixture) observation(t *testing.T, kind string) observations.TaskObservation {
	t.Helper()
	message, _ := newID()
	observation, _ := newID()
	ref, _ := newID()
	task := "controlled_task_001"
	yes := true
	expiry := time.Now().Add(time.Hour)
	return observations.TaskObservation{MessageID: message, SchemaVersion: "1.0", WorkspaceID: f.request.WorkspaceID, CaseID: f.request.CaseID, LineageID: f.lineage, EpisodeID: f.episode, ObservationID: observation, Knowledge: kind, TaskID: &task, ObservedAt: time.Now().UTC().Truncate(time.Microsecond), Complete: true, WorkspaceMatches: &yes, ListMatches: &yes, AssigneeMatches: &yes, TitleMatches: &yes, MarkerMatches: &yes, ContextMatches: &yes, ProofCode: "CONTROLLED_DELIVERY_READBACK", ProofDigest: "sha256:" + strings.Repeat("a", 64), EvidenceRef: &ref, EvidenceExpiresAt: &expiry}
}
func observationWire(t *testing.T, o observations.TaskObservation) []byte {
	t.Helper()
	b, _ := json.Marshal(o)
	var m map[string]any
	json.Unmarshal(b, &m)
	delete(m, "message_digest")
	b, _ = json.Marshal(m)
	canon, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = observationjson.Digest(canon)
	b, _ = json.Marshal(m)
	return b
}
func (f observationFixture) call(t *testing.T, b []byte, headers map[string]string) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest("POST", f.url+"/internal/v1/delivery-observations", bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := newID()
	r.Header.Set("X-Request-ID", id)
	r.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	response, e := f.client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	body, e := io.ReadAll(response.Body)
	if e != nil {
		t.Fatal(e)
	}
	return response.StatusCode, body
}
func observationCapture(t *testing.T, scenario, operation, schema string, status int, request any, b []byte) {
	t.Helper()
	path := os.Getenv("HIT99_CAPTURE_FILE")
	if path == "" {
		return
	}
	var response any
	if json.Unmarshal(b, &response) != nil {
		t.Fatal("non JSON capture", string(b))
	}
	raw, e := json.Marshal(map[string]any{"scenario": scenario, "operation": operation, "schema": schema, "status": status, "request": request, "response": response, "actual_pg187_mtls": true})
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = f.Write(append(raw, '\n')); e != nil {
		t.Fatal(e)
	}
	if e = f.Sync(); e != nil {
		t.Fatal(e)
	}
}
func (f observationFixture) apply(t *testing.T, o observations.TaskObservation, want int, scenario string) []byte {
	t.Helper()
	raw := observationWire(t, o)
	status, b := f.call(t, raw, nil)
	if status != want {
		t.Fatal(scenario, status, string(b))
	}
	var request any
	json.Unmarshal(raw, &request)
	schema := "Receipt"
	if status != 202 {
		schema = "Error"
	}
	observationCapture(t, scenario, "applyTaskObservation", schema, status, request, b)
	return b
}
func (f observationFixture) stored(t *testing.T) (string, string, int64, *string, bool, *time.Time) {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	var state, outcome string
	var revision int64
	var task *string
	var fence bool
	var confirmed *time.Time
	if e := tx.QueryRow(ctx, `SELECT state,outcome_knowledge,revision,known_task_id,ever_dispatch_fenced,last_confirmed_at FROM hp_processing.case_identity WHERE workspace_id=$1 AND case_id=$2`, f.request.WorkspaceID, f.request.CaseID).Scan(&state, &outcome, &revision, &task, &fence, &confirmed); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	return state, outcome, revision, task, fence, confirmed
}
func (f observationFixture) counts(t *testing.T, n int) {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	for table, want := range map[string]int{"case_identity": 1, "outbox": 1, "episodes": 1, "inbox": 1 + n, "safety_history": 2 + n, "history_content": 2 + n} {
		var count int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table+" WHERE workspace_id=$1", f.request.WorkspaceID).Scan(&count); e != nil || count != want {
			t.Fatal(table, count, want, e)
		}
	}
	tx.Rollback(ctx)
}
func (f observationFixture) readAPI(t *testing.T, scenario string) readcases.CaseDetail {
	t.Helper()
	reader := readStore(t, f.preparedFixture, newReadPool(t, 1))
	service := readcases.Service{Authority: readAuthority{readGrant(f.request.WorkspaceID)}, Boundary: &readBoundary{epoch: 7}, Clock: testClock{time.Now}, Repository: reader}
	server, e := casereadhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, service, "bff.test")
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", server.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go server.ServeTLS(l, "", "")
	defer server.Close()
	defer l.Close()
	client := &http.Client{Transport: f.pki.Transport(t), Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	var detail readcases.CaseDetail
	for _, item := range []struct{ path, op, schema string }{{"", "listCases", "Cases"}, {"/" + f.request.CaseID, "getCase", "CaseDetail"}, {"/" + f.request.CaseID + "/history", "getCaseHistory", "History"}} {
		r, _ := http.NewRequest("GET", "https://"+l.Addr().String()+"/api/v1/workspaces/"+f.request.WorkspaceID+"/cases"+item.path, nil)
		r.Header.Set("X-HitPipe-Session", "controlled-session")
		res, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil || res.StatusCode != 200 {
			t.Fatal(item.op, res.StatusCode, string(b), e)
		}
		observationCapture(t, scenario, item.op, item.schema, res.StatusCode, nil, b)
		if item.op == "getCase" {
			if json.Unmarshal(b, &detail) != nil {
				t.Fatal("detail decode")
			}
		}
	}
	return detail
}
func TestObservationRealMTLSPGCommitBeforeSenderACKAndReplay(t *testing.T) {
	f := newObservation(t)
	o := f.observation(t, "RESULT_VERIFIED")
	body := f.apply(t, o, 202, "verified_before_sender_ack")
	state, outcome, revision, task, fenced, confirmed := f.stored(t)
	if state != "CONFIRMED" || outcome != "RESULT_VERIFIED" || revision != 3 || task == nil || *task != *o.TaskID || !fenced || confirmed == nil || !confirmed.Equal(o.ObservedAt) {
		t.Fatal(state, outcome, revision, task, fenced, confirmed)
	}
	detail := f.readAPI(t, "verified_projection")
	if detail.Case.State != "CONFIRMED" || detail.Case.Revision != 3 || detail.Case.OutcomeKnowledge != "RESULT_VERIFIED" || !detail.ObservationComplete || detail.LastObservationAt == nil || !detail.LastObservationAt.Equal(o.ObservedAt) {
		t.Fatal(detail)
	}
	again := f.apply(t, o, 202, "same_message_original_receipt")
	if !bytes.Equal(body, again) {
		t.Fatal("receipt changed")
	}
	f.counts(t, 1)
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	var outbox string
	tx.QueryRow(ctx, `SELECT state FROM hp_processing.outbox WHERE workspace_id=$1`, f.request.WorkspaceID).Scan(&outbox)
	tx.Rollback(ctx)
	if outbox != "PENDING" {
		t.Fatal("observation depended on/changed sender ACK", outbox)
	}
	changed := o
	changed.ProofCode = "CONTROLLED_DIFFERENT_BODY"
	f.apply(t, changed, 409, "same_id_changed_digest_conflicts")
	f.counts(t, 1)
}
func TestObservationAllKnowledgeAndPartialMatches(t *testing.T) {
	for _, kind := range []string{"RESULT_VERIFIED", "TARGET_ID_OBSERVED", "POSSIBLE_EFFECT", "RESULT_MISMATCH", "INCOMPLETE", "KNOWN_NO_EFFECT", "missing_match", "expired_evidence", "missing_evidence", "false_match", "incomplete_verified"} {
		t.Run(kind, func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, kind)
			wantState, wantOutcome := "UNKNOWN", "POSSIBLE_EFFECT"
			switch kind {
			case "RESULT_VERIFIED":
				wantState, wantOutcome = "CONFIRMED", "RESULT_VERIFIED"
			case "TARGET_ID_OBSERVED":
				wantOutcome = "TARGET_ID_KNOWN"
				o.Complete = false
			case "RESULT_MISMATCH":
				no := false
				o.ListMatches = &no
				wantState, wantOutcome = "ATTENTION", "RESULT_MISMATCH"
			case "KNOWN_NO_EFFECT":
				o.TaskID = nil
				o.WorkspaceMatches = nil
				o.ListMatches = nil
				o.AssigneeMatches = nil
				o.TitleMatches = nil
				o.MarkerMatches = nil
				o.ContextMatches = nil
				wantState, wantOutcome = "ATTENTION", "NO_EFFECT"
			case "missing_match", "expired_evidence", "missing_evidence", "false_match", "incomplete_verified":
				o.Knowledge = "RESULT_VERIFIED"
				wantState, wantOutcome = "WORKING", "UNESTABLISHED"
				switch kind {
				case "missing_match":
					o.MarkerMatches = nil
				case "expired_evidence":
					x := time.Now().Add(-time.Second)
					o.EvidenceExpiresAt = &x
				case "missing_evidence":
					o.EvidenceRef = nil
				case "false_match":
					no := false
					o.ContextMatches = &no
				case "incomplete_verified":
					o.Complete = false
				}
			default:
				o.TaskID = nil
				o.Complete = false
			}
			f.apply(t, o, 202, kind)
			state, outcome, _, _, _, _ := f.stored(t)
			if state != wantState || outcome != wantOutcome {
				t.Fatal(kind, state, outcome)
			}
			detail := f.readAPI(t, "projection_"+kind)
			if wantState != "CONFIRMED" && detail.Case.TargetURL != nil {
				t.Fatal("uncertainty has verified URL")
			}
			f.counts(t, 1)
		})
	}
}
func TestObservationLateReorderedIdentityAndNewMismatch(t *testing.T) {
	f := newObservation(t)
	first := f.observation(t, "RESULT_VERIFIED")
	f.apply(t, first, 202, "first_confirmation")
	old := f.observation(t, "INCOMPLETE")
	old.TaskID = nil
	old.Complete = false
	old.ObservedAt = first.ObservedAt.Add(-time.Microsecond)
	f.apply(t, old, 202, "late_unknown_does_not_rollback")
	state, outcome, _, task, fence, confirmed := f.stored(t)
	if state != "CONFIRMED" || outcome != "RESULT_VERIFIED" || task == nil || !fence || confirmed == nil {
		t.Fatal("late regressed")
	}
	conflict := f.observation(t, "RESULT_VERIFIED")
	foreign := "different_task"
	conflict.TaskID = &foreign
	f.apply(t, conflict, 202, "identity_conflict_held")
	state, _, _, task, _, _ = f.stored(t)
	if state != "CONFIRMED" || *task != *first.TaskID {
		t.Fatal("identity reassigned")
	}
	mismatch := f.observation(t, "RESULT_MISMATCH")
	no := false
	mismatch.TitleMatches = &no
	f.apply(t, mismatch, 202, "later_actual_mismatch")
	state, outcome, _, task, fence, confirmed = f.stored(t)
	if state != "ATTENTION" || outcome != "RESULT_MISMATCH" || !fence || *task != *first.TaskID || !confirmed.Equal(first.ObservedAt) {
		t.Fatal("mismatch/history not preserved")
	}
	incomplete := f.observation(t, "INCOMPLETE")
	incomplete.TaskID = nil
	incomplete.Complete = false
	f.apply(t, incomplete, 202, "later_unreadable_retains_identity_and_confirmation")
	state, outcome, _, task, fence, confirmed = f.stored(t)
	if state != "UNKNOWN" || outcome != "RESULT_MISMATCH" || !fence || task == nil || !confirmed.Equal(first.ObservedAt) {
		t.Fatal("unknown regressed known outcome")
	}
	detail := f.readAPI(t, "later_unknown_projection")
	if detail.Case.TargetURL != nil {
		t.Fatal("invented URL")
	}
	f.counts(t, 5)
}
