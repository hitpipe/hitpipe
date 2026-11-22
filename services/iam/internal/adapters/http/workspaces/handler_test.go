package workspaces

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
)

const (
	correlation = "998b08b8-9ed5-4d1f-8ef8-c0425040337a"
	commandID   = "9ecFD0c9-8e9a-4662-8Fa1-f831585236ae"
	workspaceID = "fcebdc34-fb07-4fe0-a287-eb137782607d"
	createBody  = `{"name":"Рабочая область","command":{"request_id":"` + commandID + `"}}`
)

var (
	session = strings.Repeat("s", 43)
	csrf    = strings.Repeat("c", 32)
	created = app.Workspace{WorkspaceID: workspaceID, Name: "Рабочая область", Role: "OWNER", Revision: 1, SecurityEpoch: 1, Lifecycle: "ACTIVE"}
)

type testBackend struct {
	list   func(context.Context, runtimeaccess.Peer, string, func(context.Context, []app.Workspace) error) error
	create func(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, app.Workspace) error) error
}

func (b testBackend) List(ctx context.Context, peer runtimeaccess.Peer, session string, start func(context.Context, []app.Workspace) error) error {
	return b.list(ctx, peer, session, start)
}

func (b testBackend) Create(ctx context.Context, peer runtimeaccess.Peer, session, csrf string, command app.Command, start func(context.Context, app.Workspace) error) error {
	return b.create(ctx, peer, session, csrf, command, start)
}

type testWriter struct {
	*httptest.ResponseRecorder
	flush func()
}

func (*testWriter) SetReadDeadline(time.Time) error  { return nil }
func (*testWriter) SetWriteDeadline(time.Time) error { return nil }

func (w *testWriter) FlushError() error {
	if w.flush != nil {
		w.flush()
	}
	w.Flush()
	return nil
}

func requestFor(method, path, body string) (*http.Request, runtimeaccess.Peer) {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	leaf := &x509.Certificate{
		Raw: []byte("workspace test leaf"), DNSNames: []string{"bff.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
	}
	request.TLS = &tls.ConnectionState{
		Version: tls.VersionTLS13, HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}},
	}
	request.Header.Set("X-HitPipe-Session", session)
	if method == http.MethodPost {
		request.Header.Set("X-Request-ID", correlation)
		request.Header.Set("X-CSRF-Token", csrf)
		request.Header.Set("Content-Type", "application/json")
	}
	digest := sha256.Sum256(leaf.Raw)
	return request, runtimeaccess.Peer{DNSName: leaf.DNSNames[0], CertificateFingerprint: "sha256:" + hex.EncodeToString(digest[:])}
}

func TestCreateSeparatesCorrelationAndCommandAndFlushesBeforeReturn(t *testing.T) {
	request, peer := requestFor(http.MethodPost, Path, createBody)
	inside := false
	backend := testBackend{create: func(ctx context.Context, actualPeer runtimeaccess.Peer, actualSession, actualCSRF string, cmd app.Command, start func(context.Context, app.Workspace) error) error {
		if actualPeer != peer || actualSession != session || actualCSRF != csrf || cmd.RequestID != commandID || cmd.Name != created.Name {
			t.Fatal("unexpected trusted HTTP projection")
		}
		inside = true
		defer func() { inside = false }()
		return start(ctx, created)
	}}
	handler, err := New(backend, []runtimeaccess.Peer{peer})
	if err != nil {
		t.Fatal(err)
	}
	writer := &testWriter{ResponseRecorder: httptest.NewRecorder(), flush: func() {
		if !inside {
			t.Fatal("response flushed outside gate callback")
		}
	}}
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusCreated || !writer.Flushed || writer.Header().Get("X-Request-ID") != correlation || writer.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %d, headers %v", writer.Code, writer.Header())
	}
	var result app.Workspace
	if err := json.Unmarshal(writer.Body.Bytes(), &result); err != nil || result != created {
		t.Fatalf("wrong workspace response: %v, err %v", result, err)
	}
}

func TestListWireEnvelopeAndUnavailableProjection(t *testing.T) {
	for _, tc := range []struct {
		name string
		rows []app.Workspace
		want int
	}{
		{"empty", nil, http.StatusOK},
		{"visible", []app.Workspace{created}, http.StatusOK},
		{"over limit", make([]app.Workspace, maxWorkspaceItems+1), http.StatusServiceUnavailable},
		{"onboarding", []app.Workspace{{WorkspaceID: workspaceID, Name: "internal", Role: "OWNER", Revision: 1, SecurityEpoch: 1, Lifecycle: "ONBOARDING"}}, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, peer := requestFor(http.MethodGet, Path, "")
			backend := testBackend{list: func(ctx context.Context, _ runtimeaccess.Peer, _ string, start func(context.Context, []app.Workspace) error) error {
				return start(ctx, tc.rows)
			}}
			handler, err := New(backend, []runtimeaccess.Peer{peer})
			if err != nil {
				t.Fatal(err)
			}
			writer := &testWriter{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(writer, request)
			if writer.Code != tc.want || !runtimeaccess.ValidUUID(writer.Header().Get("X-Request-ID")) {
				t.Fatalf("got %d: %s", writer.Code, writer.Body.String())
			}
			if tc.rows == nil && writer.Body.String() != `{"items":[]}` {
				t.Fatalf("empty list does not satisfy Workspaces: %s", writer.Body.String())
			}
			if tc.want != http.StatusOK && (writer.Flushed || strings.Contains(writer.Body.String(), "ONBOARDING")) {
				t.Fatal("internal lifecycle reached the wire")
			}
		})
	}
}

func TestMalformedAndUntrustedRequestsNeverReachBackend(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		change     func(*http.Request)
		want       int
	}{
		{"unknown key", `{"name":"x","command":{"request_id":"` + commandID + `"},"role":"OWNER"}`, nil, 400},
		{"duplicate command", `{"name":"x","command":{"request_id":"` + commandID + `","request\u005fid":"` + commandID + `"}}`, nil, 400},
		{"lone surrogate", `{"name":"\ud800","command":{"request_id":"` + commandID + `"}}`, nil, 400},
		{"overlong name", `{"name":"` + strings.Repeat("я", 121) + `","command":{"request_id":"` + commandID + `"}}`, nil, 400},
		{"null revision", `{"name":"x","command":{"request_id":"` + commandID + `","expected_revision":null}}`, nil, 400},
		{"valid but inapplicable revision", `{"name":"x","command":{"request_id":"` + commandID + `","expected_revision":1}}`, nil, 412},
		{"if match", createBody, func(r *http.Request) { r.Header.Set("If-Match", `"1"`) }, 412},
		{"missing correlation", createBody, func(r *http.Request) { r.Header.Del("X-Request-ID") }, 400},
		{"unsupported media type", createBody, func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 400},
		{"unsupported content coding", createBody, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, 400},
		{"ambiguous csrf", createBody, func(r *http.Request) { r.Header.Add("X-CSRF-Token", csrf) }, 400},
		{"missing session", createBody, func(r *http.Request) { r.Header.Del("X-HitPipe-Session") }, 401},
		{"forged role", createBody, func(r *http.Request) { r.Header.Set("X-HitPipe-Role", "OWNER") }, 403},
		{"no TLS", createBody, func(r *http.Request) { r.TLS = nil }, 401},
		{"wrong fingerprint", createBody, func(r *http.Request) { r.TLS.PeerCertificates[0].Raw = []byte("peer changes") }, 403},
		{"oversize", strings.Repeat("x", privateiam.MaxRequestBytes+1), nil, 413},
		{"oversize streamed", strings.Repeat("x", privateiam.MaxRequestBytes+1), func(r *http.Request) { r.ContentLength = -1 }, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, peer := requestFor(http.MethodPost, Path, tc.body)
			if tc.change != nil {
				tc.change(request)
			}
			handler, err := New(testBackend{}, []runtimeaccess.Peer{peer})
			if err != nil {
				t.Fatal(err)
			}
			writer := &testWriter{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(writer, request)
			var response errorResponse
			if writer.Code != tc.want || json.Unmarshal(writer.Body.Bytes(), &response) != nil || !runtimeaccess.ValidUUID(response.RequestID) || response.Code == "" {
				t.Fatalf("got %d: %s", writer.Code, writer.Body.String())
			}
		})
	}
}

func TestCertificateExpiryBetweenValidationAndStart(t *testing.T) {
	request, peer := requestFor(http.MethodGet, Path, "")
	backend := testBackend{list: func(ctx context.Context, _ runtimeaccess.Peer, _ string, start func(context.Context, []app.Workspace) error) error {
		request.TLS.PeerCertificates[0].NotAfter = time.Now().Add(-time.Minute)
		return start(ctx, []app.Workspace{created})
	}}
	handler, err := New(backend, []runtimeaccess.Peer{peer})
	if err != nil {
		t.Fatal(err)
	}
	writer := &testWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusForbidden || writer.Flushed || strings.Contains(writer.Body.String(), created.Name) {
		t.Fatal("expired certificate started a protected response")
	}
}

func TestUnboundedTransportAndUncertainOutcomeDoNotReportSuccess(t *testing.T) {
	request, peer := requestFor(http.MethodPost, Path, createBody)
	calls := 0
	backend := testBackend{create: func(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, app.Workspace) error) error {
		calls++
		return app.ErrUncertain
	}}
	handler, err := New(backend, []runtimeaccess.Peer{peer})
	if err != nil {
		t.Fatal(err)
	}
	unsupported := httptest.NewRecorder()
	handler.ServeHTTP(unsupported, request)
	if unsupported.Code != http.StatusServiceUnavailable || calls != 0 {
		t.Fatal("unbounded transport dispatched a command")
	}
	request, _ = requestFor(http.MethodPost, Path, createBody)
	writer := &testWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusServiceUnavailable || writer.Flushed || calls != 1 {
		t.Fatal("uncertain committed outcome was serialized as success or automatically retried")
	}
}
