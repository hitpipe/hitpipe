// Package recheckhttp mounts the accepted recoverCase operation.
package recheckhttp

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/recheck"
)

type Codec struct{}

func (Codec) Decode(body []byte) (recheck.Input, error) {
	var in recheck.Input
	if draftjson.Validate("RecoveryCommand", body) != nil {
		return in, drafts.ErrInvalid
	}
	canonical, err := jcs.Transform(body)
	if err != nil || json.Unmarshal(canonical, &in.Command) != nil || draftjson.KnownSecret(in.Command.Reason) {
		return recheck.Input{}, drafts.ErrInvalid
	}
	in.Digest = draftjson.Hash(canonical)
	return in, nil
}

func Attach(server *http.Server, service recheck.Service, peer string) error {
	if !service.Ready() || peer == "" {
		return drafts.ErrUnavailable
	}
	if err := drafthttp.CheckPrivateListener(server); err != nil {
		return err
	}
	previous := server.Handler
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Split(r.URL.Path, "/")
		if len(p) != 8 || p[1] != "api" || p[2] != "v1" || p[3] != "workspaces" || p[5] != "cases" || p[7] != "recovery" {
			previous.ServeHTTP(w, r)
			return
		}
		serve(w, r, recheck.Request{WorkspaceID: p[4], CaseID: p[6]}, service, peer)
	})
	return nil
}

func single(r *http.Request, name string) string {
	v := r.Header.Values(name)
	if len(v) != 1 {
		return ""
	}
	return v[0]
}

func serve(w http.ResponseWriter, r *http.Request, q recheck.Request, s recheck.Service, peer string) {
	q.RequestID = single(r, "X-Request-ID")
	fail := func(err error) { writeError(w, q.RequestID, err) }
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 ||
		len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(peer) != nil {
		fail(drafts.ErrDenied)
		return
	}
	if r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		fail(drafts.ErrInvalid)
		return
	}
	for h := range r.Header {
		key := strings.ToLower(h)
		if strings.HasPrefix(key, "x-hitpipe-") && key != "x-hitpipe-session" {
			fail(drafts.ErrInvalid)
			return
		}
	}
	q.Session = single(r, "X-HitPipe-Session")
	q.CSRF = single(r, "X-CSRF-Token")
	if len(q.Session) < 43 || len(q.Session) > 128 {
		fail(drafts.ErrUnauthenticated)
		return
	}
	if len(q.CSRF) < 32 || len(q.CSRF) > 128 {
		fail(drafts.ErrDenied)
		return
	}
	if len(r.Header.Values("If-Match")) != 0 {
		v := single(r, "If-Match")
		if len(v) > 1 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 1 || n > 9007199254740991 || strconv.FormatInt(n, 10) != v {
			fail(drafts.ErrInvalid)
			return
		}
		q.IfMatch = &n
	}
	t, _, err := mime.ParseMediaType(single(r, "Content-Type"))
	if err != nil || t != "application/json" || r.Header.Get("Content-Encoding") != "" {
		fail(drafts.ErrInvalid)
		return
	}
	q.Body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		fail(err)
		return
	}
	wrote := false
	err = s.Execute(r.Context(), q, func(v readcases.CaseDetail) error {
		b, err := json.Marshal(v)
		if err != nil || r.Context().Err() != nil {
			return drafts.ErrUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		wrote = true
		w.WriteHeader(http.StatusAccepted)
		_, err = w.Write(append(b, '\n'))
		return err
	})
	if err != nil && !wrote {
		fail(err)
	}
}

func writeError(w http.ResponseWriter, id string, err error) {
	status, code := http.StatusServiceUnavailable, "RECOVERY_UNAVAILABLE"
	var size *http.MaxBytesError
	switch {
	case errors.Is(err, drafts.ErrInvalid):
		status, code = 400, "INVALID_REQUEST"
	case errors.Is(err, drafts.ErrUnauthenticated):
		status, code = 401, "UNAUTHENTICATED"
	case errors.Is(err, drafts.ErrDenied):
		status, code = 403, "RECOVERY_ACCESS_DENIED"
	case errors.Is(err, drafts.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(err, drafts.ErrConflict):
		status, code = 409, "RECOVERY_CONFLICT"
	case errors.Is(err, drafts.ErrStale):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	case errors.As(err, &size):
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	if !drafts.UUID(id) {
		id, _ = drafts.NewID()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(drafts.Error{Code: code, RequestID: id, Message: "The recovery request could not be completed"})
}
