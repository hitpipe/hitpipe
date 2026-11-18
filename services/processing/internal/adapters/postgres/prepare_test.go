package postgres

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

//go:embed testdata/sourcefacts-valid.json
var paidFixture []byte

const destination = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

type testClock struct{ now func() time.Time }

func (c testClock) Now() time.Time { return c.now() }

// Explicitly synthetic owner grant/epoch boundary. This is not runtime IAM.
type testBoundary struct {
	mu    sync.Mutex
	open  bool
	epoch int64
}

func (g *testBoundary) Start(ctx context.Context, s preparecase.Scope, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.open || g.epoch != s.Epoch || ctx.Err() != nil {
		return preparecase.ErrHold
	}
	return work(ctx)
}
func (g *testBoundary) revoke() { g.mu.Lock(); g.epoch++; g.mu.Unlock() }

type fixtureReaders struct {
	change    func(map[string]any)
	after     func()
	origin    string
	transport *http.Transport
	boundary  preparecase.Boundary
}
type fixtureReader struct {
	snapshot evaluatepaidorder.Snapshot
	after    func()
}

func (r fixtureReader) Read(ctx context.Context, q evaluatepaidorder.Query) (evaluatepaidorder.Snapshot, error) {
	if r.after != nil {
		r.after()
	}
	return r.snapshot, ctx.Err()
}

func sourceWire(t *testing.T, c preparecase.Case, change func(map[string]any)) []byte {
	t.Helper()
	b := append([]byte{}, paidFixture...)
	var m map[string]any
	json.Unmarshal(b, &m)
	now := time.Now().UTC()
	m["case_id"] = c.CaseID
	m["connection_id"] = c.Mapping.SourceConnectionID
	m["session_id"] = c.SessionID
	m["stripe_account_id"] = c.AccountID
	m["livemode"] = c.Live
	m["session_created_at"] = c.ActivationAt.Add(time.Minute).Format(time.RFC3339Nano)
	m["oldest_observed_at"] = now.Add(-3 * time.Second).Format(time.RFC3339Nano)
	m["completed_at"] = now.Add(-2 * time.Second).Format(time.RFC3339Nano)
	m["valid_until"] = now.Add(7 * time.Second).Format(time.RFC3339Nano)
	if change != nil {
		change(m)
	}
	delete(m, "digest")
	b, _ = json.Marshal(m)
	b, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	m["digest"] = intent.Digest(b)
	b, _ = json.Marshal(m)
	return b
}
func (f fixtureReaders) ForCase(ctx context.Context, c preparecase.Case, r preparecase.Request) (evaluatepaidorder.SourceFactsReader, error) {
	if f.origin != "" {
		return (intent.Readers{Origin: f.origin, Transport: f.transport, Boundary: f.boundary, Clock: testClock{time.Now}, ReadDeadline: 2 * time.Second, MaxResponseBytes: 1 << 20}).ForCase(ctx, c, r)
	}

	// Actual SourceFacts decoding is reused even for the labelled owner fixture.
	b := append([]byte{}, paidFixture...)
	var m map[string]any
	json.Unmarshal(b, &m)
	now := time.Now().UTC()
	m["case_id"] = c.CaseID
	m["connection_id"] = c.Mapping.SourceConnectionID
	m["session_id"] = c.SessionID
	m["stripe_account_id"] = c.AccountID
	m["livemode"] = c.Live
	m["session_created_at"] = c.ActivationAt.Add(time.Minute).Format(time.RFC3339Nano)
	m["oldest_observed_at"] = now.Add(-3 * time.Second).Format(time.RFC3339Nano)
	m["completed_at"] = now.Add(-2 * time.Second).Format(time.RFC3339Nano)
	m["valid_until"] = now.Add(7 * time.Second).Format(time.RFC3339Nano)
	if f.change != nil {
		f.change(m)
	}
	delete(m, "digest")
	b, _ = json.Marshal(m)
	canonical, e := jcs.Transform(b)
	if e != nil {
		return nil, e
	}
	m["digest"] = intent.Digest(canonical)
	b, _ = json.Marshal(m)
	p, e := sourcefacts.Decode(b, sourcefacts.Expected{CaseID: c.CaseID, SessionID: c.SessionID, AccountID: c.AccountID, ConnectionID: c.Mapping.SourceConnectionID, LiveMode: c.Live}, c.Policy)
	if e != nil {
		return nil, e
	}
	v := p.Provenance
	p.Snapshot.Proof = &evaluatepaidorder.SourceProof{FactsID: v.FactsID, Digest: v.Digest, SessionCreatedAt: v.SessionCreatedAt, OldestObservedAt: v.OldestObservedAt, CompletedAt: v.CompletedAt, ValidUntil: v.ValidUntil}
	return fixtureReader{p.Snapshot, f.after}, nil
}

type preparedFixture struct {
	request preparecase.Request
	service preparecase.Service
	files   *contentfiles.Store
	path    string
	gate    *testBoundary
}

func prepared(t *testing.T, keys []string) preparedFixture {
	t.Helper()
	db := connect(t)
	ws, _ := newID()
	tx := begin(t, db, ws)
	activation := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	cfg := map[string]any{"pack_id": "PAID_ORDER_TASK_V1", "source_connection_id": config, "destination_connection_id": destination, "destination_workspace_id": "cu_synthetic", "price_id": "price_fixed", "list_id": "list_synthetic", "assignee_id": "assignee_synthetic", "mode": "TEST", "required_context_keys": keys, "title_pattern": "SERVICE_ORDER", "service_label": "Synthetic service"}
	raw, _ := json.Marshal(cfg)
	canonical, e := jcs.Transform(raw)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.operations VALUES($1,$2,$3,1,'ACTIVE',$4,NULL,NULL,NULL)`, ws, op, owner, activation)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.accepted_configs VALUES($1,$2,$3,'1.0','paid-order-v1',$4,$5,$6,$7,'synthetic')`, ws, config, op, string(raw), intent.Digest(canonical), owner, activation)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$2 WHERE workspace_id=$1`, ws, config)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "content")
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	files, e := contentfiles.New(path)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { files.Close() })
	raw = wire(t, ws, activation.Add(time.Minute).Format(time.RFC3339Nano), "checkout.session.completed")
	receipt, e := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, Store{DB: db, Content: files})
	if e != nil {
		t.Fatal(e)
	}
	gate := &testBoundary{open: true, epoch: 7}
	r := preparecase.Request{WorkspaceID: ws, CaseID: receipt.RecordID, ExpectedRevision: 1, Epoch: 7}
	return preparedFixture{r, preparecase.Service{Repository: DecisionStore{db, files}, Content: files, Readers: fixtureReaders{}, Renderer: intent.Renderer{}, Boundary: gate, Clock: testClock{time.Now}}, files, path, gate}
}
func decisionCounts(t *testing.T, ws string, wantOutbox, wantHistory, wantEpisode int) {
	t.Helper()
	c := connect(t)
	tx := begin(t, c, ws)
	for i, table := range []string{"outbox", "safety_history", "episodes"} {
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n != []int{wantOutbox, wantHistory, wantEpisode}[i] {
			t.Fatalf("%s count=%d", table, n)
		}
	}
}
func TestPreparationPaidReplayAndRestart(t *testing.T) {
	f := prepared(t, []string{})
	r, e := f.service.Prepare(ctx, f.request)
	if e != nil || r.Disposition != "PENDING_OUTBOX" || r.State != "WORKING" {
		t.Fatalf("%v %v", r, e)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
	restarted, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer restarted.Close()
	again := f.service
	again.Content = restarted
	again.Repository = DecisionStore{connect(t), restarted}
	again.Readers = fixtureReaders{after: func() { t.Error("committed replay reread source") }}
	result, e := again.Prepare(ctx, f.request)
	if e != nil || result.MessageID != r.MessageID || result.Revision != r.Revision {
		t.Fatalf("restart replay: %v %v", result, e)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
	// Expired file/read cannot manufacture another immutable permission.
	again.Clock = testClock{func() time.Time { return time.Now().Add(20 * time.Second) }}
	if result, e = again.Prepare(ctx, f.request); e == nil || result != (preparecase.Result{}) {
		t.Fatal("expired intent permitted")
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
}
func TestPreparationUnpaidThenPaidSamePins(t *testing.T) {
	f := prepared(t, []string{})
	f.service.Readers = fixtureReaders{change: func(m map[string]any) { m["payment_status"] = "unpaid" }}
	r, e := f.service.Prepare(ctx, f.request)
	if e != nil || r.State != "WAITING_DATA" || r.MessageID != "" {
		t.Fatalf("unpaid permanently excluded %v %v", r, e)
	}
	decisionCounts(t, f.request.WorkspaceID, 0, 2, 0)
	f.request.ExpectedRevision = r.Revision
	f.service.Readers = fixtureReaders{}
	r, e = f.service.Prepare(ctx, f.request)
	if e != nil || r.State != "WORKING" || r.CaseID != f.request.CaseID {
		t.Fatalf("paid signal failed %v %v", r, e)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 3, 1)
}
func TestPreparationUnknownExpiredMismatchedAndCanceled(t *testing.T) {
	for _, mode := range []string{"unknown", "expired", "mismatch", "cancel", "authority", "epoch", "pause", "stale_case", "pause_after_source", "missing_context", "revoked_after_read"} {
		t.Run(mode, func(t *testing.T) {
			keys := []string{}
			if mode == "missing_context" {
				keys = []string{"brief"}
			}
			f := prepared(t, keys)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "unknown":
				f.service.Readers = fixtureReaders{change: func(m map[string]any) { m["amount_refunded"] = nil }}
			case "expired":
				f.service.Readers = fixtureReaders{change: func(m map[string]any) {
					m["oldest_observed_at"] = time.Now().Add(-20 * time.Second).UTC().Format(time.RFC3339Nano)
					m["completed_at"] = time.Now().Add(-19 * time.Second).UTC().Format(time.RFC3339Nano)
					m["valid_until"] = time.Now().Add(-10 * time.Second).UTC().Format(time.RFC3339Nano)
				}}
			case "mismatch":
				f.service.Readers = fixtureReaders{change: func(m map[string]any) { m["case_id"] = owner }}
			case "revoked_after_read":
				f.service.Readers = fixtureReaders{after: f.gate.revoke}
			case "cancel":
				f.service.Readers = fixtureReaders{after: cancel}
			case "authority":
				f.gate.open = false
			case "epoch":
				f.request.Epoch = 8
			case "pause", "stale_case", "pause_after_source":
				mutate := func() {
					db := connect(t)
					tx := begin(t, db, f.request.WorkspaceID)
					q := `UPDATE hp_processing.operations SET state='PAUSED',revision=2 WHERE workspace_id=$1`
					if mode == "stale_case" {
						q = `UPDATE hp_processing.case_identity SET revision=2 WHERE workspace_id=$1`
					}
					if _, e := tx.Exec(context.Background(), q, f.request.WorkspaceID); e != nil {
						t.Error(e)
					}
					if e := tx.Commit(context.Background()); e != nil {
						t.Error(e)
					}
				}
				if mode == "pause" {
					mutate()
				} else {
					f.service.Readers = fixtureReaders{after: mutate}
				}
			}
			r, e := f.service.Prepare(ctx, f.request)
			if r.MessageID != "" || r.Disposition == "PENDING_OUTBOX" {
				t.Fatal("invalid guard produced intent", r, e)
			}
			if mode == "mismatch" || mode == "revoked_after_read" || mode == "authority" || mode == "epoch" || mode == "cancel" || mode == "pause" || mode == "stale_case" || mode == "pause_after_source" {
				if e == nil {
					t.Fatal("expected closed gate/stale/cancel")
				}
				decisionCounts(t, f.request.WorkspaceID, 0, 1, 0)
			} else {
				decisionCounts(t, f.request.WorkspaceID, 0, 2, 0)
			}
		})
	}
}
func TestPreparationConcurrentCASOneIntent(t *testing.T) {
	f := prepared(t, []string{})
	other := f.service
	other.Repository = DecisionStore{connect(t), f.files}
	// Both snapshots must be taken before either decides, not merely two serial calls.
	var ready sync.WaitGroup
	ready.Add(2)
	barrier := make(chan struct{})
	readers := fixtureReaders{after: func() { ready.Done(); <-barrier }}
	f.service.Readers = readers
	other.Readers = readers
	type outcome struct {
		result preparecase.Result
		err    error
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for _, service := range []preparecase.Service{f.service, other} {
		wg.Add(1)
		go func(s preparecase.Service) {
			defer wg.Done()
			r, e := s.Prepare(ctx, f.request)
			results <- outcome{r, e}
		}(service)
	}
	readyCh := make(chan struct{})
	go func() { ready.Wait(); close(readyCh) }()
	select {
	case <-readyCh:
	case <-time.After(5 * time.Second):
		close(barrier)
		t.Fatal("concurrent source snapshots did not meet")
	}
	close(barrier)
	wg.Wait()
	success := 0
	for range 2 {
		out := <-results
		r, e := out.result, out.err
		if e == nil {
			success++
			if r.MessageID == "" {
				t.Fatal("empty accepted intent")
			}
		}
	}
	if success != 1 {
		t.Fatalf("accepted %d preparations", success)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
}

// Inject actual deferred SQL failure, or a lost ACK AFTER actual COMMIT. Files
// must not be deleted on an ambiguous outcome; a retry resolves durable SQL state.
type decisionFaultDB struct {
	conn  *pgx.Conn
	ws    string
	after bool
}

func (d decisionFaultDB) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.conn.BeginTx(ctx, o)
	return decisionFaultTx{tx, d.ws, d.after}, e
}

type decisionFaultTx struct {
	pgx.Tx
	ws    string
	after bool
}

func (d decisionFaultTx) Commit(ctx context.Context) error {
	var n int
	if e := d.Tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.outbox WHERE workspace_id=$1`, d.ws).Scan(&n); e != nil {
		return e
	}
	if n == 0 {
		return d.Tx.Commit(ctx)
	}
	if !d.after {
		if _, e := d.Tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$2 WHERE workspace_id=$1`, d.ws, owner); e != nil {
			return e
		}
		return d.Tx.Commit(ctx)
	}
	if e := d.Tx.Commit(ctx); e != nil {
		return e
	}
	return errors.New("injected lost commit acknowledgment")
}
func TestPreparationCommitFailureAndLostAck(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback_at_commit", true: "lost_ack_after_commit"}[after], func(t *testing.T) {
			f := prepared(t, []string{})
			f.service.Repository = DecisionStore{decisionFaultDB{connect(t), f.request.WorkspaceID, after}, f.files}
			r, e := f.service.Prepare(ctx, f.request)
			if e == nil || r != (preparecase.Result{}) {
				t.Fatal("failed/unknown commit acknowledged", r, e)
			}
			entries, _ := os.ReadDir(f.path)
			if len(entries) < 2 {
				t.Fatal("ambiguous files were deleted")
			}
			if after {
				decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
			} else {
				decisionCounts(t, f.request.WorkspaceID, 0, 1, 0)
			}
			f.service.Repository = DecisionStore{connect(t), f.files}
			r, e = f.service.Prepare(ctx, f.request)
			if e != nil || r.MessageID == "" {
				t.Fatal("retry did not resolve", r, e)
			}
			decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
		})
	}
}
func TestPreparationActualReaderNoNetworkUnderRowLock(t *testing.T) {
	f := prepared(t, []string{})
	db := connect(t)
	c, e := (DecisionStore{db, f.files}).Load(ctx, f.request)
	if e != nil {
		t.Fatal(e)
	}
	raw := sourceWire(t, c, nil)
	var networkErr error
	var mu sync.Mutex
	calls := 0
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		conn, e := pgx.Connect(context.Background(), "host=/var/run/postgresql user=hp_processing_app dbname=hit78 sslmode=disable")
		if e != nil {
			networkErr = e
			w.WriteHeader(500)
			return
		}
		defer conn.Close(context.Background())
		tx, e := conn.Begin(context.Background())
		if e == nil {
			defer tx.Rollback(context.Background())
			_, e = tx.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,true)`, f.request.WorkspaceID)
		}
		if e == nil {
			_, e = tx.Exec(context.Background(), `SELECT case_id FROM hp_processing.case_identity WHERE workspace_id=$1 AND case_id=$2 FOR UPDATE NOWAIT`, f.request.WorkspaceID, f.request.CaseID)
		}
		if e != nil {
			networkErr = e
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAnyClientCert}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: server.TLS.Certificates, MinVersion: tls.VersionTLS13}}
	f.service.Readers = fixtureReaders{origin: server.URL, transport: transport, boundary: f.gate}
	result, e := f.service.Prepare(ctx, f.request)
	mu.Lock()
	defer mu.Unlock()
	if e != nil || result.MessageID == "" || networkErr != nil || calls != 1 {
		t.Fatalf("source read under lock/failed: %v %v calls=%d result=%v", e, networkErr, calls, result)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
}

func TestPreparationFileFaultsBeforeSQL(t *testing.T) {
	for _, step := range []string{"write", "file-fsync", "rename", "directory-fsync", "history-file"} {
		t.Run(step, func(t *testing.T) {
			f := prepared(t, []string{})
			directories := 0
			hits := 0
			f.files.Step = func(name string) error {
				if name == "directory-fsync" {
					directories++
				}
				if name == step || (step == "history-file" && directories == 2) {
					hits++
					return errors.New("injected file failure")
				}
				return nil
			}
			r, e := f.service.Prepare(ctx, f.request)
			if e == nil || r != (preparecase.Result{}) {
				t.Fatal("file failure was accepted", r, e)
			}
			if hits != 1 {
				t.Fatal("intended IO fault was not reached", hits)
			}
			decisionCounts(t, f.request.WorkspaceID, 0, 1, 0)
		})
	}
}
func TestPreparationContentAvailabilityAndGuards(t *testing.T) {
	for _, mode := range []string{"valid_context", "short_context", "missing", "corrupt", "expired", "possible_effect", "existing_task"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t, []string{"brief"})
			now := time.Now().UTC()
			payload, _ := json.Marshal("synthetic brief\nsecond line")
			expiry := now.Add(time.Hour)
			if mode == "short_context" {
				expiry = now.Add(5 * time.Second)
			}
			ref, e := f.files.Write(ctx, payload, "CASE_CONTEXT", preparecase.Origin{WorkspaceID: f.request.WorkspaceID, CaseID: f.request.CaseID, Revision: 1}, expiry, now)
			if e != nil {
				t.Fatal(e)
			}
			conn := connect(t)
			tx := begin(t, conn, f.request.WorkspaceID)
			_, e = tx.Exec(ctx, `INSERT INTO hp_processing.case_content(workspace_id,content_id,case_id,field_key,value_ref,value_sha256,serialized_value_bytes,author_user_id,added_at,expires_at) VALUES($1,$2,$3,'brief',$4,$5,$6,$7,$8,$9)`, f.request.WorkspaceID, owner, f.request.CaseID, ref.ID, ref.Digest, ref.Bytes, owner, now, ref.ExpiresAt)
			if e != nil {
				t.Fatal(e)
			}
			if mode == "possible_effect" {
				_, e = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET outcome_knowledge='POSSIBLE_EFFECT',ever_dispatch_fenced=true WHERE workspace_id=$1`, f.request.WorkspaceID)
			}
			if mode == "existing_task" {
				_, e = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET known_task_id='known_synthetic_task' WHERE workspace_id=$1`, f.request.WorkspaceID)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(ctx); e != nil {
				t.Fatal(e)
			}
			if mode == "missing" {
				os.Remove(filepath.Join(f.path, ref.ID))
			}
			if mode == "corrupt" {
				b, _ := os.ReadFile(filepath.Join(f.path, ref.ID))
				b[len(b)-1] ^= 1
				os.WriteFile(filepath.Join(f.path, ref.ID), b, 0600)
			}
			if mode == "expired" {
				f.service.Clock = testClock{func() time.Time { return now.Add(2 * time.Hour) }}
			}
			r, e := f.service.Prepare(ctx, f.request)
			if mode == "valid_context" || mode == "short_context" {
				if e != nil || r.MessageID == "" {
					t.Fatal("valid required content blocked", r, e)
				}
				decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
				if mode == "short_context" {
					db := connect(t)
					tx := begin(t, db, f.request.WorkspaceID)
					var stored time.Time
					if e := tx.QueryRow(ctx, `SELECT expires_at FROM hp_processing.outbox`).Scan(&stored); e != nil {
						t.Fatal(e)
					}
					if stored.After(ref.ExpiresAt) {
						t.Fatal("intent extended required context expiry")
					}
				}
			} else {
				if r.MessageID != "" {
					t.Fatal("unavailable content or known effect produced intent", r, e)
				}
				if mode == "possible_effect" || mode == "existing_task" {
					decisionCounts(t, f.request.WorkspaceID, 0, 1, 0)
				} else {
					decisionCounts(t, f.request.WorkspaceID, 0, 2, 0)
				}
			}
		})
	}
}

type exitAfterSQLCommit struct{ preparecase.Repository }

func (e exitAfterSQLCommit) Commit(ctx context.Context, c preparecase.Case, d preparecase.Decision, now time.Time) (preparecase.Result, error) {
	r, err := e.Repository.Commit(ctx, c, d, now)
	if err == nil && r.MessageID != "" {
		os.Exit(23)
	}
	return r, err
}
func TestPreparationProcessCrashAfterSQLCommit(t *testing.T) {
	if os.Getenv("HIT79_PREPARE_HELPER") == "1" {
		root := os.Getenv("HIT79_FILE_ROOT")
		files, e := contentfiles.New(root)
		if e != nil {
			os.Exit(41)
		}
		request := preparecase.Request{WorkspaceID: os.Getenv("HIT79_WORKSPACE"), CaseID: os.Getenv("HIT79_CASE"), ExpectedRevision: 1, Epoch: 7}
		service := preparecase.Service{Repository: exitAfterSQLCommit{DecisionStore{connect(t), files}}, Content: files, Readers: fixtureReaders{}, Renderer: intent.Renderer{}, Boundary: &testBoundary{open: true, epoch: 7}, Clock: testClock{time.Now}}
		_, _ = service.Prepare(ctx, request)
		os.Exit(42)
	}
	f := prepared(t, []string{})
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command(exe, "-test.run=^TestPreparationProcessCrashAfterSQLCommit$")
	command.Env = append(os.Environ(), "HIT79_PREPARE_HELPER=1", "HIT79_FILE_ROOT="+f.path, "HIT79_WORKSPACE="+f.request.WorkspaceID, "HIT79_CASE="+f.request.CaseID)
	output, e := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("commit/crash not reached: %v %s", e, output)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	repository := DecisionStore{connect(t), reopened}
	saved, e := repository.Load(ctx, f.request)
	if e != nil || saved.OutboxRef == nil {
		t.Fatal("durable outbox lost", e)
	}
	old, e := reopened.Read(ctx, *saved.OutboxRef, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	f.service.Repository = repository
	f.service.Content = reopened
	f.service.Readers = fixtureReaders{after: func() { t.Error("restart created a new evaluation") }}
	result, e := f.service.Prepare(ctx, f.request)
	if e != nil || result.MessageID != saved.MessageID || result.Revision != saved.Revision {
		t.Fatal("restart changed intent", result, e)
	}
	again, e := reopened.Read(ctx, *saved.OutboxRef, time.Now())
	if e != nil || string(old) != string(again) {
		t.Fatal("restart mutated payload", e)
	}
	decisionCounts(t, f.request.WorkspaceID, 1, 2, 1)
}
