// Package observationhttp is only the private Processing receiver for Delivery.
package observationhttp

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

type Handler struct {
	Service observations.Service
	PeerDNS string
	MaxBody int64
}

func New(s observations.Service, peer string, maxBody int64) (http.Handler, error) {
	if !s.Ready() || peer == "" || maxBody < 1 || maxBody == math.MaxInt64 {
		return nil, observations.ErrUnavailable
	}
	return Handler{s, peer, maxBody}, nil
}

// No socket is bound before explicit ServeTLS by the caller. There is no
// production composition with permissive test grants or test certificates.
func NewServer(address string, t *tls.Config, s observations.Service, peer string, maxBody int64) (*http.Server, error) {
	host, port, e := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || (!ip.IsPrivate() && !ip.IsLoopback()) || (port != "8443" && !(ip.IsLoopback() && port == "0")) {
		return nil, observations.ErrUnavailable
	}
	h, e := New(s, peer, maxBody)
	if e != nil {
		return nil, e
	}
	if t == nil || t.ClientCAs == nil || len(t.Certificates) == 0 || t.InsecureSkipVerify || t.GetConfigForClient != nil {
		return nil, observations.ErrUnavailable
	}
	cfg := t.Clone()
	cfg.MinVersion = tls.VersionTLS13
	cfg.MaxVersion = tls.VersionTLS13
	cfg.ClientAuth = tls.RequireAndVerifyClientCert
	cfg.NextProtos = []string{"http/1.1"}
	return &http.Server{Addr: address, Handler: h, TLSConfig: cfg, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 2 * time.Second, IdleTimeout: 15 * time.Second, MaxHeaderBytes: 16 << 10}, nil
}
func fail(w http.ResponseWriter, r *http.Request, e error) {
	status, code := 503, "OBSERVATION_UNAVAILABLE"
	switch {
	case errors.Is(e, observations.ErrInvalid):
		status, code = 400, "INVALID_OBSERVATION"
	case errors.Is(e, observations.ErrDenied):
		status, code = 403, "OBSERVATION_DENIED"
	case errors.Is(e, observations.ErrNotFound):
		status, code = 404, "NOT_FOUND"
	case errors.Is(e, observations.ErrConflict):
		status, code = 409, "OBSERVATION_CONFLICT"
	case errors.Is(e, observations.ErrStale):
		status, code = 412, "REVISION_PRECONDITION_FAILED"
	}
	var limit *http.MaxBytesError
	if errors.As(e, &limit) {
		status, code = 413, "REQUEST_TOO_LARGE"
	}
	id := r.Header.Get("X-Request-ID")
	if !observations.UUID(id) {
		var b [16]byte
		if _, e = rand.Read(b[:]); e != nil {
			w.WriteHeader(503)
			return
		}
		b[6] = b[6]&15 | 64
		b[8] = b[8]&63 | 128
		id = fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(observations.Error{Code: code, Message: "Observation could not be applied", RequestID: id})
}
func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bad := func(e error) { fail(w, r, e) }
	if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 || r.TLS.PeerCertificates[0].VerifyHostname(h.PeerDNS) != nil {
		bad(observations.ErrDenied)
		return
	}
	if r.Method != "POST" || r.URL.Path != "/internal/v1/delivery-observations" || r.URL.RawQuery != "" {
		bad(observations.ErrInvalid)
		return
	}
	for k := range r.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-hitpipe-") {
			bad(observations.ErrInvalid)
			return
		}
	}
	ids := r.Header.Values("X-Request-ID")
	if len(ids) != 1 || !observations.UUID(ids[0]) {
		bad(observations.ErrInvalid)
		return
	}
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
		bad(observations.ErrInvalid)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxBody)
	body, e := io.ReadAll(r.Body)
	if e != nil {
		bad(e)
		return
	}
	started := false
	e = h.Service.Execute(r.Context(), body, func(receipt observations.Receipt) error {
		started = true
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(202)
		return json.NewEncoder(w).Encode(receipt)
	})
	if e != nil && !started {
		bad(e)
	}
}
