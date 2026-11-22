package iamprocess

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	wire "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const healthPath = "/internal/v1/health/iam"

type route struct {
	next  http.Handler
	peers map[runtimeaccess.Peer]struct{}
}

type routes map[string]route

func peerSet(peers []runtimeaccess.Peer) map[runtimeaccess.Peer]struct{} {
	set := make(map[runtimeaccess.Peer]struct{}, len(peers))
	for _, peer := range peers {
		peer.DNSName = strings.ToLower(peer.DNSName)
		set[peer] = struct{}{}
	}
	return set
}

func (rt routes) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	target, exists := rt[r.URL.EscapedPath()]
	if !exists {
		wire.Reject(w, r, http.StatusNotFound, "NOT_FOUND", "The requested operation is unavailable.")
		return
	}
	peer, err := wire.TransportPeer(r.TLS)
	if err != nil {
		wire.Reject(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Verified client transport is required.")
		return
	}
	if _, allowed := target.peers[peer]; !allowed {
		wire.Reject(w, r, http.StatusForbidden, "DENIED", "The client is not permitted for this operation.")
		return
	}
	target.next.ServeHTTP(w, r)
}

func businessHandler(config Config, auth, profile, control http.Handler) http.Handler {
	edges := peerSet(config.EdgePeers)
	bffs := peerSet(config.BFFPeers)
	peers := make([]runtimeaccess.Peer, 0, len(config.RuntimeBindings))
	for _, binding := range config.RuntimeBindings {
		peers = append(peers, runtimeaccess.Peer{DNSName: binding.DNSName,
			CertificateFingerprint: binding.CertificateFingerprint})
	}
	runtimes := peerSet(peers)
	return routes{
		"/auth/login":      {auth, edges},
		"/auth/callback":   {auth, edges},
		"/api/v1/profile":  {profile, bffs},
		wire.AuthorizePath: {control, runtimes},
		wire.RegisterPath:  {control, runtimes},
	}
}

type technicalState interface {
	TechnicalOpen(context.Context) bool
}

type health struct {
	Status          string    `json:"status"`
	Service         string    `json:"service"`
	BuildRevision   string    `json:"build_revision"`
	ProtocolVersion string    `json:"protocol_version"`
	CheckedAt       time.Time `json:"checked_at"`
}

func opsHandler(config Config, state technicalState) http.Handler {
	check := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.ForceQuery ||
			r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			wire.Reject(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The request is unsupported.")
			return
		}
		status := "CLOSED"
		if state.TechnicalOpen(r.Context()) {
			status = "LIVE"
		}
		if r.Context().Err() != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(health{
			Status: status, Service: "IAM", BuildRevision: config.BuildRevision,
			ProtocolVersion: "1.0", CheckedAt: time.Now().UTC(),
		})
	})
	return routes{healthPath: {check, peerSet(config.OPSPeers)}}
}
