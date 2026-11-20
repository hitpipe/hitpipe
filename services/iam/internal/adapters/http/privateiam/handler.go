package privateiam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const (
	MaxRequestBytes  = 16 * 1024
	MaxWriteDuration = time.Second
	AuthorizePath    = "/internal/v1/authorize"
	RegisterPath     = "/internal/v1/runtime/register"
)

// Backend owns independent inventory, current SQL facts and the one actual gate.
// Authorize must call respond exactly once, synchronously within its accepted
// response-start gate section. It must not enqueue a future response callback.
// Register returns only a committed registration and a validated current snapshot.
type Backend interface {
	Register(context.Context, runtimeaccess.Peer, runtimeaccess.Registration) (runtimeaccess.Snapshot, error)
	Authorize(context.Context, runtimeaccess.Peer, string, authorizeaction.Request, func(context.Context, authorizeaction.Authorization) error) error
}

type handler struct{ backend Backend }

func New(backend Backend) (http.Handler, error) {
	if backend == nil {
		return nil, errors.New("privateiam: backend is required")
	}
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, errors.New("privateiam: backend is required")
		}
	}
	return handler{backend}, nil
}

func (h handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestID, validID := headerValue(request, "X-Request-ID")
	if !validID || !runtimeaccess.ValidUUID(requestID) {
		requestID = newRequestID()
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "A valid request ID is required.")
		return
	}
	peer, err := transportPeer(request.TLS)
	if err != nil {
		writeError(writer, request, requestID, http.StatusUnauthorized, "UNAUTHENTICATED", "Verified client transport is required.")
		return
	}
	if request.URL.Path != AuthorizePath && request.URL.Path != RegisterPath {
		writeError(writer, request, requestID, http.StatusNotFound, "NOT_FOUND", "The requested operation is unavailable.")
		return
	}
	if request.Method != http.MethodPost || request.URL.RawQuery != "" || request.URL.EscapedPath() != request.URL.Path {
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "The request is unsupported.")
		return
	}
	contentType, validType := headerValue(request, "Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if !validType || err != nil || !strings.EqualFold(mediaType, "application/json") {
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "JSON content is required.")
		return
	}
	if request.Context().Err() != nil {
		writeBackendError(writer, request, requestID, context.Canceled)
		return
	}
	if request.ContentLength > MaxRequestBytes {
		writeError(writer, request, requestID, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The request exceeds the size limit.")
		return
	}
	if request.Body == nil {
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "The request body is invalid.")
		return
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, MaxRequestBytes+1))
	if len(body) > MaxRequestBytes {
		writeError(writer, request, requestID, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The request exceeds the size limit.")
		return
	}
	if err != nil || !utf8.Valid(body) {
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "The request body is invalid.")
		return
	}
	if request.URL.Path == RegisterPath {
		registration, err := parseRegistration(body)
		if err != nil {
			writeBackendError(writer, request, requestID, runtimeaccess.ErrInvalid)
			return
		}
		snapshot, err := h.backend.Register(request.Context(), peer, registration)
		if err != nil {
			writeBackendError(writer, request, requestID, err)
			return
		}
		if snapshot.Denials == nil {
			snapshot.Denials = []runtimeaccess.Denial{}
		}
		if !validSnapshot(snapshot) {
			writeBackendError(writer, request, requestID, runtimeaccess.ErrUnavailable)
			return
		}
		if err := writeSuccess(writer, request, requestID, snapshot); err != nil {
			panic(http.ErrAbortHandler)
		}
		return
	}
	secret, validSession := headerValue(request, "X-HitPipe-Session")
	if !validSession || len(secret) < 43 || len(secret) > 128 {
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "A session reference is required.")
		return
	}
	var input authorizeaction.Request
	// Reuse HIT-84's strict required/null, duplicate-key and unknown-field validator.
	if input.UnmarshalJSON(body) != nil {
		writeBackendError(writer, request, requestID, runtimeaccess.ErrInvalid)
		return
	}
	var lock sync.Mutex
	finished, started, calls := false, false, 0
	var responseErr error
	respond := func(work context.Context, authorization authorizeaction.Authorization) error {
		lock.Lock()
		defer lock.Unlock()
		if finished || calls != 0 {
			responseErr = runtimeaccess.ErrUnavailable
			return responseErr
		}
		calls++
		if work == nil || request.Context().Err() != nil || !validAuthorization(authorization) {
			responseErr = runtimeaccess.ErrUnavailable
			return responseErr
		}
		responseContext, cancel := context.WithCancel(work)
		stop := context.AfterFunc(request.Context(), cancel)
		defer func() { stop(); cancel() }()
		responseErr = writeJSON(writer, responseContext, requestID, http.StatusOK, authorization, &started)
		return responseErr
	}
	err = h.backend.Authorize(request.Context(), peer, secret, input, respond)
	lock.Lock()
	finished = true
	if err == nil && (calls != 1 || responseErr != nil) {
		err = runtimeaccess.ErrUnavailable
	}
	alreadyStarted := started
	lock.Unlock()
	if err != nil {
		if alreadyStarted {
			panic(http.ErrAbortHandler)
		}
		writeBackendError(writer, request, requestID, err)
	}
}

func headerValue(request *http.Request, name string) (string, bool) {
	values := request.Header.Values(name)
	return request.Header.Get(name), len(values) == 1
}

func newRequestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(http.ErrAbortHandler)
	}
	id[6] = id[6]&0x0f | 0x40
	id[8] = id[8]&0x3f | 0x80
	encoded := hex.EncodeToString(id[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}

func writeBackendError(writer http.ResponseWriter, request *http.Request, requestID string, err error) {
	switch {
	case errors.Is(err, runtimeaccess.ErrInvalid), errors.Is(err, authorizeaction.ErrInvalidRequest):
		writeError(writer, request, requestID, http.StatusBadRequest, "INVALID_REQUEST", "The request body is invalid.")
	case errors.Is(err, runtimeaccess.ErrDenied), errors.Is(err, authorizeaction.ErrDenied):
		writeError(writer, request, requestID, http.StatusForbidden, "DENIED", "The current authority does not permit this operation.")
	case errors.Is(err, runtimeaccess.ErrConflict):
		writeError(writer, request, requestID, http.StatusConflict, "CONFLICT", "The registration conflicts with current state.")
	case errors.Is(err, runtimeaccess.ErrStale):
		writeError(writer, request, requestID, http.StatusPreconditionFailed, "STALE_REVISION", "The current revision is required.")
	default:
		writeError(writer, request, requestID, http.StatusServiceUnavailable, "UNAVAILABLE", "The operation could not be established.")
	}
}

func writeError(writer http.ResponseWriter, request *http.Request, requestID string, status int, code, message string) {
	result := struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}{code, message, requestID}
	started := false
	if err := writeJSON(writer, request.Context(), requestID, status, result, &started); err != nil {
		panic(http.ErrAbortHandler)
	}
}

func writeSuccess(writer http.ResponseWriter, request *http.Request, requestID string, value any) error {
	started := false
	return writeJSON(writer, request.Context(), requestID, http.StatusOK, value, &started)
}

// Flush begins the actual HTTP/TLS response before the backend's gate callback
// returns. An enqueued, unflushed response is not an admitted network start.
func writeJSON(writer http.ResponseWriter, ctx context.Context, requestID string, status int, value any, started *bool) error {
	if ctx.Err() != nil {
		return runtimeaccess.ErrUnavailable
	}
	body, err := json.Marshal(value)
	if err != nil {
		return runtimeaccess.ErrUnavailable
	}
	deadline := time.Now().Add(MaxWriteDuration)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	control := http.NewResponseController(writer)
	if control.SetWriteDeadline(deadline) != nil {
		return runtimeaccess.ErrUnavailable
	}
	cancelledWrite := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(cancelledWrite)
		_ = control.SetWriteDeadline(time.Now())
	})
	defer func() {
		if !stop() {
			<-cancelledWrite
		}
	}()
	if ctx.Err() != nil {
		return runtimeaccess.ErrUnavailable
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Request-ID", requestID)
	*started = true
	writer.WriteHeader(status)
	if count, err := writer.Write(body); err != nil || count != len(body) {
		return runtimeaccess.ErrUnavailable
	}
	if ctx.Err() != nil || control.Flush() != nil {
		return runtimeaccess.ErrUnavailable
	}
	return nil
}
