// Package casereadhttp is the private Processing owner read listener. It is not
// a browser/BFF implementation and has no runtime IAM or permissive fallback.
package casereadhttp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

type Handler struct {
	service readcases.Service
	peerDNS string
}

func New(s readcases.Service, peerDNS string) (http.Handler, error) {
	if peerDNS == "" || s.Authority == nil || s.Boundary == nil || s.Clock == nil || s.Repository == nil {
		return nil, readcases.ErrUnavailable
	}
	return Handler{s, peerDNS}, nil
}

// NewServer requires explicit owner certificate, trusted client CA and BFF SAN.
// The production port is 8443 on a private interface; tests may use a loopback
// ephemeral listener. Nothing binds a socket until the caller explicitly serves.
func NewServer(address string, t *tls.Config, s readcases.Service, peerDNS string) (*http.Server, error) {
	host, port, e := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || (port != "8443" && !(ip.IsLoopback() && port == "0")) {
		return nil, readcases.ErrUnavailable
	}
	h, e := New(s, peerDNS)
	if e != nil {
		return nil, e
	}
	if t == nil || t.ClientCAs == nil || len(t.Certificates) == 0 || t.InsecureSkipVerify || t.GetConfigForClient != nil {
		return nil, readcases.ErrUnavailable
	}
	config := t.Clone()
	config.MinVersion = tls.VersionTLS13
	config.MaxVersion = tls.VersionTLS13
	config.ClientAuth = tls.RequireAndVerifyClientCert
	config.NextProtos = []string{"http/1.1"}
	return &http.Server{Addr: address, Handler: h, TLSConfig: config, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}, nil
}

type wireError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func fail(w http.ResponseWriter, e error) {
	status, code := 503, "READ_UNAVAILABLE"
	switch {
	case errors.Is(e, readcases.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(e, readcases.ErrUnauthenticated):
		status, code = 401, "UNAUTHENTICATED"
	case errors.Is(e, readcases.ErrDenied):
		status, code = 403, "READ_DENIED"
	case errors.Is(e, readcases.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		w.WriteHeader(503)
		return
	}
	b[6] = b[6]&15 | 64
	b[8] = b[8]&63 | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(wireError{code, "Request could not be completed", id})
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// VerifiedChains is produced by actual TLS verification, never by a header.
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.peerDNS) != nil {
		fail(w, readcases.ErrDenied)
		return
	}
	if r.Method != "GET" {
		fail(w, readcases.ErrInvalid)
		return
	}
	// These headers are neither forwarded nor consulted as authority. Rejecting
	// them also prevents an accidental future interpretation downstream.
	for k := range r.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-hitpipe-") && lower != "x-hitpipe-session" {
			fail(w, readcases.ErrInvalid)
			return
		}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) > 3 {
		parts[3] = strings.ToLower(parts[3])
	}
	if len(parts) > 5 {
		parts[5] = strings.ToLower(parts[5])
	}
	if len(parts) < 5 || len(parts) > 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workspaces" || parts[4] != "cases" || !readcases.UUID(parts[3]) {
		fail(w, readcases.ErrInvalid)
		return
	}
	q := readcases.Request{WorkspaceID: parts[3], Kind: readcases.List, Limit: 100}
	if len(parts) >= 6 {
		q.CaseID = parts[5]
		q.Kind = readcases.Detail
		if !readcases.UUID(q.CaseID) {
			fail(w, readcases.ErrInvalid)
			return
		}
	}
	if len(parts) == 7 {
		if parts[6] != "history" {
			fail(w, readcases.ErrInvalid)
			return
		}
		q.Kind = readcases.History
	}
	// url.Values.ParseQuery errors must not be silently dropped by URL.Query.
	values, e := url.ParseQuery(r.URL.RawQuery)
	if e != nil {
		fail(w, readcases.ErrInvalid)
		return
	}
	for key, v := range values {
		if len(v) != 1 {
			fail(w, readcases.ErrInvalid)
			return
		}
		switch key {
		case "limit":
			if q.Kind != readcases.List {
				fail(w, readcases.ErrInvalid)
				return
			}
			q.Limit, e = strconv.Atoi(v[0])
			if e != nil || q.Limit < 1 || q.Limit > 100 {
				fail(w, readcases.ErrInvalid)
				return
			}
		case "state":
			if q.Kind != readcases.List || !readcases.State(v[0]) {
				fail(w, readcases.ErrInvalid)
				return
			}
			q.State = v[0]
		case "cursor":
			if q.Kind == readcases.Detail || len(v[0]) < 1 || len(v[0]) > 255 {
				fail(w, readcases.ErrInvalid)
				return
			}
			q.Cursor = v[0]
		default:
			fail(w, readcases.ErrInvalid)
			return
		}
	}
	sessions := r.Header.Values("X-HitPipe-Session")
	if len(sessions) != 1 || sessions[0] == "" || len(sessions[0]) > 4096 {
		fail(w, readcases.ErrUnauthenticated)
		return
	}
	q.Session = sessions[0]
	wrote := false
	e = h.service.Execute(r.Context(), q, func(view any) error {
		body, e := json.Marshal(view)
		if e != nil || len(body) > 2<<20 {
			return readcases.ErrUnavailable
		}
		if r.Context().Err() != nil {
			return r.Context().Err()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		wrote = true
		w.WriteHeader(200)
		_, e = w.Write(append(body, '\n'))
		return e
	})
	if e != nil && !wrote {
		fail(w, e)
	}
}
