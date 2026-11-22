package casedeletion

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

const (
	issuer   = "f2e35810-8fb8-4022-99a0-16810b632200"
	owner    = "f2e35810-8fb8-4022-99a0-16810b632201"
	other    = "f2e35810-8fb8-4022-99a0-16810b632202"
	ws       = "f2e35810-8fb8-4022-99a0-16810b632203"
	otherWS  = "f2e35810-8fb8-4022-99a0-16810b632204"
	caseID   = "f2e35810-8fb8-4022-99a0-16810b632205"
	version  = "f2e35810-8fb8-4022-99a0-16810b632206"
	version2 = "f2e35810-8fb8-4022-99a0-16810b632207"
	consumer = "processing-qualification"
)

type memorySource struct {
	mu    sync.RWMutex
	state State
}

func copyState(in State) State {
	buf, e := json.Marshal(in)
	if e != nil {
		panic(e)
	}
	var out State
	if e := json.Unmarshal(buf, &out); e != nil {
		panic(e)
	}
	return out
}

func (s *memorySource) Read(_ context.Context, f func(State) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return f(copyState(s.state))
}
func (s *memorySource) Change(_ context.Context, f func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := copyState(s.state)
	if err := f(&out); err != nil {
		return err
	}
	s.state = out
	return nil
}

type ownerTest func(context.Context, OwnerCheck, func(context.Context, OwnerIdentity) error) error

func (f ownerTest) WithCurrentOwner(c context.Context, q OwnerCheck, cb func(context.Context, OwnerIdentity) error) error {
	return f(c, q, cb)
}

type targetTest func(context.Context, Scope, func(context.Context) error) error

func (f targetTest) WithExactTargets(c context.Context, s Scope, cb func(context.Context) error) error {
	return f(c, s, cb)
}

type applyTest func(context.Context, Receipt, func(context.Context) error) error

func (f applyTest) WithApplied(c context.Context, r Receipt, cb func(context.Context) error) error {
	return f(c, r, cb)
}

type setup struct {
	app    *Registry
	source *memorySource
	public ed25519.PublicKey
	now    time.Time
	known  map[string]bool
	proof  string
}

func newSetup(t *testing.T) *setup {
	t.Helper()
	state, err := NewState("test")
	if err != nil {
		t.Fatal(err)
	}
	pub, key, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	f := &setup{source: &memorySource{state: state}, public: pub, now: time.Date(2026, 11, 22, 11, 0, 0, 0, time.UTC), known: map[string]bool{version: true}}
	cfg := Config{Environment: "test", PublisherID: issuer, ProcessingPeerID: consumer, SigningKey: key, Clock: func() time.Time { return f.now }}
	f.app, err = New(cfg, f.source,
		ownerTest(func(c context.Context, q OwnerCheck, cb func(context.Context, OwnerIdentity) error) error {
			switch q.Session {
			case "fixture-owner":
				if q.WorkspaceID == ws {
					return cb(c, OwnerIdentity{owner})
				}
			case "fixture-other":
				return cb(c, OwnerIdentity{other})
			}
			return ErrUnauthorized
		}),
		targetTest(func(c context.Context, s Scope, cb func(context.Context) error) error {
			if s.WorkspaceID != ws || s.CaseID != caseID {
				return ErrUnauthorized
			}
			for _, target := range s.Targets {
				if !f.known[target.VersionID] {
					return ErrUnauthorized
				}
			}
			return cb(c)
		}),
		applyTest(func(c context.Context, r Receipt, cb func(context.Context) error) error {
			if f.proof == "" || r.ApplicationRef != f.proof {
				return ErrUnauthorized
			}
			return cb(c)
		}))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{ws, otherWS} {
		if err = f.app.ProvisionWorkspace(t.Context(), id, 1); err != nil {
			t.Fatal(err)
		}
	}
	return f
}
func selected() Scope {
	return Scope{WorkspaceID: ws, CaseID: caseID, Targets: []Target{{Class: "HISTORY", VersionID: version}}}
}

func (f *setup) confirm(t *testing.T) Decision {
	t.Helper()
	offer, err := f.app.Prepare(t.Context(), "fixture-owner", selected())
	if err != nil {
		t.Fatal(err)
	}
	code, err := offer.ConfirmationCode()
	if err != nil {
		t.Fatal(err)
	}
	d, err := f.app.Confirm(t.Context(), "fixture-owner", offer, code)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestOfferPinsOwnerAndTargetsAndRetainsPrivateAuditProof(t *testing.T) {
	f := newSetup(t)
	ctx := t.Context()
	initial, _ := f.app.Snapshot(ctx, ws, 1)
	if _, err := f.app.Prepare(ctx, "fixture-operator", selected()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("operator: %v", err)
	}
	o, err := f.app.Prepare(ctx, "fixture-owner", selected())
	if err != nil {
		t.Fatal(err)
	}
	code, _ := o.ConfirmationCode()
	wrong := o
	wrong.Payload.Scope.Targets = []Target{{Class: "HISTORY", VersionID: version2}}
	wrongCode, _ := wrong.ConfirmationCode()
	for _, attempt := range []struct {
		session string
		offer   Offer
		code    string
	}{
		{"fixture-other", o, code}, {"fixture-owner", wrong, wrongCode}, {"fixture-owner", o, wrongCode},
	} {
		if _, err = f.app.Confirm(ctx, attempt.session, attempt.offer, attempt.code); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("accepted changed intent: %v", err)
		}
	}
	delete(f.known, version)
	if _, err = f.app.Confirm(ctx, "fixture-owner", o, code); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("version hold: %v", err)
	}
	check, _ := f.app.Snapshot(ctx, ws, 1)
	if check.Payload.Revision != initial.Payload.Revision || len(check.Payload.Decisions) != 0 {
		t.Fatal("a refused attempt wrote to the registry")
	}
	f.known[version] = true
	d, err := f.app.Confirm(ctx, "fixture-owner", o, code)
	if err != nil {
		t.Fatal(err)
	}
	if d.ConfirmationRef != code || d.OwnerProof == nil || !f.app.validOwnerProof(d) {
		t.Fatal("missing exact retrievable owner audit proof")
	}
	out, err := f.app.Snapshot(ctx, ws, 1)
	if err != nil || out.Payload.Decisions[0].OwnerProof != nil {
		t.Fatalf("snapshot leaked publisher-only proof: %v", err)
	}
	if err := VerifySnapshot(out, f.public, SnapshotScope{"test", issuer, ws, 1}); err != nil {
		t.Fatal(err)
	}
	// A different issuer key cannot silently adopt existing exact proofs.
	r := f.source.state.Workspaces[ws]
	bad := r.Decisions[d.DeletionID]
	bad.OwnerProof.ConfirmationSignature = bad.OwnerProof.OfferSignature
	r.Decisions[d.DeletionID] = bad
	if _, err := f.app.Snapshot(ctx, ws, 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("corrupt stored proof still published: %v", err)
	}
}

func TestGenerationOpenRejectsOldReceiptAndChangedFullRevision(t *testing.T) {
	f := newSetup(t)
	ctx := t.Context()
	empty, _ := f.app.Snapshot(ctx, ws, 1)
	otherEmpty, _ := f.app.Snapshot(ctx, otherWS, 1)
	openCount := 0
	opened := func(context.Context) error { openCount++; return nil }
	if err := f.app.WithCurrent(ctx, ws, 1, empty, opened); err != nil {
		t.Fatal(err)
	}
	d := f.confirm(t)
	if err := f.app.WithCurrent(ctx, ws, 1, empty, opened); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale snapshot: %v", err)
	}
	if out, _ := f.app.Snapshot(ctx, otherWS, 1); out.Payload.Revision != otherEmpty.Payload.Revision {
		t.Fatal("foreign scope exposed publication")
	}
	d, err := f.app.StartApplying(ctx, ws, d.DeletionID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.app.Cancel(ctx, "fixture-owner", ws, d.DeletionID); !errors.Is(err, ErrConflict) {
		t.Fatalf("cancellation after start: %v", err)
	}
	proof, _ := Digest(d.DeletionID)
	rc := Receipt{Environment: "test", WorkspaceID: ws, DeletionID: d.DeletionID, ScopeDigest: d.ScopeDigest, Generation: 1,
		Revision: d.ApplyRevision, AppliedAt: f.now, ApplicationRef: proof}
	if err = f.app.AcceptReceipt(ctx, consumer, rc); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unverified outcome: %v", err)
	}
	f.proof = proof
	if err = f.app.AcceptReceipt(ctx, consumer, rc); err != nil {
		t.Fatal(err)
	}
	ready, _ := f.app.Snapshot(ctx, ws, 1)
	if err = f.app.AcceptReceipt(ctx, consumer, rc); err != nil {
		t.Fatal(err)
	}
	afterReplay, _ := f.app.Snapshot(ctx, ws, 1)
	if afterReplay.Payload.Revision != ready.Payload.Revision {
		t.Fatal("exact receipt replay changed revision")
	}
	if err = f.app.AdvanceGeneration(ctx, ws, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err = f.app.AcceptReceipt(ctx, consumer, rc); !errors.Is(err, ErrConflict) {
		t.Fatalf("old receipt: %v", err)
	}
	restarted, _ := f.app.Snapshot(ctx, ws, 2)
	if restarted.Payload.Decisions[0].Status != Applying || len(restarted.Payload.Decisions[0].Receipts) != 0 {
		t.Fatal("old receipt blessed new generation")
	}
	if err = f.app.WithCurrent(ctx, ws, 2, restarted, opened); !errors.Is(err, ErrConflict) {
		t.Fatalf("premature open: %v", err)
	}
	d, err = f.app.StartApplying(ctx, ws, d.DeletionID, 2)
	if err != nil {
		t.Fatal(err)
	}
	rc.Generation, rc.Revision = 2, d.ApplyRevision
	if err = f.app.AcceptReceipt(ctx, consumer, rc); err != nil {
		t.Fatal(err)
	}
	if err = f.app.WithCurrent(ctx, ws, 2, restarted, opened); !errors.Is(err, ErrConflict) {
		t.Fatalf("replaced revision: %v", err)
	}
	restored, _ := f.app.Snapshot(ctx, ws, 2)
	if err = f.app.WithCurrent(ctx, ws, 2, restored, opened); err != nil {
		t.Fatal(err)
	}
	if openCount != 2 {
		t.Fatalf("open allowed %d times", openCount)
	}
}
