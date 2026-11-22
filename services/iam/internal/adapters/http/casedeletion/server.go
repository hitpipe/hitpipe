// Package casedeletion carries the publisher's case-content deletion snapshots
// and narrowly scoped receipts over a private, provisioned Processing channel.
// It does not implement owner authority, deletion, or a public IAM endpoint.
package casedeletion

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const (
	SnapshotPath = "/internal/maintenance/v1/case-deletion/snapshot"
	ReceiptPath  = "/internal/maintenance/v1/case-deletion/receipt"

	MaxReceiptBytes  = 8 * 1024
	MaxSnapshotBytes = 4 * 1024 * 1024
	maxHeaderBytes   = 8 * 1024
	exchangeTimeout  = 5 * time.Second
)

var (
	errConfiguration = errors.New("case deletion maintenance: incomplete trusted configuration")
	errTransport     = errors.New("case deletion maintenance: exact authenticated response unavailable")
	errWire          = errors.New("case deletion maintenance: invalid wire value")
)

// Peer is a Processing identity provisioned outside requests. Workspaces is an
// explicit nonempty read/receipt allowlist. Credentials, DNS and a claimed peer
// ID are never themselves taken from a receipt or HTTP header.
type Peer struct {
	ID         string
	Transport  runtimeaccess.Peer
	Workspaces []string
}

type Backend interface {
	Snapshot(context.Context, string, uint64) (app.Snapshot, error)
	AcceptReceipt(context.Context, string, app.Receipt) error
}

type binding struct {
	id         string
	workspaces map[string]struct{}
}

type handler struct {
	backend Backend
	peers   map[runtimeaccess.Peer]binding
}

func inventory(peers []Peer) (map[runtimeaccess.Peer]binding, []runtimeaccess.Peer, error) {
	if len(peers) == 0 {
		return nil, nil, errConfiguration
	}
	allowed := make(map[runtimeaccess.Peer]binding, len(peers))
	transport := make([]runtimeaccess.Peer, 0, len(peers))
	identities := make(map[string]bool, len(peers))
	for _, peer := range peers {
		credential := peer.Transport
		if !peerID(peer.ID) || identities[peer.ID] || !runtimeaccess.ValidDNSName(credential.DNSName) ||
			!runtimeaccess.ValidFingerprint(credential.CertificateFingerprint) || len(peer.Workspaces) == 0 {
			return nil, nil, errConfiguration
		}
		credential.DNSName = strings.ToLower(credential.DNSName)
		if _, exists := allowed[credential]; exists {
			return nil, nil, errConfiguration
		}
		workspaces := make(map[string]struct{}, len(peer.Workspaces))
		for _, workspace := range peer.Workspaces {
			if _, exists := workspaces[workspace]; exists || !workspaceID(workspace) {
				return nil, nil, errConfiguration
			}
			workspaces[workspace] = struct{}{}
		}
		identities[peer.ID] = true
		allowed[credential] = binding{id: peer.ID, workspaces: workspaces}
		transport = append(transport, credential)
	}
	return allowed, transport, nil
}

func peerID(s string) bool {
	if len(s) < 1 || len(s) > 96 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func workspaceID(s string) bool {
	return app.ValidID(s)
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	value := reflect.ValueOf(v)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}

// New builds the dedicated handler. Serve it on the TLS configuration produced
// by TLSConfig; NewServer combines them without exposing a bind address.
func New(backend Backend, peers []Peer) (http.Handler, error) {
	if isNil(backend) {
		return nil, errConfiguration
	}
	allowed, _, err := inventory(peers)
	if err != nil {
		return nil, err
	}
	return handler{backend: backend, peers: allowed}, nil
}

// TLSConfig reuses the accepted private IAM certificate checks, with a distinct
// provisioned Processing client CA and allowlist provided by the maintainer.
func TLSConfig(serverCertificate tls.Certificate, processingClientCAs *x509.CertPool, peers []Peer) (*tls.Config, error) {
	_, credentials, err := inventory(peers)
	if err != nil {
		return nil, err
	}
	config, err := privateiam.TLSConfig(serverCertificate, processingClientCAs, credentials)
	if err != nil {
		return nil, errConfiguration
	}
	return config, nil
}

// NewServer leaves the listening socket to the controlled deployment fixture;
// there is no default public bind address and no mount on the public IAM mux.
func NewServer(backend Backend, serverCertificate tls.Certificate, processingClientCAs *x509.CertPool, peers []Peer) (*http.Server, error) {
	h, err := New(backend, peers)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := TLSConfig(serverCertificate, processingClientCAs, peers)
	if err != nil {
		return nil, err
	}
	return &http.Server{
		Handler: h, TLSConfig: tlsConfig,
		ReadHeaderTimeout: time.Second, ReadTimeout: exchangeTimeout,
		WriteTimeout: exchangeTimeout, IdleTimeout: exchangeTimeout,
		MaxHeaderBytes: maxHeaderBytes,
	}, nil
}

func fingerprint(cert *x509.Certificate) string {
	if cert == nil || len(cert.Raw) == 0 {
		return ""
	}
	digest := sha256.Sum256(cert.Raw)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func currentLeaf(state *tls.ConnectionState) *x509.Certificate {
	if state == nil || state.Version != tls.VersionTLS13 || len(state.PeerCertificates) == 0 || len(state.VerifiedChains) == 0 {
		return nil
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || len(leaf.Raw) == 0 {
		return nil
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
			return leaf
		}
	}
	return nil
}

func (h handler) binding(r *http.Request) (binding, bool) {
	if r.TLS == nil || !r.TLS.HandshakeComplete {
		return binding{}, false
	}
	leaf := currentLeaf(r.TLS)
	if leaf == nil || len(leaf.DNSNames) != 1 || !runtimeaccess.ValidDNSName(leaf.DNSNames[0]) ||
		len(leaf.IPAddresses) != 0 || len(leaf.URIs) != 0 || len(leaf.EmailAddresses) != 0 {
		return binding{}, false
	}
	key := runtimeaccess.Peer{DNSName: strings.ToLower(leaf.DNSNames[0]), CertificateFingerprint: fingerprint(leaf)}
	peer, ok := h.peers[key]
	return peer, ok
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	peer, ok := h.binding(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if r.URL.IsAbs() || r.URL.EscapedPath() != r.URL.Path {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), exchangeTimeout)
	defer cancel()
	switch r.URL.Path {
	case SnapshotPath:
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		workspace, generation, err := snapshotQuery(r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, allowed := peer.workspaces[workspace]; !allowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		snapshot, err := h.backend.Snapshot(ctx, workspace, generation)
		if err != nil {
			w.WriteHeader(status(err))
			return
		}
		if !validSnapshot(snapshot, workspace, generation) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		data, err := json.Marshal(snapshot)
		if err != nil || len(data) > MaxSnapshotBytes || ctx.Err() != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		if n, err := w.Write(data); err != nil || n != len(data) {
			panic(http.ErrAbortHandler)
		}
	case ReceiptPath:
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || !jsonContent(r.Header) || r.Body == nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.ContentLength > MaxReceiptBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, MaxReceiptBytes+1))
		if len(data) > MaxReceiptBytes {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		receipt, parseErr := decodeReceipt(data)
		if err != nil || parseErr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if _, allowed := peer.workspaces[receipt.WorkspaceID]; !allowed {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if err := ctx.Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if err := h.backend.AcceptReceipt(ctx, peer.id, receipt); err != nil {
			w.WriteHeader(status(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func snapshotQuery(r *http.Request) (string, uint64, error) {
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.URL.RawQuery) > 160 {
		return "", 0, errWire
	}
	v, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(v) != 2 || len(v["workspace_id"]) != 1 || len(v["generation"]) != 1 {
		return "", 0, errWire
	}
	scope, wireGeneration := v.Get("workspace_id"), v.Get("generation")
	generation, err := strconv.ParseUint(wireGeneration, 10, 64)
	if err != nil || !workspaceID(scope) || !app.ValidVersion(generation) || strconv.FormatUint(generation, 10) != wireGeneration {
		return "", 0, errWire
	}
	return scope, generation, nil
}

func jsonContent(h http.Header) bool {
	values := h.Values("Content-Type")
	if len(values) != 1 || len(h.Values("Content-Encoding")) > 0 {
		return false
	}
	mt, params, err := mime.ParseMediaType(values[0])
	return err == nil && mt == "application/json" && len(params) == 0
}
