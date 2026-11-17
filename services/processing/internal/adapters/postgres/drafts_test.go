package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"github.com/jackc/pgx/v5"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// All authority/binding replies here are controlled fixtures, not running IAM or
// provider-owner integrations. There is no production fallback using these types.
type draftAuthority struct {
	mu              sync.Mutex
	workspace, role string
	epoch           int64
	expiry          time.Time
	denied          bool
}

func (a *draftAuthority) Authorize(c context.Context, r drafts.Access) (drafts.Grant, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if r.Session != "controlled-session" {
		return drafts.Grant{}, drafts.ErrUnauthenticated
	}
	if a.denied || r.WorkspaceID != a.workspace {
		return drafts.Grant{}, drafts.ErrDenied
	}
	return drafts.Grant{WorkspaceID: a.workspace, PrincipalID: owner, SessionID: config, Role: a.role, Epoch: a.epoch, ExpiresAt: a.expiry, Allowed: true, CSRFVerified: r.CSRF == strings.Repeat("c", 32)}, nil
}

type draftGate struct {
	mu             sync.Mutex
	workspace      string
	epoch          int64
	denyPhase      string
	closed, noCall bool
	phases         []string
}

func (g *draftGate) Start(c context.Context, a drafts.Grant, phase string, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.phases = append(g.phases, phase)
	if g.closed || g.denyPhase == phase || a.Epoch != g.epoch || a.WorkspaceID != g.workspace || c.Err() != nil {
		return drafts.ErrDenied
	}
	if g.noCall {
		return nil
	}
	return work(c)
}

type draftBindings struct {
	priceID, listID, assigneeID, accountID, destinationID, label string
	mu                                                           sync.Mutex
	calls                                                        int
	unavailable, denied, stale                                   bool
}

func (b *draftBindings) Check(c context.Context, g drafts.Grant, q drafts.BindingQuery) (drafts.BindingProof, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls++
	if b.unavailable {
		return drafts.BindingProof{}, errors.New("controlled owner unavailable")
	}
	now := time.Now().UTC()
	if b.stale {
		now = now.Add(-time.Hour)
	}
	p := drafts.BindingProof{WorkspaceID: g.WorkspaceID, PrincipalID: g.PrincipalID, Epoch: g.Epoch, SourceConnectionID: q.SourceConnectionID, DestinationConnectionID: q.DestinationConnectionID, Mode: q.Mode, PriceAlias: q.PriceAlias, ListAlias: q.ListAlias, AssigneeAlias: q.AssigneeAlias, ServiceLabel: "Синтетическая услуга", SourceAccountID: "acct_controlled", DestinationWorkspaceID: "workspace_controlled", PriceID: "price_controlled", ListID: "list_controlled", AssigneeID: "assignee_controlled", Granted: !b.denied, CheckedAt: now, ExpiresAt: now.Add(time.Minute)}
	if b.priceID != "" {
		p.PriceID = b.priceID
	}
	if b.listID != "" {
		p.ListID = b.listID
	}
	if b.assigneeID != "" {
		p.AssigneeID = b.assigneeID
	}
	if b.accountID != "" {
		p.SourceAccountID = b.accountID
	}
	if b.destinationID != "" {
		p.DestinationWorkspaceID = b.destinationID
	}
	if b.label != "" {
		p.ServiceLabel = b.label
	}
	return p, nil
}

type draftTestDB struct{}
type draftTestTx struct {
	pgx.Tx
	conn *pgx.Conn
}

func (t *draftTestTx) Commit(c context.Context) error { defer t.conn.Close(c); return t.Tx.Commit(c) }
func (t *draftTestTx) Rollback(c context.Context) error {
	defer t.conn.Close(c)
	return t.Tx.Rollback(c)
}
func (draftTestDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	if os.Getenv("HIT78_ISOLATED_PG") != "1" {
		return nil, errors.New("owned isolated runner absent")
	}
	db, e := pgx.Connect(c, "host=/var/run/postgresql user=hp_processing_app dbname=hit78 sslmode=disable")
	if e != nil {
		return nil, e
	}
	tx, e := db.BeginTx(c, o)
	if e != nil {
		db.Close(c)
		return nil, e
	}
	return &draftTestTx{tx, db}, nil
}

type draftFixture struct {
	workspace, id, path, url string
	files                    *contentfiles.Store
	repo                     DraftStore
	service                  drafts.Service
	authority                *draftAuthority
	gate                     *draftGate
	bindings                 *draftBindings
	client                   *http.Client
	pki                      *testfixtures.DraftPKI
}

func newDraftFixture(t *testing.T) draftFixture {
	t.Helper()
	f := newDraftFixtureBase(t)
	f.start(t)
	return f
}
func newDraftFixtureBase(t *testing.T) draftFixture {
	t.Helper()
	connect(t)
	ws, _ := newID()
	id, _ := newID()
	path := filepath.Join(t.TempDir(), "draft-content")
	if e := os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	files, e := contentfiles.New(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { files.Close() })
	policy := drafts.Policy{ContentTTL: time.Hour, CheckTTL: 500 * time.Millisecond, ReplayNodes: 256}
	clock := testClock{time.Now}
	codec := draftjson.Codec{}
	repo := DraftStore{draftTestDB{}, files, codec, clock, policy}
	auth := &draftAuthority{workspace: ws, role: "OWNER", epoch: 7, expiry: time.Now().Add(time.Hour)}
	gate := &draftGate{workspace: ws, epoch: 7}
	bindings := &draftBindings{}
	service := drafts.Service{Repository: repo, Authority: auth, Boundary: gate, Bindings: bindings, Clock: clock, Codec: codec, Policy: policy}
	pki := testfixtures.NewDraftPKI(t)
	f := draftFixture{workspace: ws, id: id, path: path, files: files, repo: repo, service: service, authority: auth, gate: gate, bindings: bindings, pki: pki}
	return f
}
func (f *draftFixture) start(t *testing.T) {
	t.Helper()
	s, e := drafthttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, f.service, "bff.test", 1<<20)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", s.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go func() { s.ServeTLS(listener, "", "") }()
	t.Cleanup(func() { s.Close(); listener.Close() })
	f.url = "https://" + listener.Addr().String()
	f.client = &http.Client{Transport: f.pki.Transport(t), Timeout: 3 * time.Second}
}
func draftString(s string) *string { return &s }
func (f draftFixture) config() drafts.DraftConfig {
	keys := []string{"brief"}
	return drafts.DraftConfig{DraftID: f.id, Revision: 1, Name: "Синтетический частный черновик", SourceConnectionID: draftString(config), DestinationConnectionID: draftString(destination), Mode: draftString("TEST"), PriceAlias: draftString("Цена на русском"), ListAlias: draftString("Список примеров"), AssigneeAlias: draftString("Исполнитель стенда"), ServiceLabel: draftString("Синтетическая услуга"), TitlePattern: draftString("SERVICE_ORDER"), RequiredContextKeys: &keys}
}
func (f draftFixture) meta(revision *int64) drafts.CommandMeta {
	id, _ := newID()
	return drafts.CommandMeta{RequestID: id, ExpectedRevision: revision}
}
func (f draftFixture) pathFor(k drafts.Kind) string {
	base := "/api/v1/workspaces/" + f.workspace
	switch k {
	case drafts.Save:
		return base + "/drafts"
	case drafts.Get:
		return base + "/drafts/" + f.id
	case drafts.Check:
		return base + "/drafts/" + f.id + "/readiness"
	case drafts.Rehearse:
		return base + "/rehearsals"
	}
	return ""
}
func (f draftFixture) call(t *testing.T, k drafts.Kind, v any, headers map[string]string) (int, []byte) {
	t.Helper()
	var body []byte
	if v != nil {
		var e error
		body, e = json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
	}
	method := "POST"
	if k == drafts.Get {
		method = "GET"
	}
	r, e := http.NewRequest(method, f.url+f.pathFor(k), bytes.NewReader(body))
	if e != nil {
		t.Fatal(e)
	}
	r.Header.Set("X-HitPipe-Session", "controlled-session")
	r.Header.Set("X-CSRF-Token", strings.Repeat("c", 32))
	id, _ := newID()
	r.Header.Set("X-Request-ID", id)
	r.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		if value == "" {
			r.Header.Del(key)
		} else {
			r.Header.Set(key, value)
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
func draftCapture(t *testing.T, k drafts.Kind, status int, input any, output []byte, scenario string) {
	t.Helper()
	path := os.Getenv("HIT100_CAPTURE_FILE")
	if path == "" {
		return
	}
	var value any
	if json.Unmarshal(output, &value) != nil {
		t.Fatal("non-JSON response")
	}
	raw, e := json.Marshal(map[string]any{"scenario": scenario, "operation": k, "status": status, "request": input, "response": value, "actual_pg187_mtls_content": true})
	if e != nil {
		t.Fatal(e)
	}
	file, e := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		t.Fatal(e)
	}
	defer file.Close()
	if _, e = file.Write(append(raw, '\n')); e != nil {
		t.Fatal(e)
	}
	if e = file.Sync(); e != nil {
		t.Fatal(e)
	}
}
func (f draftFixture) save(t *testing.T) (drafts.DraftWrite, []byte) {
	t.Helper()
	input := drafts.DraftWrite{Command: f.meta(nil), Config: f.config()}
	status, body := f.call(t, drafts.Save, input, nil)
	if status != 201 {
		t.Fatalf("save%d %s", status, body)
	}
	draftCapture(t, drafts.Save, status, input, body, "complete_draft_save")
	return input, body
}
func (f draftFixture) counts(t *testing.T, wantCommands int) {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, f.workspace)
	for table, want := range map[string]int{"drafts": 1, "user_commands": wantCommands, "operations": 0, "accepted_configs": 0, "case_identity": 0, "outbox": 0} {
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table+" WHERE workspace_id=$1", f.workspace).Scan(&n); e != nil || n != want {
			t.Fatalf("%s n%d want%d %v", table, n, want, e)
		}
	}
}
func TestDraftHTTPFullRoundtripReadinessRehearsalAndRestart(t *testing.T) {
	f := newDraftFixture(t)
	input, saved := f.save(t)
	status, body := f.call(t, drafts.Get, nil, nil)
	if status != 200 || !bytes.Equal(saved, body) {
		t.Fatal("get roundtrip", status, string(body))
	}
	draftCapture(t, drafts.Get, status, nil, body, "get_saved_draft")
	revision := int64(1)
	command := f.meta(&revision)
	status, checked := f.call(t, drafts.Check, command, nil)
	var check drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(checked, &check) != nil || !check.Passed || len(check.Issues) != 0 || check.DraftRevision != 1 {
		t.Fatal("readiness", status, string(checked))
	}
	draftCapture(t, drafts.Check, status, command, checked, "current_owner_bindings_verified")
	examples := []drafts.ScenarioExample{{SyntheticSessionAlias: "example_paid", AmountMinor: 100, Paid: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_unpaid", AmountMinor: 100, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_zero", AmountMinor: 0, Paid: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_duplicate", AmountMinor: 100, Paid: true, Duplicate: true, MissingContextKeys: []string{}}, {SyntheticSessionAlias: "example_missing", AmountMinor: 100, Paid: true, MissingContextKeys: []string{"brief"}}}
	rehearsal := drafts.RehearsalRequest{ReadinessCheckID: check.CheckID, Command: f.meta(&revision), DraftID: f.id, DraftRevision: 1, Examples: examples}
	status, rehearsed := f.call(t, drafts.Rehearse, rehearsal, nil)
	var rr drafts.RehearsalResult
	if status != 200 || json.Unmarshal(rehearsed, &rr) != nil || rr.LiveRequestsMade != 0 || strings.Join(rr.Results, ",") != "WOULD_CONFIRM,WOULD_WAIT,WOULD_EXCLUDE,WOULD_HOLD_UNKNOWN,WOULD_WAIT" {
		t.Fatal("rehearsal", status, string(rehearsed))
	}
	draftCapture(t, drafts.Rehearse, status, rehearsal, rehearsed, "synthetic_all_outcomes_zero_live_calls")
	f.bindings.mu.Lock()
	calls := f.bindings.calls
	f.bindings.unavailable = true
	f.bindings.mu.Unlock()
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	f.service.Repository = DraftStore{draftTestDB{}, reopened, draftjson.Codec{}, testClock{time.Now}, f.service.Policy}
	f.start(t)
	for _, tc := range []struct {
		k      drafts.Kind
		input  any
		want   []byte
		status int
	}{{drafts.Save, input, saved, 201}, {drafts.Check, command, checked, 200}, {drafts.Rehearse, rehearsal, rehearsed, 200}} {
		s, b := f.call(t, tc.k, tc.input, nil)
		if s != tc.status || !bytes.Equal(b, tc.want) {
			t.Fatal("full response did not survive restart", tc.k, s, string(b))
		}
		draftCapture(t, tc.k, s, tc.input, b, "same_command_full_response_after_reopen")
	}
	f.bindings.mu.Lock()
	if f.bindings.calls != calls {
		t.Error("replay rechecked owner or manufactured new result")
	}
	f.bindings.mu.Unlock()
	f.counts(t, 3)
	db := connect(t)
	tx := begin(t, db, f.workspace)
	var technical string
	if e = tx.QueryRow(ctx, `SELECT technical_config::text FROM hp_processing.drafts WHERE workspace_id=$1`, f.workspace).Scan(&technical); e != nil {
		t.Fatal(e)
	}
	for _, private := range []string{input.Config.Name, *input.Config.PriceAlias, *input.Config.ListAlias, *input.Config.AssigneeAlias, *input.Config.ServiceLabel, "check_id", "rehearsal_id"} {
		if strings.Contains(technical, private) {
			t.Fatal("prose/full result leaked into technical_config")
		}
	}
}
func TestDraftHonestIncompleteAndUnavailableReadiness(t *testing.T) {
	for _, mode := range []string{"incomplete", "unavailable", "denied", "stale_proof", "no_owner_port"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			cfg := f.config()
			if mode == "incomplete" {
				cfg = drafts.DraftConfig{DraftID: f.id, Revision: 1, Name: "Неполный"}
			}
			status, b := f.call(t, drafts.Save, drafts.DraftWrite{Command: f.meta(nil), Config: cfg}, nil)
			if status != 201 {
				t.Fatal(status, string(b))
			}
			f.bindings.unavailable = mode == "unavailable"
			f.bindings.denied = mode == "denied"
			f.bindings.stale = mode == "stale_proof"
			if mode == "no_owner_port" {
				f.service.Bindings = nil
				f.start(t)
			}
			revision := int64(1)
			cmd := f.meta(&revision)
			status, b = f.call(t, drafts.Check, cmd, nil)
			var check drafts.ReadinessCheck
			if status != 200 || json.Unmarshal(b, &check) != nil || check.Passed || len(check.Issues) == 0 {
				t.Fatal("fabricated readiness", status, string(b))
			}
			draftCapture(t, drafts.Check, status, cmd, b, "honest_negative_"+mode)
			f.counts(t, 2)
		})
	}
}
