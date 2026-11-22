// Package privateiam exposes the two accepted private IAM business operations.
// A verified transport peer is an input to independent application inventory
// checks, never a human permission or a caller-provided identity header.
package privateiam

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

var errCertificate = errors.New("privateiam: client certificate rejected")

// TLSConfig permits only independently provisioned DNS SAN/fingerprint pairs.
// The fingerprint is SHA-256 of the complete leaf certificate's DER bytes.
// The application still checks that the peer matches its runtime inventory.
func TLSConfig(serverCertificate tls.Certificate, clientCAs *x509.CertPool, peers []runtimeaccess.Peer) (*tls.Config, error) {
	if len(serverCertificate.Certificate) == 0 || serverCertificate.PrivateKey == nil || clientCAs == nil || len(clientCAs.Subjects()) == 0 || len(peers) == 0 {
		return nil, errors.New("privateiam: incomplete TLS configuration")
	}
	if _, err := x509.ParseCertificate(serverCertificate.Certificate[0]); err != nil {
		return nil, errors.New("privateiam: invalid server certificate")
	}
	allowed := make(map[runtimeaccess.Peer]struct{}, len(peers))
	for _, peer := range peers {
		if !runtimeaccess.ValidDNSName(peer.DNSName) || !runtimeaccess.ValidFingerprint(peer.CertificateFingerprint) {
			return nil, errors.New("privateiam: invalid provisioned peer")
		}
		peer.DNSName = strings.ToLower(peer.DNSName)
		if _, exists := allowed[peer]; exists {
			return nil, errors.New("privateiam: duplicate provisioned peer")
		}
		allowed[peer] = struct{}{}
	}
	return &tls.Config{
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		Certificates:           []tls.Certificate{serverCertificate},
		ClientAuth:             tls.RequireAndVerifyClientCert,
		ClientCAs:              clientCAs.Clone(),
		SessionTicketsDisabled: true,
		VerifyConnection: func(state tls.ConnectionState) error {
			peer, err := certificatePeer(&state)
			if err != nil {
				return errCertificate
			}
			if _, exists := allowed[peer]; !exists {
				return errCertificate
			}
			return nil
		},
	}, nil
}

func transportPeer(state *tls.ConnectionState) (runtimeaccess.Peer, error) {
	if state == nil || !state.HandshakeComplete {
		return runtimeaccess.Peer{}, errCertificate
	}
	return certificatePeer(state)
}

// TransportPeer gives a composing listener the same verified, current TLS
// peer as these private endpoints.
func TransportPeer(state *tls.ConnectionState) (runtimeaccess.Peer, error) {
	return transportPeer(state)
}

func certificatePeer(state *tls.ConnectionState) (runtimeaccess.Peer, error) {
	if state == nil || state.Version != tls.VersionTLS13 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return runtimeaccess.Peer{}, errCertificate
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || len(leaf.Raw) == 0 || len(leaf.DNSNames) != 1 || !runtimeaccess.ValidDNSName(leaf.DNSNames[0]) || len(leaf.IPAddresses) != 0 || len(leaf.URIs) != 0 || len(leaf.EmailAddresses) != 0 {
		return runtimeaccess.Peer{}, errCertificate
	}
	verified := false
	now := time.Now()
	for _, chain := range state.VerifiedChains {
		if len(chain) > 0 && chain[0] != nil && leaf.Equal(chain[0]) {
			current := true
			for _, certificate := range chain {
				if certificate == nil || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
					current = false
					break
				}
			}
			if current {
				verified = true
				break
			}
		}
	}
	if !verified {
		return runtimeaccess.Peer{}, errCertificate
	}
	digest := sha256.Sum256(leaf.Raw)
	return runtimeaccess.Peer{DNSName: strings.ToLower(leaf.DNSNames[0]), CertificateFingerprint: "sha256:" + hex.EncodeToString(digest[:])}, nil
}
