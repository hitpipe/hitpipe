// Package workspaces exposes the two accepted private IAM workspace operations.
package workspaces

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

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
)

const Path = "/api/v1/workspaces"

// Backend calls start once, synchronously during the admitted response-start
// section. Create calls it only after the durable outcome is established.
type Backend interface {
	List(context.Context, runtimeaccess.Peer, string, func(context.Context, []app.Workspace) error) error
	Create(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, app.Workspace) error) error
}

type handler struct {
	backend Backend
	bff     map[runtimeaccess.Peer]struct{}
}

// New binds the HTTP routes to independently configured BFF peers. The
// listener uses privateiam.TLSConfig to verify the actual client certificate.
func New(backend Backend, bffPeers []runtimeaccess.Peer) (http.Handler, error) {
	if backend == nil || len(bffPeers) == 0 {
		return nil, errors.New("workspaces HTTP: backend and provisioned BFF peers are required")
	}
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, errors.New("workspaces HTTP: backend is required")
		}
	}
	bff := make(map[runtimeaccess.Peer]struct{}, len(bffPeers))
	for _, peer := range bffPeers {
		if !runtimeaccess.ValidDNSName(peer.DNSName) || !runtimeaccess.ValidFingerprint(peer.CertificateFingerprint) {
			return nil, errors.New("workspaces HTTP: invalid provisioned BFF peer")
		}
		peer.DNSName = strings.ToLower(peer.DNSName)
		if _, exists := bff[peer]; exists {
			return nil, errors.New("workspaces HTTP: duplicate provisioned BFF peer")
		}
		bff[peer] = struct{}{}
	}
	return handler{backend: backend, bff: bff}, nil
}

func (h handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestID, validID := correlationID(request)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Request-ID", requestID)
	if request.URL.Path != Path || request.URL.EscapedPath() != Path {
		writeError(writer, request, http.StatusNotFound, "NOT_FOUND", "The requested operation is unavailable.")
		return
	}
	peer, err := privateiam.TransportPeer(request.TLS)
	if err != nil {
		writeError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "Verified client transport is required.")
		return
	}
	if _, allowed := h.bff[peer]; !allowed || carriesAuthorityHeaders(request) {
		writeError(writer, request, http.StatusForbidden, "DENIED", "The current authority does not permit this operation.")
		return
	}
	if !validID || request.URL.RawQuery != "" || request.URL.ForceQuery ||
		(request.Method != http.MethodGet && request.Method != http.MethodPost) {
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "The request is invalid or unsupported.")
		return
	}
	session, validSession := secretHeader(request, "X-HitPipe-Session", 43)
	if !validSession {
		writeError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "A valid session is required.")
		return
	}
	if request.Context().Err() != nil {
		return
	}
	controller := http.NewResponseController(writer)
	response := responseStart{peer: peer}
	if request.Method == http.MethodGet {
		if request.ContentLength != 0 || len(request.TransferEncoding) != 0 || len(request.Header.Values("If-Match")) != 0 {
			writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "The request is invalid or unsupported.")
			return
		}
		if !canStartResponse(writer, controller) {
			writeBackendError(writer, request, app.ErrUnavailable)
			return
		}
		err = h.backend.List(request.Context(), peer, session, func(work context.Context, items []app.Workspace) error {
			if items == nil {
				items = []app.Workspace{}
			}
			return response.publish(writer, request, controller, work, http.StatusOK, workspaceList{Items: items}, validList(items))
		})
	} else {
		csrf, validCSRF := secretHeader(request, "X-CSRF-Token", 32)
		if !validCSRF {
			writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "A valid CSRF token is required.")
			return
		}
		command, readErr := readCommand(writer, request, controller)
		if readErr != nil {
			writeBackendError(writer, request, readErr)
			return
		}
		if !canStartResponse(writer, controller) {
			writeBackendError(writer, request, app.ErrUnavailable)
			return
		}
		err = h.backend.Create(request.Context(), peer, session, csrf, command, func(work context.Context, workspace app.Workspace) error {
			return response.publish(writer, request, controller, work, http.StatusCreated, workspace, validWorkspace(workspace) && workspace.Role == "OWNER")
		})
	}
	response.finish(writer, request, err)
}

var errTooLarge = errors.New("workspaces HTTP: body exceeds size limit")

func readCommand(writer http.ResponseWriter, request *http.Request, controller *http.ResponseController) (app.Command, error) {
	contentTypes := request.Header.Values("Content-Type")
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if len(contentTypes) != 1 || err != nil || !strings.EqualFold(mediaType, "application/json") || len(request.Header.Values("Content-Encoding")) != 0 {
		return app.Command{}, app.ErrInvalid
	}
	if request.ContentLength > privateiam.MaxRequestBytes {
		return app.Command{}, errTooLarge
	}
	if request.Body == nil {
		return app.Command{}, app.ErrInvalid
	}
	if controller.SetReadDeadline(shortDeadline(request.Context())) != nil {
		return app.Command{}, app.ErrUnavailable
	}
	defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, privateiam.MaxRequestBytes))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		return app.Command{}, errTooLarge
	}
	if err != nil || request.Context().Err() != nil {
		return app.Command{}, app.ErrUnavailable
	}
	command, valid := parseCommand(body)
	if !valid {
		return app.Command{}, app.ErrInvalid
	}
	// A creation has no existing aggregate revision. CAS inputs may not be
	// applied to a new aggregate or change a command's durable identity.
	if command.ExpectedRevision != nil || len(request.Header.Values("If-Match")) != 0 {
		return app.Command{}, app.ErrStale
	}
	return command, nil
}

func validList(items []app.Workspace) bool {
	if len(items) > maxWorkspaceItems {
		return false
	}
	for _, item := range items {
		if !validWorkspace(item) {
			return false
		}
	}
	return true
}

type responseStart struct {
	mu                        sync.Mutex
	peer                      runtimeaccess.Peer
	called, finished, started bool
	err                       error
}

func (r *responseStart) publish(writer http.ResponseWriter, request *http.Request, controller *http.ResponseController, work context.Context, status int, result any, valid bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		return app.ErrUnavailable
	}
	if r.called {
		r.err = app.ErrUnavailable
		return r.err
	}
	r.called = true
	if !valid || work == nil {
		r.err = app.ErrUnavailable
		return r.err
	}
	r.err = r.write(writer, request, controller, work, status, result)
	return r.err
}

func (r *responseStart) write(writer http.ResponseWriter, request *http.Request, controller *http.ResponseController, work context.Context, status int, result any) error {
	if request.Context().Err() != nil || work.Err() != nil {
		return app.ErrUnavailable
	}
	writeContext, cancel := context.WithDeadline(work, shortDeadline(request.Context()))
	stopRequest := context.AfterFunc(request.Context(), cancel)
	defer func() { stopRequest(); cancel() }()
	deadline, _ := writeContext.Deadline()
	body, err := json.Marshal(result)
	if err != nil || controller.SetWriteDeadline(deadline) != nil {
		return app.ErrUnavailable
	}
	stopWrite := interruptWriteOnCancel(writeContext, controller)
	defer func() {
		stopWrite()
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	peer, err := privateiam.TransportPeer(request.TLS)
	if err != nil || peer != r.peer {
		return app.ErrDenied
	}
	if writeContext.Err() != nil || request.Context().Err() != nil {
		return app.ErrUnavailable
	}
	writer.Header().Set("Content-Type", "application/json")
	r.started = true
	writer.WriteHeader(status)
	if count, err := writer.Write(body); err != nil || count != len(body) {
		return app.ErrUnavailable
	}
	// Flush the first bytes while still inside the shared gate's callback.
	if writeContext.Err() != nil || request.Context().Err() != nil || controller.Flush() != nil {
		return app.ErrUnavailable
	}
	return nil
}

func (r *responseStart) finish(writer http.ResponseWriter, request *http.Request, err error) {
	r.mu.Lock()
	r.finished = true
	started := r.started
	if err == nil {
		err = r.err
	}
	r.mu.Unlock()
	if started {
		if err != nil {
			panic(http.ErrAbortHandler)
		}
		return
	}
	writeBackendError(writer, request, err)
}

func interruptWriteOnCancel(ctx context.Context, controller *http.ResponseController) func() {
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(finished)
		_ = controller.SetWriteDeadline(time.Now())
	})
	return func() {
		if !stop() {
			<-finished
		}
	}
}

func shortDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(privateiam.MaxWriteDuration)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		return limit
	}
	return deadline
}

func canStartResponse(writer http.ResponseWriter, controller *http.ResponseController) bool {
	// An unsupported transport must fail before a create can reach SQL.
	return canFlush(writer) && controller.SetWriteDeadline(time.Time{}) == nil
}

func canFlush(writer http.ResponseWriter) bool {
	for range 16 {
		if _, ok := writer.(interface{ FlushError() error }); ok {
			return true
		}
		if _, ok := writer.(http.Flusher); ok {
			return true
		}
		wrapper, ok := writer.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		writer = wrapper.Unwrap()
	}
	return false
}

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeBackendError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, app.ErrInvalid):
		writeError(writer, request, http.StatusBadRequest, "INVALID_REQUEST", "The request is invalid or unsupported.")
	case errors.Is(err, app.ErrUnauthenticated):
		writeError(writer, request, http.StatusUnauthorized, "UNAUTHENTICATED", "A valid session is required.")
	case errors.Is(err, app.ErrDenied):
		writeError(writer, request, http.StatusForbidden, "DENIED", "The current authority does not permit this operation.")
	case errors.Is(err, app.ErrConflict):
		writeError(writer, request, http.StatusConflict, "CONFLICT", "The command conflicts with current state.")
	case errors.Is(err, app.ErrStale):
		writeError(writer, request, http.StatusPreconditionFailed, "STALE_REVISION", "The request revision cannot be applied.")
	case errors.Is(err, errTooLarge):
		writeError(writer, request, http.StatusRequestEntityTooLarge, "LIMIT_EXCEEDED", "The request exceeds the size limit.")
	default:
		writeError(writer, request, http.StatusServiceUnavailable, "UNAVAILABLE", "The operation could not be established.")
	}
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	if request.Context().Err() != nil {
		return
	}
	body, err := json.Marshal(errorResponse{Code: code, Message: message, RequestID: writer.Header().Get("X-Request-ID")})
	if err != nil {
		panic(http.ErrAbortHandler)
	}
	_ = http.NewResponseController(writer).SetWriteDeadline(shortDeadline(request.Context()))
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_, _ = writer.Write(body)
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
