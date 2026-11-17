// Package activationhttp attaches to the existing private owner server before serving.
package activationhttp

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"io"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	Service activation.Service
	PeerDNS string
	MaxBody int64
}

func Attach(server *http.Server, s activation.Service, peer string, maxBody int64) error {
	if server == nil || server.Handler == nil || !s.Ready() || peer == "" || maxBody != 256<<10 {
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
	old := server.Handler
	h := Handler{s, peer, maxBody}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/operations/activate") {
			h.ServeHTTP(w, r)
		} else {
			old.ServeHTTP(w, r)
		}
	})
	return nil
}
func fail(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "ACTIVATION_UNAVAILABLE"
	switch {
	case errors.Is(e, drafts.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(e, drafts.ErrUnauthenticated):
		status, code = 401, "UNAUTHENTICATED"
	case errors.Is(e, drafts.ErrDenied):
		status, code = 403, "ACTIVATION_ACCESS_DENIED"
	case errors.Is(e, drafts.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(e, drafts.ErrConflict):
		status, code = 409, "COMMAND_CONFLICT"
	case errors.Is(e, drafts.ErrStale):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	}
	var size *http.MaxBytesError
	if errors.As(e, &size) {
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	id := r.Header.Get("X-Request-ID")
	if !drafts.UUID(id) {
		id, _ = drafts.NewID()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(drafts.Error{Code: code, Message: "Activation request could not be completed", RequestID: id})
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bad := func(e error) { fail(w, r, e) }
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.PeerDNS) != nil {
		bad(drafts.ErrDenied)
		return
	}
	for k := range r.Header {
		key := strings.ToLower(k)
		if strings.HasPrefix(key, "x-hitpipe-") && key != "x-hitpipe-session" {
			bad(drafts.ErrInvalid)
			return
		}
	}
	parts := strings.Split(r.URL.Path, "/")
	if r.Method != "POST" || len(parts) != 7 || parts[1] != "api" || parts[2] != "v1" || parts[3] != "workspaces" || !drafts.UUID(parts[4]) || parts[5] != "operations" || parts[6] != "activate" || r.URL.RawPath != "" || r.URL.RawQuery != "" {
		bad(drafts.ErrInvalid)
		return
	}
	one := func(key string) string {
		v := r.Header.Values(key)
		if len(v) != 1 {
			return ""
		}
		return v[0]
	}
	q := activation.Request{WorkspaceID: parts[4], Session: one("X-HitPipe-Session"), CSRF: one("X-CSRF-Token"), RequestID: one("X-Request-ID")}
	if q.Session == "" || len(q.Session) > 4096 {
		bad(drafts.ErrUnauthenticated)
		return
	}
	if len(q.CSRF) < 32 || len(q.CSRF) > 128 || !drafts.UUID(q.RequestID) {
		bad(drafts.ErrInvalid)
		return
	}
	med, _, e := mime.ParseMediaType(one("Content-Type"))
	if e != nil || med != "application/json" || r.Header.Get("Content-Encoding") != "" {
		bad(drafts.ErrInvalid)
		return
	}
	if v := r.Header.Values("If-Match"); len(v) > 0 {
		raw := one("If-Match")
		if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
			raw = raw[1 : len(raw)-1]
		}
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || n < 1 || n > 9007199254740991 {
			bad(drafts.ErrInvalid)
			return
		}
		q.IfMatch = &n
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxBody)
	q.Body, e = io.ReadAll(r.Body)
	if e != nil {
		bad(e)
		return
	}
	wrote := false
	e = h.Service.Execute(r.Context(), q, func(body []byte, status int) error {
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
