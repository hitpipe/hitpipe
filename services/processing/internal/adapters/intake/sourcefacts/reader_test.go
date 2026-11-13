package sourcefacts

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

// Fixture gate only: NOT a shared runtime/epoch coordinator implementation.
type fixtureBoundary struct {
	deny    atomic.Bool
	calls   atomic.Int32
	noStart bool
	want    CallerContext
	bad     atomic.Bool
}

func (g *fixtureBoundary) Start(ctx context.Context, c CallerContext, start func() error) error {
	g.calls.Add(1)
	if c.WorkspaceID != g.want.WorkspaceID || c.ExpectedEpoch != g.want.ExpectedEpoch || !c.DeadlineAt.Equal(g.want.DeadlineAt) {
		g.bad.Store(true)
		return errors.New("scope")
	}
	if g.deny.Load() {
		return errors.New("fixture denied")
	}
	if g.noStart {
		return nil
	}
	return start()
}

type tlsFixture struct {
	ca    *x509.Certificate
	key   *ecdsa.PrivateKey
	roots *x509.CertPool
}

func certificates(t *testing.T) *tlsFixture {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT-77 synthetic test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return &tlsFixture{ca: ca, key: k, roots: roots}
}
func (f *tlsFixture) leaf(t *testing.T, name string, server bool) tls.Certificate {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		if name != "wrong-server.test" {
			template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, f.ca, &k.PublicKey, f.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}
}
func (f *tlsFixture) server(t *testing.T, h http.HandlerFunc) *httptest.Server {
	return f.serverConnections(t, h, nil)
}
func (f *tlsFixture) serverConnections(t *testing.T, h http.HandlerFunc, state func(net.Conn, http.ConnState)) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.Config.ConnState = state
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{f.leaf(t, "intake.test", true)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: f.roots, VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || len(cs.PeerCertificates[0].DNSNames) != 1 || cs.PeerCertificates[0].DNSNames[0] != "processing.test" {
			return errors.New("wrong service")
		}
		return nil
	}}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}
func (f *tlsFixture) transport(t *testing.T) *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: f.roots, Certificates: []tls.Certificate{f.leaf(t, "processing.test", false)}}}
}
func caller(e Expected) CallerContext {
	live := e.LiveMode
	return CallerContext{WorkspaceID: "99999999-9999-4999-8999-999999999999", CaseID: e.CaseID, SessionID: e.SessionID, ConnectionID: e.ConnectionID, AccountID: e.AccountID, LiveMode: &live, RequestID: "88888888-8888-4888-8888-888888888888", ExpectedEpoch: 2, DeadlineAt: time.Now().Add(10 * time.Second)}
}
func query(e Expected, p eligibility.Policy) evaluatepaidorder.Query {
	return evaluatepaidorder.Query{CaseID: e.CaseID, SessionID: e.SessionID, ConfigID: p.ConfigID, AccountID: e.AccountID, ConnectionID: e.ConnectionID, LiveMode: e.LiveMode}
}
func newReader(t *testing.T, s *httptest.Server, f *tlsFixture, c CallerContext, p eligibility.Policy, g *fixtureBoundary, bound int64) *Reader {
	t.Helper()
	g.want = c
	r, err := NewReader(s.URL, p, c, f.transport(t), g, bound)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func readError(t *testing.T, r *Reader, ctx context.Context, q evaluatepaidorder.Query, kind string) {
	t.Helper()
	got, err := r.ReadProjection(ctx, q)
	var e *ReadError
	if !errors.As(err, &e) || e.Kind != kind || got.Snapshot.Facts.CaseID != "" || got.Provenance.FactsID != "" {
		t.Fatalf("kind=%s got=%+v err=%v", kind, got, err)
	}
}

func TestHTTPSReadExactRequestAndProvenance(t *testing.T) {
	b, e, p, _ := inputs(t)
	c := caller(e)
	f := certificates(t)
	var calls atomic.Int32
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != "POST" || r.URL.Path != "/internal/v1/source-facts/read" || r.Header.Get("X-Request-ID") != c.RequestID || r.Header.Get("Content-Type") != "application/json" || r.TLS.Version != tls.VersionTLS13 {
			t.Error("wire operation mismatch")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		var m map[string]any
		if json.Unmarshal(raw, &m) != nil || len(m) != 6 {
			t.Error("wire fields")
		}
		if m["workspace_id"] != c.WorkspaceID || m["case_id"] != c.CaseID || m["session_id"] != c.SessionID || m["connection_id"] != c.ConnectionID || m["expected_epoch"] != float64(c.ExpectedEpoch) || m["deadline_at"] != c.DeadlineAt.Format(time.RFC3339Nano) {
			t.Errorf("caller context differs: %s", raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	g := &fixtureBoundary{}
	r := newReader(t, s, f, c, p, g, int64(len(b)+1024))
	got, err := r.ReadProjection(context.Background(), query(e, p))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || g.calls.Load() < 2 || g.bad.Load() {
		t.Fatal("missing actual start/scope")
	}
	if got.Provenance.FactsID != "33333333-3333-4333-8333-333333333333" || got.Provenance.Digest != "sha256:"+strings.Repeat("a", 64) || got.Provenance.ValidUntil != "2026-10-18T10:00:10Z" || got.Snapshot.ConfigID != p.ConfigID {
		t.Fatal("provenance/local pins")
	}
}
func TestMissingContextAndDependenciesNeverConnect(t *testing.T) {
	b, e, p, _ := inputs(t)
	c := caller(e)
	f := certificates(t)
	var calls atomic.Int32
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	g := &fixtureBoundary{want: c}
	tr := f.transport(t)
	for _, edit := range []func(*CallerContext){func(v *CallerContext) { v.WorkspaceID = "" }, func(v *CallerContext) { v.ExpectedEpoch = 0 }, func(v *CallerContext) { v.ExpectedEpoch = 9007199254740992 }, func(v *CallerContext) { v.DeadlineAt = time.Time{} }, func(v *CallerContext) { v.RequestID = "" }, func(v *CallerContext) { v.LiveMode = nil }, func(v *CallerContext) { v.ConnectionID = "11111111-1111-4111-8111-111111111111" }, func(v *CallerContext) { v.AccountID = "wrong" }, func(v *CallerContext) { v.SessionID = strings.Repeat("s", 256) }} {
		v := c
		edit(&v)
		if _, err := NewReader(s.URL, p, v, tr, g, int64(len(b)+1024)); err == nil {
			t.Fatal("invalid caller accepted")
		}
	}
	for _, makeReader := range []func() (*Reader, error){func() (*Reader, error) { return NewReader(s.URL, p, c, nil, g, 1024) }, func() (*Reader, error) { return NewReader(s.URL, p, c, tr, nil, 1024) }, func() (*Reader, error) { var absent *fixtureBoundary; return NewReader(s.URL, p, c, tr, absent, 1024) }, func() (*Reader, error) { return NewReader(s.URL, p, c, tr, g, 0) }} {
		if _, err := makeReader(); err == nil {
			t.Fatal("missing dependency accepted")
		}
	}
	r := newReader(t, s, f, c, p, g, int64(len(b)+1024))
	for _, edit := range []func(*evaluatepaidorder.Query){func(v *evaluatepaidorder.Query) { v.CaseID = "other" }, func(v *evaluatepaidorder.Query) { v.SessionID = "other" }, func(v *evaluatepaidorder.Query) { v.ConnectionID = "other" }, func(v *evaluatepaidorder.Query) { v.AccountID = "other" }, func(v *evaluatepaidorder.Query) { v.LiveMode = true }, func(v *evaluatepaidorder.Query) { v.ConfigID = "other" }} {
		q := query(e, p)
		edit(&q)
		readError(t, r, context.Background(), q, "CONTEXT")
	}
	c.DeadlineAt = time.Now().Add(-time.Second)
	r = newReader(t, s, f, c, p, g, 1024)
	readError(t, r, context.Background(), query(e, p), "CANCELED")
	if calls.Load() != 0 || g.calls.Load() != 0 {
		t.Fatal("invalid context reached start/HTTP")
	}
}
func TestMTLSAndActualSocketStart(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	var calls, connections atomic.Int32
	s := f.serverConnections(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}, func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			connections.Add(1)
		}
	})
	t.Run("deny before dial", func(t *testing.T) {
		c := caller(e)
		g := &fixtureBoundary{}
		g.deny.Store(true)
		r := newReader(t, s, f, c, p, g, 4096)
		readError(t, r, context.Background(), query(e, p), "START_DENIED")
		if connections.Load() != 0 {
			t.Fatal("denied dial started")
		}
	})
	t.Run("boundary does not start", func(t *testing.T) {
		c := caller(e)
		g := &fixtureBoundary{noStart: true}
		r := newReader(t, s, f, c, p, g, 4096)
		readError(t, r, context.Background(), query(e, p), "START_DENIED")
	})
	t.Run("missing certificate", func(t *testing.T) {
		c := caller(e)
		tr := f.transport(t)
		tr.TLSClientConfig.Certificates = nil
		if _, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096); err == nil {
			t.Fatal("no certificate accepted")
		}
	})
	t.Run("wrong client principal", func(t *testing.T) {
		c := caller(e)
		tr := f.transport(t)
		tr.TLSClientConfig.Certificates = []tls.Certificate{f.leaf(t, "delivery.test", false)}
		r, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096)
		if err != nil {
			t.Fatal(err)
		}
		readError(t, r, context.Background(), query(e, p), "TRANSPORT")
	})
	t.Run("wrong server CA", func(t *testing.T) {
		c := caller(e)
		tr := f.transport(t)
		tr.TLSClientConfig.RootCAs = certificates(t).roots
		r, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096)
		if err != nil {
			t.Fatal(err)
		}
		readError(t, r, context.Background(), query(e, p), "TRANSPORT")
	})
	t.Run("wrong server name", func(t *testing.T) {
		c := caller(e)
		tr := f.transport(t)
		tr.TLSClientConfig.ServerName = "elsewhere.test"
		if _, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096); err == nil {
			t.Fatal("wrong server name override accepted")
		}
	})
	t.Run("revoke after handshake verification before HTTP", func(t *testing.T) {
		c := caller(e)
		tr := f.transport(t)
		g := &fixtureBoundary{want: c}
		tr.TLSClientConfig.VerifyConnection = func(tls.ConnectionState) error { g.deny.Store(true); return nil }
		r, err := NewReader(s.URL, p, c, tr, g, 4096)
		if err != nil {
			t.Fatal(err)
		}
		readError(t, r, context.Background(), query(e, p), "START_DENIED")
	})
	if calls.Load() != 0 {
		t.Fatal("rejected requests reached HTTP handler")
	}
}

func TestResponseFailuresNeverRetryOrPublishProjection(t *testing.T) {
	b, e, p, _ := inputs(t)
	for _, tt := range []struct {
		name            string
		status          int
		body            []byte
		kind            string
		media, encoding string
		chunked         bool
	}{
		{"401", 401, b, "HTTP_STATUS", "application/json", "", false}, {"403", 403, b, "HTTP_STATUS", "application/json", "", false}, {"429", 429, b, "HTTP_STATUS", "application/json", "", false}, {"503", 503, b, "HTTP_STATUS", "application/json", "", false},
		{"malformed", 200, []byte(`{"facts_id":`), "MESSAGE", "application/json", "", false},
		{"case mismatch", 200, change(t, b, "case_id", "44444444-4444-4444-8444-444444444444"), "MESSAGE", "application/json", "", false},
		{"account mismatch", 200, change(t, b, "stripe_account_id", "other"), "MESSAGE", "application/json", "", false},
		{"connection mismatch", 200, change(t, b, "connection_id", "44444444-4444-4444-8444-444444444444"), "MESSAGE", "application/json", "", false},
		{"session mismatch", 200, change(t, b, "session_id", "other"), "MESSAGE", "application/json", "", false},
		{"mode mismatch", 200, change(t, b, "livemode", true), "MESSAGE", "application/json", "", false},
		{"OAS oversized field", 200, change(t, b, "session_id", strings.Repeat("s", 256)), "MESSAGE", "application/json", "", false},
		{"transport bound ContentLength", 200, []byte(strings.Repeat(" ", len(b)+2048)), "RESPONSE_BOUND", "application/json", "", false},
		{"transport bound chunked", 200, []byte(strings.Repeat(" ", len(b)+2048)), "RESPONSE_BOUND", "application/json", "", true},
		{"media", 200, b, "MESSAGE", "text/plain", "", false}, {"encoding", 200, b, "MESSAGE", "application/json", "gzip", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := certificates(t)
			var calls atomic.Int32
			s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", tt.media)
				if tt.encoding != "" {
					w.Header().Set("Content-Encoding", tt.encoding)
				}
				w.WriteHeader(tt.status)
				if tt.chunked {
					w.(http.Flusher).Flush()
				}
				w.Write(tt.body)
			})
			c := caller(e)
			r := newReader(t, s, f, c, p, &fixtureBoundary{}, int64(len(b)+1024))
			got, err := r.ReadProjection(context.Background(), query(e, p))
			var detail *ReadError
			if !errors.As(err, &detail) || detail.Kind != tt.kind || got.Snapshot.Facts.CaseID != "" || got.Provenance.FactsID != "" || calls.Load() != 1 {
				t.Fatalf("%+v %v calls=%d", got, err, calls.Load())
			}
			if tt.status != 200 && detail.StatusCode != tt.status {
				t.Fatal("HTTP statuses conflated")
			}
			if strings.Contains(err.Error(), "https://") || strings.Contains(err.Error(), string(tt.body)) {
				t.Fatal("unsafe diagnostic")
			}
		})
	}
}
func TestRedirectAndDroppedResponseDoNotReplay(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	var targetCalls, sourceCalls atomic.Int32
	target := f.server(t, func(w http.ResponseWriter, r *http.Request) { targetCalls.Add(1); w.Write(b) })
	source := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		sourceCalls.Add(1)
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	})
	c := caller(e)
	r := newReader(t, source, f, c, p, &fixtureBoundary{}, 4096)
	readError(t, r, context.Background(), query(e, p), "HTTP_STATUS")
	if sourceCalls.Load() != 1 || targetCalls.Load() != 0 {
		t.Fatal("redirect followed")
	}
	var drops atomic.Int32
	dropped := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		drops.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		conn.Close()
	})
	r = newReader(t, dropped, f, caller(e), p, &fixtureBoundary{}, 4096)
	readError(t, r, context.Background(), query(e, p), "TRANSPORT")
	if drops.Load() != 1 {
		t.Fatal("POST replayed")
	}
}
func TestHTTPSCompositionUsesExistingFinancialEvaluator(t *testing.T) {
	b, e, p, now := inputs(t)
	for _, tt := range []struct {
		name   string
		body   []byte
		now    time.Time
		status eligibility.Status
		reason string
	}{
		{"normal", b, now, eligibility.Valid, "FINANCIAL_ELIGIBILITY_ONLY"},
		{"refund", change(t, b, "amount_refunded", 1), now, eligibility.Ineligible, "REFUND"},
		{"unknown", change(t, b, "amount_received", nil), now, eligibility.Incomplete, "UNKNOWN_FACT"},
		{"expired", b, now.Add(5 * time.Second), eligibility.RefreshRequired, "PROOF_EXPIRED"},
		{"malformed", []byte(`{}`), now, eligibility.Incomplete, "READER_UNAVAILABLE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := certificates(t)
			var calls atomic.Int32
			s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Write(tt.body)
			})
			c := caller(e)
			r := newReader(t, s, f, c, p, &fixtureBoundary{}, int64(len(b)+1024))
			got := evaluatepaidorder.EvaluatePaidOrder(context.Background(), evaluatepaidorder.Request{Policy: p, CaseID: e.CaseID, SessionID: e.SessionID, Now: tt.now}, r)
			if got.Status != tt.status || got.Reason != tt.reason || calls.Load() != 1 {
				t.Fatalf("%+v calls=%d", got, calls.Load())
			}
		})
	}
}
func TestCancelAndTimeoutJoinWithoutStaleResult(t *testing.T) {
	b, e, p, _ := inputs(t)
	for _, mode := range []string{"cancel before", "cancel headers", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			f := certificates(t)
			started := make(chan struct{})
			finished := make(chan struct{})
			var calls atomic.Int32
			s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				close(started)
				<-r.Context().Done()
				close(finished)
			})
			c := caller(e)
			if mode == "deadline" {
				c.DeadlineAt = time.Now().Add(200 * time.Millisecond)
			}
			reader := newReader(t, s, f, c, p, &fixtureBoundary{}, int64(len(b)+1024))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel before" {
				cancel()
			}
			done := make(chan error, 1)
			go func() {
				projection, err := reader.ReadProjection(ctx, query(e, p))
				if projection.Snapshot.Facts.CaseID != "" || projection.Provenance.FactsID != "" {
					done <- errors.New("stale projection")
					return
				}
				done <- err
			}()
			if mode == "cancel headers" {
				select {
				case <-started:
					cancel()
				case <-time.After(5 * time.Second):
					t.Fatal("handler did not start")
				}
			}
			select {
			case err := <-done:
				var detail *ReadError
				if !errors.As(err, &detail) || detail.Kind != "CANCELED" {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("reader goroutine did not finish")
			}
			if mode != "cancel before" {
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("server did not see cancellation")
				}
				if calls.Load() != 1 {
					t.Fatal("cancel replay")
				}
			} else if calls.Load() != 0 {
				t.Fatal("canceled context reached server")
			}
		})
	}
}
func TestConcurrentReadsOwnValues(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	var calls atomic.Int32
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	c := caller(e)
	r := newReader(t, s, f, c, p, &fixtureBoundary{}, int64(len(b)+1024))
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := r.ReadProjection(context.Background(), query(e, p))
			if err != nil {
				t.Error(err)
				return
			}
			if got.Provenance.FactsID == "" || *got.Snapshot.Facts.AmountRefunded != 0 {
				t.Error("lost/shared result")
			}
			*got.Snapshot.Facts.AmountRefunded = 17
		}()
	}
	wg.Wait()
	if calls.Load() != 8 {
		t.Fatal(calls.Load())
	}
}

func TestEstablishedScopeMismatchDeniedByInjectedBoundary(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	base := caller(e)
	var calls atomic.Int32
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.Write(b) })
	for _, alter := range []func(*CallerContext){func(c *CallerContext) { c.WorkspaceID = "77777777-7777-4777-8777-777777777777" }, func(c *CallerContext) { c.ExpectedEpoch++ }} {
		c := base
		alter(&c)
		g := &fixtureBoundary{want: base}
		r, err := NewReader(s.URL, p, c, f.transport(t), g, 4096)
		if err != nil {
			t.Fatal(err)
		}
		readError(t, r, context.Background(), query(e, p), "START_DENIED")
	}
	if calls.Load() != 0 {
		t.Fatal("mismatched established scope reached server")
	}
}
func TestCanceledReuseCannotReturnPreviousProjection(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	var calls atomic.Int32
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	c := caller(e)
	r := newReader(t, s, f, c, p, &fixtureBoundary{}, 4096)
	first, err := r.ReadProjection(context.Background(), query(e, p))
	if err != nil || first.Provenance.FactsID == "" {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	readError(t, r, ctx, query(e, p), "CANCELED")
	if calls.Load() != 1 {
		t.Fatal("canceled reuse started network")
	}
}

func TestServerCertificateSANMismatch(t *testing.T) {
	_, e, p, _ := inputs(t)
	f := certificates(t)
	var calls atomic.Int32
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{f.leaf(t, "wrong-server.test", true)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: f.roots}
	s.StartTLS()
	defer s.Close()
	c := caller(e)
	r := newReader(t, s, f, c, p, &fixtureBoundary{}, 4096)
	readError(t, r, context.Background(), query(e, p), "TRANSPORT")
	if calls.Load() != 0 {
		t.Fatal("wrong SAN server received HTTP request")
	}
}
func TestTransportRetainsExplicitDialerAndRejectsUnsafeTLS(t *testing.T) {
	b, e, p, _ := inputs(t)
	f := certificates(t)
	s := f.server(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	c := caller(e)
	var dials atomic.Int32
	tr := f.transport(t)
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	r, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Read(context.Background(), query(e, p)); err != nil {
		t.Fatal(err)
	}
	if dials.Load() != 1 {
		t.Fatal("injected dialer not preserved")
	}
	for _, change := range []func(*http.Transport){func(t *http.Transport) { t.TLSClientConfig.InsecureSkipVerify = true }, func(t *http.Transport) { t.TLSClientConfig.MinVersion = tls.VersionTLS12 }, func(t *http.Transport) { t.TLSClientConfig.RootCAs = nil }, func(t *http.Transport) { t.Proxy = http.ProxyFromEnvironment }} {
		tr := f.transport(t)
		change(tr)
		if _, err := NewReader(s.URL, p, c, tr, &fixtureBoundary{want: c}, 4096); err == nil {
			t.Fatal("unsafe transport accepted")
		}
	}
}
