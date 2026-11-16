package profile

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
)

type serviceFunc func(context.Context, string, authorizeaction.Caller, func(getprofile.Profile) error) error

func (f serviceFunc) Serve(ctx context.Context, secret string, caller authorizeaction.Caller, emit func(getprofile.Profile) error) error {
	return f(ctx, secret, caller, emit)
}

type profilePKI struct {
	ca          *x509.Certificate
	key         ed25519.PrivateKey
	roots       *x509.CertPool
	server, bff tls.Certificate
	caller      authorizeaction.Caller
}

func testPKI(t *testing.T) profilePKI {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("fixture key generation failed")
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT85 synthetic CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(2 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, public, key)
	if err != nil {
		t.Fatal("fixture CA creation failed")
	}
	ca, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal("fixture CA parsing failed")
	}
	p := profilePKI{ca: ca, key: key, roots: x509.NewCertPool()}
	p.roots.AddCert(ca)
	p.server = p.certificate(t, "iam.test", x509.ExtKeyUsageServerAuth, false)
	p.bff = p.certificate(t, "bff.test", x509.ExtKeyUsageClientAuth, false)
	digest := sha256.Sum256(p.bff.Certificate[0])
	p.caller = authorizeaction.Caller{Service: "BFF", RuntimeID: "10000000-0000-4000-8000-000000000001", BootID: "20000000-0000-4000-8000-000000000001", Generation: 3, CertificateFingerprint: "sha256:" + hex.EncodeToString(digest[:])}
	return p
}

func (p profilePKI) certificate(t *testing.T, dns string, usage x509.ExtKeyUsage, expired bool) tls.Certificate {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal("fixture leaf key generation failed")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal("fixture serial generation failed")
	}
	now := time.Now()
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "HIT85 synthetic leaf"}, DNSNames: []string{dns}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), ExtKeyUsage: []x509.ExtKeyUsage{usage}, KeyUsage: x509.KeyUsageDigitalSignature}
	if expired {
		leaf.NotBefore, leaf.NotAfter = now.Add(-2*time.Hour), now.Add(-time.Hour)
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, p.ca, public, p.key)
	if err != nil {
		t.Fatal("fixture leaf creation failed")
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func testServer(t *testing.T, p profilePKI, service Service) *httptest.Server {
	t.Helper()
	handler, err := New(service, Config{BFFCaller: p.caller, BFFDNSName: "bff.test"})
	if err != nil {
		t.Fatal("fixture HTTP handler configuration failed")
	}
	tlsConfig, err := TLSConfig(p.server, p.roots)
	if err != nil {
		t.Fatal("fixture mutual TLS configuration failed")
	}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = tlsConfig
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func testClient(t *testing.T, p profilePKI, certificates []tls.Certificate, maxTLS uint16) *http.Client {
	t.Helper()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: p.roots, ServerName: "iam.test", Certificates: certificates, MinVersion: tls.VersionTLS12, MaxVersion: maxTLS}, DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: 5 * time.Second}
}

const fixtureSecret = "HIT85-SYNTHETIC-OPAQUE-SESSION-AAAAAAAAAAAAAAAAAAAA"

func fixtureProfile() getprofile.Profile {
	return getprofile.Profile{UserID: "30000000-0000-4000-8000-000000000001", DisplayName: "Синтетический профиль", Timezone: "Europe/Moscow", Revision: 4}
}

func TestProfileHTTPConfiguration(t *testing.T) {
	p := testPKI(t)
	valid := Config{BFFCaller: p.caller, BFFDNSName: "bff.test"}
	service := serviceFunc(func(context.Context, string, authorizeaction.Caller, func(getprofile.Profile) error) error {
		return nil
	})
	if _, err := New(nil, valid); !errors.Is(err, ErrConfiguration) {
		t.Fatal("missing service accepted")
	}
	var nilService serviceFunc
	if _, err := New(nilService, valid); !errors.Is(err, ErrConfiguration) {
		t.Fatal("typed nil service accepted")
	}
	for _, alter := range []func(*Config){func(c *Config) { c.BFFCaller.Service = "IAM" }, func(c *Config) { c.BFFCaller.Generation = 0 }, func(c *Config) { c.BFFCaller.CertificateFingerprint = "sha256:wrong" }, func(c *Config) { c.BFFDNSName = "*.test" }, func(c *Config) { c.BFFCaller.RuntimeID = "browser-user" }} {
		invalid := valid
		alter(&invalid)
		if _, err := New(service, invalid); !errors.Is(err, ErrConfiguration) {
			t.Fatal("unbound BFF configuration accepted")
		}
	}
	if _, err := TLSConfig(p.server, nil); !errors.Is(err, ErrConfiguration) {
		t.Fatal("missing client CA accepted")
	}
	if _, err := TLSConfig(tls.Certificate{}, p.roots); !errors.Is(err, ErrConfiguration) {
		t.Fatal("missing server certificate accepted")
	}
	config, err := TLSConfig(p.server, p.roots)
	if err != nil || config.MinVersion != tls.VersionTLS13 || config.MaxVersion != tls.VersionTLS13 || config.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatal("mutual TLS policy was weakened")
	}
}

func TestProfileHTTPSMutualTLSBFFBinding(t *testing.T) {
	p := testPKI(t)
	var calls atomic.Int32
	service := serviceFunc(func(_ context.Context, secret string, caller authorizeaction.Caller, emit func(getprofile.Profile) error) error {
		calls.Add(1)
		if secret != fixtureSecret || caller != p.caller {
			return getprofile.ErrForbidden
		}
		return emit(fixtureProfile())
	})
	server := testServer(t, p, service)
	foreignPKI := testPKI(t)
	for _, tc := range []struct {
		name         string
		certificates []tls.Certificate
		maxTLS       uint16
		success      bool
	}{
		{"BFF", []tls.Certificate{p.bff}, tls.VersionTLS13, true},
		{"missing certificate", nil, tls.VersionTLS13, false},
		{"foreign CA", []tls.Certificate{foreignPKI.bff}, tls.VersionTLS13, false},
		{"foreign SAN", []tls.Certificate{p.certificate(t, "delivery.test", x509.ExtKeyUsageClientAuth, false)}, tls.VersionTLS13, false},
		{"same SAN different fingerprint", []tls.Certificate{p.certificate(t, "bff.test", x509.ExtKeyUsageClientAuth, false)}, tls.VersionTLS13, false},
		{"expired certificate", []tls.Certificate{p.certificate(t, "bff.test", x509.ExtKeyUsageClientAuth, true)}, tls.VersionTLS13, false},
		{"TLS12", []tls.Certificate{p.bff}, tls.VersionTLS12, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/profile", nil)
			request.Header.Set("X-HitPipe-Session", fixtureSecret)
			response, err := testClient(t, p, tc.certificates, tc.maxTLS).Do(request)
			if err != nil {
				if tc.success {
					t.Fatal("verified BFF TLS request failed")
				}
				if calls.Load() != before {
					t.Fatal("failed handshake reached application")
				}
				return
			}
			defer response.Body.Close()
			if tc.success {
				var got getprofile.Profile
				if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&got) != nil || got != fixtureProfile() || response.TLS.Version != tls.VersionTLS13 || response.Header.Get("Cache-Control") != "no-store" {
					t.Fatal("verified private profile response differs")
				}
			} else if response.StatusCode == http.StatusOK || calls.Load() != before {
				t.Fatal("unbound certificate reached application")
			}
		})
	}
}

func TestProfileHTTPErrorAndResponseStart(t *testing.T) {
	p := testPKI(t)
	for _, tc := range []struct {
		name   string
		action func(func(getprofile.Profile) error) error
		status int
	}{
		{"missing session", func(func(getprofile.Profile) error) error { return getprofile.ErrUnauthenticated }, 401},
		{"forbidden caller", func(func(getprofile.Profile) error) error { return getprofile.ErrForbidden }, 403},
		{"invalid request", func(func(getprofile.Profile) error) error { return getprofile.ErrInvalidRequest }, 400},
		{"database or content failure", func(func(getprofile.Profile) error) error { return errors.New("SENSITIVE-DB-OR-PROFILE") }, 503},
		{"cancelled", func(func(getprofile.Profile) error) error { return getprofile.ErrCancelled }, 503},
		{"no emission", func(func(getprofile.Profile) error) error { return nil }, 503},
		{"invalid profile", func(emit func(getprofile.Profile) error) error { return emit(getprofile.Profile{}) }, 503},
		{"error after emission", func(emit func(getprofile.Profile) error) error {
			if err := emit(fixtureProfile()); err != nil {
				return err
			}
			return getprofile.ErrUnavailable
		}, 200},
		{"second emission", func(emit func(getprofile.Profile) error) error {
			if err := emit(fixtureProfile()); err != nil {
				return err
			}
			return emit(fixtureProfile())
		}, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := testServer(t, p, serviceFunc(func(_ context.Context, _ string, _ authorizeaction.Caller, emit func(getprofile.Profile) error) error {
				return tc.action(emit)
			}))
			request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/v1/profile", nil)
			request.Header.Set("X-HitPipe-Session", fixtureSecret)
			response, err := testClient(t, p, []tls.Certificate{p.bff}, tls.VersionTLS13).Do(request)
			if err != nil {
				t.Fatal("fixture request failed")
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != tc.status || strings.Contains(string(body), fixtureSecret) || strings.Contains(string(body), "SENSITIVE") || response.Header.Get("Cache-Control") != "no-store" {
				t.Fatal("unsafe status or error body")
			}
			decoder := json.NewDecoder(strings.NewReader(string(body)))
			if tc.status == 200 {
				var p getprofile.Profile
				if decoder.Decode(&p) != nil || p != fixtureProfile() {
					t.Fatal("profile response corrupted by later error")
				}
			} else {
				var p errorResponse
				if decoder.Decode(&p) != nil || !validUUID(p.RequestID) || p.Code == "" || p.Message == "" {
					t.Fatal("error is not the accepted bounded projection")
				}
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				t.Fatal("more than one HTTP JSON response emitted")
			}
		})
	}
}

func TestProfileHTTPCredentialsAndRoutes(t *testing.T) {
	p := testPKI(t)
	var calls atomic.Int32
	server := testServer(t, p, serviceFunc(func(_ context.Context, secret string, caller authorizeaction.Caller, emit func(getprofile.Profile) error) error {
		calls.Add(1)
		if secret != fixtureSecret || caller != p.caller {
			return getprofile.ErrUnauthenticated
		}
		return emit(fixtureProfile())
	}))
	client := testClient(t, p, []tls.Certificate{p.bff}, tls.VersionTLS13)
	for _, tc := range []struct {
		name, method, path string
		values             []string
		cookie             bool
		status             int
	}{
		{"cookie only", "GET", "/api/v1/profile", nil, true, 401},
		{"duplicate credential", "GET", "/api/v1/profile", []string{fixtureSecret, fixtureSecret}, false, 401},
		{"wrong session with forged identity", "GET", "/api/v1/profile", []string{strings.Repeat("x", 43)}, false, 401},
		{"write method", "POST", "/api/v1/profile", []string{fixtureSecret}, false, 405},
		{"another route", "GET", "/api/v1/session", []string{fixtureSecret}, false, 404},
		{"client-selected principal", "GET", "/api/v1/profile?user_id=other", []string{fixtureSecret}, false, 404},
		{"claims do not replace session identity", "GET", "/api/v1/profile", []string{fixtureSecret}, true, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			request, _ := http.NewRequest(tc.method, server.URL+tc.path, nil)
			for _, value := range tc.values {
				request.Header.Add("X-HitPipe-Session", value)
			}
			if tc.cookie {
				request.AddCookie(&http.Cookie{Name: "__Host-hp_session", Value: fixtureSecret})
			}
			request.Header.Set("X-HitPipe-User", "forged-principal")
			request.Header.Set("X-HitPipe-Role", "OWNER")
			request.Header.Set("X-HitPipe-Workspace", "forged-workspace")
			request.Header.Set("X-Request-ID", "not-a-trusted-id")
			response, err := client.Do(request)
			if err != nil {
				t.Fatal("fixture request failed")
			}
			defer response.Body.Close()
			if response.StatusCode != tc.status {
				t.Fatalf("unexpected bounded status: %d", response.StatusCode)
			}
			if tc.status == 200 {
				var got getprofile.Profile
				if json.NewDecoder(response.Body).Decode(&got) != nil || got != fixtureProfile() {
					t.Fatal("browser claims changed server identity")
				}
			}
			if tc.status != 200 && tc.name != "wrong session with forged identity" && calls.Load() != before {
				t.Fatal("invalid transport input reached application")
			}
		})
	}
}

type boundedWriter struct {
	*httptest.ResponseRecorder
	deadline                 time.Time
	initialDeadline          time.Time
	block                    bool
	writes, headers, flushes int
}

func (w *boundedWriter) SetWriteDeadline(value time.Time) error {
	w.deadline = value
	if !value.IsZero() {
		w.initialDeadline = value
	}
	return nil
}
func (w *boundedWriter) WriteHeader(status int) { w.headers++; w.ResponseRecorder.WriteHeader(status) }
func (w *boundedWriter) Write(data []byte) (int, error) {
	w.writes++
	if w.block {
		if w.deadline.IsZero() {
			return 0, errors.New("missing synthetic write deadline")
		}
		timer := time.NewTimer(time.Until(w.deadline))
		defer timer.Stop()
		<-timer.C
		return 0, os.ErrDeadlineExceeded
	}
	return w.ResponseRecorder.Write(data)
}
func (w *boundedWriter) FlushError() error { w.flushes++; return nil }

func verifiedFixtureRequest(t *testing.T, p profilePKI) *http.Request {
	t.Helper()
	leaf, err := x509.ParseCertificate(p.bff.Certificate[0])
	if err != nil {
		t.Fatal("fixture leaf parsing failed")
	}
	request := httptest.NewRequest(http.MethodGet, "https://iam.test/api/v1/profile", nil)
	request.Header.Set("X-HitPipe-Session", fixtureSecret)
	request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, p.ca}}}
	return request
}

func TestProfileHTTPBoundedWriteAndUnsupportedWriter(t *testing.T) {
	p := testPKI(t)
	var calls int
	var emissionErr error
	service := serviceFunc(func(ctx context.Context, _ string, _ authorizeaction.Caller, emit func(getprofile.Profile) error) error {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Error("HTTP application context is unbounded")
		}
		emissionErr = emit(fixtureProfile())
		return emissionErr
	})
	handler, err := New(service, Config{BFFCaller: p.caller, BFFDNSName: "bff.test"})
	if err != nil {
		t.Fatal(err)
	}
	unsupported := httptest.NewRecorder()
	handler.ServeHTTP(unsupported, verifiedFixtureRequest(t, p))
	if unsupported.Code != 503 || calls != 0 {
		t.Fatal("unsupported writer entered protected application gate")
	}
	request := verifiedFixtureRequest(t, p)
	ctx, cancel := context.WithTimeout(request.Context(), 30*time.Millisecond)
	defer cancel()
	request = request.WithContext(ctx)
	writer := &boundedWriter{ResponseRecorder: httptest.NewRecorder(), block: true}
	start := time.Now()
	handler.ServeHTTP(writer, request)
	if calls != 1 || emissionErr == nil || time.Since(start) > 500*time.Millisecond || writer.initialDeadline.IsZero() || writer.headers != 1 || writer.writes != 1 || writer.Body.Len() != 0 || !writer.deadline.IsZero() {
		t.Fatal("bounded failed write retried, emitted error after response start or kept its deadline")
	}
	writer = &boundedWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, verifiedFixtureRequest(t, p))
	if writer.Code != 200 || writer.flushes != 1 || writer.headers != 1 || !writer.deadline.IsZero() {
		t.Fatal("profile response did not flush its actual start inside the bounded emitter")
	}
}

func TestProfileHTTPRechecksCertificateBeforeResponseStart(t *testing.T) {
	p := testPKI(t)
	request := verifiedFixtureRequest(t, p)
	service := serviceFunc(func(context.Context, string, authorizeaction.Caller, func(getprofile.Profile) error) error {
		return nil
	})
	service = func(_ context.Context, _ string, _ authorizeaction.Caller, emit func(getprofile.Profile) error) error {
		// Controlled expiry observation at the transport boundary. Actual valid,
		// expired and foreign certificate handshakes are tested over HTTPS above.
		request.TLS.PeerCertificates[0].NotAfter = time.Now().Add(-time.Second)
		return emit(fixtureProfile())
	}
	handler, err := New(service, Config{BFFCaller: p.caller, BFFDNSName: "bff.test"})
	if err != nil {
		t.Fatal(err)
	}
	writer := &boundedWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, request)
	if writer.Code != 403 || writer.flushes != 0 || strings.Contains(writer.Body.String(), fixtureProfile().DisplayName) {
		t.Fatal("expired certificate started a protected profile response")
	}
}
