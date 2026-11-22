//go:build hit148_probe && integration && linux

// This overlay composes the pinned IAM issuer with the real Processing
// maintenance authority. It never overlays an IAM implementation file.
package casedeletion_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	filereg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/filestore/casedeletion"
	httpreg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/casedeletion"
	pgowner "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/casedeletion"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	pm "github.com/hitpipe/hitpipe/services/processing/maintenance"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	sourceDSN  = "host=/var/run/postgresql dbname=hit148_source sslmode=disable user="
	restoreDSN = "host=/var/run/postgresql dbname=hit148restore sslmode=disable user="
)

type proofAuthority struct {
	mu       sync.Mutex
	current  map[string]pm.CaseAuthority
	verified atomic.Int32
}

func (a *proofAuthority) forWorkspace(ws string) pm.CaseAuthority {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.current[ws]
}
func (a *proofAuthority) WithExactTargets(c context.Context, scope app.Scope, confirm func(context.Context) error) error {
	targets := make([]pm.DeletionTarget, len(scope.Targets))
	for i, t := range scope.Targets {
		targets[i] = pm.DeletionTarget{Class: t.Class, VersionID: t.VersionID}
	}
	return a.forWorkspace(scope.WorkspaceID).WithExactTargets(c, pm.DeletionScope{
		WorkspaceID: scope.WorkspaceID, CaseID: scope.CaseID, Targets: targets,
	}, confirm)
}
func (a *proofAuthority) WithApplied(c context.Context, receipt app.Receipt, commit func(context.Context) error) error {
	return a.forWorkspace(receipt.WorkspaceID).WithApplied(c, pm.DeletionReceipt(receipt), func(c context.Context) error {
		a.verified.Add(1)
		return commit(c)
	})
}

type versionFile struct {
	Case, Entry string
	Ref         struct {
		ID, Digest string
		ExpiresAt  time.Time
	}
}
type seededFiles struct {
	Case, OtherCase, NeighborCase, Lineage, KnownTask string
	History, Context, Shared, Other, Neighbor         versionFile
}

func commandApplied(t *testing.T, env []string, input []byte, program string, args ...string) ([]byte, int) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), program, args...)
	cmd.Env = append(os.Environ(), env...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		return out, 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return out, exit.ExitCode()
}

func checkedCommand(t *testing.T, env []string, input []byte, program string, args ...string) []byte {
	t.Helper()
	out, exit := commandApplied(t, env, input, program, args...)
	if exit != 0 {
		t.Fatalf("%s exit=%d: %s", program, exit, out)
	}
	return out
}

func copyFixtureVolume(t *testing.T, source, target string) {
	t.Helper()
	list, err := os.ReadDir(source)
	requireOK(t, err)
	for _, f := range list {
		if f.Name() == ".hit148-synthetic-index" {
			continue
		}
		if !f.Type().IsRegular() || strings.HasPrefix(f.Name(), ".") {
			t.Fatal("unexpected data before synthetic backup")
		}
		raw, err := os.ReadFile(filepath.Join(source, f.Name()))
		requireOK(t, err)
		requireOK(t, os.WriteFile(filepath.Join(target, f.Name()), raw, 0600))
	}
}

func TestProcessingAppliesAndRestoresRealHIT154(t *testing.T) {
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	db := openFixtureDB(t, now)
	workspace, neighbor := db.workspaces[0], db.workspaces[1]
	owner, err := pgowner.New(db.iam, fixtureClock{now})
	requireOK(t, err)
	base := t.TempDir()
	source, backup, restored, publisher := filepath.Join(base, "hit148-source"), filepath.Join(base, "backup"),
		filepath.Join(base, "restored"), filepath.Join(base, "publisher")
	for _, dir := range []string{source, backup, restored, publisher} {
		requireOK(t, os.Mkdir(dir, 0700))
	}
	checkedCommand(t, []string{"HIT78_ISOLATED_PG=1", "HIT148_FIXTURE_VOLUME=" + source,
		"HIT148_WORKSPACE=" + workspace, "HIT148_NEIGHBOR=" + neighbor, "HIT148_OWNER=" + db.owners[0]}, nil,
		"/test/storage.test", "-test.run=^TestHIT148Seed$", "-test.count=1")
	var fixture seededFiles
	raw, err := os.ReadFile(filepath.Join(source, ".hit148-synthetic-index"))
	requireOK(t, err)
	requireOK(t, json.Unmarshal(raw, &fixture))
	requireOK(t, os.Remove(filepath.Join(source, ".hit148-synthetic-index")))
	copyFixtureVolume(t, source, backup)
	copySQL := func(phase string) {
		out := checkedCommand(t, []string{"HIT78_ISOLATED_PG=1", "HIT148_COPY_PHASE=" + phase,
			"HIT148_WORKSPACE=" + workspace, "HIT148_NEIGHBOR=" + neighbor}, nil,
			"/test/storage.test", "-test.v", "-test.run=^TestHIT148CopySQL$", "-test.count=1")
		t.Logf("HIT-128 PostgreSQL %s:\n%s", phase, out)
	}
	copySQL("backup")
	srcPool, err := pgxpool.New(ctx, sourceDSN+"hp_processing_purge")
	requireOK(t, err)
	defer srcPool.Close()
	restorePool, err := pgxpool.New(ctx, restoreDSN+"hp_processing_purge")
	requireOK(t, err)
	defer restorePool.Close()
	scope1, err := pm.NewCaseAuthority(srcPool, source, workspace, 1)
	requireOK(t, err)
	scope2, err := pm.NewCaseAuthority(restorePool, restored, workspace, 2)
	requireOK(t, err)
	other, err := pm.NewCaseAuthority(srcPool, source, neighbor, 1)
	requireOK(t, err)
	proofs := &proofAuthority{current: map[string]pm.CaseAuthority{workspace: scope1, neighbor: other}}

	pub, sign, err := ed25519.GenerateKey(rand.Reader)
	requireOK(t, err)
	publisherID := fixtureUUID()
	initial, err := app.NewState(labEnv)
	requireOK(t, err)
	store, err := filereg.Initialize(publisher, initial)
	requireOK(t, err)
	issuer, err := app.New(app.Config{Environment: labEnv, PublisherID: publisherID, ProcessingPeerID: processingPeer,
		SigningKey: sign}, store, owner, proofs, proofs)
	requireOK(t, err)
	for _, ws := range []string{workspace, neighbor} {
		requireOK(t, issuer.ProvisionWorkspace(ctx, ws, 1))
	}

	keys := newLabTLS(t)
	server, err := httpreg.NewServer(issuer, keys.publish, keys.ca, []httpreg.Peer{
		{ID: processingPeer, Workspaces: db.workspaces[:], Transport: runtimeaccess.Peer{DNSName: "processing.test", CertificateFingerprint: pin(keys.read)}},
	})
	requireOK(t, err)
	var interrupt, outage atomic.Bool
	var posted atomic.Int32
	original := server.Handler
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == httpreg.SnapshotPath && outage.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path == httpreg.ReceiptPath {
			posted.Add(1)
			if interrupt.Swap(false) {
				// The application has finished and released its lease, but the
				// publisher never receives this receipt.
				panic(http.ErrAbortHandler)
			}
		}
		original.ServeHTTP(w, r)
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	requireOK(t, err)
	go server.ServeTLS(listener, "", "")
	defer server.Close()
	origin := "https://" + listener.Addr().String()
	consumer, err := httpreg.NewClient(origin, keys.read, keys.ca, pin(keys.publish))
	requireOK(t, err)
	cfgDir := t.TempDir()
	write := func(name string, data []byte) string {
		p := filepath.Join(cfgDir, name)
		requireOK(t, os.WriteFile(p, data, 0600))
		return p
	}
	pemFile := func(name, kind string, der []byte) string {
		return write(name, pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}))
	}
	private, err := x509.MarshalPKCS8PrivateKey(keys.read.PrivateKey)
	requireOK(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	requireOK(t, err)
	cert := pemFile("consumer.pem", "CERTIFICATE", keys.read.Certificate[0])
	clientKey := pemFile("consumer.key", "PRIVATE KEY", private)
	ca := pemFile("ca.pem", "CERTIFICATE", keys.caDER)
	pubKey := pemFile("publisher.pem", "PUBLIC KEY", pubDER)

	config := func(ws string, g uint64, serverOrigin string) string {
		raw, err := json.Marshal(map[string]any{"origin": serverOrigin, "server_pin": pin(keys.publish), "publisher_id": publisherID,
			"workspace_id": ws, "generation": g, "ca": ca, "client_certificate": cert, "client_key": clientKey, "publisher_public_key": pubKey})
		requireOK(t, err)
		return write(fmt.Sprintf("cfg-%s-%d.json", ws, g), raw)
	}
	sourceCfg := config(workspace, 1, origin)
	apply := func(label, cfg, volume, dsn string, expected int) {
		t.Helper()
		out, exit := commandApplied(t, []string{"HITPIPE_PURGE_DSN=" + dsn}, nil, "/test/processing-purge",
			"--local-stand", "--content-dir", volume, "--apply-deletion-inventory", cfg)
		if exit != expected {
			t.Fatalf("%s exit=%d want=%d: %s", label, exit, expected, out)
		}
		t.Logf("%s exit=%d %s", label, exit, strings.TrimSpace(string(out)))
	}
	apply("no authorized decision", sourceCfg, source, sourceDSN+"hp_processing_purge", 1)
	apply("unknown generation", config(workspace, 99, origin), source, sourceDSN+"hp_processing_purge", 1)

	scope := app.Scope{WorkspaceID: workspace, CaseID: fixture.Case, Targets: []app.Target{
		{Class: "CASE_CONTEXT", VersionID: fixture.Context.Ref.ID},
		{Class: "CASE_CONTEXT", VersionID: fixture.Shared.Ref.ID},
		{Class: "HISTORY", VersionID: fixture.History.Ref.ID},
	}}
	alien := scope
	alien.CaseID = fixture.NeighborCase
	if _, err = issuer.Prepare(ctx, db.session[0], alien); err == nil {
		t.Fatal("foreign case passed actual Processing proof")
	}
	badVersion := scope
	badVersion.Targets = []app.Target{{Class: "HISTORY", VersionID: fixture.Other.Ref.ID}}
	if _, err = issuer.Prepare(ctx, db.session[0], badVersion); err == nil {
		t.Fatal("another case's version was proved")
	}
	if _, err = issuer.Prepare(ctx, db.session[1], scope); err == nil {
		t.Fatal("foreign owner accepted")
	}
	empty, err := consumer.Snapshot(ctx, workspace, 1)
	requireOK(t, err)
	offer, err := issuer.Prepare(ctx, db.session[0], scope)
	requireOK(t, err)
	code, err := offer.ConfirmationCode()
	requireOK(t, err)
	tampered := offer
	tampered.Payload.Scope = badVersion
	if _, err = issuer.Confirm(ctx, db.session[0], tampered, code); err == nil {
		t.Fatal("conflicting confirmation accepted")
	}
	decision, err := issuer.Confirm(ctx, db.session[0], offer, code)
	requireOK(t, err)
	start, err := issuer.StartApplying(ctx, workspace, decision.DeletionID, 1)
	requireOK(t, err)
	if err = issuer.Cancel(ctx, db.session[0], workspace, decision.DeletionID); err == nil {
		t.Fatal("APPLYING was cancelled")
	}
	expect(t, issuer.WithCurrent(ctx, workspace, 1, empty, func(context.Context) error {
		t.Error("stale signed revision called open")
		return nil
	}), app.ErrConflict)
	apply("purge denied for app", sourceCfg, source, sourceDSN+"hp_processing_app", 1)
	apply("purge denied for worker", sourceCfg, source, sourceDSN+"hp_processing_worker", 1)
	staleProof := app.Receipt{Environment: labEnv, WorkspaceID: workspace, DeletionID: start.DeletionID, ScopeDigest: start.ScopeDigest,
		Generation: 1, Revision: start.ApplyRevision, AppliedAt: time.Now().UTC(), ApplicationRef: start.ScopeDigest}
	if err = consumer.SubmitReceipt(ctx, staleProof); err == nil {
		t.Fatal("receipt passed without durable application")
	}
	checkFile := func(label, volume string, ref versionFile, exists bool) {
		t.Helper()
		_, err := os.Stat(filepath.Join(volume, ref.Ref.ID))
		if exists && err != nil || !exists && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s retain=%v: %v", label, exists, err)
		}
		if exists {
			original, err := os.ReadFile(filepath.Join(backup, ref.Ref.ID))
			requireOK(t, err)
			actual, err := os.ReadFile(filepath.Join(volume, ref.Ref.ID))
			requireOK(t, err)
			if !bytes.Equal(original, actual) {
				t.Fatalf("%s: retained file differs from pre-deletion bytes", label)
			}
		}
	}
	checkFile("before unauthorized application", source, fixture.History, true)
	checkFile("before unauthorized application", source, fixture.Context, true)

	// Missing bytes without an application mark cannot prove a deletion.
	path := filepath.Join(source, fixture.History.Ref.ID)
	bytesBefore, err := os.ReadFile(path)
	requireOK(t, err)
	requireOK(t, os.Remove(path))
	requestsBefore := posted.Load()
	apply("unknown missing body", sourceCfg, source, sourceDSN+"hp_processing_purge", 1)
	if posted.Load() != requestsBefore {
		t.Fatal("unexplained missing file produced a receipt")
	}
	checkFile("other selected version after rejected scope", source, fixture.Context, true)
	requireOK(t, os.WriteFile(path, bytesBefore, 0600))

	interrupt.Store(true)
	apply("transport interrupted after durable deletion", sourceCfg, source, sourceDSN+"hp_processing_purge", 1)
	paused, err := consumer.Snapshot(ctx, workspace, 1)
	requireOK(t, err)
	currentDecision(t, paused, decision.DeletionID, app.Applying)
	if proofs.verified.Load() != 0 {
		t.Fatal("publisher recorded proof before actual retry")
	}
	checkFile("selected history", source, fixture.History, false)
	checkFile("selected context", source, fixture.Context, false)
	checkFile("shared link", source, fixture.Shared, true)
	checkFile("other case", source, fixture.Other, true)
	checkFile("neighbor workspace", source, fixture.Neighbor, true)
	apply("retry same application and actual proof", sourceCfg, source, sourceDSN+"hp_processing_purge", 0)
	published, err := consumer.Snapshot(ctx, workspace, 1)
	requireOK(t, err)
	g1 := currentDecision(t, published, decision.DeletionID, app.Applied)
	if len(g1.Receipts) != 1 || proofs.verified.Load() != 1 {
		t.Fatal("expected exactly one proof-backed receipt")
	}
	before := posted.Load()
	apply("repeat already applied", sourceCfg, source, sourceDSN+"hp_processing_purge", 0)
	if posted.Load() != before || proofs.verified.Load() != 1 {
		t.Fatal("duplicate applied request sent another receipt")
	}
	forged := g1.Receipts[0]
	forged.Revision++
	if err = consumer.SubmitReceipt(ctx, forged); err == nil {
		t.Fatal("incorrect receipt revision accepted")
	}

	copyFixtureVolume(t, backup, restored)
	copySQL("restore")
	checkFile("restored original bytes", restored, fixture.History, true)
	requireOK(t, issuer.AdvanceGeneration(ctx, workspace, 1, 2))
	proofs.mu.Lock()
	proofs.current[workspace] = scope2
	proofs.mu.Unlock()
	apply("old generation cannot read after advance", sourceCfg, source, sourceDSN+"hp_processing_purge", 1)
	if err = consumer.SubmitReceipt(ctx, g1.Receipts[0]); err == nil {
		t.Fatal("old generation receipt accepted")
	}
	restoredCfg := config(workspace, 2, origin)
	g2pending, err := consumer.Snapshot(ctx, workspace, 2)
	requireOK(t, err)
	restoredDecision := currentDecision(t, g2pending, decision.DeletionID, app.Applying)
	if !slices.Equal(restoredDecision.Scope.Targets, g1.Scope.Targets) {
		t.Fatal("restore expanded deletion scope")
	}
	expect(t, issuer.WithCurrent(ctx, workspace, 2, g2pending, func(context.Context) error {
		t.Error("pending restore called open")
		return nil
	}), app.ErrConflict)

	// A plain restored runtime must actually serve CLOSED, even when its
	// ordinary IAM stand would be ready. Maintenance runs with it stopped
	// because HIT-126 requires the exclusive volume lease.
	runtime := newDeletionRuntimeStand(t, keys, owner, db, fixture, restored, write, ca)
	runtime.checkClosedStandalone(t)
	apply("restore reapplies before opening", restoredCfg, restored, restoreDSN+"hp_processing_purge", 0)
	checkFile("restored selected history", restored, fixture.History, false)
	checkFile("restored selected context", restored, fixture.Context, false)
	checkFile("restored shared file", restored, fixture.Shared, true)
	checkFile("restored other case", restored, fixture.Other, true)
	checkFile("restored neighbor", restored, fixture.Neighbor, true)
	g2, err := consumer.Snapshot(ctx, workspace, 2)
	requireOK(t, err)
	applied2 := currentDecision(t, g2, decision.DeletionID, app.Applied)
	if len(applied2.Receipts) != 1 || applied2.Receipts[0].Generation != 2 || proofs.verified.Load() != 2 {
		t.Fatal("current generation lacks its own durable proof")
	}
	runtime.checkRestored(t, scope2, issuer, consumer, g2pending, workspace, fixture, &outage)
	copySQL("verify")
	requireOK(t, server.Close())
	apply("issuer unavailable", restoredCfg, restored, restoreDSN+"hp_processing_purge", 1)
	t.Logf("complete: two checked durable receipts, exact old history expires_at preserved, no duplicate external effect")
}
