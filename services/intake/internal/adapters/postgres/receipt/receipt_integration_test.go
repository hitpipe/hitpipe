//go:build integration

package receipt

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/contentref"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ws = "11111111-1111-4111-8111-111111111111"
const connID = "22222222-2222-4222-8222-222222222222"
const otherWS = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const fixtureSecret = "synthetic-signature-secret"

type testClock struct{ unix atomic.Int64 }

func (c *testClock) Now(context.Context) (time.Time, error) {
	return time.Unix(c.unix.Load(), 123456789), nil
}

type secretPort struct{ calls atomic.Int64 }

func (s *secretPort) Resolve(_ context.Context, id string) ([]byte, error) {
	s.calls.Add(1)
	if id != "install_synthetic" {
		return nil, errors.New("synthetic missing")
	}
	return []byte(fixtureSecret), nil
}

type fixtures struct {
	t             *testing.T
	app, migrator *pgxpool.Pool
	repo          *Repository
	clock         *testClock
	files         *contentstore.Store
	path          string
	secrets       *secretPort
	consumer      *receivesignal.Consumer
	connection    receivesignal.Connection
}

func (f *fixtures) sql(query string, args ...any) pgx.Rows {
	f.t.Helper()
	tx, e := f.migrator.Begin(context.Background())
	if e != nil {
		f.t.Fatal(e)
	}
	if _, e = tx.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,true)`, ws); e != nil {
		f.t.Fatal(e)
	}
	rows, e := tx.Query(context.Background(), query, args...)
	if e != nil {
		tx.Rollback(context.Background())
		f.t.Fatal(e)
	}
	return &txRows{Rows: rows, tx: tx}
}

type txRows struct {
	pgx.Rows
	tx pgx.Tx
}

func (r *txRows) Close() { r.Rows.Close(); r.tx.Commit(context.Background()) }
func (f *fixtures) exec(query string, args ...any) {
	f.t.Helper()
	r := f.sql(query, args...)
	for r.Next() {
	}
	e := r.Err()
	r.Close()
	if e != nil {
		f.t.Fatal(e)
	}
}
func (f *fixtures) scalar(query string, args ...any) int {
	f.t.Helper()
	r := f.sql(query, args...)
	defer r.Close()
	if !r.Next() {
		f.t.Fatal("no scalar")
	}
	var n int
	if e := r.Scan(&n); e != nil {
		f.t.Fatal(e)
	}
	return n
}
func newFixtures(t *testing.T) *fixtures {
	t.Helper()
	ctx := context.Background()
	appDSN, migDSN := os.Getenv("HIT86_APP_DSN"), os.Getenv("HIT86_MIGRATOR_DSN")
	if appDSN == "" || migDSN == "" {
		t.Fatal("integration requires isolated runner; never skip")
	}
	appCfg, e := pgxpool.ParseConfig(appDSN)
	if e != nil {
		t.Fatal(e)
	}
	appCfg.MaxConns = 5
	app, e := pgxpool.NewWithConfig(ctx, appCfg)
	if e != nil {
		t.Fatal(e)
	}
	m, e := pgxpool.New(ctx, migDSN)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(app.Close)
	t.Cleanup(m.Close)
	if _, e = m.Exec(ctx, `TRUNCATE hp_intake.source_connections,hp_intake.connection_secrets,hp_intake.ingress_quota,hp_intake.signals,hp_intake.source_facts,hp_intake.gap_markers,hp_intake.inbox,hp_intake.outbox,hp_intake.job_index CASCADE`); e != nil {
		t.Fatal(e)
	}
	clock := &testClock{}
	clock.unix.Store(1800000000)
	p, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	os.Chmod(p, 0700)
	store, e := contentstore.Open(p, func() time.Time { now, _ := clock.Now(ctx); return now })
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	repo, e := New(app)
	if e != nil {
		t.Fatal(e)
	}
	mode := false
	connection := receivesignal.Connection{Binding: normalizesignal.Binding{WorkspaceID: ws, ConnectionID: connID, InstallationID: "install_synthetic", StripeAccountID: "acct_synthetic", Livemode: &mode, AccountScope: normalizesignal.ConnectedAccount, APIVersion: "synthetic-version"}, SecurityEpoch: 1}
	sec := &secretPort{}
	c, e := receivesignal.New(ws, sec, clock, 72*time.Hour, repo, store)
	if e != nil {
		t.Fatal(e)
	}
	f := &fixtures{t: t, app: app, migrator: m, repo: repo, clock: clock, files: store, path: p, secrets: sec, consumer: c, connection: connection}
	f.exec(`INSERT INTO hp_intake.source_connections(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at) VALUES($1,$2,'acct_synthetic',false,1,'READY',$3)`, ws, connID, time.Unix(1799999900, 0))
	return f
}
func (f *fixtures) delivery(event, kind string) receivesignal.Delivery {
	body := []byte(fmt.Sprintf(`{"id":%q,"object":"event","account":"acct_synthetic","api_version":"synthetic-version","type":%q,"created":1800000000,"livemode":false,"data":{"object":{"id":"cs_synthetic","object":"checkout.session","created":1700000000,"livemode":false,"metadata":{"brief":"NEVER_STORE"},"customer_email":"NEVER_STORE"}}}`, event, kind))
	d := receivesignal.Delivery{Connection: f.connection, RawBody: body}
	return f.sign(d)
}
func (f *fixtures) sign(d receivesignal.Delivery) receivesignal.Delivery {
	h := hmac.New(sha256.New, []byte(fixtureSecret))
	fmt.Fprintf(h, "%d.", f.clock.unix.Load())
	h.Write(d.RawBody)
	d.Signature = fmt.Sprintf("t=%d,v1=%s", f.clock.unix.Load(), hex.EncodeToString(h.Sum(nil)))
	return d
}
func (f *fixtures) receive(event string) *normalizesignal.Signal {
	f.t.Helper()
	s, e := f.consumer.Receive(context.Background(), f.delivery(event, "checkout.session.completed"))
	if e != nil {
		f.t.Fatal(e)
	}
	return s
}
func (f *fixtures) outboxRef(id string) contentref.Ref {
	r := f.sql(`SELECT payload_ref::text,payload_sha256,payload_bytes,expires_at FROM hp_intake.outbox WHERE message_id=$1`, id)
	defer r.Close()
	if !r.Next() {
		f.t.Fatal("missing outbox")
	}
	var ref contentref.Ref
	if e := r.Scan(&ref.ID, &ref.Digest, &ref.Bytes, &ref.ExpiresAt); e != nil {
		f.t.Fatal(e)
	}
	return ref
}
func TestIntegrationCommitFilesDigestAndReplay(t *testing.T) {
	f := newFixtures(t)
	for _, kind := range []string{"checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		t.Run(kind, func(t *testing.T) {
			d := f.delivery("evt_"+kind, kind)
			s, e := f.consumer.Receive(context.Background(), d)
			if e != nil {
				t.Fatal(e)
			}
			ref := f.outboxRef(s.MessageID)
			b, e := f.files.Read(ref)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(b), "NEVER_STORE") {
				t.Fatal("content leak")
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(b, &fields) != nil || len(fields) != 14 {
				t.Fatal("schema fields")
			}
			delete(fields, "message_digest")
			raw, _ := json.Marshal(fields)
			canonical, e := jcs.Transform(raw)
			if e != nil {
				t.Fatal(e)
			}
			hash := sha256.Sum256(canonical)
			if s.MessageDigest != "sha256:"+hex.EncodeToString(hash[:]) {
				t.Fatal("wire digest")
			}
			f.clock.unix.Add(10)
			d = f.sign(d)
			again, e := f.consumer.Receive(context.Background(), d)
			if e != nil || *again != *s {
				t.Fatal("replay changed", e)
			}
			b2, e := f.files.Read(ref)
			if e != nil || string(b) != string(b2) {
				t.Fatal("replay bytes changed")
			}
			d.Signature = "forged"
			if got, e := f.consumer.Receive(context.Background(), d); e == nil || got != nil {
				t.Fatal("forged replay")
			}
		})
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 3 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 3 || f.scalar(`SELECT count(*) FROM hp_intake.inbox`) != 0 {
		t.Fatal("SQL shape")
	}
	if f.secrets.calls.Load() < 9 {
		t.Fatal("authentication skipped")
	}
}
func TestIntegrationConcurrentEventsAndInstances(t *testing.T) {
	f := newFixtures(t)
	var wg sync.WaitGroup
	results := make(chan *normalizesignal.Signal, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := New(f.app)
			if e != nil {
				errs <- e
				return
			}
			c, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, r, f.files)
			if e != nil {
				errs <- e
				return
			}
			s, e := c.Receive(context.Background(), f.delivery("evt_concurrent", "checkout.session.completed"))
			if e != nil {
				errs <- e
			} else {
				results <- s
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	var first *normalizesignal.Signal
	n := 0
	for s := range results {
		if first == nil {
			first = s
		}
		if *s != *first {
			t.Fatal("different receipt")
		}
		n++
	}
	if n != 12 || f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 1 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("duplicate")
	}
	files, _ := os.ReadDir(f.path)
	if len(files) != 1 {
		t.Fatal("duplicate files")
	}
}
func TestIntegrationRejectContextIdentityAndShape(t *testing.T) {
	f := newFixtures(t)
	d := f.delivery("evt_original", "checkout.session.completed")
	f.receive("evt_original")
	d.RawBody = []byte(strings.Replace(string(d.RawBody), "cs_synthetic", "cs_other", 1))
	d = f.sign(d)
	if s, e := f.consumer.Receive(context.Background(), d); !errors.Is(e, receivesignal.ErrConflict) || s != nil {
		t.Fatal("event identity overwritten", e)
	}
	cases := map[string]func(*receivesignal.Delivery){"unknown-mode": func(d *receivesignal.Delivery) { d.Connection.Binding.Livemode = nil }, "wrong-account": func(d *receivesignal.Delivery) { d.Connection.Binding.StripeAccountID = "acct_other" }, "wrong-workspace": func(d *receivesignal.Delivery) { d.Connection.Binding.WorkspaceID = otherWS }, "unknown-epoch": func(d *receivesignal.Delivery) { d.Connection.SecurityEpoch = 0 }, "wrong-epoch": func(d *receivesignal.Delivery) { d.Connection.SecurityEpoch = 2 }, "signature": func(d *receivesignal.Delivery) { d.Signature = "forged" }, "shape": func(d *receivesignal.Delivery) { d.RawBody = []byte(`{}`); *d = f.sign(*d) }}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			d := f.delivery("evt_reject_"+name, "checkout.session.completed")
			change(&d)
			if s, e := f.consumer.Receive(context.Background(), d); e == nil || s != nil {
				t.Fatal("rejected input accepted")
			}
		})
	}
	for _, health := range []string{"REVOKED", "DEGRADED", "CHECKING"} {
		f.exec(`UPDATE hp_intake.source_connections SET health=$1`, health)
		if s, e := f.consumer.Receive(context.Background(), f.delivery("evt_health", "checkout.session.completed")); !errors.Is(e, receivesignal.ErrContext) || s != nil {
			t.Fatal("health accepted", e)
		}
	}
	f.exec(`UPDATE hp_intake.source_connections SET health='READY',revoked_at=now()`)
	if s, e := f.consumer.Receive(context.Background(), f.delivery("evt_revoked", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("revoked accepted")
	}
	f.exec(`UPDATE hp_intake.source_connections SET revoked_at=NULL,verified_at=NULL`)
	if s, e := f.consumer.Receive(context.Background(), f.delivery("evt_unverified", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("unverified accepted")
	}
	f.exec(`DELETE FROM hp_intake.signals`)
	f.exec(`DELETE FROM hp_intake.outbox`)
	f.exec(`DELETE FROM hp_intake.source_connections`)
	if s, e := f.consumer.Receive(context.Background(), f.delivery("evt_missing", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("missing accepted")
	}
}
func TestIntegrationRollbackUnknownCommitAndCleanup(t *testing.T) {
	for _, stage := range []string{"after_signal", "before_commit", "after_commit"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixtures(t)
			f.repo.fault = func(where string) error {
				if where == stage {
					return errors.New("synthetic fault")
				}
				return nil
			}
			d := f.delivery("evt_fault", "checkout.session.completed")
			s, e := f.consumer.Receive(context.Background(), d)
			if e == nil || s != nil {
				t.Fatal("false success")
			}
			expected := 0
			if stage == "after_commit" {
				expected = 1
				if !errors.Is(e, receivesignal.ErrUncertain) {
					t.Fatal("unknown not explicit")
				}
			}
			if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != expected || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != expected {
				t.Fatal("partial SQL")
			}
			files, _ := os.ReadDir(f.path)
			if len(files) != 1 {
				t.Fatal("actual orphan missing")
			}
			f.repo.fault = nil
			r, _ := New(f.app)
			c, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, r, f.files)
			if e != nil {
				t.Fatal(e)
			}
			retry, e := c.Receive(context.Background(), d)
			if e != nil {
				t.Fatal(e)
			}
			if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 1 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
				t.Fatal("recovery duplicate")
			}
			removed, e := c.Cleanup(context.Background(), f.connection)
			if e != nil || removed != 1-expected {
				t.Fatal("cleanup", removed, e)
			}
			if _, e := f.files.Read(f.outboxRef(retry.MessageID)); e != nil {
				t.Fatal("accepted file removed")
			}
		})
	}
}
func TestIntegrationExpiryMissingContentAndRLS(t *testing.T) {
	f := newFixtures(t)
	s := f.receive("evt_expiry")
	ref := f.outboxRef(s.MessageID)
	f.clock.unix.Add(72 * 3600)
	d := f.sign(f.delivery("evt_expiry", "checkout.session.completed"))
	if got, e := f.consumer.Receive(context.Background(), d); e == nil || got != nil {
		t.Fatal("expired replay accepted")
	}
	if _, e := f.files.Read(ref); e == nil {
		t.Fatal("expired content served")
	}
	f.clock.unix.Add(-72 * 3600)
	if e := os.Remove(filepath.Join(f.path, ref.ID)); e != nil {
		t.Fatal(e)
	}
	if got, e := f.consumer.Receive(context.Background(), f.delivery("evt_expiry", "checkout.session.completed")); e == nil || got != nil {
		t.Fatal("missing content success")
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("missing content replaced")
	}
	ctx := context.Background()
	tx, e := f.app.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, otherWS)
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM hp_intake.signals`).Scan(&n); e != nil || n != 0 {
		t.Fatal("RLS foreign leak", e)
	}
	r, _ := New(f.migrator)
	if e := r.WithSession(ctx, func(receivesignal.Session) error { return nil }); !errors.Is(e, receivesignal.ErrContext) {
		t.Fatal("owner role admitted")
	}
}

// Synthetic boundary fixtures are inserted under the migrator's scoped RLS.
// They exercise exact quota trigger limits, not a realistic normalized workload.
func TestIntegrationConcurrentQuotaBoundaries(t *testing.T) {
	for _, kind := range []string{"count", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixtures(t)
			size := 1
			count := 999
			if kind == "bytes" {
				size = 16777216 - 900
				count = 1
			}
			f.exec(`INSERT INTO hp_intake.signals(workspace_id,signal_id,connection_id,event_id,event_type,session_id,account_id,livemode,source_created_at,first_received_at,expires_at,serialized_bytes,state,reason_code) SELECT $1, md5(i::text)::uuid,$2,'evt_seed_'||i,'checkout.session.completed','cs_seed','acct_synthetic',false,$3,$3,$4,$5,'HELD','SYNTHETIC_BOUNDARY' FROM generate_series(1,$6) i`, ws, connID, time.Unix(1800000000, 0), time.Unix(1800003600, 0), size, count)
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					_, e := f.consumer.Receive(context.Background(), f.delivery(fmt.Sprintf("evt_capacity_%d", i), "checkout.session.completed"))
					errs <- e
				}(i)
			}
			wg.Wait()
			close(errs)
			success, quota := 0, 0
			for e := range errs {
				if e == nil {
					success++
				} else if errors.Is(e, receivesignal.ErrQuota) {
					quota++
				} else {
					t.Fatal("unexpected capacity outcome", e)
				}
			}
			if success != 1 || quota != 1 {
				t.Fatal("quota was not atomic", success, quota)
			}
			if f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
				t.Fatal("partial outbox")
			}
			if _, e := f.consumer.Cleanup(context.Background(), f.connection); e != nil {
				t.Fatal(e)
			}
			files, _ := os.ReadDir(f.path)
			if len(files) != 1 {
				t.Fatal("capacity orphan cleanup")
			}
		})
	}
}

type observingContent struct {
	Content receivesignal.Content
	before  func()
	after   func(contentref.Ref)
	fail    bool
}

func (c observingContent) Put(b []byte, e time.Time) (contentref.Ref, error) {
	if c.before != nil {
		c.before()
	}
	if c.fail {
		return contentref.Ref{}, errors.New("synthetic file failure")
	}
	r, err := c.Content.Put(b, e)
	if err == nil && c.after != nil {
		c.after(r)
	}
	return r, err
}
func (c observingContent) Read(r contentref.Ref) ([]byte, error) { return c.Content.Read(r) }
func (c observingContent) Purge(m map[string]bool) (int, error)  { return c.Content.Purge(m) }
func TestIntegrationFileBeforeTransactionAndFailure(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	observe := func() {
		if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 0 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 0 {
			t.Fatal("SQL preceded file")
		}
		r := f.sql(`SELECT count(*) FROM pg_stat_activity WHERE usename='hp_intake_app' AND xact_start IS NOT NULL`)
		defer r.Close()
		r.Next()
		var n int
		if r.Scan(&n) != nil || n != 0 {
			t.Fatal("SQL transaction held during fsync")
		}
	}
	content := observingContent{Content: f.files, before: observe, after: func(ref contentref.Ref) {
		observe()
		if _, e := f.files.Read(ref); e != nil {
			t.Fatal(e)
		}
	}}
	c, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, content)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Receive(ctx, f.delivery("evt_order", "checkout.session.completed")); e != nil {
		t.Fatal(e)
	}
	content = observingContent{Content: f.files, fail: true}
	c, _ = receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, content)
	if s, e := c.Receive(ctx, f.delivery("evt_file_failure", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("file error success")
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 1 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("file failure wrote SQL")
	}
}

func TestIntegrationConnectionAndWorkspaceSeparation(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	original := f.receive("evt_scoped")
	secondID := "33333333-3333-4333-8333-333333333333"
	f.exec(`INSERT INTO hp_intake.source_connections(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at) VALUES($1,$2,'acct_synthetic',false,1,'READY',$3)`, ws, secondID, time.Unix(1799999900, 0))
	d := f.delivery("evt_scoped", "checkout.session.completed")
	d.Connection.Binding.ConnectionID = secondID
	second, e := f.consumer.Receive(ctx, d)
	if e != nil || second.ReceiptID == original.ReceiptID {
		t.Fatal("connections collapsed", e)
	}
	tx, e := f.migrator.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, otherWS); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(ctx, `INSERT INTO hp_intake.source_connections(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at) VALUES($1,$2,'acct_synthetic',false,1,'READY',$3)`, otherWS, connID, time.Unix(1799999900, 0)); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	path, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	os.Chmod(path, 0700)
	files, e := contentstore.Open(path, func() time.Time { n, _ := f.clock.Now(ctx); return n })
	if e != nil {
		t.Fatal(e)
	}
	defer files.Close()
	consumer, e := receivesignal.New(otherWS, f.secrets, f.clock, 72*time.Hour, f.repo, files)
	if e != nil {
		t.Fatal(e)
	}
	d = f.delivery("evt_scoped", "checkout.session.completed")
	d.Connection.Binding.WorkspaceID = otherWS
	foreign, e := consumer.Receive(ctx, d)
	if e != nil || foreign.ReceiptID == original.ReceiptID {
		t.Fatal("workspaces collapsed", e)
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 2 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 2 {
		t.Fatal("wrong scoped count")
	}
	if n, e := f.consumer.Cleanup(ctx, f.connection); n != 0 || e != nil {
		t.Fatal("foreign references affected own cleanup", e)
	}
	entries, _ := os.ReadDir(path)
	if len(entries) != 1 {
		t.Fatal("foreign file changed")
	}
}
func TestIntegrationRecoveryWithNewPoolAndFiles(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	first := f.receive("evt_restart")
	pool, e := pgxpool.New(ctx, os.Getenv("HIT86_APP_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	repo, e := New(pool)
	if e != nil {
		t.Fatal(e)
	}
	files, e := contentstore.Open(f.path, func() time.Time { n, _ := f.clock.Now(ctx); return n })
	if e != nil {
		t.Fatal(e)
	}
	defer files.Close()
	consumer, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, repo, files)
	if e != nil {
		t.Fatal(e)
	}
	f.clock.unix.Add(60)
	repeated, e := consumer.Receive(ctx, f.sign(f.delivery("evt_restart", "checkout.session.completed")))
	if e != nil || *first != *repeated {
		t.Fatal("restart changed receipt", e)
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("restart duplicated outbox")
	}
}
func TestIntegrationRevocationBetweenFileAndCommit(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	content := observingContent{Content: f.files, after: func(contentref.Ref) {
		f.exec(`UPDATE hp_intake.source_connections SET health='REVOKED',revoked_at=now()`)
	}}
	consumer, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, content)
	if e != nil {
		t.Fatal(e)
	}
	if s, e := consumer.Receive(ctx, f.delivery("evt_revoke_between", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("revoked between stages accepted")
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 0 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 0 {
		t.Fatal("partial revoked receipt")
	}
	f.exec(`UPDATE hp_intake.source_connections SET health='READY',revoked_at=NULL`)
	if n, e := f.consumer.Cleanup(ctx, f.connection); e != nil || n != 1 {
		t.Fatal("revoked orphan cleanup", e)
	}
}

func TestIntegrationLeaseAndScopeResetAfterRollback(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	f.repo.fault = func(string) error { return errors.New("synthetic rollback") }
	if s, e := f.consumer.Receive(ctx, f.delivery("evt_reset", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("fault accepted")
	}
	f.repo.fault = nil
	conn, e := f.app.Acquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Release()
	var scope string
	if e = conn.QueryRow(ctx, `SELECT coalesce(current_setting('hp.workspace_id',true),'')`).Scan(&scope); e != nil || scope != "" {
		t.Fatal("scope leaked", e)
	}
	var locked bool
	if e = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, leaseKey).Scan(&locked); e != nil || !locked {
		t.Fatal("lease leaked", e)
	}
	conn.Exec(ctx, `SELECT pg_advisory_unlock($1)`, leaseKey)
	var n int
	if e = conn.QueryRow(ctx, `SELECT count(*) FROM hp_intake.signals`).Scan(&n); e != nil || n != 0 {
		t.Fatal("unscoped RLS read", e)
	}
}

func TestIntegrationContentLostOrExpiredBeforeSQL(t *testing.T) {
	for _, kind := range []string{"missing", "expired"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixtures(t)
			ctx := context.Background()
			content := observingContent{Content: f.files, after: func(ref contentref.Ref) {
				if kind == "missing" {
					if e := os.Remove(filepath.Join(f.path, ref.ID)); e != nil {
						t.Fatal(e)
					}
				} else {
					f.clock.unix.Add(72 * 3600)
				}
			}}
			consumer, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, content)
			if e != nil {
				t.Fatal(e)
			}
			if s, e := consumer.Receive(ctx, f.delivery("evt_expired_during_file", "checkout.session.completed")); e == nil || s != nil {
				t.Fatal("unavailable new content admitted")
			}
			if f.scalar(`SELECT count(*) FROM hp_intake.signals`) != 0 || f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 0 {
				t.Fatal("SQL accepted unavailable content")
			}
		})
	}
}

func TestIntegrationCleanupWaitsForUncommittedFile(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	saved := make(chan struct{})
	proceed := make(chan struct{})
	defer func() {
		select {
		case <-proceed:
		default:
			close(proceed)
		}
	}()
	content := observingContent{Content: f.files, after: func(contentref.Ref) { close(saved); <-proceed }}
	consumer, e := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, content)
	if e != nil {
		t.Fatal(e)
	}
	received := make(chan error, 1)
	go func() {
		_, e := consumer.Receive(ctx, f.delivery("evt_cleanup_race", "checkout.session.completed"))
		received <- e
	}()
	<-saved
	cleaned := make(chan error, 1)
	go func() {
		n, e := f.consumer.Cleanup(ctx, f.connection)
		if e == nil && n != 0 {
			e = errors.New("accepted publication removed")
		}
		cleaned <- e
	}()
	select {
	case e := <-cleaned:
		t.Fatal("cleanup bypassed pending publication", e)
	case <-time.After(30 * time.Millisecond):
	}
	close(proceed)
	if e := <-received; e != nil {
		t.Fatal(e)
	}
	if e := <-cleaned; e != nil {
		t.Fatal(e)
	}
	if f.scalar(`SELECT count(*) FROM hp_intake.outbox`) != 1 {
		t.Fatal("publication lost")
	}
	files, _ := os.ReadDir(f.path)
	if len(files) != 1 {
		t.Fatal("accepted file lost")
	}
}
func TestIntegrationExpiredStateIsNotReadableReceipt(t *testing.T) {
	f := newFixtures(t)
	f.receive("evt_marked_expired")
	f.exec(`UPDATE hp_intake.signals SET state='EXPIRED'`)
	if s, e := f.consumer.Receive(context.Background(), f.delivery("evt_marked_expired", "checkout.session.completed")); e == nil || s != nil {
		t.Fatal("expired state accepted")
	}
}
