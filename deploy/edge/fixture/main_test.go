package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func verifiedPeer(names ...string) *tls.ConnectionState {
	leaf := &x509.Certificate{DNSNames: names}
	return &tls.ConnectionState{
		Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{leaf},
		VerifiedChains: [][]*x509.Certificate{{leaf}},
	}
}

func hash(p []byte) string {
	sum := sha256.Sum256(p)
	return hex.EncodeToString(sum[:])
}

func TestObservationDoesNotExposeSyntheticInputs(t *testing.T) {
	raw := []byte("synthetic-body-\x00\r\n")
	req := httptest.NewRequest("POST", "https://fixture.invalid/webhooks/stripe/example?fixture-private-query", bytes.NewReader(raw))
	req.TLS = verifiedPeer("edge")
	req.Header.Set("X-HitPipe-Example", "fixture-private-x")
	req.Header.Set("Authorization", "fixture-private-auth")
	req.Header.Set("Cookie", "fixture-private-cookie")
	req.Header.Set("Stripe-Signature", "fixture-private-signature")

	got := httptest.NewRecorder()
	observe("intake").ServeHTTP(got, req)
	if got.Code != http.StatusOK || strings.Contains(got.Body.String(), "fixture-private") || bytes.Contains(got.Body.Bytes(), raw) {
		t.Fatalf("unexpected status or raw synthetic input: status %d", got.Code)
	}
	var report observation
	if err := json.Unmarshal(got.Body.Bytes(), &report); err != nil {
		t.Fatal("fixture observation is not JSON")
	}
	want := observation{
		Role: "intake", Method: "POST", Path: "/webhooks/stripe/example", TLSVersion: "TLS1.3", PeerDNS: "edge",
		HitPipeHeaderPresent: true, AuthorizationPresent: true, CookiePresent: true,
		StripeSignatureSHA256: hash([]byte("fixture-private-signature")), BodySHA256: hash(raw), BodyBytes: int64(len(raw)),
	}
	if report != want {
		t.Fatal("fixture did not preserve digest and header-presence observations")
	}
}

func TestObserveRefusesAmbiguousOrOversizedInput(t *testing.T) {
	for _, test := range []struct {
		name      string
		bodyBytes int
		headers   int
		want      int
	}{
		{"limit", maxBodyBytes, 1, http.StatusOK},
		{"above limit", maxBodyBytes + 1, 1, http.StatusRequestEntityTooLarge},
		{"repeated signature", 1, 2, http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://fixture.invalid/webhooks/stripe/example", bytes.NewReader(make([]byte, test.bodyBytes)))
			req.TLS = verifiedPeer("edge")
			for i := 0; i < test.headers; i++ {
				req.Header.Add("Stripe-Signature", "fixture-synthetic")
			}
			r := httptest.NewRecorder()
			observe("intake").ServeHTTP(r, req)
			if r.Code != test.want {
				t.Fatalf("status = %d, want %d", r.Code, test.want)
			}
		})
	}
}

func TestClientIdentityIsIndependentOfHTTPHeaders(t *testing.T) {
	for _, test := range []struct {
		name string
		role string
		tls  *tls.ConnectionState
		want int
	}{
		{"direct plain request", "iam", nil, http.StatusUnauthorized},
		{"client missing", "iam", &tls.ConnectionState{Version: tls.VersionTLS13}, http.StatusUnauthorized},
		{"wrong verified identity", "iam", verifiedPeer("not-edge"), http.StatusUnauthorized},
		{"ambiguous identity", "iam", verifiedPeer("edge", "not-edge"), http.StatusUnauthorized},
		{"correct verified identity", "iam", verifiedPeer("edge"), http.StatusOK},
		{"IdP server authentication only", "keycloak", &tls.ConnectionState{Version: tls.VersionTLS13}, http.StatusOK},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "https://fixture.invalid/auth/login", nil)
			req.TLS = test.tls
			req.Header.Set("X-HitPipe-Client", "edge")
			r := httptest.NewRecorder()
			observe(test.role).ServeHTTP(r, req)
			if r.Code != test.want {
				t.Fatalf("status = %d, want %d", r.Code, test.want)
			}
		})
	}
}
