package startgate_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

const (
	unitRuntime         = "11111111-1111-4111-8111-111111111111"
	unitBoot            = "22222222-2222-4222-8222-222222222222"
	unitWorkspace       = "33333333-3333-4333-8333-333333333333"
	unitSubject         = "44444444-4444-4444-8444-444444444444"
	unitOtherWorkspace  = "55555555-5555-4555-8555-555555555555"
	unitOtherSubject    = "66666666-6666-4666-8666-666666666666"
	unitRevocation      = "77777777-7777-4777-8777-777777777777"
	unitOtherRevocation = "88888888-8888-4888-8888-888888888888"
)

func unitIdentity() startgate.Identity {
	return startgate.Identity{RuntimeID: unitRuntime, BootID: unitBoot, Generation: 1}
}

func unitNew(t *testing.T) *startgate.Gate {
	t.Helper()
	g, err := startgate.New(unitIdentity())
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func unitSnapshot(revision uint64, state startgate.State) startgate.RuntimeSnapshot {
	return startgate.RuntimeSnapshot{RuntimeID: unitRuntime, BootID: unitBoot, Generation: 1,
		SnapshotRevision: revision, Gate: state}
}

func unitKey(workspace *string, scope startgate.Scope, subject string) startgate.Subject {
	return startgate.Subject{WorkspaceID: workspace, Scope: scope, SubjectID: subject}
}

func unitString(value string) *string { return &value }

func unitRequest(key startgate.Subject) startgate.StartRequest {
	return startgate.StartRequest{Authorities: []startgate.Authority{{Subject: key, Epoch: 1, Allowed: true}}}
}

func unitDefaultRequest() startgate.StartRequest {
	return unitRequest(unitKey(unitString(unitWorkspace), startgate.Session, unitSubject))
}

func unitOpen(t *testing.T, g *startgate.Gate) {
	t.Helper()
	if err := g.ApplySnapshot(context.Background(), unitSnapshot(1, startgate.Open)); err != nil {
		t.Fatal(err)
	}
}

func unitBarrier(key startgate.Subject) startgate.BarrierCommand {
	return startgate.BarrierCommand{TargetRuntimeID: unitRuntime, BootID: unitBoot, Generation: 1,
		RevocationID: unitRevocation, Subject: key, Epoch: 2, DeadlineAt: time.Now().UTC().Add(time.Hour)}
}

func TestUnitClosedAndExplicitAuthority(t *testing.T) {
	g := unitNew(t)
	calls := 0
	start := func(context.Context) error { calls++; return nil }
	if state, err := g.State(context.Background()); state != startgate.Closed || err != nil {
		t.Fatalf("initial state %s: %v", state, err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), start); !errors.Is(err, startgate.ErrClosed) {
		t.Fatal(err)
	}
	unitOpen(t, g)
	for _, authority := range []startgate.Authority{
		{Subject: unitDefaultRequest().Authorities[0].Subject, Epoch: 1, Allowed: false},
		{Subject: unitDefaultRequest().Authorities[0].Subject, Epoch: 999999, Allowed: true, RevocationPending: true},
	} {
		if err := g.Start(context.Background(), startgate.StartRequest{Authorities: []startgate.Authority{authority}}, start); !errors.Is(err, startgate.ErrDenied) {
			t.Fatal(err)
		}
	}
	if calls != 0 {
		t.Fatal("closed or denied input crossed start boundary")
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), start); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("explicit current allow did not start")
	}
}

func TestUnitIdentityAndSafeIntegers(t *testing.T) {
	for name, identity := range map[string]startgate.Identity{
		"empty runtime":     {BootID: unitBoot, Generation: 1},
		"malformed boot":    {RuntimeID: unitRuntime, BootID: "not-a-uuid", Generation: 1},
		"zero generation":   {RuntimeID: unitRuntime, BootID: unitBoot},
		"unsafe generation": {RuntimeID: unitRuntime, BootID: unitBoot, Generation: startgate.MaxSafeInteger + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := startgate.New(identity); !errors.Is(err, startgate.ErrInvalid) {
				t.Fatal(err)
			}
		})
	}
	g := unitNew(t)
	for name, mutate := range map[string]func(*startgate.RuntimeSnapshot){
		"wrong runtime":    func(s *startgate.RuntimeSnapshot) { s.RuntimeID = unitOtherSubject },
		"wrong boot":       func(s *startgate.RuntimeSnapshot) { s.BootID = unitOtherSubject },
		"wrong generation": func(s *startgate.RuntimeSnapshot) { s.Generation++ },
		"zero revision":    func(s *startgate.RuntimeSnapshot) { s.SnapshotRevision = 0 },
		"unsafe revision":  func(s *startgate.RuntimeSnapshot) { s.SnapshotRevision = startgate.MaxSafeInteger + 1 },
		"unknown gate":     func(s *startgate.RuntimeSnapshot) { s.Gate = "AUTOGRANT" },
	} {
		t.Run(name, func(t *testing.T) {
			s := unitSnapshot(1, startgate.Open)
			mutate(&s)
			if err := g.ApplySnapshot(context.Background(), s); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
			if state, _ := g.State(context.Background()); state != startgate.Closed {
				t.Fatal("invalid snapshot opened gate")
			}
		})
	}
	// Uppercase spelling denotes the same UUID, not another runtime.
	s := unitSnapshot(1, startgate.Open)
	s.RuntimeID = strings.ToUpper(s.RuntimeID)
	if err := g.ApplySnapshot(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func TestUnitSnapshotAtomicityAndMonotonicDenials(t *testing.T) {
	g := unitNew(t)
	valid := startgate.Denial{Subject: unitDefaultRequest().Authorities[0].Subject,
		Epoch: 4, RevocationID: unitRevocation, AcceptedAt: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)}
	invalid := valid
	invalid.RevocationID = unitOtherRevocation
	invalid.Epoch = 0
	bad := unitSnapshot(1, startgate.Open)
	bad.Denials = []startgate.Denial{valid, invalid}
	if err := g.ApplySnapshot(context.Background(), bad); !errors.Is(err, startgate.ErrInvalid) {
		t.Fatal(err)
	}
	unitOpen(t, g)
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("partial denial escaped rejected snapshot: %v", err)
	}
	accepted := unitSnapshot(2, startgate.Open)
	accepted.Denials = []startgate.Denial{valid}
	if err := g.ApplySnapshot(context.Background(), accepted); err != nil {
		t.Fatal(err)
	}
	if err := g.ApplySnapshot(context.Background(), accepted); err != nil {
		t.Fatalf("identical replay: %v", err)
	}
	conflict := unitSnapshot(2, startgate.Open)
	if err := g.ApplySnapshot(context.Background(), conflict); !errors.Is(err, startgate.ErrConflict) {
		t.Fatal(err)
	}
	if err := g.ApplySnapshot(context.Background(), unitSnapshot(1, startgate.Open)); !errors.Is(err, startgate.ErrStaleSnapshot) {
		t.Fatal(err)
	}
	if err := g.ApplySnapshot(context.Background(), unitSnapshot(3, startgate.Open)); err != nil {
		t.Fatal(err)
	}
	req := unitDefaultRequest()
	req.Authorities[0].Epoch = startgate.MaxSafeInteger
	if err := g.Start(context.Background(), req, func(context.Context) error { t.Fatal("epoch regranted denied scope"); return nil }); !errors.Is(err, startgate.ErrDenied) {
		t.Fatal(err)
	}
	for revision, state := range map[uint64]startgate.State{4: startgate.Closed, 5: startgate.Syncing} {
		// Separate instances avoid depending on Go map iteration order.
		other := unitNew(t)
		s := unitSnapshot(revision, state)
		if err := other.ApplySnapshot(context.Background(), s); err != nil {
			t.Fatal(err)
		}
		if err := other.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { return nil }); !errors.Is(err, startgate.ErrClosed) {
			t.Fatal(err)
		}
	}
}

func TestUnitBarrierScopeIsolationAndGlobalSession(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	key := unitDefaultRequest().Authorities[0].Subject
	command := unitBarrier(key)
	if _, err := g.ApplyBarrier(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]startgate.Subject{
		"other workspace": unitKey(unitString(unitOtherWorkspace), startgate.Session, unitSubject),
		"other subject":   unitKey(unitString(unitWorkspace), startgate.Session, unitOtherSubject),
		"other scope":     unitKey(unitString(unitWorkspace), startgate.Connection, unitSubject),
	} {
		t.Run(name, func(t *testing.T) {
			if err := g.Start(context.Background(), unitRequest(key), func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
	global := unitBarrier(unitKey(nil, startgate.Session, unitSubject))
	global.RevocationID = unitOtherRevocation
	if _, err := g.ApplyBarrier(context.Background(), global); err != nil {
		t.Fatal(err)
	}
	for _, workspace := range []*string{nil, unitString(unitWorkspace), unitString(unitOtherWorkspace)} {
		if err := g.Start(context.Background(), unitRequest(unitKey(workspace, startgate.Session, unitSubject)), func(context.Context) error { t.Fatal("global deny bypass"); return nil }); !errors.Is(err, startgate.ErrDenied) {
			t.Fatal(err)
		}
	}
	if err := g.Start(context.Background(), unitRequest(unitKey(unitString(""), startgate.Session, unitSubject)), func(context.Context) error { return nil }); !errors.Is(err, startgate.ErrInvalid) {
		t.Fatal("empty workspace silently became null", err)
	}
}

func TestUnitBarrierReplayConflictAndCopiedState(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	command := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
	ack, err := g.ApplyBarrier(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if !ack.OldStartSectionsDrained || ack.AppliedEpoch != command.Epoch || ack.RuntimeID != unitRuntime || ack.BootID != unitBoot || ack.Generation != 1 || ack.AcknowledgedAt.IsZero() {
		t.Fatalf("incomplete ACK: %+v", ack)
	}
	replay, err := g.ApplyBarrier(context.Background(), command)
	if err != nil || !reflect.DeepEqual(ack, replay) {
		t.Fatalf("replay changed ACK: %+v %v", replay, err)
	}
	conflict := command
	conflict.Epoch++
	if result, err := g.ApplyBarrier(context.Background(), conflict); !errors.Is(err, startgate.ErrConflict) || result.OldStartSectionsDrained {
		t.Fatalf("conflicting epoch received ACK: %+v %v", result, err)
	}
	conflict = command
	conflict.DeadlineAt = conflict.DeadlineAt.Add(time.Second)
	if _, err := g.ApplyBarrier(context.Background(), conflict); !errors.Is(err, startgate.ErrConflict) {
		t.Fatal("changed deadline accepted as duplicate", err)
	}
	originalDeadline := command.DeadlineAt
	*ack.Subject.WorkspaceID = unitOtherWorkspace
	*command.Subject.WorkspaceID = unitOtherWorkspace
	original := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
	original.DeadlineAt = originalDeadline
	replay, err = g.ApplyBarrier(context.Background(), original)
	if err != nil || replay.Subject.WorkspaceID == nil || *replay.Subject.WorkspaceID != unitWorkspace {
		t.Fatal("caller changed stored ACK", err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("caller changed retained denial"); return nil }); !errors.Is(err, startgate.ErrDenied) {
		t.Fatal(err)
	}
}

func TestUnitLostAckReplayAfterOriginalDeadline(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	command := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
	command.DeadlineAt = time.Now().UTC().Add(20 * time.Millisecond)
	original, err := g.ApplyBarrier(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithDeadline(context.Background(), command.DeadlineAt)
	defer cancel()
	<-deadline.Done()
	replay, err := g.ApplyBarrier(context.Background(), command)
	if err != nil || !reflect.DeepEqual(original, replay) {
		t.Fatalf("lost ACK could not be recovered unchanged after deadline: %+v %v", replay, err)
	}
}

func TestUnitStartCallbackFailureAndPanicReleaseLock(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	expected := errors.New("controlled start failed")
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { return expected }); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("callback panic swallowed")
			}
		}()
		_ = g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { panic("controlled panic") })
	}()
	if _, err := g.ApplyBarrier(context.Background(), unitBarrier(unitDefaultRequest().Authorities[0].Subject)); err != nil {
		t.Fatal("callback leaked lock", err)
	}
}

func TestUnitTimeoutNeverAcknowledgesUndrainedSection(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	command := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
	bounded, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ack, err := g.ApplyBarrier(bounded, command)
	if !errors.Is(err, context.DeadlineExceeded) || ack.OldStartSectionsDrained {
		close(release)
		t.Fatalf("undrained barrier acknowledged: %+v %v", ack, err)
	}
	// A timed-out barrier did not steal the token from the active callback;
	// its conservative process hold rejects additional starts immediately.
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := g.Start(cancelled, unitDefaultRequest(), func(context.Context) error { t.Fatal("cancelled waiter entered"); return nil }); !errors.Is(err, startgate.ErrClosed) {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	ack, err = g.ApplyBarrier(context.Background(), command)
	if err != nil || !ack.OldStartSectionsDrained {
		t.Fatalf("drained section not acknowledged: %+v %v", ack, err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("old authority started after ACK"); return nil }); !errors.Is(err, startgate.ErrClosed) {
		t.Fatal(err)
	}
}

func TestUnitExpiredValidBarrierRetainsScopedDenialWithoutAck(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	command := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
	command.DeadlineAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	ack, err := g.ApplyBarrier(context.Background(), command)
	if !errors.Is(err, context.DeadlineExceeded) || ack.OldStartSectionsDrained || !ack.AcknowledgedAt.IsZero() {
		t.Fatalf("expired new barrier acquired fabricated ACK: %+v %v", ack, err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("known expired revocation allowed a start"); return nil }); !errors.Is(err, startgate.ErrDenied) {
		t.Fatal(err)
	}
	if err := g.Start(context.Background(), unitRequest(unitKey(unitString(unitOtherWorkspace), startgate.Session, unitOtherSubject)), func(context.Context) error { return nil }); err != nil {
		t.Fatal("applied scoped denial unnecessarily closed unrelated scope", err)
	}
}

func TestUnitInterruptedBarrierHoldsProcessAcrossOpenSnapshot(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		finished <- g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	ack, err := g.ApplyBarrier(cancelled, unitBarrier(unitDefaultRequest().Authorities[0].Subject))
	if !errors.Is(err, context.Canceled) || ack.OldStartSectionsDrained {
		close(release)
		t.Fatalf("interrupted barrier manufactured drain: %+v %v", ack, err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("process hold admitted another start"); return nil }); !errors.Is(err, startgate.ErrClosed) {
		close(release)
		t.Fatal(err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if err := g.ApplySnapshot(context.Background(), unitSnapshot(2, startgate.Open)); err != nil {
		t.Fatal(err)
	}
	if state, err := g.State(context.Background()); err != nil || state != startgate.Closed {
		t.Fatalf("snapshot erased process hold: %s %v", state, err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("snapshot cleared process hold"); return nil }); !errors.Is(err, startgate.ErrClosed) {
		t.Fatal(err)
	}
}

func TestUnitForeignExpiredBarrierDoesNotHoldCurrentRuntime(t *testing.T) {
	g := unitNew(t)
	unitOpen(t, g)
	for name, change := range map[string]func(*startgate.BarrierCommand){
		"runtime":    func(command *startgate.BarrierCommand) { command.TargetRuntimeID = unitOtherSubject },
		"boot":       func(command *startgate.BarrierCommand) { command.BootID = unitOtherSubject },
		"generation": func(command *startgate.BarrierCommand) { command.Generation++ },
	} {
		t.Run(name, func(t *testing.T) {
			command := unitBarrier(unitDefaultRequest().Authorities[0].Subject)
			command.DeadlineAt = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			change(&command)
			if ack, err := g.ApplyBarrier(context.Background(), command); !errors.Is(err, startgate.ErrIdentity) || ack.OldStartSectionsDrained {
				t.Fatal("foreign barrier acknowledged", err)
			}
			if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { return nil }); err != nil {
				t.Fatal("foreign barrier closed current runtime", err)
			}
		})
	}
}

func TestUnitNewBootAndZeroGateFailClosed(t *testing.T) {
	var zero startgate.Gate
	if err := zero.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("zero gate started"); return nil }); !errors.Is(err, startgate.ErrUninitialized) {
		t.Fatal(err)
	}
	identity := unitIdentity()
	identity.BootID = unitOtherSubject
	g, err := startgate.New(identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.ApplySnapshot(context.Background(), unitSnapshot(1, startgate.Open)); !errors.Is(err, startgate.ErrIdentity) {
		t.Fatal(err)
	}
	if ack, err := g.ApplyBarrier(context.Background(), unitBarrier(unitDefaultRequest().Authorities[0].Subject)); !errors.Is(err, startgate.ErrIdentity) || ack.OldStartSectionsDrained {
		t.Fatal("old boot received ACK", err)
	}
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("new boot inherited open gate"); return nil }); !errors.Is(err, startgate.ErrClosed) {
		t.Fatal(err)
	}
}

func TestUnitSnapshotCopiesCallerOwnedPointers(t *testing.T) {
	g := unitNew(t)
	snapshot := unitSnapshot(1, startgate.Open)
	accepted := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC)
	effective := accepted.Add(time.Second)
	snapshot.Denials = []startgate.Denial{{Subject: unitDefaultRequest().Authorities[0].Subject, Epoch: 2, RevocationID: unitRevocation, AcceptedAt: accepted, EffectiveAt: &effective}}
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	*snapshot.Denials[0].Subject.WorkspaceID = unitOtherWorkspace
	snapshot.Denials[0].Subject.SubjectID = unitOtherSubject
	*snapshot.Denials[0].EffectiveAt = time.Time{}
	snapshot.Denials = nil
	if err := g.Start(context.Background(), unitDefaultRequest(), func(context.Context) error { t.Fatal("mutable caller snapshot escaped"); return nil }); !errors.Is(err, startgate.ErrDenied) {
		t.Fatal(err)
	}
}
