package getprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

type lookupFunc func(context.Context, string) (sessionpolicy.Snapshot, error)

func (f lookupFunc) Lookup(c context.Context, r string) (sessionpolicy.Snapshot, error) {
	return f(c, r)
}

type clockFunc func(context.Context) (sessionpolicy.Observation, error)

func (f clockFunc) Now(c context.Context) (sessionpolicy.Observation, error) { return f(c) }

type readerFunc func(context.Context, string, authorizeaction.Caller) (Snapshot, error)

func (f readerFunc) Read(c context.Context, r string, a authorizeaction.Caller) (Snapshot, error) {
	return f(c, r, a)
}

type contentFunc func(context.Context, string, string) (Profile, error)

func (f contentFunc) Read(c context.Context, r, d string) (Profile, error) { return f(c, r, d) }

type gateFunc func(context.Context, Binding, func(context.Context, Admission) error) error

func (f gateFunc) Within(c context.Context, b Binding, next func(context.Context, Admission) error) error {
	return f(c, b, next)
}

type admission struct {
	binding Binding
	confirm func(context.Context, Facts) error
}

func (a *admission) Binding() Binding                         { return a.binding }
func (a *admission) Confirm(c context.Context, f Facts) error { return a.confirm(c, f) }

const principalID = "ab000000-0000-4000-8000-000000000001"
const sessionID = "ab000000-0000-4000-8000-000000000002"
const runtimeID = "ab000000-0000-4000-8000-000000000003"
const bootID = "ab000000-0000-4000-8000-000000000004"
const profileRef = "ab000000-0000-4000-8000-000000000005"
const otherID = "ab000000-0000-4000-8000-000000000006"
const rawSession = "HIT85-SYNTHETIC-OPAQUE-SESSION-012345678901234567890"

var testTime = time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)
var digest = "sha256:" + strings.Repeat("a", 64)

func ref(value string) (string, error) {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}
func caller() authorizeaction.Caller {
	return authorizeaction.Caller{Service: "BFF", RuntimeID: runtimeID, BootID: bootID, Generation: 2, CertificateFingerprint: digest}
}
func policy() sessionpolicy.Snapshot {
	return sessionpolicy.Snapshot{Found: true, CreatedAt: testTime.Add(-time.Minute), ExpiresAt: testTime.Add(time.Hour), IdleExpiresAt: testTime.Add(30 * time.Minute)}
}
func snapshot() Snapshot {
	return Snapshot{Session: authorizeaction.Session{Found: true, ID: sessionID, PrincipalID: principalID, Epoch: 3, Policy: policy()}, Principal: authorizeaction.Principal{Found: true, ID: principalID}, Runtime: authorizeaction.Runtime{Found: true, Caller: caller(), Status: "OPEN"}, ProfileRef: profileRef, ProfileDigest: digest}
}
func profile() Profile {
	return Profile{UserID: principalID, DisplayName: "Синтетический студент", Timezone: "Europe/Moscow", Revision: 9}
}

type fixture struct {
	service                                                         Service
	facts                                                           Snapshot
	profile                                                         Profile
	lookup                                                          sessionpolicy.Snapshot
	observation                                                     sessionpolicy.Observation
	lookupErr, readerErr, contentErr, clockErr, confirmErr, gateErr error
	afterContent, afterConfirm                                      func()
	changeBinding                                                   func(*Binding)
	active                                                          bool
	reads, lookups, contents, emits, confirms                       int
	emitted                                                         Profile
	confirmed                                                       Facts
}

func fresh(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{facts: snapshot(), profile: profile(), lookup: policy(), observation: sessionpolicy.Observation{At: testTime, Trusted: true}}
	var err error
	f.service, err = New(lookupFunc(func(ctx context.Context, r string) (sessionpolicy.Snapshot, error) {
		f.lookups++
		if !f.active {
			t.Error("session read outside gate")
		}
		expected, _ := ref(rawSession)
		if r != expected {
			t.Error("not exact opaque-secret digest")
		}
		return f.lookup, f.lookupErr
	}), authorizeaction.ReferenceFunc(ref), clockFunc(func(context.Context) (sessionpolicy.Observation, error) { return f.observation, f.clockErr }), readerFunc(func(ctx context.Context, r string, a authorizeaction.Caller) (Snapshot, error) {
		f.reads++
		if !f.active {
			t.Error("SQL outside response gate")
		}
		if d, ok := ctx.Deadline(); !ok || time.Until(d) > criticalBudget {
			t.Error("unbounded SQL context")
		}
		expected, _ := ref(rawSession)
		if r != expected || a != caller() {
			t.Error("reader identity/reference changed")
		}
		return f.facts, f.readerErr
	}), contentFunc(func(ctx context.Context, r, d string) (Profile, error) {
		f.contents++
		if !f.active {
			t.Error("content outside gate")
		}
		if r != profileRef || d != digest {
			t.Error("wrong opaque profile reference/digest")
		}
		if f.afterContent != nil {
			f.afterContent()
		}
		return f.profile, f.contentErr
	}), gateFunc(func(ctx context.Context, b Binding, next func(context.Context, Admission) error) error {
		f.active = true
		defer func() { f.active = false }()
		if f.changeBinding != nil {
			f.changeBinding(&b)
		}
		err := next(ctx, &admission{binding: b, confirm: func(_ context.Context, facts Facts) error {
			f.confirms++
			f.confirmed = facts
			if f.afterConfirm != nil {
				f.afterConfirm()
			}
			return f.confirmErr
		}})
		if err != nil {
			return err
		}
		return f.gateErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *fixture) emit(p Profile) error {
	f.emits++
	if !f.active {
		panic("response emitted outside gate")
	}
	f.emitted = p
	return nil
}
func TestRequiresEveryPortAndZeroServiceCannotEmit(t *testing.T) {
	f := fresh(t)
	for _, modify := range []func(*Service){func(s *Service) { s.lookup = nil }, func(s *Service) { s.lookup = lookupFunc(nil) }, func(s *Service) { s.reference = nil }, func(s *Service) { s.reference = authorizeaction.ReferenceFunc(nil) }, func(s *Service) { s.clock = nil }, func(s *Service) { s.clock = clockFunc(nil) }, func(s *Service) { s.reader = nil }, func(s *Service) { s.reader = readerFunc(nil) }, func(s *Service) { s.content = nil }, func(s *Service) { s.content = contentFunc(nil) }, func(s *Service) { s.gate = nil }, func(s *Service) { s.gate = gateFunc(nil) }} {
		s := f.service
		modify(&s)
		if _, err := New(s.lookup, s.reference, s.clock, s.reader, s.content, s.gate); err != ErrConfiguration {
			t.Fatal(err)
		}
		if err := s.Serve(context.Background(), rawSession, caller(), f.emit); err != ErrConfiguration {
			t.Fatal(err)
		}
	}
	if err := (Service{}).Serve(context.Background(), rawSession, caller(), f.emit); err != ErrConfiguration || f.emits != 0 {
		t.Fatal("zero service emitted")
	}
}
func TestOwnGlobalProfileEmitsInsideGateFromCurrentPrincipal(t *testing.T) {
	f := fresh(t)
	f.profile.UserID = strings.ToUpper(f.profile.UserID)
	c := caller()
	c.RuntimeID = strings.ToUpper(c.RuntimeID)
	c.BootID = strings.ToUpper(c.BootID)
	if err := f.service.Serve(context.Background(), rawSession, c, f.emit); err != nil {
		t.Fatal(err)
	}
	if f.emits != 1 || f.emitted.UserID != principalID || f.emitted.DisplayName != profile().DisplayName || f.emitted.Timezone != profile().Timezone || f.emitted.Revision != 9 || f.reads != 2 || f.lookups != 1 || f.contents != 1 || f.confirms != 1 {
		t.Fatal("wrong profile, stale identity or missing boundary")
	}
	facts := f.confirmed
	if facts.SessionID != sessionID || facts.PrincipalID != principalID || facts.ProfileRef != profileRef || facts.ProfileDigest != digest || facts.SessionEpoch != 3 || facts.Caller != caller() {
		t.Fatal("confirmation not bound to exact current SESSION/profile/runtime")
	}
}
func TestMissingSessionInvalidCallerAndCancellationDoNotEmit(t *testing.T) {
	for _, value := range []string{"", "short", sessionID, strings.Repeat("x", 129)} {
		f := fresh(t)
		if e := f.service.Serve(context.Background(), value, caller(), f.emit); e != ErrUnauthenticated || f.emits != 0 || f.reads != 0 {
			t.Fatal("invalid session reached protected read")
		}
	}
	for _, modify := range []func(*authorizeaction.Caller){func(c *authorizeaction.Caller) { c.Service = "IAM" }, func(c *authorizeaction.Caller) { c.Service = "BROWSER" }, func(c *authorizeaction.Caller) { c.RuntimeID = "bad" }, func(c *authorizeaction.Caller) { c.BootID = "bad" }, func(c *authorizeaction.Caller) { c.Generation = 0 }, func(c *authorizeaction.Caller) { c.Generation = authorizeaction.MaxWireInteger + 1 }, func(c *authorizeaction.Caller) { c.CertificateFingerprint = "unverified" }} {
		f := fresh(t)
		c := caller()
		modify(&c)
		if e := f.service.Serve(context.Background(), rawSession, c, f.emit); e != ErrForbidden || f.emits != 0 || f.reads != 0 {
			t.Fatal("unverified service caller reached profile")
		}
	}
	f := fresh(t)
	if e := f.service.Serve(nil, rawSession, caller(), f.emit); e != ErrInvalidRequest {
		t.Fatal(e)
	}
	if e := f.service.Serve(context.Background(), rawSession, caller(), nil); e != ErrInvalidRequest {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := f.service.Serve(ctx, rawSession, caller(), f.emit); e != ErrCancelled || f.emits != 0 {
		t.Fatal(e)
	}
}
func TestCurrentIdentitySessionRuntimeAndContentFailuresNeverEmit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fixture)
		want   error
	}{
		{"unknown legacy session", func(f *fixture) { f.lookup.Found = false }, ErrUnauthenticated}, {"revoked legacy session", func(f *fixture) { f.lookup.Revoked = true }, ErrUnauthenticated}, {"expired legacy session", func(f *fixture) { f.lookup.ExpiresAt = testTime }, ErrUnauthenticated}, {"legacy lookup error", func(f *fixture) { f.lookupErr = errors.New("SENSITIVE-session") }, ErrUnavailable},
		{"fresh session missing", func(f *fixture) { f.facts.Session.Found = false }, ErrUnauthenticated}, {"fresh session revoked", func(f *fixture) { f.facts.Session.Policy.Revoked = true }, ErrUnauthenticated}, {"fresh session expired", func(f *fixture) { f.facts.Session.Policy.IdleExpiresAt = testTime }, ErrUnauthenticated}, {"unknown principal", func(f *fixture) { f.facts.Principal.Found = false }, ErrUnauthenticated}, {"disabled principal", func(f *fixture) { f.facts.Principal.Disabled = true }, ErrUnauthenticated}, {"mismatched principal", func(f *fixture) { f.facts.Principal.ID = otherID }, ErrUnavailable},
		{"zero epoch", func(f *fixture) { f.facts.Session.Epoch = 0 }, ErrUnavailable}, {"unsafe epoch", func(f *fixture) { f.facts.Session.Epoch = authorizeaction.MaxWireInteger + 1 }, ErrUnavailable}, {"missing runtime", func(f *fixture) { f.facts.Runtime.Found = false }, ErrForbidden}, {"CLOSED runtime", func(f *fixture) { f.facts.Runtime.Status = "CLOSED" }, ErrForbidden}, {"SYNCING runtime", func(f *fixture) { f.facts.Runtime.Status = "SYNCING" }, ErrForbidden}, {"FENCED runtime", func(f *fixture) { f.facts.Runtime.Status = "FENCED" }, ErrForbidden}, {"old runtime generation", func(f *fixture) { f.facts.Runtime.Caller.Generation++ }, ErrForbidden}, {"wrong runtime boot", func(f *fixture) { f.facts.Runtime.Caller.BootID = otherID }, ErrForbidden}, {"wrong runtime cert", func(f *fixture) { f.facts.Runtime.Caller.CertificateFingerprint = "sha256:" + strings.Repeat("b", 64) }, ErrForbidden},
		{"missing profile ref", func(f *fixture) { f.facts.ProfileRef = "" }, ErrUnavailable}, {"missing digest", func(f *fixture) { f.facts.ProfileDigest = "" }, ErrUnavailable}, {"bad digest", func(f *fixture) { f.facts.ProfileDigest = "sha256:" + strings.Repeat("A", 64) }, ErrUnavailable}, {"SQL error", func(f *fixture) { f.readerErr = errors.New("SENSITIVE-SQL-arguments") }, ErrUnavailable}, {"content error", func(f *fixture) { f.contentErr = errors.New("SENSITIVE-profile-body") }, ErrUnavailable},
		{"untrusted clock", func(f *fixture) { f.observation.Trusted = false }, ErrUnavailable}, {"zero clock", func(f *fixture) { f.observation.At = time.Time{} }, ErrUnavailable}, {"clock error", func(f *fixture) { f.clockErr = errors.New("SENSITIVE-clock") }, ErrUnavailable}, {"unverified content user", func(f *fixture) { f.profile.UserID = otherID }, ErrUnavailable}, {"empty name", func(f *fixture) { f.profile.DisplayName = "" }, ErrUnavailable}, {"oversized name", func(f *fixture) { f.profile.DisplayName = strings.Repeat("я", 121) }, ErrUnavailable}, {"bad UTF8", func(f *fixture) { f.profile.DisplayName = string([]byte{0xff}) }, ErrUnavailable}, {"empty timezone", func(f *fixture) { f.profile.Timezone = "" }, ErrUnavailable}, {"oversized timezone", func(f *fixture) { f.profile.Timezone = strings.Repeat("z", 81) }, ErrUnavailable}, {"zero revision", func(f *fixture) { f.profile.Revision = 0 }, ErrUnavailable}, {"unsafe revision", func(f *fixture) { f.profile.Revision = authorizeaction.MaxWireInteger + 1 }, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			tc.change(f)
			if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != tc.want || f.emits != 0 {
				t.Fatalf("want %v/no emit; got %v/emits=%d", tc.want, e, f.emits)
			}
		})
	}
}
func TestSessionDenyCannotDisappearThroughWorkspaceFilter(t *testing.T) {
	for _, workspace := range []string{"", otherID} {
		for _, status := range []string{"PENDING", "INCIDENT", "EFFECTIVE"} {
			f := fresh(t)
			f.facts.Revocations = []authorizeaction.Revocation{{Scope: "SESSION", SubjectID: sessionID, WorkspaceID: workspace, Epoch: 4, Status: status}}
			if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnauthenticated || f.emits != 0 {
				t.Fatal("exact SESSION deny did not hold")
			}
		}
	}
	f := fresh(t)
	f.facts.Revocations = []authorizeaction.Revocation{{Scope: "WORKSPACE", SubjectID: otherID, WorkspaceID: otherID, Epoch: 4, Status: "PENDING"}}
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != nil || f.emits != 1 || len(f.confirmed.Revocations) != 0 {
		t.Fatal("invented workspace grant for global profile")
	}
	f = fresh(t)
	f.facts.Revocations = []authorizeaction.Revocation{{Scope: "SESSION", SubjectID: sessionID, Epoch: 4, Status: "UNKNOWN"}}
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 {
		t.Fatal("unknown deny status allowed")
	}
}
func TestChangesDuringContentIOInvalidateEarlierOwnerFacts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fixture)
	}{
		{"profile reference", func(f *fixture) { f.facts.ProfileRef = otherID }}, {"profile digest", func(f *fixture) { f.facts.ProfileDigest = "sha256:" + strings.Repeat("b", 64) }}, {"principal switched", func(f *fixture) { f.facts.Principal.ID = otherID; f.facts.Session.PrincipalID = otherID }}, {"session switched", func(f *fixture) { f.facts.Session.ID = otherID }}, {"epoch changed", func(f *fixture) { f.facts.Session.Epoch++ }}, {"session revoked", func(f *fixture) { f.facts.Session.Policy.Revoked = true }}, {"principal disabled", func(f *fixture) { f.facts.Principal.Disabled = true }}, {"runtime fenced", func(f *fixture) { f.facts.Runtime.Status = "FENCED" }}, {"session lifetime changed", func(f *fixture) { f.facts.Session.Policy.ExpiresAt = f.facts.Session.Policy.ExpiresAt.Add(time.Minute) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			f.afterContent = func() { tc.change(f) }
			if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e == nil || f.emits != 0 || f.reads != 2 {
				t.Fatal("stale profile emitted after authority/file switch")
			}
		})
	}
}
func TestAdmissionBindingConfirmationAndFinalClock(t *testing.T) {
	for _, change := range []func(*Binding){func(b *Binding) { b.Caller.Generation++ }, func(b *Binding) { b.Caller.Service = "AI" }, func(b *Binding) { b.Reference = strings.Repeat("b", 64) }} {
		f := fresh(t)
		f.changeBinding = change
		if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 || f.reads != 0 {
			t.Fatal("mismatched admission reached SQL")
		}
	}
	f := fresh(t)
	f.confirmErr = errors.New("SENSITIVE-old-epoch")
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 {
		t.Fatal("unknown/current deny gate emitted")
	}
	for _, change := range []func(*fixture){func(f *fixture) { f.observation.Trusted = false }, func(f *fixture) { f.observation.At = f.facts.Session.Policy.IdleExpiresAt }, func(f *fixture) { f.observation.At = f.facts.Session.Policy.CreatedAt.Add(-time.Second) }} {
		f := fresh(t)
		f.afterConfirm = func() { change(f) }
		if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e == nil || f.emits != 0 {
			t.Fatal("failed final time check emitted")
		}
	}
}
func TestCancelDuringContentOrConfirmationPreventsResponse(t *testing.T) {
	for _, stage := range []string{"content", "confirmation"} {
		f := fresh(t)
		ctx, cancel := context.WithCancel(context.Background())
		if stage == "content" {
			f.afterContent = cancel
		} else {
			f.afterConfirm = cancel
		}
		if e := f.service.Serve(ctx, rawSession, caller(), f.emit); e != ErrCancelled || f.emits != 0 {
			t.Fatal("cancelled read emitted profile")
		}
		cancel()
	}
}
func TestEmitterAndPostCallbackErrorsNeverCauseASecondResponse(t *testing.T) {
	f := fresh(t)
	f.gateErr = errors.New("SENSITIVE-post-emission")
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 1 {
		t.Fatal("post-emission error retried or was hidden")
	}
	f = fresh(t)
	if e := f.service.Serve(context.Background(), rawSession, caller(), func(p Profile) error { f.emits++; return errors.New("SENSITIVE-write-failure") }); e != ErrUnavailable || f.emits != 1 {
		t.Fatal("failed writer retried")
	}
	f = fresh(t)
	gate := f.service.gate
	f.service.gate = gateFunc(func(c context.Context, b Binding, next func(context.Context, Admission) error) error {
		err := gate.Within(c, b, next)
		if err != nil {
			return err
		}
		return gate.Within(c, b, next)
	})
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 1 {
		t.Fatal("gate callback emitted more than once")
	}
	f = fresh(t)
	f.service.gate = gateFunc(func(context.Context, Binding, func(context.Context, Admission) error) error { return nil })
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 {
		t.Fatal("gate skipped all checks")
	}
}
func TestCriticalBudgetIsRealAndCannotBeRemovedByGate(t *testing.T) {
	f := fresh(t)
	f.service.gate = gateFunc(func(ctx context.Context, _ Binding, _ func(context.Context, Admission) error) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > criticalBudget {
			t.Error("no bounded admission context")
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 {
		t.Fatal("critical deadline did not close read")
	}
	f = fresh(t)
	f.service.gate = gateFunc(func(_ context.Context, b Binding, next func(context.Context, Admission) error) error {
		return next(context.Background(), &admission{binding: b, confirm: func(context.Context, Facts) error { return nil }})
	})
	if e := f.service.Serve(context.Background(), rawSession, caller(), f.emit); e != ErrUnavailable || f.emits != 0 || f.reads != 0 {
		t.Fatal("gate discarded bounded context")
	}
}
func TestConcurrentReadsAfterCommittedSessionRevokeCannotEmit(t *testing.T) {
	// Controlled shared gate/authority doubles; not a production coordinator.
	var lock sync.Mutex
	facts := snapshot()
	var emitted atomic.Int32
	gate := gateFunc(func(ctx context.Context, b Binding, next func(context.Context, Admission) error) error {
		lock.Lock()
		defer lock.Unlock()
		return next(ctx, &admission{binding: b, confirm: func(context.Context, Facts) error { return nil }})
	})
	service, err := New(lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) { return policy(), nil }), authorizeaction.ReferenceFunc(ref), clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
		return sessionpolicy.Observation{At: testTime, Trusted: true}, nil
	}), readerFunc(func(context.Context, string, authorizeaction.Caller) (Snapshot, error) { return facts, nil }), contentFunc(func(context.Context, string, string) (Profile, error) { return profile(), nil }), gate)
	if err != nil {
		t.Fatal(err)
	}
	emit := func(Profile) error { emitted.Add(1); return nil }
	if e := service.Serve(context.Background(), rawSession, caller(), emit); e != nil {
		t.Fatal(e)
	}
	lock.Lock()
	facts.Session.Policy.Revoked = true
	facts.Session.Epoch++
	lock.Unlock()
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			if e := service.Serve(context.Background(), rawSession, caller(), emit); e != ErrUnauthenticated {
				t.Error("committed SESSION revoke did not hold")
			}
		})
	}
	group.Wait()
	if emitted.Load() != 1 {
		t.Fatal("protected response started after committed deny")
	}
}
