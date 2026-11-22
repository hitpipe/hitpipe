// Package revocation coordinates durable IAM denials for trusted SESSION and
// WORKSPACE subjects. Authentication and authority checks belong to its caller.
package revocation

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

type Status string

const (
	Pending   Status = "PENDING"
	Effective Status = "EFFECTIVE"
	Incident  Status = "INCIDENT"

	// Outcome deadline; retry and recovery never extend the stored deadline.
	Deadline = 5 * time.Second
)

var (
	ErrInvalid     = errors.New("revocation: invalid input")
	ErrConflict    = errors.New("revocation: conflicting operation")
	ErrNotFound    = errors.New("revocation: operation not found")
	ErrUncertain   = errors.New("revocation: commit outcome unknown")
	ErrUnavailable = errors.New("revocation: durable state unavailable")
)

type Intent struct {
	// The trusted caller retains ID across retries, including an uncertain commit.
	ID         string
	Subject    startgate.Subject
	ReasonCode string
}

type Participant struct {
	Identity               startgate.Identity
	CertificateFingerprint string
	AcknowledgedAt         *time.Time
	AppliedEpoch           uint64
}

type Record struct {
	ID                 string
	Subject            startgate.Subject
	Epoch              uint64
	AcceptedAt         time.Time
	DeadlineAt         time.Time
	Status             Status
	EffectiveAt        *time.Time
	DeadlineBreachedAt *time.Time
	IncidentID         *string
	ReasonCode         string
	Participants       []Participant
}

func (r Record) Command(identity startgate.Identity) startgate.BarrierCommand {
	return startgate.BarrierCommand{
		TargetRuntimeID: identity.RuntimeID, BootID: identity.BootID,
		Generation: identity.Generation, RevocationID: r.ID,
		Subject: cloneSubject(r.Subject), Epoch: r.Epoch, DeadlineAt: r.DeadlineAt,
	}
}

func (r Record) DeadlineBreached() bool {
	return r.DeadlineBreachedAt != nil
}

func (r Record) MissingRuntimeIDs() []string {
	missing := make([]string, 0, len(r.Participants))
	for _, p := range r.Participants {
		if p.AcknowledgedAt == nil || p.AppliedEpoch != r.Epoch {
			missing = append(missing, p.Identity.RuntimeID)
		}
	}
	sort.Strings(missing)
	return missing
}

type Store interface {
	Begin(context.Context, Intent, time.Time) (Record, error)
	Get(context.Context, string) (Record, error)
	Open(context.Context, string, int) ([]string, error)
	Acknowledge(context.Context, string, startgate.BarrierAck) error
	Finalize(context.Context, string, time.Time) (Record, error)
}

type Sender interface {
	// Send authenticates the exact runtime certificate associated with target.
	Send(context.Context, Participant, startgate.BarrierCommand) (startgate.BarrierAck, error)
}

type Coordinator struct {
	store  Store
	sender Sender
	clock  func() time.Time
}

func New(store Store, sender Sender, clock func() time.Time) (*Coordinator, error) {
	if store == nil || sender == nil {
		return nil, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	return &Coordinator{store: store, sender: sender, clock: clock}, nil
}

// Begin persists the deny, intent and mandatory roster atomically. It performs
// no network action, including when the commit response is uncertain.
func (c *Coordinator) Begin(ctx context.Context, want Intent) (Record, error) {
	if ctx == nil {
		return Record{}, ErrInvalid
	}
	if err := ValidateIntent(want); err != nil {
		return Record{}, err
	}
	want.Subject = cloneSubject(want.Subject)
	return c.store.Begin(ctx, want, NormalizeTime(c.clock()))
}

// Drive makes one delivery attempt per unconfirmed participant, independently
// so one unavailable runtime cannot hold up acknowledgements from the others.
// Neither a transport error nor a registry/lease state is a fencing proof.
func (c *Coordinator) Drive(ctx context.Context, id string) (Record, error) {
	if ctx == nil || !validUUID(id) {
		return Record{}, ErrInvalid
	}
	record, err := c.store.Finalize(ctx, id, NormalizeTime(c.clock()))
	if err != nil {
		return Record{}, err
	}
	if record.Status == Effective {
		return record, nil
	}
	var work sync.WaitGroup
	durableErrors := make(chan error, len(record.Participants))
	for _, participant := range record.Participants {
		if participant.AcknowledgedAt != nil || participant.CertificateFingerprint == "" {
			continue
		}
		work.Go(func() {
			if err := c.deliver(ctx, record, participant); err != nil {
				durableErrors <- err
			}
		})
	}
	work.Wait()
	close(durableErrors)
	latest, err := c.store.Finalize(ctx, id, NormalizeTime(c.clock()))
	if err != nil {
		return Record{}, err
	}
	if latest.Status == Effective {
		return latest, nil
	}
	for err := range durableErrors {
		if err != nil {
			return latest, err
		}
	}
	return latest, nil
}

func (c *Coordinator) deliver(ctx context.Context, record Record, participant Participant) error {
	attemptTime := record.DeadlineAt.Sub(c.clock())
	if attemptTime <= 0 {
		// The same still-running boot may replay an already-produced, on-time ACK.
		// It cannot produce a new ACK after the original deadline.
		attemptTime = time.Second
	}
	send, cancel := context.WithTimeout(ctx, min(attemptTime, 3*time.Second))
	command := record.Command(participant.Identity)
	ack, err := c.sender.Send(send, participant, command)
	cancel()
	identity := startgate.Identity{RuntimeID: ack.RuntimeID, BootID: ack.BootID, Generation: ack.Generation}
	if err != nil || identity != participant.Identity || ValidateAck(record, ack) != nil ||
		ack.AcknowledgedAt.After(c.clock()) {
		return nil
	}
	if err := c.store.Acknowledge(ctx, record.ID, ack); err != nil {
		return err
	}
	return nil
}

// Recover scans one page of unfinished work using a stable UUID cursor. Pass
// an empty cursor after restart and again when a page returns an empty cursor.
func (c *Coordinator) Recover(ctx context.Context, after string, limit int) (string, error) {
	if ctx == nil || (after != "" && !validUUID(after)) || limit < 1 || limit > 64 {
		return "", ErrInvalid
	}
	ids, err := c.store.Open(ctx, after, limit)
	if err != nil {
		return after, err
	}
	// Continue after a per-record storage error: the next scan can retry that
	// record, and one unavailable revocation cannot starve others in this page.
	var failures []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return after, errors.Join(append(failures, err)...)
		}
		if _, err := c.Drive(ctx, id); err != nil {
			failures = append(failures, err)
		}
		after = id
	}
	if len(ids) < limit {
		return "", errors.Join(failures...)
	}
	return after, errors.Join(failures...)
}

var (
	uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	codePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
)

func validUUID(s string) bool {
	return s != "00000000-0000-0000-0000-000000000000" && uuidPattern.MatchString(s)
}

func ValidateIntent(want Intent) error {
	if !validUUID(want.ID) || !validUUID(want.Subject.SubjectID) || !codePattern.MatchString(want.ReasonCode) {
		return ErrInvalid
	}
	switch want.Subject.Scope {
	case startgate.Session:
		if want.Subject.WorkspaceID != nil {
			return ErrInvalid
		}
	case startgate.Workspace:
		if want.Subject.WorkspaceID == nil || *want.Subject.WorkspaceID != want.Subject.SubjectID {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func NormalizeTime(t time.Time) time.Time {
	// PostgreSQL timestamps have microsecond resolution. Keep the epoch/deadline
	// identical on the initial send and every replay.
	return t.UTC().Truncate(time.Microsecond)
}

func ValidateAck(record Record, ack startgate.BarrierAck) error {
	if ack.RevocationID != record.ID || !sameSubject(ack.Subject, record.Subject) ||
		ack.AppliedEpoch != record.Epoch || !ack.OldStartSectionsDrained ||
		ack.AcknowledgedAt.IsZero() || ack.AcknowledgedAt.Before(record.AcceptedAt) ||
		!ack.AcknowledgedAt.Before(record.DeadlineAt) {
		return ErrInvalid
	}
	for _, participant := range record.Participants {
		if participant.Identity == (startgate.Identity{
			RuntimeID: ack.RuntimeID, BootID: ack.BootID, Generation: ack.Generation,
		}) {
			return nil
		}
	}
	return ErrInvalid
}

func sameSubject(a, b startgate.Subject) bool {
	if a.Scope != b.Scope || a.SubjectID != b.SubjectID || (a.WorkspaceID == nil) != (b.WorkspaceID == nil) {
		return false
	}
	return a.WorkspaceID == nil || *a.WorkspaceID == *b.WorkspaceID
}

func cloneSubject(s startgate.Subject) startgate.Subject {
	if s.WorkspaceID != nil {
		value := *s.WorkspaceID
		s.WorkspaceID = &value
	}
	return s
}
