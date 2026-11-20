package privateiam

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const unitRequestID = "10101010-1010-4010-8010-101010101010"
const unitRuntimeID = "20202020-2020-4020-8020-202020202020"
const unitBootID = "30303030-3030-4030-8030-303030303030"
const unitAuthorizeJSON = `{"workspace_id":"40404040-4040-4040-8040-404040404040","action":"WORKSPACE_READ","resource_id":null,"runtime_id":"20202020-2020-4020-8020-202020202020"}`

type unitBackend struct {
	registerCalls, authorizeCalls int
	register                      func(runtimeaccess.Peer, runtimeaccess.Registration) (runtimeaccess.Snapshot, error)
	authorize                     func(context.Context, runtimeaccess.Peer, string, authorizeaction.Request, func(context.Context, authorizeaction.Authorization) error) error
}

func (b *unitBackend) Register(_ context.Context, peer runtimeaccess.Peer, input runtimeaccess.Registration) (runtimeaccess.Snapshot, error) {
	b.registerCalls++
	if b.register != nil {
		return b.register(peer, input)
	}
	return runtimeaccess.Snapshot{RuntimeID: input.RuntimeID, BootID: input.BootID, Generation: input.Generation, Revision: 1, Gate: "CLOSED"}, nil
}

func (b *unitBackend) Authorize(ctx context.Context, peer runtimeaccess.Peer, secret string, input authorizeaction.Request, respond func(context.Context, authorizeaction.Authorization) error) error {
	b.authorizeCalls++
	if b.authorize != nil {
		return b.authorize(ctx, peer, secret, input, respond)
	}
	return respond(ctx, unitAuthorization())
}

func unitAuthorization() authorizeaction.Authorization {
	return authorizeaction.Authorization{Allowed: true, Epoch: 1, CheckedAt: time.Now().UTC()}
}

type unitWriter struct {
	*httptest.ResponseRecorder
	deadline time.Time
	statuses []int
}

func (w *unitWriter) SetWriteDeadline(value time.Time) error { w.deadline = value; return nil }
func (w *unitWriter) WriteHeader(status int) {
	w.statuses = append(w.statuses, status)
	w.ResponseRecorder.WriteHeader(status)
}
func (w *unitWriter) FlushError() error { w.ResponseRecorder.Flush(); return nil }

func unitRequest(t *testing.T, path, body string) *http.Request {
	t.Helper()
	leaf := unitLeaf(t, []string{"bff.hit115.test"})
	request := httptest.NewRequest(http.MethodPost, "https://iam.hit115.test"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", unitRequestID)
	request.Header.Set("X-HitPipe-Session", strings.Repeat("x", 43))
	request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true, PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	return request
}

func unitServe(t *testing.T, backend *unitBackend, request *http.Request) *unitWriter {
	t.Helper()
	handler, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	writer := &unitWriter{ResponseRecorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, request)
	return writer
}

func TestRegistrationRequiredNullDuplicateAndRange(t *testing.T) {
	valid := map[string]any{"runtime_id": unitRuntimeID, "service": "BFF", "generation": 1, "boot_id": unitBootID, "certificate_fingerprint": "sha256:" + strings.Repeat("a", 64)}
	encode := func(value map[string]any) string {
		out, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	baseline := encode(valid)
	if _, err := parseRegistration([]byte(baseline)); err != nil {
		t.Fatal(err)
	}
	for key := range valid {
		t.Run("missing_"+key, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range valid {
				if k != key {
					copy[k] = v
				}
			}
			if _, err := parseRegistration([]byte(encode(copy))); err == nil {
				t.Fatal("accepted absent required field")
			}
		})
		t.Run("null_"+key, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range valid {
				copy[k] = v
			}
			copy[key] = nil
			if _, err := parseRegistration([]byte(encode(copy))); err == nil {
				t.Fatal("accepted null required field")
			}
		})
	}
	for name, body := range map[string]string{
		"duplicate":  strings.Replace(baseline, `"generation":1`, `"generation":1,"generation":2`, 1),
		"unknown":    strings.TrimSuffix(baseline, "}") + `,"workspace_id":"` + unitRuntimeID + `"}`,
		"fractional": strings.Replace(baseline, `"generation":1`, `"generation":1.5`, 1),
		"quoted":     strings.Replace(baseline, `"generation":1`, `"generation":"1"`, 1),
		"overflow":   strings.Replace(baseline, `"generation":1`, `"generation":9007199254740992`, 1),
		"zero":       strings.Replace(baseline, `"generation":1`, `"generation":0`, 1),
		"uuid":       strings.Replace(baseline, unitRuntimeID, "not-a-uuid", 1),
		"trailing":   baseline + ` {}`, "array": `[]`, "null": `null`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRegistration([]byte(body)); err == nil {
				t.Fatal("accepted invalid registration")
			}
		})
	}
}

func TestHandlerUsesOnlyActualVerifiedPeerAndStrictWire(t *testing.T) {
	backend := &unitBackend{}
	backend.authorize = func(ctx context.Context, peer runtimeaccess.Peer, secret string, _ authorizeaction.Request, respond func(context.Context, authorizeaction.Authorization) error) error {
		if peer.DNSName != "bff.hit115.test" || len(peer.CertificateFingerprint) != 71 || secret != strings.Repeat("x", 43) {
			t.Fatal("incorrect trusted inputs")
		}
		return respond(ctx, unitAuthorization())
	}
	request := unitRequest(t, AuthorizePath, unitAuthorizeJSON)
	request.Header.Set("X-HitPipe-Service", "DELIVERY")
	request.Header.Set("X-Client-Cert", "unverified-header")
	writer := unitServe(t, backend, request)
	if writer.Code != 200 || backend.authorizeCalls != 1 || !writer.Flushed || len(writer.statuses) != 1 {
		t.Fatalf("unexpected response %d", writer.Code)
	}
	if strings.Contains(writer.Body.String(), strings.Repeat("x", 43)) {
		t.Fatal("response leaked secret")
	}
	for _, bad := range []string{
		`{"workspace_id":"40404040-4040-4040-8040-404040404040","action":"WORKSPACE_READ","runtime_id":"20202020-2020-4020-8020-202020202020"}`,
		strings.TrimSuffix(unitAuthorizeJSON, "}") + `,"role":"OWNER"}`,
		strings.Replace(unitAuthorizeJSON, `"action":"WORKSPACE_READ"`, `"action":"WORKSPACE_READ","action":"PROFILE_READ"`, 1),
	} {
		if response := unitServe(t, backend, unitRequest(t, AuthorizePath, bad)); response.Code != 400 {
			t.Fatal("accepted invalid authorize wire")
		}
	}
	if backend.authorizeCalls != 1 {
		t.Fatal("invalid input reached backend")
	}
}

func TestHeadersTLSAndBodyLimitsFailBeforeBackend(t *testing.T) {
	for name, alter := range map[string]func(*http.Request){
		"noTLS":            func(r *http.Request) { r.TLS = nil },
		"unverified":       func(r *http.Request) { r.TLS.VerifiedChains = nil },
		"requestID":        func(r *http.Request) { r.Header.Del("X-Request-ID") },
		"duplicateID":      func(r *http.Request) { r.Header.Add("X-Request-ID", unitRequestID) },
		"session":          func(r *http.Request) { r.Header.Del("X-HitPipe-Session") },
		"duplicateSession": func(r *http.Request) { r.Header.Add("X-HitPipe-Session", strings.Repeat("y", 43)) },
		"contentType":      func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") },
		"query":            func(r *http.Request) { r.URL.RawQuery = "role=OWNER" },
	} {
		t.Run(name, func(t *testing.T) {
			backend := &unitBackend{}
			request := unitRequest(t, AuthorizePath, unitAuthorizeJSON)
			alter(request)
			writer := unitServe(t, backend, request)
			if writer.Code < 400 || backend.authorizeCalls != 0 {
				t.Fatal("untrusted request reached backend")
			}
			var envelope struct {
				Code, Message string
				RequestID     string `json:"request_id"`
			}
			if json.Unmarshal(writer.Body.Bytes(), &envelope) != nil || envelope.Code == "" || envelope.Message == "" || !runtimeaccess.ValidUUID(envelope.RequestID) {
				t.Fatal("invalid error envelope")
			}
		})
	}
	for _, length := range []int64{-1, MaxRequestBytes + 1} {
		backend := &unitBackend{}
		request := unitRequest(t, RegisterPath, strings.Repeat(" ", MaxRequestBytes+1))
		request.ContentLength = length
		if writer := unitServe(t, backend, request); writer.Code != 413 || backend.registerCalls != 0 {
			t.Fatal("size bound was bypassed")
		}
	}
}

func TestAuthorizationRequiresSynchronousCallbackAndGateContext(t *testing.T) {
	backend := &unitBackend{authorize: func(context.Context, runtimeaccess.Peer, string, authorizeaction.Request, func(context.Context, authorizeaction.Authorization) error) error {
		return nil
	}}
	if writer := unitServe(t, backend, unitRequest(t, AuthorizePath, unitAuthorizeJSON)); writer.Code != 503 {
		t.Fatal("nil backend result granted success without callback")
	}
	gateContext, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	backend.authorize = func(_ context.Context, _ runtimeaccess.Peer, _ string, _ authorizeaction.Request, respond func(context.Context, authorizeaction.Authorization) error) error {
		return respond(gateContext, unitAuthorization())
	}
	writer := unitServe(t, backend, unitRequest(t, AuthorizePath, unitAuthorizeJSON))
	deadline, _ := gateContext.Deadline()
	if writer.Code != 200 || !writer.deadline.Equal(deadline) {
		t.Fatal("write extended beyond gate deadline")
	}
	cancel()
	if writer = unitServe(t, backend, unitRequest(t, AuthorizePath, unitAuthorizeJSON)); writer.Code != 503 {
		t.Fatal("cancelled gate context granted success")
	}
	var delayed func(context.Context, authorizeaction.Authorization) error
	backend.authorize = func(_ context.Context, _ runtimeaccess.Peer, _ string, _ authorizeaction.Request, respond func(context.Context, authorizeaction.Authorization) error) error {
		delayed = respond
		return nil
	}
	writer = unitServe(t, backend, unitRequest(t, AuthorizePath, unitAuthorizeJSON))
	before := writer.Body.String()
	if delayed(context.Background(), unitAuthorization()) == nil || writer.Body.String() != before {
		t.Fatal("late callback wrote a response")
	}
}

func TestBackendErrorsStayRedactedAndUseAcceptedStatuses(t *testing.T) {
	for err, status := range map[error]int{runtimeaccess.ErrDenied: 403, runtimeaccess.ErrConflict: 409, runtimeaccess.ErrStale: 412, runtimeaccess.ErrUnavailable: 503, runtimeaccess.ErrInvalid: 400} {
		backend := &unitBackend{authorize: func(context.Context, runtimeaccess.Peer, string, authorizeaction.Request, func(context.Context, authorizeaction.Authorization) error) error {
			return fmt.Errorf("sensitive SQL argument: %w", err)
		}}
		writer := unitServe(t, backend, unitRequest(t, AuthorizePath, unitAuthorizeJSON))
		if writer.Code != status || strings.Contains(writer.Body.String(), "sensitive SQL") {
			t.Fatal("wrong status or leaked diagnostic")
		}
	}
	var nilBackend *unitBackend
	if _, err := New(nilBackend); err == nil {
		t.Fatal("typed nil backend accepted")
	}
	if _, err := New(nil); err == nil {
		t.Fatal("nil backend accepted")
	}
}

func TestRegistrationResponseContainsOnlyExactSnapshotFields(t *testing.T) {
	input := fmt.Sprintf(`{"runtime_id":%q,"service":"BFF","generation":1,"boot_id":%q,"certificate_fingerprint":%q}`, unitRuntimeID, unitBootID, "sha256:"+strings.Repeat("a", 64))
	backend := &unitBackend{}
	writer := unitServe(t, backend, unitRequest(t, RegisterPath, input))
	var actual map[string]json.RawMessage
	if writer.Code != 200 || json.Unmarshal(writer.Body.Bytes(), &actual) != nil || len(actual) != 6 || string(actual["denials"]) != "[]" {
		t.Fatal("incorrect snapshot wire")
	}
}
