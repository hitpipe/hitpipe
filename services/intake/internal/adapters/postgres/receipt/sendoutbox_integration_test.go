//go:build integration

package receipt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/postgres/outbox"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/processing"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/sendoutbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

var senderScope = app.Scope{Workspace: ws, Connection: connID, Account: "acct_synthetic", Epoch: 1}
var senderPolicy = app.Policy{Batch: 4, Attempts: 3, Lease: 5 * time.Second, Timeout: 2 * time.Second, BaseBackoff: 100 * time.Millisecond, MaxBackoff: 500 * time.Millisecond}

type testSendGate func(context.Context, app.Scope, func(context.Context) error) error

func (f testSendGate) Start(c context.Context, s app.Scope, run func(context.Context) error) error {
	return f(c, s, run)
}
func allowSender(c context.Context, s app.Scope, run func(context.Context) error) error {
	if s != senderScope || c.Err() != nil {
		return app.ErrDenied
	}
	return run(c)
}

func senderPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("HIT118_WORKER_DSN")
	if dsn == "" {
		t.Fatal("isolated worker DSN required")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 5
	p, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}
func senderRepo(t *testing.T, p *pgxpool.Pool) *outbox.Repository {
	t.Helper()
	r, err := outbox.New(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func sender(t *testing.T, f *fixtures, r app.Repository, g testSendGate, c app.Transport, p app.Policy) *app.Worker {
	t.Helper()
	w, err := app.New(ws, r, g, f.files, c, p)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func senderFixture(t *testing.T) (*fixtures, *outbox.Repository) {
	t.Helper()
	f := newFixtures(t)
	f.clock.unix.Store(time.Now().Unix() - 10)
	return f, senderRepo(t, senderPool(t))
}

type countListener struct {
	net.Listener
	accepted atomic.Int64
}

func (l *countListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err == nil {
		l.accepted.Add(1)
	}
	return c, err
}

type processor struct {
	listener    *countListener
	roots       *x509.CertPool
	certificate tls.Certificate
}

func (p processor) client(t *testing.T, name string) *processing.Client {
	t.Helper()
	c, err := processing.New(p.listener.Addr().String(), name, p.roots, p.certificate)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func testProcessor(t *testing.T, handler http.HandlerFunc) processor {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	cert := func(serial int64, name string, usage x509.ExtKeyUsage) tls.Certificate {
		t.Helper()
		p, k, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		x := &x509.Certificate{SerialNumber: big.NewInt(serial), DNSNames: []string{name},
			NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{usage}}
		der, e := x509.CreateCertificate(rand.Reader, x, root, p, key)
		if e != nil {
			t.Fatal(e)
		}
		return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	l := &countListener{Listener: listener}
	cfg := &tls.Config{Certificates: []tls.Certificate{cert(2, "processing.fixture", x509.ExtKeyUsageServerAuth)},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"}}
	s := &http.Server{TLSConfig: cfg, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.TLS.PeerCertificates[0].VerifyHostname("intake.fixture") != nil ||
			r.Method != "POST" || r.URL.Path != processing.Path || r.URL.RawQuery != "" ||
			r.Header.Get("Content-Type") != "application/json" || !app.ValidUUID(r.Header.Get("X-Request-ID")) {
			t.Error("unexpected private request")
			w.WriteHeader(400)
			return
		}
		handler(w, r)
	})}
	done := make(chan error, 1)
	go func() { done <- s.Serve(tls.NewListener(l, cfg)) }()
	t.Cleanup(func() {
		c, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		if err := s.Shutdown(c); err != nil {
			t.Error(err)
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	})
	return processor{l, roots, cert(3, "intake.fixture", x509.ExtKeyUsageClientAuth)}
}

func responseReceipt(w http.ResponseWriter, id, disposition string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(map[string]string{"message_id": id, "disposition": disposition})
}
func senderState(f *fixtures, id string, want app.Outcome, attempts int) {
	f.t.Helper()
	r := f.sql(`SELECT state,delivery_count,claim_token IS NULL,lease_until IS NULL FROM hp_intake.outbox WHERE message_id=$1`, id)
	defer r.Close()
	var state string
	var count int
	var a, b bool
	if !r.Next() || r.Scan(&state, &count, &a, &b) != nil || state != string(want) || count != attempts || !a || !b {
		f.t.Fatalf("outbox state %s/%d want %s/%d", state, count, want, attempts)
	}
}
func lostACKServer(t *testing.T, f *fixtures, id string, original []byte, seen *atomic.Int64) processor {
	t.Helper()
	ctx := context.Background()
	// The controlled Processing fixture uses a real durable minimal inbox table
	// in the disposable database. It exercises commit/ack loss and dedup only.
	_, err := f.migrator.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.fixture_processing_inbox
		(message_id uuid PRIMARY KEY, digest bytea NOT NULL);
		TRUNCATE public.fixture_processing_inbox`)
	if err != nil {
		t.Fatal(err)
	}
	return testProcessor(t, func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		if err != nil || !bytes.Equal(b, original) {
			t.Error("payload changed on transport")
			w.WriteHeader(400)
			return
		}
		var locks int
		err = f.migrator.QueryRow(r.Context(), `SELECT count(*) FROM pg_stat_activity WHERE usename='hp_intake_worker' AND state='idle in transaction'`).Scan(&locks)
		if err != nil || locks != 0 {
			t.Error("SQL transaction held during network", locks, err)
			w.WriteHeader(500)
			return
		}
		hash := sha256.Sum256(b)
		tx, err := f.migrator.Begin(r.Context())
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		defer tx.Rollback(context.Background())
		tag, err := tx.Exec(r.Context(), `INSERT INTO public.fixture_processing_inbox VALUES($1,$2) ON CONFLICT DO NOTHING`, id, hash[:])
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		var saved []byte
		if err := tx.QueryRow(r.Context(), `SELECT digest FROM public.fixture_processing_inbox WHERE message_id=$1`, id).Scan(&saved); err != nil || !bytes.Equal(saved, hash[:]) || tx.Commit(r.Context()) != nil {
			t.Error("durable fixture commit failed", err)
			w.WriteHeader(500)
			return
		}
		if seen.Add(1) == 1 {
			c, _, err := http.NewResponseController(w).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = c.Close()
			return
		}
		if tag.RowsAffected() != 0 {
			t.Error("duplicate inserted")
		}
		responseReceipt(w, id, "DUPLICATE")
	})
}

func TestIntegrationSenderLostACKAndRestart(t *testing.T) {
	f, repo := senderFixture(t)
	s := f.receive("evt_sender_loss")
	ref := f.outboxRef(s.MessageID)
	raw, err := f.files.Read(ref)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	p := lostACKServer(t, f, s.MessageID, raw, &calls)
	client := p.client(t, "processing.fixture")
	gate := testSendGate(allowSender)
	res, err := sender(t, f, repo, gate, client, senderPolicy).Run(context.Background(), senderScope)
	if err != nil || res.Retried != 1 || calls.Load() != 1 {
		t.Fatal("lost ACK was not one retry", res, err, calls.Load())
	}
	senderState(f, s.MessageID, app.Pending, 1)
	if n := f.scalar("SELECT count(*) FROM public.fixture_processing_inbox"); n != 1 {
		t.Fatal("receiver did not commit before loss")
	}
	time.Sleep(120 * time.Millisecond)
	restarted := senderRepo(t, senderPool(t))
	res, err = sender(t, f, restarted, gate, client, senderPolicy).Run(context.Background(), senderScope)
	if err != nil || res.Acked != 1 || calls.Load() != 2 {
		t.Fatal("restart did not establish the original receipt", res, err, calls.Load())
	}
	senderState(f, s.MessageID, app.Acked, 2)
	if f.outboxRef(s.MessageID) != ref || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 1 ||
		f.scalar("SELECT count(*) FROM hp_intake.signals WHERE state='RECEIVED'") != 1 {
		t.Fatal("ack changed original message or business state")
	}
}

func TestIntegrationSenderLeaseAndScope(t *testing.T) {
	f, repo := senderFixture(t)
	msg := f.receive("evt_sender_lease")
	const workers = 6
	var wg sync.WaitGroup
	results := make(chan app.Attempt, workers)
	for range workers {
		wg.Go(func() {
			a, ok, err := repo.Claim(context.Background(), senderScope, time.Second, time.Now())
			if err != nil {
				t.Error(err)
				return
			}
			if ok {
				results <- a
			}
		})
	}
	wg.Wait()
	close(results)
	if len(results) != 1 {
		t.Fatalf("claims=%d", len(results))
	}
	first := <-results
	f.exec("UPDATE hp_intake.outbox SET lease_until=clock_timestamp()-interval '1 second'")
	second, ok, err := repo.Claim(context.Background(), senderScope, 5*time.Second, time.Now())
	if err != nil || !ok || first.Token == second.Token || first.Signal != second.Signal || first.Content != second.Content {
		t.Fatal("reclaim lost identity", err)
	}
	if err := repo.Finish(context.Background(), senderScope, first, app.Acked, time.Time{}); !errors.Is(err, app.ErrStale) {
		t.Fatal("old lease completed", err)
	}
	foreign := senderScope
	foreign.Workspace = otherWS
	if _, ok, err = repo.Claim(context.Background(), foreign, time.Second, time.Now()); err != nil || ok {
		t.Fatal("foreign scope exposed work", err)
	}
	if err = repo.Finish(context.Background(), foreign, second, app.Acked, time.Time{}); !errors.Is(err, app.ErrStale) {
		t.Fatal("foreign scope completed", err)
	}
	if err = repo.Finish(context.Background(), senderScope, second, app.Acked, time.Time{}); err != nil {
		t.Fatal(err)
	}
	senderState(f, msg.MessageID, app.Acked, 2)
	if f.scalar("SELECT count(*) FROM hp_intake.job_index") != 0 {
		t.Fatal("sender used global job index")
	}
	if _, _, err = senderRepo(t, f.app).Claim(context.Background(), senderScope, time.Second, time.Now()); !errors.Is(err, app.ErrDenied) {
		t.Fatal("app role used as worker", err)
	}
}

func TestIntegrationSenderReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, disposition string
		status            int
		want              app.Outcome
	}{
		{"applied", "APPLIED", 202, app.Acked}, {"duplicate", "DUPLICATE", 202, app.Acked},
		{"held by Processing", "HELD", 202, app.Acked}, {"conflict", "CONFLICT", 202, app.Held},
		{"conflict HTTP", "", 409, app.Held}, {"wrong id", "", 202, app.Held}, {"bad JSON", "", 202, app.Held},
		{"duplicate fields", "", 202, app.Held}, {"redirect", "", 307, app.Held}, {"rate limit", "", 429, app.Pending},
		{"unavailable", "", 503, app.Pending},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, repo := senderFixture(t)
			msg := f.receive("evt_sender_receipt")
			var count atomic.Int64
			p := testProcessor(t, func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				switch tc.name {
				case "wrong id":
					responseReceipt(w, ws, "APPLIED")
				case "bad JSON":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(202)
					_, _ = io.WriteString(w, "{")
				case "duplicate fields":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(202)
					_, _ = fmt.Fprintf(w, `{"message_id":%q,"disposition":"CONFLICT","disposition":"APPLIED"}`, msg.MessageID)
				case "redirect":
					w.Header().Set("Location", processing.Path)
					w.WriteHeader(307)
				case "rate limit":
					w.Header().Set("Retry-After", "5")
					w.WriteHeader(429)
				default:
					if tc.disposition != "" {
						responseReceipt(w, msg.MessageID, tc.disposition)
					} else {
						w.WriteHeader(tc.status)
					}
				}
			})
			started := time.Now()
			if _, err := sender(t, f, repo, testSendGate(allowSender), p.client(t, "processing.fixture"), senderPolicy).Run(context.Background(), senderScope); err != nil {
				t.Fatal(err)
			}
			senderState(f, msg.MessageID, tc.want, 1)
			if count.Load() != 1 || p.listener.accepted.Load() != 1 {
				t.Fatal("unexpected redirect/retry or no request")
			}
			if tc.name == "rate limit" {
				rows := f.sql("SELECT next_attempt_at FROM hp_intake.outbox")
				defer rows.Close()
				var due time.Time
				if !rows.Next() || rows.Scan(&due) != nil || due.Before(started.Add(5*time.Second)) {
					t.Fatal("Retry-After was not honored")
				}
			}
		})
	}
}

func TestIntegrationSenderFailClosed(t *testing.T) {
	for _, kind := range []string{"no grant", "revoked before dial", "stale epoch", "missing file", "changed file", "expired", "wrong TLS name", "closed pool", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			f, repo := senderFixture(t)
			msg := f.receive("evt_sender_denied")
			policy := senderPolicy
			var starts atomic.Int64
			gate := testSendGate(func(c context.Context, s app.Scope, run func(context.Context) error) error {
				n := starts.Add(1)
				if kind == "no grant" {
					return nil
				}
				if kind == "revoked before dial" && n >= 3 {
					return app.ErrDenied
				}
				return allowSender(c, s, run)
			})
			ref := f.outboxRef(msg.MessageID)
			scope := senderScope
			expected := app.Held
			attempts := 1
			name := "processing.fixture"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "no grant", "stale epoch", "closed pool", "cancelled":
				expected = app.Pending
				attempts = 0
			}
			switch kind {
			case "stale epoch":
				f.exec("UPDATE hp_intake.source_connections SET security_epoch=2")
			case "missing file":
				if err := os.Remove(filepath.Join(f.path, ref.ID)); err != nil {
					t.Fatal(err)
				}
			case "changed file":
				if err := os.WriteFile(filepath.Join(f.path, ref.ID), bytes.Repeat([]byte{'x'}, ref.Bytes), 0600); err != nil {
					t.Fatal(err)
				}
			case "expired":
				// Expiry of the durable outbox reference, independent of fixture time.
				f.exec("UPDATE hp_intake.outbox SET created_at=clock_timestamp()-interval '3 hours',expires_at=clock_timestamp()-interval '1 hour'")
			case "wrong TLS name":
				name = "wrong.fixture"
			case "closed pool":
				q := senderPool(t)
				repo = senderRepo(t, q)
				q.Close()
			case "cancelled":
				cancel()
			}
			p := testProcessor(t, func(w http.ResponseWriter, r *http.Request) {
				t.Error("unexpected HTTP on denied scope")
				w.WriteHeader(503)
			})
			_, err := sender(t, f, repo, gate, p.client(t, name), policy).Run(ctx, scope)
			if kind != "no grant" && kind != "closed pool" && kind != "cancelled" && kind != "revoked before dial" && err != nil {
				t.Fatal(err)
			}
			senderState(f, msg.MessageID, expected, attempts)
			if kind != "wrong TLS name" && p.listener.accepted.Load() != 0 {
				t.Fatal("network start bypassed authority or content check", starts.Load())
			}
		})
	}
}

func TestIntegrationSenderCommitFailure(t *testing.T) {
	f, repo := senderFixture(t)
	msg := f.receive("evt_sender_commit")
	f.exec(`CREATE FUNCTION hp_intake.fixture_sender_commit_failure() RETURNS trigger LANGUAGE plpgsql AS
		$$ BEGIN RAISE EXCEPTION 'synthetic commit failure'; END $$`)
	f.exec(`CREATE CONSTRAINT TRIGGER fixture_sender_commit_failure AFTER UPDATE ON hp_intake.outbox
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.state='ACKED')
		EXECUTE FUNCTION hp_intake.fixture_sender_commit_failure()`)
	drop := func() {
		f.exec("DROP TRIGGER IF EXISTS fixture_sender_commit_failure ON hp_intake.outbox")
		f.exec("DROP FUNCTION IF EXISTS hp_intake.fixture_sender_commit_failure()")
	}
	defer drop()
	var calls atomic.Int64
	p := testProcessor(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		kind := "APPLIED"
		if calls.Add(1) > 1 {
			kind = "DUPLICATE"
		}
		responseReceipt(w, msg.MessageID, kind)
	})
	w := sender(t, f, repo, testSendGate(allowSender), p.client(t, "processing.fixture"), senderPolicy)
	res, err := w.Run(context.Background(), senderScope)
	if !errors.Is(err, app.ErrUnavailable) || res.Acked != 0 || calls.Load() != 1 {
		t.Fatal("commit failure improperly acknowledged", res, err)
	}
	if f.scalar("SELECT count(*) FROM hp_intake.outbox WHERE state='CLAIMED' AND delivery_count=1") != 1 {
		t.Fatal("commit failure changed lease")
	}
	drop()
	f.exec("UPDATE hp_intake.outbox SET lease_until=clock_timestamp()-interval '1 second'")
	res, err = w.Run(context.Background(), senderScope)
	if err != nil || res.Acked != 1 || calls.Load() != 2 {
		t.Fatal("retry after commit failure", res, err)
	}
	senderState(f, msg.MessageID, app.Acked, 2)
}

func TestIntegrationSenderRetryBudgetAndReconnect(t *testing.T) {
	f, repo := senderFixture(t)
	first := f.receive("evt_sender_retry_budget")
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := target.Addr().String()
	target.Close()

	var received atomic.Int64
	var activeID atomic.Value
	activeID.Store(first.MessageID)
	p := testProcessor(t, func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Error(err)
		}
		received.Add(1)
		responseReceipt(w, activeID.Load().(string), "APPLIED")
	})
	unavailable, err := processing.New(address, "processing.fixture", p.roots, p.certificate)
	if err != nil {
		t.Fatal(err)
	}
	policy := senderPolicy
	policy.Attempts = 2
	run := func(transport *processing.Client, expected app.Summary) {
		t.Helper()
		got, err := sender(t, f, repo, testSendGate(allowSender), transport, policy).Run(context.Background(), senderScope)
		if err != nil || got != expected {
			t.Fatal(got, expected, err)
		}
	}
	run(unavailable, app.Summary{Retried: 1})
	senderState(f, first.MessageID, app.Pending, 1)
	// Move the already observed retry due time into the past rather than
	// waiting on the clock. Delivery still needs a new lease.
	f.exec("UPDATE hp_intake.outbox SET next_attempt_at=clock_timestamp()-interval '1 second'")
	run(unavailable, app.Summary{Held: 1})
	senderState(f, first.MessageID, app.Held, 2)

	second := f.receive("evt_sender_reconnect")
	activeID.Store(second.MessageID)
	run(unavailable, app.Summary{Retried: 1})
	f.exec("UPDATE hp_intake.outbox SET next_attempt_at=clock_timestamp()-interval '1 second' WHERE message_id=$1", second.MessageID)
	run(p.client(t, "processing.fixture"), app.Summary{Acked: 1})
	senderState(f, second.MessageID, app.Acked, 2)
	if received.Load() != 1 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 2 {
		t.Fatal("reconnection did not retain original identities")
	}
}
