package privateiam

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

type authorizeBodyBackend struct {
	calls  atomic.Int64
	inputs chan authorizeaction.Request
}

func (*authorizeBodyBackend) Register(context.Context, runtimeaccess.Peer, runtimeaccess.Registration) (runtimeaccess.Snapshot, error) {
	return runtimeaccess.Snapshot{}, runtimeaccess.ErrUnavailable
}

func (backend *authorizeBodyBackend) Authorize(ctx context.Context, _ runtimeaccess.Peer, _ string, input authorizeaction.Request, emit func(context.Context, authorizeaction.Authorization) error) error {
	backend.calls.Add(1)
	backend.inputs <- input
	return emit(ctx, unitAuthorization())
}

// Exercise the actual POST boundary over a CA-verified, provisioned mTLS peer.
// The controlled backend observes only inputs that passed the reused validator.
func TestAuthorizePOSTBodyValidationOverMTLS(t *testing.T) {
	ca, caKey := unitCA(t)
	serverCertificate := unitIssued(t, ca, caKey, []string{"iam.hit115.test"}, x509.ExtKeyUsageServerAuth, false)
	clientCertificate := unitIssued(t, ca, caKey, []string{"bff.hit115.test"}, x509.ExtKeyUsageClientAuth, false)
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	tlsConfig, err := TLSConfig(serverCertificate, pool, []runtimeaccess.Peer{unitPeer(clientCertificate.Leaf)})
	if err != nil {
		t.Fatal(err)
	}
	backend := &authorizeBodyBackend{inputs: make(chan authorizeaction.Request, 1)}
	handler, err := New(backend)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs:      pool,
		ServerName:   "iam.hit115.test",
		Certificates: []tls.Certificate{clientCertificate},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}

	baseline := map[string]any{
		"workspace_id": "40404040-4040-4040-8040-404040404040",
		"action":       "WORKSPACE_READ",
		"resource_id":  nil,
		"runtime_id":   unitRuntimeID,
	}
	encode := func(change func(map[string]any)) string {
		fields := make(map[string]any, len(baseline))
		for key, value := range baseline {
			fields[key] = value
		}
		if change != nil {
			change(fields)
		}
		body, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	type bodyCase struct {
		name     string
		body     string
		status   int
		resource *string
	}
	resourceID := "50505050-5050-4050-8050-505050505050"
	validJSON := encode(nil)
	cases := []bodyCase{
		{name: "required_resource_null", body: validJSON, status: http.StatusOK},
		{name: "resource_uuid", body: encode(func(fields map[string]any) { fields["resource_id"] = resourceID }), status: http.StatusOK, resource: &resourceID},
		{name: "unknown_field", body: encode(func(fields map[string]any) { fields["role"] = "OWNER" }), status: http.StatusBadRequest},
		{name: "unknown_action", body: encode(func(fields map[string]any) { fields["action"] = "BYPASS" }), status: http.StatusBadRequest},
		{name: "wrong_action_type", body: encode(func(fields map[string]any) { fields["action"] = true }), status: http.StatusBadRequest},
		{name: "trailing_document", body: validJSON + " {}", status: http.StatusBadRequest},
		{name: "array_body", body: "[]", status: http.StatusBadRequest},
		{name: "null_body", body: "null", status: http.StatusBadRequest},
	}
	for _, field := range []string{"workspace_id", "action", "resource_id", "runtime_id"} {
		cases = append(cases, bodyCase{
			name:   "missing_" + field,
			body:   encode(func(fields map[string]any) { delete(fields, field) }),
			status: http.StatusBadRequest,
		})
		value, err := json.Marshal(baseline[field])
		if err != nil {
			t.Fatal(err)
		}
		cases = append(cases, bodyCase{
			name:   "duplicate_" + field,
			body:   strings.TrimSuffix(validJSON, "}") + ",\"" + field + "\":" + string(value) + "}",
			status: http.StatusBadRequest,
		})
	}
	for _, field := range []string{"workspace_id", "runtime_id", "resource_id"} {
		cases = append(cases,
			bodyCase{name: "invalid_uuid_" + field, body: encode(func(fields map[string]any) { fields[field] = "invalid-uuid" }), status: http.StatusBadRequest},
			bodyCase{name: "wrong_uuid_type_" + field, body: encode(func(fields map[string]any) { fields[field] = 123 }), status: http.StatusBadRequest},
		)
	}
	for _, field := range []string{"workspace_id", "action", "runtime_id"} {
		cases = append(cases, bodyCase{
			name:   "null_" + field,
			body:   encode(func(fields map[string]any) { fields[field] = nil }),
			status: http.StatusBadRequest,
		})
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			before := backend.calls.Load()
			request, err := http.NewRequest(http.MethodPost, server.URL+AuthorizePath, strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Request-ID", unitRequestID)
			request.Header.Set("X-HitPipe-Session", strings.Repeat("x", 43))
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			if response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || response.StatusCode != test.status {
				t.Fatalf("expected verified TLS1.3 HTTP %d, got %d", test.status, response.StatusCode)
			}
			if test.status == http.StatusBadRequest {
				if backend.calls.Load() != before {
					t.Fatal("invalid POST reached authorization backend")
				}
				var envelope struct {
					Code      string `json:"code"`
					Message   string `json:"message"`
					RequestID string `json:"request_id"`
				}
				if json.Unmarshal(body, &envelope) != nil || envelope.Code != "INVALID_REQUEST" || envelope.Message == "" || envelope.RequestID != unitRequestID {
					t.Fatal("invalid request did not return the accepted error envelope")
				}
				return
			}
			if backend.calls.Load() != before+1 {
				t.Fatal("valid POST did not reach authorization exactly once")
			}
			input := <-backend.inputs
			if input.WorkspaceID != baseline["workspace_id"] || input.RuntimeID != unitRuntimeID || input.Action != baseline["action"] {
				t.Fatal("valid POST context changed during decoding")
			}
			if test.resource == nil && input.ResourceID != nil || test.resource != nil && (input.ResourceID == nil || *input.ResourceID != *test.resource) {
				t.Fatal("resource null/UUID semantics were not preserved")
			}
		})
	}
}
