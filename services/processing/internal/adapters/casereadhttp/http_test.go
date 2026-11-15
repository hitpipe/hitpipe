package casereadhttp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

const ws = "11111111-1111-4111-8111-111111111111"

type clock struct{}

func (clock) Now() time.Time { return time.Now() }

// These are explicit unit fixtures, not an IAM/runtime implementation.
type authority struct {
	grant readcases.Grant
	err   error
	calls int
}

func (a *authority) Authorize(ctx context.Context, r readcases.Request) (readcases.Grant, error) {
	a.calls++
	return a.grant, a.err
}

type gate struct {
	mu    sync.Mutex
	phase string
	calls int
}

func (g *gate) Start(ctx context.Context, a readcases.Grant, phase string, fn func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls++
	if ctx.Err() != nil || phase == g.phase {
		return readcases.ErrDenied
	}
	return fn(ctx)
}

type repository struct {
	calls int
	err   error
}

func (r *repository) Read(ctx context.Context, q readcases.Request, g readcases.Grant) (any, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return readcases.Cases{Items: []readcases.CaseSummary{}, SnapshotAt: time.Now().UTC()}, nil
}
func components() (readcases.Service, *authority, *gate, *repository) {
	a := &authority{grant: readcases.Grant{WorkspaceID: ws, PrincipalID: "22222222-2222-4222-8222-222222222222", Epoch: 1, ExpiresAt: time.Now().Add(time.Minute)}}
	g := &gate{}
	r := &repository{}
	return readcases.Service{Authority: a, Boundary: g, Clock: clock{}, Repository: r}, a, g, r
}

type pki struct {
	roots *x509.CertPool
	ca    *x509.Certificate
	key   *ecdsa.PrivateKey
	der   []byte
}

func newPKI(t *testing.T) pki {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "synthetic owner test CA"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	ca, e := x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return pki{roots, ca, key, der}
}
func (p pki) cert(t *testing.T, name string, server bool, expired bool) tls.Certificate {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	if e != nil {
		t.Fatal(e)
	}
	tmpl := &x509.Certificate{SerialNumber: serial, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	}
	if expired {
		tmpl.NotAfter = time.Now().Add(-time.Minute)
	}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, p.ca, &key.PublicKey, p.key)
	if e != nil {
		t.Fatal(e)
	}
	keyDER, e := x509.MarshalECPrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}))
	if e != nil {
		t.Fatal(e)
	}
	return cert
}
func fixture(t *testing.T, s readcases.Service) (*httptest.Server, pki) {
	t.Helper()
	p := newPKI(t)
	config := &tls.Config{Certificates: []tls.Certificate{p.cert(t, "processing.private", true, false)}, ClientCAs: p.roots}
	server, e := NewServer("127.0.0.1:0", config, s, "bff.private")
	if e != nil {
		t.Fatal(e)
	}
	ts := httptest.NewUnstartedServer(server.Handler)
	ts.TLS = server.TLSConfig
	ts.Config.ErrorLog = log.New(io.Discard, "", 0)
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts, p
}
func client(p pki, cert *tls.Certificate) *http.Client {
	cfg := &tls.Config{RootCAs: p.roots, MinVersion: tls.VersionTLS13}
	if cert != nil {
		cfg.Certificates = []tls.Certificate{*cert}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func request(t *testing.T, c *http.Client, url, session string, extra map[string]string) (int, []byte, error) {
	t.Helper()
	r, e := http.NewRequest("GET", url, nil)
	if e != nil {
		t.Fatal(e)
	}
	if session != "" {
		r.Header.Set("X-HitPipe-Session", session)
	}
	for k, v := range extra {
		r.Header.Set(k, v)
	}
	res, e := c.Do(r)
	if e != nil {
		return 0, nil, e
	}
	defer res.Body.Close()
	b, e := io.ReadAll(res.Body)
	return res.StatusCode, b, e
}

func TestRealMTLSAndFreshReadBoundaries(t *testing.T) {
	for _, name := range []string{"ok", "no cert", "foreign cert", "untrusted cert", "expired cert", "missing session", "forged authority", "stale grant", "wrong workspace", "closed SQL gate", "revoked before response", "denied session", "database error", "invalid query"} {
		t.Run(name, func(t *testing.T) {
			s, a, g, repo := components()
			ts, p := fixture(t, s)
			cert := p.cert(t, "bff.private", false, false)
			var presented *tls.Certificate = &cert
			session := "synthetic opaque session"
			extra := map[string]string{}
			path := "/api/v1/workspaces/" + ws + "/cases"
			want := 200
			reads := 1
			switch name {
			case "no cert":
				presented = nil
				want = 0
				reads = 0
			case "foreign cert":
				cert = p.cert(t, "delivery.private", false, false)
				want = 403
				reads = 0
			case "untrusted cert":
				cert = newPKI(t).cert(t, "bff.private", false, false)
				want = 0
				reads = 0
			case "expired cert":
				cert = p.cert(t, "bff.private", false, true)
				want = 0
				reads = 0
			case "missing session":
				session = ""
				want = 401
				reads = 0
			case "forged authority":
				extra["X-HitPipe-Epoch"] = "999"
				want = 400
				reads = 0
			case "stale grant":
				a.grant.ExpiresAt = time.Now().Add(-time.Second)
				want = 403
				reads = 0
			case "wrong workspace":
				a.grant.WorkspaceID = "33333333-3333-4333-8333-333333333333"
				want = 403
				reads = 0
			case "closed SQL gate":
				g.phase = "SQL_CONTENT_READ"
				want = 403
				reads = 0
			case "revoked before response":
				g.phase = "RESPONSE_START"
				want = 403
			case "denied session":
				a.err = readcases.ErrUnauthenticated
				want = 401
				reads = 0
			case "database error":
				repo.err = readcases.ErrUnavailable
				want = 503
			case "invalid query":
				path += "?limit=101"
				want = 400
				reads = 0
			}
			c := client(p, presented)
			defer c.CloseIdleConnections()
			status, b, e := request(t, c, ts.URL+path, session, extra)
			if status != want || (want == 0 && e == nil) || (want != 0 && e != nil) || repo.calls != reads {
				t.Fatalf("status=%d want=%d reads=%d want=%d err=%v body=%s", status, want, repo.calls, reads, e, b)
			}
			if want == 200 {
				if !strings.Contains(string(b), `"items":[]`) {
					t.Fatal(string(b))
				}
				if g.calls != 2 || a.calls != 1 {
					t.Fatal("missing fresh authority or response gate")
				}
			} else if want > 0 {
				var out wireError
				if json.Unmarshal(b, &out) != nil || !readcases.UUID(out.RequestID) || strings.Contains(string(b), "synthetic") {
					t.Fatal("unsafe error", string(b))
				}
			}
		})
	}
}
func TestParsingAndUnavailableComposition(t *testing.T) {
	if _, e := New(readcases.Service{}, "bff.private"); e == nil {
		t.Fatal("permissive empty composition")
	}
	s, _, _, repo := components()
	ts, p := fixture(t, s)
	cert := p.cert(t, "bff.private", false, false)
	c := client(p, &cert)
	defer c.CloseIdleConnections()
	for _, suffix := range []string{"?limit=0", "?limit=1&limit=2", "?cursor=", "?state=unknown", "?unknown=x", "?cursor=%zz", "/../../other", "/not-a-uuid"} {
		code, _, e := request(t, c, ts.URL+"/api/v1/workspaces/"+ws+"/cases"+suffix, "s", nil)
		if e == nil && code == 200 {
			t.Fatal("accepted", suffix)
		}
	}
	if repo.calls != 0 {
		t.Fatal("invalid input reached storage")
	}
}
