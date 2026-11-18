package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDispatchHoldsWithoutNetworkStart(t *testing.T) {
	for _, mode := range []string{"closed_gate", "missing_gate", "no_gate_call", "stale_epoch", "missing_content", "corrupt_content", "expiry", "wrong_content_hash", "attempt_bound"} {
		t.Run(mode, func(t *testing.T) {
			f := newSendFixture(t)
			var calls atomic.Int32
			peer := f.pki.Server(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(202) }, "delivery.test")
			sender, e := deliveryhttp.New(peer.URL, f.pki.Transport(t), 4096, testfixtures.Clock{})
			if e != nil {
				t.Fatal(e)
			}
			f.service.Sender = sender
			switch mode {
			case "closed_gate":
				f.gate.Close()
			case "missing_gate":
				f.service.Boundary = nil
			case "no_gate_call":
				f.gate.NoInvoke = true
			case "stale_epoch":
				f.gate.Epoch++
			case "expiry":
				f.service.Clock = testClock{func() time.Time { return time.Now().Add(time.Minute) }}
			case "attempt_bound":
				f.service.Policy.MaxAttempts = 1
				db := connect(t)
				tx := begin(t, db, f.request().WorkspaceID)
				_, e = tx.Exec(ctx, `UPDATE hp_processing.outbox SET delivery_count=1 WHERE workspace_id=$1`, f.request().WorkspaceID)
				if e != nil {
					t.Fatal(e)
				}
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
			case "missing_content", "corrupt_content", "wrong_content_hash":
				db := connect(t)
				tx := begin(t, db, f.request().WorkspaceID)
				var ref string
				if e = tx.QueryRow(ctx, `SELECT payload_ref::text FROM hp_processing.outbox WHERE workspace_id=$1`, f.request().WorkspaceID).Scan(&ref); e != nil {
					t.Fatal(e)
				}
				tx.Rollback(ctx)
				path := filepath.Join(f.f.path, ref)
				if mode == "missing_content" {
					e = os.Remove(path)
				} else if mode == "corrupt_content" {
					e = os.WriteFile(path, []byte("corrupt"), 0600)
				} else {
					tx = begin(t, db, f.request().WorkspaceID)
					_, e = tx.Exec(ctx, `UPDATE hp_processing.outbox SET payload_sha256=$2 WHERE workspace_id=$1`, f.request().WorkspaceID, "sha256:"+strings.Repeat("f", 64))
					if e == nil {
						e = tx.Commit(ctx)
					}
				}
				if e != nil {
					t.Fatal(e)
				}
			}
			results, e := f.service.RunScope(ctx, f.request())
			if calls.Load() != 0 {
				t.Fatal("unsafe network start", mode)
			}
			if e == nil && (len(results) != 1 || results[0].State != "HELD") {
				t.Fatal("reported false success", results)
			}
		})
	}
}
func TestDispatchRevocationBetweenConnectAndTLSWrite(t *testing.T) {
	f := newSendFixture(t)
	var requests atomic.Int32
	peer := f.pki.Server(t, func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(202) }, "delivery.test")
	tr := f.pki.Transport(t)
	var replaced atomic.Bool
	tr.DialContext = func(c context.Context, network, address string) (net.Conn, error) {
		conn, e := (&net.Dialer{}).DialContext(c, network, address)
		if e != nil {
			return nil, e
		}
		if replaced.CompareAndSwap(false, true) {
			tx, e := f.repo.tx(c, f.request().WorkspaceID)
			if e != nil {
				conn.Close()
				return nil, e
			}
			token, _ := newID()
			_, e = tx.Exec(c, `UPDATE hp_processing.outbox SET claim_token=$3 WHERE workspace_id=$1 AND message_id=$2`, f.request().WorkspaceID, f.message, token)
			if e == nil {
				e = tx.Commit(c)
			} else {
				tx.Rollback(c)
			}
			if e != nil {
				conn.Close()
				return nil, e
			}
		}
		return conn, nil
	}
	sender, e := deliveryhttp.New(peer.URL, tr, 4096, testfixtures.Clock{})
	if e != nil {
		t.Fatal(e)
	}
	f.service.Sender = sender
	_, e = f.service.RunScope(ctx, f.request())
	if e == nil || requests.Load() != 0 || !replaced.Load() {
		t.Fatal("stale socket started HTTP", e, requests.Load())
	}
}
func TestDispatchRetryAfterAndHardExpiry(t *testing.T) {
	for _, mode := range []string{"short_retry", "beyond_facts", "bad_receipt", "conflict_status"} {
		t.Run(mode, func(t *testing.T) {
			f := newSendFixture(t)
			var calls atomic.Int32
			peer := f.pki.Server(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				switch mode {
				case "conflict_status":
					w.WriteHeader(409)
				case "bad_receipt":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(202)
					json.NewEncoder(w).Encode(map[string]string{"message_id": owner, "disposition": "APPLIED"})
				default:
					seconds := "1"
					if mode == "beyond_facts" {
						seconds = "30"
					}
					w.Header().Set("Retry-After", seconds)
					w.WriteHeader(429)
				}
			}, "delivery.test")
			sender, e := deliveryhttp.New(peer.URL, f.pki.Transport(t), 4096, testfixtures.Clock{})
			if e != nil {
				t.Fatal(e)
			}
			f.service.Sender = sender
			before, hash, expiry, _ := f.outbox(t)
			r, e := f.service.RunScope(ctx, f.request())
			if e != nil || len(r) != 1 {
				t.Fatal(r, e)
			}
			state, hash2, expiry2, count := f.outbox(t)
			want := "PENDING"
			if mode == "beyond_facts" || mode == "conflict_status" {
				want = "HELD"
			}
			if before != "PENDING" || state != want || hash != hash2 || !expiry.Equal(expiry2) || count != 1 {
				t.Fatal("retry/expiry semantics", r)
			}
			if mode == "short_retry" {
				tx, e := f.repo.tx(ctx, f.request().WorkspaceID)
				if e != nil {
					t.Fatal(e)
				}
				var at, now time.Time
				if e = tx.QueryRow(ctx, `SELECT next_attempt_at,clock_timestamp() FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2`, f.request().WorkspaceID, f.message).Scan(&at, &now); e != nil {
					t.Fatal(e)
				}
				tx.Rollback(ctx)
				if at.Sub(now) < 800*time.Millisecond {
					t.Fatal("Retry-After ignored")
				}
				again, e := f.service.RunScope(ctx, f.request())
				if e != nil || len(again) != 0 || calls.Load() != 1 {
					t.Fatal("early retry", again, e)
				}
			}
		})
	}
}

type lostAckDB struct{ dispatchTestDB }
type lostAckTx struct {
	pgx.Tx
	finish bool
}

func (d lostAckDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.dispatchTestDB.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &lostAckTx{Tx: tx}, nil
}
func (t *lostAckTx) Exec(c context.Context, q string, a ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "SET state=$3") && len(a) > 2 && a[2] == "ACKED" {
		t.finish = true
	}
	return t.Tx.Exec(c, q, a...)
}
func (t *lostAckTx) Commit(c context.Context) error {
	e := t.Tx.Commit(c)
	if e == nil && t.finish {
		return errors.New("controlled SQL commit acknowledgement loss")
	}
	return e
}
func TestDispatchSQLCommitAckLossAndPoolScopeReset(t *testing.T) {
	f := newSendFixture(t)
	f.service.Sender, _ = f.peer(t, false, "")
	f.service.Repository = DispatchStore{lostAckDB{}}
	if _, e := f.service.RunScope(ctx, f.request()); e == nil {
		t.Fatal("unknown commit reported success")
	}
	state, _, _, n := f.outbox(t)
	if state != "ACKED" || n != 1 {
		t.Fatal("actual commit missing", state, n)
	}
	f.service.Repository = f.repo
	r, e := f.service.RunScope(ctx, f.request())
	if e != nil || len(r) != 0 {
		t.Fatal("committed outbox resent", r, e)
	}
	// Reuse ONE real connection across scopes, then global metadata discovery.
	c, e := pgx.Connect(ctx, "host=/var/run/postgresql user=hp_processing_worker dbname=hit78 sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close(ctx)
	repo := DispatchStore{c}
	for _, ws := range []string{f.request().WorkspaceID, owner} {
		if e = repo.Index(ctx, dispatchintent.Request{WorkspaceID: ws, Epoch: 7}, 10); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = repo.Jobs(ctx, "", 10); e != nil {
		t.Fatal(e)
	}
	var scope *string
	if e = c.QueryRow(ctx, `SELECT NULLIF(current_setting('hp.workspace_id',true),'')`).Scan(&scope); e != nil || scope != nil {
		t.Fatal("scope leaked after pool reuse", scope, e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = repo.Jobs(canceled, "", 10); e == nil {
		t.Fatal("cancelled database read")
	}
	c.Close(ctx)
	if _, e = repo.Jobs(ctx, "", 10); e == nil {
		t.Fatal("closed database read")
	}
	if _, e = f.repo.Jobs(ctx, "", 10); e != nil {
		t.Fatal("fresh connection recovery", e)
	}
}

type rollbackDispatchDB struct {
	dispatchTestDB
	mode string
}
type rollbackDispatchTx struct {
	pgx.Tx
	mode string
}

func (d rollbackDispatchDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.dispatchTestDB.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &rollbackDispatchTx{tx, d.mode}, nil
}
func (t *rollbackDispatchTx) Exec(c context.Context, q string, a ...any) (pgconn.CommandTag, error) {
	if (t.mode == "claim" && strings.Contains(q, "UPDATE hp_processing.job_index SET claim_token=")) || (t.mode == "finish" && strings.Contains(q, "DELETE FROM hp_processing.job_index")) {
		return pgconn.CommandTag{}, errors.New("controlled second-statement failure")
	}
	return t.Tx.Exec(c, q, a...)
}
func TestDispatchAtomicRollbackBetweenOutboxAndIndex(t *testing.T) {
	f := newSendFixture(t)
	if e := f.repo.Index(ctx, f.request(), 10); e != nil {
		t.Fatal(e)
	}
	broken := DispatchStore{rollbackDispatchDB{mode: "claim"}}
	if _, e := broken.Claim(ctx, f.request(), f.message, time.Second); e == nil {
		t.Fatal("partial claim succeeded")
	}
	state, _, _, n := f.outbox(t)
	if state != "PENDING" || n != 0 {
		t.Fatal("outbox claim escaped rollback", state, n)
	}
	claim, e := f.repo.Claim(ctx, f.request(), f.message, time.Second)
	if e != nil {
		t.Fatal(e)
	}
	broken = DispatchStore{rollbackDispatchDB{mode: "finish"}}
	if e = broken.Finish(ctx, claim, "ACKED", time.Now()); e == nil {
		t.Fatal("partial finish succeeded")
	}
	state, _, _, n = f.outbox(t)
	if state != "CLAIMED" || n != 1 {
		t.Fatal("finish escaped rollback", state, n)
	}
	if e = f.repo.Check(ctx, claim); e != nil {
		t.Fatal("rollback lost original lease", e)
	}
	if e = f.repo.Finish(ctx, claim, "HELD", time.Now()); e != nil {
		t.Fatal(e)
	}
}
