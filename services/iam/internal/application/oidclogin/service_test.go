package oidclogin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

type testClock struct {
	seconds atomic.Int64
	trusted atomic.Bool
}

func (c *testClock) Now(context.Context) (sessionpolicy.Observation, error) {
	return sessionpolicy.Observation{At: time.Unix(c.seconds.Load(), 0), Trusted: c.trusted.Load()}, nil
}

type testClient struct {
	exchanges atomic.Int64
	exchange  func(code, verifier, nonce string) (issuer, subject string, verified bool, err error)
}

func (c *testClient) AuthorizationURL(state, nonce, challenge string) string {
	params := url.Values{"state": {state}, "nonce": {nonce}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://idp.test/authorize?" + params.Encode()
}

func (c *testClient) Exchange(_ context.Context, code, verifier, nonce string) (string, string, bool, error) {
	c.exchanges.Add(1)
	if c.exchange != nil {
		return c.exchange(code, verifier, nonce)
	}
	return "https://idp.test/realm", "subject-105", true, nil
}

func fixture(t *testing.T) (*Service, *testClient, *testClock) {
	t.Helper()
	client, clock := &testClient{}, &testClock{}
	clock.seconds.Store(1_800_000_000)
	clock.trusted.Store(true)
	service, err := New(client, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service, client, clock
}

func begin(t *testing.T, service *Service) Attempt {
	t.Helper()
	attempt, err := service.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return attempt
}

func TestOIDCChallengeAndRequestBoundProof(t *testing.T) {
	s, client, clock := fixture(t)
	ctx := context.Background()
	attempt := begin(t, s)
	location, err := url.Parse(attempt.Location)
	if err != nil {
		t.Fatal("invalid authorization URL")
	}
	query := location.Query()
	if query.Get("state") != attempt.State || query.Get("code_challenge_method") != "S256" ||
		attempt.Binding == attempt.State || strings.Contains(attempt.Location, attempt.Binding) ||
		!attempt.ExpiresAt.Equal(time.Unix(clock.seconds.Load(), 0).Add(10*time.Minute)) {
		t.Fatal("wrong redirect, binding, or expiry")
	}
	client.exchange = func(code, verifier, nonce string) (string, string, bool, error) {
		digest := sha256.Sum256([]byte(verifier))
		if code != "fixture-code" || nonce != query.Get("nonce") || !validToken(nonce) || !validToken(verifier) ||
			verifier == nonce || verifier == attempt.State || nonce == attempt.State || nonce == attempt.Binding ||
			verifier == attempt.Binding || base64.RawURLEncoding.EncodeToString(digest[:]) != query.Get("code_challenge") {
			t.Fatal("code, independent nonce, or verifier did not match the stored S256 challenge")
		}
		return "https://idp.test/realm", "subject-105", true, nil
	}
	verifiedCtx, proof, err := s.VerifyCallback(ctx, "fixture-code", attempt.State, attempt.Binding)
	if err != nil || proof == "" || proof == attempt.State {
		t.Fatal("verified callback could not issue an internal proof")
	}
	if _, err := s.Consume(ctx, proof); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("proof escaped its original request context")
	}
	if _, err := s.Consume(verifiedCtx, attempt.State); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("browser state was accepted as a proof")
	}
	other, _, _ := fixture(t)
	if _, err := other.Consume(verifiedCtx, proof); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("another service consumed the proof")
	}
	login, err := s.Consume(verifiedCtx, proof)
	if err != nil || !login.EmailVerified || login.Subject != "subject-105" ||
		login.Issuer != "https://idp.test/realm" || !login.AttemptExpiresAt.Equal(attempt.ExpiresAt) {
		t.Fatal("verified identity projection was invalid")
	}
	if _, err := s.Consume(verifiedCtx, proof); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("internal proof was reusable")
	}
}

func TestForeignCancellationExpiryAndRestart(t *testing.T) {
	s, client, clock := fixture(t)
	ctx := context.Background()
	owned, foreign := begin(t, s), begin(t, s)
	if owned.State == foreign.State || owned.Binding == foreign.Binding {
		t.Fatal("separate attempts shared a browser binding or state")
	}
	for _, binding := range []string{"", foreign.Binding} {
		if _, _, err := s.VerifyCallback(ctx, "", owned.State, binding); !errors.Is(err, ErrInvalidAttempt) {
			t.Fatal("foreign browser accepted")
		}
	}
	if _, _, err := s.VerifyCallback(ctx, "valid", owned.State, owned.Binding); err != nil {
		t.Fatal("a foreign attempt consumed the rightful browser's request")
	}
	if _, _, err := s.VerifyCallback(ctx, "", foreign.State, foreign.Binding); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("canceled callback was accepted")
	}
	if _, _, err := s.VerifyCallback(ctx, "valid", foreign.State, foreign.Binding); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("canceled attempt could be replayed")
	}
	expired := begin(t, s)
	clock.seconds.Add(600)
	if _, _, err := s.VerifyCallback(ctx, "valid", expired.State, expired.Binding); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("attempt reached its exact expiry and was accepted")
	}
	lost := begin(t, s)
	restarted, err := New(client, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := restarted.VerifyCallback(ctx, "valid", lost.State, lost.Binding); !errors.Is(err, ErrInvalidAttempt) {
		t.Fatal("a pre-restart attempt survived")
	}
	if got := client.exchanges.Load(); got != 1 {
		t.Fatalf("OIDC exchanges = %d, want one", got)
	}
}

func TestConcurrentCallbackAndIssuerProofConsumption(t *testing.T) {
	s, client, _ := fixture(t)
	attempt := begin(t, s)
	var accepted atomic.Int64
	var group sync.WaitGroup
	for range 16 {
		group.Go(func() {
			ctx, proof, err := s.VerifyCallback(context.Background(), "fixture-code", attempt.State, attempt.Binding)
			if err != nil {
				if !errors.Is(err, ErrInvalidAttempt) {
					t.Error("callback received an unrelated error")
				}
				return
			}
			var consumers sync.WaitGroup
			for range 8 {
				consumers.Go(func() {
					if _, err := s.Consume(ctx, proof); err == nil {
						accepted.Add(1)
					}
				})
			}
			consumers.Wait()
		})
	}
	group.Wait()
	if client.exchanges.Load() != 1 || accepted.Load() != 1 {
		t.Fatal("concurrent callback or issuer consumption was not exactly once")
	}
}

func TestProviderErrorsAndExpiredExchangeNeverYieldProof(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		verified bool
		advance  int64
		provider error
		expected error
	}{
		{"code exchange failed", false, 0, ErrProviderUnavailable, ErrProviderUnavailable},
		{"ID token rejected", false, 0, errors.New("private diagnostics"), ErrInvalidIdentity},
		{"unverified email", false, 0, nil, ErrInvalidIdentity},
		{"exchange crosses deadline", true, 600, nil, ErrInvalidAttempt},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s, client, clock := fixture(t)
			attempt := begin(t, s)
			client.exchange = func(_, _, _ string) (string, string, bool, error) {
				clock.seconds.Add(scenario.advance)
				return "https://idp.test/realm", "subject-105", scenario.verified, scenario.provider
			}
			ctx := context.Background()
			if _, proof, err := s.VerifyCallback(ctx, "fixture-code", attempt.State, attempt.Binding); proof != "" || !errors.Is(err, scenario.expected) {
				t.Fatal("failed exchange produced an incorrect outcome")
			}
			if _, _, err := s.VerifyCallback(ctx, "fixture-code", attempt.State, attempt.Binding); !errors.Is(err, ErrInvalidAttempt) {
				t.Fatal("failed exchange left a replayable attempt")
			}
		})
	}
}

func TestCapacityReclaimsExpiredAttemptsAndUntrustedTimeDenies(t *testing.T) {
	s, _, clock := fixture(t)
	for range maxPendingAttempts {
		begin(t, s)
	}
	ctx := context.Background()
	if _, err := s.Begin(ctx); !errors.Is(err, ErrAttemptCapacity) {
		t.Fatal("attempt capacity was not enforced")
	}
	clock.seconds.Add(600)
	begin(t, s)
	clock.trusted.Store(false)
	if _, err := s.Begin(ctx); !errors.Is(err, issuesession.ErrClockUnavailable) {
		t.Fatal("untrusted clock permitted a login attempt")
	}
}
