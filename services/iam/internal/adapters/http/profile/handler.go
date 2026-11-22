// Package profile exposes only the private IAM profile read for a verified BFF.
// It configures no production listener or runtime admission implementation.
package profile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
)

var ErrConfiguration = errors.New("profile_http_configuration_invalid")

type Service interface {
	Serve(context.Context, string, authorizeaction.Caller, func(getprofile.Profile) error) error
}

type guardedService interface {
	ServeWithContext(context.Context, string, authorizeaction.Caller, func(context.Context, getprofile.Profile) error) error
}

// Config explicitly binds a verified client certificate to one BFF runtime.
// BFFDNSName is an exact configured DNS SAN, not a canonical deployment hostname.
// Certificate rotation or runtime replacement requires a fresh explicit binding.
type Config struct {
	BFFCaller  authorizeaction.Caller
	BFFDNSName string
}

type Handler struct {
	service Service
	config  Config
}

func absent(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	var id [16]byte
	_, err := hex.Decode(id[:], []byte(strings.ReplaceAll(value, "-", "")))
	return err == nil && id != [16]byte{}
}

func validFingerprint(value string) bool {
	if len(value) != 71 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, c := range value[7:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func New(service Service, config Config) (*Handler, error) {
	c := config.BFFCaller
	if absent(service) || c.Service != "BFF" || !validUUID(c.RuntimeID) || !validUUID(c.BootID) || c.Generation < 1 || c.Generation > 9007199254740991 || !validFingerprint(c.CertificateFingerprint) || config.BFFDNSName == "" || strings.TrimSpace(config.BFFDNSName) != config.BFFDNSName || strings.ContainsAny(config.BFFDNSName, "*/\\\x00\r\n\t :") {
		return nil, ErrConfiguration
	}
	return &Handler{service: service, config: config}, nil
}

// TLSConfig requires TLS 1.3 and verified mutual TLS. The Handler additionally
// checks the exact BFF SAN and configured leaf fingerprint; any trusted client
// certificate alone is insufficient. Callers own listener binding and secrets.
func TLSConfig(certificate tls.Certificate, clientCAs *x509.CertPool) (*tls.Config, error) {
	if len(certificate.Certificate) == 0 || absent(certificate.PrivateKey) || clientCAs == nil {
		return nil, ErrConfiguration
	}
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil || leaf.IsCA {
		return nil, ErrConfiguration
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Certificates: []tls.Certificate{certificate},
		ClientAuth:   tls.RequireAndVerifyClientCert, ClientCAs: clientCAs.Clone(),
	}, nil
}

func (h *Handler) verifiedCaller(state *tls.ConnectionState) (authorizeaction.Caller, bool) {
	if state == nil || !state.HandshakeComplete || state.Version != tls.VersionTLS13 || len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 || len(state.PeerCertificates) == 0 {
		return authorizeaction.Caller{}, false
	}
	leaf := state.PeerCertificates[0]
	if leaf == nil || state.VerifiedChains[0][0] == nil || !leaf.Equal(state.VerifiedChains[0][0]) || leaf.IsCA {
		return authorizeaction.Caller{}, false
	}
	// Standard TLS verification established the chain and client-auth EKU. Check
	// expiry again at this read so a long-lived connection is not an expiry bypass.
	now := time.Now()
	for _, certificate := range state.VerifiedChains[0] {
		if certificate == nil || now.Before(certificate.NotBefore) || !now.Before(certificate.NotAfter) {
			return authorizeaction.Caller{}, false
		}
	}
	fingerprint := sha256.Sum256(leaf.Raw)
	if "sha256:"+hex.EncodeToString(fingerprint[:]) != h.config.BFFCaller.CertificateFingerprint {
		return authorizeaction.Caller{}, false
	}
	for _, dnsName := range leaf.DNSNames {
		if dnsName == h.config.BFFDNSName {
			return h.config.BFFCaller, true
		}
	}
	return authorizeaction.Caller{}, false
}

func validProfile(p getprofile.Profile) bool {
	return validUUID(p.UserID) && p.Revision > 0 && p.Revision <= 9007199254740991 && utf8.ValidString(p.DisplayName) && utf8.RuneCountInString(p.DisplayName) >= 1 && utf8.RuneCountInString(p.DisplayName) <= 120 && utf8.ValidString(p.Timezone) && utf8.RuneCountInString(p.Timezone) >= 1 && utf8.RuneCountInString(p.Timezone) <= 80
}

func requestID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	var dst [36]byte
	hex.Encode(dst[0:8], b[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], b[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], b[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], b[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], b[10:16])
	return string(dst[:]), nil
}

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	id, err := requestID()
	if err != nil {
		// No synthetic or caller-controlled ID replaces failed entropy. Do not
		// emit a malformed structured error or reveal its underlying failure.
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	body, err := json.Marshal(errorResponse{Code: code, Message: message, RequestID: id})
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func serviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, getprofile.ErrInvalidRequest):
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Некорректный запрос.")
	case errors.Is(err, getprofile.ErrUnauthenticated):
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Требуется действительная сессия.")
	case errors.Is(err, getprofile.ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Доступ не разрешён.")
	case errors.Is(err, getprofile.ErrCancelled):
		writeError(w, http.StatusServiceUnavailable, "REQUEST_CANCELLED", "Запрос отменён.")
	default:
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Профиль сейчас недоступен.")
	}
}

func canFlush(w http.ResponseWriter) bool {
	for range 16 {
		if _, ok := w.(interface{ FlushError() error }); ok {
			return true
		}
		if _, ok := w.(http.Flusher); ok {
			return true
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = unwrapper.Unwrap()
	}
	return false
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if h == nil || absent(h.service) {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Профиль сейчас недоступен.")
		return
	}
	if r.URL.Path != "/api/v1/profile" || r.URL.RawQuery != "" {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Запрошенный ресурс недоступен.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Метод не поддерживается.")
		return
	}
	caller, verified := h.verifiedCaller(r.TLS)
	if !verified {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "Доступ не разрешён.")
		return
	}
	credentials := r.Header.Values("X-HitPipe-Session")
	if len(credentials) != 1 || len(credentials[0]) < 43 || len(credentials[0]) > 128 || strings.ContainsAny(credentials[0], "\x00\r\n, \t") {
		writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "Требуется действительная сессия.")
		return
	}
	bounded, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	deadline, _ := bounded.Deadline()
	controller := http.NewResponseController(w)
	// Fail before entering the protected gate if this transport cannot bound
	// writes or flush their actual start inside the gate. Recording a header in
	// net/http's small response buffer alone is not a network response start.
	if !canFlush(w) || controller.SetWriteDeadline(deadline) != nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Профиль сейчас недоступен.")
		return
	}
	defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
	var emitted, started, flushed atomic.Bool
	publish := func(work context.Context, profile getprofile.Profile) error {
		if work == nil || !emitted.CompareAndSwap(false, true) || work.Err() != nil || bounded.Err() != nil || !validProfile(profile) {
			return getprofile.ErrUnavailable
		}
		// Buffer and validate before the first response byte. This callback runs
		// inside the application actual-response-start gate, after confirmation.
		body, err := json.Marshal(profile)
		if err != nil {
			return getprofile.ErrUnavailable
		}
		if work.Err() != nil || bounded.Err() != nil {
			return getprofile.ErrCancelled
		}
		currentCaller, verified := h.verifiedCaller(r.TLS)
		if !verified || currentCaller != caller {
			return getprofile.ErrForbidden
		}
		if limit, ok := work.Deadline(); ok && limit.Before(deadline) && controller.SetWriteDeadline(limit) != nil {
			return getprofile.ErrUnavailable
		}
		canceled := make(chan struct{})
		stop := context.AfterFunc(work, func() {
			defer close(canceled)
			_ = controller.SetWriteDeadline(time.Now())
		})
		defer func() {
			if !stop() {
				<-canceled
			}
		}()
		if work.Err() != nil || bounded.Err() != nil {
			return getprofile.ErrCancelled
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		started.Store(true)
		w.WriteHeader(http.StatusOK)
		if n, err := w.Write(body); err != nil || n != len(body) {
			return getprofile.ErrUnavailable
		}
		if work.Err() != nil || bounded.Err() != nil || controller.Flush() != nil {
			return getprofile.ErrUnavailable
		}
		flushed.Store(true)
		return nil
	}
	var err error
	service, guarded := h.service.(guardedService)
	if guarded {
		err = service.ServeWithContext(bounded, credentials[0], caller, publish)
	} else {
		err = h.service.Serve(bounded, credentials[0], caller, func(profile getprofile.Profile) error {
			return publish(bounded, profile)
		})
	}
	if started.Load() {
		// A gate/transport error after response start cannot undo that start. Never
		// append error JSON, retry the read or emit a second HTTP response.
		if guarded && !flushed.Load() {
			panic(http.ErrAbortHandler)
		}
		return
	}
	if err == nil {
		err = getprofile.ErrUnavailable
	}
	serviceError(w, err)
}
