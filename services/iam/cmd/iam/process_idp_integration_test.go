//go:build integration && linux

package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

const (
	processClientID = "hit138-isolated-client"
	processCallback = "https://browser.hit138.test/auth/callback"
	fixtureKeyID    = "hit138-ephemeral-signing-key"
)

type processIDP struct {
	issuer, subject, secret string
	key                     *rsa.PrivateKey
	keys                    []byte
	mu                      sync.Mutex
	code, nonce, challenge  string
}

func newProcessIDP(t *testing.T, cert tls.Certificate, subject string) *processIDP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("generate isolated IdP signing key")
	}
	jwk := map[string]any{
		"kid": fixtureKeyID, "kty": "RSA", "alg": "RS256", "use": "sig",
		"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
	keys, err := json.Marshal(map[string]any{"keys": []map[string]any{jwk}})
	if err != nil {
		t.Fatal("encode isolated IdP signing key")
	}
	idp := &processIDP{subject: subject, secret: processSecret(), key: key, keys: keys}
	server := httptest.NewUnstartedServer(idp)
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	t.Cleanup(server.Close)
	idp.issuer = server.URL
	return idp
}

func (f *processIDP) issueTestCode(nonce, challenge string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.code, f.nonce, f.challenge = processSecret(), nonce, challenge
	return f.code
}

func (f *processIDP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/keys":
		_, _ = w.Write(f.keys)
	case r.Method == http.MethodPost && r.URL.Path == "/token":
		f.exchange(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *processIDP) exchange(w http.ResponseWriter, r *http.Request) {
	if r.ParseForm() != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	user, password, ok := r.BasicAuth()
	challenge := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	f.mu.Lock()
	defer f.mu.Unlock()
	if !ok || user != processClientID || password != f.secret || f.code == "" || r.PostForm.Get("code") != f.code ||
		r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("redirect_uri") != processCallback ||
		base64.RawURLEncoding.EncodeToString(challenge[:]) != f.challenge {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	f.code = ""
	now := time.Now().Unix()
	header, _ := json.Marshal(map[string]any{"alg": "RS256", "kid": fixtureKeyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": f.issuer, "aud": processClientID, "sub": f.subject, "nonce": f.nonce,
		"email_verified": true, "iat": now - 2, "nbf": now - 2, "exp": now + 120,
	})
	data := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, hash[:])
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"id_token": data + "." + base64.RawURLEncoding.EncodeToString(signature),
	})
}
