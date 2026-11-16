// Package drafthttp is only the private Processing owner listener. It does not
// implement a browser/BFF, real IAM, provider access or automatic activation.
package drafthttp

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Handler struct {
	Service drafts.Service
	PeerDNS string
	MaxBody int64
}

func New(s drafts.Service, peer string, maxBody int64) (http.Handler, error) {
	if !s.Ready() || peer == "" || maxBody < 1 || maxBody == math.MaxInt64 {
		return nil, drafts.ErrUnavailable
	}
	return Handler{s, peer, maxBody}, nil
}
func NewServer(address string, t *tls.Config, s drafts.Service, peer string, maxBody int64) (*http.Server, error) {
	host, port, e := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || (port != "8443" && !(ip.IsLoopback() && port == "0")) {
		return nil, drafts.ErrUnavailable
	}
	h, e := New(s, peer, maxBody)
	if e != nil {
		return nil, e
	}
	if t == nil || t.ClientCAs == nil || len(t.Certificates) == 0 || t.InsecureSkipVerify || t.GetConfigForClient != nil {
		return nil, drafts.ErrUnavailable
	}
	cfg := t.Clone()
	cfg.MinVersion = tls.VersionTLS13
	cfg.MaxVersion = tls.VersionTLS13
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	cfg.NextProtos = []string{"http/1.1"}
	return &http.Server{Addr: address, Handler: h, TLSConfig: cfg, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}, nil
}
func fail(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "DRAFT_UNAVAILABLE"
	switch {
	case errors.Is(e, drafts.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(e, drafts.ErrUnauthenticated):
		status, code = 401, "UNAUTHENTICATED"
	case errors.Is(e, drafts.ErrDenied):
		status, code = 403, "DRAFT_ACCESS_DENIED"
	case errors.Is(e, drafts.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(e, drafts.ErrConflict):
		status, code = 409, "COMMAND_CONFLICT"
	case errors.Is(e, drafts.ErrStale):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	}
	var bound *http.MaxBytesError
	if errors.As(e, &bound) {
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	id := r.Header.Get("X-Request-ID")
	if !drafts.UUID(id) {
		var err error
		id, err = drafts.NewID()
		if err != nil {
			w.WriteHeader(503)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(drafts.Error{Code: code, Message: "Draft request could not be completed", RequestID: id})
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bad := func(e error) { fail(w, r, e) }
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.PeerDNS) != nil {
		bad(drafts.ErrDenied)
		return
	}
	for k := range r.Header {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "x-hitpipe-") && lower != "x-hitpipe-session" {
			bad(drafts.ErrInvalid)
			return
		}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) < 5 || len(parts) > 7 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "workspaces" || !drafts.UUID(parts[3]) || r.URL.RawQuery != "" {
		bad(drafts.ErrInvalid)
		return
	}
	q := drafts.Request{WorkspaceID: parts[3]}
	switch {
	case len(parts) == 5 && parts[4] == "drafts" && r.Method == http.MethodPost:
		q.Kind = drafts.Save
	case len(parts) == 6 && parts[4] == "drafts" && r.Method == http.MethodGet:
		q.Kind = drafts.Get
		q.DraftID = parts[5]
	case len(parts) == 7 && parts[4] == "drafts" && parts[6] == "readiness" && r.Method == http.MethodPost:
		q.Kind = drafts.Check
		q.DraftID = parts[5]
	case len(parts) == 5 && parts[4] == "rehearsals" && r.Method == http.MethodPost:
		q.Kind = drafts.Rehearse
	default:
		bad(drafts.ErrInvalid)
		return
	}
	if (q.Kind == drafts.Get || q.Kind == drafts.Check) && !drafts.UUID(q.DraftID) {
		bad(drafts.ErrInvalid)
		return
	}
	session := r.Header.Values("X-HitPipe-Session")
	if len(session) != 1 || session[0] == "" || len(session[0]) > 4096 {
		bad(drafts.ErrUnauthenticated)
		return
	}
	q.Session = session[0]
	if q.Kind != drafts.Get {
		csrf := r.Header.Values("X-CSRF-Token")
		correlation := r.Header.Values("X-Request-ID")
		if len(csrf) != 1 || len(csrf[0]) < 32 || len(csrf[0]) > 128 || len(correlation) != 1 || !drafts.UUID(correlation[0]) {
			bad(drafts.ErrInvalid)
			return
		}
		q.CSRF = csrf[0]
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			bad(drafts.ErrInvalid)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, h.MaxBody)
		q.Body, e = io.ReadAll(r.Body)
		if e != nil {
			bad(e)
			return
		}
		if match := r.Header.Values("If-Match"); len(match) > 0 {
			if len(match) != 1 {
				bad(drafts.ErrInvalid)
				return
			}
			raw := match[0]
			if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
				raw = strings.Trim(raw, "\"")
			}
			n, e := strconv.ParseInt(raw, 10, 64)
			if e != nil || n < 1 || n > 9007199254740991 {
				bad(drafts.ErrInvalid)
				return
			}
			q.IfMatch = &n
		}
	} else if r.ContentLength != 0 {
		bad(drafts.ErrInvalid)
		return
	}
	wrote := false
	e := h.Service.Execute(r.Context(), q, func(body []byte, status int) error {
		if r.Context().Err() != nil {
			return r.Context().Err()
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		wrote = true
		w.WriteHeader(status)
		_, e := w.Write(append(body, '\n'))
		return e
	})
	if e != nil && !wrote {
		bad(e)
	}
}

// Attach retains other owner routes on the same private server. Call before ServeTLS.
func Attach(server *http.Server, s drafts.Service, peer string, maxBody int64) error {
	h, e := New(s, peer, maxBody)
	if e != nil {
		return e
	}
	if server == nil || server.Handler == nil {
		return drafts.ErrUnavailable
	}
	host, port, e := net.SplitHostPort(server.Addr)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || (port != "8443" && !(ip.IsLoopback() && port == "0")) {
		return drafts.ErrUnavailable
	}
	t := server.TLSConfig
	if t == nil || t.ClientCAs == nil || len(t.Certificates) == 0 || t.ClientAuth != tls.RequireAndVerifyClientCert || t.MinVersion != tls.VersionTLS13 || t.MaxVersion != tls.VersionTLS13 || t.InsecureSkipVerify || t.GetConfigForClient != nil {
		return drafts.ErrUnavailable
	}
	previous := server.Handler
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 5 && parts[1] == "api" && parts[2] == "v1" && parts[3] == "workspaces" && len(parts) >= 6 && (parts[5] == "drafts" || parts[5] == "rehearsals") {
			h.ServeHTTP(w, r)
			return
		}
		previous.ServeHTTP(w, r)
	})
	return nil
}
