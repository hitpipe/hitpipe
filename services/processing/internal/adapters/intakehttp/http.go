// Package intakehttp attaches the Intake route to the existing private server.
package intakehttp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
)

const Path = "/internal/v1/intake-signals"

// HP-NFR-004: one incoming signal is bounded to256KiB.
const MaxSignalBytes int64 = 256 << 10

type Handler struct {
	service receivesignals.Service
	peer    string
}

func New(s receivesignals.Service, peer string) (http.Handler, error) {
	if !s.Ready() || peer == "" {
		return nil, receivesignals.ErrUnavailable
	}
	return Handler{s, peer}, nil
}

// Attach is called before ServeTLS. It retains existing read/Delivery handlers,
// TLS policy, timeouts and single listener. No new socket or production grant.
func Attach(server *http.Server, s receivesignals.Service, peer string) error {
	h, e := New(s, peer)
	if e != nil {
		return e
	}
	if server == nil || server.Handler == nil {
		return receivesignals.ErrUnavailable
	}
	host, port, e := net.SplitHostPort(server.Addr)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || (port != "8443" && !(ip.IsLoopback() && port == "0")) {
		return receivesignals.ErrUnavailable
	}
	t := server.TLSConfig
	if t == nil || t.ClientCAs == nil || len(t.Certificates) == 0 || t.ClientAuth != tls.RequireAndVerifyClientCert || t.MinVersion != tls.VersionTLS13 || t.MaxVersion != tls.VersionTLS13 || t.InsecureSkipVerify || t.GetConfigForClient != nil {
		return receivesignals.ErrUnavailable
	}
	previous := server.Handler
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == Path || strings.HasPrefix(r.URL.Path, Path+"/") {
			h.ServeHTTP(w, r)
			return
		}
		previous.ServeHTTP(w, r)
	})
	return nil
}
func fail(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "SIGNAL_UNAVAILABLE"
	switch {
	case errors.Is(e, admission.ErrInvalidSignal):
		status, code = 400, "INVALID_SIGNAL"
	case errors.Is(e, admission.ErrNotAuthorized):
		status, code = 403, "SIGNAL_DENIED"
	case errors.Is(e, admission.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(e, admission.ErrConflict):
		status, code = 409, "MESSAGE_CONFLICT"
	case errors.Is(e, admission.ErrNotAdmissible):
		status, code = 409, "SIGNAL_NOT_ADMISSIBLE"
	case errors.Is(e, admission.ErrStaleRevision):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	}
	var limit *http.MaxBytesError
	if errors.As(e, &limit) {
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	requestID := r.Header.Get("X-Request-ID")
	if !receivesignals.UUID(requestID) {
		var b [16]byte
		if _, e = rand.Read(b[:]); e != nil {
			w.WriteHeader(503)
			return
		}
		b[6] = b[6]&15 | 64
		b[8] = b[8]&63 | 128
		requestID = fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}{code, "Signal could not be accepted", requestID})
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bad := func(e error) { fail(w, r, e) }
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.peer) != nil {
		bad(admission.ErrNotAuthorized)
		return
	}
	if r.Method != "POST" || r.URL.Path != Path || r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		bad(admission.ErrInvalidSignal)
		return
	}
	for k := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-hitpipe-") {
			bad(admission.ErrInvalidSignal)
			return
		}
	}
	ids := r.Header.Values("X-Request-ID")
	if len(ids) != 1 || !receivesignals.UUID(ids[0]) {
		bad(admission.ErrInvalidSignal)
		return
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || len(r.Header.Values("Content-Type")) != 1 || media != "application/json" || len(r.Header.Values("Content-Encoding")) != 0 {
		bad(admission.ErrInvalidSignal)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxSignalBytes)
	raw, e := io.ReadAll(r.Body)
	if e != nil {
		bad(e)
		return
	}
	wrote := false
	e = h.service.Execute(r.Context(), raw, func(receipt admission.Receipt) error {
		wrote = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(202)
		return json.NewEncoder(w).Encode(receipt)
	})
	if e != nil && !wrote {
		bad(e)
	}
}
