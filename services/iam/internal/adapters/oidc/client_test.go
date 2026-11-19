package oidc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	issuer      = "https://oidc.invalid/realms/hit106"
	clientID    = "test:confidential client"
	secret      = "synthetic:client secret"
	redirectURI = "https://iam.invalid/auth/callback"
	code        = "synthetic-authorization-code"
	nonce       = "synthetic-browser-nonce"
	subject     = "subject-1"
	keyID       = "signing-key-1"
)

var (
	now      = time.Unix(1_800_000_000, 0)
	verifier = strings.Repeat("v", 43)
)

func fixtureConfig(server *httptest.Server) Config {
	ca := x509.NewCertPool()
	ca.AddCert(server.Certificate())
	return Config{
		Issuer:                issuer,
		AuthorizationEndpoint: server.URL + "/authorize",
		TokenEndpoint:         server.URL + "/token",
		JWKSURI:               server.URL + "/certs",
		RedirectURI:           redirectURI,
		ClientID:              clientID,
		ClientSecret:          secret,
		RootCAs:               ca,
		Now:                   func() time.Time { return now },
	}
}

func newTestClient(t *testing.T, config Config) *Client {
	t.Helper()
	client, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.http.CloseIdleConnections)
	return client
}

func signingKeys(t *testing.T) (*rsa.PrivateKey, map[string]any) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwk := map[string]any{
		"kid": keyID, "kty": "RSA", "alg": "RS256", "use": "sig", "key_ops": []string{"verify"},
		"n": base64url.EncodeToString(key.N.Bytes()),
		"e": base64url.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
	return key, jwk
}

func testClaims() map[string]any {
	return map[string]any{
		"iss": issuer, "aud": clientID, "azp": clientID, "sub": subject,
		"nonce": nonce, "email_verified": true,
		"iat": now.Unix() - 10, "nbf": now.Unix() - 10, "exp": now.Unix() + 300,
	}
}

func jsonBytes(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func signedToken(t *testing.T, key *rsa.PrivateKey, header map[string]any, claims []byte) string {
	t.Helper()
	payload := base64url.EncodeToString(jsonBytes(t, header)) + "." + base64url.EncodeToString(claims)
	digest := sha256.Sum256([]byte(payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return payload + "." + base64url.EncodeToString(signature)
}

func testHeader() map[string]any {
	return map[string]any{"alg": "RS256", "typ": "JWT", "kid": keyID}
}

func testServer(t *testing.T, token, keys []byte, checkTokenRequest func(*http.Request)) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			if checkTokenRequest != nil {
				checkTokenRequest(r)
			}
			_, _ = w.Write(token)
		case "/certs":
			if r.Method != http.MethodGet || r.Header.Get("Authorization") != "" {
				t.Error("JWKS request unexpectedly used credentials or another method")
			}
			_, _ = w.Write(keys)
		default:
			t.Error("request went to an unexpected endpoint")
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func expectExchangeError(t *testing.T, client *Client, code, verifier, nonce string, want error) {
	t.Helper()
	gotIssuer, gotSubject, verified, err := client.Exchange(t.Context(), code, verifier, nonce)
	if !errors.Is(err, want) || gotIssuer != "" || gotSubject != "" || verified {
		t.Fatal("exchange did not return the expected sanitized failure")
	}
}

func TestAuthorizationURLAndVerifiedTLSExchange(t *testing.T) {
	key, jwk := signingKeys(t)
	claims := testClaims()
	claims["aud"] = []string{clientID}
	const accessToken = "synthetic-access-token"
	accessDigest := sha256.Sum256([]byte(accessToken))
	claims["at_hash"] = base64url.EncodeToString(accessDigest[:len(accessDigest)/2])
	token := signedToken(t, key, testHeader(), jsonBytes(t, claims))
	payload := jsonBytes(t, map[string]any{"id_token": token, "access_token": accessToken})
	keys := jsonBytes(t, map[string]any{"keys": []any{jwk}})
	server := testServer(t, payload, keys, func(r *http.Request) {
		want := url.Values{
			"grant_type": {"authorization_code"}, "redirect_uri": {redirectURI},
			"code": {code}, "code_verifier": {verifier},
		}
		name, password, authenticated := r.BasicAuth()
		if r.Method != http.MethodPost || r.URL.RawQuery != "" || !authenticated ||
			name != url.QueryEscape(clientID) || password != url.QueryEscape(secret) ||
			r.ParseForm() != nil || r.PostForm.Encode() != want.Encode() {
			t.Error("expected confidential code exchange and exact redirect URI")
		}
	})
	client := newTestClient(t, fixtureConfig(server))
	authorization, err := url.Parse(client.AuthorizationURL("synthetic-state", nonce, "synthetic-challenge"))
	if err != nil {
		t.Fatal("expected valid authorization URL")
	}
	query := authorization.Query()
	want := url.Values{
		"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirectURI},
		"scope": {"openid email"}, "state": {"synthetic-state"}, "nonce": {nonce},
		"code_challenge_method": {"S256"}, "code_challenge": {"synthetic-challenge"},
	}
	if authorization.Path != "/authorize" || query.Encode() != want.Encode() {
		t.Fatal("authorization parameters differed from confidential OIDC with PKCE S256")
	}
	gotIssuer, gotSubject, verified, err := client.Exchange(t.Context(), code, verifier, nonce)
	if err != nil || gotIssuer != issuer || gotSubject != subject || !verified {
		t.Fatalf("private-authority HTTPS exchange failed: %v", err)
	}
}

func TestExchangeRejectsInvalidSignedClaims(t *testing.T) {
	key, jwk := signingKeys(t)
	keys := jsonBytes(t, map[string]any{"keys": []any{jwk}})
	tests := []struct {
		name, claim string
		value       any
	}{
		{"exact issuer", "iss", issuer + "/"},
		{"audience", "aud", "other-client"},
		{"untrusted additional audience", "aud", []string{clientID, "other-client"}},
		{"authorized party", "azp", "other-client"},
		{"nonce", "nonce", "another-attempt"},
		{"expiry boundary", "exp", now.Unix()},
		{"missing expiry", "exp", nil},
		{"future issued at", "iat", now.Unix() + 1},
		{"missing issued at", "iat", nil},
		{"future not before", "nbf", now.Unix() + 1},
		{"blank subject", "sub", " "},
		{"email verification type", "email_verified", "true"},
		{"missing email verification", "email_verified", nil},
		{"mismatched access hash", "at_hash", "invalid-hash"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := testClaims()
			if test.value == nil {
				delete(claims, test.claim)
			} else {
				claims[test.claim] = test.value
			}
			token := signedToken(t, key, testHeader(), jsonBytes(t, claims))
			payload := jsonBytes(t, map[string]any{"id_token": token})
			server := testServer(t, payload, keys, nil)
			expectExchangeError(t, newTestClient(t, fixtureConfig(server)), code, verifier, nonce, ErrInvalidResponse)
		})
	}
	t.Run("unverified email remains false", func(t *testing.T) {
		claims := testClaims()
		claims["email_verified"] = false
		token := signedToken(t, key, testHeader(), jsonBytes(t, claims))
		server := testServer(t, jsonBytes(t, map[string]any{"id_token": token}), keys, nil)
		client := newTestClient(t, fixtureConfig(server))
		gotIssuer, gotSubject, verified, err := client.Exchange(t.Context(), code, verifier, nonce)
		if err != nil || gotIssuer != issuer || gotSubject != subject || verified {
			t.Fatal("verified identity and unverified email must remain distinct")
		}
	})
}

func TestExchangeRejectsInvalidSignatureAndKeys(t *testing.T) {
	key, jwk := signingKeys(t)
	otherKey, _ := signingKeys(t)
	claims := jsonBytes(t, testClaims())
	wrongAlg := testHeader()
	wrongAlg["alg"] = "HS256"
	unknownKID := testHeader()
	unknownKID["kid"] = "key-not-in-configured-jwks"
	wrongPurpose := maps.Clone(jwk)
	wrongPurpose["use"] = "enc"
	duplicateIssuer := append([]byte(`{"iss":"https://other.invalid",`), claims[1:]...)
	tests := []struct {
		name   string
		key    *rsa.PrivateKey
		header map[string]any
		claims []byte
		keys   []any
	}{
		{"wrong signer", otherKey, testHeader(), claims, []any{jwk}},
		{"wrong algorithm", key, wrongAlg, claims, []any{jwk}},
		{"unknown key id", key, unknownKID, claims, []any{jwk}},
		{"duplicate key id", key, testHeader(), claims, []any{jwk, jwk}},
		{"wrong key purpose", key, testHeader(), claims, []any{wrongPurpose}},
		{"duplicate issuer claim", key, testHeader(), duplicateIssuer, []any{jwk}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			token := signedToken(t, test.key, test.header, test.claims)
			payload := jsonBytes(t, map[string]any{"id_token": token})
			keys := jsonBytes(t, map[string]any{"keys": test.keys})
			server := testServer(t, payload, keys, nil)
			expectExchangeError(t, newTestClient(t, fixtureConfig(server)), code, verifier, nonce, ErrInvalidResponse)
		})
	}
}

func TestMachineRequestsDoNotFollowTLSRedirects(t *testing.T) {
	var redirected atomic.Int32
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(other.Close)
	key, _ := signingKeys(t)
	token := signedToken(t, key, testHeader(), jsonBytes(t, testClaims()))
	payload := jsonBytes(t, map[string]any{"id_token": token})
	for _, path := range []string{"/token", "/certs"} {
		t.Run(path, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == path {
					http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(payload)
			}))
			t.Cleanup(server.Close)
			config := fixtureConfig(server)
			config.RootCAs.AddCert(other.Certificate())
			expectExchangeError(t, newTestClient(t, config), code, verifier, nonce, ErrUnavailable)
		})
	}
	if redirected.Load() != 0 {
		t.Fatal("provider redirect was followed")
	}
}

func TestInvalidCodeAndProviderResponseLimits(t *testing.T) {
	var unexpectedRequests atomic.Int32
	for _, test := range []struct {
		name, path, body string
		status           int
		want             error
	}{
		{"invalid grant", "/token", `{"error":"invalid_grant"}`, http.StatusBadRequest, ErrInvalidResponse},
		{"invalid client", "/token", `{"error":"invalid_client"}`, http.StatusUnauthorized, ErrUnavailable},
		{"OIDC service unavailable", "/token", "", http.StatusServiceUnavailable, ErrUnavailable},
		{"oversized token response", "/token", strings.Repeat(" ", tokenBodyLimit+1), http.StatusOK, ErrInvalidResponse},
		{"oversized keys response", "/certs", strings.Repeat(" ", keysBodyLimit+1), http.StatusOK, ErrInvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			key, _ := signingKeys(t)
			token := signedToken(t, key, testHeader(), jsonBytes(t, testClaims()))
			payload := jsonBytes(t, map[string]any{"id_token": token})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == test.path {
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, test.body)
					return
				}
				if r.URL.Path != "/token" {
					unexpectedRequests.Add(1)
				}
				_, _ = w.Write(payload)
			}))
			t.Cleanup(server.Close)
			client := newTestClient(t, fixtureConfig(server))
			expectExchangeError(t, client, code, verifier, nonce, test.want)
			expectExchangeError(t, client, "", verifier, nonce, ErrInvalidResponse)
			expectExchangeError(t, client, code, "invalid", nonce, ErrInvalidResponse)
			expectExchangeError(t, client, code, verifier, "", ErrInvalidResponse)
		})
	}
	if unexpectedRequests.Load() != 0 {
		t.Fatal("unexpected provider request")
	}
}

func TestTLSRequiresTrustedAuthorityCorrectSANAndValidity(t *testing.T) {
	for _, test := range []struct {
		name     string
		trusted  bool
		validSAN bool
		validAge bool
	}{
		{"trusted CA", false, true, true},
		{"subject alternative name", true, false, true},
		{"expiry", true, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, ca := testCertificateServer(t, test.validSAN, test.validAge)
			config := fixtureConfig(server)
			config.RootCAs = x509.NewCertPool()
			if test.trusted {
				config.RootCAs.AddCert(ca)
			}
			expectExchangeError(t, newTestClient(t, config), code, verifier, nonce, ErrUnavailable)
		})
	}
}

func testCertificateServer(t *testing.T, validSAN, validAge bool) (*httptest.Server, *x509.Certificate) {
	t.Helper()
	clock := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: clock.Add(-48 * time.Hour), NotAfter: clock.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caBytes)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: clock.Add(-24 * time.Hour), NotAfter: clock.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"oidc.invalid"},
	}
	if validSAN {
		leaf.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	}
	if !validAge {
		leaf.NotAfter = clock.Add(-time.Hour)
	}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafBytes, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("HTTP request passed invalid TLS validation")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{
		Certificate: [][]byte{leafBytes, caBytes}, PrivateKey: leafKey,
	}}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, ca
}

func TestProviderSettingsRequireFixedHTTPSURLs(t *testing.T) {
	config := Config{
		Issuer: issuer, AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token",
		JWKSURI: issuer + "/certs", RedirectURI: redirectURI, ClientID: clientID, ClientSecret: secret,
	}
	for _, url := range []string{"http://iam.invalid/auth/callback", "https://u:p@iam.invalid/auth/callback",
		redirectURI + "#fragment", redirectURI + "?next=https://other.invalid", "//iam.invalid/auth/callback"} {
		config.RedirectURI = url
		if _, err := New(config); err == nil || err.Error() != "invalid OIDC provider settings" {
			t.Fatal("invalid callback settings were accepted or disclosed")
		}
	}
	config.RedirectURI = redirectURI
	config.ClientSecret = ""
	if _, err := New(config); err == nil {
		t.Fatal("public client configuration was accepted")
	}
}

func TestExchangeCanceledContextIsSanitized(t *testing.T) {
	server := testServer(t, nil, nil, func(*http.Request) { t.Error("canceled exchange made a request") })
	client := newTestClient(t, fixtureConfig(server))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, _, _, err := client.Exchange(ctx, code, verifier, nonce); !errors.Is(err, ErrUnavailable) {
		t.Fatal("canceled exchange leaked transport details")
	}
}
