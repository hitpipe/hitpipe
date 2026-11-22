// Package oidclogin owns bounded, single-use OIDC attempts for one IAM instance.
package oidclogin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
)

const (
	maxPendingAttempts = 128
	tokenSize          = 32
	encodedTokenSize   = 43
)

var (
	ErrInvalidAttempt      = errors.New("oidc_attempt_invalid")
	ErrInvalidIdentity     = errors.New("oidc_identity_denied")
	ErrProviderUnavailable = errors.New("oidc_provider_unavailable")
	ErrAttemptCapacity     = errors.New("oidc_attempt_capacity")
)

// Client exchanges a code only with the configured issuer, then verifies its ID token.
type Client interface {
	AuthorizationURL(state, nonce, challenge string) string
	Exchange(ctx context.Context, code, verifier, nonce string) (issuer, subject string, emailVerified bool, err error)
}

// Attempt is the browser redirect and the value of its private binding cookie.
// None of its fields belong in ordinary diagnostics.
type Attempt struct {
	Location, State, Binding string
	ExpiresAt                time.Time
}

type pendingAttempt struct {
	bindingHash          [sha256.Size]byte
	nonce, verifier      string
	createdAt, expiresAt time.Time
}

type Service struct {
	client Client
	clock  issuesession.Clock
	mu     sync.Mutex
	live   map[string]pendingAttempt
}

func New(client Client, clock issuesession.Clock) (*Service, error) {
	if client == nil || clock == nil {
		return nil, issuesession.ErrConfiguration
	}
	return &Service{client: client, clock: clock, live: make(map[string]pendingAttempt)}, nil
}

func randomToken() string {
	var token [tokenSize]byte
	// Go's crypto/rand.Read fills the buffer or terminates on entropy failure.
	_, _ = rand.Read(token[:])
	return base64.RawURLEncoding.EncodeToString(token[:])
}

func validToken(token string) bool {
	if len(token) != encodedTokenSize {
		return false
	}
	_, err := base64.RawURLEncoding.Strict().DecodeString(token)
	return err == nil
}

func (s *Service) now(ctx context.Context) (time.Time, error) {
	if ctx == nil || ctx.Err() != nil {
		return time.Time{}, issuesession.ErrCancelled
	}
	observation, err := s.clock.Now(ctx)
	if err != nil || !observation.Usable() {
		return time.Time{}, issuesession.ErrClockUnavailable
	}
	return observation.At.UTC(), ctx.Err()
}

func (s *Service) Begin(ctx context.Context) (Attempt, error) {
	now, err := s.now(ctx)
	if err != nil {
		return Attempt{}, err
	}
	state, binding, nonce, verifier := randomToken(), randomToken(), randomToken(), randomToken()
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	location := s.client.AuthorizationURL(state, nonce, challenge)
	if location == "" {
		return Attempt{}, ErrProviderUnavailable
	}
	expires := now.Add(issuesession.MaxLoginAttemptLifetime)

	s.mu.Lock()
	defer s.mu.Unlock()
	for key, old := range s.live {
		if !now.Before(old.expiresAt) {
			delete(s.live, key)
		}
	}
	if len(s.live) >= maxPendingAttempts {
		return Attempt{}, ErrAttemptCapacity
	}
	if _, exists := s.live[state]; exists {
		return Attempt{}, ErrAttemptCapacity
	}
	s.live[state] = pendingAttempt{sha256.Sum256([]byte(binding)), nonce, verifier, now, expires}
	return Attempt{Location: location, State: state, Binding: binding, ExpiresAt: expires}, nil
}

func (s *Service) take(state, binding string, now time.Time) (pendingAttempt, bool) {
	digest := sha256.Sum256([]byte(binding))
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt, exists := s.live[state]
	if !exists || subtle.ConstantTimeCompare(attempt.bindingHash[:], digest[:]) != 1 {
		return pendingAttempt{}, false
	}
	delete(s.live, state)
	return attempt, !now.Before(attempt.createdAt) && now.Before(attempt.expiresAt)
}

type proofKey struct{}

type callbackProof struct {
	owner *Service
	token string
	login issuesession.VerifiedLogin
	used  atomic.Bool
}

// VerifyCallback consumes a browser-bound attempt before code exchange. An
// empty code cancels the bound attempt. A foreign browser cannot cancel it.
// The returned proof is usable only inside the returned request context.
func (s *Service) VerifyCallback(ctx context.Context, code, state, binding string) (context.Context, string, error) {
	if !validToken(state) || !validToken(binding) {
		return ctx, "", ErrInvalidAttempt
	}
	now, err := s.now(ctx)
	if err != nil {
		return ctx, "", err
	}
	attempt, ok := s.take(state, binding, now)
	if !ok || !validCode(code) {
		return ctx, "", ErrInvalidAttempt
	}
	issuer, subject, verified, err := s.client.Exchange(ctx, code, attempt.verifier, attempt.nonce)
	if errors.Is(err, ErrProviderUnavailable) || ctx.Err() != nil {
		return ctx, "", ErrProviderUnavailable
	}
	if err != nil || issuer == "" || subject == "" || !verified {
		return ctx, "", ErrInvalidIdentity
	}
	finished, err := s.now(ctx)
	if err != nil {
		return ctx, "", err
	}
	if finished.Before(now) || !finished.Before(attempt.expiresAt) {
		return ctx, "", ErrInvalidAttempt
	}
	proof := &callbackProof{
		owner: s,
		token: randomToken(),
		login: issuesession.VerifiedLogin{
			Issuer: issuer, Subject: subject, EmailVerified: verified,
			AttemptCreatedAt: attempt.createdAt, AttemptExpiresAt: attempt.expiresAt,
		},
	}
	return context.WithValue(ctx, proofKey{}, proof), proof.token, nil
}

func validCode(code string) bool {
	if len(code) == 0 || len(code) > 255 {
		return false
	}
	for _, c := range []byte(code) {
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

func (s *Service) Consume(ctx context.Context, token string) (issuesession.VerifiedLogin, error) {
	if ctx == nil || ctx.Err() != nil {
		return issuesession.VerifiedLogin{}, ErrInvalidAttempt
	}
	proof, ok := ctx.Value(proofKey{}).(*callbackProof)
	if !ok || proof.owner != s || subtle.ConstantTimeCompare([]byte(token), []byte(proof.token)) != 1 || !proof.used.CompareAndSwap(false, true) {
		return issuesession.VerifiedLogin{}, ErrInvalidAttempt
	}
	return proof.login, nil
}

var _ issuesession.Verifier = (*Service)(nil)
