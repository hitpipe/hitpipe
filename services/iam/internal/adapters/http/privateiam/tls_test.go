package privateiam

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

func unitCA(t *testing.T) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT115 unit CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return certificate, private
}

func unitIssued(t *testing.T, ca *x509.Certificate, caKey ed25519.PrivateKey, names []string, usage x509.ExtKeyUsage, expired bool) tls.Certificate {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	notAfter := time.Now().Add(time.Hour)
	if expired {
		notAfter = time.Now().Add(-time.Minute)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "bff.hit115.test"}, DNSNames: names, NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage}}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, public, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.Raw}, PrivateKey: private, Leaf: leaf}
}

func unitLeaf(t *testing.T, names []string) *x509.Certificate {
	t.Helper()
	ca, key := unitCA(t)
	return unitIssued(t, ca, key, names, x509.ExtKeyUsageClientAuth, false).Leaf
}

func unitPeer(certificate *x509.Certificate) runtimeaccess.Peer {
	digest := sha256.Sum256(certificate.Raw)
	return runtimeaccess.Peer{DNSName: strings.ToLower(certificate.DNSNames[0]), CertificateFingerprint: "sha256:" + hex.EncodeToString(digest[:])}
}

func TestTLSListenerPinsPeerSANFingerprintAndTLS13(t *testing.T) {
	ca, key := unitCA(t)
	serverCertificate := unitIssued(t, ca, key, []string{"iam.hit115.test"}, x509.ExtKeyUsageServerAuth, false)
	clientCertificate := unitIssued(t, ca, key, []string{"bff.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	config, err := TLSConfig(serverCertificate, pool, []runtimeaccess.Peer{unitPeer(clientCertificate.Leaf)})
	if err != nil {
		t.Fatal(err)
	}
	if config.MinVersion != tls.VersionTLS13 || config.MaxVersion != tls.VersionTLS13 || config.ClientAuth != tls.RequireAndVerifyClientCert || !config.SessionTicketsDisabled {
		t.Fatal("insecure TLS configuration")
	}
	backend := &unitBackend{}
	handler, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = config
	server.StartTLS()
	defer server.Close()
	send := func(certificate *tls.Certificate, version uint16) (*http.Response, error) {
		clientConfig := &tls.Config{RootCAs: pool, ServerName: "iam.hit115.test", MinVersion: version, MaxVersion: version}
		if certificate != nil {
			clientConfig.Certificates = []tls.Certificate{*certificate}
		}
		transport := &http.Transport{TLSClientConfig: clientConfig}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: time.Second}
		request, err := http.NewRequest(http.MethodPost, server.URL+AuthorizePath, strings.NewReader(unitAuthorizeJSON))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Request-ID", unitRequestID)
		request.Header.Set("X-HitPipe-Session", strings.Repeat("x", 43))
		return client.Do(request)
	}
	response, err := send(&clientCertificate, tls.VersionTLS13)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 || backend.authorizeCalls != 1 {
		t.Fatal("valid peer did not reach handler")
	}
	foreign := unitIssued(t, ca, key, []string{"foreign.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	replacement := unitIssued(t, ca, key, []string{"bff.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	ambiguous := unitIssued(t, ca, key, []string{"bff.hit115.test", "foreign.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	cnOnly := unitIssued(t, ca, key, nil, x509.ExtKeyUsageClientAuth, false)
	expired := unitIssued(t, ca, key, []string{"bff.hit115.test"}, x509.ExtKeyUsageClientAuth, true)
	wrongUsage := unitIssued(t, ca, key, []string{"bff.hit115.test"}, x509.ExtKeyUsageServerAuth, false)
	for name, certificate := range map[string]*tls.Certificate{"missing": nil, "foreignSAN": &foreign, "wrongFingerprint": &replacement, "ambiguousSAN": &ambiguous, "CNOnly": &cnOnly, "expired": &expired, "wrongEKU": &wrongUsage} {
		t.Run(name, func(t *testing.T) {
			if response, err := send(certificate, tls.VersionTLS13); err == nil {
				response.Body.Close()
				t.Fatal("unprovisioned certificate accepted")
			}
		})
	}
	if response, err := send(&clientCertificate, tls.VersionTLS12); err == nil {
		response.Body.Close()
		t.Fatal("TLS1.2 accepted")
	}
	if backend.authorizeCalls != 1 {
		t.Fatal("rejected transport reached backend")
	}
}

func TestTLSConfigurationAndRequestPeerRevalidation(t *testing.T) {
	ca, key := unitCA(t)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	server := unitIssued(t, ca, key, []string{"iam.hit115.test"}, x509.ExtKeyUsageServerAuth, false)
	client := unitIssued(t, ca, key, []string{"BFF.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	peer := unitPeer(client.Leaf)
	config, err := TLSConfig(server, pool, []runtimeaccess.Peer{peer})
	if err != nil {
		t.Fatal(err)
	}
	state := tls.ConnectionState{Version: tls.VersionTLS13, PeerCertificates: []*x509.Certificate{client.Leaf}, VerifiedChains: [][]*x509.Certificate{{client.Leaf, ca}}}
	// VerifyConnection runs during the handshake, before HandshakeComplete flips.
	if err := config.VerifyConnection(state); err != nil {
		t.Fatal(err)
	}
	if _, err := transportPeer(&state); err == nil {
		t.Fatal("incomplete request handshake accepted")
	}
	state.HandshakeComplete = true
	actual, err := transportPeer(&state)
	if err != nil || actual != peer {
		t.Fatal("DNS case or certificate binding changed")
	}
	expired := *client.Leaf
	expired.NotAfter = time.Now().Add(-time.Second)
	state.PeerCertificates = []*x509.Certificate{&expired}
	state.VerifiedChains = [][]*x509.Certificate{{&expired, ca}}
	if _, err := transportPeer(&state); err == nil {
		t.Fatal("cached handshake outlived certificate validity")
	}
	if _, err := TLSConfig(tls.Certificate{}, pool, []runtimeaccess.Peer{peer}); err == nil {
		t.Fatal("absent server certificate accepted")
	}
	if _, err := TLSConfig(server, nil, []runtimeaccess.Peer{peer}); err == nil {
		t.Fatal("absent client CA accepted")
	}
	if _, err := TLSConfig(server, pool, nil); err == nil {
		t.Fatal("absent peer inventory accepted")
	}
	if _, err := TLSConfig(server, pool, []runtimeaccess.Peer{peer, peer}); err == nil {
		t.Fatal("duplicate peer inventory accepted")
	}
}
