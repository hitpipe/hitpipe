package profileupdate

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const (
	commandID = "9ecFD0c9-8e9a-4662-8Fa1-f831585236ae"
	body      = `{"display_name":"Алиса","timezone":"Europe/Moscow","command":{"request_id":"` + commandID + `","expected_revision":7}}`
)

var (
	session = strings.Repeat("s", 43)
	csrf    = strings.Repeat("c", 32)
	result  = getprofile.Profile{UserID: "fcebdc34-fb07-4fe0-a287-eb137782607d", DisplayName: "Алиса", Timezone: "Europe/Moscow", Revision: 8}
)

type updateFunc func(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, getprofile.Profile) error) error

func (f updateFunc) Update(ctx context.Context, peer runtimeaccess.Peer, session, csrf string, command app.Command,
	start func(context.Context, getprofile.Profile) error) error {
	return f(ctx, peer, session, csrf, command, start)
}

type testWriter struct {
	*httptest.ResponseRecorder
	flush func() error
}

func (*testWriter) SetReadDeadline(time.Time) error  { return nil }
func (*testWriter) SetWriteDeadline(time.Time) error { return nil }

func (w *testWriter) FlushError() error {
	if w.flush != nil {
		if err := w.flush(); err != nil {
			return err
		}
	}
	w.Flush()
	return nil
}

func patchRequest(body string) (*http.Request, runtimeaccess.Peer) {
	r := httptest.NewRequest(http.MethodPatch, Path, strings.NewReader(body))
	leaf := &x509.Certificate{Raw: []byte("test BFF leaf"), DNSNames: []string{"bff.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	r.TLS = &tls.ConnectionState{Version: tls.VersionTLS13, HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf}}}
	r.Header.Set("X-Request-ID", strings.ToLower(commandID))
	r.Header.Set("X-HitPipe-Session", session)
	r.Header.Set("X-CSRF-Token", csrf)
	r.Header.Set("Content-Type", "application/json")
	digest := sha256.Sum256(leaf.Raw)
	return r, runtimeaccess.Peer{DNSName: "bff.test", CertificateFingerprint: "sha256:" + hex.EncodeToString(digest[:])}
}

func newHandler(t *testing.T, backend Backend, peer runtimeaccess.Peer) http.Handler {
	t.Helper()
	h, err := New(backend, []runtimeaccess.Peer{peer})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestPatchForwardsExactCommandAndFlushesUnderGate(t *testing.T) {
	r, peer := patchRequest(body)
	r.Header.Set("If-Match", `"7"`)
	inside := false
	backend := updateFunc(func(work context.Context, caller runtimeaccess.Peer, gotSession, gotCSRF string, cmd app.Command, start func(context.Context, getprofile.Profile) error) error {
		if caller != peer || gotSession != session || gotCSRF != csrf ||
			cmd != (app.Command{RequestID: commandID, DisplayName: result.DisplayName, Timezone: result.Timezone, ExpectedRevision: 7}) {
			t.Fatal("unexpected trusted update projection")
		}
		inside = true
		defer func() { inside = false }()
		return start(work, result)
	})
	w := &testWriter{ResponseRecorder: httptest.NewRecorder(), flush: func() error {
		if !inside {
			t.Fatal("response flushed after the gate callback")
		}
		return nil
	}}
	newHandler(t, backend, peer).ServeHTTP(w, r)
	var got getprofile.Profile
	if w.Code != http.StatusOK || !w.Flushed || w.Header().Get("Cache-Control") != "no-store" ||
		w.Header().Get("X-Request-ID") != strings.ToLower(commandID) ||
		json.Unmarshal(w.Body.Bytes(), &got) != nil || got != result {
		t.Fatal("successful update did not return the exact full Profile")
	}
}

func TestInvalidPatchDoesNotReachBackend(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		change      func(*http.Request)
		status      int
	}{
		{"unknown field", strings.Replace(body, `"timezone":`, `"user_id":"client", "timezone":`, 1), nil, 400},
		{"escaped duplicate", strings.Replace(body, `"timezone":`, `"time\u007aone":"x", "timezone":`, 1), nil, 400},
		{"unknown nested", strings.Replace(body, `"expected_revision":`, `"role":"OWNER", "expected_revision":`, 1), nil, 400},
		{"null value", strings.Replace(body, `"Алиса"`, "null", 1), nil, 400},
		{"null revision", strings.Replace(body, `:7`, ":null", 1), nil, 400},
		{"missing revision", strings.Replace(body, `,"expected_revision":7`, "", 1), nil, 400},
		{"numeric overflow", strings.Replace(body, `:7`, ":9007199254740992", 1), nil, 400},
		{"invalid Unicode", strings.Replace(body, "Алиса", `\ud800`, 1), nil, 400},
		{"too many code points", strings.Replace(body, "Алиса", strings.Repeat("я", 121), 1), nil, 400},
		{"extra JSON", body + `{}`, nil, 400},
		{"missing request ID", body, func(r *http.Request) { r.Header.Del("X-Request-ID") }, 400},
		{"request ID mismatch", body, func(r *http.Request) { r.Header.Set("X-Request-ID", result.UserID) }, 400},
		{"missing CSRF", body, func(r *http.Request) { r.Header.Del("X-CSRF-Token") }, 400},
		{"ambiguous CSRF", body, func(r *http.Request) { r.Header.Add("X-CSRF-Token", csrf) }, 400},
		{"missing session", body, func(r *http.Request) { r.Header.Del("X-HitPipe-Session") }, 401},
		{"no transport", body, func(r *http.Request) { r.TLS = nil }, 401},
		{"unbound transport", body, func(r *http.Request) { r.TLS.PeerCertificates[0].Raw = []byte("another test leaf") }, 403},
		{"browser authority", body, func(r *http.Request) { r.Header.Set("X-HitPipe-Role", "OWNER") }, 403},
		{"wrong conditional", body, func(r *http.Request) { r.Header.Set("If-Match", `"8"`) }, 412},
		{"weak conditional", body, func(r *http.Request) { r.Header.Set("If-Match", `W/"7"`) }, 412},
		{"wrong encoding", body, func(r *http.Request) { r.Header.Set("Content-Encoding", "gzip") }, 400},
		{"stream too long", strings.Repeat("x", privateiam.MaxRequestBytes+1), func(r *http.Request) { r.ContentLength = -1 }, 413},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, peer := patchRequest(tc.input)
			if tc.change != nil {
				tc.change(r)
			}
			w := &testWriter{ResponseRecorder: httptest.NewRecorder()}
			backend := updateFunc(func(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, getprofile.Profile) error) error {
				t.Fatal("invalid input reached backend")
				return app.ErrUnavailable
			})
			newHandler(t, backend, peer).ServeHTTP(w, r)
			var failure errorResponse
			if w.Code != tc.status || json.Unmarshal(w.Body.Bytes(), &failure) != nil || !runtimeaccess.ValidUUID(failure.RequestID) ||
				failure.RequestID != w.Header().Get("X-Request-ID") || failure.Code == "" || w.Flushed ||
				strings.Contains(w.Body.String(), csrf) || strings.Contains(w.Body.String(), session) {
				t.Fatalf("invalid input got unexpected status or error envelope: %d", w.Code)
			}
		})
	}
}

func TestUnconfirmedProfileDoesNotReportSuccess(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want int
	}{
		{nil, 503}, {app.ErrUncertain, 503}, {app.ErrUnavailable, 503}, {app.ErrNotFound, 404},
		{app.ErrUnauthenticated, 401}, {app.ErrDenied, 403}, {app.ErrConflict, 409}, {app.ErrStale, 412},
	} {
		r, peer := patchRequest(body)
		w := &testWriter{ResponseRecorder: httptest.NewRecorder()}
		calls := 0
		backend := updateFunc(func(context.Context, runtimeaccess.Peer, string, string, app.Command, func(context.Context, getprofile.Profile) error) error {
			calls++
			return tc.err
		})
		newHandler(t, backend, peer).ServeHTTP(w, r)
		if w.Code != tc.want || w.Flushed || calls != 1 || strings.Contains(w.Body.String(), result.DisplayName) {
			t.Fatalf("unexpected wire response for %v: %d", tc.err, w.Code)
		}
	}
}

func TestExpiredPeerAndBrokenFlush(t *testing.T) {
	r, peer := patchRequest(body)
	backend := updateFunc(func(ctx context.Context, _ runtimeaccess.Peer, _, _ string, _ app.Command, start func(context.Context, getprofile.Profile) error) error {
		r.TLS.PeerCertificates[0].NotAfter = time.Now().Add(-time.Minute)
		return start(ctx, result)
	})
	w := &testWriter{ResponseRecorder: httptest.NewRecorder()}
	newHandler(t, backend, peer).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || w.Flushed || strings.Contains(w.Body.String(), result.DisplayName) {
		t.Fatal("expired session transport started a protected response")
	}

	r, peer = patchRequest(body)
	backend = updateFunc(func(ctx context.Context, _ runtimeaccess.Peer, _, _ string, _ app.Command, start func(context.Context, getprofile.Profile) error) error {
		return start(ctx, result)
	})
	w = &testWriter{ResponseRecorder: httptest.NewRecorder(), flush: func() error { return errors.New("test flush failed") }}
	defer func() {
		if got := recover(); got != http.ErrAbortHandler || strings.Contains(w.Body.String(), "UNAVAILABLE") {
			t.Fatal("interrupted response was retried or given a second body")
		}
	}()
	newHandler(t, backend, peer).ServeHTTP(w, r)
}
