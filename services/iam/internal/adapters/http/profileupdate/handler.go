// Package profileupdate serves the BFF-only PATCH operation on the private IAM listener.
package profileupdate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const Path = "/api/v1/profile"

// Backend calls start once, synchronously, after establishing the durable
// command outcome and while holding the accepted response-start gate.
type Backend interface {
	Update(context.Context, runtimeaccess.Peer, string, string, app.Command,
		func(context.Context, getprofile.Profile) error) error
}

type handler struct {
	backend Backend
	bff     map[runtimeaccess.Peer]struct{}
}

// New binds the PATCH adapter to the BFF entries in the independently
// provisioned runtime inventory. The listener verifies the TLS chain.
func New(backend Backend, peers []runtimeaccess.Peer) (http.Handler, error) {
	if backend == nil || len(peers) == 0 {
		return nil, errors.New("profile update HTTP: backend and provisioned BFF peers are required")
	}
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, errors.New("profile update HTTP: backend is required")
		}
	}
	bff := make(map[runtimeaccess.Peer]struct{}, len(peers))
	for _, peer := range peers {
		if !runtimeaccess.ValidDNSName(peer.DNSName) || !runtimeaccess.ValidFingerprint(peer.CertificateFingerprint) {
			return nil, errors.New("profile update HTTP: invalid provisioned BFF peer")
		}
		peer.DNSName = strings.ToLower(peer.DNSName)
		if _, exists := bff[peer]; exists {
			return nil, errors.New("profile update HTTP: duplicate provisioned BFF peer")
		}
		bff[peer] = struct{}{}
	}
	return &handler{backend: backend, bff: bff}, nil
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID, ok := singleHeader(r, "X-Request-ID")
	validID := ok && runtimeaccess.ValidUUID(requestID)
	if !validID {
		requestID = newRequestID()
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path != Path || r.URL.EscapedPath() != Path {
		writeFailure(w, r, app.ErrNotFound)
		return
	}
	if r.Method != http.MethodPatch {
		w.Header().Set("Allow", http.MethodPatch)
		writeError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Метод не поддерживается.")
		return
	}
	peer, err := privateiam.TransportPeer(r.TLS)
	if err != nil {
		writeFailure(w, r, app.ErrUnauthenticated)
		return
	}
	if _, allowed := h.bff[peer]; !allowed || hasAuthorityHeaders(r) {
		writeFailure(w, r, app.ErrDenied)
		return
	}
	if !validID || r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeFailure(w, r, app.ErrInvalid)
		return
	}
	session, ok := secretHeader(r, "X-HitPipe-Session", 43)
	if !ok {
		writeFailure(w, r, app.ErrUnauthenticated)
		return
	}
	csrf, ok := secretHeader(r, "X-CSRF-Token", 32)
	if !ok {
		writeFailure(w, r, app.ErrInvalid)
		return
	}

	control := http.NewResponseController(w)
	cmd, err := readCommand(w, r, control, requestID)
	if err != nil {
		writeFailure(w, r, err)
		return
	}
	if r.Context().Err() != nil || !canFlush(w) || control.SetWriteDeadline(time.Time{}) != nil {
		writeFailure(w, r, app.ErrUnavailable)
		return
	}

	var response responseStart
	err = h.backend.Update(r.Context(), peer, session, csrf, cmd, func(work context.Context, profile getprofile.Profile) error {
		response.mu.Lock()
		defer response.mu.Unlock()
		if response.finished || response.called {
			response.err = app.ErrUnavailable
			return response.err
		}
		response.called = true
		response.err = response.write(w, r, control, work, peer, profile)
		return response.err
	})
	response.finish(w, r, err)
}

type responseStart struct {
	mu                        sync.Mutex
	called, started, finished bool
	err                       error
}

func (r *responseStart) write(w http.ResponseWriter, request *http.Request, control *http.ResponseController,
	work context.Context, expectedPeer runtimeaccess.Peer, profile getprofile.Profile) error {
	if work == nil || work.Err() != nil || request.Context().Err() != nil || !validProfile(profile) {
		return app.ErrUnavailable
	}
	body, err := json.Marshal(profile)
	if err != nil {
		return app.ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(work, deadline(request.Context()))
	stopRequest := context.AfterFunc(request.Context(), cancel)
	defer func() { stopRequest(); cancel() }()
	limit, _ := ctx.Deadline()
	if control.SetWriteDeadline(limit) != nil {
		return app.ErrUnavailable
	}
	interrupted := make(chan struct{})
	stopWrite := context.AfterFunc(ctx, func() {
		defer close(interrupted)
		_ = control.SetWriteDeadline(time.Now())
	})
	defer func() {
		if !stopWrite() {
			<-interrupted
		}
		_ = control.SetWriteDeadline(time.Time{})
	}()
	peer, err := privateiam.TransportPeer(request.TLS)
	if err != nil || peer != expectedPeer {
		return app.ErrDenied
	}
	if ctx.Err() != nil || request.Context().Err() != nil {
		return app.ErrUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	r.started = true
	w.WriteHeader(http.StatusOK)
	if n, err := w.Write(body); err != nil || n != len(body) {
		return app.ErrUnavailable
	}
	// A header still buffered by net/http is not an actual network start.
	if ctx.Err() != nil || control.Flush() != nil {
		return app.ErrUnavailable
	}
	return nil
}

func (r *responseStart) finish(w http.ResponseWriter, request *http.Request, err error) {
	r.mu.Lock()
	r.finished = true
	started := r.started
	if err == nil {
		err = r.err
	}
	r.mu.Unlock()
	if !started {
		writeFailure(w, request, err)
	} else if err != nil {
		panic(http.ErrAbortHandler)
	}
}

func deadline(ctx context.Context) time.Time {
	limit := time.Now().Add(privateiam.MaxWriteDuration)
	if bound, ok := ctx.Deadline(); ok && bound.Before(limit) {
		return bound
	}
	return limit
}

func canFlush(w http.ResponseWriter) bool {
	for range 16 {
		if _, ok := w.(interface{ FlushError() error }); ok {
			return true
		}
		if _, ok := w.(http.Flusher); ok {
			return true
		}
		wrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = wrapper.Unwrap()
	}
	return false
}

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeFailure(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalid):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Некорректный запрос.")
	case errors.Is(err, app.ErrUnauthenticated):
		writeError(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Требуется действительная сессия.")
	case errors.Is(err, app.ErrDenied):
		writeError(w, r, http.StatusForbidden, "DENIED", "Доступ не разрешён.")
	case errors.Is(err, app.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Запрошенный ресурс недоступен.")
	case errors.Is(err, app.ErrConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "Команда конфликтует с текущим состоянием.")
	case errors.Is(err, app.ErrStale):
		writeError(w, r, http.StatusPreconditionFailed, "STALE_REVISION", "Требуется актуальная ревизия.")
	case errors.Is(err, errTooLarge):
		writeError(w, r, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "Размер запроса превышает предел.")
	default:
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Исход операции не подтверждён.")
	}
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	if r.Context().Err() != nil {
		// Closing the request must not allow net/http to synthesize a 200.
		panic(http.ErrAbortHandler)
	}
	body, err := json.Marshal(errorResponse{code, message, w.Header().Get("X-Request-ID")})
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = http.NewResponseController(w).SetWriteDeadline(deadline(r.Context()))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func newRequestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(http.ErrAbortHandler)
	}
	id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
