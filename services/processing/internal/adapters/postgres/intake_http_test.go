package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Controlled installation/current IAM inputs plus actual own operation read.
// This is deliberately not a production IAM/Intake implementation or fallback.
type intakeContexts struct {
	mu              sync.Mutex
	pool            Beginner
	installed       receivesignals.Identity
	operation, mode string
	epoch           int64
}

func (a *intakeContexts) Establish(c context.Context, q receivesignals.Identity) (receivesignals.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !strings.EqualFold(q.WorkspaceID, a.installed.WorkspaceID) || !strings.EqualFold(q.ConnectionID, a.installed.ConnectionID) || q.AccountID != a.installed.AccountID || q.Live != a.installed.Live || a.mode == "denied" {
		return receivesignals.Grant{}, admission.ErrNotAuthorized
	}
	tx, e := a.pool.BeginTx(c, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if e != nil {
		return receivesignals.Grant{}, e
	}
	defer tx.Rollback(c)
	if _, e = tx.Exec(c, `SELECT set_config('hp.workspace_id',$1,true)`, a.installed.WorkspaceID); e != nil {
		return receivesignals.Grant{}, e
	}
	var revision int64
	e = tx.QueryRow(c, `SELECT revision FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id=$2`, a.installed.WorkspaceID, a.operation).Scan(&revision)
	if errors.Is(e, pgx.ErrNoRows) {
		return receivesignals.Grant{}, admission.ErrNotFound
	}
	if e != nil {
		return receivesignals.Grant{}, e
	}
	grant := receivesignals.Grant{Context: admission.EstablishedContext{WorkspaceID: a.installed.WorkspaceID, ConnectionID: a.installed.ConnectionID, AccountID: a.installed.AccountID, Live: a.installed.Live, OperationID: a.operation, ExpectedRevision: revision, AuthorityVerified: true}, PrincipalID: owner, Epoch: a.epoch, ExpiresAt: time.Now().Add(time.Hour), Allowed: true}
	switch a.mode {
	case "expired":
		grant.ExpiresAt = time.Now().Add(-time.Second)
	case "revoke":
		grant.RevocationPending = true
	case "unverified":
		grant.Context.AuthorityVerified = false
	case "revision":
		grant.Context.ExpectedRevision++
	case "foreign_context":
		grant.Context.ConnectionID = owner
	case "zero_epoch":
		grant.Epoch = 0
	}
	return grant, nil
}

type intakeGate struct {
	mu    sync.Mutex
	epoch int64
	mode  string
}

func (g *intakeGate) Start(c context.Context, a receivesignals.Grant, phase string, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c.Err() != nil || a.Epoch != g.epoch || g.mode == "closed" || g.mode == phase {
		return admission.ErrNotAuthorized
	}
	if g.mode == "no_call" {
		return nil
	}
	return work(c)
}

type intakeFixture struct {
	ws, path, url string
	files         *contentfiles.Store
	repo          Store
	service       receivesignals.Service
	source        *intakeContexts
	gate          *intakeGate
	pki           *testfixtures.ObservationPKI
	client        *http.Client
	server        *http.Server
	extra         http.Handler
}

func newIntake(t *testing.T) intakeFixture {
	t.Helper()
	return intakeAt(t, workspace(t), "")
}
func intakeAt(t *testing.T, ws, path string) intakeFixture {
	t.Helper()
	if path == "" {
		path = t.TempDir() + "/content"
		if e := os.Mkdir(path, 0700); e != nil {
			t.Fatal(e)
		}
	}
	files, e := contentfiles.New(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { files.Close() })
	pool := newObservationPool(t, 4)
	source := &intakeContexts{pool: pool, installed: receivesignals.Identity{WorkspaceID: ws, ConnectionID: config, AccountID: "acct_synthetic", Live: false}, operation: op, epoch: 7}
	gate := &intakeGate{epoch: 7}
	repo := Store{DB: pool, Content: files}
	f := intakeFixture{ws: ws, path: path, files: files, repo: repo, source: source, gate: gate, pki: testfixtures.NewObservationPKI(t)}
	f.service = receivesignals.Service{Contexts: source, Boundary: gate, Clock: testClock{time.Now}, Decoder: normalizedsignal.Decoder{}, Store: repo}
	f.start(t)
	return f
}
func (f *intakeFixture) start(t *testing.T) {
	t.Helper()
	// Actual existing HIT95 server, unchanged read service, one listener. Only the
	// exact Intake route is attached; all other requests still use read handler.
	fakePrepared := preparedFixture{files: f.files}
	read := readcases.Service{Authority: readAuthority{readGrant(f.ws)}, Boundary: &readBoundary{epoch: 7}, Clock: testClock{time.Now}, Repository: readStore(t, fakePrepared, newReadPool(t, 1))}
	server, e := casereadhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, read, "bff.test")
	if e != nil {
		t.Fatal(e)
	}
	if f.extra != nil {
		previous := server.Handler
		extra := f.extra
		server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/internal/v1/delivery-observations" {
				extra.ServeHTTP(w, r)
			} else {
				previous.ServeHTTP(w, r)
			}
		})
	}
	if e = intakehttp.Attach(server, f.service, "intake.test"); e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", server.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go server.ServeTLS(l, "", "")
	t.Cleanup(func() { server.Close(); l.Close() })
	f.server = server
	f.url = "https://" + l.Addr().String()
	tr := f.pki.Transport(t)
	tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "intake.test", false, false)}
	f.client = &http.Client{Transport: tr, Timeout: 4 * time.Second}
	t.Cleanup(f.client.CloseIdleConnections)
}
func intakeWire(t *testing.T, ws string, edit func(map[string]any)) []byte {
	t.Helper()
	raw := wire(t, ws, "2026-11-14T00:00:00Z", "checkout.session.completed")
	var m map[string]any
	json.Unmarshal(raw, &m)
	delete(m, "message_digest")
	if edit != nil {
		edit(m)
	}
	raw, _ = json.Marshal(m)
	canonical, e := jcs.Transform(raw)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = intent.Digest(canonical)
	raw, _ = json.Marshal(m)
	return raw
}
func (f intakeFixture) call(t *testing.T, raw []byte, method, path string, headers http.Header) (int, []byte, error) {
	t.Helper()
	req, e := http.NewRequest(method, f.url+path, bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	id, _ := newID()
	req.Header.Set("X-Request-ID", id)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header[k] = v
	}
	res, e := f.client.Do(req)
	if e != nil {
		return 0, nil, e
	}
	defer res.Body.Close()
	body, e := io.ReadAll(res.Body)
	return res.StatusCode, body, e
}
func intakeCapture(t *testing.T, scenario string, status int, raw, body []byte) {
	t.Helper()
	path := os.Getenv("HIT104_CAPTURE_FILE")
	if path == "" {
		return
	}
	var req, res any
	json.Unmarshal(raw, &req)
	if json.Unmarshal(body, &res) != nil {
		t.Fatal(string(body))
	}
	row, _ := json.Marshal(map[string]any{"scenario": scenario, "operation": "receiveNormalizedSignal", "schema": map[bool]string{true: "Receipt", false: "Error"}[status == 202], "status": status, "request": req, "response": res, "actual_pg187_mtls": true})
	f, e := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, e = f.Write(append(row, '\n')); e != nil {
		t.Fatal(e)
	}
	if e = f.Sync(); e != nil {
		t.Fatal(e)
	}
}
func (f intakeFixture) apply(t *testing.T, raw []byte, want int, scenario string) []byte {
	t.Helper()
	status, b, e := f.call(t, raw, "POST", intakehttp.Path, nil)
	if e != nil || status != want {
		t.Fatalf("%s status=%d want=%d err=%v body=%s", scenario, status, want, e, b)
	}
	intakeCapture(t, scenario, status, raw, b)
	return b
}
func TestIntakeHTTPActivationBoundaryAndReceipt(t *testing.T) {
	for _, tc := range []struct {
		name, created, event string
		want                 int
	}{{"before", "2026-11-13T23:59:59Z", "checkout.session.completed", 409}, {"exact", "2026-11-14T00:00:00Z", "checkout.session.completed", 202}, {"after", "2026-11-14T00:00:01Z", "checkout.session.completed", 202}, {"old_session_late_payment", "2026-11-13T23:59:59Z", "checkout.session.async_payment_succeeded", 409}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newIntake(t)
			raw := intakeWire(t, f.ws, func(m map[string]any) { m["source_created_at"] = tc.created; m["event_type"] = tc.event })
			b := f.apply(t, raw, tc.want, tc.name)
			n := 0
			if tc.want == 202 {
				n = 1
				var r admission.Receipt
				if json.Unmarshal(b, &r) != nil || r.Disposition != "APPLIED" || r.RecordID == "" {
					t.Fatal(string(b))
				}
				db := connect(t)
				tx := begin(t, db, f.ws)
				var state, outcome, receiptID string
				var rev int
				if e := tx.QueryRow(ctx, `SELECT state,outcome_knowledge,revision FROM hp_processing.case_identity`).Scan(&state, &outcome, &rev); e != nil {
					t.Fatal(e)
				}
				if state != "RECEIVED" || outcome != "UNESTABLISHED" || rev != 1 {
					t.Fatal("202 pretended business success")
				}
				if e := tx.QueryRow(ctx, `SELECT proof_ref::text FROM hp_processing.safety_history`).Scan(&receiptID); e != nil {
					t.Fatal(e)
				}
				var v admission.Signal
				json.Unmarshal(raw, &v)
				if receiptID != v.ReceiptID {
					t.Fatal("Intake receipt lost")
				}
				tx.Rollback(ctx)
				ref, at := historyRef(t, f.ws, r.RecordID)
				body, e := f.files.ReadOwned(ctx, ref, preparecase.Origin{WorkspaceID: f.ws, CaseID: r.RecordID, Revision: 1}, at)
				if e != nil {
					t.Fatal(e)
				}
				var h struct {
					IntakeOrigin struct {
						Service, ReceiptID                           string
						SessionCreatedAt, FirstReceivedAt, ExpiresAt time.Time
					}
				}
				if json.Unmarshal(body, &h) != nil || h.IntakeOrigin.Service != "INTAKE" || h.IntakeOrigin.ReceiptID != v.ReceiptID || !h.IntakeOrigin.SessionCreatedAt.Equal(v.SourceCreatedAt) || !h.IntakeOrigin.FirstReceivedAt.Equal(v.FirstReceivedAt) || !h.IntakeOrigin.ExpiresAt.Equal(v.ExpiresAt) {
					t.Fatal("Intake origin changed", string(body))
				}
			}
			counts(t, f.ws, n, n, n)
		})
	}
}
func TestIntakeHTTPStrictWireAndHTTPBoundary(t *testing.T) {
	f := newIntake(t)
	good := intakeWire(t, f.ws, nil)
	cases := map[string][]byte{}
	var original map[string]json.RawMessage
	json.Unmarshal(good, &original)
	for key := range original {
		var m map[string]json.RawMessage
		json.Unmarshal(good, &m)
		delete(m, key)
		b, _ := json.Marshal(m)
		cases["missing_"+key] = b
		m[key] = json.RawMessage("null")
		b, _ = json.Marshal(m)
		cases["null_"+key] = b
	}
	for name, edit := range map[string]func(map[string]any){"uuid": func(m map[string]any) { m["message_id"] = "invalid" }, "time": func(m map[string]any) { m["source_created_at"] = "bad" }, "time_comma": func(m map[string]any) { m["source_created_at"] = "2026-11-14T00:00:00,5Z" }, "time_zone": func(m map[string]any) { m["expires_at"] = "2026-11-15T00:00:00+24:00" }, "enum": func(m map[string]any) { m["event_type"] = "payment_intent.succeeded" }, "version": func(m map[string]any) { m["schema_version"] = "2.0" }, "number": func(m map[string]any) { m["livemode"] = 0 }, "empty": func(m map[string]any) { m["event_id"] = "" }, "length": func(m map[string]any) { m["session_id"] = strings.Repeat("x", 256) }, "unknown": func(m map[string]any) { m["authority_verified"] = true }} {
		cases[name] = intakeWire(t, f.ws, edit)
	}
	for name, value := range map[string]string{"exact_duplicate": `"livemode":false,"livemode":false`, "case_first": `"LIVEMODE":true,"livemode":false`, "case_last": `"livemode":false,"LIVEMODE":true`, "escaped_duplicate": `"livemode":false,"\u006civemode":true`, "bad_number": `"livemode":1e999`, "nested": `"livemode":{"value":false}`} {
		cases[name] = []byte(strings.Replace(string(good), `"livemode":false`, value, 1))
	}
	cases["trailing"] = append(append([]byte{}, good...), []byte(` {}`)...)
	cases["array"] = append(append([]byte("["), good...), ']')
	cases["digest"] = []byte(strings.Replace(string(good), "cs_synthetic", "cs_other", 1))
	cases["surrogate"] = []byte(strings.Replace(string(good), "cs_synthetic", `\ud800`, 1))
	cases["bad_utf8"] = []byte(strings.Replace(string(good), "cs_synthetic", string([]byte{255}), 1))
	cases["empty_body"] = nil
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) { f.apply(t, raw, 400, name); counts(t, f.ws, 0, 0, 0) })
	}
	f.apply(t, bytes.Repeat([]byte(" "), int(intakehttp.MaxSignalBytes)+1), 413, "body_limit")
	counts(t, f.ws, 0, 0, 0)
	for _, tc := range []struct {
		name, method, path string
		headers            http.Header
	}{{"method", "GET", intakehttp.Path, nil}, {"query", "POST", intakehttp.Path + "?x=1", nil}, {"empty_query", "POST", intakehttp.Path + "?", nil}, {"slash", "POST", intakehttp.Path + "/", nil}, {"escaped_path", "POST", "/internal/v1/%69ntake-signals", nil}, {"request_id", "POST", intakehttp.Path, http.Header{"X-Request-Id": {}}}, {"duplicate_id", "POST", intakehttp.Path, http.Header{"X-Request-Id": {owner, config}}}, {"authority_header", "POST", intakehttp.Path, http.Header{"X-Hitpipe-Role": {"OWNER"}}}, {"content_type", "POST", intakehttp.Path, http.Header{"Content-Type": {"text/plain"}}}, {"encoding", "POST", intakehttp.Path, http.Header{"Content-Encoding": {"gzip"}}}} {
		t.Run(tc.name, func(t *testing.T) {
			code, b, e := f.call(t, good, tc.method, tc.path, tc.headers)
			if e != nil || code != 400 {
				t.Fatal(code, string(b), e)
			}
			intakeCapture(t, tc.name, code, good, b)
			counts(t, f.ws, 0, 0, 0)
		})
	}
	f.apply(t, good, 202, "canonical_false")
	padded := append(append([]byte{}, good...), bytes.Repeat([]byte(" "), int(intakehttp.MaxSignalBytes)-len(good))...)
	f.apply(t, padded, 202, "exact_256KiB_limit_canonical_false")
	counts(t, f.ws, 1, 1, 1)
}
func TestIntakeHTTPBindingAuthorityLifecycleAndTLS(t *testing.T) {
	for _, mode := range []string{"workspace", "connection", "account", "mode", "missing_operation", "revision", "paused", "draft", "denied", "expired", "revoke", "unverified", "foreign_context", "zero_epoch", "epoch", "closed", "no_call", "response_gate", "wrong_service", "expired_cert", "foreign_ca", "missing_cert", "wrong_server"} {
		t.Run(mode, func(t *testing.T) {
			f := newIntake(t)
			want := 403
			raw := intakeWire(t, f.ws, func(m map[string]any) {
				switch mode {
				case "workspace":
					m["workspace_id"] = owner
				case "connection":
					m["connection_id"] = owner
				case "account":
					m["stripe_account_id"] = "acct_foreign"
				case "mode":
					m["livemode"] = true
				}
			})
			switch mode {
			case "missing_operation":
				f.source.operation = owner
				want = 404
			case "revision":
				f.source.mode = mode
				want = 412
			case "paused", "draft":
				db := connect(t)
				tx := begin(t, db, f.ws)
				state := "PAUSED"
				if mode == "draft" {
					state = "DRAFT"
				}
				if _, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET state=$2 WHERE workspace_id=$1`, f.ws, state); e != nil {
					t.Fatal(e)
				}
				if e := tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				want = 409
			case "denied", "expired", "revoke", "unverified", "foreign_context", "zero_epoch":
				f.source.mode = mode
			case "epoch":
				f.gate.epoch++
			case "closed", "no_call":
				f.gate.mode = mode
			case "response_gate":
				f.gate.mode = "INTAKE_RECEIPT_RESPONSE_START"
			default:
				tr := f.pki.Transport(t)
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "intake.test", false, false)}
				switch mode {
				case "wrong_service":
					tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, false)}
				case "expired_cert":
					tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "intake.test", false, true)}
				case "foreign_ca":
					other := testfixtures.NewObservationPKI(t)
					tr.TLSClientConfig.Certificates = []tls.Certificate{other.Leaf(t, "intake.test", false, false)}
				case "missing_cert":
					tr.TLSClientConfig.Certificates = nil
				case "wrong_server":
					tr.TLSClientConfig.ServerName = "foreign.test"
				}
				f.client = &http.Client{Transport: tr, Timeout: time.Second}
				defer f.client.CloseIdleConnections()
			}
			if mode == "expired_cert" || mode == "foreign_ca" || mode == "missing_cert" || mode == "wrong_server" {
				_, _, e := f.call(t, raw, "POST", intakehttp.Path, nil)
				if e == nil {
					t.Fatal("invalid TLS permitted")
				}
				counts(t, f.ws, 0, 0, 0)
				return
			}
			f.apply(t, raw, want, "security_"+mode)
			n := 0
			if mode == "response_gate" {
				n = 1
				f.gate.mu.Lock()
				f.gate.mode = ""
				f.gate.mu.Unlock()
				f.apply(t, raw, 202, "response_gate_recovered_original_receipt")
			}
			counts(t, f.ws, n, n, n)
		})
	}
}
func TestIntakeHTTPReplayPauseConcurrencyIdentityAndReadRoute(t *testing.T) {
	f := newIntake(t)
	raw := intakeWire(t, f.ws, nil)
	first := f.apply(t, raw, 202, "first")
	var receipt admission.Receipt
	json.Unmarshal(first, &receipt)
	ref, _ := historyRef(t, f.ws, receipt.RecordID)
	db := connect(t)
	tx := begin(t, db, f.ws)
	if _, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET state='PAUSED',revision=2 WHERE workspace_id=$1`, f.ws); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	f.repo.DB = newObservationPool(t, 2)
	files, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer files.Close()
	f.files = files
	f.repo.Content = files
	f.service.Store = f.repo
	f.start(t)
	again := f.apply(t, raw, 202, "same_receipt_after_pause_revision_restart")
	if !bytes.Equal(first, again) {
		t.Fatal("replay changed receipt")
	}
	after, _ := historyRef(t, f.ws, receipt.RecordID)
	if ref != after {
		t.Fatal("replay extended HISTORY")
	}
	f.apply(t, intakeWire(t, f.ws, nil), 409, "new_message_paused")
	counts(t, f.ws, 1, 1, 1)
	// Same listener's original read route remains BFF-only and returns real history.
	bff := f.pki.Transport(t)
	f.client = &http.Client{Transport: bff, Timeout: time.Second}
	defer f.client.CloseIdleConnections()
	path := "/api/v1/workspaces/" + f.ws + "/cases/" + receipt.RecordID + "/history"
	code, b, e := f.call(t, nil, "GET", path, http.Header{"X-Hitpipe-Session": {"synthetic"}})
	if e != nil || code != 200 {
		t.Fatal("existing read route lost", code, string(b), e)
	}
	code, _, e = f.call(t, raw, "POST", intakehttp.Path, nil)
	if e != nil || code != 403 {
		t.Fatal("BFF admitted as Intake")
	}
	f.start(t)
	tx = begin(t, db, f.ws)
	if _, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET state='ACTIVE',revision=3 WHERE workspace_id=$1`, f.ws); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	f.apply(t, intakeWire(t, f.ws, func(m map[string]any) {
		m["source_created_at"] = "2026-11-13T23:59:59Z"
		m["event_type"] = "checkout.session.async_payment_succeeded"
	}), 409, "resume_does_not_move_first_activation")
	for _, same := range []bool{true, false} {
		t.Run(fmt.Sprintf("concurrent_same_%t", same), func(t *testing.T) {
			base := intakeWire(t, f.ws, nil)
			var wg sync.WaitGroup
			outs := make(chan []byte, 6)
			for i := 0; i < 6; i++ {
				b := base
				if !same {
					b = intakeWire(t, f.ws, nil)
				}
				wg.Add(1)
				go func(raw []byte) { defer wg.Done(); outs <- f.apply(t, raw, 202, "concurrent") }(b)
			}
			wg.Wait()
			close(outs)
			var original []byte
			for out := range outs {
				if same && original != nil && !bytes.Equal(original, out) {
					t.Fatal("non-idempotent concurrent receipt")
				}
				original = out
			}
			n := 2
			if !same {
				n = 8
			}
			counts(t, f.ws, n, 1, 1)
		})
	}
	// Same ID with changed digest cannot overwrite accepted message.
	var original map[string]any
	json.Unmarshal(raw, &original)
	conflict := intakeWire(t, f.ws, func(m map[string]any) { m["message_id"] = original["message_id"]; m["event_id"] = "evt_conflict" })
	f.apply(t, conflict, 409, "same_id_different_digest")
	counts(t, f.ws, 8, 1, 1)
	f.apply(t, intakeWire(t, f.ws, func(m map[string]any) { m["session_id"] = "cs_different" }), 202, "other_session_not_merged")
	counts(t, f.ws, 9, 2, 2)
	// Account and mode are trusted new installations, not changed solely in JSON.
	f.source.mu.Lock()
	f.source.installed.Live = true
	f.source.mu.Unlock()
	f.apply(t, intakeWire(t, f.ws, func(m map[string]any) { m["livemode"] = true }), 202, "same_session_other_trusted_mode")
	counts(t, f.ws, 10, 3, 3)
	f.source.mu.Lock()
	f.source.installed.Live = false
	f.source.installed.AccountID = "acct_other"
	f.source.mu.Unlock()
	f.apply(t, intakeWire(t, f.ws, func(m map[string]any) { m["stripe_account_id"] = "acct_other" }), 202, "same_session_other_trusted_account")
	counts(t, f.ws, 11, 4, 4)
}

func TestIntakeHTTPUUIDIdentityAndExistingDeliveryRoute(t *testing.T) {
	f := newIntake(t)
	raw := intakeWire(t, f.ws, func(m map[string]any) {
		m["workspace_id"] = strings.ToUpper(f.ws)
		m["connection_id"] = strings.ToUpper(config)
	})
	body := f.apply(t, raw, 202, "uppercase_uuid_same_identity")
	var receipt admission.Receipt
	json.Unmarshal(body, &receipt)
	ref, at := historyRef(t, f.ws, receipt.RecordID)
	if _, e := f.files.ReadOwned(ctx, ref, preparecase.Origin{WorkspaceID: f.ws, CaseID: receipt.RecordID, Revision: 1}, at); e != nil {
		t.Fatal("UUID scope/content origin split", e)
	}
	other := newObservation(t)
	handler, e := observationhttp.New(other.service, "delivery.test", 1<<16)
	if e != nil {
		t.Fatal(e)
	}
	f.extra = handler
	f.start(t)
	tr := f.pki.Transport(t)
	tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, false)}
	f.client = &http.Client{Transport: tr, Timeout: time.Second}
	defer f.client.CloseIdleConnections()
	code, b, e := f.call(t, observationWire(t, other.observation(t, "RESULT_VERIFIED")), "POST", "/internal/v1/delivery-observations", nil)
	if e != nil || code != 202 {
		t.Fatal("existing Delivery handler lost", code, string(b), e)
	}
	other.counts(t, 1)
	code, _, e = f.call(t, raw, "POST", intakehttp.Path, nil)
	if e != nil || code != 403 {
		t.Fatal("Delivery admitted as Intake")
	}
}
