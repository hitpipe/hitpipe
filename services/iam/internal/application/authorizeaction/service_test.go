package authorizeaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"strings"
	"sync"
	"testing"
	"time"
)

type lookupFunc func(context.Context, string) (sessionpolicy.Snapshot, error)

func (f lookupFunc) Lookup(c context.Context, r string) (sessionpolicy.Snapshot, error) {
	return f(c, r)
}

type clockFunc func(context.Context) (sessionpolicy.Observation, error)

func (f clockFunc) Now(c context.Context) (sessionpolicy.Observation, error) { return f(c) }

type callerFunc func(context.Context) (Caller, error)

func (f callerFunc) Current(c context.Context) (Caller, error) { return f(c) }

type readerFunc func(context.Context, ReadRequest) (Snapshot, error)

func (f readerFunc) Read(c context.Context, r ReadRequest) (Snapshot, error) { return f(c, r) }

type ownerFunc func(context.Context, Actor, Request) (Proof, error)

func (f ownerFunc) Prove(c context.Context, a Actor, r Request) (Proof, error) { return f(c, a, r) }

type gateFunc func(context.Context, GateBinding, func(context.Context, Admission) error) error

func (f gateFunc) Within(c context.Context, b GateBinding, n func(context.Context, Admission) error) error {
	return f(c, b, n)
}

type admission struct {
	binding GateBinding
	confirm func(context.Context, ScopeEpochs) (int64, error)
}

func (a *admission) Binding() GateBinding                                    { return a.binding }
func (a *admission) Confirm(c context.Context, e ScopeEpochs) (int64, error) { return a.confirm(c, e) }

const workspaceID = "ab000000-0000-4000-8000-000000000001"
const principalID = "ab000000-0000-4000-8000-000000000002"
const sessionID = "ab000000-0000-4000-8000-000000000003"
const runtimeID = "ab000000-0000-4000-8000-000000000004"
const bootID = "ab000000-0000-4000-8000-000000000005"
const resourceID = "ab000000-0000-4000-8000-000000000006"
const grantID = "ab000000-0000-4000-8000-000000000007"
const foreignID = "ab000000-0000-4000-8000-000000000008"
const rawSession = "HIT84-SYNTHETIC-OPAQUE-SESSION-012345678901234567890"

var testTime = time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)

func ref(value string) (string, error) {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:]), nil
}
func caller() Caller {
	return Caller{Service: "BFF", RuntimeID: runtimeID, BootID: bootID, Generation: 2, CertificateFingerprint: "sha256:" + strings.Repeat("a", 64)}
}
func policy() sessionpolicy.Snapshot {
	return sessionpolicy.Snapshot{Found: true, CreatedAt: testTime.Add(-time.Minute), ExpiresAt: testTime.Add(time.Hour), IdleExpiresAt: testTime.Add(30 * time.Minute)}
}
func snapshot() Snapshot {
	return Snapshot{Session: Session{Found: true, ID: sessionID, PrincipalID: principalID, Epoch: 3, Policy: policy()}, Principal: Principal{Found: true, ID: principalID}, Workspace: Workspace{Found: true, ID: workspaceID, Lifecycle: "ACTIVE", Epoch: 7}, Membership: Membership{Found: true, WorkspaceID: workspaceID, PrincipalID: principalID, Role: "OWNER", Status: "ACTIVE", Revision: 11}, Runtime: Runtime{Found: true, Caller: caller(), Status: "OPEN"}}
}
func request(action string) Request {
	value := resourceID
	return Request{WorkspaceID: workspaceID, RuntimeID: runtimeID, Action: action, ResourceID: &value}
}
func proof(a Actor, r Request) Proof {
	operation := map[string]string{"PROFILE_READ": "getProfile", "PROFILE_WRITE": "updateProfile", "WORKSPACE_READ": "listWorkspaces", "WORKSPACE_MANAGE": "createWorkspace", "CONFIG_READ": "getDraft", "CONFIG_WRITE": "saveDraft", "ACTIVATE": "activateOperation", "CASE_READ": "getCase", "CASE_RECOVER": "recoverCase", "CONNECTOR_READ": "listConnections", "CONNECTOR_WRITE": "connectService", "AI_PREPARE": "prepareAIProposal"}[r.Action]
	p := Proof{Verified: true, PrincipalID: a.PrincipalID, WorkspaceID: r.WorkspaceID, ResourceID: r.ResourceID, Action: r.Action, Operation: operation}
	if r.Action == "CASE_RECOVER" {
		p.RecoveryAction = "RECHECK"
	}
	if r.Action == "CONNECTOR_READ" || r.Action == "CONNECTOR_WRITE" {
		p.BindingGrantID = grantID
	}
	return p
}

type fixture struct {
	service                                                                  Service
	facts                                                                    Snapshot
	caller                                                                   Caller
	observation                                                              sessionpolicy.Observation
	lookup                                                                   sessionpolicy.Snapshot
	lookupErr, readerErr, callerErr, clockErr, ownerErr, confirmErr, gateErr error
	wireEpoch                                                                int64
	ownerProof                                                               func(Actor, Request) Proof
	afterOwner, afterConfirm                                                 func()
	changeBinding                                                            func(*GateBinding)
	lookupCalls, reads, ownerCalls, confirms                                 int
	active                                                                   bool
	confirmed                                                                ScopeEpochs
}

func fresh(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{facts: snapshot(), caller: caller(), observation: sessionpolicy.Observation{At: testTime, Trusted: true}, lookup: policy(), wireEpoch: 7}
	f.facts.Binding = Binding{Found: true, GrantID: grantID, WorkspaceID: workspaceID, Status: "VERIFIED", Epoch: 13}
	var err error
	f.service, err = New(lookupFunc(func(_ context.Context, r string) (sessionpolicy.Snapshot, error) {
		f.lookupCalls++
		if !f.active {
			t.Error("session read outside gate")
		}
		want, _ := ref(rawSession)
		if r != want {
			t.Error("not exact opaque-secret reference")
		}
		return f.lookup, f.lookupErr
	}), ReferenceFunc(ref), clockFunc(func(context.Context) (sessionpolicy.Observation, error) { return f.observation, f.clockErr }), callerFunc(func(context.Context) (Caller, error) { return f.caller, f.callerErr }), readerFunc(func(_ context.Context, r ReadRequest) (Snapshot, error) {
		f.reads++
		if !f.active {
			t.Error("SQL outside gate")
		}
		want, _ := ref(rawSession)
		if r.Reference != want {
			t.Error("raw secret/UUID/digest was not transformed")
		}
		s := f.facts
		if r.BindingGrantID == "" {
			s.Binding = Binding{}
		}
		return s, f.readerErr
	}), ownerFunc(func(_ context.Context, a Actor, r Request) (Proof, error) {
		f.ownerCalls++
		p := proof(a, r)
		if f.ownerProof != nil {
			p = f.ownerProof(a, r)
		}
		if f.afterOwner != nil {
			f.afterOwner()
		}
		return p, f.ownerErr
	}), gateFunc(func(c context.Context, b GateBinding, next func(context.Context, Admission) error) error {
		f.active = true
		defer func() { f.active = false }()
		if f.changeBinding != nil {
			f.changeBinding(&b)
		}
		e := next(c, &admission{binding: b, confirm: func(_ context.Context, s ScopeEpochs) (int64, error) {
			f.confirms++
			f.confirmed = s
			if f.afterConfirm != nil {
				f.afterConfirm()
			}
			return f.wireEpoch, f.confirmErr
		}})
		if e != nil {
			return e
		}
		return f.gateErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func failed(t *testing.T, r Authorization, e, want error) {
	t.Helper()
	if r != (Authorization{}) || e != want {
		t.Fatalf("want zero reply/%v got allowed=%v err=%v", want, r.Allowed, e)
	}
}
func TestMandatoryPortsAndZeroValue(t *testing.T) {
	f := fresh(t)
	for _, modify := range []func(*Service){func(s *Service) { s.lookup = nil }, func(s *Service) { s.lookup = lookupFunc(nil) }, func(s *Service) { s.reference = nil }, func(s *Service) { s.reference = ReferenceFunc(nil) }, func(s *Service) { s.clock = nil }, func(s *Service) { s.clock = clockFunc(nil) }, func(s *Service) { s.caller = nil }, func(s *Service) { s.caller = callerFunc(nil) }, func(s *Service) { s.reader = nil }, func(s *Service) { s.reader = readerFunc(nil) }, func(s *Service) { s.owner = nil }, func(s *Service) { s.owner = ownerFunc(nil) }, func(s *Service) { s.gate = nil }, func(s *Service) { s.gate = gateFunc(nil) }} {
		s := f.service
		modify(&s)
		if _, e := New(s.lookup, s.reference, s.clock, s.caller, s.reader, s.owner, s.gate); e != ErrConfiguration {
			t.Fatal(e)
		}
		r, e := s.Execute(context.Background(), rawSession, request("CASE_READ"))
		failed(t, r, e, ErrConfiguration)
	}
	r, e := (Service{}).Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrConfiguration)
}
func TestValidReplyReusesConsumerAndKeepsScopeEpochsSeparate(t *testing.T) {
	f := fresh(t)
	r, e := f.service.Execute(context.Background(), rawSession, request("CONNECTOR_READ"))
	if e != nil || !r.Allowed {
		t.Fatal(e)
	}
	if f.lookupCalls != 1 || f.reads != 2 || f.ownerCalls != 1 || f.confirms != 1 || r.Epoch != 7 || !r.CheckedAt.Equal(testTime) || r.PrincipalID != principalID || r.SessionID != sessionID || r.WorkspaceID != workspaceID || r.Role != "OWNER" || r.RevocationPending {
		t.Fatal("incorrect current authority/reply")
	}
	if f.confirmed.SessionEpoch != 3 || f.confirmed.WorkspaceEpoch != 7 || f.confirmed.BindingEpoch != 13 || f.confirmed.MembershipRevision != 11 {
		t.Fatal("scope epochs/revision mixed")
	}
	data, e := json.Marshal(r)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 8 {
		t.Fatal("not OAS projection")
	}
	reference, _ := ref(rawSession)
	if strings.Contains(string(data), rawSession) || strings.Contains(string(data), reference) {
		t.Fatal("credential leaked")
	}
}
func TestRoleActionMatrix(t *testing.T) {
	allowed := map[string]map[string]bool{"OWNER": {"PROFILE_READ": true, "PROFILE_WRITE": true, "WORKSPACE_READ": true, "WORKSPACE_MANAGE": true, "CONFIG_READ": true, "CONFIG_WRITE": true, "ACTIVATE": true, "CASE_READ": true, "CASE_RECOVER": true, "CONNECTOR_READ": true, "CONNECTOR_WRITE": true, "AI_PREPARE": true}, "OPERATOR": {"PROFILE_READ": true, "PROFILE_WRITE": true, "WORKSPACE_READ": true, "CASE_READ": true, "CASE_RECOVER": true}, "READER": {"PROFILE_READ": true, "PROFILE_WRITE": true, "WORKSPACE_READ": true, "CONFIG_READ": true, "CASE_READ": true}}
	actions := []string{"PROFILE_READ", "PROFILE_WRITE", "WORKSPACE_READ", "WORKSPACE_MANAGE", "CONFIG_READ", "CONFIG_WRITE", "ACTIVATE", "CASE_READ", "CASE_CONTEXT_WRITE", "CASE_RECOVER", "OWNER_CLOSE", "EXPORT", "CONNECTOR_READ", "CONNECTOR_WRITE", "AI_PREPARE"}
	for role, matrix := range allowed {
		for _, action := range actions {
			t.Run(role+"/"+action, func(t *testing.T) {
				f := fresh(t)
				f.facts.Membership.Role = role
				q := request(action)
				if action == "PROFILE_READ" || action == "PROFILE_WRITE" {
					q.ResourceID = nil
				}
				r, e := f.service.Execute(context.Background(), rawSession, q)
				if matrix[action] {
					if e != nil || !r.Allowed {
						t.Fatal(e)
					}
				} else {
					failed(t, r, e, ErrDenied)
				}
			})
		}
	}
}
func TestInvalidInputAndCancellationDoNotReadAuthority(t *testing.T) {
	for _, tc := range []struct {
		name, secret string
		modify       func(*Request)
	}{{"public UUID", sessionID, func(*Request) {}}, {"short", "bad", func(*Request) {}}, {"long", strings.Repeat("x", 129), func(*Request) {}}, {"workspace", rawSession, func(r *Request) { r.WorkspaceID = "bad" }}, {"runtime", rawSession, func(r *Request) { r.RuntimeID = "bad" }}, {"resource", rawSession, func(r *Request) { v := "bad"; r.ResourceID = &v }}, {"action", rawSession, func(r *Request) { r.Action = "PAYMENT_WRITE" }}} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			q := request("CASE_READ")
			tc.modify(&q)
			r, e := f.service.Execute(context.Background(), tc.secret, q)
			failed(t, r, e, ErrInvalidRequest)
			if f.lookupCalls+f.reads != 0 {
				t.Fatal("invalid input read authority")
			}
		})
	}
	f := fresh(t)
	r, e := f.service.Execute(nil, rawSession, request("CASE_READ"))
	failed(t, r, e, ErrInvalidRequest)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r, e = f.service.Execute(ctx, rawSession, request("CASE_READ"))
	failed(t, r, e, ErrCancelled)
}
func TestInvalidCurrentFactsAndInfrastructureFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fixture)
		want   error
	}{
		{"unknown session", func(f *fixture) { f.lookup.Found = false }, ErrDenied}, {"revoked session", func(f *fixture) { f.lookup.Revoked = true }, ErrDenied}, {"expired session", func(f *fixture) { f.lookup.ExpiresAt = testTime }, ErrDenied}, {"idle expired", func(f *fixture) { f.lookup.IdleExpiresAt = testTime }, ErrDenied}, {"fresh session revoked", func(f *fixture) { f.facts.Session.Policy.Revoked = true }, ErrDenied}, {"fresh session unknown", func(f *fixture) { f.facts.Session.Found = false }, ErrDenied},
		{"zero epoch", func(f *fixture) { f.facts.Session.Epoch = 0 }, ErrUnavailable}, {"unsafe epoch", func(f *fixture) { f.facts.Workspace.Epoch = MaxWireInteger + 1 }, ErrUnavailable}, {"zero revision", func(f *fixture) { f.facts.Membership.Revision = 0 }, ErrUnavailable},
		{"principal disabled", func(f *fixture) { f.facts.Principal.Disabled = true }, ErrDenied}, {"principal unknown", func(f *fixture) { f.facts.Principal.Found = false }, ErrDenied}, {"foreign principal", func(f *fixture) { f.facts.Principal.ID = foreignID }, ErrDenied}, {"foreign membership", func(f *fixture) { f.facts.Membership.PrincipalID = foreignID }, ErrDenied}, {"foreign workspace", func(f *fixture) { f.facts.Workspace.ID = foreignID }, ErrDenied}, {"foreign membership scope", func(f *fixture) { f.facts.Membership.WorkspaceID = foreignID }, ErrDenied},
		{"invited", func(f *fixture) { f.facts.Membership.Status = "INVITED" }, ErrDenied}, {"revoked", func(f *fixture) { f.facts.Membership.Revoked = true }, ErrDenied}, {"unknown role", func(f *fixture) { f.facts.Membership.Role = "SUPPORT" }, ErrDenied}, {"deleting workspace", func(f *fixture) { f.facts.Workspace.Lifecycle = "DELETING" }, ErrDenied}, {"deleted workspace", func(f *fixture) { f.facts.Workspace.Lifecycle = "DELETED" }, ErrDenied},
		{"runtime closed", func(f *fixture) { f.facts.Runtime.Status = "CLOSED" }, ErrDenied}, {"runtime fenced", func(f *fixture) { f.facts.Runtime.Status = "FENCED" }, ErrDenied}, {"runtime unknown", func(f *fixture) { f.facts.Runtime.Found = false }, ErrDenied}, {"wrong generation", func(f *fixture) { f.facts.Runtime.Caller.Generation++ }, ErrDenied}, {"wrong boot", func(f *fixture) { f.facts.Runtime.Caller.BootID = foreignID }, ErrDenied}, {"wrong certificate", func(f *fixture) { f.facts.Runtime.Caller.CertificateFingerprint = "sha256:" + strings.Repeat("b", 64) }, ErrDenied},
		{"browser caller", func(f *fixture) { f.caller.Service = "BROWSER" }, ErrDenied}, {"wrong caller runtime", func(f *fixture) { f.caller.RuntimeID = foreignID }, ErrDenied}, {"zero generation", func(f *fixture) { f.caller.Generation = 0 }, ErrDenied}, {"untrusted clock", func(f *fixture) { f.observation.Trusted = false }, ErrUnavailable}, {"zero clock", func(f *fixture) { f.observation.At = time.Time{} }, ErrUnavailable},
		{"lookup error", func(f *fixture) { f.lookupErr = errors.New("SENSITIVE-lookup") }, ErrUnavailable}, {"reader error", func(f *fixture) { f.readerErr = errors.New("SENSITIVE-SQL-args") }, ErrUnavailable}, {"caller error", func(f *fixture) { f.callerErr = errors.New("SENSITIVE-certificate") }, ErrUnavailable}, {"clock error", func(f *fixture) { f.clockErr = errors.New("SENSITIVE-clock") }, ErrUnavailable}, {"owner error", func(f *fixture) { f.ownerErr = errors.New("SENSITIVE-object") }, ErrUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			tc.change(f)
			r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
			failed(t, r, e, tc.want)
		})
	}
}
func TestRoleCannotReplaceExactOwnerResourceProof(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Proof)
	}{{"unknown", func(p *Proof) { p.Verified = false }}, {"foreign actor", func(p *Proof) { p.PrincipalID = foreignID }}, {"foreign scope", func(p *Proof) { p.WorkspaceID = foreignID }}, {"foreign resource", func(p *Proof) { v := foreignID; p.ResourceID = &v }}, {"different action", func(p *Proof) { p.Action = "CONFIG_WRITE" }}, {"missing purpose", func(p *Proof) { p.Operation = "" }}, {"wrong purpose", func(p *Proof) { p.Operation = "saveDraft" }}, {"deferred purpose", func(p *Proof) { p.Operation = "acceptConfigurationChange" }}} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh(t)
			f.ownerProof = func(a Actor, r Request) Proof { p := proof(a, r); tc.change(&p); return p }
			r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
			failed(t, r, e, ErrDenied)
		})
	}
	f := fresh(t)
	r, e := f.service.Execute(context.Background(), rawSession, request("PROFILE_READ"))
	failed(t, r, e, ErrDenied)
	f = fresh(t)
	f.ownerProof = func(a Actor, r Request) Proof { p := proof(a, r); p.BindingGrantID = ""; return p }
	r, e = f.service.Execute(context.Background(), rawSession, request("CONNECTOR_READ"))
	failed(t, r, e, ErrDenied)
}
func TestRetainedRecoveryAndLifecycleCommands(t *testing.T) {
	for _, action := range []string{"", "BIND_TASK", "CLOSE_WITHOUT_CONFIRMATION", "CONTINUE_SAFE", "RECONSIDER", "REOPEN_CLOSED"} {
		f := fresh(t)
		f.ownerProof = func(a Actor, r Request) Proof { p := proof(a, r); p.RecoveryAction = action; return p }
		r, e := f.service.Execute(context.Background(), rawSession, request("CASE_RECOVER"))
		failed(t, r, e, ErrDenied)
	}
	for _, action := range []string{"PAUSE", "PREPARE_RESUME", "ACCEPT_RESUME", "ARCHIVE", "", "DELETE"} {
		t.Run(action, func(t *testing.T) {
			f := fresh(t)
			f.ownerProof = func(a Actor, r Request) Proof {
				p := proof(a, r)
				p.Operation = "changeOperationLifecycle"
				p.LifecycleAction = action
				return p
			}
			r, e := f.service.Execute(context.Background(), rawSession, request("ACTIVATE"))
			if action == "PAUSE" || action == "PREPARE_RESUME" || action == "ACCEPT_RESUME" {
				if e != nil || !r.Allowed {
					t.Fatal(e)
				}
			} else {
				failed(t, r, e, ErrDenied)
			}
		})
	}
}
func TestGateRequiresExactBindingCurrentWorkspaceEpochAndFinalSuccess(t *testing.T) {
	for _, change := range []func(*GateBinding){func(b *GateBinding) { b.Caller.Generation++ }, func(b *GateBinding) { b.Request.Action = "CONFIG_WRITE" }, func(b *GateBinding) { b.Request.WorkspaceID = foreignID }, func(b *GateBinding) { v := foreignID; b.Request.ResourceID = &v }, func(b *GateBinding) { b.Reference = strings.Repeat("b", 64) }} {
		f := fresh(t)
		f.changeBinding = change
		r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
		failed(t, r, e, ErrUnavailable)
		if f.lookupCalls+f.reads != 0 {
			t.Fatal("unbound gate read SQL")
		}
	}
	for _, epoch := range []int64{0, -1, 1, 3, 8, 13, MaxWireInteger + 1} {
		f := fresh(t)
		f.wireEpoch = epoch
		r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
		failed(t, r, e, ErrUnavailable)
	}
	f := fresh(t)
	f.gateErr = errors.New("SENSITIVE-final-gate")
	r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrUnavailable)
	f = fresh(t)
	f.confirmErr = errors.New("unknown/old deny epoch")
	r, e = f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrUnavailable)
	f = fresh(t)
	f.service.gate = gateFunc(func(context.Context, GateBinding, func(context.Context, Admission) error) error { return nil })
	r, e = f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrUnavailable)
}
func TestFreshReadsAfterOwnerWaitCatchChanges(t *testing.T) {
	for _, change := range []func(*fixture){func(f *fixture) { f.facts.Membership.Role = "READER" }, func(f *fixture) { f.facts.Membership.Status = "REVOKED" }, func(f *fixture) { f.facts.Session.Policy.Revoked = true }, func(f *fixture) { f.facts.Workspace.Epoch++ }} {
		f := fresh(t)
		f.afterOwner = func() { change(f) }
		r, e := f.service.Execute(context.Background(), rawSession, request("CONFIG_WRITE"))
		if e == nil || r != (Authorization{}) || f.reads != 2 {
			t.Fatal("stale pre-owner authority allowed")
		}
	}
}
func TestGateWaitPrecedesAllReads(t *testing.T) {
	f := fresh(t)
	entered, release := make(chan struct{}), make(chan struct{})
	gate := f.service.gate
	f.service.gate = gateFunc(func(c context.Context, b GateBinding, next func(context.Context, Admission) error) error {
		close(entered)
		<-release
		return gate.Within(c, b, next)
	})
	done := make(chan error, 1)
	go func() {
		r, e := f.service.Execute(context.Background(), rawSession, request("CONFIG_WRITE"))
		if r != (Authorization{}) {
			done <- errors.New("unexpected grant")
		} else {
			done <- e
		}
	}()
	<-entered
	if f.reads+f.lookupCalls != 0 {
		t.Fatal("SQL before gate")
	}
	f.facts.Membership.Role = "READER"
	close(release)
	if e := <-done; e != ErrDenied {
		t.Fatal(e)
	}
}
func TestRevocationScopeAndHistoryAreNotFlattened(t *testing.T) {
	for _, scope := range []string{"SESSION", "WORKSPACE", "MEMBERSHIP", "CONNECTION"} {
		for _, status := range []string{"PENDING", "INCIDENT"} {
			t.Run(scope+status, func(t *testing.T) {
				f := fresh(t)
				subject := sessionID
				action := "CASE_READ"
				if scope == "WORKSPACE" {
					subject = workspaceID
				}
				if scope == "CONNECTION" {
					subject = grantID
					action = "CONNECTOR_READ"
				}
				f.facts.Revocations = []Revocation{{Scope: scope, SubjectID: subject, WorkspaceID: workspaceID, Epoch: 19, Status: status}}
				r, e := f.service.Execute(context.Background(), rawSession, request(action))
				failed(t, r, e, ErrDenied)
			})
		}
	}
	for _, confirmed := range []bool{false, true} {
		f := fresh(t)
		f.facts.Revocations = []Revocation{{Scope: "WORKSPACE", SubjectID: workspaceID, WorkspaceID: workspaceID, Epoch: 5, Status: "EFFECTIVE"}}
		if !confirmed {
			f.confirmErr = errors.New("no explicit current-grant overlay")
		}
		r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
		if confirmed {
			if e != nil || !r.Allowed || len(f.confirmed.Revocations) != 1 {
				t.Fatal("deny history lost")
			}
		} else {
			failed(t, r, e, ErrUnavailable)
		}
	}
	f := fresh(t)
	f.facts.Revocations = []Revocation{{Scope: "SESSION", SubjectID: sessionID, Epoch: 2, Status: "EFFECTIVE"}}
	r, e := f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrDenied)
	f = fresh(t)
	f.facts.Revocations = []Revocation{{Scope: "MEMBERSHIP", SubjectID: foreignID, WorkspaceID: workspaceID, Epoch: 2, Status: "EFFECTIVE"}}
	r, e = f.service.Execute(context.Background(), rawSession, request("CASE_READ"))
	failed(t, r, e, ErrDenied)
	f = fresh(t)
	f.facts.Revocations = []Revocation{{Scope: "CONNECTION", SubjectID: foreignID, WorkspaceID: workspaceID, Epoch: 19, Status: "PENDING"}}
	q := request("PROFILE_READ")
	q.ResourceID = nil
	r, e = f.service.Execute(context.Background(), rawSession, q)
	if e != nil || !r.Allowed || len(f.confirmed.Revocations) != 0 {
		t.Fatal("unrelated connector blocked profile")
	}
}
func TestFinalClockAndCancellationWithholdGrant(t *testing.T) {
	for _, mode := range []string{"clock error", "untrusted", "expired", "backwards", "cancel", "cancel after callback"} {
		t.Run(mode, func(t *testing.T) {
			f := fresh(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.afterConfirm = func() {
				switch mode {
				case "clock error":
					f.clockErr = errors.New("clock")
				case "untrusted":
					f.observation.Trusted = false
				case "expired":
					f.observation.At = f.facts.Session.Policy.IdleExpiresAt
				case "backwards":
					f.observation.At = f.facts.Session.Policy.CreatedAt.Add(-time.Second)
				case "cancel":
					cancel()
				}
			}
			if mode == "cancel after callback" {
				gate := f.service.gate
				f.service.gate = gateFunc(func(c context.Context, b GateBinding, next func(context.Context, Admission) error) error {
					e := gate.Within(c, b, next)
					cancel()
					return e
				})
			}
			r, e := f.service.Execute(ctx, rawSession, request("CASE_READ"))
			if e == nil || r != (Authorization{}) {
				t.Fatal("grant despite failed final check")
			}
		})
	}
}
func TestOnboardingAndUppercaseUUIDs(t *testing.T) {
	f := fresh(t)
	f.facts.Workspace.Lifecycle = "ONBOARDING"
	q := request("CONFIG_WRITE")
	q.WorkspaceID = strings.ToUpper(q.WorkspaceID)
	q.RuntimeID = strings.ToUpper(q.RuntimeID)
	v := strings.ToUpper(*q.ResourceID)
	q.ResourceID = &v
	f.caller.RuntimeID = strings.ToUpper(f.caller.RuntimeID)
	f.caller.BootID = strings.ToUpper(f.caller.BootID)
	r, e := f.service.Execute(context.Background(), rawSession, q)
	if e != nil || !r.Allowed {
		t.Fatal(e)
	}
}
func TestConcurrentChecksAfterCommittedMembershipRevoke(t *testing.T) {
	// Controlled in-process gate/authority doubles, not a distributed adapter.
	var lock sync.Mutex
	facts := snapshot()
	reads := 0
	gate := gateFunc(func(c context.Context, b GateBinding, next func(context.Context, Admission) error) error {
		lock.Lock()
		defer lock.Unlock()
		return next(c, &admission{binding: b, confirm: func(context.Context, ScopeEpochs) (int64, error) { return 7, nil }})
	})
	s, e := New(lookupFunc(func(context.Context, string) (sessionpolicy.Snapshot, error) { return policy(), nil }), ReferenceFunc(ref), clockFunc(func(context.Context) (sessionpolicy.Observation, error) {
		return sessionpolicy.Observation{At: testTime, Trusted: true}, nil
	}), callerFunc(func(context.Context) (Caller, error) { return caller(), nil }), readerFunc(func(context.Context, ReadRequest) (Snapshot, error) { reads++; return facts, nil }), ownerFunc(func(_ context.Context, a Actor, r Request) (Proof, error) { return proof(a, r), nil }), gate)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Execute(context.Background(), rawSession, request("CONFIG_WRITE"))
	if e != nil || !r.Allowed {
		t.Fatal(e)
	}
	lock.Lock()
	facts.Membership.Status = "REVOKED"
	facts.Membership.Revision++
	lock.Unlock()
	var group sync.WaitGroup
	for range 32 {
		group.Go(func() {
			r, e := s.Execute(context.Background(), rawSession, request("CONFIG_WRITE"))
			if e != ErrDenied || r != (Authorization{}) {
				t.Error("committed revoke bypassed")
			}
		})
	}
	group.Wait()
	if reads != 34 {
		t.Fatal("cached authority used")
	}
}

// Both roles permit these broad action names, but the owner's resource proof
// can cover a wider area. A changed actor must obtain a new scoped proof.
func TestRoleChangeInvalidatesOwnerProofEvenWhenBothRolesCanRead(t *testing.T) {
	for _, action := range []string{"CASE_READ", "CONFIG_READ"} {
		t.Run(action, func(t *testing.T) {
			f := fresh(t)
			f.afterOwner = func() { f.facts.Membership.Role = "READER" }
			result, err := f.service.Execute(context.Background(), rawSession, request(action))
			failed(t, result, err, ErrDenied)
			if f.reads != 2 || f.confirms != 0 {
				t.Fatal("changed actor reused an earlier owner proof")
			}
		})
	}
}
