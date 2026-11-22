// Package revocation exposes the accepted IAM-only barrier operation on its own
// private HTTPS listener. The process must inject its existing actual-start gate.
package revocation

import (
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

const (
	Path         = "/internal/v1/runtime/barrier"
	maxBodyBytes = 16 * 1024
	// An already-started critical section may take up to two seconds.
	// Leave transport headroom; the gate still enforces DeadlineAt.
	maxExchangeDuration = 3 * time.Second
)

type Handler struct {
	gate           *startgate.Gate
	iamClientCAs   *x509.CertPool
	iamFingerprint string
}

// NewHandler only accepts the provisioned IAM client and never creates a new
// gate. The private HTTPS server must also set read, write and header timeouts
// and use ServerTLS.
func NewHandler(gate *startgate.Gate, iamClientCAs *x509.CertPool, iamFingerprint string) (*Handler, error) {
	if gate == nil || !validRoots(iamClientCAs) || !runtimeaccess.ValidFingerprint(iamFingerprint) {
		return nil, errConfiguration
	}
	return &Handler{gate: gate, iamClientCAs: iamClientCAs.Clone(), iamFingerprint: iamFingerprint}, nil
}

func requestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(http.ErrAbortHandler)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func onlyHeader(h http.Header, name string) string {
	values := h.Values(name)
	if len(values) != 1 {
		return ""
	}
	return values[0]
}

func jsonHeader(h http.Header) bool {
	media, _, err := mime.ParseMediaType(onlyHeader(h, "Content-Type"))
	return err == nil && strings.EqualFold(media, "application/json") && len(h.Values("Content-Encoding")) == 0
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id := onlyHeader(r.Header, "X-Request-ID")
	if !runtimeaccess.ValidUUID(id) {
		writeError(w, requestID(), http.StatusBadRequest, "INVALID_REQUEST", "A valid request ID is required.")
		return
	}
	if r.TLS == nil || !r.TLS.HandshakeComplete || verifyIAM(r.TLS, h.iamClientCAs, h.iamFingerprint) != nil {
		writeError(w, id, http.StatusUnauthorized, "UNAUTHENTICATED", "Verified IAM client transport is required.")
		return
	}
	if r.URL.Path != Path {
		writeError(w, id, http.StatusNotFound, "NOT_FOUND", "The requested operation is unavailable.")
		return
	}
	if r.Method != http.MethodPost || r.URL.IsAbs() || r.URL.RawQuery != "" || r.URL.ForceQuery ||
		r.URL.EscapedPath() != Path || !jsonHeader(r.Header) || r.Body == nil {
		writeError(w, id, http.StatusBadRequest, "INVALID_REQUEST", "The request is unsupported.")
		return
	}
	if r.ContentLength > maxBodyBytes {
		writeError(w, id, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The request exceeds the size limit.")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if len(body) > maxBodyBytes {
		writeError(w, id, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The request exceeds the size limit.")
		return
	}
	command, parseErr := parseCommand(body)
	if err != nil || parseErr != nil {
		writeError(w, id, http.StatusBadRequest, "INVALID_REQUEST", "The barrier command is invalid.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), maxExchangeDuration)
	defer cancel()
	ack, err := h.gate.ApplyBarrier(ctx, command)
	switch {
	case errors.Is(err, startgate.ErrInvalid):
		writeError(w, id, http.StatusBadRequest, "INVALID_REQUEST", "The barrier command is invalid.")
	case errors.Is(err, startgate.ErrIdentity), errors.Is(err, startgate.ErrStaleSnapshot):
		writeError(w, id, http.StatusPreconditionFailed, "STALE_REVISION", "The current runtime identity is required.")
	case errors.Is(err, startgate.ErrConflict):
		writeError(w, id, http.StatusConflict, "CONFLICT", "The barrier conflicts with current state.")
	case err != nil, ctx.Err() != nil:
		writeError(w, id, http.StatusServiceUnavailable, "UNAVAILABLE", "The barrier acknowledgement could not be established.")
	case !ackMatches(command, ack):
		writeError(w, id, http.StatusPreconditionFailed, "STALE_REVISION", "The exact barrier acknowledgement is unavailable.")
	default:
		writeJSON(w, id, http.StatusOK, ackValue(ack))
	}
}

func writeError(w http.ResponseWriter, id string, status int, code, message string) {
	writeJSON(w, id, status, struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}{Code: code, Message: message, RequestID: id})
}

func writeJSON(w http.ResponseWriter, id string, status int, result any) {
	body, err := json.Marshal(result)
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", id)
	w.WriteHeader(status)
	if n, err := w.Write(body); err != nil || n != len(body) {
		panic(http.ErrAbortHandler)
	}
}
