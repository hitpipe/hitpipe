//go:build integration

package sourcefacts_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	content "github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentfacts"
	inbound "github.com/hitpipe/hitpipe/services/intake/internal/adapters/http/sourcefacts"
	pg "github.com/hitpipe/hitpipe/services/intake/internal/adapters/postgres/facts"
	stripe "github.com/hitpipe/hitpipe/services/intake/internal/adapters/stripe"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"github.com/jackc/pgx/v5"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const ws = "11111111-1111-4111-8111-111111111111"
const other = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
const conn = "22222222-2222-4222-8222-222222222222"
const caseID = "33333333-3333-4333-8333-333333333333"
const requestID = "44444444-4444-4444-8444-444444444444"
const secret = "SYNTHETIC_CREDENTIAL_NEVER_RETAIN"

type authority struct {
	mu       sync.Mutex
	bindings map[string]app.Binding
	denied   bool
	starts   int
}

func (a *authority) Current(ctx context.Context, q sf.Request) (app.Binding, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	b, ok := a.bindings[q.WorkspaceID]
	if !ok || a.denied || q.CaseID != caseID || q.SessionID != "cs_synthetic" {
		return app.Binding{}, app.Fail("context")
	}
	return b.Clone(), nil
}
func (a *authority) Start(ctx context.Context, b app.Binding, action func() error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.starts++
	if a.denied || !b.Same(a.bindings[b.WorkspaceID]) || ctx.Err() != nil {
		return app.Fail("context")
	}
	return action()
}
func (a *authority) revoke() { a.mu.Lock(); defer a.mu.Unlock(); a.denied = true }

type credentials struct {
	a    *authority
	hook func()
}

func (k credentials) ReadKey(context.Context, app.Binding) ([]byte, error) {
	if k.hook != nil {
		k.hook()
	}
	return []byte(secret), nil
}

type fixture struct {
	t           *testing.T
	authority   *authority
	repo        *pg.Repository
	content     *content.Store
	provider    *httptest.Server
	service     *app.Service
	server      *inbound.Server
	client      *http.Client
	url, root   string
	clockOffset atomic.Int64
	requests    atomic.Int64
	mu          sync.Mutex
	events      []string
	mutate      func(string, map[string]any)
	statusPath  string
	status      int
	after       func(string)
	bytesBound  int64
	pages       int
	keyHook     func()
	delayPath   string
}

func (f *fixture) now() time.Time { return time.Now().Add(time.Duration(f.clockOffset.Load())).UTC() }
func query(t *testing.T, role, sql string, args ...any) pgx.Rows {
	t.Helper()
	dsn := os.Getenv("HIT117_MIGRATOR_DSN")
	if role == "app" {
		dsn = os.Getenv("HIT117_APP_DSN")
	}
	c, e := pgx.Connect(context.Background(), dsn)
	if e != nil {
		t.Fatal("fixture connection")
	}
	t.Cleanup(func() { c.Close(context.Background()) })
	rows, e := c.Query(context.Background(), sql, args...)
	if e != nil {
		t.Fatal("fixture query")
	}
	return rows
}
func seed(t *testing.T) {
	t.Helper()
	c, e := pgx.Connect(context.Background(), os.Getenv("HIT117_MIGRATOR_DSN"))
	if e != nil {
		t.Fatal("database fixture unavailable")
	}
	defer c.Close(context.Background())
	for _, workspace := range []string{ws, other} {
		tx, e := c.Begin(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,true)`, workspace)
		if e == nil {
			_, e = tx.Exec(context.Background(), `INSERT INTO hp_intake.source_connections VALUES($1,$2,'acct_synthetic',false,1,'READY',now(),NULL) ON CONFLICT(workspace_id,connection_id) DO UPDATE SET security_epoch=1,health='READY',revoked_at=NULL,verified_at=now(),account_id='acct_synthetic',livemode=false`, workspace, conn)
		}
		if e != nil {
			t.Fatal("seed failed")
		}
		if tx.Commit(context.Background()) != nil {
			t.Fatal("seed commit")
		}
	}
}
func cert(t *testing.T, ca *x509.Certificate, key *ecdsa.PrivateKey, dns string, server, expired bool) (tls.Certificate, *x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tmpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: dns}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, DNSNames: []string{dns}}
	if expired {
		tmpl.NotBefore = time.Now().Add(-2 * time.Hour)
		tmpl.NotAfter = time.Now().Add(-time.Hour)
	}
	if server {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	if ca == nil {
		tmpl.IsCA = true
		tmpl.ExtKeyUsage = nil
		tmpl.BasicConstraintsValid = true
		tmpl.KeyUsage |= x509.KeyUsageCertSign
		ca = tmpl
		key = k
	}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	pk, _ := x509.MarshalECPrivateKey(k)
	pair, e := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: pk}))
	if e != nil {
		t.Fatal(e)
	}
	parsed, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	return pair, parsed, k
}
func newFixture(t *testing.T, setup func(*fixture)) *fixture {
	t.Helper()
	if os.Getenv("HIT117_APP_DSN") == "" || os.Getenv("HIT117_MIGRATOR_DSN") == "" {
		t.Fatal("run tools/check_sourcefacts.py; integration DSNs required")
	}
	seed(t)
	live := false
	b := app.Binding{WorkspaceID: ws, ConnectionID: conn, AccountID: "acct_synthetic", CredentialRef: "fixture-only", APIVersion: "2026-10-28.endive", Revision: "fixture-revision-1", Epoch: 1, LiveMode: &live, Ready: true}
	bo := b.Clone()
	bo.WorkspaceID = other
	a := &authority{bindings: map[string]app.Binding{ws: b, other: bo}}
	f := &fixture{t: t, authority: a, bytesBound: 65536, pages: 4}
	if setup != nil {
		setup(f)
	}
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if os.Chmod(root, 0700) != nil {
		t.Fatal("chmod")
	}
	f.root = root
	f.content, e = content.New(root, f.now)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(f.content.Close)
	f.repo, e = pg.New(context.Background(), os.Getenv("HIT117_APP_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(f.repo.Close)
	f.provider = httptest.NewUnstartedServer(http.HandlerFunc(f.stripe))
	f.provider.Config.ErrorLog = log.New(io.Discard, "", 0)
	f.provider.StartTLS()
	t.Cleanup(f.provider.Close)
	roots := x509.NewCertPool()
	roots.AddCert(f.provider.Certificate())
	provider, e := stripe.New(f.provider.URL, &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}, credentials{a, f.keyHook}, f.now, stripe.Bounds{ResponseBytes: f.bytesBound, Pages: f.pages, Items: 300})
	if e != nil {
		t.Fatal(e)
	}
	f.service, e = app.New(a, f.repo, f.content, provider, f.now, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	_, ca, key := cert(t, nil, nil, "fixture-ca", false, false)
	server, _, _ := cert(t, ca, key, "intake.test", true, false)
	processing, _, _ := cert(t, ca, key, "processing.test", false, false)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	f.server, e = inbound.New("127.0.0.1:0", "processing.test", &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{server}, ClientCAs: pool}, f.service, 8192)
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	f.url = "https://" + listener.Addr().String() + "/internal/v1/source-facts/read"
	go f.server.Serve(listener)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		f.server.Close(ctx)
	})
	f.client = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{processing}}}, Timeout: 3 * time.Second}
	t.Cleanup(func() { f.client.CloseIdleConnections() })
	for _, v := range []struct {
		name             string
		foreign, expired bool
	}{{"other.test", false, false}, {"processing.test", false, true}, {"processing.test", true, false}} {
		var c tls.Certificate
		if v.foreign {
			c, _, _ = cert(t, nil, nil, v.name, false, false)
		} else {
			c, _, _ = cert(t, ca, key, v.name, false, v.expired)
		}
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{c}}}, Timeout: time.Second}
		r, _ := http.NewRequest("POST", f.url, strings.NewReader("{}"))
		_, err := client.Do(r)
		client.CloseIdleConnections()
		if err == nil {
			t.Fatal("unauthorized certificate accepted")
		}
	}
	return f
}
func (f *fixture) request() sf.Request {
	return sf.Request{WorkspaceID: ws, ConnectionID: conn, SessionID: "cs_synthetic", CaseID: caseID, ExpectedEpoch: 1, DeadlineAt: time.Now().Add(5 * time.Second).UTC()}
}
func (f *fixture) callBytes(raw []byte) (int, []byte) {
	f.t.Helper()
	r, _ := http.NewRequest("POST", f.url, bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Request-ID", requestID)
	resp, e := f.client.Do(r)
	if e != nil {
		f.t.Errorf("authorized HTTPS failed: %v", e)
		return 0, nil
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil {
		f.t.Fatal("response read")
	}
	if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte("RAW_CUSTOMER_MARKER")) {
		f.t.Fatal("sensitive response")
	}
	return resp.StatusCode, b
}
func (f *fixture) call(q sf.Request) (int, []byte) { b, _ := json.Marshal(q); return f.callBytes(b) }
func (f *fixture) stripe(w http.ResponseWriter, r *http.Request) {
	f.requests.Add(1)
	if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+secret || r.Header.Get("Stripe-Account") != "acct_synthetic" || r.Header.Get("Stripe-Version") != "2026-10-28.endive" {
		f.t.Error("unapproved provider request")
		w.WriteHeader(403)
		return
	}
	f.mu.Lock()
	f.events = append(f.events, r.URL.RequestURI())
	f.mu.Unlock()
	if r.URL.Path == f.delayPath {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
	if r.URL.Path == f.statusPath {
		w.Header().Set("Location", f.provider.URL+"/forbidden")
		w.WriteHeader(f.status)
		return
	}
	m := fixtureObject(r.URL.Path)
	if m == nil {
		f.t.Error("unexpected provider path", r.URL.Path)
		w.WriteHeader(404)
		return
	}
	if f.mutate != nil {
		f.mutate(r.URL.Path, m)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Stripe-Version", "2026-10-28.endive")
	json.NewEncoder(w).Encode(m)
	if f.after != nil {
		f.after(r.URL.Path)
	}
}
func fixtureObject(path string) map[string]any {
	switch path {
	case "/v1/account":
		return map[string]any{"id": "acct_synthetic", "object": "account"}
	case "/v1/checkout/sessions/cs_synthetic":
		return map[string]any{"id": "cs_synthetic", "object": "checkout.session", "livemode": false, "mode": "payment", "payment_status": "paid", "currency": "usd", "amount_total": int64(1200), "created": int64(1700000000), "payment_intent": "pi_synthetic", "customer_email": "RAW_CUSTOMER_MARKER"}
	case "/v1/checkout/sessions/cs_synthetic/line_items":
		return list(map[string]any{"id": "li_synthetic", "object": "item", "currency": "usd", "quantity": int64(1), "price": map[string]any{"id": "price_synthetic", "object": "price"}})
	case "/v1/prices/price_synthetic":
		return map[string]any{"id": "price_synthetic", "object": "price", "livemode": false, "currency": "usd", "type": "one_time", "billing_scheme": "per_unit", "custom_unit_amount": nil, "recurring": nil, "transform_quantity": nil, "unit_amount": int64(1200)}
	case "/v1/payment_intents/pi_synthetic":
		return map[string]any{"id": "pi_synthetic", "object": "payment_intent", "livemode": false, "status": "succeeded", "currency": "usd", "amount_received": int64(1200), "latest_charge": "ch_synthetic"}
	case "/v1/charges/ch_synthetic":
		return map[string]any{"id": "ch_synthetic", "object": "charge", "livemode": false, "currency": "usd", "payment_intent": "pi_synthetic", "paid": true, "status": "succeeded", "captured": true, "amount_captured": int64(1200), "amount_refunded": int64(0), "disputed": false}
	case "/v1/refunds", "/v1/disputes":
		return list()
	}
	return nil
}
func list(items ...map[string]any) map[string]any {
	data := make([]map[string]any, len(items))
	copy(data, items)
	return map[string]any{"object": "list", "data": data, "has_more": false}
}
func TestIntegrationHTTPSPostgresContentAndRecovery(t *testing.T) {
	f := newFixture(t, nil)
	q := f.request()
	status, body := f.call(q)
	if status != 200 {
		t.Fatalf("status %d: %s", status, body)
	}
	facts, e := sf.Decode(body)
	if e != nil || !facts.Complete || facts.AmountReceived == nil || *facts.AmountReceived != 1200 || facts.AmountRefunded == nil || *facts.AmountRefunded != 0 || facts.Disputed == nil || *facts.Disputed {
		t.Fatal("facts mismatch", e)
	}
	if facts.ValidUntil.After(q.DeadlineAt) || facts.ValidUntil.After(facts.OldestObservedAt.Add(10*time.Second)) {
		t.Fatal("validity stretched")
	}
	if f.requests.Load() != 8 {
		t.Fatal("unexpected requests", f.requests.Load())
	}
	got, e := f.service.Load(context.Background(), q, facts.FactsID)
	if e != nil || !bytes.Equal(got.Body, body) {
		t.Fatal("persistent readback", e)
	}
	newrepo, e := pg.New(context.Background(), os.Getenv("HIT117_APP_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	defer newrepo.Close()
	newService, e := app.New(f.authority, newrepo, f.content, noopProvider{}, f.now, time.Hour)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = newService.Load(context.Background(), q, facts.FactsID); e != nil {
		t.Fatal("new instance recovery", e)
	}
	wrong := q
	wrong.WorkspaceID = other
	if _, e = f.service.Load(context.Background(), wrong, facts.FactsID); e == nil {
		t.Fatal("cross workspace read")
	}
	f.clockOffset.Store(int64(11 * time.Second))
	if _, e = f.service.Load(context.Background(), q, facts.FactsID); e == nil {
		t.Fatal("expired facts")
	}
}

type noopProvider struct{}

func (noopProvider) Read(context.Context, sf.Request, app.Binding, app.Guard) (sf.Facts, error) {
	return sf.Facts{}, app.Fail("unused")
}
func TestIntegrationStrictRequestDenialsBeforeNetwork(t *testing.T) {
	f := newFixture(t, nil)
	base, _ := json.Marshal(f.request())
	tests := map[string][]byte{"unknown": []byte(strings.Replace(string(base), `"case_id":`, `"extra":true,"case_id":`, 1)), "duplicate": []byte(strings.Replace(string(base), `"case_id":`, `"case_id":"`+caseID+`","case_id":`, 1)), "null": []byte(strings.Replace(string(base), `"expected_epoch":1`, `"expected_epoch":null`, 1)), "uuid": []byte(strings.Replace(string(base), caseID, "bad", 1)), "epoch": []byte(strings.Replace(string(base), `"expected_epoch":1`, `"expected_epoch":0`, 1)), "fraction": []byte(strings.Replace(string(base), `"expected_epoch":1`, `"expected_epoch":1.5`, 1)), "unknown-scope": []byte(strings.Replace(string(base), ws, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", 1)), "oversized": bytes.Repeat([]byte(" "), 8193)}
	for name, b := range tests {
		t.Run(name, func(t *testing.T) {
			status, _ := f.callBytes(b)
			if status == 200 {
				t.Fatal("accepted")
			}
		})
	}
	q := f.request()
	q.DeadlineAt = time.Now().Add(-time.Second)
	if status, _ := f.call(q); status == 200 {
		t.Fatal("expired request")
	}
	if f.requests.Load() != 0 {
		t.Fatal("denial caused network")
	}
}
func TestIntegrationUnknownAndNegativeFactsNeverInvented(t *testing.T) {
	tests := []struct {
		name, path, key string
		value           any
		check           func(sf.Facts) bool
	}{
		{"missing-capture", "/v1/charges/ch_synthetic", "amount_captured", nil, func(f sf.Facts) bool { return !f.Complete && f.AmountCaptured == nil }},
		{"null-mode", "/v1/checkout/sessions/cs_synthetic", "mode", nil, func(f sf.Facts) bool { return !f.Complete && f.Mode == nil }},
		{"unknown-payment-status", "/v1/checkout/sessions/cs_synthetic", "payment_status", "future", func(f sf.Facts) bool { return !f.Complete && f.PaymentStatus != nil && *f.PaymentStatus == "UNKNOWN" }},
		{"unpaid", "/v1/checkout/sessions/cs_synthetic", "payment_status", "unpaid", func(f sf.Facts) bool { return f.Complete && f.PaymentStatus != nil && *f.PaymentStatus == "unpaid" }},
		{"uncaptured", "/v1/charges/ch_synthetic", "captured", false, func(f sf.Facts) bool { return f.Complete && f.ChargeCaptured != nil && !*f.ChargeCaptured }},
		{"recurring-price", "/v1/prices/price_synthetic", "type", "recurring", func(f sf.Facts) bool { return !f.Complete && f.PriceID == nil }},
		{"different-currency", "/v1/payment_intents/pi_synthetic", "currency", "eur", func(f sf.Facts) bool { return f.Complete && f.AllCurrenciesEqual != nil && !*f.AllCurrenciesEqual }},
		{"no-linked-payment", "/v1/checkout/sessions/cs_synthetic", "payment_intent", nil, func(f sf.Facts) bool {
			return !f.Complete && f.PaymentIntentStatus == "UNKNOWN" && f.AmountReceived == nil
		}},
	}
	for _, x := range tests {
		t.Run(x.name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.mutate = func(path string, m map[string]any) {
					if path == x.path {
						if x.value == nil {
							delete(m, x.key)
						} else {
							m[x.key] = x.value
						}
					}
				}
			})
			status, body := f.call(f.request())
			if status != 200 {
				t.Fatal("not a bounded snapshot", status)
			}
			fact, e := sf.Decode(body)
			if e != nil || !x.check(fact) {
				t.Fatal("invented facts", e)
			}
		})
	}
}
func TestIntegrationProviderIdentityAndContextMismatch(t *testing.T) {
	for _, x := range []struct {
		name, path, key string
		v               any
	}{{"account", "/v1/account", "id", "acct_other"}, {"mode", "/v1/checkout/sessions/cs_synthetic", "livemode", true}, {"unknown-mode", "/v1/payment_intents/pi_synthetic", "livemode", nil}, {"session", "/v1/checkout/sessions/cs_synthetic", "id", "cs_other"}, {"payment-link", "/v1/charges/ch_synthetic", "payment_intent", "pi_other"}} {
		t.Run(x.name, func(t *testing.T) {
			f := newFixture(t, func(f *fixture) {
				f.mutate = func(p string, m map[string]any) {
					if p == x.path {
						m[x.key] = x.v
					}
				}
			})
			if status, _ := f.call(f.request()); status == 200 {
				t.Fatal("mismatch accepted")
			}
		})
	}
}
func TestIntegrationNetworkFailuresNoRedirectOrRetry(t *testing.T) {
	for _, status := range []int{302, 307, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newFixture(t, func(f *fixture) { f.statusPath = "/v1/account"; f.status = status })
			got, _ := f.call(f.request())
			if got == 200 || f.requests.Load() != 1 || (status == 429 && got != 429) {
				t.Fatal("retry/redirect/false success", got, f.requests.Load())
			}
		})
	}
	t.Run("body-bound", func(t *testing.T) {
		f := newFixture(t, func(f *fixture) { f.bytesBound = 20 })
		if got, _ := f.call(f.request()); got == 200 || f.requests.Load() != 1 {
			t.Fatal("bound")
		}
	})
	t.Run("timeout", func(t *testing.T) {
		f := newFixture(t, func(f *fixture) { f.delayPath = "/v1/account" })
		if got, _ := f.call(f.request()); got == 200 || f.requests.Load() != 1 {
			t.Fatal("timeout")
		}
	})
}
func TestIntegrationPaginationCannotOverstateCompleteness(t *testing.T) {
	f := newFixture(t, func(f *fixture) {
		f.pages = 1
		f.mutate = func(p string, m map[string]any) {
			if strings.HasSuffix(p, "line_items") {
				m["has_more"] = true
			}
		}
	})
	status, body := f.call(f.request())
	fact, e := sf.Decode(body)
	if status != 200 || e != nil || fact.Complete || fact.LineItemsComplete || fact.LineItemCount != nil || fact.PriceID != nil {
		t.Fatal("truncated list overstated", status, e)
	}
}
func TestIntegrationActualStartGateAndChangesBetweenReads(t *testing.T) {
	t.Run("revoke-after-credential-before-connect", func(t *testing.T) {
		f := newFixture(t, func(f *fixture) { f.keyHook = func() { f.authority.revoke() } })
		status, _ := f.call(f.request())
		if status == 200 || f.requests.Load() != 0 {
			t.Fatal("revoked actual start")
		}
	})
	t.Run("scope-changed-between-reads", func(t *testing.T) {
		f := newFixture(t, func(f *fixture) {
			f.after = func(p string) {
				if p == "/v1/account" {
					f.authority.revoke()
				}
			}
		})
		status, _ := f.call(f.request())
		if status == 200 || f.requests.Load() != 1 {
			t.Fatal("stale scope reused", f.requests.Load())
		}
	})
	t.Run("protected-response-start", func(t *testing.T) {
		f := newFixture(t, nil)
		r, e := f.service.Read(context.Background(), f.request())
		if e != nil {
			t.Fatal(e)
		}
		f.authority.revoke()
		called := false
		e = f.service.Respond(context.Background(), r, func() error { called = true; return nil })
		if e == nil || called {
			t.Fatal("response started after deny")
		}
	})
}
func TestIntegrationConcurrentWorkspaceIsolationAndNoIngressWrites(t *testing.T) {
	f := newFixture(t, nil)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := f.request()
			if i%2 != 0 {
				q.WorkspaceID = other
			}
			status, body := f.call(q)
			if status != 200 {
				t.Error("concurrent read", status)
				return
			}
			v, e := sf.Decode(body)
			if e != nil {
				t.Error("decode")
				return
			}
			wrong := q
			if wrong.WorkspaceID == ws {
				wrong.WorkspaceID = other
			} else {
				wrong.WorkspaceID = ws
			}
			if _, e = f.service.Load(context.Background(), wrong, v.FactsID); e == nil {
				t.Error("foreign facts visible")
			}
		}(i)
	}
	wg.Wait()
	for _, workspace := range []string{ws, other} {
		c, e := pgx.Connect(context.Background(), os.Getenv("HIT117_APP_DSN"))
		if e != nil {
			t.Fatal("query connect")
		}
		if _, e = c.Exec(context.Background(), `SELECT set_config('hp.workspace_id',$1,false)`, workspace); e != nil {
			t.Fatal("scope")
		}
		var signals, outbox int
		e = c.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM hp_intake.signals),(SELECT count(*) FROM hp_intake.outbox)`).Scan(&signals, &outbox)
		c.Close(context.Background())
		if e != nil || signals != 0 || outbox != 0 {
			t.Fatal("ingress writes")
		}
	}
}
