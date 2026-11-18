// Package sourcefacts builds only the private Intake readSourceFacts listener.
// There is no public webhook, health/control route or caller-header authority.
package sourcefacts

import (
	"context"
	"crypto/tls"
	"errors"
	app "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	sf "github.com/hitpipe/hitpipe/services/intake/internal/domain/sourcefacts"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"time"
)

type Server struct {
	http *http.Server
	tls  *tls.Config
}

// address is an explicitly provisioned private listener (normally :8443).
// bodyLimit is a local configured transport resource bound, not an OAS threshold.
func New(address, processingDNS string, cfg *tls.Config, service *app.Service, bodyLimit int64) (*Server, error) {
	if address == "" || processingDNS == "" || cfg == nil || service == nil || bodyLimit <= 0 || bodyLimit >= 1<<31 || cfg.ClientCAs == nil || len(cfg.Certificates) != 1 || cfg.InsecureSkipVerify || cfg.MinVersion < tls.VersionTLS13 || cfg.GetConfigForClient != nil || cfg.GetCertificate != nil {
		return nil, app.Fail("dependency")
	}
	t := cfg.Clone()
	t.ClientCAs = cfg.ClientCAs.Clone()
	t.ClientAuth = tls.RequireAndVerifyClientCert
	t.NextProtos = []string{"http/1.1"}
	prior := t.VerifyConnection
	t.VerifyConnection = func(c tls.ConnectionState) error {
		if len(c.VerifiedChains) == 0 || len(c.PeerCertificates) == 0 {
			return app.Fail("caller")
		}
		leaf := c.PeerCertificates[0]
		if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != processingDNS || len(leaf.URIs) != 0 || len(leaf.IPAddresses) != 0 {
			return app.Fail("caller")
		}
		if prior != nil {
			return prior(c)
		}
		return nil
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !sf.UUID(id) {
			id, _ = sf.NewID()
			writeError(w, 400, "request", id)
			return
		}
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 {
			writeError(w, 401, "caller", id)
			return
		}
		if r.Method != "POST" || r.URL.Path != "/internal/v1/source-facts/read" || r.URL.RawQuery != "" {
			writeError(w, 404, "not_found", id)
			return
		}
		media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			writeError(w, 400, "request", id)
			return
		}
		if r.ContentLength > bodyLimit {
			writeError(w, 413, "request_size", id)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, bodyLimit+1))
		if e != nil {
			writeError(w, 400, "request", id)
			return
		}
		if int64(len(raw)) > bodyLimit {
			writeError(w, 413, "request_size", id)
			return
		}
		q, e := sf.DecodeRequest(raw)
		if e != nil {
			writeError(w, 400, "request", id)
			return
		}
		ctx, cancel := context.WithDeadline(r.Context(), q.DeadlineAt)
		defer cancel()
		result, e := service.Read(ctx, q)
		if e != nil {
			writeError(w, status(e), app.Code(e), id)
			return
		}
		started := false
		e = service.Respond(ctx, result, func() error {
			deadline := result.Facts.ValidUntil
			if soon := time.Now().Add(2 * time.Second); soon.Before(deadline) {
				deadline = soon
			}
			if e := http.NewResponseController(w).SetWriteDeadline(deadline); e != nil {
				return e
			}
			started = true
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Request-ID", id)
			w.WriteHeader(200)
			_, e := w.Write(result.Body)
			if e != nil {
				return e
			}
			return http.NewResponseController(w).Flush()
		})
		if e != nil && !started {
			writeError(w, status(e), app.Code(e), id)
		}
	})
	srv := &http.Server{Addr: address, Handler: h, TLSConfig: t, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, IdleTimeout: 2 * time.Second, MaxHeaderBytes: 16 * 1024, ErrorLog: log.New(io.Discard, "", 0)}
	srv.SetKeepAlivesEnabled(false)
	return &Server{srv, t}, nil
}
func (s *Server) Serve(listener net.Listener) error {
	return s.http.Serve(tls.NewListener(listener, s.tls))
}
func status(e error) int {
	switch app.Code(e) {
	case "request":
		return 400
	case "context", "caller":
		return 403
	case "rate_limited":
		return 429
	default:
		return 503
	}
}
func writeError(w http.ResponseWriter, status int, code, id string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status) // All strings below are bounded locally, never provider/dependency text.
	if !sf.UUID(id) {
		id, _ = sf.NewID()
	}
	switch code {
	case "request", "request_size", "context", "caller", "rate_limited", "not_found", "expired", "content", "database", "credential", "cancelled", "provider_message", "provider_transport", "provider_status", "provider_context":
	default:
		code = "unavailable"
	}
	_, _ = io.WriteString(w, `{"code":"`+code+`","message":"Source facts unavailable","request_id":"`+id+`"}`)
}
func (s *Server) Close(ctx context.Context) error {
	e := s.http.Shutdown(ctx)
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
