// Package testfixtures contains explicitly controlled local qualification peers.
// It is never wired into a runtime or an authority implementation.
package testfixtures

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }

type Gate struct {
	mu               sync.Mutex
	Workspace        string
	Epoch            int64
	Closed, NoInvoke bool
	Calls            int
}

func (g *Gate) Start(ctx context.Context, c dispatchintent.Claim, work func(context.Context) error) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Calls++
	if g.Closed || c.WorkspaceID != g.Workspace || c.Epoch != g.Epoch || ctx.Err() != nil {
		return errors.New("controlled authority denied")
	}
	if g.NoInvoke {
		return nil
	}
	return work(ctx)
}
func (g *Gate) Close() { g.mu.Lock(); defer g.mu.Unlock(); g.Closed = true }

type PKI struct {
	Root *x509.CertPool
	ca   *x509.Certificate
	key  *ecdsa.PrivateKey
}

func NewPKI(t *testing.T) *PKI {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT-98 controlled CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &k.PublicKey, k)
	if e != nil {
		t.Fatal(e)
	}
	ca, e = x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	return &PKI{roots, ca, k}
}
func (p *PKI) Leaf(t *testing.T, name string, server, expired bool) tls.Certificate {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}
	if server {
		cert.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		if name == "delivery.test" {
			cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		}
	}
	if expired {
		cert.NotAfter = time.Now().Add(-time.Minute)
	}
	der, e := x509.CreateCertificate(rand.Reader, cert, p.ca, &k.PublicKey, p.key)
	if e != nil {
		t.Fatal(e)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}
}
func (p *PKI) Transport(t *testing.T) *http.Transport {
	return &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: p.Root, Certificates: []tls.Certificate{p.Leaf(t, "processing.test", false, false)}}}
}
func (p *PKI) Server(t *testing.T, h http.HandlerFunc, name string) *httptest.Server {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	s.Config.ErrorLog = log.New(io.Discard, "", 0)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{p.Leaf(t, name, true, false)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: p.Root, VerifyConnection: func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) != 1 || len(cs.PeerCertificates[0].DNSNames) != 1 || cs.PeerCertificates[0].DNSNames[0] != "processing.test" {
			return errors.New("controlled caller denied")
		}
		return nil
	}}
	s.StartTLS()
	t.Cleanup(s.Close)
	return s
}
