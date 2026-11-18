package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
)

type controlledQueue struct {
	mu               sync.Mutex
	now              func() time.Time
	current          lifecycle.QueueEvidence
	missing, changed bool
	held             []admission.Signal
	applied          map[string]bool
	discarded        int64
}

func (q *controlledQueue) Capture(_ context.Context, _ drafts.Grant, s lifecycle.QueueScope) (lifecycle.QueueEvidence, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.missing {
		return lifecycle.QueueEvidence{}, drafts.ErrUnavailable
	}
	q.current.Scope = s
	q.current.ObservedAt = q.now().UTC().Truncate(time.Microsecond)
	q.current.ExpiresAt = q.current.ObservedAt.Add(10 * time.Minute)
	q.current.SignalCount = int64(len(q.held))
	q.current.SerializedBytes = 0
	q.current.OldestReceivedAt = nil
	for _, signal := range q.held {
		payload, err := json.Marshal(signal)
		if err != nil {
			return lifecycle.QueueEvidence{}, err
		}
		q.current.SerializedBytes += int64(len(payload))
		if q.current.OldestReceivedAt == nil || signal.FirstReceivedAt.Before(*q.current.OldestReceivedAt) {
			oldest := signal.FirstReceivedAt.UTC().Truncate(time.Microsecond)
			q.current.OldestReceivedAt = &oldest
		}
	}
	return q.current, nil
}

func (q *controlledQueue) WithCurrent(ctx context.Context, _ drafts.Grant, e lifecycle.QueueEvidence, f func(context.Context) error) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.missing {
		return drafts.ErrUnavailable
	}
	if q.changed || e.Generation != q.current.Generation || !e.Valid(q.current.Scope, q.now()) {
		return drafts.ErrConflict
	}
	return f(ctx)
}

// apply is the stand's queue-owner action. It uses the exact durable
// decision read back from Processing and never touches an external provider.
func (q *controlledQueue) apply(decision lifecycle.ResumeDecision) ([]admission.Signal, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.applied[decision.RequestID] {
		return nil, nil
	}
	if decision.Generation != q.current.Generation || decision.SignalCount != int64(len(q.held)) ||
		decision.SerializedBytes != q.current.SerializedBytes || !decision.Scope.PauseAcceptedAt.Equal(q.current.Scope.PauseAcceptedAt) {
		return nil, drafts.ErrConflict
	}
	var release []admission.Signal
	switch decision.Disposition {
	case lifecycle.Process:
		release = append(release, q.held...)
	case lifecycle.Discard:
		q.discarded += decision.SignalCount
	default:
		return nil, drafts.ErrConflict
	}
	q.applied[decision.RequestID] = true
	q.held = nil
	return release, nil
}

type lifecycleFixture struct {
	activationFixture
	op        drafts.Operation
	lifecycle LifecycleStore
	queue     *controlledQueue
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	f := newActivationFixture(t)
	in := f.ready(t)
	status, body := f.activate(t, in, nil)
	var op drafts.Operation
	if status != 201 || json.Unmarshal(body, &op) != nil {
		t.Fatalf("activation %d %s", status, body)
	}
	generation, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	x := &lifecycleFixture{activationFixture: f, op: op, lifecycle: LifecycleStore{Drafts: f.repo},
		queue: &controlledQueue{now: time.Now, current: lifecycle.QueueEvidence{Generation: generation, Complete: true,
			Gaps: []string{"Known earlier expiry: one signal"}}, applied: make(map[string]bool)}}
	x.startLifecycle(t)
	return x
}

func (f *lifecycleFixture) startLifecycle(t *testing.T) {
	t.Helper()
	f.startActivation(t, func(s *http.Server) error {
		return lifecyclehttp.Attach(s, lifecycle.Service{Repository: f.lifecycle, Authority: f.authority, Boundary: f.gate,
			Bindings: f.bindings, Queue: f.queue, Clock: f.lifecycle.Drafts.Clock, Codec: draftjson.Codec{}}, "bff.test")
	})
}

func (f *lifecycleFixture) restart(t *testing.T, db Beginner) {
	t.Helper()
	files, err := contentfiles.New(f.path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Close() })
	f.lifecycle.Drafts.Files = files
	f.lifecycle.Drafts.DB = db
	f.startLifecycle(t)
}

func (f *lifecycleFixture) command(action string, revision int64) drafts.LifecycleCommand {
	return drafts.LifecycleCommand{Command: f.meta(&revision), Action: action}
}

func (f *lifecycleFixture) http(method, path string, data []byte, id string, headers map[string]string) (int, []byte, error) {
	req, err := http.NewRequest(method, f.url+"/api/v1/workspaces/"+f.workspace+"/operations"+path, bytes.NewReader(data))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("X-HitPipe-Session", "controlled-session")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
		req.Header.Set("X-Request-ID", id)
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return resp.StatusCode, b, err
}

func (f *lifecycleFixture) checked(t *testing.T, method, path string, body []byte, id string, headers map[string]string, want int) []byte {
	t.Helper()
	status, out, err := f.http(method, path, body, id, headers)
	if err != nil || status != want {
		t.Fatalf("%s %s: status %d, want %d; %s; %v", method, path, status, want, out, err)
	}
	if want >= 400 && draftjson.Validate("Error", out) != nil {
		t.Fatalf("wrong error wire response: %s", out)
	}
	if file := os.Getenv("HIT103_CAPTURE_FILE"); file != "" && want < 400 {
		capture, err := json.Marshal(struct {
			Method, Path      string
			Status            int
			Request, Response json.RawMessage
		}{method, path, status, body, out})
		if err != nil {
			t.Fatal(err)
		}
		fd, err := os.OpenFile(file, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fd.Write(append(capture, '\n')); err != nil {
			fd.Close()
			t.Fatal(err)
		}
		if err = fd.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func (f *lifecycleFixture) change(t *testing.T, c drafts.LifecycleCommand, want int) []byte {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return f.checked(t, http.MethodPost, "/"+f.op.OperationID+"/lifecycle", raw, c.Command.RequestID, nil, want)
}

func (f *lifecycleFixture) list(t *testing.T) drafts.Operation {
	t.Helper()
	b := f.checked(t, http.MethodGet, "", nil, "", nil, 200)
	var data drafts.Operations
	if draftjson.Validate("Operations", b) != nil || json.Unmarshal(b, &data) != nil || len(data.Items) != 1 {
		t.Fatalf("invalid operations: %s", b)
	}
	return data.Items[0]
}

func (f *lifecycleFixture) snapshot(t *testing.T) drafts.ResumeSnapshot {
	t.Helper()
	b := f.checked(t, http.MethodGet, "/"+f.op.OperationID+"/resume-snapshot", nil, "", nil, 200)
	var snapshot drafts.ResumeSnapshot
	if draftjson.Validate("ResumeSnapshot", b) != nil || json.Unmarshal(b, &snapshot) != nil {
		t.Fatalf("invalid snapshot: %s", b)
	}
	return snapshot
}

func (f *lifecycleFixture) admit(t *testing.T, s admission.Signal, rev int64) (admission.Receipt, error) {
	t.Helper()
	return (Store{DB: draftTestDB{}, Content: f.files}).Accept(ctx, s, admission.EstablishedContext{WorkspaceID: f.workspace, OperationID: f.op.OperationID,
		ExpectedRevision: rev, ConnectionID: s.ConnectionID, AccountID: s.AccountID, AuthorityVerified: true})
}

func (f *lifecycleFixture) signal(t *testing.T, creation time.Time, session string) admission.Signal {
	t.Helper()
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	return admission.Signal{WorkspaceID: f.workspace, MessageID: id, ReceiptID: id, ConnectionID: config,
		Digest: "sha256:" + strings.Repeat("e", 64), AccountID: "acct_controlled", SessionID: session, SourceCreatedAt: creation, FirstReceivedAt: time.Now().UTC()}
}

func TestLifecyclePauseDrainAndResumeBothDispositions(t *testing.T) {
	for _, disposition := range []string{lifecycle.Process, lifecycle.Discard} {
		t.Run(disposition, func(t *testing.T) {
			f := newLifecycleFixture(t)
			original := f.list(t)
			accepted, err := f.admit(t, f.signal(t, *original.FirstActivationAt, "cs_before_pause"), original.Revision)
			if err != nil {
				t.Fatal(err)
			}
			c := connect(t)
			tx := begin(t, c, f.workspace)
			episode, _ := newID()
			lineage, _ := newID()
			when := time.Now().UTC().Truncate(time.Microsecond)
			if _, err = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET state='WORKING',lineage_id=$3,ever_dispatch_fenced=true,outcome_knowledge='POSSIBLE_EFFECT' WHERE workspace_id=$1 AND case_id=$2`, f.workspace, accepted.RecordID, lineage); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at) VALUES($1,$2,$3,$4,$5)`, f.workspace, episode, accepted.RecordID, when, when.Add(300*time.Second)); err != nil {
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			pause := f.command(lifecycle.Pause, original.Revision)
			pauseResult := f.change(t, pause, 202)
			draining := f.list(t)
			if draining.State != "DRAINING" || draining.DrainDeadlineAt == nil || draining.Revision != 2 {
				t.Fatalf("pause must show actual draining: %+v", draining)
			}
			var acceptedAt time.Time
			tx = begin(t, c, f.workspace)
			err = tx.QueryRow(ctx, `SELECT pause_accepted_at FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id=$2`, f.workspace, original.OperationID).Scan(&acceptedAt)
			if err != nil || !draining.DrainDeadlineAt.Equal(acceptedAt.Add(lifecycle.DrainLimit)) {
				t.Fatal("wrong durable drain boundary", acceptedAt, err)
			}
			tx.Rollback(ctx)
			held := f.signal(t, time.Now(), "cs_held")
			if _, err = f.admit(t, held, draining.Revision); !errors.Is(err, admission.ErrNotAdmissible) {
				t.Fatal("new admission during drain", err)
			}
			old := f.signal(t, original.FirstActivationAt.Add(-time.Second), "cs_old_late_payment")
			f.queue.held = []admission.Signal{held, old}
			f.change(t, f.command(lifecycle.Prepare, draining.Revision), 409)
			// New server and content instance must preserve the drain boundary.
			f.restart(t, draftTestDB{})
			reopened := f.list(t)
			if !reopened.DrainDeadlineAt.Equal(*draining.DrainDeadlineAt) || reopened.Revision != draining.Revision {
				t.Fatal("restart changed drain deadline")
			}
			if err = f.lifecycle.Drain(ctx, f.workspace); err != nil || f.list(t).State != "DRAINING" {
				t.Fatal("premature pause", err)
			}
			drain := f.lifecycle
			drain.Drafts.Clock = testClock{func() time.Time { return *draining.DrainDeadlineAt }}
			if err = drain.Drain(ctx, f.workspace); err != nil {
				t.Fatal(err)
			}
			paused := f.list(t)
			if paused.State != "PAUSED" || paused.Revision != 3 || !paused.DrainDeadlineAt.Equal(*draining.DrainDeadlineAt) {
				t.Fatalf("wrong paused state: %+v", paused)
			}
			if b := f.change(t, pause, 202); !bytes.Equal(b, pauseResult) {
				t.Fatal("replay returned current instead of original operation")
			}
			tx = begin(t, c, f.workspace)
			var state, storedLineage, result, reason string
			var fenced, finished bool
			err = tx.QueryRow(ctx, `SELECT k.state,k.lineage_id::text,k.outcome_knowledge,k.ever_dispatch_fenced,e.completed_at IS NOT NULL,h.reason_code
 FROM hp_processing.case_identity k JOIN hp_processing.episodes e USING(workspace_id,case_id)
 JOIN hp_processing.safety_history h USING(workspace_id,case_id) JOIN hp_processing.history_content d USING(workspace_id,entry_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2 AND h.reason_code='PAUSE_DRAIN_UNFINISHED'`, f.workspace, accepted.RecordID).Scan(&state, &storedLineage, &result, &fenced, &finished, &reason)
			tx.Rollback(ctx)
			if err != nil || state != "UNKNOWN" || !fenced || !finished || storedLineage != lineage || result != "POSSIBLE_EFFECT" {
				t.Fatal("unfinished outcome or fence was overwritten", state, storedLineage, result, fenced, finished, reason, err)
			}
			prep := f.command(lifecycle.Prepare, paused.Revision)
			f.change(t, prep, 202)
			p := f.snapshot(t)
			if p.Revision != 4 || p.SignalCount != 2 || p.SerializedBytes != f.queue.current.SerializedBytes || p.OldestReceivedAt == nil || len(p.Gaps) != 1 {
				t.Fatalf("snapshot mismatch: %+v", p)
			}
			cmd := f.command(lifecycle.Accept, p.Revision)
			cmd.ResumeSnapshotID, cmd.HeldDisposition = &p.SnapshotID, &disposition
			if _, err = f.lifecycle.CommittedResume(ctx, f.workspace, original.OperationID, cmd.Command.RequestID, f.queue.current.Generation); !errors.Is(err, drafts.ErrNotFound) {
				t.Fatal("uncommitted decision was readable", err)
			}
			resume := f.change(t, cmd, 202)
			current := f.list(t)
			if current.State != "ACTIVE" || current.Revision != 5 || current.DrainDeadlineAt != nil ||
				!current.FirstActivationAt.Equal(*original.FirstActivationAt) ||
				*current.AcceptedConfigDigest != *original.AcceptedConfigDigest || *current.AcceptedConfigID != *original.AcceptedConfigID {
				t.Fatalf("resume changed operation identity: %+v", current)
			}
			f.restart(t, draftTestDB{})
			decision, err := f.lifecycle.CommittedResume(ctx, f.workspace, original.OperationID, cmd.Command.RequestID, f.queue.current.Generation)
			if err != nil || decision.Disposition != disposition || decision.SignalCount != 2 ||
				decision.SerializedBytes != p.SerializedBytes || decision.ResultRevision != 5 || !decision.Scope.FirstActivationAt.Equal(*original.FirstActivationAt) {
				t.Fatalf("missing exact owner disposition %+v: %v", decision, err)
			}
			if b := f.change(t, cmd, 202); !bytes.Equal(b, resume) {
				t.Fatal("lost full resume result on replay")
			}
			released, err := f.queue.apply(decision)
			if err != nil {
				t.Fatal(err)
			}
			if again, err := f.queue.apply(decision); err != nil || len(again) != 0 {
				t.Fatal("owner applied the decision twice", again, err)
			}
			if disposition == lifecycle.Process {
				if len(released) != 2 {
					t.Fatal("owner did not release the exact generation", released)
				}
				if _, err = f.admit(t, released[1], decision.ResultRevision); !errors.Is(err, admission.ErrNotAdmissible) {
					t.Fatal("old Session admitted", err)
				}
				if _, err = f.admit(t, released[0], decision.ResultRevision); err != nil {
					t.Fatal("eligible held signal not admitted", err)
				}
				dup, err := f.admit(t, f.signal(t, *original.FirstActivationAt, "cs_before_pause"), decision.ResultRevision)
				if err != nil || dup.RecordID != accepted.RecordID {
					t.Fatal("resume generated second case", dup, err)
				}
			} else if len(released) != 0 || f.queue.discarded != 2 || decision.OldestReceivedAt == nil || len(decision.Gaps) != 1 {
				t.Fatal("discard lost the known gap basis or released signals", decision)
			}
		})
	}
}

func TestLifecycleWrongSnapshotBindingAndAuthority(t *testing.T) {
	f := newLifecycleFixture(t)
	path := "/" + f.op.OperationID + "/lifecycle"
	pause := f.command(lifecycle.Pause, 1)
	raw, _ := json.Marshal(pause)
	for _, check := range []struct {
		headers map[string]string
		status  int
	}{
		{map[string]string{"X-CSRF-Token": ""}, 400},
		{map[string]string{"X-CSRF-Token": strings.Repeat("x", 32)}, 403},
		{map[string]string{"X-HitPipe-Session": ""}, 401},
		{map[string]string{"If-Match": "2"}, 412},
		{map[string]string{"X-HitPipe-Role": "OWNER"}, 400},
	} {
		f.checked(t, http.MethodPost, path, raw, pause.Command.RequestID, check.headers, check.status)
	}
	for _, suffix := range []string{`,"Action":"PAUSE"}`, `,"action":"PAUSE"}`, `,"held_disposition":null}`, `,"unexpected":"x"}`} {
		f.checked(t, http.MethodPost, path, append(bytes.Clone(raw[:len(raw)-1]), suffix...), pause.Command.RequestID, nil, 400)
	}
	f.checked(t, http.MethodPost, path+"?x=1", raw, pause.Command.RequestID, nil, 400)
	f.change(t, pause, 202)
	if err := f.lifecycle.Drain(ctx, f.workspace); err != nil {
		t.Fatal(err)
	}
	f.queue.missing = true
	f.change(t, f.command(lifecycle.Prepare, 3), 503)
	f.queue.missing = false
	f.queue.current.Complete = false
	f.change(t, f.command(lifecycle.Prepare, 3), 503)
	f.queue.current.Complete = true
	f.change(t, f.command(lifecycle.Prepare, 3), 202)
	p := f.snapshot(t)
	held := lifecycle.Discard
	accept := f.command(lifecycle.Accept, 4)
	accept.HeldDisposition = &held
	accept.ResumeSnapshotID = &f.op.OperationID
	f.change(t, accept, 409)
	accept.ResumeSnapshotID = &p.SnapshotID
	f.queue.changed = true
	f.change(t, accept, 409)
	f.queue.changed = false
	f.bindings.listID = "modified_list"
	f.change(t, accept, 409)
	f.bindings.listID = ""
	f.gate.denyPhase = "LIFECYCLE_QUEUE_RESERVATION"
	f.change(t, accept, 403)
	f.gate.denyPhase = ""
	f.authority.role = "READER"
	f.change(t, accept, 403)
	f.authority.role = "OWNER"
	other, _ := newID()
	f.checked(t, http.MethodGet, "/"+other+"/resume-snapshot", nil, "", nil, 404)
	f.lifecycle.Drafts.Clock = testClock{func() time.Time { return p.ExpiresAt }}
	f.startLifecycle(t)
	f.change(t, accept, 409)
	if op := f.list(t); op.State != "PAUSED" || op.Revision != 4 {
		t.Fatalf("failed resume altered state: %+v", op)
	}
}

func TestLifecycleConcurrentPauseCASAndLostCommit(t *testing.T) {
	f := newLifecycleFixture(t)
	path := "/" + f.op.OperationID + "/lifecycle"
	cmds := []drafts.LifecycleCommand{f.command(lifecycle.Pause, 1), f.command(lifecycle.Pause, 1)}
	type result struct {
		status int
		body   []byte
		err    error
		cmd    drafts.LifecycleCommand
	}
	results := make(chan result, 2)
	for _, cmd := range cmds {
		go func() {
			raw, _ := json.Marshal(cmd)
			s, b, e := f.http(http.MethodPost, path, raw, cmd.Command.RequestID, nil)
			results <- result{s, b, e, cmd}
		}()
	}
	var win result
	for range cmds {
		got := <-results
		if got.err != nil || got.status != 202 && got.status != 412 {
			t.Fatalf("CAS request: %+v %s", got, got.body)
		}
		if got.status == 202 {
			if win.status != 0 {
				t.Fatal("two pause commands committed")
			}
			win = got
		}
	}
	if win.status != 202 || !bytes.Equal(win.body, f.change(t, win.cmd, 202)) {
		t.Fatal("winning command not replayed")
	}
	reason := "different body"
	win.cmd.Reason = &reason
	f.change(t, win.cmd, 409)
	if err := f.lifecycle.Drain(ctx, f.workspace); err != nil {
		t.Fatal(err)
	}
	f.change(t, f.command(lifecycle.Prepare, 3), 202)
	p := f.snapshot(t)
	cmd := f.command(lifecycle.Accept, 4)
	held := lifecycle.Process
	cmd.HeldDisposition, cmd.ResumeSnapshotID = &held, &p.SnapshotID
	f.restart(t, draftCommitLossDB{})
	f.change(t, cmd, 503)
	f.restart(t, draftTestDB{})
	decision, err := f.lifecycle.CommittedResume(ctx, f.workspace, f.op.OperationID, cmd.Command.RequestID, f.queue.current.Generation)
	if err != nil || decision.ResultRevision != 5 {
		t.Fatal("commit could not be resolved after reconnect", decision, err)
	}
	f.change(t, cmd, 202)
	if op := f.list(t); op.Revision != 5 || op.State != "ACTIVE" {
		t.Fatal(fmt.Sprintf("resume replay changed state: %+v", op))
	}
}
