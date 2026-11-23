package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
)

type identity struct {
	RuntimeID, Token, CSRF, Signing, StripeKey string
	CursorKey                                  []byte
}

func randomBytes(size int) []byte {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		log.Fatal("randomness unavailable")
	}
	return b
}

func mustUUID() string {
	b := randomBytes(16)
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func loadIdentity(state string) (*identity, error) {
	path := filepath.Join(state, "identity.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		var keys identity
		if err := json.Unmarshal(raw, &keys); err != nil {
			return nil, err
		}
		if !drafts.UUID(keys.RuntimeID) || len(keys.Token) != 43 || len(keys.CSRF) != 43 ||
			len(keys.Signing) != 43 || len(keys.StripeKey) != 43 || len(keys.CursorKey) != 32 {
			return nil, errors.New("invalid school identity")
		}
		return &keys, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	secret := func() string { return base64.RawURLEncoding.EncodeToString(randomBytes(32)) }
	keys := &identity{RuntimeID: mustUUID(), Token: secret(), CSRF: secret(), Signing: secret(), StripeKey: secret(), CursorKey: randomBytes(32)}
	return keys, writeJSON(path, keys)
}

type keypair struct {
	Cert, Key []byte
}

type certificates struct {
	CA     []byte
	Leaves map[string]keypair
	roots  *x509.CertPool
}

func initCertificates(state string) (*certificates, error) {
	path := filepath.Join(state, "certificates.json")
	raw, err := os.ReadFile(path)
	c := &certificates{}
	if err == nil {
		if err = json.Unmarshal(raw, c); err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		c, err = generateCertificates()
		if err != nil {
			return nil, err
		}
		if err := writeJSON(path, c); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}
	c.roots = x509.NewCertPool()
	if !c.roots.AppendCertsFromPEM(c.CA) {
		return nil, errors.New("invalid school CA")
	}
	return c, nil
}

func generateCertificates() (*certificates, error) {
	now := time.Now()
	newCertificate := func(name string) x509.Certificate {
		return x509.Certificate{SerialNumber: new(big.Int).SetBytes(randomBytes(16)), Subject: pkix.Name{CommonName: name},
			NotBefore: now.Add(-time.Minute), NotAfter: now.Add(30 * 24 * time.Hour)}
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ca := newCertificate("HitPipe private school CA")
	ca.IsCA, ca.BasicConstraintsValid = true, true
	ca.SubjectKeyId = randomBytes(20)
	ca.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	raw, err := x509.CreateCertificate(rand.Reader, &ca, &ca, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}

	c := &certificates{CA: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), Leaves: make(map[string]keypair)}
	for _, item := range []struct {
		name, ip string
		server   bool
	}{
		{"processing.fixture", "127.0.0.1", true}, {"iam.test", "127.0.0.2", true},
		{"intake.test-server", "127.0.0.5", true}, {"delivery.test-server", "127.0.0.6", true},
		{"stripe.fixture", "127.0.0.1", true}, {"clickup.fixture", "127.0.0.1", true},
		{"processing-client.test", "", false}, {"intake.test", "", false},
		{"delivery.test", "", false}, {"bff.test", "", false}, {"ops.test", "", false},
	} {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		name := item.name
		if name == "intake.test-server" {
			name = "intake.test"
		}
		if name == "delivery.test-server" {
			name = "delivery.test"
		}
		leaf := newCertificate(name)
		leaf.DNSNames = []string{name}
		leaf.KeyUsage = x509.KeyUsageDigitalSignature
		if item.server {
			leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
			leaf.IPAddresses = []net.IP{net.ParseIP(item.ip)}
		} else {
			leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		}
		raw, err := x509.CreateCertificate(rand.Reader, &leaf, &ca, &key.PublicKey, caKey)
		if err != nil {
			return nil, err
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		c.Leaves[item.name] = keypair{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})}
	}
	return c, nil
}

func (c *certificates) pair(name string) tls.Certificate {
	k := c.Leaves[name]
	pair, err := tls.X509KeyPair(k.Cert, k.Key)
	if err != nil {
		log.Fatalf("school TLS keypair %s invalid", name)
	}
	return pair
}

func (c *certificates) client(peer, serverName string) *http.Client {
	config := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: c.roots, ServerName: serverName}
	if peer != "" {
		config.Certificates = []tls.Certificate{c.pair(peer)}
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: config, DisableKeepAlives: true}, Timeout: 4 * time.Second}
}

func (s *backend) serveHTTPS(ctx context.Context, address, peer, cert string, handler http.HandlerFunc) error {
	l, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{s.certs.pair(cert)}}
	if peer != "" {
		config.ClientAuth, config.ClientCAs = tls.RequireAndVerifyClientCert, s.certs.roots
	}
	server := &http.Server{TLSConfig: config, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 4 * time.Second, WriteTimeout: 4 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peer != "" && (r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(peer) != nil) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			handler(w, r)
		})}
	context.AfterFunc(ctx, func() { server.Close() })
	go func() {
		if err := server.ServeTLS(l, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTPS listener %s stopped: %v", cert, err)
		}
	}()
	return nil
}

func (s *backend) authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !drafts.UUID(r.Header.Get("X-Request-ID")) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/internal/v1/runtime/register":
		var in drafts.RuntimeRegistration
		sum := sha256.Sum256(s.certs.pair("processing-client.test").Certificate[0])
		if draftjson.Validate("RuntimeRegistration", raw) != nil || json.Unmarshal(raw, &in) != nil ||
			in.RuntimeID != s.keys.RuntimeID || in.CertificateFingerprint != "sha256:"+hex.EncodeToString(sum[:]) ||
			in.Service != "PROCESSING" || in.Generation != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_ = json.NewEncoder(w).Encode(drafts.RuntimeSnapshot{RuntimeID: in.RuntimeID, BootID: in.BootID, Generation: in.Generation,
			Gate: "OPEN", SnapshotRevision: 2, Denials: []drafts.DenyEpochSnapshot{}})
	case "/internal/v1/authorize":
		var in drafts.AuthorizeRequest
		if draftjson.Validate("AuthorizeRequest", raw) != nil || json.Unmarshal(raw, &in) != nil || in.RuntimeID != s.keys.RuntimeID ||
			in.WorkspaceID != workspaceID || !constantEqual(r.Header.Get("X-HitPipe-Session"), s.keys.Token) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		switch in.Action {
		case "CASE_READ", "CONFIG_READ", "CONFIG_WRITE", "ACTIVATE", "CONNECTOR_READ", "CONNECTOR_WRITE", "CASE_RECOVER":
		default:
			w.WriteHeader(http.StatusForbidden)
			return
		}
		owner, role, session := ownerID, "OWNER", sessionID
		_ = json.NewEncoder(w).Encode(drafts.Authorization{Allowed: true, Epoch: 7, CheckedAt: time.Now().UTC(),
			WorkspaceID: &in.WorkspaceID, Role: &role, PrincipalID: &owner, SessionID: &session})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func constantEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
