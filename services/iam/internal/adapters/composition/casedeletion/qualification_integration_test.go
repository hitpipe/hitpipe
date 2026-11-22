//go:build integration && linux

package casedeletion_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	filereg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/filestore/casedeletion"
	httpreg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/casedeletion"
	pgowner "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/casedeletion"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
)

func requireOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func expect(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("unexpected reason: got %v; want %v", got, want)
	}
}

func currentDecision(t *testing.T, snap app.Snapshot, id string, status app.Status) app.Decision {
	t.Helper()
	for _, d := range snap.Payload.Decisions {
		if d.DeletionID == id {
			if d.Status != status {
				t.Fatalf("decision %s: state=%s; want %s", id, d.Status, status)
			}
			return d
		}
	}
	t.Fatalf("expected genuine decision %s", id)
	return app.Decision{}
}

func readVerified(t *testing.T, client *httpreg.Client, pub ed25519.PublicKey, publisherID, workspace string, generation uint64) app.Snapshot {
	t.Helper()
	snap, err := client.Snapshot(t.Context(), workspace, generation)
	requireOK(t, err)
	requireOK(t, app.VerifySnapshot(snap, pub, app.SnapshotScope{
		Environment: labEnv, PublisherID: publisherID, WorkspaceID: workspace, Generation: generation,
	}))
	return snap
}

func testExactSubordinateState(t *testing.T, subordinate *subordinateCopy, scope app.Scope, kept []caseItem, wantGeneration uint64) {
	t.Helper()
	state, err := subordinate.read()
	requireOK(t, err)
	if state.Generation != wantGeneration || !slices.Equal(kept, state.Items) ||
		slices.ContainsFunc(state.Items, func(item caseItem) bool { return selected(item, scope) }) {
		t.Fatal("synthetic copy lost an unselected version or retained a selected version")
	}
}

// TestPostgresCaseDeletionQualification composes genuine PostgreSQL-based
// owner checks, the durable separate issuer, and real private mTLS. The
// content/receipt proof and local CLOSED/OPEN flag exist only in this fixture
// and deliberately do not claim HIT-148's production consumer implementation.
func TestPostgresCaseDeletionQualification(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openFixtureDB(t, now)
	owner, err := pgowner.New(db.iam, fixtureClock{now})
	requireOK(t, err)
	workspace, otherWorkspace := db.workspaces[0], db.workspaces[1]
	session, foreignSession := db.session[0], db.session[1]

	root := t.TempDir()
	publisherDir, copyDir := filepath.Join(root, "outside-restore"), filepath.Join(root, "restore-set")
	requireOK(t, os.Mkdir(publisherDir, 0700))
	requireOK(t, os.Mkdir(copyDir, 0700))
	subordinate := &subordinateCopy{dir: copyDir}

	caseID, otherCaseID := fixtureUUID(), fixtureUUID()
	chosenContext := app.Target{Class: "CASE_CONTEXT", VersionID: fixtureUUID()}
	chosenHistory := app.Target{Class: "HISTORY", VersionID: fixtureUUID()}
	otherVersion := app.Target{Class: "CASE_CONTEXT", VersionID: fixtureUUID()}
	remainingVersion := app.Target{Class: "HISTORY", VersionID: fixtureUUID()}
	scope := app.Scope{WorkspaceID: workspace, CaseID: caseID, Targets: []app.Target{chosenContext, chosenHistory}}
	singleVersion := app.Scope{WorkspaceID: workspace, CaseID: caseID, Targets: []app.Target{remainingVersion}}
	kept := []caseItem{
		{WorkspaceID: otherWorkspace, CaseID: otherCaseID, Target: otherVersion, Detail: syntheticText},
		{WorkspaceID: workspace, CaseID: caseID, Target: remainingVersion, Detail: syntheticText},
		// Identical version identity in a different case stays outside the scope.
		{WorkspaceID: workspace, CaseID: otherCaseID, Target: chosenContext, Detail: syntheticText},
	}
	initialCopy := copyState{Generation: 1, Items: append([]caseItem{
		{WorkspaceID: workspace, CaseID: caseID, Target: chosenContext, Detail: syntheticText},
		{WorkspaceID: workspace, CaseID: caseID, Target: chosenHistory, Detail: syntheticText},
	}, kept...)}
	requireOK(t, subordinate.save(initialCopy))
	preDeletionBackup, err := os.ReadFile(subordinate.path())
	requireOK(t, err)

	initial, err := app.NewState(labEnv)
	requireOK(t, err)
	store, err := filereg.Initialize(publisherDir, initial)
	requireOK(t, err)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	requireOK(t, err)
	publisherID := fixtureUUID()
	cfg := app.Config{Environment: labEnv, PublisherID: publisherID, ProcessingPeerID: processingPeer,
		SigningKey: private, Clock: func() time.Time { return now }}
	registry, err := app.New(cfg, store, owner, subordinate, subordinate)
	requireOK(t, err)
	for _, id := range db.workspaces {
		requireOK(t, registry.ProvisionWorkspace(ctx, id, 1))
	}

	certs := newLabTLS(t)
	client, badClient, stopServer := certs.serve(t, registry, db.workspaces[:])
	if _, err := badClient.Snapshot(ctx, workspace, 1); err == nil {
		t.Fatal("another certificate cannot read a workspace even under the same fixture CA and SAN")
	}
	emptied := readVerified(t, client, public, publisherID, workspace, 1)
	emptyNeighbor := readVerified(t, client, public, publisherID, otherWorkspace, 1)
	if len(emptied.Payload.Decisions) != 0 || len(emptyNeighbor.Payload.Decisions) != 0 || !emptied.Payload.Full {
		t.Fatal("trusted current full empty snapshot not represented")
	}
	if _, err := client.Snapshot(ctx, fixtureUUID(), 1); err == nil {
		t.Fatal("unprovisioned scope did not fail closed")
	}
	requireOK(t, registry.WithCurrent(ctx, workspace, 1, emptied, func(context.Context) error { return nil }))
	t.Logf("real TLS 1.3 publisher read: valid full empty rev=%d; other cert and absent source scope denied", emptied.Payload.Revision)

	_, err = registry.Prepare(ctx, foreignSession, scope)
	expect(t, err, app.ErrUnauthorized)
	_, err = registry.Prepare(ctx, session, app.Scope{WorkspaceID: workspace, CaseID: caseID, Targets: []app.Target{otherVersion}})
	expect(t, err, app.ErrUnauthorized)
	offer, err := registry.Prepare(ctx, session, scope)
	requireOK(t, err)
	confirm, err := offer.ConfirmationCode()
	requireOK(t, err)

	db.role(t, 0, "OPERATOR")
	_, err = registry.Confirm(ctx, session, offer, confirm)
	expect(t, err, app.ErrUnauthorized)
	db.role(t, 0, "OWNER")

	tampered := offer
	tampered.Payload.Scope.Targets = []app.Target{chosenContext, remainingVersion}
	badCode, err := tampered.ConfirmationCode()
	requireOK(t, err)
	_, err = registry.Confirm(ctx, session, tampered, badCode)
	expect(t, err, app.ErrUnauthorized)
	afterRefusals := readVerified(t, client, public, publisherID, workspace, 1)
	if len(afterRefusals.Payload.Decisions) != 0 || afterRefusals.Payload.Revision != emptied.Payload.Revision {
		t.Fatal("refused operation left a decision or changed revision")
	}
	t.Log("owner checks: foreign workspace, foreign target, downgraded operator and tampered offer denied without new record")

	confirmed, err := registry.Confirm(ctx, session, offer, confirm)
	requireOK(t, err)
	cancelOffer, err := registry.Prepare(ctx, session, singleVersion)
	requireOK(t, err)
	cancelCode, err := cancelOffer.ConfirmationCode()
	requireOK(t, err)
	toCancel, err := registry.Confirm(ctx, session, cancelOffer, cancelCode)
	requireOK(t, err)
	requireOK(t, registry.Cancel(ctx, session, workspace, toCancel.DeletionID))
	_, err = registry.StartApplying(ctx, workspace, toCancel.DeletionID, 1)
	expect(t, err, app.ErrConflict)

	if err := registry.WithCurrent(ctx, workspace, 1, emptied, func(context.Context) error {
		t.Error("old but authentic snapshot let a new generation open")
		return nil
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("signed stale revision: %v", err)
	}
	applying, err := registry.StartApplying(ctx, workspace, confirmed.DeletionID, 1)
	requireOK(t, err)
	expect(t, registry.Cancel(ctx, session, workspace, applying.DeletionID), app.ErrConflict)
	snap := readVerified(t, client, public, publisherID, workspace, 1)
	currentDecision(t, snap, applying.DeletionID, app.Applying)
	currentDecision(t, snap, toCancel.DeletionID, app.Cancelled)
	early, err := fixtureReceipt(applying, now)
	requireOK(t, err)
	if err := client.SubmitReceipt(ctx, early); err == nil {
		t.Fatal("authenticated receipt without durable subordinate evidence was accepted")
	}
	stillApplying := readVerified(t, client, public, publisherID, workspace, 1)
	if stillApplying.Payload.Revision != snap.Payload.Revision {
		t.Fatal("refused early receipt changed publisher revision")
	}
	seen := currentDecision(t, stillApplying, applying.DeletionID, app.Applying)
	receipt1, err := subordinate.apply(seen, now)
	requireOK(t, err)
	testExactSubordinateState(t, subordinate, scope, kept, 1)
	t.Logf("committed APPLYING before synthetic mutation id=%s generation=%d apply_revision=%d; pre-proof receipt rejected",
		applying.DeletionID, applying.ApplyGeneration, applying.ApplyRevision)

	// Lose the first receipt attempt at the service boundary. Reopen the
	// publisher over the same separately durable state and retry identical ID.
	stopServer()
	store, err = filereg.Open(publisherDir)
	requireOK(t, err)
	registry, err = app.New(cfg, store, owner, subordinate, subordinate)
	requireOK(t, err)
	client, _, stopServer = certs.serve(t, registry, db.workspaces[:])
	got, err := registry.Confirm(ctx, session, offer, confirm)
	requireOK(t, err)
	if got.Status != app.Applying || got.DeletionID != applying.DeletionID || got.ApplyRevision != applying.ApplyRevision {
		t.Fatal("restart changed applying identity or rejected exact existing decision after target disappearance")
	}
	recovered, err := subordinate.apply(got, now)
	requireOK(t, err)
	if recovered != receipt1 {
		t.Fatal("retry generated a different durable application receipt")
	}
	requireOK(t, client.SubmitReceipt(ctx, recovered))
	g1 := readVerified(t, client, public, publisherID, workspace, 1)
	currentDecision(t, g1, applying.DeletionID, app.Applied)
	requireOK(t, client.SubmitReceipt(ctx, recovered))
	if repeated := readVerified(t, client, public, publisherID, workspace, 1); repeated.Payload.Revision != g1.Payload.Revision {
		t.Fatal("repeated durable receipt changed publisher revision")
	}
	t.Logf("reopened issuer, reloaded same id and apply revision; durable receipt stored once, published rev=%d", g1.Payload.Revision)

	var gate atomic.Bool
	open := func(context.Context) error { gate.Store(true); return nil }
	publisherBeforeRestore, err := os.ReadFile(filepath.Join(publisherDir, "registry.json"))
	requireOK(t, err)
	requireOK(t, subordinate.write(preDeletionBackup))
	restored, err := subordinate.read()
	requireOK(t, err)
	if restored.Generation != 1 || !slices.Equal(initialCopy.Items, restored.Items) {
		t.Fatal("did not restore the pre-delete content fixture")
	}
	publisherAfterRestore, err := os.ReadFile(filepath.Join(publisherDir, "registry.json"))
	requireOK(t, err)
	if sha256.Sum256(publisherBeforeRestore) != sha256.Sum256(publisherAfterRestore) {
		t.Fatal("subordinate restore changed issuer source")
	}
	requireOK(t, registry.AdvanceGeneration(ctx, workspace, 1, 2))
	restored.Generation = 2
	requireOK(t, subordinate.save(restored))
	if err := client.SubmitReceipt(ctx, receipt1); err == nil {
		t.Fatal("receipt from previous generation accepted for restored copy")
	}
	expect(t, registry.WithCurrent(ctx, workspace, 2, g1, open), app.ErrInvalid)
	g2 := readVerified(t, client, public, publisherID, workspace, 2)
	reapply := currentDecision(t, g2, applying.DeletionID, app.Applying)
	if len(reapply.Receipts) != 0 || reapply.ApplyGeneration != 2 {
		t.Fatal("old receipt appeared to attest current generation")
	}
	expect(t, registry.WithCurrent(ctx, workspace, 2, g2, open), app.ErrConflict)
	if gate.Load() {
		t.Fatal("restore opened before applying current generation")
	}

	incomplete := g2
	incomplete.Payload.Full = false
	scope2 := app.SnapshotScope{Environment: labEnv, PublisherID: publisherID, WorkspaceID: workspace, Generation: 2}
	expect(t, app.VerifySnapshot(incomplete, public, scope2), app.ErrInvalid)
	foreignScope := scope2
	foreignScope.WorkspaceID = otherWorkspace
	expect(t, app.VerifySnapshot(g2, public, foreignScope), app.ErrInvalid)
	otherPublic, _, err := ed25519.GenerateKey(rand.Reader)
	requireOK(t, err)
	expect(t, app.VerifySnapshot(g2, otherPublic, scope2), app.ErrUnauthorized)
	if gate.Load() {
		t.Fatal("partial or foreign source opened restore")
	}

	receipt2, err := subordinate.apply(reapply, now)
	requireOK(t, err)
	testExactSubordinateState(t, subordinate, scope, kept, 2)
	requireOK(t, client.SubmitReceipt(ctx, receipt2))
	eligible := readVerified(t, client, public, publisherID, workspace, 2)
	currentDecision(t, eligible, applying.DeletionID, app.Applied)
	newOffer, err := registry.Prepare(ctx, session, singleVersion)
	requireOK(t, err)
	newCode, err := newOffer.ConfirmationCode()
	requireOK(t, err)
	newDecision, err := registry.Confirm(ctx, session, newOffer, newCode)
	requireOK(t, err)
	expect(t, registry.WithCurrent(ctx, workspace, 2, eligible, open), app.ErrConflict)
	if gate.Load() {
		t.Fatal("restore opened while a newer decision was published")
	}
	requireOK(t, registry.Cancel(ctx, session, workspace, newDecision.DeletionID))
	eligible = readVerified(t, client, public, publisherID, workspace, 2)
	requireOK(t, registry.WithCurrent(ctx, workspace, 2, eligible, open))
	if !gate.Load() {
		t.Fatal("guard did not open after current exact reapplication")
	}
	t.Logf("restored subordinate backup independently; generation=2 old receipt/stale/partial/foreign states denied; applied exact old id and opened under rev=%d",
		eligible.Payload.Revision)

	neighborAfterRestore := readVerified(t, client, public, publisherID, otherWorkspace, 1)
	if len(neighborAfterRestore.Payload.Decisions) != 0 || neighborAfterRestore.Payload.Revision != emptyNeighbor.Payload.Revision {
		t.Fatal("operation changed other workspace")
	}
	out, err := json.Marshal(eligible)
	requireOK(t, err)
	atRest, err := os.ReadFile(filepath.Join(publisherDir, "registry.json"))
	requireOK(t, err)
	for _, marker := range []string{session, foreignSession, syntheticText, base64.StdEncoding.EncodeToString(private)} {
		if strings.Contains(string(out), marker) || strings.Contains(string(atRest), marker) {
			t.Fatal("forbidden synthetic payload or credential leaked into the authority")
		}
	}
	if strings.Contains(string(out), "owner_proof") {
		t.Fatal("issuer-only proof was published")
	}

	gate.Store(false)
	requireOK(t, os.Rename(publisherDir, filepath.Join(root, "offline-source")))
	if _, err := client.Snapshot(ctx, workspace, 2); err == nil {
		t.Fatal("missing issuer was mistaken for a complete empty snapshot")
	}
	if err := registry.WithCurrent(ctx, workspace, 2, eligible, open); !errors.Is(err, filereg.ErrUnavailable) || gate.Load() {
		t.Fatalf("source absence did not keep restore closed: %v", err)
	}
	stopServer()
	t.Log("independent source missing over real TLS: no snapshot; restore stayed closed; registry/publication excluded synthetic content and credentials")
}
