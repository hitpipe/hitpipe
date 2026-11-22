package httpauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/oidclogin"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

type fixtureClock struct{ instant time.Time }

func (c fixtureClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: c.instant, Trusted: true}, nil
}

type fixtureOIDC struct {
	exchanges int
	verified  bool
	err       error
}

func (*fixtureOIDC) AuthorizationURL(state, nonce, challenge string) string {
	query := url.Values{"state": {state}, "nonce": {nonce}, "code_challenge": {challenge}}
	return "https://idp.test/authorize?" + query.Encode()
}

func (c *fixtureOIDC) Exchange(context.Context, string, string, string) (string, string, bool, error) {
	c.exchanges++
	return "https://idp.test/realm", "subject-106", c.verified, c.err
}

type fixtureGuard struct{ deny bool }

func (g *fixtureGuard) Check(context.Context, issuesession.Identity) (issuesession.GuardDecision, error) {
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 1, RotationRequired: g.deny}, nil
}

type fixtureStore struct {
	records []issuesession.Record
	err     error
}

func (s *fixtureStore) Create(_ context.Context, record issuesession.Record) (issuesession.Persisted, error) {
	s.records = append(s.records, record)
	if s.err != nil {
		return issuesession.Persisted{}, s.err
	}
	return issuesession.Persisted{PrincipalID: record.PrincipalID, SessionID: record.SessionID}, nil
}

type fixture struct {
	handler *Handler
	client  *fixtureOIDC
	guard   *fixtureGuard
	store   *fixtureStore
	now     time.Time
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{
		client: &fixtureOIDC{verified: true}, guard: &fixtureGuard{}, store: &fixtureStore{},
		now: time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC),
	}
	clock := fixtureClock{f.now}
	flow, err := oidclogin.New(f.client, clock)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := issuesession.New(flow, clock, f.guard, f.store, issuesession.Policy{AbsoluteLifetime: 2 * time.Hour, IdleLifetime: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	f.handler, err = New(flow, issuer, "/signed-in")
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func serve(handler http.Handler, method, target string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func begin(t *testing.T, f fixture) (string, *http.Cookie) {
	t.Helper()
	response := serve(f.handler, http.MethodGet, "/auth/login")
	if response.Code != http.StatusFound || response.Body.Len() != 0 {
		t.Fatalf("login status = %d, expected empty 302", response.Code)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil || location.Host != "idp.test" || !location.IsAbs() {
		t.Fatal("login did not redirect to the configured provider")
	}
	state := location.Query().Get("state")
	cookies := response.Result().Cookies()
	if state == "" || len(cookies) != 1 {
		t.Fatal("login missing a state or browser binding")
	}
	c := cookies[0]
	if c.Name != bindingCookieName(state) || c.Value == "" || c.Path != "/" || c.Domain != "" ||
		!c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 600 || !c.Expires.Equal(f.now.Add(10*time.Minute)) {
		t.Fatal("login binding cookie has unsafe metadata")
	}
	return state, c
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || response.Header().Get("Content-Type") != "application/json" ||
		response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" ||
		response.Header().Get("Location") != "" {
		t.Fatalf("status = %d, want %d without redirect", response.Code, status)
	}
	var result errorBody
	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || result.Code != code || result.Message == "" || !uuid.MatchString(result.RequestID) {
		t.Fatal("response does not fit the error envelope")
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName || cookie.MaxAge >= 0 || cookie.Value != "" {
			t.Fatal("failed sign-in returned a live cookie")
		}
	}
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestLoginAndCallbackUseRealIssuerContract(t *testing.T) {
	f := newFixture(t)
	state, binding := begin(t, f)
	code := "fixture-authorization-code"
	query := url.Values{"state": {state}, "code": {code}, "iss": {"https://idp.test/realm"}, "session_state": {"idp-session"}}
	response := serve(f.handler, http.MethodGet, "/auth/callback?"+query.Encode(), binding)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/signed-in" || response.Body.Len() != 0 ||
		response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("callback status = %d, want empty 303 to the configured path", response.Code)
	}
	if f.client.exchanges != 1 || len(f.store.records) != 1 {
		t.Fatal("callback did not issue exactly one session")
	}
	record := f.store.records[0]
	if record.Issuer != "https://idp.test/realm" || record.Subject != "subject-106" ||
		record.SessionHash == record.CSRFHash || record.CSRFHash == [32]byte{} {
		t.Fatal("issuer did not persist the checked identity and separate digests")
	}
	var session *http.Cookie
	deletedBinding := false
	for _, cookie := range response.Result().Cookies() {
		switch cookie.Name {
		case sessionCookieName:
			session = cookie
		case binding.Name:
			deletedBinding = cookie.MaxAge < 0 && cookie.Value == ""
		default:
			t.Fatal("callback returned an unexpected cookie")
		}
	}
	if !deletedBinding || session == nil || session.Value == "" || session.Value == code || session.Domain != "" || session.Path != "/" ||
		!session.Secure || !session.HttpOnly || session.SameSite != http.SameSiteLaxMode || !session.Expires.Equal(record.ExpiresAt) ||
		sha256.Sum256([]byte(session.Value)) != record.SessionHash {
		t.Fatal("session or binding cookie does not match issuer authority")
	}
	assertError(t, serve(f.handler, http.MethodGet, "/auth/callback?"+query.Encode(), binding), http.StatusBadRequest, "invalid_login_attempt")
	if len(f.store.records) != 1 || f.client.exchanges != 1 {
		t.Fatal("callback replay reached the issuer or code exchange")
	}
}

func TestBadQueryAndForeignBindingCannotConsumeOwnedAttempt(t *testing.T) {
	f := newFixture(t)
	state, binding := begin(t, f)
	_, foreignBinding := begin(t, f)
	query := "/auth/callback?state=" + state + "&code=fixture-code"
	bad := []struct {
		target  string
		cookies []*http.Cookie
	}{
		{query + "&state=" + state, []*http.Cookie{binding}},
		{query + "&code=second", []*http.Cookie{binding}},
		{query + "&redirect=https://elsewhere.test", []*http.Cookie{binding}},
		{query + "&iss=%zz", []*http.Cookie{binding}},
		{query, nil},
		{query, []*http.Cookie{binding, binding}},
		{query, []*http.Cookie{{Name: binding.Name, Value: foreignBinding.Value}}},
	}
	for _, tc := range bad {
		assertError(t, serve(f.handler, http.MethodGet, tc.target, tc.cookies...), http.StatusBadRequest, "invalid_login_attempt")
	}
	assertError(t, serve(f.handler, http.MethodGet, query+strings.Repeat("x", maxCallbackQuery), binding), http.StatusRequestEntityTooLarge, "request_too_large")
	if f.client.exchanges != 0 || len(f.store.records) != 0 {
		t.Fatal("invalid callback reached code exchange or session issuance")
	}
	if got := serve(f.handler, http.MethodGet, query, binding).Code; got != http.StatusSeeOther {
		t.Fatalf("the owner's valid callback did not survive invalid attempts: %d", got)
	}
}

func TestCancellationAndExistingSessionDenyBeforeExchange(t *testing.T) {
	f := newFixture(t)
	session := &http.Cookie{Name: sessionCookieName, Value: "fixture-existing"}
	assertError(t, serve(f.handler, http.MethodGet, "/auth/login", session), http.StatusConflict, "session_rotation_unavailable")
	state, binding := begin(t, f)
	query := "/auth/callback?state=" + state + "&code=fixture-code"
	assertError(t, serve(f.handler, http.MethodGet, query, binding, session), http.StatusConflict, "session_rotation_unavailable")
	assertError(t, serve(f.handler, http.MethodGet, query, binding), http.StatusBadRequest, "invalid_login_attempt")
	state, binding = begin(t, f)
	cancel := "/auth/callback?state=" + state + "&error=access_denied&error_description=private-fixture-detail"
	assertError(t, serve(f.handler, http.MethodGet, cancel, binding), http.StatusBadRequest, "invalid_login_attempt")
	assertError(t, serve(f.handler, http.MethodGet, "/auth/callback?state="+state+"&code=fixture-code", binding), http.StatusBadRequest, "invalid_login_attempt")
	if f.client.exchanges != 0 || len(f.store.records) != 0 {
		t.Fatal("canceled sign-in or session replacement reached code exchange or issuance")
	}
}

func TestFailedProviderAndIssuerNeverEmitSession(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(fixture)
		status int
		code   string
	}{
		{"unverified email", func(f fixture) { f.client.verified = false }, http.StatusUnauthorized, "sign_in_denied"},
		{"invalid token", func(f fixture) { f.client.err = errors.New("private-fixture-detail") }, http.StatusUnauthorized, "sign_in_denied"},
		{"unavailable IdP", func(f fixture) { f.client.err = oidclogin.ErrProviderUnavailable }, http.StatusServiceUnavailable, "sign_in_unavailable"},
		{"rotation denied", func(f fixture) { f.guard.deny = true }, http.StatusForbidden, "session_denied"},
		{"unknown commit", func(f fixture) { f.store.err = errors.New("private-fixture-detail") }, http.StatusServiceUnavailable, "sign_in_unavailable"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newFixture(t)
			scenario.mutate(f)
			state, binding := begin(t, f)
			query := "/auth/callback?state=" + state + "&code=private-fixture-code"
			response := serve(f.handler, http.MethodGet, query, binding)
			body := response.Body.String()
			if strings.Contains(body, "private-fixture-") || strings.Contains(body, binding.Value) || strings.Contains(body, state) {
				t.Fatal("callback echoed secret input or private diagnostics")
			}
			assertError(t, response, scenario.status, scenario.code)
			assertError(t, serve(f.handler, http.MethodGet, query, binding), http.StatusBadRequest, "invalid_login_attempt")
			if f.client.exchanges != 1 || len(f.store.records) > 1 {
				t.Fatal("failed issuance retried")
			}
		})
	}
}

func TestOnlyAcceptedRoutesAndSafeFixedRedirect(t *testing.T) {
	f := newFixture(t)
	assertError(t, serve(f.handler, http.MethodGet, "/auth/login?return_to=https://elsewhere.test"), http.StatusBadRequest, "invalid_request")
	assertError(t, serve(f.handler, http.MethodPost, "/auth/login"), http.StatusMethodNotAllowed, "method_not_allowed")
	assertError(t, serve(f.handler, http.MethodHead, "/auth/callback"), http.StatusMethodNotAllowed, "method_not_allowed")
	assertError(t, serve(f.handler, http.MethodGet, "/auth/session"), http.StatusNotFound, "not_found")
	assertError(t, serve(f.handler, http.MethodGet, "/auth/%6cogin"), http.StatusNotFound, "not_found")
	for _, target := range []string{"", "https://elsewhere.test", "//elsewhere.test", "/%2felsewhere.test", "/\\elsewhere.test", "/path?next=elsewhere", "/path#target", "/\npath"} {
		if _, err := New(f.handler.flow, f.handler.issuer, target); !errors.Is(err, issuesession.ErrConfiguration) {
			t.Fatal("unsafe redirect path was accepted")
		}
	}
}
