package intake

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const route = "/internal/v1/delivery-intents"
const maxWireBytes = 512 * 1024

// NewHTTP constructs the internal TLS 1.3 listener. The caller owns binding
// its private address and supplying independently provisioned trust material.
func NewHTTP(s *Service, serverCert tls.Certificate, clientCAs *x509.CertPool, processingSAN string) (*http.Server, error) {
	if s == nil || len(serverCert.Certificate) == 0 || serverCert.PrivateKey == nil ||
		clientCAs == nil || processingSAN == "" {
		return nil, ErrUnavailable
	}
	h := handler{service: s, processingSAN: processingSAN}
	return &http.Server{
		Addr: ":8443", Handler: h,
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 4 * time.Second,
		WriteTimeout: 8 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8 * 1024,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    clientCAs.Clone(),
			Certificates: []tls.Certificate{serverCert},
		},
	}, nil
}

type handler struct {
	service       *Service
	query         QueryReader
	processingSAN string
}

func NewHTTPWithQuery(s *Service, q QueryReader, cert tls.Certificate, roots *x509.CertPool, san string) (*http.Server, error) {
	if q == nil {
		return nil, ErrUnavailable
	}
	server, err := NewHTTP(s, cert, roots, san)
	if err != nil {
		return nil, err
	}
	server.Handler = handler{service: s, query: q, processingSAN: san}
	return server, nil
}

type errorBody struct {
	Code      string    `json:"code"`
	Message   string    `json:"message"`
	RequestID uuid.UUID `json:"request_id"`
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := uuid.New()
	values := r.Header.Values("X-Request-ID")
	validID := false
	if len(values) == 1 && len(values[0]) == 36 {
		if id, err := uuid.Parse(values[0]); err == nil {
			requestID, validID = id, true
		}
	}
	isQuery := r.URL.Path == queryRoute && h.query != nil
	if (r.URL.Path != route && !isQuery) || r.Method != http.MethodPost {
		writeFailure(w, http.StatusNotFound, "NOT_FOUND", requestID)
		return
	}
	if r.TLS == nil || r.TLS.Version < tls.VersionTLS13 || len(r.TLS.VerifiedChains) == 0 {
		writeFailure(w, http.StatusUnauthorized, "CERTIFICATE_REQUIRED", requestID)
		return
	}
	if cert := r.TLS.VerifiedChains[0][0]; cert.VerifyHostname(h.processingSAN) != nil ||
		!hasExactDNSName(cert, h.processingSAN) {
		writeFailure(w, http.StatusUnauthorized, "CALLER_DENIED", requestID)
		return
	}
	if !validID {
		writeFailure(w, http.StatusBadRequest, "INVALID_REQUEST_ID", requestID)
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		writeFailure(w, http.StatusBadRequest, "INVALID_CONTENT_TYPE", requestID)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWireBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeFailure(w, http.StatusRequestEntityTooLarge, "SIZE_LIMIT", requestID)
		} else {
			writeFailure(w, http.StatusBadRequest, "INVALID_BODY", requestID)
		}
		return
	}
	if isQuery {
		q, err := DecodeQuery(raw)
		if err != nil {
			writeFailure(w, http.StatusBadRequest, "INVALID_QUERY", requestID)
			return
		}
		o, err := h.query.QueryKnownTask(r.Context(), q)
		if err == nil {
			writeJSON(w, http.StatusOK, o)
		} else {
			writeServiceFailure(w, err, requestID)
		}
		return
	}
	intent, err := DecodeIntent(raw)
	if err != nil {
		var invalidInput invalid
		if errors.As(err, &invalidInput) && invalidInput.size {
			writeFailure(w, http.StatusRequestEntityTooLarge, "SIZE_LIMIT", requestID)
		} else {
			writeFailure(w, http.StatusBadRequest, "INVALID_INTENT", requestID)
		}
		return
	}
	receipt, err := h.service.Accept(r.Context(), intent)
	if err == nil {
		writeJSON(w, http.StatusAccepted, receipt)
		return
	}
	writeServiceFailure(w, err, requestID)
}

func writeServiceFailure(w http.ResponseWriter, err error, requestID uuid.UUID) {
	switch {
	case errors.Is(err, ErrDenied):
		writeFailure(w, http.StatusForbidden, "SCOPE_DENIED", requestID)
	case errors.Is(err, ErrNotFound):
		writeFailure(w, http.StatusNotFound, "NOT_FOUND", requestID)
	case errors.Is(err, ErrConflict):
		writeFailure(w, http.StatusConflict, "IDENTITY_CONFLICT", requestID)
	case errors.Is(err, ErrStale):
		writeFailure(w, http.StatusPreconditionFailed, "STALE_INTENT", requestID)
	case errors.Is(err, ErrQuota):
		writeFailure(w, http.StatusTooManyRequests, "QUOTA_REFUSED", requestID)
	default:
		writeFailure(w, http.StatusServiceUnavailable, "ADMISSION_UNAVAILABLE", requestID)
	}
}

func hasExactDNSName(cert *x509.Certificate, name string) bool {
	for _, actual := range cert.DNSNames {
		if actual == name {
			return true
		}
	}
	return false
}

func writeFailure(w http.ResponseWriter, status int, code string, id uuid.UUID) {
	writeJSON(w, status, errorBody{Code: code, Message: http.StatusText(status), RequestID: id})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
