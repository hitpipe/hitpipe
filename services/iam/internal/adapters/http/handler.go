// Package httpauth exposes the two public browser OIDC endpoints owned by IAM.
package httpauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/oidclogin"
)

const (
	sessionCookieName   = "__Host-hp_session"
	bindingCookiePrefix = "__Host-hp_oidc_"
	maxCallbackQuery    = 4096
)

type Issuer interface {
	Execute(context.Context, string) (issuesession.Result, error)
}

type Handler struct {
	flow        *oidclogin.Service
	issuer      Issuer
	successPath string
}

// New takes a configured same-origin path. No redirect target is read from the browser.
func New(flow *oidclogin.Service, issuer Issuer, successPath string) (*Handler, error) {
	if flow == nil || issuer == nil || !validSuccessPath(successPath) {
		return nil, issuesession.ErrConfiguration
	}
	return &Handler{flow: flow, issuer: issuer, successPath: successPath}, nil
}

func validSuccessPath(path string) bool {
	parsed, err := url.Parse(path)
	return err == nil && parsed.Path == path && strings.HasPrefix(path, "/") &&
		!strings.HasPrefix(path, "//") && !strings.ContainsAny(path, "\\\t\r\n")
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	switch r.URL.EscapedPath() {
	case "/auth/login", "/auth/callback":
	default:
		writeError(w, http.StatusNotFound, "not_found", "No such route.")
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Only GET is supported.")
		return
	}
	if r.ContentLength != 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "The request must not have a body.")
		return
	}
	if r.URL.Path == "/auth/login" {
		h.begin(w, r)
	} else {
		h.callback(w, r)
	}
}

func hasSession(r *http.Request) bool {
	return len(r.CookiesNamed(sessionCookieName)) != 0
}

func bindingCookieName(state string) string {
	digest := sha256.Sum256([]byte(state))
	return bindingCookiePrefix + base64.RawURLEncoding.EncodeToString(digest[:])
}

func bindingCookie(name, value string, expiry time.Time, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: name, Value: value, Path: "/", Secure: true, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: maxAge,
	}
}

func (h *Handler) begin(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, http.StatusBadRequest, "invalid_request", "The login request does not take query parameters.")
		return
	}
	if hasSession(r) {
		sessionConflict(w)
		return
	}
	attempt, err := h.flow.Begin(r.Context())
	if errors.Is(err, oidclogin.ErrAttemptCapacity) {
		writeError(w, http.StatusTooManyRequests, "too_many_login_attempts", "Try signing in later.")
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	maxAge := int(issuesession.MaxLoginAttemptLifetime / time.Second)
	http.SetCookie(w, bindingCookie(bindingCookieName(attempt.State), attempt.Binding, attempt.ExpiresAt, maxAge))
	redirect(w, http.StatusFound, attempt.Location)
}

func callbackParams(raw string) (url.Values, bool) {
	params, err := url.ParseQuery(raw)
	if err != nil {
		return nil, false
	}
	for name, values := range params {
		if len(values) != 1 {
			return nil, false
		}
		switch name {
		case "code", "state", "iss", "session_state", "error", "error_description", "error_uri":
			// Keycloak supplies standard authorization-response metadata. The
			// issuer's authenticated ID token, not query metadata, establishes identity.
		default:
			return nil, false
		}
	}
	return params, true
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.RawQuery
	if len(raw) > maxCallbackQuery {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "The callback query is too large.")
		return
	}
	params, ok := callbackParams(raw)
	if !ok {
		invalidAttempt(w)
		return
	}
	state, code := params.Get("state"), params.Get("code")
	if state == "" || len(state) > 255 || len(code) > 255 {
		invalidAttempt(w)
		return
	}
	name := bindingCookieName(state)
	bindings := r.CookiesNamed(name)
	if len(bindings) != 1 {
		invalidAttempt(w)
		return
	}
	binding := bindings[0].Value
	http.SetCookie(w, bindingCookie(name, "", time.Unix(0, 0).UTC(), -1))
	if hasSession(r) {
		// Consume an owned attempt without exchanging its code. Rotation and
		// revocation are separate authority operations, so leave the session alone.
		_, _, _ = h.flow.VerifyCallback(r.Context(), "", state, binding)
		sessionConflict(w)
		return
	}
	if params.Has("error") {
		code = ""
	}
	ctx, proof, err := h.flow.VerifyCallback(r.Context(), code, state, binding)
	switch {
	case errors.Is(err, oidclogin.ErrInvalidAttempt):
		invalidAttempt(w)
		return
	case errors.Is(err, oidclogin.ErrInvalidIdentity):
		credentialDenied(w)
		return
	case err != nil:
		unavailable(w)
		return
	}
	result, err := h.issuer.Execute(ctx, proof)
	switch {
	case errors.Is(err, issuesession.ErrConfirmationDenied):
		credentialDenied(w)
		return
	case errors.Is(err, issuesession.ErrIssuanceDenied):
		writeError(w, http.StatusForbidden, "session_denied", "Sign-in is unavailable for this session.")
		return
	case err != nil:
		unavailable(w)
		return
	}
	cookie := issuedCookie(result)
	if cookie == nil {
		unavailable(w)
		return
	}
	http.SetCookie(w, cookie)
	redirect(w, http.StatusSeeOther, h.successPath)
}

func issuedCookie(result issuesession.Result) *http.Cookie {
	metadata := result.Cookie
	if !result.IsIssued() || !result.ExpiresAt.After(result.CreatedAt) || metadata.Name != sessionCookieName ||
		metadata.Path != "/" || metadata.Domain != "" || !metadata.Secure || !metadata.HTTPOnly || metadata.SameSite != "Lax" {
		return nil
	}
	cookie := &http.Cookie{
		Name: metadata.Name, Value: result.Session.Reveal(), Path: metadata.Path,
		Secure: metadata.Secure, HttpOnly: metadata.HTTPOnly, SameSite: http.SameSiteLaxMode,
		Expires: result.ExpiresAt.UTC(),
	}
	if cookie.Valid() != nil {
		return nil
	}
	return cookie
}

func redirect(w http.ResponseWriter, status int, location string) {
	w.Header().Set("Location", location)
	w.WriteHeader(status)
}

func invalidAttempt(w http.ResponseWriter) {
	writeError(w, http.StatusBadRequest, "invalid_login_attempt", "Start a new sign-in.")
}

func credentialDenied(w http.ResponseWriter) {
	writeError(w, http.StatusUnauthorized, "sign_in_denied", "The identity provider did not confirm this sign-in.")
}

func sessionConflict(w http.ResponseWriter) {
	writeError(w, http.StatusConflict, "session_rotation_unavailable", "An existing session cannot be replaced here.")
}

func unavailable(w http.ResponseWriter) {
	writeError(w, http.StatusServiceUnavailable, "sign_in_unavailable", "Sign-in is temporarily unavailable.")
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var id [16]byte
	_, _ = rand.Read(id[:])
	id[6], id[8] = id[6]&0x0f|0x40, id[8]&0x3f|0x80
	text := hex.EncodeToString(id[:])
	requestID := text[:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorBody{Code: code, Message: message, RequestID: requestID})
}

var _ http.Handler = (*Handler)(nil)
