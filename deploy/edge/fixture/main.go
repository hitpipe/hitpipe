// The HIT-141 TLS peer is a test fixture. It has no product routes or authority.
package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxBodyBytes = 256 * 1024
	deadline     = 4 * time.Second
)

var errPeer = errors.New("unexpected fixture client identity")

type observation struct {
	Role                  string `json:"role"`
	Method                string `json:"method"`
	Path                  string `json:"path"`
	TLSVersion            string `json:"tls_version"`
	PeerDNS               string `json:"peer_dns"`
	HitPipeHeaderPresent  bool   `json:"hitpipe_header_present"`
	AuthorizationPresent  bool   `json:"authorization_present"`
	CookiePresent         bool   `json:"cookie_present"`
	StripeSignatureSHA256 string `json:"stripe_signature_sha256"`
	BodySHA256            string `json:"body_sha256"`
	BodyBytes             int64  `json:"body_bytes"`
}

func edgePeer(state tls.ConnectionState) error {
	if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return errPeer
	}
	leaf := state.PeerCertificates[0]
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "edge" ||
		len(leaf.IPAddresses)+len(leaf.EmailAddresses)+len(leaf.URIs) != 0 {
		return errPeer
	}
	return nil
}

func tlsSettings(role string, certificate tls.Certificate, ca *x509.CertPool) *tls.Config {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
	}
	if role != "keycloak" {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = ca
		cfg.VerifyConnection = edgePeer
	}
	return cfg
}

func observe(role string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 ||
			(role != "keycloak" && edgePeer(*r.TLS) != nil) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		defer r.Body.Close()
		o := observation{Role: role, Method: r.Method, Path: r.URL.Path, TLSVersion: "TLS1.3"}
		if role != "keycloak" {
			o.PeerDNS = "edge"
		}
		for name := range r.Header {
			lower := strings.ToLower(name)
			o.HitPipeHeaderPresent = o.HitPipeHeaderPresent || strings.HasPrefix(lower, "x-hitpipe-")
			o.AuthorizationPresent = o.AuthorizationPresent || lower == "authorization"
			o.CookiePresent = o.CookiePresent || lower == "cookie"
		}
		signatures := r.Header.Values("Stripe-Signature")
		if len(signatures) > 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(signatures) == 1 {
			digest := sha256.Sum256([]byte(signatures[0]))
			o.StripeSignatureSHA256 = hex.EncodeToString(digest[:])
		}
		// Do not retain, echo or log the request body, header contents or query.
		digest := sha256.New()
		n, err := io.Copy(digest, http.MaxBytesReader(w, r.Body, maxBodyBytes))
		if err != nil {
			var over *http.MaxBytesError
			if errors.As(err, &over) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}
		o.BodyBytes = n
		o.BodySHA256 = hex.EncodeToString(digest.Sum(nil))
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(o)
	})
}

func httpsFixture(role, certPath, keyPath, caPath string) error {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return errors.New("fixture certificate unavailable")
	}
	var roots *x509.CertPool
	if role != "keycloak" {
		raw, err := os.ReadFile(caPath)
		if err != nil {
			return errors.New("fixture client trust unavailable")
		}
		roots = x509.NewCertPool()
		if !roots.AppendCertsFromPEM(raw) {
			return errors.New("fixture client trust invalid")
		}
	}
	server := &http.Server{
		Handler:           observe(role),
		TLSConfig:         tlsSettings(role, cert, roots),
		ReadHeaderTimeout: deadline,
		ReadTimeout:       deadline,
		WriteTimeout:      deadline,
		IdleTimeout:       deadline,
		MaxHeaderBytes:    32 * 1024,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	listener, err := net.Listen("tcp", ":8443")
	if err != nil {
		return errors.New("fixture HTTPS listener unavailable")
	}
	defer listener.Close()
	fmt.Println("fixture ready")
	if err := server.ServeTLS(listener, "", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("fixture HTTPS listener stopped unexpectedly")
	}
	return nil
}

func privateFixture() error {
	// These are active harmless sentinels, not real database/OPS/business services.
	ports := []string{"5432", "8443", "9443", "9444"}
	listeners := make([]net.Listener, 0, len(ports))
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for _, port := range ports {
		l, err := net.Listen("tcp", ":"+port)
		if err != nil {
			return errors.New("fixture private listener unavailable")
		}
		listeners = append(listeners, l)
	}
	stopped := make(chan struct{}, len(listeners))
	for _, l := range listeners {
		go func() {
			defer func() { stopped <- struct{}{} }()
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				_ = c.SetWriteDeadline(time.Now().Add(time.Second))
				_, _ = io.WriteString(c, "fixture-private\n")
				_ = c.Close()
			}
		}()
	}
	fmt.Println("fixture ready")
	<-stopped
	return errors.New("fixture private listener stopped unexpectedly")
}

func main() {
	role := flag.String("role", "", "synthetic iam, bff, intake, keycloak, or private peer")
	cert := flag.String("cert", "/tls/server.crt", "fixture server certificate")
	key := flag.String("key", "/tls/server.key", "fixture server key")
	ca := flag.String("ca", "/tls/service-ca.crt", "fixture service CA")
	flag.Parse()
	var err error
	if flag.NArg() != 0 {
		err = errors.New("unexpected fixture argument")
	} else {
		switch *role {
		case "iam", "bff", "intake", "keycloak":
			err = httpsFixture(*role, *cert, *key, *ca)
		case "private":
			err = privateFixture()
		default:
			err = errors.New("unknown fixture role")
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
