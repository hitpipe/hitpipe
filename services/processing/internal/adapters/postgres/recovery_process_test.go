package postgres

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

func recoverExpiredDetail(t *testing.T, tx pgx.Tx, files *contentfiles.Store, ws, cid, entry string) preparecase.Ref {
	t.Helper()
	at := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Hour)
	expiry := at.Add(time.Hour)
	ref, err := files.Write(ctx, []byte(`{"controlled":true}`), "HISTORY",
		preparecase.Origin{WorkspaceID: ws, CaseID: cid, Revision: 1}, expiry, at)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history
 (workspace_id,entry_id,case_id,actor_kind,at,reason_code,old_state,new_state,proof_ref)
 VALUES($1,$2,$3,'SYSTEM',$4,'RECOVERY_EXPIRED_FIXTURE','UNKNOWN','UNKNOWN',$5)`, ws, entry, cid, at, ref.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		ws, entry, ref.ID, ref.Digest, ref.Bytes, at, ref.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestRecoveryCrashChild(t *testing.T) {
	if os.Getenv("HIT128_RECOVERY_STAND") != "1" || os.Getenv("HIT128_CRASH_CHILD") != "1" {
		t.Skip("the crash is only run by the disposable recovery stand")
	}
	if len(os.Args) < 5 {
		t.Fatal("missing controlled crash arguments")
	}
	args := os.Args[len(os.Args)-5:]
	path, ws, cid, entry, marker := args[0], args[1], args[2], args[3], args[4]
	files, err := contentfiles.New(path)
	if err != nil {
		t.Fatal(err)
	}
	tx := begin(t, connect(t), ws)
	ref := recoverExpiredDetail(t, tx, files, ws, cid, entry)
	// The file and directory have been fsynced; the SQL transaction remains
	// open and never reaches COMMIT. The parent checks both sides of this cut.
	if err = os.WriteFile(marker, []byte(ref.ID), 0600); err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(os.Getpid(), syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	t.Fatal("SIGKILL was not delivered")
}

func recoveringCrash(t *testing.T, files, ws, cid string, app *pgx.Conn) string {
	t.Helper()
	entry, _ := newID()
	path := filepath.Join(t.TempDir(), "orphan-ref")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryCrashChild$", "--", files, ws, cid, entry, path)
	cmd.Env = append(os.Environ(), "HIT128_CRASH_CHILD=1")
	raw, err := cmd.CombinedOutput()
	var failure *exec.ExitError
	if !errors.As(err, &failure) || failure.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("child did not reach the commit boundary: %v\n%s", err, raw)
	}
	id, err := os.ReadFile(path)
	if err != nil || !drafts.UUID(string(id)) {
		t.Fatal("orphan marker missing", err)
	}
	path = filepath.Join(files, string(id))
	if _, err = os.Stat(path); err != nil {
		t.Fatal("durable orphan did not survive SIGKILL", err)
	}
	tx := begin(t, app, ws)
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM hp_processing.history_content WHERE entry_id=$1)
 + (SELECT count(*) FROM hp_processing.safety_history WHERE entry_id=$1)`, entry).Scan(&count); err != nil || count != 0 {
		t.Fatal("uncommitted SQL survived the process kill", count, err)
	}
	t.Log("SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent")
	return path
}

func stopRecoveryProcess(t *testing.T, p *processingChild) {
	t.Helper()
	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var e *exec.ExitError
	if err := <-p.done; !errors.As(err, &e) || e.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatal("expected SIGKILL when stopping Processing", err)
	}
	t.Log("Processing stopped by SIGKILL")
}

type recoveryIdentity struct {
	Operation, Config, Digest, Case, Lineage, Target, State, Knowledge string
	Fence                                                              bool
	Cases, Commands, Inbox, Intents, Queries                           int
}

func restoredIdentity(t *testing.T, db *pgx.Conn, ws, cid string) recoveryIdentity {
	t.Helper()
	tx := begin(t, db, ws)
	defer tx.Rollback(ctx)
	var s recoveryIdentity
	err := tx.QueryRow(ctx, `SELECT k.operation_id::text,k.config_id::text,c.digest,k.case_id::text,
 k.lineage_id::text,k.known_task_id,k.state,k.outcome_knowledge,k.ever_dispatch_fenced,
 (SELECT count(*) FROM hp_processing.case_identity),
 (SELECT count(*) FROM hp_processing.user_commands),
 (SELECT count(*) FROM hp_processing.inbox),
 (SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_INTENT'),
 (SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_QUERY')
 FROM hp_processing.case_identity k JOIN hp_processing.accepted_configs c USING(workspace_id,config_id)
 WHERE k.case_id=$1`, cid).Scan(&s.Operation, &s.Config, &s.Digest, &s.Case, &s.Lineage, &s.Target,
		&s.State, &s.Knowledge, &s.Fence, &s.Cases, &s.Commands, &s.Inbox, &s.Intents, &s.Queries)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type recoveryOutbox struct {
	state    string
	attempts int
	lease    bool
	expiry   time.Time
}

type recoveryDriver struct {
	boot   func(log string, args ...string)
	health func(label, state string)
	stop   func()
}

func localRecoveryDriver(stand *processStand) recoveryDriver {
	var child *processingChild
	return recoveryDriver{
		boot:   func(log string, args ...string) { child = stand.bootWith(log, args...) },
		health: func(label, state string) { stand.recoveryHealth(stand.t, child, label, state) },
		stop:   func() { stopRecoveryProcess(stand.t, child) },
	}
}

func recoveredOutbox(t *testing.T, db *pgx.Conn, ws, id string) recoveryOutbox {
	t.Helper()
	tx := begin(t, db, ws)
	defer tx.Rollback(ctx)
	var q recoveryOutbox
	err := tx.QueryRow(ctx, `SELECT state,delivery_count,claim_token IS NOT NULL OR lease_until IS NOT NULL,expires_at
 FROM hp_processing.outbox WHERE message_id=$1`, id).Scan(&q.state, &q.attempts, &q.lease, &q.expiry)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func (s *processStand) recoveryHealth(t *testing.T, p *processingChild, label, state string) {
	t.Helper()
	code, raw := s.pollHealth(p, state)
	t.Logf("%s OPS mTLS HTTP %d %s", label, code, bytes.TrimSpace(raw))
}

func (s *processStand) recoveryWaitRegistration(t *testing.T, previous int64) {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for s.registrations.Load() <= previous {
		if time.Now().After(until) {
			t.Fatal("stand IAM did not receive the registration")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The IAM handler published the response. Allow its client to apply it
	// before the test reads Health, rather than asserting against a race.
	time.Sleep(100 * time.Millisecond)
}

func TestProcessingRecoveryOwnedStand(t *testing.T) {
	binary, purge := os.Getenv("HIT111_PROCESSING_BIN"), os.Getenv("HIT126_PURGE_BIN")
	if os.Getenv("HIT128_RECOVERY_STAND") != "1" || binary == "" || purge == "" {
		t.Skip("requires the private HIT-128 PostgreSQL stand and built commands")
	}
	verifyProcessingRecovery(t, newObservation(t), binary, purge, localRecoveryDriver)
}

func verifyProcessingRecovery(t *testing.T, f observationFixture, binary, purge string, driver func(*processStand) recoveryDriver) {
	t.Helper()
	started := time.Now()
	ws, cid := f.request.WorkspaceID, f.request.CaseID
	app := connect(t)
	other := prepared(t, []string{}).request
	stand := createProcessStand(t, binary, f.path, map[string]string{ws: "acct_synthetic", other.WorkspaceID: "acct_synthetic"})
	runtime := driver(stand)
	boot := func(log string) { runtime.boot(log, "-config", stand.config, "-stand-config", stand.stand) }
	stand.waitDelivery.Store(true)
	var deliveryQueries atomic.Int32
	stand.queryReply = func(w http.ResponseWriter, r *http.Request) {
		// Observe a real outbound query without returning an observation/ACK.
		deliveryQueries.Add(1)
		<-r.Context().Done()
	}
	boot("recovery-source.log")
	runtime.health("original", "CLOSED")
	stand.iamOpen.Store(true)
	runtime.health("original", "READY")
	select {
	case got := <-stand.deliverySeen:
		if got != f.lineage {
			t.Fatal("unexpected first create request")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("original create did not reach the controlled peer")
	}
	// Check the actual Intake route for the already known payment session.
	// This new signal creates an inbox receipt on the original case.
	eventAt := time.Now().UTC()
	signal := intakeWire(t, ws, func(wire map[string]any) {
		wire["source_created_at"] = eventAt.Add(-time.Hour).Format(time.RFC3339Nano)
		wire["first_received_at"] = eventAt.Format(time.RFC3339Nano)
		wire["expires_at"] = eventAt.Add(time.Hour).Format(time.RFC3339Nano)
	})
	admitted := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", signal, nil, 202)
	var intakeRecord struct {
		RecordID string `json:"record_id"`
	}
	if json.Unmarshal(admitted, &intakeRecord) != nil || intakeRecord.RecordID != cid {
		t.Fatal("duplicate payment session did not resolve to the original case")
	}

	// Its HTTP response is still withheld. Inject a separately authenticated
	// known-target observation, then a real user command for read-only recheck.
	obs := observationWire(t, f.observation(t, "TARGET_ID_OBSERVED"))
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", obs, nil, 202)
	_, _, revision, _, _, _ := f.stored(t)
	commandID, _ := newID()
	command := recheckCommand(t, commandID, revision)
	caseURL := "/api/v1/workspaces/" + ws + "/cases/" + cid
	headers := map[string]string{"X-HitPipe-Session": stand.token}
	stand.command(caseURL+"/recovery", command, commandID, 202)
	stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)

	// Crash one file/SQL writer, then add separate committed metadata for an
	// expired detail. Both file types must survive export without being copied.
	orphan := recoveringCrash(t, f.path, ws, cid, app)
	expiredEntry, _ := newID()
	tx := begin(t, app, ws)
	expired := recoverExpiredDetail(t, tx, f.files, ws, cid, expiredEntry)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	stand.bootMu.Lock()
	boot := stand.current
	stand.bootMu.Unlock()
	revocation, _ := newID()
	barrier := drafts.BarrierCommand{TargetRuntimeID: stand.identity, BootID: boot.BootID, Generation: boot.Generation,
		RevocationID: revocation, WorkspaceID: &other.WorkspaceID, Scope: "WORKSPACE", SubjectID: other.WorkspaceID, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	ack := stand.must("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", marshalProcess(t, barrier), nil, 200)
	if draftjson.Validate("BarrierAck", ack) != nil {
		t.Fatal("invalid actual barrier acknowledgement")
	}
	stand.bootMu.Lock()
	stand.denials = append(stand.denials, drafts.DenyEpochSnapshot{WorkspaceID: &other.WorkspaceID, Scope: "WORKSPACE", SubjectID: other.WorkspaceID,
		Epoch: 8, RevocationID: revocation, AcceptedAt: time.Now().Add(-time.Second).UTC()})
	stand.bootMu.Unlock()
	runtime.stop()
	stand.iamStale.Store(true)
	boot("recovery-same-db.log")
	runtime.health("cold restart, same database", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	runtime.stop()

	before := restoredIdentity(t, app, ws, cid)
	queriesBefore := deliveryQueries.Load()
	if !before.Fence || before.State != "UNKNOWN" || before.Knowledge != "TARGET_ID_KNOWN" ||
		before.Cases != 1 || before.Commands != 1 || before.Inbox != 3 || before.Intents != 1 || before.Queries != 1 {
		t.Fatal("unexpected source safety summary", before.Fence, before.State, before.Knowledge,
			before.Cases, before.Commands, before.Inbox, before.Intents, before.Queries)
	}
	first := recoveredOutbox(t, app, ws, f.lineage)
	if first.state != "CLAIMED" || !first.lease || first.attempts != 1 {
		t.Fatal("original effect did not reach the interrupted transport state", first)
	}
	tx = begin(t, app, ws)
	oldClaim := dispatchintent.Claim{Request: dispatchintent.Request{WorkspaceID: ws, Epoch: 7}, MessageID: f.lineage}
	if err := tx.QueryRow(ctx, `SELECT claim_token::text,lease_until FROM hp_processing.outbox WHERE message_id=$1`,
		f.lineage).Scan(&oldClaim.Token, &oldClaim.LeaseUntil); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	archive := captureRecovery(t, app, ws)
	stand.t.Logf("source intent state=%s lease=%t attempts=%d; known identity and fence persisted", first.state, first.lease, first.attempts)
	t.Logf("source safety: %+v", before)

	cluster := createRecoveryCluster(t, stand)
	target := cluster.connect(t, "hp_processing_app")
	if err := applyRecovery(target, ws, archive); err != nil {
		t.Fatal(err)
	}
	if err := applyRecovery(target, ws, archive); !errors.Is(err, errRecoveryNotEmpty) {
		t.Fatal("repeat restore must refuse non-empty target scope", err)
	}
	if err := applyRecovery(target, other.WorkspaceID, archive); err == nil {
		t.Fatal("RLS accepted a restore into a foreign workspace")
	}
	tx = begin(t, target, other.WorkspaceID)
	var foreign int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing.operations").Scan(&foreign); err != nil || foreign != 0 {
		t.Fatal("foreign restore was not atomic", foreign, err)
	}
	tx.Rollback(ctx)
	after := restoredIdentity(t, target, ws, cid)
	if before != after {
		t.Fatal("restored identity, command, inbox, outbox or fence differs")
	}
	held := recoveredOutbox(t, target, ws, f.lineage)
	if held.state != "HELD" || held.lease || held.attempts != first.attempts || !held.expiry.Equal(first.expiry) {
		t.Fatal("restore promoted or rearmed unfinished create", held)
	}
	query := recoveredOutbox(t, target, ws, commandID)
	if query.state != "HELD" || query.lease {
		t.Fatal("restore promoted unfinished recheck", query)
	}
	t.Logf("restored safety: %+v", after)
	t.Logf("restored intent: state=%s lease=%t attempts=%d expiry_unchanged=%t", held.state, held.lease, held.attempts, held.expiry.Equal(first.expiry))
	worker := DispatchStore{DB: cluster.connect(t, "hp_processing_worker")}
	if err := worker.Check(ctx, oldClaim); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatal("restored worker accepted an old job/claim or returned an unexpected error", err)
	}
	t.Log("restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back")

	cluster.runtimeConfig(t, stand)
	runtime.boot("recovery-normal-closed.log", "-config", stand.config)
	runtime.health("ordinary configuration without authoritative stand inputs", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	runtime.stop()

	previous := stand.registrations.Load()
	boot("recovery-target.log")
	stand.recoveryWaitRegistration(t, previous)
	runtime.health("target with stale boot snapshot", "CLOSED")
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 503)
	stand.registerGeneration.Store(2)
	stand.iamStale.Store(false)
	stand.recoveryWaitRegistration(t, stand.registrations.Load())
	runtime.health("target with wrong generation", "CLOSED")
	stand.registerGeneration.Store(0)
	runtime.health("target with fresh boot and generation", "READY")
	revokedURL := "/api/v1/workspaces/" + other.WorkspaceID + "/cases/" + other.CaseID
	stand.must("bff.test", "8443", "GET", revokedURL, nil, headers, 403)
	stand.authorizationEpoch.Store(9)
	stand.must("bff.test", "8443", "GET", revokedURL, nil, headers, 403)
	stand.must("bff.test", "8443", "GET", caseURL, nil, headers, 200)
	stand.command(caseURL+"/recovery", command, commandID, 202)
	stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", obs, nil, 202)
	replayed := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", signal, nil, 202)
	if !bytes.Equal(replayed, admitted) {
		t.Fatal("restored Intake replay lost the original receipt")
	}
	t.Log("fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded")
	history := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)
	var h readcases.HistoryView
	if json.Unmarshal(history, &h) != nil || len(h.Items) < 2 {
		t.Fatal("nonempty restored history unavailable")
	}
	expiredMetadata, valid := false, false
	for _, item := range h.Items {
		if item.EntryID == expiredEntry {
			expiredMetadata = item.EvidenceRef != nil && *item.EvidenceRef == expired.ID && item.ExpiresAt.Equal(expired.ExpiresAt)
		} else if item.EvidenceRef != nil {
			valid = true
		}
	}
	if !expiredMetadata || !valid {
		t.Fatal("did not retain live refs and expired metadata distinctly", expiredMetadata, valid)
	}
	runtime.stop()

	tx = begin(t, target, ws)
	var activeRef string
	if err := tx.QueryRow(ctx, `SELECT detail_ref::text FROM hp_processing.history_content
 WHERE entry_id<>$1 AND expires_at>clock_timestamp() ORDER BY created_at LIMIT 1`, expiredEntry).Scan(&activeRef); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	activePath := filepath.Join(f.path, activeRef)
	saved := activePath + ".recovery-test"
	if err := os.Rename(activePath, saved); err != nil {
		t.Fatal(err)
	}
	boot("recovery-missing-content.log")
	runtime.health("target, one missing detail", "READY")
	missing := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 503)
	if err := os.WriteFile(activePath, []byte("deliberately damaged synthetic content"), 0600); err != nil {
		t.Fatal(err)
	}
	// Preserve fixture ownership so the container sees corrupt bytes, even when
	// the test driver bootstraps PostgreSQL as root.
	info, err := os.Stat(saved)
	if err != nil {
		t.Fatal(err)
	}
	owner := info.Sys().(*syscall.Stat_t)
	if int(owner.Uid) != os.Geteuid() || int(owner.Gid) != os.Getegid() {
		if err := os.Chown(activePath, int(owner.Uid), int(owner.Gid)); err != nil {
			t.Fatal(err)
		}
	}
	corrupt := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 503)
	if !bytes.Contains(missing, []byte("READ_UNAVAILABLE")) || !bytes.Contains(corrupt, []byte("READ_UNAVAILABLE")) {
		t.Fatal("unexpected missing or corrupt detail result")
	}
	t.Log("live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE")
	runtime.stop()
	if err := os.Rename(saved, activePath); err != nil {
		t.Fatal(err)
	}
	if err := f.files.Close(); err != nil {
		t.Fatal(err)
	}
	purgeCmd := exec.Command(purge, "--local-stand", "--content-dir", f.path, "--workspace", ws, "--limit", "100")
	purgeCmd.Env = append(os.Environ(), "HITPIPE_PURGE_DSN="+cluster.tlsDSN("hp_processing_purge"))
	raw := recoveryCommand(t, purgeCmd)
	if !json.Valid(raw) {
		t.Fatal("unexpected cleanup output")
	}
	t.Logf("purge result: %s", strings.TrimSpace(string(raw)))
	for _, path := range []string{orphan, filepath.Join(f.path, expired.ID)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("expired file was not physically removed", err)
		}
	}
	if err := applyRecovery(target, ws, archive); !errors.Is(err, errRecoveryNotEmpty) {
		t.Fatal("post-purge replay should still refuse a nonempty target", err)
	}
	if _, err := os.Stat(filepath.Join(f.path, expired.ID)); !os.IsNotExist(err) {
		t.Fatal("restore resurrected an already removed file", err)
	}
	boot("recovery-after-purge.log")
	runtime.health("target after expiry cleanup", "READY")
	still := stand.must("bff.test", "8443", "GET", caseURL+"/history", nil, headers, 200)
	var afterHistory readcases.HistoryView
	if json.Unmarshal(still, &afterHistory) != nil || !reflect.DeepEqual(h.Items, afterHistory.Items) {
		t.Fatal("expired file removal changed retained history metadata")
	}
	runtime.stop()
	t.Log("purge removed expired bytes while preserving the exact public history metadata and expiry")
	if after := restoredIdentity(t, target, ws, cid); before != after ||
		stand.deliveryCount.Load() != 1 || deliveryQueries.Load() != queriesBefore ||
		recoveredOutbox(t, target, ws, f.lineage) != held {
		t.Fatal("replay or purge changed safety/identity or repeated the external effect")
	}
	t.Logf("RECOVERY PASS: controlled Delivery create calls=%d, query calls before/after=%d/%d, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=%d",
		stand.deliveryCount.Load(), queriesBefore, deliveryQueries.Load(), time.Since(started).Milliseconds())
}
