// Package oidc connects IAM to the configured confidential OpenID Connect provider.
package oidc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/oidclogin"
)

var (
	ErrInvalidResponse = errors.New("invalid OIDC authorization response")
	ErrUnavailable     = oidclogin.ErrProviderUnavailable
	errInvalidSettings = errors.New("invalid OIDC provider settings")
)

const (
	providerTimeout = 10 * time.Second
	tokenBodyLimit  = 64 << 10
	keysBodyLimit   = 128 << 10
)

type Config struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	JWKSURI               string
	RedirectURI           string
	ClientID              string
	ClientSecret          string
	// RootCAs replaces the system trust store when supplied.
	RootCAs *x509.CertPool
	Now     func() time.Time
}

type Client struct {
	issuer, tokenEndpoint, jwksURI, redirectURI string
	clientID, clientSecret                      string
	authorizationEndpoint                       url.URL
	http                                        *http.Client
	now                                         func() time.Time
}

// New uses explicit provider endpoints and the same exact callback URI for both
// requests in the authorization code flow.
func New(config Config) (*Client, error) {
	authorization, err := httpsURL(config.AuthorizationEndpoint)
	if err != nil {
		return nil, err
	}
	for _, value := range []string{config.Issuer, config.TokenEndpoint, config.JWKSURI, config.RedirectURI} {
		if _, err := httpsURL(value); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(config.ClientID) == "" || strings.TrimSpace(config.ClientSecret) == "" {
		return nil, errInvalidSettings
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	var roots *x509.CertPool
	if config.RootCAs != nil {
		roots = config.RootCAs.Clone()
	}
	transport := &http.Transport{
		DialContext:            (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		IdleConnTimeout:        90 * time.Second,
		MaxIdleConns:           8,
		MaxConnsPerHost:        8,
		MaxResponseHeaderBytes: 32 << 10,
		ForceAttemptHTTP2:      true,
	}
	return &Client{
		issuer:                config.Issuer,
		authorizationEndpoint: *authorization,
		tokenEndpoint:         config.TokenEndpoint,
		jwksURI:               config.JWKSURI,
		redirectURI:           config.RedirectURI,
		clientID:              config.ClientID,
		clientSecret:          config.ClientSecret,
		now:                   now,
		http: &http.Client{
			Transport: transport,
			Timeout:   providerTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func httpsURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || !strings.HasPrefix(value, "https://") || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(value, "#") ||
		parsed.String() != value {
		return nil, errInvalidSettings
	}
	return parsed, nil
}

func (c *Client) AuthorizationURL(state, nonce, challenge string) string {
	auth := c.authorizationEndpoint
	auth.RawQuery = url.Values{
		"client_id":             {c.clientID},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"nonce":                 {nonce},
		"redirect_uri":          {c.redirectURI},
		"response_type":         {"code"},
		"scope":                 {"openid email"},
		"state":                 {state},
	}.Encode()
	return auth.String()
}

func (c *Client) Exchange(ctx context.Context, code, verifier, nonce string) (issuer, subject string, emailVerified bool, err error) {
	if len(code) == 0 || len(code) > 255 || !validVerifier(verifier) || nonce == "" {
		return "", "", false, ErrInvalidResponse
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {c.redirectURI},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", false, ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(url.QueryEscape(c.clientID), url.QueryEscape(c.clientSecret))
	response, status, err := c.requestObject(request, tokenBodyLimit, "application/json")
	if err != nil {
		return "", "", false, err
	}
	if status == http.StatusBadRequest {
		oauthError, _ := member[string](response, "error")
		if oauthError == "invalid_grant" {
			return "", "", false, ErrInvalidResponse
		}
		return "", "", false, ErrUnavailable
	}
	idToken, ok := member[string](response, "id_token")
	if !ok || response["error"] != nil {
		return "", "", false, ErrInvalidResponse
	}
	return c.verifyToken(ctx, idToken, nonce, response)
}

func validVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if !(char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' ||
			char >= '0' && char <= '9' || strings.ContainsRune("-._~", char)) {
			return false
		}
	}
	return true
}

func (c *Client) requestObject(request *http.Request, limit int64, accept string) (object, int, error) {
	request.Header.Set("Accept", accept)
	response, err := c.http.Do(request)
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	defer response.Body.Close()
	status := response.StatusCode
	if status != http.StatusOK && (request.Method != http.MethodPost || status != http.StatusBadRequest) {
		return nil, status, ErrUnavailable
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" && mediaType != accept {
		return nil, status, ErrInvalidResponse
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, status, ErrUnavailable
	}
	if int64(len(body)) > limit {
		return nil, status, ErrInvalidResponse
	}
	fields, ok := jsonObject(body)
	if !ok {
		return nil, status, ErrInvalidResponse
	}
	return fields, status, nil
}
