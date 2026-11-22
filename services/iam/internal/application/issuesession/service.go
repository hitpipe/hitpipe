package issuesession

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

const MaxLoginAttemptLifetime = 10 * time.Minute

var (
	ErrConfiguration      = errors.New("session_issuance_configuration_invalid")
	ErrInvalidRequest     = errors.New("session_issuance_request_invalid")
	ErrCancelled          = errors.New("session_issuance_request_cancelled")
	ErrConfirmationDenied = errors.New("session_issuance_confirmation_denied")
	ErrClockUnavailable   = errors.New("session_issuance_clock_unavailable")
	ErrGuardUnavailable   = errors.New("session_issuance_guard_unavailable")
	ErrIssuanceDenied     = errors.New("session_issuance_denied")
	ErrEntropyUnavailable = errors.New("session_issuance_entropy_unavailable")
	ErrStoreUnavailable   = errors.New("session_issuance_store_unavailable")
)

// VerifiedLogin is the verifier's immutable identity projection, never a browser
// claim or an email-to-principal lookup. No login token enters a session Record.
type VerifiedLogin struct {
	Issuer, Subject                    string
	EmailVerified                      bool
	AttemptCreatedAt, AttemptExpiresAt time.Time
}

// Verifier validates an opaque login proof and atomically consumes it at most
// once, including across concurrent callers. It must check the trusted provider,
// signature, expected flow/audience and login-attempt binding before returning.
// Production Keycloak/OIDC and the one-use attempt authority belong to HIT-106;
// this package ships no permissive verifier or default implementation.
type Verifier interface {
	Consume(context.Context, string) (VerifiedLogin, error)
}

type Clock interface {
	Now(context.Context) (sessionpolicy.Observation, error)
}

type Identity struct{ Issuer, Subject string }

type GuardState string

const IssuanceAllowed GuardState = "allowed"

// GuardDecision requires an explicit positive authority observation. Zero,
// rotation-required, pending-revocation and unknown states all deny issuance.
// The same positive Epoch must remain current after the commit before secrets
// can be returned. This observation is not a rotation/revocation coordinator.
type GuardDecision struct {
	State            GuardState
	Epoch            int64
	RotationRequired bool
	PendingRevoke    bool
}

func (d GuardDecision) allowed() bool {
	return d.State == IssuanceAllowed && d.Epoch > 0 && !d.RotationRequired && !d.PendingRevoke
}

type IssuanceGuard interface {
	Check(context.Context, Identity) (GuardDecision, error)
}

// Policy has no default lifetime: its values must be supplied from accepted
// deployment policy. Fixture values in tests do not establish production TTLs.
type Policy struct{ AbsoluteLifetime, IdleLifetime time.Duration }

func (p Policy) valid() bool {
	return p.IdleLifetime > 0 && p.IdleLifetime <= p.AbsoluteLifetime && p.AbsoluteLifetime <= sessionpolicy.MaxAbsoluteLifetime
}

// Secret requires an explicit Reveal at the future transport boundary. Generic
// formatting and JSON redact its raw value, including when nested in a Result.
type Secret struct{ value string }

func (s Secret) Reveal() string               { return s.value }
func (s Secret) String() string               { return "[REDACTED]" }
func (s Secret) GoString() string             { return "issuesession.Secret([REDACTED])" }
func (s Secret) Format(f fmt.State, _ rune)   { _, _ = io.WriteString(f, "[REDACTED]") }
func (s Secret) MarshalJSON() ([]byte, error) { return json.Marshal("[REDACTED]") }

// CookieMetadata is a fixed transport contract, not an HTTP handler or response.
// CSRF remains a separate value; its delivery and validation are future transport
// work. Domain deliberately remains empty for the __Host- cookie.
type CookieMetadata struct {
	Name, Path, Domain, SameSite string
	Secure, HTTPOnly             bool
}

type Result struct {
	PrincipalID, SessionID              string
	CreatedAt, ExpiresAt, IdleExpiresAt time.Time
	Session, CSRF                       Secret
	Cookie                              CookieMetadata
}

func (r Result) IsIssued() bool {
	return r.Session.value != "" && r.CSRF.value != "" && validUUID(r.PrincipalID) && validUUID(r.SessionID)
}

// Service is immutable after New. Injected ports must be safe for the caller's
// intended concurrency/lifetime. Production randomness is crypto/rand only.
type Service struct {
	verifier Verifier
	clock    Clock
	guard    IssuanceGuard
	store    Store
	policy   Policy
	entropy  io.Reader // unexported fault-injection seam; New always uses rand.Reader.
}

func absent(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}
	return false
}

func New(verifier Verifier, clock Clock, guard IssuanceGuard, store Store, policy Policy) (Service, error) {
	if absent(verifier) || absent(clock) || absent(guard) || absent(store) || !policy.valid() {
		return Service{}, ErrConfiguration
	}
	return Service{verifier: verifier, clock: clock, guard: guard, store: store, policy: policy, entropy: rand.Reader}, nil
}

// Identity lengths are internal port budgets aligned with the store, not a new
// public registration or OIDC wire contract. Exact trusted values are retained.
func validIdentityPart(value string, max int) bool {
	if !utf8.ValidString(value) || len(value) == 0 || len(value) > max || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func (login VerifiedLogin) valid(now time.Time) bool {
	created, expires := login.AttemptCreatedAt.UTC(), login.AttemptExpiresAt.UTC()
	return login.EmailVerified && validIdentityPart(login.Issuer, 2048) && validIdentityPart(login.Subject, 255) &&
		usableTime(created) && usableTime(expires) && expires.After(created) &&
		expires.Sub(created) <= MaxLoginAttemptLifetime && !now.Before(created) && now.Before(expires)
}

func usableTime(value time.Time) bool {
	return !value.IsZero() && value.UTC().Year() >= 1 && value.UTC().Year() <= 9999
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	var bytes [16]byte
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	_, err := hex.Decode(bytes[:], []byte(compact))
	return err == nil && value != "00000000-0000-0000-0000-000000000000"
}

func uuid(reader io.Reader) (string, error) {
	var bytes [16]byte
	if _, err := io.ReadFull(reader, bytes[:]); err != nil {
		return "", ErrEntropyUnavailable
	}
	bytes[6], bytes[8] = bytes[6]&0x0f|0x40, bytes[8]&0x3f|0x80
	var dst [36]byte
	hex.Encode(dst[0:8], bytes[0:4])
	dst[8] = '-'
	hex.Encode(dst[9:13], bytes[4:6])
	dst[13] = '-'
	hex.Encode(dst[14:18], bytes[6:8])
	dst[18] = '-'
	hex.Encode(dst[19:23], bytes[8:10])
	dst[23] = '-'
	hex.Encode(dst[24:36], bytes[10:16])
	return string(dst[:]), nil
}

func secret(reader io.Reader) (Secret, error) {
	var bytes [32]byte
	if _, err := io.ReadFull(reader, bytes[:]); err != nil {
		return Secret{}, ErrEntropyUnavailable
	}
	return Secret{value: base64.RawURLEncoding.EncodeToString(bytes[:])}, nil
}

func (s Service) now(ctx context.Context) (time.Time, error) {
	observation, err := s.clock.Now(ctx)
	if ctx.Err() != nil {
		return time.Time{}, ErrCancelled
	}
	if err != nil || !observation.Usable() {
		return time.Time{}, ErrClockUnavailable
	}
	return observation.At.UTC(), nil
}

// Execute never retries a proof or store write. A store failure, uncertain commit,
// cancellation or failed post-commit authority/time recheck returns zero Result.
// A row may already exist after an uncertain commit or post-commit failure; its
// withheld secret is not returned and this function does not claim a rollback.
func (s Service) Execute(ctx context.Context, proof string) (Result, error) {
	if absent(s.verifier) || absent(s.clock) || absent(s.guard) || absent(s.store) || absent(s.entropy) || !s.policy.valid() {
		return Result{}, ErrConfiguration
	}
	if ctx == nil || len(proof) == 0 || len(proof) > 4096 {
		return Result{}, ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	login, err := s.verifier.Consume(ctx, proof)
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	if err != nil {
		return Result{}, ErrConfirmationDenied
	}
	now, err := s.now(ctx)
	if err != nil {
		return Result{}, err
	}
	if !login.valid(now) {
		return Result{}, ErrConfirmationDenied
	}
	identity := Identity{Issuer: login.Issuer, Subject: login.Subject}
	guard, err := s.guard.Check(ctx, identity)
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	if err != nil {
		return Result{}, ErrGuardUnavailable
	}
	if !guard.allowed() {
		return Result{}, ErrIssuanceDenied
	}
	session, err := secret(s.entropy)
	if err != nil {
		return Result{}, err
	}
	csrf, err := secret(s.entropy)
	if err != nil {
		return Result{}, err
	}
	principalID, err := uuid(s.entropy)
	if err != nil {
		return Result{}, err
	}
	sessionID, err := uuid(s.entropy)
	if err != nil {
		return Result{}, err
	}
	created, err := s.now(ctx)
	if err != nil {
		return Result{}, err
	}
	if created.Before(now) || !login.valid(created) {
		return Result{}, ErrConfirmationDenied
	}
	record := Record{
		Issuer: identity.Issuer, Subject: identity.Subject, PrincipalID: principalID, SessionID: sessionID,
		SessionHash: sha256.Sum256([]byte(session.value)), CSRFHash: sha256.Sum256([]byte(csrf.value)),
		CreatedAt: created, ExpiresAt: created.Add(s.policy.AbsoluteLifetime),
		IdleExpiresAt: created.Add(s.policy.IdleLifetime), Epoch: guard.Epoch,
	}
	if !sessionpolicy.Evaluate(sessionpolicy.Snapshot{Found: true, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, IdleExpiresAt: record.IdleExpiresAt}, sessionpolicy.Observation{At: created, Trusted: true}).IsValid() {
		return Result{}, ErrClockUnavailable
	}
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	persisted, err := s.store.Create(ctx, record)
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	if err != nil || !validUUID(persisted.PrincipalID) || persisted.SessionID != sessionID {
		return Result{}, ErrStoreUnavailable
	}
	currentGuard, err := s.guard.Check(ctx, identity)
	if ctx.Err() != nil {
		return Result{}, ErrCancelled
	}
	if err != nil {
		return Result{}, ErrGuardUnavailable
	}
	if !currentGuard.allowed() || currentGuard.Epoch != guard.Epoch {
		return Result{}, ErrIssuanceDenied
	}
	finished, err := s.now(ctx)
	if err != nil {
		return Result{}, err
	}
	if !login.valid(finished) || !sessionpolicy.Evaluate(sessionpolicy.Snapshot{Found: true, CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, IdleExpiresAt: record.IdleExpiresAt}, sessionpolicy.Observation{At: finished, Trusted: true}).IsValid() {
		return Result{}, ErrIssuanceDenied
	}
	return Result{
		PrincipalID: persisted.PrincipalID, SessionID: sessionID,
		CreatedAt: record.CreatedAt, ExpiresAt: record.ExpiresAt, IdleExpiresAt: record.IdleExpiresAt,
		Session: session, CSRF: csrf,
		Cookie: CookieMetadata{Name: "__Host-hp_session", Path: "/", Secure: true, HTTPOnly: true, SameSite: "Lax"},
	}, nil
}
