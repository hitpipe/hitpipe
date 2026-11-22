package revocation

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

const (
	revID     = "1926f8de-6f23-4cfe-8f00-884c3087bc01"
	subjectID = "1926f8de-6f23-4cfe-8f00-884c3087bc02"
	runtime1  = "1926f8de-6f23-4cfe-8f00-884c3087bc03"
	runtime2  = "1926f8de-6f23-4cfe-8f00-884c3087bc04"
	boot1     = "1926f8de-6f23-4cfe-8f00-884c3087bc05"
	boot2     = "1926f8de-6f23-4cfe-8f00-884c3087bc06"
)

var startTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func sessionIntent() Intent {
	return Intent{ID: revID, Subject: startgate.Subject{Scope: startgate.Session, SubjectID: subjectID},
		ReasonCode: "USER_SESSION_REVOKED"}
}

func sampleRecord() Record {
	return Record{
		ID: revID, Subject: sessionIntent().Subject, AcceptedAt: startTime, DeadlineAt: startTime.Add(Deadline),
		Epoch: 2, Status: Pending, Participants: []Participant{
			{Identity: startgate.Identity{RuntimeID: runtime1, BootID: boot1, Generation: 1}, CertificateFingerprint: "pin"},
			{Identity: startgate.Identity{RuntimeID: runtime2, BootID: boot2, Generation: 2}, CertificateFingerprint: "pin"},
		},
	}
}

func validAck(command startgate.BarrierCommand) startgate.BarrierAck {
	return startgate.BarrierAck{
		RevocationID: command.RevocationID, RuntimeID: command.TargetRuntimeID, BootID: command.BootID,
		Generation: command.Generation, AppliedEpoch: command.Epoch, Subject: command.Subject,
		OldStartSectionsDrained: true, AcknowledgedAt: command.DeadlineAt.Add(-time.Second),
	}
}

type testStore struct {
	Store
	begin       func(Intent, time.Time) (Record, error)
	finalize    func(string, time.Time) (Record, error)
	acknowledge func(startgate.BarrierAck) error
	open        func(string, int) ([]string, error)
}

func (s testStore) Begin(_ context.Context, i Intent, t time.Time) (Record, error) {
	return s.begin(i, t)
}
func (s testStore) Finalize(_ context.Context, id string, t time.Time) (Record, error) {
	return s.finalize(id, t)
}
func (s testStore) Acknowledge(_ context.Context, _ string, ack startgate.BarrierAck) error {
	return s.acknowledge(ack)
}
func (s testStore) Open(_ context.Context, after string, limit int) ([]string, error) {
	return s.open(after, limit)
}

type testSender func(context.Context, Participant, startgate.BarrierCommand) (startgate.BarrierAck, error)

func (s testSender) Send(ctx context.Context, p Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
	return s(ctx, p, cmd)
}

func TestUncertainBeginDoesNotDeliverAndSameIntentCanResume(t *testing.T) {
	record := sampleRecord()
	var begins int
	var sends atomic.Int32
	store := testStore{
		begin: func(want Intent, at time.Time) (Record, error) {
			if want != sessionIntent() || !at.Equal(startTime) {
				t.Errorf("different persisted intent: %+v; time=%v", want, at)
			}
			begins++
			if begins == 1 {
				return Record{}, ErrUncertain
			}
			return record, nil
		},
		finalize: func(id string, _ time.Time) (Record, error) {
			if id != record.ID {
				t.Errorf("wrong id for durable state lookup: %s", id)
			}
			return record, nil
		},
	}
	sender := testSender(func(_ context.Context, p Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
		if begins != 2 || cmd != record.Command(p.Identity) {
			t.Errorf("noncanonical delivery: %+v", cmd)
		}
		sends.Add(1)
		return startgate.BarrierAck{}, errors.New("offline")
	})
	c, err := New(store, sender, func() time.Time { return startTime })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Begin(t.Context(), sessionIntent()); !errors.Is(err, ErrUncertain) {
		t.Fatalf("begin: %v", err)
	}
	if got, err := c.Begin(t.Context(), sessionIntent()); err != nil || got.ID != record.ID || got.Epoch != record.Epoch {
		t.Fatalf("retry: %+v, %v", got, err)
	}
	if sends.Load() != 0 {
		t.Fatal("Begin performed network delivery")
	}
	if got, err := c.Drive(t.Context(), record.ID); err != nil || got.Status != Pending || sends.Load() != 2 {
		t.Fatalf("drive: %+v, %v, sent=%d", got, err, sends.Load())
	}
}

func TestValidAckPersistsWhileOtherTargetIsUnavailable(t *testing.T) {
	record := sampleRecord()
	var clock atomic.Int64
	clock.Store(startTime.Add(4 * time.Second).UnixNano())
	validPersisted := make(chan struct{})
	var stored []startgate.BarrierAck
	store := testStore{
		finalize: func(_ string, _ time.Time) (Record, error) { return record, nil },
		acknowledge: func(ack startgate.BarrierAck) error {
			stored = append(stored, ack)
			close(validPersisted)
			return nil
		},
	}
	sender := testSender(func(ctx context.Context, p Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
		if p.Identity.RuntimeID == runtime1 {
			return validAck(cmd), nil
		}
		select {
		case <-ctx.Done():
			t.Error("a slow target blocked persistence of another target's ACK")
			return startgate.BarrierAck{}, ctx.Err()
		case <-validPersisted:
			// A valid ACK for a different participant must not count as this response.
			return validAck(record.Command(record.Participants[0].Identity)), nil
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	c, _ := New(store, sender, func() time.Time { return time.Unix(0, clock.Load()) })
	if got, err := c.Drive(ctx, record.ID); err != nil || got.Status != Pending {
		t.Fatalf("driving with partial ACK: %+v, %v", got, err)
	}
	if len(stored) != 1 || stored[0].RuntimeID != runtime1 {
		t.Fatalf("persisted unexpected ACKs: %+v", stored)
	}
}

func TestAckValidation(t *testing.T) {
	record := sampleRecord()
	ack := validAck(record.Command(record.Participants[0].Identity))
	if err := ValidateAck(record, ack); err != nil {
		t.Fatal(err)
	}
	changes := map[string]func(*startgate.BarrierAck){
		"revocation": func(a *startgate.BarrierAck) { a.RevocationID = subjectID },
		"boot":       func(a *startgate.BarrierAck) { a.BootID = boot2 },
		"scope":      func(a *startgate.BarrierAck) { a.Subject.Scope = startgate.Workspace },
		"subject":    func(a *startgate.BarrierAck) { a.Subject.SubjectID = boot1 },
		"workspace":  func(a *startgate.BarrierAck) { w := subjectID; a.Subject.WorkspaceID = &w },
		"generation": func(a *startgate.BarrierAck) { a.Generation++ },
		"epoch":      func(a *startgate.BarrierAck) { a.AppliedEpoch++ },
		"drained":    func(a *startgate.BarrierAck) { a.OldStartSectionsDrained = false },
		"premature":  func(a *startgate.BarrierAck) { a.AcknowledgedAt = record.AcceptedAt.Add(-time.Nanosecond) },
		"expired":    func(a *startgate.BarrierAck) { a.AcknowledgedAt = record.DeadlineAt },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			altered := ack
			change(&altered)
			if err := ValidateAck(record, altered); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted different %s", name)
			}
		})
	}
}

func TestRecoveryRespectsRosterAndOnlyRetriesMissingParticipant(t *testing.T) {
	record := sampleRecord()
	ack := validAck(record.Command(record.Participants[0].Identity))
	record.Participants[0].AcknowledgedAt = &ack.AcknowledgedAt
	record.Participants[0].AppliedEpoch = ack.AppliedEpoch
	record.Status = Incident
	breach := record.DeadlineAt
	record.DeadlineBreachedAt = &breach
	var mu sync.Mutex
	var final bool
	store := testStore{
		open: func(after string, limit int) ([]string, error) {
			if after != "" || limit != 8 {
				t.Errorf("wrong cursor %q/%d", after, limit)
			}
			return []string{record.ID}, nil
		},
		finalize: func(_ string, _ time.Time) (Record, error) {
			mu.Lock()
			defer mu.Unlock()
			out := record
			if final {
				out.Status = Effective
			}
			return out, nil
		},
		acknowledge: func(a startgate.BarrierAck) error {
			mu.Lock()
			defer mu.Unlock()
			if a.RuntimeID != runtime2 {
				t.Errorf("confirmed participant was retried: %s", a.RuntimeID)
			}
			final = true
			return nil
		},
	}
	c, _ := New(store, testSender(func(_ context.Context, p Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
		if p.Identity.RuntimeID != runtime2 || cmd != record.Command(record.Participants[1].Identity) {
			t.Errorf("recovery diverged from the stored command: %+v", cmd)
		}
		return validAck(cmd), nil
	}), func() time.Time { return breach.Add(time.Second) })
	if next, err := c.Recover(t.Context(), "", 8); err != nil || next != "" || !final {
		t.Fatalf("recovery did not persist the original timely ACK: next=%q err=%v final=%t", next, err, final)
	}
}
