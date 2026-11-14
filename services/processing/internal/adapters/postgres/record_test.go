package postgres

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/jackc/pgx/v5"
)

const op = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const config = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
const owner = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"

var ctx = context.Background()

func connect(t *testing.T) *pgx.Conn {
	t.Helper()
	if os.Getenv("HIT78_ISOLATED_PG") != "1" {
		t.Skip("requires owned isolated HIT-78 PostgreSQL runner")
	}
	c, e := pgx.Connect(ctx, "host=/var/run/postgresql user=hp_processing_app dbname=hit78 sslmode=disable")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close(ctx) })
	return c
}
func begin(t *testing.T, c *pgx.Conn, ws string) pgx.Tx {
	t.Helper()
	tx, e := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { tx.Rollback(ctx) })
	if _, e = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", ws); e != nil {
		t.Fatal(e)
	}
	return tx
}
func workspace(t *testing.T) string {
	t.Helper()
	ws, e := newID()
	if e != nil {
		t.Fatal(e)
	}
	c := connect(t)
	tx := begin(t, c, ws)
	for _, q := range []string{
		`INSERT INTO hp_processing.operations(workspace_id,operation_id,owner_user_id,revision,state,first_activation_at) VALUES($1,'` + op + `','` + owner + `',1,'ACTIVE','2026-11-14T00:00:00Z')`,
		`INSERT INTO hp_processing.accepted_configs VALUES($1,'` + config + `','` + op + `','1.0','synthetic','{"pack_id":"synthetic"}','sha256:` + strings.Repeat("a", 64) + `','` + owner + `','2026-11-14T00:00:00Z','synthetic')`,
		`UPDATE hp_processing.operations SET selected_config_id='` + config + `' WHERE workspace_id=$1`,
	} {
		if _, e = tx.Exec(ctx, q, ws); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return ws
}

// This fixture bypasses only wire validation to test the private storage layer.
// It is NOT a production authority/binding or activation-boundary implementation.
func signal(t *testing.T, ws string) admission.Signal {
	t.Helper()
	id, e := newID()
	if e != nil {
		t.Fatal(e)
	}
	return admission.Signal{WorkspaceID: ws, MessageID: id, Digest: "sha256:" + strings.Repeat("a", 64), AccountID: "acct_synthetic", SessionID: "cs_synthetic", ReceiptID: id}
}
func apply(t *testing.T, s admission.Signal) recordResult {
	t.Helper()
	c := connect(t)
	tx := begin(t, c, s.WorkspaceID)
	r, e := record(ctx, tx, s, op, config, owner)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	return r
}
func counts(t *testing.T, ws string, wantInbox, wantCase, wantHistory int) {
	t.Helper()
	c := connect(t)
	tx := begin(t, c, ws)
	for i, table := range []string{"inbox", "case_identity", "safety_history"} {
		var n int
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table).Scan(&n); e != nil {
			t.Fatal(e)
		}
		if n != []int{wantInbox, wantCase, wantHistory}[i] {
			t.Fatalf("%s count=%d", table, n)
		}
	}
}
func TestRecordCommitRepeatConflictAndReconnect(t *testing.T) {
	ws := workspace(t)
	s := signal(t, ws)
	first := apply(t, s)
	if first.Disposition != "APPLIED" || first.RecordID == "" {
		t.Fatal(first)
	}
	if again := apply(t, s); again != first {
		t.Fatalf("repeat changed result: %v", again)
	}
	c := connect(t)
	tx := begin(t, c, ws)
	other := s
	other.Digest = "sha256:" + strings.Repeat("b", 64)
	if _, e := record(ctx, tx, other, op, config, owner); !errors.Is(e, errConflict) {
		t.Fatalf("conflict: %v", e)
	}
	tx.Rollback(ctx)
	next := signal(t, ws)
	if r := apply(t, next); r.RecordID != first.RecordID {
		t.Fatal("second message created another identity")
	}
	counts(t, ws, 2, 1, 1)
	tx = begin(t, c, ws)
	var revision int
	var gotOwner, gotOp, gotConfig string
	if e := tx.QueryRow(ctx, `SELECT revision,owner_user_id::text,operation_id::text,config_id::text FROM hp_processing.case_identity`).Scan(&revision, &gotOwner, &gotOp, &gotConfig); e != nil {
		t.Fatal(e)
	}
	if revision != 1 || gotOwner != owner || gotOp != op || gotConfig != config {
		t.Fatal("pinned state changed")
	}
	tx.Rollback(ctx)
	next.SessionID = "cs_different"
	next.MessageID, _ = newID()
	apply(t, next)
	counts(t, ws, 3, 2, 2)
	ws2 := workspace(t)
	next.WorkspaceID = ws2
	apply(t, next)
	counts(t, ws2, 1, 1, 1)
}
func TestRecordRollbackAndCommitFailure(t *testing.T) {
	for _, mode := range []string{"rollback", "commit_failure", "history_failure"} {
		t.Run(mode, func(t *testing.T) {
			ws := workspace(t)
			s := signal(t, ws)
			c := connect(t)
			tx := begin(t, c, ws)
			if mode == "history_failure" {
				s.ReceiptID = "invalid-uuid"
			}
			_, e := record(ctx, tx, s, op, config, owner)
			if mode == "history_failure" {
				if e == nil {
					t.Fatal("expected failure after case insert")
				}
				tx.Rollback(ctx)
			} else {
				if e != nil {
					t.Fatal(e)
				}
				if mode == "rollback" {
					tx.Rollback(ctx)
				} else {
					// Deferrable selected_config FK fails at COMMIT after all three writes.
					if _, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id='dddddddd-dddd-4ddd-8ddd-dddddddddddd' WHERE workspace_id=$1`, ws); e != nil {
						t.Fatal(e)
					}
					if e = tx.Commit(ctx); e == nil {
						t.Fatal("expected commit failure")
					}
				}
			}
			counts(t, ws, 0, 0, 0)
		})
	}
}
func TestRecordConcurrentIdentityAndMessage(t *testing.T) {
	for _, sameMessage := range []bool{false, true} {
		t.Run(map[bool]string{true: "same_message", false: "two_messages"}[sameMessage], func(t *testing.T) {
			ws := workspace(t)
			a, b := signal(t, ws), signal(t, ws)
			if sameMessage {
				b = a
			}
			c1, c2 := connect(t), connect(t)
			start := make(chan struct{})
			results := make(chan recordResult, 2)
			errs := make(chan error, 2)
			var wg sync.WaitGroup
			for i, s := range []admission.Signal{a, b} {
				wg.Add(1)
				go func(c *pgx.Conn, s admission.Signal) {
					defer wg.Done()
					<-start
					tx, e := c.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
					if e != nil {
						errs <- e
						return
					}
					defer tx.Rollback(ctx)
					_, e = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", ws)
					if e != nil {
						errs <- e
						return
					}
					r, e := record(ctx, tx, s, op, config, owner)
					if e == nil {
						e = tx.Commit(ctx)
					}
					if e != nil {
						errs <- e
						return
					}
					results <- r
				}([]*pgx.Conn{c1, c2}[i], s)
			}
			close(start)
			wg.Wait()
			close(errs)
			for e := range errs {
				t.Fatal(e)
			}
			x, y := <-results, <-results
			if x.RecordID != y.RecordID {
				t.Fatal("concurrent identity split")
			}
			n := 2
			if sameMessage {
				n = 1
			}
			counts(t, ws, n, 1, 1)
		})
	}
}
func TestRecordRLSRejectsForeignWorkspace(t *testing.T) {
	ws, foreign := workspace(t), workspace(t)
	s := signal(t, foreign)
	c := connect(t)
	tx := begin(t, c, ws)
	if _, e := record(ctx, tx, s, op, config, owner); e == nil {
		t.Fatal("foreign write permitted")
	}
	tx.Rollback(ctx)
	counts(t, foreign, 0, 0, 0)
}

func TestEnvironment(t *testing.T) {
	t.Log(runtime.Version(), runtime.GOOS, runtime.GOARCH, "isolated_pg="+os.Getenv("HIT78_ISOLATED_PG"))
}
