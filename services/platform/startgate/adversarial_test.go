package startgate_test

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
	runtimeID    = "10000000-0000-4000-8000-000000000001"
	bootID       = "10000000-0000-4000-8000-000000000002"
	workspaceA   = "10000000-0000-4000-8000-000000000003"
	workspaceB   = "10000000-0000-4000-8000-000000000004"
	sessionID    = "10000000-0000-4000-8000-000000000005"
	revocationID = "10000000-0000-4000-8000-000000000006"
)

func fixture(t *testing.T) (*startgate.Gate, startgate.RuntimeSnapshot, startgate.StartRequest, startgate.BarrierCommand) {
	t.Helper()
	g, err := startgate.New(startgate.Identity{RuntimeID: runtimeID, BootID: bootID, Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := startgate.RuntimeSnapshot{RuntimeID: runtimeID, BootID: bootID, Generation: 1, SnapshotRevision: 1, Gate: startgate.Open}
	workspace := workspaceA
	subject := startgate.Subject{WorkspaceID: &workspace, Scope: startgate.Session, SubjectID: sessionID}
	req := startgate.StartRequest{Authorities: []startgate.Authority{{Subject: subject, Epoch: 1, Allowed: true}}}
	barrier := startgate.BarrierCommand{TargetRuntimeID: runtimeID, BootID: bootID, Generation: 1,
		RevocationID: revocationID, Subject: subject, Epoch: 2, DeadlineAt: time.Now().Add(5 * time.Second)}
	return g, snapshot, req, barrier
}

func noStart(t *testing.T, g *startgate.Gate, request startgate.StartRequest) {
	t.Helper()
	called := false
	err := g.Start(context.Background(), request, func(context.Context) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("forbidden actual start: err=%v called=%v", err, called)
	}
}

func TestAdversarialConcurrentActualStartsAfterAcknowledgedBarrier(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	var acknowledged atomic.Bool
	var late atomic.Int64
	var starts atomic.Int64
	actualStart := func(context.Context) error {
		if acknowledged.Load() {
			late.Add(1)
		}
		starts.Add(1)
		return nil
	}
	if err := g.Start(context.Background(), request, actualStart); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	ready := make(chan struct{})
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-ready
			for j := 0; j < 100; j++ {
				err := g.Start(context.Background(), request, actualStart)
				if err != nil && !errors.Is(err, startgate.ErrDenied) {
					t.Errorf("start: %v", err)
					return
				}
			}
		}()
	}
	close(ready)
	ack, err := g.ApplyBarrier(context.Background(), barrier)
	if err != nil || !ack.OldStartSectionsDrained || ack.AppliedEpoch != 2 {
		t.Fatalf("ACK=%+v err=%v", ack, err)
	}
	acknowledged.Store(true)
	workers.Wait()
	for i := 0; i < 100; i++ {
		noStart(t, g, request)
	}
	if starts.Load() == 0 || late.Load() != 0 {
		t.Fatalf("starts=%d after ACK=%d", starts.Load(), late.Load())
	}
}

func TestAdversarialNewSnapshotAndLargerEpochCannotRegrant(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ApplyBarrier(context.Background(), barrier); err != nil {
		t.Fatal(err)
	}
	// A delayed registration result can carry a higher snapshot revision while
	// omitting a barrier received locally. It must never erase the denial.
	snapshot.SnapshotRevision++
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	request.Authorities[0].Epoch = startgate.MaxSafeInteger
	noStart(t, g, request)
	snapshot.SnapshotRevision--
	if err := g.ApplySnapshot(context.Background(), snapshot); !errors.Is(err, startgate.ErrStaleSnapshot) {
		t.Fatalf("old snapshot: %v", err)
	}
	noStart(t, g, request)
}

func TestAdversarialGlobalSessionDenialAndWorkspaceIsolation(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	barrier.Subject.WorkspaceID = nil
	if _, err := g.ApplyBarrier(context.Background(), barrier); err != nil {
		t.Fatal(err)
	}
	for _, workspace := range []string{workspaceA, workspaceB} {
		request.Authorities[0].Subject.WorkspaceID = &workspace
		noStart(t, g, request)
	}
	// Same opaque ID in another authority scope is a distinct subject.
	request.Authorities[0].Subject.Scope = startgate.Connection
	if err := g.Start(context.Background(), request, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestAdversarialOwnedSnapshotAndAckCopies(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	workspace := workspaceA
	snapshot.Denials = []startgate.Denial{{Subject: startgate.Subject{WorkspaceID: &workspace,
		Scope: startgate.Session, SubjectID: sessionID}, Epoch: 1, RevocationID: revocationID, AcceptedAt: time.Now()}}
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	workspace = workspaceB
	snapshot.Denials[0].Subject.SubjectID = runtimeID
	noStart(t, g, request)
	barrier.Epoch = 1
	ack, err := g.ApplyBarrier(context.Background(), barrier)
	if err != nil {
		t.Fatal(err)
	}
	*ack.Subject.WorkspaceID = workspaceB
	replay, err := g.ApplyBarrier(context.Background(), barrier)
	if err != nil || *replay.Subject.WorkspaceID != workspaceA {
		t.Fatalf("mutable cached ACK: %+v err=%v", replay, err)
	}
}

func TestAdversarialCancelledBarrierCannotAcknowledgeBlockedStart(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- g.Start(context.Background(), request, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("start did not enter")
	}
	ctx, cancel := context.WithCancel(context.Background())
	barrierDone := make(chan struct{})
	var ack startgate.BarrierAck
	var err error
	go func() { ack, err = g.ApplyBarrier(ctx, barrier); close(barrierDone) }()
	cancel()
	select {
	case <-barrierDone:
		if !errors.Is(err, context.Canceled) || ack.OldStartSectionsDrained {
			t.Errorf("false ACK before drain: %+v %v", ack, err)
		}
	case <-time.After(2 * time.Second):
		t.Error("cancelled barrier did not return")
	}
	close(release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("original start did not finish")
	}
	if _, err := g.ApplyBarrier(context.Background(), barrier); err != nil {
		t.Fatal(err)
	}
	noStart(t, g, request)
}

func TestAdversarialAlreadyStartedWorkMayFinishAfterAck(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	continueWork := make(chan struct{})
	finished := make(chan struct{})
	err := g.Start(context.Background(), request, func(context.Context) error {
		// This channel marks a controlled actual-start boundary. The remainder
		// represents work that has already begun, not a scheduled future start.
		go func() { <-continueWork; close(finished) }()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.ApplyBarrier(context.Background(), barrier); err != nil {
		t.Fatal(err)
	}
	noStart(t, g, request)
	select {
	case <-finished:
		t.Fatal("barrier claimed cancellation of previous work")
	default:
	}
	close(continueWork)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("previous work did not complete")
	}
}

func TestAdversarialAllAuthoritySubjectsCheckedBeforeCallback(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := g.ApplyBarrier(context.Background(), barrier); err != nil {
		t.Fatal(err)
	}
	allow := startgate.Authority{Subject: startgate.Subject{Scope: startgate.Connection, SubjectID: runtimeID}, Epoch: 1, Allowed: true}
	request.Authorities = append([]startgate.Authority{allow}, request.Authorities...)
	noStart(t, g, request)
	request.Authorities = []startgate.Authority{allow}
	request.Authorities[0].Allowed = false
	noStart(t, g, request)
	request.Authorities[0].Allowed = true
	request.Authorities[0].RevocationPending = true
	noStart(t, g, request)
	noStart(t, g, startgate.StartRequest{})
}

func TestAdversarialLateBarrierCannotLeaveKnownRevocationOpen(t *testing.T) {
	g, snapshot, request, barrier := fixture(t)
	if err := g.ApplySnapshot(context.Background(), snapshot); err != nil {
		t.Fatal(err)
	}
	barrier.DeadlineAt = time.Now().Add(-time.Second)
	ack, err := g.ApplyBarrier(context.Background(), barrier)
	if !errors.Is(err, context.DeadlineExceeded) || ack.OldStartSectionsDrained {
		t.Fatalf("late barrier fabricated success: %+v %v", ack, err)
	}
	noStart(t, g, request)
	request.Authorities[0].Epoch = startgate.MaxSafeInteger
	noStart(t, g, request)
}
