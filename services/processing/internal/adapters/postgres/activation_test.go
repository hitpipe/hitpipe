package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type activationFixture struct {
	draftFixture
	act   activation.Service
	store ActivationStore
}

func newActivationFixture(t *testing.T) activationFixture {
	t.Helper()
	f := newDraftFixtureBase(t)
	f.service.Policy.CheckTTL = time.Minute
	f.repo.Policy = f.service.Policy
	f.service.Repository = f.repo
	a := activationFixture{draftFixture: f, store: ActivationStore{f.repo}}
	a.act = activation.Service{Repository: a.store, Authority: f.authority, Boundary: f.gate, Bindings: f.bindings, Clock: f.service.Clock, Codec: draftjson.ActivationCodec{}}
	a.startActivation(t)
	return a
}
func (f *activationFixture) startActivation(t *testing.T) {
	t.Helper()
	s, e := drafthttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, f.service, "bff.test", 256<<10)
	if e != nil {
		t.Fatal(e)
	}
	previous := s.Handler
	s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/retained-owner-route" {
			w.WriteHeader(204)
			return
		}
		previous.ServeHTTP(w, r)
	})
	if e = drafthttp.Attach(s, f.service, "bff.test", 256<<10); e != nil {
		t.Fatal(e)
	}
	if e = activationhttp.Attach(s, f.act, "bff.test", 256<<10); e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", s.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go s.ServeTLS(l, "", "")
	t.Cleanup(func() { s.Close(); l.Close() })
	f.url = "https://" + l.Addr().String()
	f.client = &http.Client{Transport: f.pki.Transport(t), Timeout: 4 * time.Second}
}
func (f activationFixture) ready(t *testing.T) drafts.ActivationCommand {
	t.Helper()
	f.save(t)
	rev := int64(1)
	status, b := f.call(t, drafts.Check, f.meta(&rev), nil)
	var ck drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(b, &ck) != nil || !ck.Passed {
		t.Fatal("readiness", status, string(b))
	}
	ex := []drafts.ScenarioExample{{SyntheticSessionAlias: "example_paid", AmountMinor: 100, Paid: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_unpaid", AmountMinor: 100, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_zero", Paid: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_duplicate", AmountMinor: 100, Paid: true, Duplicate: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_missing", AmountMinor: 100, Paid: true, MissingContextKeys: []string{"brief"}}}
	status, b = f.call(t, drafts.Rehearse, drafts.RehearsalRequest{ReadinessCheckID: ck.CheckID, Command: f.meta(&rev), DraftID: f.id, DraftRevision: 1, Examples: ex}, nil)
	var rr drafts.RehearsalResult
	if status != 200 || json.Unmarshal(b, &rr) != nil {
		t.Fatal("rehearsal", status, string(b))
	}
	return drafts.ActivationCommand{Command: f.meta(nil), DraftID: f.id, DraftRevision: 1, ReadinessCheckID: ck.CheckID, RehearsalID: rr.RehearsalID, AcceptedConfigDigest: ck.ActivationPreview.AcceptedConfigDigest, AcceptanceTextVersion: "managed-operation-v1", ConfirmActivate: true}
}
func activationCapture(t *testing.T, status int, in any, b []byte, scenario string) {
	t.Helper()
	path := os.Getenv("HIT102_CAPTURE_FILE")
	if path == "" {
		return
	}
	var out any
	if json.Unmarshal(b, &out) != nil {
		t.Fatal("non JSON")
	}
	raw, _ := json.Marshal(map[string]any{"scenario": scenario, "operation": "activateOperation", "status": status, "request": in, "response": out, "actual_pg187_mtls_content": true})
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
func (f activationFixture) activate(t *testing.T, in drafts.ActivationCommand, h map[string]string) (int, []byte) {
	t.Helper()
	b, _ := json.Marshal(in)
	return f.raw(t, b, in.Command.RequestID, h)
}
func (f activationFixture) raw(t *testing.T, b []byte, id string, h map[string]string) (int, []byte) {
	t.Helper()
	r, e := http.NewRequest("POST", f.url+"/api/v1/workspaces/"+f.workspace+"/operations/activate", bytes.NewReader(b))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-HitPipe-Session", "controlled-session")
	r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
	r.Header.Set("X-Request-ID", id)
	for k, v := range h {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	res, e := f.client.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	out, e := io.ReadAll(res.Body)
	if e != nil {
		t.Fatal(e)
	}
	return res.StatusCode, out
}
func (f activationFixture) assertCounts(t *testing.T, operations, commands int) {
	t.Helper()
	tx := begin(t, connect(t), f.workspace)
	for table, want := range map[string]int{"operations": operations, "accepted_configs": operations, "drafts": 1, "user_commands": commands, "case_identity": 0, "outbox": 0, "safety_history": 0} {
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table+" WHERE workspace_id=$1", f.workspace).Scan(&n); e != nil || n != want {
			t.Fatal(table, n, want, e)
		}
	}
}
func TestActivationFullRoundTripRestartReplayAndInitialBoundary(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	before := time.Now()
	status, b := f.activate(t, in, nil)
	var op drafts.Operation
	if status != 201 || json.Unmarshal(b, &op) != nil || op.State != "ACTIVE" || op.FirstActivationAt == nil || op.FirstActivationAt.Before(before.Add(-time.Second)) || op.AcceptedConfigDigest == nil || *op.AcceptedConfigDigest != in.AcceptedConfigDigest || op.Revision != 1 || op.DrainDeadlineAt != nil || op.Name != f.config().Name {
		t.Fatal(status, string(b))
	}
	activationCapture(t, status, in, b, "full_current_config_all_expected_negative_rehearsals_accepted")
	f.assertCounts(t, 1, 4)
	tx := begin(t, connect(t), f.workspace)
	var ownerID, acceptedBy, selected, digest, rehearsal, canonical string
	if e := tx.QueryRow(ctx, `SELECT o.owner_user_id::text,a.accepted_by::text,o.selected_config_id::text,a.digest,a.rehearsal_digest,a.canonical_config::text FROM hp_processing.operations o JOIN hp_processing.accepted_configs a ON(a.workspace_id,a.config_id)=(o.workspace_id,o.selected_config_id) WHERE o.workspace_id=$1`, f.workspace).Scan(&ownerID, &acceptedBy, &selected, &digest, &rehearsal, &canonical); e != nil || ownerID != owner || acceptedBy != owner || selected != *op.AcceptedConfigID || digest != in.AcceptedConfigDigest || !strings.HasPrefix(rehearsal, "sha256:") {
		t.Fatal("atomic binding", e)
	}
	if strings.Contains(canonical, f.config().Name) || strings.Contains(canonical, *f.config().PriceAlias) || strings.Contains(canonical, "rehearsal_id") {
		t.Fatal("prose in SQL")
	}
	if !strings.Contains(canonical, "price_controlled") || !strings.Contains(canonical, "PAID_ORDER_TASK_V1") {
		t.Fatal("resolved accepted config missing")
	}
	tx.Rollback(ctx)
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	f.store.Drafts.Files = reopened
	f.act.Repository = f.store
	f.bindings.unavailable = true
	f.startActivation(t)
	status, again := f.activate(t, in, nil)
	if status != 201 || !bytes.Equal(again, b) {
		t.Fatal("full restart replay", status, string(again))
	}
	activationCapture(t, status, in, again, "exact_full_result_after_files_and_connection_reopen")
	tx = begin(t, connect(t), f.workspace)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET state='PAUSED',revision=revision+1 WHERE workspace_id=$1`, f.workspace); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	status, again = f.activate(t, in, nil)
	if status != 201 || !bytes.Equal(again, b) {
		t.Fatal("technical replay changed boundary/state")
	}
	activationCapture(t, status, in, again, "historical_receipt_after_pause_does_not_resume")
	tx = begin(t, connect(t), f.workspace)
	var boundary time.Time
	var state string
	if e = tx.QueryRow(ctx, `SELECT state,first_activation_at FROM hp_processing.operations WHERE workspace_id=$1`, f.workspace).Scan(&state, &boundary); e != nil || state != "PAUSED" || !boundary.Equal(*op.FirstActivationAt) {
		t.Fatal("replay resumed or reset", e)
	}
	tx.Rollback(ctx)
	changed := in
	changed.ConfirmActivate = false
	status, _ = f.activate(t, changed, nil)
	if status != 400 {
		t.Fatal("invalid changed command accepted")
	}
	changed = in
	changed.ReadinessCheckID = config
	status, b = f.activate(t, changed, nil)
	if status != 409 {
		t.Fatal("payload conflict", status, string(b))
	}
	f.assertCounts(t, 1, 4)
}
func TestActivationCurrentEvidenceAndAuthorityFailures(t *testing.T) {
	for _, mode := range []string{"csrf_absent", "csrf_wrong", "session_absent", "session_unknown", "role", "foreign_workspace", "browser_authority", "request_id", "confirm", "acceptance_version", "digest", "draft_revision", "check_missing", "rehearsal_missing", "draft_changed", "expired", "owner_unavailable", "owner_denied", "stale_binding", "epoch_changed", "gate_read", "gate_commit", "gate_response", "gate_noninvoking"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			headers := map[string]string{}
			want := 409
			switch mode {
			case "csrf_absent":
				headers["X-CSRF-Token"] = ""
				want = 400
			case "csrf_wrong":
				headers["X-CSRF-Token"] = strings.Repeat("w", 32)
				want = 403
			case "session_absent":
				headers["X-HitPipe-Session"] = ""
				want = 401
			case "session_unknown":
				headers["X-HitPipe-Session"] = "not-stored"
				want = 401
			case "role":
				f.authority.role = "READER"
				want = 403
			case "foreign_workspace":
				f.workspace = config
				want = 403
			case "browser_authority":
				headers["X-HitPipe-Role"] = "OWNER"
				want = 400
			case "request_id":
				headers["X-Request-ID"] = config
				want = 400
			case "confirm":
				in.ConfirmActivate = false
				want = 400
			case "acceptance_version":
				in.AcceptanceTextVersion = "invented"
				want = 400
			case "digest":
				in.AcceptedConfigDigest = "sha256:" + strings.Repeat("b", 64)
				want = 409
			case "draft_revision":
				in.DraftRevision = 2
				want = 412
			case "check_missing":
				in.ReadinessCheckID = config
			case "rehearsal_missing":
				in.RehearsalID = config
			case "draft_changed":
				rev := int64(1)
				cfg := f.config()
				cfg.Name = "Changed"
				s, b := f.call(t, drafts.Save, drafts.DraftWrite{Command: f.meta(&rev), Config: cfg}, nil)
				if s != 201 {
					t.Fatal(s, string(b))
				}
				want = 412
			case "expired":
				f.store.Drafts.Clock = testClock{func() time.Time { return time.Now().Add(2 * time.Minute) }}
				f.act.Clock = f.store.Drafts.Clock
				f.act.Repository = f.store
				f.startActivation(t)
			case "owner_unavailable":
				f.bindings.unavailable = true
				want = 503
			case "owner_denied":
				f.bindings.denied = true
				want = 403
			case "stale_binding":
				f.bindings.stale = true
				want = 409
			case "epoch_changed":
				f.authority.epoch = 8
				f.gate.epoch = 8
				want = 409
			case "gate_read":
				f.gate.denyPhase = "ACTIVATION_SQL_CONTENT_READ"
				want = 403
			case "gate_commit":
				f.gate.denyPhase = "ACTIVATION_COMMAND_COMMIT"
				want = 403
			case "gate_response":
				f.gate.denyPhase = "ACTIVATION_RESPONSE_START"
				want = 403
			case "gate_noninvoking":
				f.gate.noCall = true
				want = 403
			}
			s, b := f.activate(t, in, headers)
			if s != want {
				t.Fatal(mode, s, want, string(b))
			}
			activationCapture(t, s, in, b, mode)
			if mode != "foreign_workspace" {
				ops, commands := 0, 3
				if mode == "gate_response" {
					ops, commands = 1, 4
				}
				if mode == "draft_changed" {
					commands = 4
				}
				f.assertCounts(t, ops, commands)
			}
		})
	}
}
func TestActivationStrictHTTPJSONAndBounds(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	raw, _ := json.Marshal(in)
	for _, b := range [][]byte{append(bytes.Clone(raw), []byte(` {}`)...), []byte(strings.Replace(string(raw), `"draft_revision":1`, `"draft_revision":1,"DRAFT_REVISION":2`, 1)), []byte(strings.Replace(string(raw), `"draft_revision":1`, `"DRAFT_REVISION":2,"draft_revision":1`, 1)), []byte(strings.Replace(string(raw), `"request_id":`, `"REQUEST_ID":`, 1)), []byte(strings.Replace(string(raw), `"command":{`, `"command":{"expected_revision":1,"EXPECTED_REVISION":2,`, 1)), []byte(strings.Replace(string(raw), `"command":{`, `"command":{"EXPECTED_REVISION":2,"expected_revision":1,`, 1)), []byte(strings.Replace(string(raw), `"draft_revision":1`, `"draft_revision":null`, 1))} {
		s, out := f.raw(t, b, in.Command.RequestID, nil)
		if s != 400 {
			t.Fatal("ambiguous JSON accepted", s, string(out))
		}
		activationCapture(t, s, string(b), out, "strict_JSON_rejected")
	}
	padded := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), (256<<10)-len(raw))...)
	s, out := f.raw(t, append(bytes.Clone(padded), ' '), in.Command.RequestID, nil)
	if s != 413 {
		t.Fatal("body bound", s)
	}
	activationCapture(t, s, in, out, "body_over_256KiB_rejected")
	s, out = f.raw(t, padded, in.Command.RequestID, nil)
	if s != 201 {
		t.Fatal("exact 256KiB canonical input", s, string(out))
	}
	activationCapture(t, s, in, out, "exact_256KiB_valid_input")
}
func TestActivationConcurrentCommandsAndSingleInvariant(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{false: "different_commands", true: "same_command"}[same], func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			var wg sync.WaitGroup
			statuses := make(chan int, 6)
			for i := 0; i < 6; i++ {
				cmd := in
				if !same {
					cmd.Command = f.meta(nil)
				}
				wg.Add(1)
				go func() { defer wg.Done(); s, _ := f.activate(t, cmd, nil); statuses <- s }()
			}
			wg.Wait()
			close(statuses)
			success := 0
			for s := range statuses {
				if s == 201 {
					success++
				} else if s != 409 && s != 412 {
					t.Fatal("concurrent status", s)
				}
			}
			if same && success != 6 || !same && success != 1 {
				t.Fatal("winners", success)
			}
			f.assertCounts(t, 1, 4)
		})
	}
	f := newActivationFixture(t)
	in := f.ready(t)
	s, b := f.activate(t, in, nil)
	if s != 201 {
		t.Fatal(s, string(b))
	}
	other := newActivationFixture(t)
	other.workspace = f.workspace
	other.authority.workspace = f.workspace
	other.gate.workspace = f.workspace
	other.startActivation(t)
	second := other.ready(t)
	s, b = other.activate(t, second, nil)
	if s != 409 {
		t.Fatal("second SINGLE", s, string(b))
	}
	tx := begin(t, connect(t), f.workspace)
	var n int
	tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.accepted_configs WHERE workspace_id=$1`, f.workspace).Scan(&n)
	if n != 1 {
		t.Fatal("partial accepted config")
	}
}
func TestActivationExistingOperationRevisionAndFirstBoundary(t *testing.T) {
	for _, mode := range []string{"missing_revision", "stale", "if_match", "foreign_owner", "prepared", "original_boundary"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			tx := begin(t, connect(t), f.workspace)
			own := owner
			if mode == "foreign_owner" {
				own = config
			}
			var first *time.Time
			if mode == "original_boundary" {
				v := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
				first = &v
			}
			if _, e := tx.Exec(ctx, `INSERT INTO hp_processing.operations(workspace_id,operation_id,owner_user_id,revision,state,first_activation_at) VALUES($1,$2,$3,7,'PREPARED',$4)`, f.workspace, op, own, first); e != nil {
				t.Fatal(e)
			}
			if _, e := tx.Exec(ctx, `UPDATE hp_processing.drafts SET operation_id=$2 WHERE workspace_id=$1`, f.workspace, op); e != nil {
				t.Fatal(e)
			}
			if e := tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			rev := int64(7)
			if mode != "missing_revision" {
				in.Command.ExpectedRevision = &rev
			}
			h := map[string]string{}
			want := 201
			switch mode {
			case "missing_revision":
				want = 412
			case "stale":
				rev = 6
				want = 412
			case "if_match":
				h["If-Match"] = "8"
				want = 412
			case "foreign_owner":
				want = 403
			}
			s, b := f.activate(t, in, h)
			if s != want {
				t.Fatal(s, want, string(b))
			}
			if s == 201 {
				var out drafts.Operation
				json.Unmarshal(b, &out)
				if out.OperationID != op || out.Revision != 8 || first != nil && !out.FirstActivationAt.Equal(*first) {
					t.Fatal("operation vs draft revision or initial boundary")
				}
			}
			activationCapture(t, s, in, b, "existing_operation_"+mode)
		})
	}
}

type activationAbortDB struct{ draftTestDB }
type activationAbortTx struct {
	pgx.Tx
	command bool
}

func (d activationAbortDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	t, e := d.draftTestDB.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &activationAbortTx{Tx: t}, nil
}
func (t *activationAbortTx) Exec(c context.Context, q string, a ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "INSERT INTO hp_processing.user_commands") {
		t.command = true
	}
	return t.Tx.Exec(c, q, a...)
}
func (t *activationAbortTx) Commit(c context.Context) error {
	if t.command {
		_, _ = t.Tx.Exec(c, "SELECT 1/0")
	}
	return t.Tx.Commit(c)
}
func TestActivationFileAndCommitFailureRecovery(t *testing.T) {
	for _, fault := range []string{"write", "file-fsync", "rename", "directory-fsync", "commit_abort", "commit_ack_loss"} {
		t.Run(fault, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			injected := false
			if fault == "commit_abort" {
				f.store.Drafts.DB = activationAbortDB{}
				f.act.Repository = f.store
				f.startActivation(t)
			} else if fault == "commit_ack_loss" {
				f.store.Drafts.DB = draftCommitLossDB{}
				f.act.Repository = f.store
				f.startActivation(t)
			} else {
				f.files.Step = func(step string) error {
					if step == fault {
						injected = true
						return errors.New("controlled IO failure")
					}
					return nil
				}
			}
			s, b := f.activate(t, in, nil)
			if s != 503 || !strings.HasPrefix(fault, "commit_") && !injected {
				t.Fatal("unconfirmed persistence returned success", s, injected, string(b))
			}
			activationCapture(t, s, in, b, "fault_"+fault)
			ops, commands := 0, 3
			if fault == "commit_ack_loss" {
				ops, commands = 1, 4
			}
			f.assertCounts(t, ops, commands)
			f.files.Step = nil
			f.store.Drafts.DB = draftTestDB{}
			f.act.Repository = f.store
			f.startActivation(t)
			s, b = f.activate(t, in, nil)
			if s != 201 {
				t.Fatal("recovery", s, string(b))
			}
			f.assertCounts(t, 1, 4)
			activationCapture(t, s, in, b, "recovered_"+fault)
		})
	}
}
func TestActivationProcessCrashAfterCommit(t *testing.T) {
	if os.Getenv("HIT102_CRASH") == "1" {
		files, e := contentfiles.New(os.Getenv("HIT102_CONTENT"))
		if e != nil {
			os.Exit(21)
		}
		defer files.Close()
		raw, e := os.ReadFile(os.Getenv("HIT102_REQUEST"))
		if e != nil {
			os.Exit(22)
		}
		var in drafts.ActivationCommand
		json.Unmarshal(raw, &in)
		ws := os.Getenv("HIT102_WS")
		repo := ActivationStore{DraftStore{DB: draftCommitLossDB{crash: true}, Files: files, Codec: draftjson.Codec{}, Clock: testClock{time.Now}, Policy: drafts.Policy{ContentTTL: time.Hour, CheckTTL: time.Minute, ReplayNodes: 256}}}
		svc := activation.Service{Repository: repo, Authority: &draftAuthority{workspace: ws, role: "OWNER", epoch: 7, expiry: time.Now().Add(time.Hour)}, Boundary: &draftGate{workspace: ws, epoch: 7}, Bindings: &draftBindings{}, Clock: testClock{time.Now}, Codec: draftjson.ActivationCodec{}}
		_ = svc.Execute(ctx, activation.Request{WorkspaceID: ws, Session: "controlled-session", CSRF: strings.Repeat("c", 32), RequestID: in.Command.RequestID, Body: raw}, func([]byte, int) error { return errors.New("should crash first") })
		os.Exit(23)
	}
	f := newActivationFixture(t)
	in := f.ready(t)
	raw, _ := json.Marshal(in)
	path := filepath.Join(t.TempDir(), "request.json")
	if e := os.WriteFile(path, raw, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestActivationProcessCrashAfterCommit$")
	cmd.Env = append(os.Environ(), "HIT102_CRASH=1", "HIT102_CONTENT="+f.path, "HIT102_WS="+f.workspace, "HIT102_REQUEST="+path)
	e := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 24 {
		t.Fatal("real process did not reach postcommit crash", e)
	}
	f.bindings.unavailable = true
	f.startActivation(t)
	s, b := f.activate(t, in, nil)
	if s != 201 {
		t.Fatal("postcrash durable result unavailable", s, string(b))
	}
	var result drafts.Operation
	json.Unmarshal(b, &result)
	s, again := f.activate(t, in, nil)
	if s != 201 || !bytes.Equal(b, again) {
		t.Fatal("replay changed full result")
	}
	f.assertCounts(t, 1, 4)
	activationCapture(t, s, in, again, "real_child_process_exit24_after_commit_before_response")
}
func TestActivationExpiredPurgedAndMissingContentNeverReconstructs(t *testing.T) {
	for _, mode := range []string{"expired", "purged", "missing_response", "corrupted_root"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			s, b := f.activate(t, in, nil)
			if s != 201 {
				t.Fatal(s, string(b))
			}
			tx := begin(t, connect(t), f.workspace)
			var ref, digest string
			var expiry time.Time
			if e := tx.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&ref, &digest, &expiry); e != nil {
				t.Fatal(e)
			}
			tx.Rollback(ctx)
			switch mode {
			case "expired":
				clock := testClock{func() time.Time { return time.Now().Add(2 * time.Hour) }}
				f.store.Drafts.Clock = clock
				f.act.Clock = clock
				f.authority.expiry = time.Now().Add(4 * time.Hour)
			case "purged":
				if e := f.files.PurgeExpired(time.Now().Add(2 * time.Hour)); e != nil {
					t.Fatal("purge", e)
				}
			case "corrupted_root":
				if e := os.WriteFile(filepath.Join(f.path, ref), []byte("damaged"), 0600); e != nil {
					t.Fatal(e)
				}
			case "missing_response":
				raw, e := f.files.ReadDraftRoot(ctx, ref, digest, expiry, origin(f.workspace, f.id, 1), time.Now())
				if e != nil {
					t.Fatal(e)
				}
				var root drafts.Root
				json.Unmarshal(raw, &root)
				raw, e = f.files.ReadDraft(ctx, *root.Tail, origin(f.workspace, f.id, 1), time.Now())
				if e != nil {
					t.Fatal(e)
				}
				var node drafts.CommandNode
				json.Unmarshal(raw, &node)
				if e = os.Remove(filepath.Join(f.path, node.Response.ID)); e != nil {
					t.Fatal(e)
				}
			}
			reopened, e := contentfiles.New(f.path)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			f.store.Drafts.Files = reopened
			f.act.Repository = f.store
			f.startActivation(t)
			s, b = f.activate(t, in, nil)
			if s != 503 {
				t.Fatal("metadata-only replay fabricated", mode, s, string(b))
			}
			f.assertCounts(t, 1, 4)
			activationCapture(t, s, in, b, "content_after_reopen_"+mode)
		})
	}
}
func TestActivationTLSCallerAndRouteBoundaries(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	raw, _ := json.Marshal(in)
	for _, mode := range []string{"intake", "delivery", "expired", "no_cert", "wrong_ca", "wrong_server"} {
		t.Run(mode, func(t *testing.T) {
			tr := f.pki.Transport(t)
			wantError := false
			switch mode {
			case "intake", "delivery":
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, mode+".test", false, false)}
			case "expired":
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "bff.test", false, true)}
				wantError = true
			case "no_cert":
				tr.TLSClientConfig.Certificates = nil
				wantError = true
			case "wrong_ca":
				other := newActivationFixture(t)
				tr.TLSClientConfig.Certificates = []tls.Certificate{other.pki.Leaf(t, "bff.test", false, false)}
				wantError = true
			case "wrong_server":
				tr.TLSClientConfig.ServerName = "not-processing.test"
				wantError = true
			}
			client := &http.Client{Transport: tr, Timeout: time.Second}
			defer tr.CloseIdleConnections()
			r, _ := http.NewRequest("POST", f.url+"/api/v1/workspaces/"+f.workspace+"/operations/activate", bytes.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("X-HitPipe-Session", "controlled-session")
			r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
			r.Header.Set("X-Request-ID", in.Command.RequestID)
			res, e := client.Do(r)
			if wantError {
				if e == nil {
					res.Body.Close()
					t.Fatal("untrusted TLS accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer res.Body.Close()
			if res.StatusCode != 403 {
				t.Fatal("wrong service caller", res.StatusCode)
			}
		})
	}
	f.assertCounts(t, 0, 3)
	for _, mode := range []string{"get", "query", "escaped_path", "wrong_path"} {
		r, _ := http.NewRequest("POST", f.url+"/api/v1/workspaces/"+f.workspace+"/operations/activate", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-HitPipe-Session", "controlled-session")
		r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
		r.Header.Set("X-Request-ID", in.Command.RequestID)
		switch mode {
		case "get":
			r.Method = "GET"
		case "query":
			r.URL.RawQuery = "activate=true"
		case "escaped_path":
			r.URL.RawPath = strings.Replace(r.URL.Path, "activate", "%61ctivate", 1)
		case "wrong_path":
			r.URL.Path = "/api/v1/operations/activate"
		}
		res, e := f.client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		res.Body.Close()
		if res.StatusCode != 400 {
			t.Fatal(mode, res.StatusCode)
		}
	}
	// The draft handler remains attached to this exact listener, not replaced.
	status, b := f.call(t, drafts.Get, nil, nil)
	if status != 200 {
		t.Fatal("existing handler lost", status, string(b))
	}
}

// Mutates only this test's own durable files, rebuilding their hashes/SQL pointer
// to test semantic validation independently from byte-corruption detection.
func (f activationFixture) rewriteRehearsal(t *testing.T, change func(*drafts.RehearsalResult, *drafts.CommandNode)) {
	t.Helper()
	tx := begin(t, connect(t), f.workspace)
	var id, digest string
	var expiry time.Time
	if e := tx.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&id, &digest, &expiry); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	raw, e := f.files.ReadDraftRoot(ctx, id, digest, expiry, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var root drafts.Root
	json.Unmarshal(raw, &root)
	raw, e = f.files.ReadDraft(ctx, *root.Tail, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var node drafts.CommandNode
	json.Unmarshal(raw, &node)
	if node.Kind != drafts.Rehearse {
		t.Fatal("not rehearsal tail")
	}
	raw, e = f.files.ReadDraft(ctx, node.Response, origin(f.workspace, f.id, 1), time.Now())
	if e != nil {
		t.Fatal(e)
	}
	var rr drafts.RehearsalResult
	json.Unmarshal(raw, &rr)
	change(&rr, &node)
	raw, e = f.service.Codec.Encode(drafts.Rehearse, rr)
	if e != nil {
		t.Fatal(e)
	}
	node.Response, e = f.repo.writeFile(ctx, raw, f.workspace, f.id, 1, expiry, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	raw, _ = json.Marshal(node)
	tail, e := f.repo.writeFile(ctx, raw, f.workspace, f.id, 1, expiry, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	root.Tail = &tail
	raw, _ = json.Marshal(root)
	ref, e := f.repo.writeFile(ctx, raw, f.workspace, f.id, 1, expiry, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	tx = begin(t, connect(t), f.workspace)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.drafts SET prose_ref=$2,prose_sha256=$3 WHERE workspace_id=$1`, f.workspace, ref.ID, ref.Digest); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
}

type changedActivationBindings struct {
	base  *draftBindings
	field string
}

func (b changedActivationBindings) Check(c context.Context, g drafts.Grant, q drafts.BindingQuery) (drafts.BindingProof, error) {
	p, e := b.base.Check(c, g, q)
	switch b.field {
	case "price":
		p.PriceID = "price_changed"
	case "list":
		p.ListID = "list_changed"
	case "assignee":
		p.AssigneeID = "assignee_changed"
	case "destination":
		p.DestinationWorkspaceID = "other_workspace"
	case "account":
		p.SourceAccountID = "acct_other"
	case "connection":
		p.SourceConnectionID = config
	case "mode":
		p.Mode = "LIVE"
	case "workspace":
		p.WorkspaceID = config
	case "principal":
		p.PrincipalID = config
	case "epoch":
		p.Epoch++
	case "expired":
		p.ExpiresAt = time.Now().Add(-time.Second)
	}
	return p, e
}
func TestActivationNoStaleOrFabricatedPreparation(t *testing.T) {
	for _, mode := range []string{"incomplete", "failed_check", "foreign_check", "foreign_rehearsal", "changed_result", "missing_original_examples", "changed_original_examples", "missing_preparation_response"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			var in drafts.ActivationCommand
			want := 409
			if mode == "incomplete" || mode == "failed_check" {
				cfg := f.config()
				if mode == "incomplete" {
					cfg.ServiceLabel = nil
				}
				s, b := f.call(t, drafts.Save, drafts.DraftWrite{Command: f.meta(nil), Config: cfg}, nil)
				if s != 201 {
					t.Fatal(s, string(b))
				}
				f.bindings.denied = mode == "failed_check"
				rev := int64(1)
				s, b = f.call(t, drafts.Check, f.meta(&rev), nil)
				var ck drafts.ReadinessCheck
				if s != 200 || json.Unmarshal(b, &ck) != nil || ck.Passed {
					t.Fatal("negative check expected", s, string(b))
				}
				f.bindings.denied = false
				s, b = f.call(t, drafts.Rehearse, drafts.RehearsalRequest{ReadinessCheckID: ck.CheckID, Command: f.meta(&rev), DraftID: f.id, DraftRevision: 1, Examples: []drafts.ScenarioExample{{SyntheticSessionAlias: "example_unpaid", MissingContextKeys: []string{}}}}, nil)
				if s != 409 {
					t.Fatal("failed readiness rehearsed", s, string(b))
				}
				rr := drafts.RehearsalResult{RehearsalID: config}
				in = drafts.ActivationCommand{Command: f.meta(nil), DraftID: f.id, DraftRevision: 1, ReadinessCheckID: ck.CheckID, RehearsalID: rr.RehearsalID, AcceptedConfigDigest: ck.ConfigDigest, AcceptanceTextVersion: "managed-operation-v1", ConfirmActivate: true}
			} else {
				in = f.ready(t)
				switch mode {
				case "foreign_check", "foreign_rehearsal":
					other := newActivationFixture(t)
					oi := other.ready(t)
					if mode == "foreign_check" {
						in.ReadinessCheckID = oi.ReadinessCheckID
					} else {
						in.RehearsalID = oi.RehearsalID
					}
				case "changed_result":
					f.rewriteRehearsal(t, func(rr *drafts.RehearsalResult, _ *drafts.CommandNode) { rr.Results[0] = "WOULD_WAIT" })
				case "missing_original_examples":
					f.rewriteRehearsal(t, func(_ *drafts.RehearsalResult, n *drafts.CommandNode) { n.Examples = nil })
					want = 503
				case "changed_original_examples":
					f.rewriteRehearsal(t, func(_ *drafts.RehearsalResult, n *drafts.CommandNode) { n.Examples[0].Paid = false })
					want = 503
				case "missing_preparation_response":
					tx := begin(t, connect(t), f.workspace)
					var id, digest string
					var expiry time.Time
					tx.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&id, &digest, &expiry)
					tx.Rollback(ctx)
					raw, e := f.files.ReadDraftRoot(ctx, id, digest, expiry, origin(f.workspace, f.id, 1), time.Now())
					if e != nil {
						t.Fatal(e)
					}
					var root drafts.Root
					json.Unmarshal(raw, &root)
					raw, e = f.files.ReadDraft(ctx, *root.Tail, origin(f.workspace, f.id, 1), time.Now())
					if e != nil {
						t.Fatal(e)
					}
					var node drafts.CommandNode
					json.Unmarshal(raw, &node)
					if e = os.Remove(filepath.Join(f.path, node.Response.ID)); e != nil {
						t.Fatal(e)
					}
					want = 503
				}
			}
			s, b := f.activate(t, in, nil)
			if s != want {
				t.Fatal(mode, s, want, string(b))
			}
			f.assertCounts(t, 0, 3)
			activationCapture(t, s, in, b, "preparation_"+mode)
		})
	}
	for _, field := range []string{"price", "list", "assignee", "destination", "account", "mode", "workspace", "principal", "epoch", "expired"} {
		t.Run("binding_"+field, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			f.act.Bindings = changedActivationBindings{f.bindings, field}
			f.startActivation(t)
			s, b := f.activate(t, in, nil)
			want := 409
			if field == "workspace" || field == "principal" {
				want = 403
			}
			if s != want {
				t.Fatal("changed substantial binding accepted", field, s, want, string(b))
			}
			f.assertCounts(t, 0, 3)
			activationCapture(t, s, in, b, "changed_binding_"+field)
		})
	}
}
func TestActivationMissingPortsDenyAndForeignRLS(t *testing.T) {
	f := newActivationFixture(t)
	in := f.ready(t)
	for _, missing := range []string{"repository", "authority", "boundary", "bindings", "clock", "codec", "typed_nil"} {
		s := f.act
		switch missing {
		case "repository":
			s.Repository = nil
		case "authority":
			s.Authority = nil
		case "boundary":
			s.Boundary = nil
		case "bindings":
			s.Bindings = nil
		case "clock":
			s.Clock = nil
		case "codec":
			s.Codec = nil
		case "typed_nil":
			var b *draftBindings
			s.Bindings = b
		}
		if s.Ready() {
			t.Fatal("missing substantial port accepted", missing)
		}
	}
	tx := begin(t, connect(t), config)
	var n int
	if e := tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.drafts WHERE draft_id=$1`, f.id).Scan(&n); e != nil || n != 0 {
		t.Fatal("cross workspace RLS", n, e)
	}
	tx.Rollback(ctx)
	for _, mode := range []string{"absent_grant", "expired_grant", "closed_gate", "stale_gate_epoch"} {
		t.Run(mode, func(t *testing.T) {
			f := newActivationFixture(t)
			in := f.ready(t)
			switch mode {
			case "absent_grant":
				f.authority.denied = true
			case "expired_grant":
				f.authority.expiry = time.Now().Add(-time.Second)
			case "closed_gate":
				f.gate.closed = true
			case "stale_gate_epoch":
				f.gate.epoch++
			}
			s, b := f.activate(t, in, nil)
			if s != 403 {
				t.Fatal(mode, s, string(b))
			}
			f.assertCounts(t, 0, 3)
			activationCapture(t, s, in, b, mode)
		})
	}
	_ = in
}

func TestActivationAttachRetainsPrivateServerAndRejectsUnsafeSetup(t *testing.T) {
	f := newActivationFixture(t)
	res, e := f.client.Get(f.url + "/retained-owner-route")
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 204 {
		t.Fatal("previous owner handler replaced")
	}
	for _, mode := range []string{"public", "port", "no_tls", "no_ca", "no_certificate", "no_client_auth", "tls12", "unbounded_tls", "insecure", "dynamic_tls", "missing_service", "bad_size"} {
		t.Run(mode, func(t *testing.T) {
			s, e := drafthttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, f.service, "bff.test", 256<<10)
			if e != nil {
				t.Fatal(e)
			}
			service := f.act
			size := int64(256 << 10)
			switch mode {
			case "public":
				s.Addr = "0.0.0.0:8443"
			case "port":
				s.Addr = "127.0.0.1:8080"
			case "no_tls":
				s.TLSConfig = nil
			case "no_ca":
				s.TLSConfig.ClientCAs = nil
			case "no_certificate":
				s.TLSConfig.Certificates = nil
			case "no_client_auth":
				s.TLSConfig.ClientAuth = tls.NoClientCert
			case "tls12":
				s.TLSConfig.MinVersion = tls.VersionTLS12
			case "unbounded_tls":
				s.TLSConfig.MaxVersion = 0
			case "insecure":
				s.TLSConfig.InsecureSkipVerify = true
			case "dynamic_tls":
				s.TLSConfig.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) { return nil, nil }
			case "missing_service":
				service.Bindings = nil
			case "bad_size":
				size++
			}
			if activationhttp.Attach(s, service, "bff.test", size) == nil {
				t.Fatal("unsafe attachment accepted", mode)
			}
		})
	}
}
