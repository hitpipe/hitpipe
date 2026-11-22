package revocation

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var (
	errConfiguration = errors.New("revocation control: invalid TLS or endpoint configuration")
	errCertificate   = errors.New("revocation control: certificate rejected")
)

// ServerTLS configures the private control listener, with an IAM-only client
// CA and the exact provisioned IAM client leaf fingerprint. It does not open
// a listener or authorize other endpoints.
func ServerTLS(serverCertificate tls.Certificate, iamClientCAs *x509.CertPool, iamFingerprint string) (*tls.Config, error) {
	if !validCertificateConfig(serverCertificate) || !validRoots(iamClientCAs) || !runtimeaccess.ValidFingerprint(iamFingerprint) {
		return nil, errConfiguration
	}
	roots := iamClientCAs.Clone()
	return &tls.Config{
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{serverCertificate},
		ClientAuth:   tls.RequireAndVerifyClientCert, ClientCAs: roots,
		SessionTicketsDisabled: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyIAM(&state, roots, iamFingerprint)
		},
	}, nil
}

func validRoots(pool *x509.CertPool) bool { return pool != nil && len(pool.Subjects()) != 0 }

func validCertificateConfig(c tls.Certificate) bool {
	if len(c.Certificate) == 0 || c.PrivateKey == nil {
		return false
	}
	_, err := x509.ParseCertificate(c.Certificate[0])
	return err == nil
}

func leafMatches(state *tls.ConnectionState, expected string) bool {
	if state == nil || state.Version != tls.VersionTLS13 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return false
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || len(leaf.Raw) == 0 {
		return false
	}
	digest := sha256.Sum256(leaf.Raw)
	if "sha256:"+hex.EncodeToString(digest[:]) != expected {
		return false
	}
	now := time.Now()
	for _, chain := range state.VerifiedChains {
		if len(chain) == 0 || chain[0] == nil || !leaf.Equal(chain[0]) {
			continue
		}
		valid := true
		for _, cert := range chain {
			if cert == nil || now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
				valid = false
				break
			}
		}
		if valid {
			return true
		}
	}
	return false
}

func verifyIAM(state *tls.ConnectionState, roots *x509.CertPool, expected string) error {
	if !leafMatches(state, expected) {
		return errCertificate
	}
	leaf := state.PeerCertificates[0]
	// Require a single explicit service DNS SAN. No CN fallback or alternate
	// SAN identity is supported for IAM client certificates.
	if len(leaf.DNSNames) != 1 || !runtimeaccess.ValidDNSName(leaf.DNSNames[0]) ||
		len(leaf.IPAddresses) != 0 || len(leaf.URIs) != 0 || len(leaf.EmailAddresses) != 0 {
		return errCertificate
	}
	intermediates := x509.NewCertPool()
	for _, cert := range state.PeerCertificates[1:] {
		if cert == nil {
			return errCertificate
		}
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots: roots, Intermediates: intermediates, DNSName: leaf.DNSNames[0],
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		return errCertificate
	}
	return nil
}
