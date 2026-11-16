package authorizeaction

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

const MaxWireInteger int64 = 9007199254740991

var (
	ErrConfiguration  = errors.New("authorization_configuration_invalid")
	ErrInvalidRequest = errors.New("authorization_request_invalid")
	ErrDenied         = errors.New("authorization_denied")
	ErrUnavailable    = errors.New("authorization_unavailable")
	ErrCancelled      = errors.New("authorization_cancelled")
)

type Service struct {
	lookup    authenticatesession.SessionLookup
	reference ReferenceDeriver
	clock     authenticatesession.Clock
	caller    CallerAuthority
	reader    Reader
	owner     ResourceAuthority
	gate      StartGate
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
func New(lookup authenticatesession.SessionLookup, reference ReferenceDeriver, clock authenticatesession.Clock, caller CallerAuthority, reader Reader, owner ResourceAuthority, gate StartGate) (Service, error) {
	if absent(lookup) || absent(reference) || absent(clock) || absent(caller) || absent(reader) || absent(owner) || absent(gate) {
		return Service{}, ErrConfiguration
	}
	return Service{lookup: lookup, reference: reference, clock: clock, caller: caller, reader: reader, owner: owner, gate: gate}, nil
}
func validInteger(value int64) bool { return value > 0 && value <= MaxWireInteger }
func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if len(compact) != 32 {
		return false
	}
	var raw [16]byte
	_, err := hex.Decode(raw[:], []byte(compact))
	return err == nil && raw != ([16]byte{})
}
func validReference(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, b := range []byte(value) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}
func validFingerprint(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validReference(strings.TrimPrefix(value, "sha256:"))
}
func validCaller(c Caller) bool {
	if !validUUID(c.RuntimeID) || !validUUID(c.BootID) || !validInteger(c.Generation) || !validFingerprint(c.CertificateFingerprint) {
		return false
	}
	switch c.Service {
	case "IAM", "INTAKE", "PROCESSING", "DELIVERY", "BFF", "AI":
		return true
	}
	return false
}
func copyRequest(r Request) Request {
	r.WorkspaceID = strings.ToLower(r.WorkspaceID)
	r.RuntimeID = strings.ToLower(r.RuntimeID)
	if r.ResourceID != nil {
		value := strings.ToLower(*r.ResourceID)
		r.ResourceID = &value
	}
	return r
}
func sameResource(left, right *string) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}
func sameRequest(left, right Request) bool {
	return left.WorkspaceID == right.WorkspaceID && left.Action == right.Action && left.RuntimeID == right.RuntimeID && sameResource(left.ResourceID, right.ResourceID)
}
func sameBinding(left, right GateBinding) bool {
	return left.Caller == right.Caller && left.Reference == right.Reference && sameRequest(left.Request, right.Request)
}
func validRequest(request Request) bool {
	return validUUID(request.WorkspaceID) && validUUID(request.RuntimeID) && (request.ResourceID == nil || validUUID(*request.ResourceID)) && validAction(request.Action)
}
func validAction(action string) bool {
	switch action {
	case "PROFILE_READ", "PROFILE_WRITE", "WORKSPACE_READ", "WORKSPACE_MANAGE", "CONFIG_READ", "CONFIG_WRITE", "ACTIVATE", "CASE_READ", "CASE_CONTEXT_WRITE", "CASE_RECOVER", "OWNER_CLOSE", "EXPORT", "CONNECTOR_READ", "CONNECTOR_WRITE", "AI_PREPARE":
		return true
	}
	return false
}
func retained(action string) bool {
	return action != "CASE_CONTEXT_WRITE" && action != "OWNER_CLOSE" && action != "EXPORT"
}
func roleAllows(role, action string) bool {
	switch role {
	case "OWNER":
		return retained(action)
	case "OPERATOR":
		switch action {
		case "PROFILE_READ", "PROFILE_WRITE", "WORKSPACE_READ", "CASE_READ", "CASE_RECOVER":
			return true
		}
	case "READER":
		switch action {
		case "PROFILE_READ", "PROFILE_WRITE", "WORKSPACE_READ", "CONFIG_READ", "CASE_READ":
			return true
		}
	}
	return false
}

// These purpose groups are a conservative local composition mapping, not a new
// canonical operation-to-action table. Every positive case still requires the
// trusted owner proof for the exact retained operation and resource scope.
func operationMatches(proof Proof) bool {
	switch proof.Action {
	case "PROFILE_READ":
		return proof.Operation == "getProfile"
	case "PROFILE_WRITE":
		return proof.Operation == "updateProfile"
	case "WORKSPACE_READ":
		return proof.Operation == "listWorkspaces"
	case "WORKSPACE_MANAGE":
		return proof.Operation == "createWorkspace"
	case "CONFIG_READ":
		switch proof.Operation {
		case "listOperations", "getDraft", "getAdmissionGaps", "getResumeSnapshot":
			return true
		}
	case "CONFIG_WRITE":
		switch proof.Operation {
		case "saveDraft", "checkDraftReadiness", "rehearseDraft":
			return true
		}
	case "ACTIVATE":
		if proof.Operation == "activateOperation" {
			return proof.LifecycleAction == ""
		}
		if proof.Operation != "changeOperationLifecycle" {
			return false
		}
		switch proof.LifecycleAction {
		case "PAUSE", "PREPARE_RESUME", "ACCEPT_RESUME":
			return true
		}
		return false
	case "CASE_READ":
		switch proof.Operation {
		case "listCases", "getCase", "getCaseHistory", "getObservationEvidence":
			return true
		}
	case "CASE_RECOVER":
		return proof.Operation == "recoverCase" && proof.RecoveryAction == "RECHECK"
	case "CONNECTOR_READ":
		return proof.Operation == "listConnections" || proof.Operation == "listConnectionOptions"
	case "CONNECTOR_WRITE":
		return proof.Operation == "connectService"
	case "AI_PREPARE":
		return proof.Operation == "prepareAIProposal" || proof.Operation == "generateAIProposal"
	}
	return false
}
func validProof(proof Proof, actor Actor, request Request) bool {
	if (request.Action == "PROFILE_READ" || request.Action == "PROFILE_WRITE") && request.ResourceID != nil && *request.ResourceID != actor.PrincipalID {
		return false
	}
	if request.Action != "ACTIVATE" && proof.LifecycleAction != "" {
		return false
	}
	return proof.Verified && proof.PrincipalID == actor.PrincipalID && proof.WorkspaceID == request.WorkspaceID && proof.Action == request.Action && sameResource(proof.ResourceID, request.ResourceID) && operationMatches(proof) && (proof.BindingGrantID == "" || validUUID(proof.BindingGrantID)) && (proof.Action == "CASE_RECOVER" || proof.RecoveryAction == "")
}
func cancelled(original, current context.Context) bool {
	return original.Err() != nil || current == nil || current.Err() != nil
}

type snapshotLookup struct{ snapshot sessionpolicy.Snapshot }

func (s snapshotLookup) Lookup(context.Context, string) (sessionpolicy.Snapshot, error) {
	return s.snapshot, nil
}
func (s Service) authenticate(ctx context.Context, reference string, lookup authenticatesession.SessionLookup) error {
	consumer, err := authenticatesession.New(lookup, s.clock)
	if err != nil {
		return ErrUnavailable
	}
	result := consumer.Execute(ctx, reference)
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if !result.IsValid() {
		if result.State == "unavailable" {
			return ErrUnavailable
		}
		return ErrDenied
	}
	return nil
}
func (s Service) now(ctx context.Context) (time.Time, error) {
	observation, err := s.clock.Now(ctx)
	if ctx.Err() != nil {
		return time.Time{}, ErrCancelled
	}
	if err != nil || !observation.Usable() {
		return time.Time{}, ErrUnavailable
	}
	return observation.At.UTC(), nil
}
func validateSnapshot(snapshot Snapshot, request ReadRequest, caller Caller) (Actor, error) {
	session, principal, workspace, membership, runtime := snapshot.Session, snapshot.Principal, snapshot.Workspace, snapshot.Membership, snapshot.Runtime
	if !session.Found || !principal.Found || !workspace.Found || !membership.Found || !runtime.Found {
		return Actor{}, ErrDenied
	}
	if !validUUID(session.ID) || !validUUID(session.PrincipalID) || !validInteger(session.Epoch) || !session.Policy.Found || !validInteger(workspace.Epoch) || !validInteger(membership.Revision) {
		return Actor{}, ErrUnavailable
	}
	if principal.Disabled || principal.ID != session.PrincipalID || workspace.ID != request.WorkspaceID || membership.WorkspaceID != request.WorkspaceID || membership.PrincipalID != principal.ID || membership.Status != "ACTIVE" || membership.Revoked || runtime.Caller != caller || runtime.Status != "OPEN" {
		return Actor{}, ErrDenied
	}
	if workspace.Lifecycle != "ACTIVE" && workspace.Lifecycle != "ONBOARDING" {
		return Actor{}, ErrDenied
	}
	if membership.Role != "OWNER" && membership.Role != "OPERATOR" && membership.Role != "READER" {
		return Actor{}, ErrDenied
	}
	if request.BindingGrantID != "" {
		b := snapshot.Binding
		if !b.Found || b.GrantID != request.BindingGrantID || b.WorkspaceID != request.WorkspaceID || b.Status != "VERIFIED" {
			return Actor{}, ErrDenied
		}
		if !validInteger(b.Epoch) {
			return Actor{}, ErrUnavailable
		}
	} else if snapshot.Binding.Found {
		return Actor{}, ErrUnavailable
	}
	for _, revocation := range snapshot.Revocations {
		if !validInteger(revocation.Epoch) || !validUUID(revocation.SubjectID) || revocation.WorkspaceID != "" && revocation.WorkspaceID != request.WorkspaceID {
			return Actor{}, ErrUnavailable
		}
		relevant := false
		switch revocation.Scope {
		case "SESSION":
			relevant = revocation.SubjectID == session.ID
		case "WORKSPACE":
			if revocation.WorkspaceID == "" || revocation.SubjectID != request.WorkspaceID {
				return Actor{}, ErrUnavailable
			}
			relevant = true
		case "MEMBERSHIP": // No canonical membership UUID exists in this schema. Do not invent one.
			if revocation.WorkspaceID == "" {
				return Actor{}, ErrUnavailable
			}
			return Actor{}, ErrDenied
		case "CONNECTION":
			if revocation.WorkspaceID == "" {
				return Actor{}, ErrUnavailable
			}
			relevant = request.BindingGrantID != "" && revocation.SubjectID == request.BindingGrantID
		default:
			return Actor{}, ErrUnavailable
		}
		if !relevant {
			continue
		}
		switch revocation.Status {
		case "PENDING", "INCIDENT":
			return Actor{}, ErrDenied
		case "EFFECTIVE": // Exact current scope facts and deny history must be confirmed by Admission.
			if revocation.Scope == "SESSION" {
				return Actor{}, ErrDenied
			}
		default:
			return Actor{}, ErrUnavailable
		}
	}
	return Actor{PrincipalID: principal.ID, SessionID: session.ID, WorkspaceID: workspace.ID, Role: membership.Role}, nil
}
func applicableRevocations(snapshot Snapshot) []Revocation {
	var result []Revocation
	for _, revocation := range snapshot.Revocations {
		if revocation.Scope == "SESSION" && revocation.SubjectID == snapshot.Session.ID ||
			revocation.Scope == "WORKSPACE" && revocation.SubjectID == snapshot.Workspace.ID ||
			revocation.Scope == "MEMBERSHIP" && revocation.WorkspaceID == snapshot.Workspace.ID ||
			revocation.Scope == "CONNECTION" && snapshot.Binding.Found && revocation.SubjectID == snapshot.Binding.GrantID {
			result = append(result, revocation)
		}
	}
	return result
}
func scopeFacts(snapshot Snapshot) ScopeEpochs {
	facts := ScopeEpochs{PrincipalID: snapshot.Principal.ID, SessionID: snapshot.Session.ID, WorkspaceID: snapshot.Workspace.ID, SessionEpoch: snapshot.Session.Epoch, WorkspaceEpoch: snapshot.Workspace.Epoch, MembershipRevision: snapshot.Membership.Revision, Revocations: applicableRevocations(snapshot)}
	if snapshot.Binding.Found {
		facts.BindingGrantID = snapshot.Binding.GrantID
		facts.BindingEpoch = snapshot.Binding.Epoch
	}
	return facts
}

// Execute uses only the opaque session secret and trusted port observations. It
// grants no external action itself. A later real owner response/read/request
// still requires its own current shared gate; this local result is not a lease.
func (s Service) Execute(ctx context.Context, opaqueSession string, request Request) (Authorization, error) {
	if absent(s.lookup) || absent(s.reference) || absent(s.clock) || absent(s.caller) || absent(s.reader) || absent(s.owner) || absent(s.gate) {
		return Authorization{}, ErrConfiguration
	}
	if ctx == nil || len(opaqueSession) < 43 || len(opaqueSession) > 128 || !validRequest(request) {
		return Authorization{}, ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return Authorization{}, ErrCancelled
	}
	if !retained(request.Action) {
		return Authorization{}, ErrDenied
	}
	request = copyRequest(request)
	reference, err := s.reference.Reference(opaqueSession)
	if ctx.Err() != nil {
		return Authorization{}, ErrCancelled
	}
	if err != nil || !validReference(reference) {
		return Authorization{}, ErrDenied
	}
	caller, err := s.caller.Current(ctx)
	if ctx.Err() != nil {
		return Authorization{}, ErrCancelled
	}
	if err != nil {
		return Authorization{}, ErrUnavailable
	}
	if !validCaller(caller) || !strings.EqualFold(caller.RuntimeID, request.RuntimeID) {
		return Authorization{}, ErrDenied
	}
	caller.RuntimeID = strings.ToLower(caller.RuntimeID)
	caller.BootID = strings.ToLower(caller.BootID)
	binding := GateBinding{Caller: caller, Request: copyRequest(request), Reference: reference}
	var result Authorization
	var callbacks atomic.Int32
	err = s.gate.Within(ctx, binding, func(work context.Context, admission Admission) error {
		if callbacks.Add(1) != 1 {
			return ErrUnavailable
		}
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		if absent(admission) || !sameBinding(admission.Binding(), GateBinding{Caller: caller, Request: request, Reference: reference}) {
			return ErrUnavailable
		}
		if err := s.authenticate(work, reference, s.lookup); err != nil {
			return err
		}
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		read := ReadRequest{Reference: reference, WorkspaceID: request.WorkspaceID, RuntimeID: request.RuntimeID}
		snapshot, err := s.reader.Read(work, read)
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		if err != nil {
			return ErrUnavailable
		}
		actor, err := validateSnapshot(snapshot, read, caller)
		if err != nil {
			return err
		}
		if !roleAllows(actor.Role, request.Action) {
			return ErrDenied
		}
		if err = s.authenticate(work, reference, snapshotLookup{snapshot.Session.Policy}); err != nil {
			return err
		}
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		provedActor := actor
		proof, err := s.owner.Prove(work, provedActor, copyRequest(request))
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		if err != nil {
			return ErrUnavailable
		}
		if !validProof(proof, actor, request) || (request.Action == "CONNECTOR_READ" || request.Action == "CONNECTOR_WRITE") && proof.BindingGrantID == "" {
			return ErrDenied
		}
		// Every potentially blocking authority port is followed by a fresh IAM read;
		// waiting inside a gate never turns the earlier snapshot into a permanent grant.
		proof.BindingGrantID = strings.ToLower(proof.BindingGrantID)
		read.BindingGrantID = proof.BindingGrantID
		snapshot, err = s.reader.Read(work, read)
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		if err != nil {
			return ErrUnavailable
		}
		actor, err = validateSnapshot(snapshot, read, caller)
		if err != nil {
			return err
		}
		if actor != provedActor || !roleAllows(actor.Role, request.Action) || !validProof(proof, actor, request) {
			return ErrDenied
		}
		if err = s.authenticate(work, reference, snapshotLookup{snapshot.Session.Policy}); err != nil {
			return err
		}
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		epoch, err := admission.Confirm(work, scopeFacts(snapshot))
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		if err != nil || !validInteger(epoch) || epoch != snapshot.Workspace.Epoch {
			return ErrUnavailable
		}
		checkedAt, err := s.now(work)
		if err != nil {
			return err
		}
		if cancelled(ctx, work) {
			return ErrCancelled
		}
		// The confirmation port itself may take time. The response uses a fresh clock
		// and the same accepted policy, not a copied valid boolean from before that wait.
		if !sessionpolicy.Evaluate(snapshot.Session.Policy, sessionpolicy.Observation{At: checkedAt, Trusted: true}).IsValid() {
			return ErrDenied
		}
		result = Authorization{Allowed: true, Epoch: epoch, CheckedAt: checkedAt, PrincipalID: actor.PrincipalID, SessionID: actor.SessionID, WorkspaceID: actor.WorkspaceID, Role: actor.Role}
		return nil
	})
	if ctx.Err() != nil {
		return Authorization{}, ErrCancelled
	}
	if err != nil {
		if errors.Is(err, ErrDenied) {
			return Authorization{}, ErrDenied
		}
		if errors.Is(err, ErrCancelled) {
			return Authorization{}, ErrCancelled
		}
		return Authorization{}, ErrUnavailable
	}
	if callbacks.Load() != 1 || !result.Allowed {
		return Authorization{}, ErrUnavailable
	}
	return result, nil
}
