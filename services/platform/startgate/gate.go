// Package startgate serializes the actual beginning of protected work with a
// local revocation barrier. It is not an authorization service or coordinator.
//
// Owners must share one Gate across every protected start in a process, supply
// all applicable subjects from trusted current authority, and execute the actual
// start synchronously inside Start's callback. The callback must be short,
// bounded, and cooperative with its context. It must not enqueue an unguarded
// future start, reenter this Gate, or keep the lock for a request's whole lifetime.
// The package cannot cancel arbitrary code or qualify a network transport.
package startgate

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"time"
)

const MaxSafeInteger uint64 = 9007199254740991

type State string

const (
	Closed  State = "CLOSED"
	Syncing State = "SYNCING"
	Open    State = "OPEN"
)

type Scope string

const (
	Session    Scope = "SESSION"
	Membership Scope = "MEMBERSHIP"
	Connection Scope = "CONNECTION"
	Workspace  Scope = "WORKSPACE"
)

var (
	ErrInvalid       = errors.New("startgate: invalid input")
	ErrIdentity      = errors.New("startgate: runtime identity mismatch")
	ErrStaleSnapshot = errors.New("startgate: stale snapshot")
	ErrConflict      = errors.New("startgate: conflicting replay")
	ErrClosed        = errors.New("startgate: gate is not open")
	ErrDenied        = errors.New("startgate: authority denied or revoked")
	ErrUninitialized = errors.New("startgate: uninitialized gate")
)

type Identity struct {
	RuntimeID  string
	BootID     string
	Generation uint64
}

// Subject is a revocation authority key. A nil workspace means global, not an
// empty UUID. A global denial matches this scope/subject in every workspace.
type Subject struct {
	WorkspaceID *string
	Scope       Scope
	SubjectID   string
}

// Authority is owner-provided trusted input, not a grant minted by this package.
// Epoch alone never grants access. The owner must supply every applicable
// SESSION, MEMBERSHIP, CONNECTION and WORKSPACE key together in one Start.
type Authority struct {
	Subject           Subject
	Epoch             uint64
	Allowed           bool
	RevocationPending bool
}

type StartRequest struct {
	Authorities []Authority
}

type Denial struct {
	Subject      Subject
	Epoch        uint64
	RevocationID string
	AcceptedAt   time.Time
	EffectiveAt  *time.Time
}

type RuntimeSnapshot struct {
	RuntimeID        string
	BootID           string
	Generation       uint64
	SnapshotRevision uint64
	Gate             State
	Denials          []Denial
}

type BarrierCommand struct {
	TargetRuntimeID string
	BootID          string
	Generation      uint64
	RevocationID    string
	Subject         Subject
	Epoch           uint64
	DeadlineAt      time.Time
}

type BarrierAck struct {
	RevocationID            string
	RuntimeID               string
	BootID                  string
	Subject                 Subject
	Generation              uint64
	AppliedEpoch            uint64
	OldStartSectionsDrained bool
	AcknowledgedAt          time.Time
}

type subjectKey struct {
	workspace string
	scope     Scope
	subject   string
}

type revocation struct {
	key   subjectKey
	epoch uint64
}

type appliedBarrier struct {
	command BarrierCommand
	ack     BarrierAck
}

// Gate's zero value fails closed. Construct it with New and never copy it.
// Denials are sticky for this object's lifetime. Regrant and coordinator recovery
// require separate owner wiring; a new epoch or newer snapshot does not erase a
// known revocation. A new boot must use a new Gate and fresh registration.
// A valid local barrier whose wait is interrupted also sets a sticky process
// hold. This prevents starts after a noncooperative section eventually exits,
// without pretending that section was drained or that the barrier was applied.
// Only a new Gate and fresh registration can recover from that conservative
// hold; an OPEN snapshot cannot clear it.
type Gate struct {
	lock        chan struct{}
	identity    Identity
	hold        atomic.Bool
	state       State
	snapshot    RuntimeSnapshot
	denials     map[subjectKey]uint64
	revocations map[string]revocation
	barriers    map[string]appliedBarrier
}

func New(identity Identity) (*Gate, error) {
	identity, err := normalizeIdentity(identity)
	if err != nil {
		return nil, err
	}
	g := &Gate{
		lock: make(chan struct{}, 1), identity: identity, state: Closed,
		denials: make(map[subjectKey]uint64), revocations: make(map[string]revocation),
		barriers: make(map[string]appliedBarrier),
	}
	g.lock <- struct{}{}
	return g, nil
}

func (g *Gate) acquire(ctx context.Context) error {
	if g == nil || g.lock == nil {
		return ErrUninitialized
	}
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.lock:
		// If token and cancellation became ready together, no operation starts.
		if err := ctx.Err(); err != nil {
			g.release()
			return err
		}
		return nil
	}
}

func (g *Gate) release() { g.lock <- struct{}{} }

func (g *Gate) State(ctx context.Context) (State, error) {
	if err := g.acquire(ctx); err != nil {
		return Closed, err
	}
	defer g.release()
	if g.hold.Load() {
		return Closed, nil
	}
	return g.state, nil
}

// ApplySnapshot validates the complete snapshot before changing anything. It
// accepts an identical replay, rejects old/conflicting revisions, and merges
// denials monotonically. Snapshot omission never removes a known denial.
func (g *Gate) ApplySnapshot(ctx context.Context, snapshot RuntimeSnapshot) error {
	normalized, err := normalizeSnapshot(snapshot)
	if err != nil {
		return err
	}
	if err := g.acquire(ctx); err != nil {
		return err
	}
	defer g.release()
	if (Identity{normalized.RuntimeID, normalized.BootID, normalized.Generation}) != g.identity {
		return ErrIdentity
	}
	if normalized.SnapshotRevision < g.snapshot.SnapshotRevision {
		return ErrStaleSnapshot
	}
	if normalized.SnapshotRevision == g.snapshot.SnapshotRevision {
		if reflect.DeepEqual(normalized, g.snapshot) {
			return nil
		}
		return ErrConflict
	}
	for _, denial := range normalized.Denials {
		key, _ := normalizeSubject(denial.Subject)
		if previous, ok := g.revocations[denial.RevocationID]; ok && previous != (revocation{key, denial.Epoch}) {
			return ErrConflict
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, denial := range normalized.Denials {
		key, _ := normalizeSubject(denial.Subject)
		g.mergeDenial(key, denial.Epoch, denial.RevocationID)
	}
	g.snapshot = normalized
	g.state = normalized.Gate
	return nil
}

// Start calls actualStart under the SAME exclusive lock used by ApplyBarrier.
// No permit escapes the lock. The callback must synchronously cross the actual
// start boundary before returning; already started work may finish afterwards.
// A callback panic propagates after releasing the lock. Callback errors do not
// imply that an external effect did not begin and must not trigger blind retries.
func (g *Gate) Start(ctx context.Context, request StartRequest, actualStart func(context.Context) error) error {
	authorities, err := normalizeAuthorities(request.Authorities)
	if err != nil {
		return err
	}
	if actualStart == nil {
		return ErrInvalid
	}
	if g != nil && g.hold.Load() {
		return ErrClosed
	}
	if err := g.acquire(ctx); err != nil {
		return err
	}
	defer g.release()
	if g.hold.Load() || g.state != Open {
		return ErrClosed
	}
	for key, authority := range authorities {
		if !authority.Allowed || authority.RevocationPending {
			return ErrDenied
		}
		if _, denied := g.denials[key]; denied {
			return ErrDenied
		}
		global := key
		global.workspace = ""
		if _, denied := g.denials[global]; denied {
			return ErrDenied
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return actualStart(ctx)
}

// ApplyBarrier waits for the previous actual-start section to leave, installs a
// sticky denial and only then returns ACK. Deadline/cancellation while waiting
// returns no ACK and never releases a lock owned by somebody else's callback.
// Arbitrary noncooperative callbacks cannot be force-cancelled safely; runtime
// qualification must enforce their documented bounded-start precondition.
func (g *Gate) ApplyBarrier(ctx context.Context, command BarrierCommand) (BarrierAck, error) {
	command, key, err := normalizeBarrier(command)
	if err != nil {
		return BarrierAck{}, err
	}
	if ctx == nil {
		return BarrierAck{}, ErrInvalid
	}
	if g == nil || g.lock == nil {
		return BarrierAck{}, ErrUninitialized
	}
	// Identity is immutable. Reject a foreign runtime before a timeout can
	// close this process. Invalid/foreign input is never a local revocation.
	if (Identity{command.TargetRuntimeID, command.BootID, command.Generation}) != g.identity {
		return BarrierAck{}, ErrIdentity
	}
	bounded := ctx
	cancel := func() {}
	if command.DeadlineAt.After(time.Now()) {
		bounded, cancel = context.WithDeadline(ctx, command.DeadlineAt)
	}
	defer cancel()
	// An already expired command may only obtain an immediately available lock
	// to recover its original ACK. It cannot queue unbounded work or manufacture
	// a late ACK. This keeps lost-response replay useful without moving deadlines.
	if !command.DeadlineAt.After(time.Now()) {
		err = g.acquireExpiredReplay(ctx)
	} else {
		err = g.acquire(bounded)
	}
	if err != nil {
		// We have a valid exact-runtime revocation but cannot safely drain the
		// section. Prevent all later starts without taking or releasing its
		// lock. This is a conservative local hold, never an ACK or fencing proof.
		g.hold.Store(true)
		return BarrierAck{}, err
	}
	defer g.release()
	if (Identity{command.TargetRuntimeID, command.BootID, command.Generation}) != g.identity {
		return BarrierAck{}, ErrIdentity
	}
	if previous, ok := g.barriers[command.RevocationID]; ok {
		if sameBarrier(previous.command, command) {
			return copyAck(previous.ack), nil
		}
		return BarrierAck{}, ErrConflict
	}
	if previous, ok := g.revocations[command.RevocationID]; ok && previous != (revocation{key, command.Epoch}) {
		return BarrierAck{}, ErrConflict
	}
	g.mergeDenial(key, command.Epoch, command.RevocationID)
	if err := bounded.Err(); err != nil {
		return BarrierAck{}, err
	}
	acknowledgedAt := time.Now().UTC()
	if !acknowledgedAt.Before(command.DeadlineAt) {
		return BarrierAck{}, context.DeadlineExceeded
	}
	ack := BarrierAck{
		RevocationID: command.RevocationID, RuntimeID: g.identity.RuntimeID,
		BootID: g.identity.BootID, Generation: g.identity.Generation,
		Subject: copySubject(command.Subject), AppliedEpoch: g.denials[key],
		OldStartSectionsDrained: true, AcknowledgedAt: acknowledgedAt,
	}
	g.barriers[command.RevocationID] = appliedBarrier{command, ack}
	return copyAck(ack), nil
}

func (g *Gate) mergeDenial(key subjectKey, epoch uint64, id string) {
	if epoch > g.denials[key] {
		g.denials[key] = epoch
	}
	g.revocations[id] = revocation{key, epoch}
}

func (g *Gate) acquireExpiredReplay(ctx context.Context) error {
	if g == nil || g.lock == nil {
		return ErrUninitialized
	}
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-g.lock:
		if err := ctx.Err(); err != nil {
			g.release()
			return err
		}
		return nil
	default:
		return context.DeadlineExceeded
	}
}

func sameBarrier(a, b BarrierCommand) bool {
	ak, _ := normalizeSubject(a.Subject)
	bk, _ := normalizeSubject(b.Subject)
	return a.TargetRuntimeID == b.TargetRuntimeID && a.BootID == b.BootID &&
		a.Generation == b.Generation && a.RevocationID == b.RevocationID &&
		ak == bk && a.Epoch == b.Epoch && a.DeadlineAt.Equal(b.DeadlineAt)
}

func copySubject(s Subject) Subject {
	if s.WorkspaceID != nil {
		workspace := *s.WorkspaceID
		s.WorkspaceID = &workspace
	}
	return s
}

func copyAck(ack BarrierAck) BarrierAck { ack.Subject = copySubject(ack.Subject); return ack }

func normalizeIdentity(identity Identity) (Identity, error) {
	var ok bool
	identity.RuntimeID, ok = uuid(identity.RuntimeID)
	if !ok {
		return Identity{}, ErrInvalid
	}
	identity.BootID, ok = uuid(identity.BootID)
	if !ok || !safeInteger(identity.Generation) {
		return Identity{}, ErrInvalid
	}
	return identity, nil
}

func normalizeSubject(subject Subject) (subjectKey, error) {
	id, ok := uuid(subject.SubjectID)
	if !ok {
		return subjectKey{}, ErrInvalid
	}
	switch subject.Scope {
	case Session, Membership, Connection, Workspace:
	default:
		return subjectKey{}, ErrInvalid
	}
	key := subjectKey{scope: subject.Scope, subject: id}
	if subject.WorkspaceID != nil {
		workspace, ok := uuid(*subject.WorkspaceID)
		if !ok {
			return subjectKey{}, ErrInvalid
		}
		key.workspace = workspace
	}
	return key, nil
}

func subjectFromKey(key subjectKey) Subject {
	s := Subject{Scope: key.scope, SubjectID: key.subject}
	if key.workspace != "" {
		workspace := key.workspace
		s.WorkspaceID = &workspace
	}
	return s
}

func normalizeAuthorities(authorities []Authority) (map[subjectKey]Authority, error) {
	if len(authorities) == 0 {
		return nil, ErrInvalid
	}
	result := make(map[subjectKey]Authority, len(authorities))
	for _, authority := range authorities {
		key, err := normalizeSubject(authority.Subject)
		if err != nil || !safeInteger(authority.Epoch) {
			return nil, ErrInvalid
		}
		authority.Subject = subjectFromKey(key)
		if previous, ok := result[key]; ok && (previous.Epoch != authority.Epoch ||
			previous.Allowed != authority.Allowed || previous.RevocationPending != authority.RevocationPending) {
			return nil, ErrConflict
		}
		result[key] = authority
	}
	return result, nil
}

func normalizeSnapshot(snapshot RuntimeSnapshot) (RuntimeSnapshot, error) {
	identity, err := normalizeIdentity(Identity{snapshot.RuntimeID, snapshot.BootID, snapshot.Generation})
	if err != nil || !safeInteger(snapshot.SnapshotRevision) || len(snapshot.Denials) > 10000 {
		return RuntimeSnapshot{}, ErrInvalid
	}
	switch snapshot.Gate {
	case Closed, Syncing, Open:
	default:
		return RuntimeSnapshot{}, ErrInvalid
	}
	snapshot.RuntimeID, snapshot.BootID = identity.RuntimeID, identity.BootID
	denials := make([]Denial, 0, len(snapshot.Denials))
	seen := make(map[string]Denial, len(snapshot.Denials))
	for _, denial := range snapshot.Denials {
		key, err := normalizeSubject(denial.Subject)
		id, ok := uuid(denial.RevocationID)
		if err != nil || !ok || !safeInteger(denial.Epoch) || !validTime(denial.AcceptedAt) {
			return RuntimeSnapshot{}, ErrInvalid
		}
		denial.Subject, denial.RevocationID = subjectFromKey(key), id
		denial.AcceptedAt = denial.AcceptedAt.UTC()
		if denial.EffectiveAt != nil {
			if !validTime(*denial.EffectiveAt) || denial.EffectiveAt.Before(denial.AcceptedAt) {
				return RuntimeSnapshot{}, ErrInvalid
			}
			effective := denial.EffectiveAt.UTC()
			denial.EffectiveAt = &effective
		}
		if previous, exists := seen[id]; exists {
			if !reflect.DeepEqual(previous, denial) {
				return RuntimeSnapshot{}, ErrConflict
			}
			continue
		}
		seen[id] = denial
		denials = append(denials, denial)
	}
	snapshot.Denials = denials
	return snapshot, nil
}

func normalizeBarrier(command BarrierCommand) (BarrierCommand, subjectKey, error) {
	identity, err := normalizeIdentity(Identity{command.TargetRuntimeID, command.BootID, command.Generation})
	key, keyErr := normalizeSubject(command.Subject)
	id, ok := uuid(command.RevocationID)
	if err != nil || keyErr != nil || !ok || !safeInteger(command.Epoch) || !validTime(command.DeadlineAt) {
		return BarrierCommand{}, subjectKey{}, ErrInvalid
	}
	command.TargetRuntimeID, command.BootID = identity.RuntimeID, identity.BootID
	command.Subject, command.RevocationID = subjectFromKey(key), id
	command.DeadlineAt = command.DeadlineAt.UTC()
	return command, key, nil
}

func safeInteger(value uint64) bool { return value >= 1 && value <= MaxSafeInteger }

func validTime(value time.Time) bool {
	return !value.IsZero() && value.Year() >= 1 && value.Year() <= 9999
}

// uuid accepts the canonical UUID spelling (case-insensitive), including nil
// UUIDs allowed by the OAS format. It never treats an empty string as null.
func uuid(value string) (string, bool) {
	if len(value) != 36 {
		return "", false
	}
	for i := range value {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if value[i] != '-' {
				return "", false
			}
			continue
		}
		c := value[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return "", false
		}
	}
	return strings.ToLower(value), true
}
