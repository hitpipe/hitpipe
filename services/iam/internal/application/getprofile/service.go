package getprofile

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

var (
	ErrConfiguration   = errors.New("profile_configuration_invalid")
	ErrInvalidRequest  = errors.New("profile_request_invalid")
	ErrUnauthenticated = errors.New("profile_unauthenticated")
	ErrForbidden       = errors.New("profile_forbidden")
	ErrUnavailable     = errors.New("profile_unavailable")
	ErrCancelled       = errors.New("profile_cancelled")
)

// criticalBudget bounds admission waiting, protected reads and response start.
// It is the accepted A012 two-second critical-operation bound, not a cache TTL.
const criticalBudget = 2 * time.Second

type Service struct {
	lookup    authenticatesession.SessionLookup
	reference authorizeaction.ReferenceDeriver
	clock     authenticatesession.Clock
	reader    Reader
	content   Content
	gate      Gate
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
func New(lookup authenticatesession.SessionLookup, reference authorizeaction.ReferenceDeriver, clock authenticatesession.Clock, reader Reader, content Content, gate Gate) (Service, error) {
	if absent(lookup) || absent(reference) || absent(clock) || absent(reader) || absent(content) || absent(gate) {
		return Service{}, ErrConfiguration
	}
	return Service{lookup: lookup, reference: reference, clock: clock, reader: reader, content: content, gate: gate}, nil
}
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
	for _, v := range []byte(value) {
		if !(v >= '0' && v <= '9' || v >= 'a' && v <= 'f') {
			return false
		}
	}
	return true
}
func validDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validReference(strings.TrimPrefix(value, "sha256:"))
}
func validCaller(c authorizeaction.Caller) bool {
	return c.Service == "BFF" && validUUID(c.RuntimeID) && validUUID(c.BootID) && c.Generation > 0 && c.Generation <= authorizeaction.MaxWireInteger && validDigest(c.CertificateFingerprint)
}
func normalizeCaller(c authorizeaction.Caller) authorizeaction.Caller {
	c.RuntimeID = strings.ToLower(c.RuntimeID)
	c.BootID = strings.ToLower(c.BootID)
	return c
}
func stopped(original, current context.Context) error {
	if original.Err() != nil {
		return ErrCancelled
	}
	if current == nil || current.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
func validateSnapshot(snapshot Snapshot, caller authorizeaction.Caller) error {
	session, principal, runtime := snapshot.Session, snapshot.Principal, snapshot.Runtime
	if !session.Found || !session.Policy.Found || !principal.Found || principal.Disabled {
		return ErrUnauthenticated
	}
	if !validUUID(session.ID) || !validUUID(session.PrincipalID) || !validUUID(principal.ID) || !strings.EqualFold(principal.ID, session.PrincipalID) || session.Epoch < 1 || session.Epoch > authorizeaction.MaxWireInteger {
		return ErrUnavailable
	}
	if !runtime.Found || runtime.Status != "OPEN" || !validCaller(runtime.Caller) || normalizeCaller(runtime.Caller) != caller {
		return ErrForbidden
	}
	if !validUUID(snapshot.ProfileRef) || !validDigest(snapshot.ProfileDigest) {
		return ErrUnavailable
	}
	for _, revocation := range snapshot.Revocations {
		// A profile is global identity. Workspace/membership/connector grants do not
		// create authority for it; a SESSION deny cannot disappear by a tenant filter.
		switch revocation.Scope {
		case "WORKSPACE", "MEMBERSHIP", "CONNECTION":
			continue
		case "SESSION":
		default:
			return ErrUnavailable
		}
		if !validUUID(revocation.SubjectID) {
			return ErrUnavailable
		}
		if !strings.EqualFold(revocation.SubjectID, session.ID) {
			continue
		}
		if revocation.Epoch < 1 || revocation.Epoch > authorizeaction.MaxWireInteger || revocation.WorkspaceID != "" && !validUUID(revocation.WorkspaceID) {
			return ErrUnavailable
		}
		switch revocation.Status {
		case "PENDING", "INCIDENT", "EFFECTIVE":
			return ErrUnauthenticated
		default:
			return ErrUnavailable
		}
	}
	return nil
}
func samePolicy(left, right sessionpolicy.Snapshot) bool {
	return left.Found == right.Found && left.Revoked == right.Revoked && left.CreatedAt.Equal(right.CreatedAt) && left.ExpiresAt.Equal(right.ExpiresAt) && left.IdleExpiresAt.Equal(right.IdleExpiresAt)
}
func sameOwnerFacts(left, right Snapshot) bool {
	return strings.EqualFold(left.Session.ID, right.Session.ID) && strings.EqualFold(left.Session.PrincipalID, right.Session.PrincipalID) && left.Session.Epoch == right.Session.Epoch && samePolicy(left.Session.Policy, right.Session.Policy) && strings.EqualFold(left.Principal.ID, right.Principal.ID) && left.Principal.Disabled == right.Principal.Disabled && strings.EqualFold(left.ProfileRef, right.ProfileRef) && left.ProfileDigest == right.ProfileDigest && normalizeCaller(left.Runtime.Caller) == normalizeCaller(right.Runtime.Caller) && left.Runtime.Status == right.Runtime.Status
}
func validProfile(profile Profile, principal string) bool {
	return validUUID(profile.UserID) && strings.EqualFold(profile.UserID, principal) && utf8.ValidString(profile.DisplayName) && utf8.RuneCountInString(profile.DisplayName) >= 1 && utf8.RuneCountInString(profile.DisplayName) <= 120 && utf8.ValidString(profile.Timezone) && utf8.RuneCountInString(profile.Timezone) >= 1 && utf8.RuneCountInString(profile.Timezone) <= 80 && profile.Revision >= 1 && profile.Revision <= authorizeaction.MaxWireInteger
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
		return ErrUnavailable
	}
	if result.IsValid() {
		return nil
	}
	if result.State == "unavailable" {
		return ErrUnavailable
	}
	return ErrUnauthenticated
}
func currentFacts(snapshot Snapshot, caller authorizeaction.Caller) Facts {
	var revocations []authorizeaction.Revocation
	for _, revocation := range snapshot.Revocations {
		if revocation.Scope == "SESSION" && strings.EqualFold(revocation.SubjectID, snapshot.Session.ID) {
			revocations = append(revocations, revocation)
		}
	}
	return Facts{SessionID: strings.ToLower(snapshot.Session.ID), PrincipalID: strings.ToLower(snapshot.Principal.ID), ProfileRef: strings.ToLower(snapshot.ProfileRef), ProfileDigest: snapshot.ProfileDigest, SessionEpoch: snapshot.Session.Epoch, Caller: caller, Revocations: revocations}
}

// Serve emits only the session owner's verified profile, exactly once, while
// the trusted actual-response-start gate remains held. Ports must honor context;
// emit must be a bounded response writer and must not launch detached work.
// Once emit has been invoked its bytes cannot be rolled back. Any subsequent
// error is diagnostic only: the transport must not retry or send another reply.
func (s Service) Serve(ctx context.Context, opaqueSession string, caller authorizeaction.Caller, emit func(Profile) error) error {
	if emit == nil {
		return ErrInvalidRequest
	}
	return s.ServeWithContext(ctx, opaqueSession, caller, func(_ context.Context, profile Profile) error {
		return emit(profile)
	})
}

// ServeWithContext passes the actual start guard's expiration and cancellation
// to a transport that can enforce them during its first write and flush.
func (s Service) ServeWithContext(ctx context.Context, opaqueSession string, caller authorizeaction.Caller, emit func(context.Context, Profile) error) error {
	if absent(s.lookup) || absent(s.reference) || absent(s.clock) || absent(s.reader) || absent(s.content) || absent(s.gate) {
		return ErrConfiguration
	}
	if ctx == nil || emit == nil {
		return ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if len(opaqueSession) < 43 || len(opaqueSession) > 128 {
		return ErrUnauthenticated
	}
	if !validCaller(caller) {
		return ErrForbidden
	}
	caller = normalizeCaller(caller)
	bounded, cancel := context.WithTimeout(ctx, criticalBudget)
	defer cancel()
	reference, err := s.reference.Reference(opaqueSession)
	if errStopped := stopped(ctx, bounded); errStopped != nil {
		return errStopped
	}
	if err != nil || !validReference(reference) {
		return ErrUnauthenticated
	}
	binding := Binding{Caller: caller, Reference: reference}
	var callbacks atomic.Int32
	var emitted, completed atomic.Bool
	err = s.gate.Within(bounded, binding, func(work context.Context, admission Admission) error {
		if callbacks.Add(1) != 1 {
			return ErrUnavailable
		}
		if err := stopped(ctx, bounded); err != nil {
			return err
		}
		if err := stopped(ctx, work); err != nil {
			return err
		}
		workDeadline, workBounded := work.Deadline()
		deadline, _ := bounded.Deadline()
		if !workBounded || workDeadline.After(deadline) {
			return ErrUnavailable
		}
		if absent(admission) || admission.Binding() != binding {
			return ErrUnavailable
		}
		if err := s.authenticate(work, reference, s.lookup); err != nil {
			return err
		}
		if err := stopped(ctx, bounded); err != nil {
			return err
		}
		before, err := s.reader.Read(work, reference, caller)
		if errStopped := stopped(ctx, work); errStopped != nil {
			return errStopped
		}
		if errStopped := stopped(ctx, bounded); errStopped != nil {
			return errStopped
		}
		if err != nil {
			return ErrUnavailable
		}
		if err = validateSnapshot(before, caller); err != nil {
			return err
		}
		if err = s.authenticate(work, reference, snapshotLookup{before.Session.Policy}); err != nil {
			return err
		}
		profile, err := s.content.Read(work, strings.ToLower(before.ProfileRef), before.ProfileDigest)
		if errStopped := stopped(ctx, work); errStopped != nil {
			return errStopped
		}
		if errStopped := stopped(ctx, bounded); errStopped != nil {
			return errStopped
		}
		if err != nil || !validProfile(profile, before.Principal.ID) {
			return ErrUnavailable
		}
		after, err := s.reader.Read(work, reference, caller)
		if errStopped := stopped(ctx, work); errStopped != nil {
			return errStopped
		}
		if errStopped := stopped(ctx, bounded); errStopped != nil {
			return errStopped
		}
		if err != nil {
			return ErrUnavailable
		}
		if err = validateSnapshot(after, caller); err != nil {
			return err
		}
		if !sameOwnerFacts(before, after) {
			return ErrUnavailable
		}
		if err = s.authenticate(work, reference, snapshotLookup{after.Session.Policy}); err != nil {
			return err
		}
		if err := stopped(ctx, bounded); err != nil {
			return err
		}
		facts := currentFacts(after, caller)
		err = admission.Confirm(work, facts)
		if errStopped := stopped(ctx, work); errStopped != nil {
			return errStopped
		}
		if errStopped := stopped(ctx, bounded); errStopped != nil {
			return errStopped
		}
		if err != nil {
			return ErrUnavailable
		}
		observation, err := s.clock.Now(work)
		if errStopped := stopped(ctx, work); errStopped != nil {
			return errStopped
		}
		if errStopped := stopped(ctx, bounded); errStopped != nil {
			return errStopped
		}
		if err != nil || !observation.Usable() {
			return ErrUnavailable
		}
		if !sessionpolicy.Evaluate(after.Session.Policy, observation).IsValid() {
			return ErrUnauthenticated
		}
		profile.UserID = strings.ToLower(after.Principal.ID)
		start := func(guarded context.Context) error {
			if err := stopped(ctx, guarded); err != nil {
				return err
			}
			if !emitted.CompareAndSwap(false, true) {
				return ErrUnavailable
			}
			err := emit(guarded, profile)
			if errStopped := stopped(ctx, bounded); errStopped != nil {
				return errStopped
			}
			if err != nil {
				return ErrUnavailable
			}
			completed.Store(true)
			return nil
		}
		if atomicStart, ok := admission.(StartAdmission); ok {
			return atomicStart.Start(work, facts, start)
		}
		return start(work)
	})
	if errStopped := stopped(ctx, bounded); errStopped != nil {
		return errStopped
	}
	if err != nil {
		switch {
		case errors.Is(err, ErrUnauthenticated):
			return ErrUnauthenticated
		case errors.Is(err, ErrForbidden):
			return ErrForbidden
		case errors.Is(err, ErrCancelled):
			return ErrCancelled
		default:
			return ErrUnavailable
		}
	}
	if callbacks.Load() != 1 || !completed.Load() {
		return ErrUnavailable
	}
	return nil
}
