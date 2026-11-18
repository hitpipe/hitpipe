// Package lifecyclehttp adds operation lifecycle routes to the existing
// private Processing listener.
package lifecyclehttp

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
)

type handler struct {
	service lifecycle.Service
	peer    string
}

func Attach(server *http.Server, service lifecycle.Service, peer string) error {
	if !service.Ready() || peer == "" {
		return drafts.ErrUnavailable
	}
	if err := drafthttp.CheckPrivateListener(server); err != nil {
		return err
	}
	previous := server.Handler
	h := handler{service, peer}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Split(r.URL.Path, "/")
		if len(p) >= 6 && p[1] == "api" && p[2] == "v1" && p[3] == "workspaces" && p[5] == "operations" &&
			(len(p) == 6 || len(p) == 8 && (p[7] == "lifecycle" || p[7] == "resume-snapshot")) {
			h.serve(w, r, p)
			return
		}
		previous.ServeHTTP(w, r)
	})
	return nil
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	status, code := 503, "LIFECYCLE_UNAVAILABLE"
	switch {
	case errors.Is(err, drafts.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(err, drafts.ErrUnauthenticated):
		status, code = 401, "UNAUTHENTICATED"
	case errors.Is(err, drafts.ErrDenied):
		status, code = 403, "LIFECYCLE_ACCESS_DENIED"
	case errors.Is(err, drafts.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(err, drafts.ErrConflict):
		status, code = 409, "COMMAND_CONFLICT"
	case errors.Is(err, drafts.ErrStale):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	}
	var size *http.MaxBytesError
	if errors.As(err, &size) {
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	id := r.Header.Get("X-Request-ID")
	if !drafts.UUID(id) {
		id, err = drafts.NewID()
		if err != nil {
			w.WriteHeader(503)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(drafts.Error{Code: code, RequestID: id, Message: "The operation lifecycle request could not be completed"})
}

func (h handler) serve(w http.ResponseWriter, r *http.Request, path []string) {
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 ||
		len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.peer) != nil {
		fail(w, r, drafts.ErrDenied)
		return
	}
	for name := range r.Header {
		key := strings.ToLower(name)
		if strings.HasPrefix(key, "x-hitpipe-") && key != "x-hitpipe-session" {
			fail(w, r, drafts.ErrInvalid)
			return
		}
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		fail(w, r, drafts.ErrInvalid)
		return
	}
	header := func(key string) string {
		values := r.Header.Values(key)
		if len(values) != 1 {
			return ""
		}
		return values[0]
	}
	q := lifecycle.Request{WorkspaceID: path[4], Session: header("X-HitPipe-Session")}
	switch {
	case len(path) == 6 && r.Method == http.MethodGet:
		q.Kind = lifecycle.List
	case len(path) == 8 && path[7] == "resume-snapshot" && r.Method == http.MethodGet:
		q.Kind, q.OperationID = lifecycle.Snapshot, path[6]
	case len(path) == 8 && path[7] == "lifecycle" && r.Method == http.MethodPost:
		q.Kind, q.OperationID, q.CSRF, q.RequestID = lifecycle.Change, path[6], header("X-CSRF-Token"), header("X-Request-ID")
		media, _, err := mime.ParseMediaType(header("Content-Type"))
		if err != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			fail(w, r, drafts.ErrInvalid)
			return
		}
		if len(r.Header.Values("If-Match")) != 0 {
			raw := header("If-Match")
			if len(raw) > 1 && raw[0] == '"' && raw[len(raw)-1] == '"' {
				raw = raw[1 : len(raw)-1]
			}
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n < 1 || n > 9007199254740991 || strconv.FormatInt(n, 10) != raw {
				fail(w, r, drafts.ErrInvalid)
				return
			}
			q.IfMatch = &n
		}
		q.Body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
		if err != nil {
			fail(w, r, err)
			return
		}
	default:
		fail(w, r, drafts.ErrInvalid)
		return
	}
	if q.Kind != lifecycle.Change && (r.ContentLength != 0 || len(r.TransferEncoding) != 0 || len(r.Header.Values("If-Match")) != 0) {
		fail(w, r, drafts.ErrInvalid)
		return
	}
	wrote := false
	err := h.service.Execute(r.Context(), q, func(body []byte, status int) error {
		if err := r.Context().Err(); err != nil {
			return err
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		wrote = true
		w.WriteHeader(status)
		_, err := w.Write(append(body, '\n'))
		return err
	})
	if err != nil && !wrote {
		fail(w, r, err)
	}
}
