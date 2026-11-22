package issuesession

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

type verifierFunc func(context.Context, string) (VerifiedLogin, error)

func (f verifierFunc) Consume(c context.Context, p string) (VerifiedLogin, error) { return f(c, p) }

type clockFunc func(context.Context) (sessionpolicy.Observation, error)

func (f clockFunc) Now(c context.Context) (sessionpolicy.Observation, error) { return f(c) }

type guardFunc func(context.Context, Identity) (GuardDecision, error)

func (f guardFunc) Check(c context.Context, i Identity) (GuardDecision, error) { return f(c, i) }

type storeFunc func(context.Context, Record) (Persisted, error)

func (f storeFunc) Create(c context.Context, r Record) (Persisted, error) { return f(c, r) }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("SENSITIVE-entropy-error") }

// These TTLs are fixtures, not adopted numeric production policy.
var fixturePolicy = Policy{AbsoluteLifetime: 8 * time.Hour, IdleLifetime: 30 * time.Minute}
var fixtureTime = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func fixtureLogin() VerifiedLogin {
	return VerifiedLogin{Issuer: "https://identity.fixture.invalid/realm", Subject: "provider-subject-123", EmailVerified: true, AttemptCreatedAt: fixtureTime.Add(-time.Minute), AttemptExpiresAt: fixtureTime.Add(4 * time.Minute)}
}

type fixture struct {
	service                                          Service
	login                                            VerifiedLogin
	observation                                      sessionpolicy.Observation
	decision                                         GuardDecision
	verifierErr, clockErr, guardErr, storeErr        error
	consumeCalls, clockCalls, guardCalls, storeCalls int
	record                                           Record
	persist                                          func(Record) Persisted
	afterStore                                       func()
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{login: fixtureLogin(), observation: sessionpolicy.Observation{At: fixtureTime, Trusted: true}, decision: GuardDecision{State: IssuanceAllowed, Epoch: 7}}
	var err error
	f.service, err = New(verifierFunc(func(_ context.Context, p string) (VerifiedLogin, error) {
		f.consumeCalls++
		if p != "opaque-proof" {
			return VerifiedLogin{}, errors.New("SENSITIVE-invalid-proof")
		}
		return f.login, f.verifierErr
	}), clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
		f.clockCalls++
		return f.observation, f.clockErr
	}), guardFunc(func(_ context.Context, i Identity) (GuardDecision, error) {
		f.guardCalls++
		if i != (Identity{Issuer: f.login.Issuer, Subject: f.login.Subject}) {
			t.Error("identity changed")
		}
		return f.decision, f.guardErr
	}), storeFunc(func(_ context.Context, r Record) (Persisted, error) {
		f.storeCalls++
		f.record = r
		if f.afterStore != nil {
			f.afterStore()
		}
		if f.persist != nil {
			return f.persist(r), f.storeErr
		}
		return Persisted{PrincipalID: r.PrincipalID, SessionID: r.SessionID}, f.storeErr
	}), fixturePolicy)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func assertFailed(t *testing.T, result Result, err, want error) {
	t.Helper()
	if err != want || result != (Result{}) || result.IsIssued() || result.Session.Reveal() != "" || result.CSRF.Reveal() != "" {
		t.Fatalf("want zero result and bounded %v; got issued=%v err=%v", want, result.IsIssued(), err)
	}
}
func TestRequiresDependenciesAndExplicitPolicy(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		name string
		v    Verifier
		c    Clock
		g    IssuanceGuard
		s    Store
	}{
		{"missing verifier", nil, f.service.clock, f.service.guard, f.service.store}, {"typed nil verifier", verifierFunc(nil), f.service.clock, f.service.guard, f.service.store},
		{"missing clock", f.service.verifier, nil, f.service.guard, f.service.store}, {"typed nil clock", f.service.verifier, clockFunc(nil), f.service.guard, f.service.store},
		{"missing guard", f.service.verifier, f.service.clock, nil, f.service.store}, {"typed nil guard", f.service.verifier, f.service.clock, guardFunc(nil), f.service.store},
		{"missing store", f.service.verifier, f.service.clock, f.service.guard, nil}, {"typed nil store", f.service.verifier, f.service.clock, f.service.guard, storeFunc(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.v, tc.c, tc.g, tc.s, fixturePolicy); err != ErrConfiguration {
				t.Fatal(err)
			}
		})
	}
	for _, p := range []Policy{{}, {AbsoluteLifetime: time.Hour}, {AbsoluteLifetime: time.Hour, IdleLifetime: -time.Second}, {AbsoluteLifetime: time.Hour, IdleLifetime: 2 * time.Hour}, {AbsoluteLifetime: 24*time.Hour + time.Nanosecond, IdleLifetime: time.Hour}} {
		if _, err := New(f.service.verifier, f.service.clock, f.service.guard, f.service.store, p); err != ErrConfiguration {
			t.Fatalf("policy %v: %v", p, err)
		}
	}
	if f.consumeCalls+f.clockCalls+f.guardCalls+f.storeCalls != 0 {
		t.Fatal("constructor invoked dependency")
	}
	result, err := (Service{}).Execute(context.Background(), "opaque-proof")
	assertFailed(t, result, err, ErrConfiguration)
}
func TestInvalidInputDoesNotConsumeProof(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		c    context.Context
		p    string
		want error
	}{{"nil context", nil, "opaque-proof", ErrInvalidRequest}, {"empty", context.Background(), "", ErrInvalidRequest}, {"oversized", context.Background(), strings.Repeat("p", 4097), ErrInvalidRequest}, {"cancelled", cancelled, "opaque-proof", ErrCancelled}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			result, err := f.service.Execute(tc.c, tc.p)
			assertFailed(t, result, err, tc.want)
			if f.consumeCalls != 0 {
				t.Fatal("invalid input consumed")
			}
		})
	}
}
func TestRejectsUnverifiedExpiredOrUncertainInputsBeforeStore(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*fixture)
		want   error
	}{
		{"verifier error", func(f *fixture) { f.verifierErr = errors.New("SENSITIVE-provider-token") }, ErrConfirmationDenied},
		{"unverified email", func(f *fixture) { f.login.EmailVerified = false }, ErrConfirmationDenied},
		{"missing issuer", func(f *fixture) { f.login.Issuer = "" }, ErrConfirmationDenied},
		{"missing subject", func(f *fixture) { f.login.Subject = "" }, ErrConfirmationDenied},
		{"too long subject", func(f *fixture) { f.login.Subject = strings.Repeat("s", 256) }, ErrConfirmationDenied},
		{"whitespace subject", func(f *fixture) { f.login.Subject = " subject" }, ErrConfirmationDenied},
		{"control subject", func(f *fixture) { f.login.Subject = "sub\nject" }, ErrConfirmationDenied},
		{"invalid UTF8", func(f *fixture) { f.login.Subject = string([]byte{0xff}) }, ErrConfirmationDenied},
		{"zero attempt", func(f *fixture) { f.login.AttemptCreatedAt = time.Time{} }, ErrConfirmationDenied},
		{"future attempt", func(f *fixture) { f.login.AttemptCreatedAt = fixtureTime.Add(time.Second) }, ErrConfirmationDenied},
		{"expired equality", func(f *fixture) { f.login.AttemptExpiresAt = fixtureTime }, ErrConfirmationDenied},
		{"too long attempt", func(f *fixture) {
			f.login.AttemptExpiresAt = f.login.AttemptCreatedAt.Add(MaxLoginAttemptLifetime + time.Nanosecond)
		}, ErrConfirmationDenied},
		{"inverted attempt", func(f *fixture) { f.login.AttemptExpiresAt = f.login.AttemptCreatedAt }, ErrConfirmationDenied},
		{"clock error", func(f *fixture) { f.clockErr = errors.New("SENSITIVE-clock") }, ErrClockUnavailable},
		{"untrusted clock", func(f *fixture) { f.observation.Trusted = false }, ErrClockUnavailable},
		{"zero clock", func(f *fixture) { f.observation.At = time.Time{} }, ErrClockUnavailable},
		{"guard error", func(f *fixture) { f.guardErr = errors.New("SENSITIVE-guard") }, ErrGuardUnavailable},
		{"zero guard", func(f *fixture) { f.decision = GuardDecision{} }, ErrIssuanceDenied},
		{"unknown guard", func(f *fixture) { f.decision.State = "unknown" }, ErrIssuanceDenied},
		{"zero epoch", func(f *fixture) { f.decision.Epoch = 0 }, ErrIssuanceDenied},
		{"negative epoch", func(f *fixture) { f.decision.Epoch = -1 }, ErrIssuanceDenied},
		{"rotation hold", func(f *fixture) { f.decision.RotationRequired = true }, ErrIssuanceDenied},
		{"pending revoke", func(f *fixture) { f.decision.PendingRevoke = true }, ErrIssuanceDenied},
		{"entropy failure", func(f *fixture) { f.service.entropy = errorReader{} }, ErrEntropyUnavailable},
		{"session entropy short", func(f *fixture) { f.service.entropy = bytes.NewReader(make([]byte, 31)) }, ErrEntropyUnavailable},
		{"CSRF entropy short", func(f *fixture) { f.service.entropy = bytes.NewReader(make([]byte, 63)) }, ErrEntropyUnavailable},
		{"principal UUID entropy short", func(f *fixture) { f.service.entropy = bytes.NewReader(make([]byte, 79)) }, ErrEntropyUnavailable},
		{"session UUID entropy short", func(f *fixture) { f.service.entropy = bytes.NewReader(make([]byte, 95)) }, ErrEntropyUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.modify(f)
			r, e := f.service.Execute(context.Background(), "opaque-proof")
			assertFailed(t, r, e, tc.want)
			if f.storeCalls != 0 {
				t.Fatal("unsafe store write")
			}
		})
	}
}
func TestCreatesHashedSessionAfterCommit(t *testing.T) {
	f := newFixture(t)
	const existing = "b87dfe6f-6df2-4160-8dba-634de2aa3fe0"
	f.persist = func(r Record) Persisted { return Persisted{PrincipalID: existing, SessionID: r.SessionID} }
	result, err := f.service.Execute(context.Background(), "opaque-proof")
	if err != nil || !result.IsIssued() {
		t.Fatalf("issued=%v err=%v", result.IsIssued(), err)
	}
	if f.consumeCalls != 1 || f.storeCalls != 1 || f.guardCalls != 2 || f.clockCalls != 3 {
		t.Fatalf("counts consume=%d store=%d guard=%d clock=%d", f.consumeCalls, f.storeCalls, f.guardCalls, f.clockCalls)
	}
	if result.PrincipalID != existing || result.SessionID != f.record.SessionID || f.record.Epoch != 7 || f.record.Issuer != f.login.Issuer || f.record.Subject != f.login.Subject {
		t.Fatal("identity/session/epoch changed")
	}
	if !validUUID(f.record.PrincipalID) || !validUUID(result.SessionID) || f.record.PrincipalID == result.SessionID || result.SessionID[14] != '4' || !strings.ContainsRune("89ab", rune(result.SessionID[19])) {
		t.Fatal("invalid UUIDs")
	}
	for _, tc := range []struct {
		s Secret
		h [32]byte
	}{{result.Session, f.record.SessionHash}, {result.CSRF, f.record.CSRFHash}} {
		raw := tc.s.Reveal()
		decoded, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(raw) != 43 || len(decoded) != 32 || sha256.Sum256([]byte(raw)) != tc.h {
			t.Fatal("incorrect entropy/encoding/exact-byte hash")
		}
	}
	if result.Session.Reveal() == result.CSRF.Reveal() || f.record.SessionHash == f.record.CSRFHash {
		t.Fatal("not independent secrets")
	}
	if result.CreatedAt != fixtureTime || result.ExpiresAt != fixtureTime.Add(fixturePolicy.AbsoluteLifetime) || result.IdleExpiresAt != fixtureTime.Add(fixturePolicy.IdleLifetime) {
		t.Fatal("policy changed")
	}
	if result.Cookie != (CookieMetadata{Name: "__Host-hp_session", Path: "/", Secure: true, HTTPOnly: true, SameSite: "Lax"}) {
		t.Fatal("cookie contract changed")
	}
	if !sessionpolicy.Evaluate(sessionpolicy.Snapshot{Found: true, CreatedAt: result.CreatedAt, ExpiresAt: result.ExpiresAt, IdleExpiresAt: result.IdleExpiresAt}, f.observation).IsValid() {
		t.Fatal("existing policy rejects issued session")
	}
}
func TestFormattingAndJSONRedactSecrets(t *testing.T) {
	f := newFixture(t)
	r, e := f.service.Execute(context.Background(), "opaque-proof")
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	outputs := []string{string(encoded), fmt.Sprintf("%v", r), fmt.Sprintf("%+v", r), fmt.Sprintf("%#v", r), fmt.Sprintf("%s %q %x %X %#v", r.Session, r.CSRF, r.Session, r.CSRF, r.Session), fmt.Sprintf("%+v", f.record)}
	for _, output := range outputs {
		if strings.Contains(output, r.Session.Reveal()) || strings.Contains(output, r.CSRF.Reveal()) {
			t.Fatal("secret leaked through generic representation")
		}
	}
	if !strings.Contains(string(encoded), "REDACTED") {
		t.Fatal("missing JSON redaction")
	}
}
func TestPostCommitFailureWithholdsSecrets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(*fixture, context.CancelFunc)
		want   error
	}{
		{"unknown commit", func(f *fixture, _ context.CancelFunc) { f.storeErr = errors.New("SENSITIVE-commit-unknown") }, ErrStoreUnavailable},
		{"cancel after commit", func(_ *fixture, c context.CancelFunc) { c() }, ErrCancelled},
		{"clock error", func(f *fixture, _ context.CancelFunc) { f.clockErr = io.EOF }, ErrClockUnavailable},
		{"untrusted clock", func(f *fixture, _ context.CancelFunc) { f.observation.Trusted = false }, ErrClockUnavailable},
		{"clock backwards", func(f *fixture, _ context.CancelFunc) { f.observation.At = fixtureTime.Add(-time.Second) }, ErrIssuanceDenied},
		{"proof expired during commit", func(f *fixture, _ context.CancelFunc) { f.observation.At = f.login.AttemptExpiresAt }, ErrIssuanceDenied},
		{"session expired during commit", func(f *fixture, _ context.CancelFunc) { f.observation.At = fixtureTime.Add(fixturePolicy.IdleLifetime) }, ErrIssuanceDenied},
		{"guard error", func(f *fixture, _ context.CancelFunc) { f.guardErr = io.EOF }, ErrGuardUnavailable},
		{"guard unknown", func(f *fixture, _ context.CancelFunc) { f.decision.State = "unknown" }, ErrIssuanceDenied},
		{"epoch changed", func(f *fixture, _ context.CancelFunc) { f.decision.Epoch++ }, ErrIssuanceDenied},
		{"pending revoke", func(f *fixture, _ context.CancelFunc) { f.decision.PendingRevoke = true }, ErrIssuanceDenied},
		{"rotation hold", func(f *fixture, _ context.CancelFunc) { f.decision.RotationRequired = true }, ErrIssuanceDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.afterStore = func() { tc.modify(f, cancel) }
			r, e := f.service.Execute(ctx, "opaque-proof")
			assertFailed(t, r, e, tc.want)
			if f.storeCalls != 1 || f.record.SessionHash == ([32]byte{}) {
				t.Fatal("did not simulate committed write")
			}
		})
	}
}
func TestMalformedCommitReceiptWithholdsSecrets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response func(Record) Persisted
	}{
		{"zero", func(Record) Persisted { return Persisted{} }},
		{"non UUID", func(r Record) Persisted {
			return Persisted{PrincipalID: "email@example.invalid", SessionID: r.SessionID}
		}},
		{"nil UUID", func(r Record) Persisted {
			return Persisted{PrincipalID: "00000000-0000-0000-0000-000000000000", SessionID: r.SessionID}
		}},
		{"wrong session", func(r Record) Persisted { return Persisted{PrincipalID: r.PrincipalID, SessionID: r.PrincipalID} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.persist = tc.response
			r, e := f.service.Execute(context.Background(), "opaque-proof")
			assertFailed(t, r, e, ErrStoreUnavailable)
		})
	}
}
func TestFreshClockRejectsProofExpiredBeforeStore(t *testing.T) {
	f := newFixture(t)
	f.service.guard = guardFunc(func(context.Context, Identity) (GuardDecision, error) {
		f.observation.At = f.login.AttemptExpiresAt
		return f.decision, nil
	})
	r, e := f.service.Execute(context.Background(), "opaque-proof")
	assertFailed(t, r, e, ErrConfirmationDenied)
	if f.storeCalls != 0 {
		t.Fatal("expired proof reached store")
	}
}
func TestMaximumPolicyAndAttemptAreAllowed(t *testing.T) {
	f := newFixture(t)
	f.login.AttemptCreatedAt = fixtureTime
	f.login.AttemptExpiresAt = fixtureTime.Add(MaxLoginAttemptLifetime)
	f.service.policy = Policy{AbsoluteLifetime: 24 * time.Hour, IdleLifetime: 24 * time.Hour}
	r, e := f.service.Execute(context.Background(), "opaque-proof")
	if e != nil || !r.IsIssued() || r.ExpiresAt != fixtureTime.Add(24*time.Hour) {
		t.Fatalf("issued=%v err=%v", r.IsIssued(), e)
	}
}
func TestCancellationInDependenciesStopsBeforeWrite(t *testing.T) {
	for _, stage := range []string{"verifier", "clock", "guard"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch stage {
			case "verifier":
				f.service.verifier = verifierFunc(func(context.Context, string) (VerifiedLogin, error) { cancel(); return f.login, nil })
			case "clock":
				f.service.clock = clockFunc(func(context.Context) (sessionpolicy.Observation, error) { cancel(); return f.observation, nil })
			case "guard":
				f.service.guard = guardFunc(func(context.Context, Identity) (GuardDecision, error) { cancel(); return f.decision, nil })
			}
			r, e := f.service.Execute(ctx, "opaque-proof")
			assertFailed(t, r, e, ErrCancelled)
			if f.storeCalls != 0 {
				t.Fatal("cancelled call wrote session")
			}
		})
	}
}
func TestConcurrentOneUseConfirmationAndReplay(t *testing.T) {
	// Controlled authority double. A production verifier must atomically enforce
	// this in its authoritative attempt store; none is shipped by this slice.
	var lock sync.Mutex
	consumed := false
	verifier := verifierFunc(func(_ context.Context, p string) (VerifiedLogin, error) {
		lock.Lock()
		defer lock.Unlock()
		if p != "opaque-proof" || consumed {
			return VerifiedLogin{}, errors.New("already consumed")
		}
		consumed = true
		return fixtureLogin(), nil
	})
	var writes atomic.Int32
	s, e := New(verifier, clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
		return sessionpolicy.Observation{At: fixtureTime, Trusted: true}, nil
	}), guardFunc(func(context.Context, Identity) (GuardDecision, error) {
		return GuardDecision{State: IssuanceAllowed, Epoch: 2}, nil
	}), storeFunc(func(_ context.Context, r Record) (Persisted, error) {
		writes.Add(1)
		return Persisted{PrincipalID: r.PrincipalID, SessionID: r.SessionID}, nil
	}), fixturePolicy)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var issued atomic.Int32
	for range 32 {
		wg.Go(func() {
			r, e := s.Execute(context.Background(), "opaque-proof")
			if e == nil && r.IsIssued() {
				issued.Add(1)
			} else if e != ErrConfirmationDenied || r != (Result{}) {
				t.Errorf("unexpected concurrent issued=%v err=%v", r.IsIssued(), e)
			}
		})
	}
	wg.Wait()
	if issued.Load() != 1 || writes.Load() != 1 {
		t.Fatalf("issued=%d writes=%d", issued.Load(), writes.Load())
	}
	r, e := s.Execute(context.Background(), "opaque-proof")
	assertFailed(t, r, e, ErrConfirmationDenied)
	if writes.Load() != 1 {
		t.Fatal("replay created another session")
	}
}
