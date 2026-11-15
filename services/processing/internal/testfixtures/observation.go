// Package testfixtures provides only explicitly controlled local test certificates.
package testfixtures

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"
)

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
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "HIT-99 controlled CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
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
		if name == "processing.test" {
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
	return &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: p.Root, Certificates: []tls.Certificate{p.Leaf(t, "bff.test", false, false)}}}
}
