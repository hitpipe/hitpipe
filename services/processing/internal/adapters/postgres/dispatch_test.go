package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test-only connection factory for the runner's owned socket. Runtime must inject
// its real bounded worker pool; this helper never accepts an external DSN.
type dispatchTestDB struct{}
type dispatchTestTx struct {
	pgx.Tx
	c *pgx.Conn
}

func (t *dispatchTestTx) Commit(ctx context.Context) error {
	defer t.c.Close(ctx)
	return t.Tx.Commit(ctx)
}
func (t *dispatchTestTx) Rollback(ctx context.Context) error {
	defer t.c.Close(ctx)
	return t.Tx.Rollback(ctx)
}
func (dispatchTestDB) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	if os.Getenv("HIT78_ISOLATED_PG") != "1" {
		return nil, errors.New("owned fixture absent")
	}
	c, e := pgx.Connect(ctx, "host=/var/run/postgresql user=hp_processing_worker dbname=hit78 sslmode=disable")
	if e != nil {
		return nil, e
	}
	tx, e := c.BeginTx(ctx, o)
	if e != nil {
		c.Close(ctx)
		return nil, e
	}
	return &dispatchTestTx{tx, c}, nil
}

type sendFixture struct {
	f        preparedFixture
	service  dispatchintent.Service
	repo     DispatchStore
	message  string
	gate     *testfixtures.Gate
	peerPath string
	pki      *testfixtures.PKI
}

func newSendFixture(t *testing.T) sendFixture {
	t.Helper()
	f := prepared(t, []string{})
	r, e := f.service.Prepare(ctx, f.request)
	if e != nil {
		t.Fatal(e)
	}
	g := &testfixtures.Gate{Workspace: f.request.WorkspaceID, Epoch: 7}
	repo := DispatchStore{dispatchTestDB{}}
	path := filepath.Join(t.TempDir(), "delivery")
	if e = os.Mkdir(path, 0700); e != nil {
		t.Fatal(e)
	}
	s := dispatchintent.Service{Repository: repo, Content: f.files, Decoder: deliveryhttp.Codec{}, Boundary: g, Clock: testfixtures.Clock{}, Policy: dispatchintent.Policy{Batch: 10, MaxAttempts: 3, Lease: 2 * time.Second, RequestTimeout: time.Second, BaseBackoff: 40 * time.Millisecond, MaxBackoff: 200 * time.Millisecond}, Jitter: dispatchintent.CryptoJitter}
	return sendFixture{f, s, repo, r.MessageID, g, path, testfixtures.NewPKI(t)}
}
func (f sendFixture) request() dispatchintent.Request {
	return dispatchintent.Request{WorkspaceID: f.f.request.WorkspaceID, Epoch: 7}
}
func captureDispatch(t *testing.T, scenario string, value any) {
	t.Helper()
	path := os.Getenv("HIT98_CAPTURE_FILE")
	if path == "" {
		return
	}
	raw, e := json.Marshal(map[string]any{"scenario": scenario, "actual_pg187_mtls_owner_content": true, "value": value})
	if e != nil {
		t.Fatal(e)
	}
	file, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
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

// Controlled Delivery is NOT another participant's implementation. It durably
// writes the received bytes before its receipt, and recovers dedup from disk.
func (f sendFixture) peer(t *testing.T, dropFirst bool, override string) (*deliveryhttp.Sender, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	var mu sync.Mutex
	peer := f.pki.Server(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		attempt := n.Add(1)
		raw, e := io.ReadAll(io.LimitReader(r.Body, 65537))
		if e != nil || len(raw) > 65536 {
			w.WriteHeader(413)
			return
		}
		var body map[string]any
		if json.Unmarshal(raw, &body) != nil || body["message_id"] != f.message || r.URL.Path != "/internal/v1/delivery-intents" || r.Header.Get("X-Request-ID") != f.message {
			w.WriteHeader(400)
			return
		}
		path := filepath.Join(f.peerPath, f.message)
		prior, e := os.ReadFile(path)
		disposition := "APPLIED"
		if e == nil {
			if !bytes.Equal(prior, raw) {
				disposition = "CONFLICT"
			} else {
				disposition = "DUPLICATE"
			}
		} else if !os.IsNotExist(e) {
			w.WriteHeader(503)
			return
		} else {
			file, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				w.WriteHeader(503)
				return
			}
			_, e = file.Write(raw)
			if e == nil {
				e = file.Sync()
			}
			file.Close()
			if e != nil {
				w.WriteHeader(503)
				return
			}
			dir, e := os.Open(f.peerPath)
			if e != nil {
				w.WriteHeader(503)
				return
			}
			e = dir.Sync()
			dir.Close()
			if e != nil {
				w.WriteHeader(503)
				return
			}
		}
		if override != "" {
			disposition = override
		}
		receipt := map[string]string{"message_id": f.message, "disposition": disposition}
		captureDispatch(t, "controlled_delivery_durable_receipt", map[string]any{"request": body, "receipt": receipt, "attempt": attempt})
		if dropFirst && attempt == 1 {
			c, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				c.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(receipt)
	}, "delivery.test")
	sender, e := deliveryhttp.New(peer.URL, f.pki.Transport(t), 4096, testfixtures.Clock{})
	if e != nil {
		t.Fatal(e)
	}
	return sender, &n
}
func (f sendFixture) outbox(t *testing.T) (string, string, time.Time, int) {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, f.request().WorkspaceID)
	var state, hash string
	var expiry time.Time
	var count int
	e := tx.QueryRow(ctx, `SELECT state,digest,expires_at,delivery_count FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2`, f.request().WorkspaceID, f.message).Scan(&state, &hash, &expiry, &count)
	if e != nil {
		t.Fatal(e)
	}
	return state, hash, expiry, count
}
func TestDispatchActualProducerMTLSReceiptAndKnowledge(t *testing.T) {
	for _, disposition := range []string{"APPLIED", "DUPLICATE", "HELD", "CONFLICT"} {
		t.Run(disposition, func(t *testing.T) {
			f := newSendFixture(t)
			f.service.Sender, _ = f.peer(t, false, disposition)
			before, hash, expiry, _ := f.outbox(t)
			results, e := f.service.RunScope(ctx, f.request())
			if e != nil || len(results) != 1 {
				t.Fatal(results, e)
			}
			want := "HELD"
			if disposition == "APPLIED" || disposition == "DUPLICATE" {
				want = "ACKED"
			}
			state, hash2, expiry2, n := f.outbox(t)
			if before != "PENDING" || state != want || hash != hash2 || !expiry.Equal(expiry2) || n != 1 {
				t.Fatal("outbox state or identity", state, n)
			}
			db := connect(t)
			tx := begin(t, db, f.request().WorkspaceID)
			var knowledge, caseState string
			if e = tx.QueryRow(ctx, `SELECT state,outcome_knowledge FROM hp_processing.case_identity WHERE workspace_id=$1 AND case_id=$2`, f.request().WorkspaceID, f.f.request.CaseID).Scan(&caseState, &knowledge); e != nil || caseState != "WORKING" || knowledge != "UNESTABLISHED" {
				t.Fatal("receipt fabricated confirmation", caseState, knowledge, e)
			}
			captureDispatch(t, "outbox_"+disposition, map[string]any{"before": before, "after": state, "message_id": f.message, "digest": hash, "expires_at": expiry, "case_state": caseState, "outcome_knowledge": knowledge})
		})
	}
}
func TestDispatchLostACKReopenAndSameBytes(t *testing.T) {
	f := newSendFixture(t)
	sender, n := f.peer(t, true, "")
	f.service.Sender = sender
	r, e := f.service.RunScope(ctx, f.request())
	if e != nil || len(r) != 1 || r[0].State != "PENDING" {
		t.Fatal(r, e)
	}
	_, hash, expiry, _ := f.outbox(t)
	reopened, e := contentfiles.New(f.f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	f.service.Content = reopened
	f.service.Repository = DispatchStore{dispatchTestDB{}}
	time.Sleep(70 * time.Millisecond)
	// New HTTPS peer/transport opens the same receiver files, recovering dedup.
	f.service.Sender, _ = f.peer(t, false, "")
	r, e = f.service.RunScope(ctx, f.request())
	if e != nil || len(r) != 1 || r[0].Disposition != "DUPLICATE" || r[0].State != "ACKED" || n.Load() != 1 {
		t.Fatal(r, e)
	}
	state, hash2, expiry2, count := f.outbox(t)
	if state != "ACKED" || hash != hash2 || !expiry.Equal(expiry2) || count != 2 {
		t.Fatal("retry changed identity/TTL")
	}
	captureDispatch(t, "lost_ack_reopen", map[string]any{"result": r, "attempts": count, "digest": hash, "expires_at": expiry})
}
func TestDispatchConcurrentClaimStaleLeaseAndWorkspace(t *testing.T) {
	f := newSendFixture(t)
	if e := f.repo.Index(ctx, f.request(), 10); e != nil {
		t.Fatal(e)
	}
	var winners atomic.Int32
	var chosen dispatchintent.Claim
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := f.repo.Claim(ctx, f.request(), f.message, 150*time.Millisecond)
			if e == nil {
				winners.Add(1)
				mu.Lock()
				chosen = c
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("concurrent claims", winners.Load())
	}
	time.Sleep(180 * time.Millisecond)
	next, e := f.repo.Claim(ctx, f.request(), f.message, time.Second)
	if e != nil || next.Token == chosen.Token {
		t.Fatal("lease not recovered", e)
	}
	if e = f.repo.Finish(ctx, chosen, "ACKED", time.Now()); !errors.Is(e, dispatchintent.ErrStale) {
		t.Fatal("stale sender acked", e)
	}
	wrong := next
	wrong.WorkspaceID = owner
	if e = f.repo.Check(ctx, wrong); e == nil {
		t.Fatal("cross-tenant claim accepted")
	}
	if e = f.repo.Finish(ctx, next, "HELD", time.Now()); e != nil {
		t.Fatal(e)
	}
	app := DispatchStore{connect(t)}
	if _, e = app.Jobs(ctx, "", 10); e == nil {
		t.Fatal("app acquired worker index")
	}
}
func TestDispatchNoSQLLockDuringActualNetwork(t *testing.T) {
	f := newSendFixture(t)
	var checked atomic.Bool
	peer := f.pki.Server(t, func(w http.ResponseWriter, r *http.Request) {
		db := connect(t)
		tx := begin(t, db, f.request().WorkspaceID)
		_, e := tx.Exec(ctx, `SELECT message_id FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2 FOR UPDATE NOWAIT`, f.request().WorkspaceID, f.message)
		if e != nil {
			t.Error("network retains SQL lock", e)
		} else {
			checked.Store(true)
		}
		tx.Rollback(ctx)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]string{"message_id": f.message, "disposition": "APPLIED"})
	}, "delivery.test")
	sender, e := deliveryhttp.New(peer.URL, f.pki.Transport(t), 4096, testfixtures.Clock{})
	if e != nil {
		t.Fatal(e)
	}
	f.service.Sender = sender
	r, e := f.service.RunScope(ctx, f.request())
	if e != nil || len(r) != 1 || r[0].State != "ACKED" || !checked.Load() {
		t.Fatal(r, e)
	}
}
func TestDispatchProcessDiesAfterClaim(t *testing.T) {
	if os.Getenv("HIT98_CLAIM_CRASH") == "1" {
		repo := DispatchStore{dispatchTestDB{}}
		r := dispatchintent.Request{WorkspaceID: os.Getenv("HIT98_WS"), Epoch: 7}
		if _, e := repo.Claim(ctx, r, os.Getenv("HIT98_MESSAGE"), 150*time.Millisecond); e != nil {
			os.Exit(23)
		}
		os.Exit(24)
	}
	f := newSendFixture(t)
	if e := f.repo.Index(ctx, f.request(), 10); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDispatchProcessDiesAfterClaim$")
	cmd.Env = append(os.Environ(), "HIT98_CLAIM_CRASH=1", "HIT98_WS="+f.request().WorkspaceID, "HIT98_MESSAGE="+f.message)
	e := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 24 {
		t.Fatal("child did not crash after real claim", e)
	}
	time.Sleep(180 * time.Millisecond)
	f.service.Sender, _ = f.peer(t, false, "")
	r, e := f.service.RunScope(ctx, f.request())
	if e != nil || len(r) != 1 || r[0].State != "ACKED" {
		t.Fatal("restart recovery", r, e)
	}
	_, _, _, n := f.outbox(t)
	if n != 2 {
		t.Fatal("claim count", n)
	}
	captureDispatch(t, "process_crash_claim_recovery", r)
}
