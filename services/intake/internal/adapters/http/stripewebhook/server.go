// Package stripewebhook serves the Stripe-signed Intake ingress over HTTPS.
package stripewebhook

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/intake/internal/application/normalizesignal"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
)

const (
	pathPrefix     = "/webhooks/stripe/"
	signatureLimit = 2048
)

var (
	ErrNotFound = errors.New("installation not found")
	ErrDenied   = errors.New("installation denied")
	ErrStale    = errors.New("installation revision changed")
)

// Installation is the owner-checked binding for one endpoint and revision.
type Installation struct {
	EndpointID string
	Connection receivesignal.Connection
	Revision   int64
	Ready      bool
}

// Installations resolves the owner's current binding using only the route ID.
type Installations interface {
	Lookup(context.Context, string) (Installation, error)
}

// StartGate checks current owner authority, readiness, binding and revision at
// actual start. If granted, it calls receive once, synchronously, and waits for
// it to finish. Any unknown or revoked authority must deny start.
type StartGate interface {
	Start(ctx context.Context, installation Installation, receive func() error) error
}

// Server serves only the webhook route. The caller owns listener placement and
// explicitly supplies TLS certificates and the transport deadline.
type Server struct {
	http *http.Server
	tls  *tls.Config
}

// New composes existing workspace-scoped durable receivers. Missing workspace
// receivers deny admission. There are no default grants or credentials.
func New(cfg *tls.Config, timeout time.Duration, installs Installations, gate StartGate, receivers map[string]*receivesignal.Consumer) (*Server, error) {
	if cfg == nil || len(cfg.Certificates) == 0 || cfg.GetConfigForClient != nil ||
		timeout <= 0 || missing(installs) || missing(gate) || len(receivers) == 0 {
		return nil, errors.New("webhook dependencies required")
	}
	owners := make(map[string]*receivesignal.Consumer, len(receivers))
	for workspace, receiver := range receivers {
		if workspace == "" || receiver == nil {
			return nil, errors.New("invalid workspace receiver")
		}
		owners[workspace] = receiver
	}
	t := cfg.Clone()
	if t.MinVersion < tls.VersionTLS13 {
		t.MinVersion = tls.VersionTLS13
	}
	if t.MaxVersion != 0 && t.MaxVersion < t.MinVersion {
		return nil, errors.New("invalid webhook TLS versions")
	}
	t.NextProtos = []string{"http/1.1"}
	h := handler{timeout: timeout, installs: installs, gate: gate, receivers: owners}
	srv := &http.Server{
		Handler: h, TLSConfig: t,
		ReadHeaderTimeout: timeout, ReadTimeout: timeout, IdleTimeout: timeout,
		ErrorLog: log.New(io.Discard, "", 0),
	}
	srv.SetKeepAlivesEnabled(false)
	return &Server{http: srv, tls: t}, nil
}

func missing(port any) bool {
	if port == nil {
		return true
	}
	v := reflect.ValueOf(port)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func (s *Server) Serve(listener net.Listener) error {
	return s.http.Serve(tls.NewListener(listener, s.tls))
}

func (s *Server) Close(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

type handler struct {
	timeout   time.Duration
	installs  Installations
	gate      StartGate
	receivers map[string]*receivesignal.Consumer
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	controller := http.NewResponseController(w)
	// Prevent implicit body draining on early rejection. Reading always finishes
	// before any handler response is written.
	if err := controller.EnableFullDuplex(); err != nil {
		writeFailure(w, http.StatusServiceUnavailable)
		return
	}
	status := h.receive(r)
	if err := controller.SetWriteDeadline(time.Now().Add(h.timeout)); err != nil {
		return
	}
	if status != http.StatusNoContent {
		writeFailure(w, status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (h handler) receive(r *http.Request) int {
	if r.TLS == nil {
		return http.StatusUnauthorized
	}
	if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.Path, pathPrefix) {
		return http.StatusNotFound
	}
	id := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if id == "" || strings.ContainsRune(id, '/') {
		return http.StatusNotFound
	}
	if !utf8.ValidString(id) || utf8.RuneCountInString(id) > 255 ||
		strings.ContainsFunc(id, unicode.IsControl) || id == "." || id == ".." ||
		r.URL.RawQuery != "" || r.URL.ForceQuery {
		return http.StatusBadRequest
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" || len(r.Header.Values("Content-Encoding")) != 0 {
		return http.StatusBadRequest
	}
	headers := r.Header.Values("Stripe-Signature")
	if len(headers) != 1 || headers[0] == "" {
		return http.StatusUnauthorized
	}
	if r.ContentLength > normalizesignal.MaxBodyBytes || len(headers[0]) > signatureLimit {
		return http.StatusRequestEntityTooLarge
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	raw, err := io.ReadAll(io.LimitReader(r.Body, normalizesignal.MaxBodyBytes+1))
	if len(raw) > normalizesignal.MaxBodyBytes {
		return http.StatusRequestEntityTooLarge
	}
	var timeout net.Error
	if ctx.Err() != nil || (errors.As(err, &timeout) && timeout.Timeout()) {
		return http.StatusServiceUnavailable
	}
	if err != nil || len(raw) == 0 {
		return http.StatusBadRequest
	}
	installation, err := h.installs.Lookup(ctx, id)
	if err != nil {
		return failureStatus(err)
	}
	b := installation.Connection.Binding
	receiver := h.receivers[b.WorkspaceID]
	if installation.EndpointID != id || installation.Revision <= 0 || !installation.Ready ||
		installation.Connection.SecurityEpoch <= 0 || b.InstallationID == "" || b.ConnectionID == "" ||
		b.StripeAccountID == "" || b.Livemode == nil || b.APIVersion == "" ||
		(b.AccountScope != normalizesignal.DirectAccount && b.AccountScope != normalizesignal.ConnectedAccount) ||
		receiver == nil {
		return http.StatusForbidden
	}
	mode := *b.Livemode
	installation.Connection.Binding.Livemode = &mode
	var outcome error
	var called, committed bool
	err = h.gate.Start(ctx, installation, func() error {
		if called || ctx.Err() != nil {
			committed = false
			return receivesignal.ErrUnavailable
		}
		called = true
		var signal *normalizesignal.Signal
		signal, outcome = receiver.Receive(ctx, receivesignal.Delivery{
			Connection: installation.Connection, RawBody: raw, Signature: headers[0],
		})
		committed = outcome == nil && signal != nil
		return outcome
	})
	if err != nil {
		return failureStatus(err)
	}
	if outcome != nil {
		return failureStatus(outcome)
	}
	if !committed || ctx.Err() != nil {
		return http.StatusServiceUnavailable
	}
	return http.StatusNoContent
}

func failureStatus(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrDenied), errors.Is(err, receivesignal.ErrContext):
		return http.StatusForbidden
	case errors.Is(err, ErrStale):
		return http.StatusPreconditionFailed
	case errors.Is(err, receivesignal.ErrAuthentication):
		return http.StatusUnauthorized
	case errors.Is(err, receivesignal.ErrInvalidEvent):
		return http.StatusBadRequest
	case errors.Is(err, receivesignal.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, receivesignal.ErrQuota):
		return http.StatusTooManyRequests
	default:
		return http.StatusServiceUnavailable
	}
}

func writeFailure(w http.ResponseWriter, status int) {
	codes := map[int]string{
		400: "invalid_request", 401: "unauthenticated", 403: "denied",
		404: "not_found", 409: "conflict", 412: "stale_revision",
		413: "request_size", 429: "quota", 503: "unavailable",
	}
	var id [16]byte
	rand.Read(id[:])
	id[6] = id[6]&15 | 64
	id[8] = id[8]&63 | 128
	requestID := fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}{codes[status], http.StatusText(status), requestID})
}
