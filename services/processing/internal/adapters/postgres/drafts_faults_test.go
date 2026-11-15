package postgres

import (
	"bytes"
	"context"

	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDraftCommandConflictVersionCASAndHistoricalReplay(t *testing.T) {
	f := newDraftFixture(t)
	initial, saved := f.save(t)
	conflict := initial
	conflict.Config.Name = "Другой смысл той же команды"
	status, b := f.call(t, drafts.Save, conflict, nil)
	if status != 409 {
		t.Fatal(status, string(b))
	}
	draftCapture(t, drafts.Save, status, conflict, b, "same_id_different_body_conflict")
	rev := int64(1)
	checkCommand := f.meta(&rev)
	status, checked := f.call(t, drafts.Check, checkCommand, nil)
	if status != 200 {
		t.Fatal(status, string(checked))
	}
	update := drafts.DraftWrite{Command: f.meta(&rev), Config: f.config()}
	update.Config.ListAlias = draftString("Другой список")
	status, b = f.call(t, drafts.Save, update, nil)
	var newer drafts.DraftConfig
	if status != 201 || json.Unmarshal(b, &newer) != nil || newer.Revision != 2 {
		t.Fatal("version update", status, string(b))
	}
	draftCapture(t, drafts.Save, status, update, b, "updated_revision2")
	status, b = f.call(t, drafts.Save, initial, nil)
	if status != 201 || !bytes.Equal(b, saved) {
		t.Fatal("old save response rewritten", status, string(b))
	}
	status, b = f.call(t, drafts.Check, checkCommand, nil)
	if status != 200 || !bytes.Equal(b, checked) {
		t.Fatal("old check response rewritten", status, string(b))
	}
	draftCapture(t, drafts.Check, status, checkCommand, b, "historical_check_remains_bound_to_revision1")
	stale := drafts.DraftWrite{Command: f.meta(&rev), Config: f.config()}
	status, b = f.call(t, drafts.Save, stale, nil)
	if status != 412 {
		t.Fatal("stale config overwritten", status, string(b))
	}
	status, b = f.call(t, drafts.Save, stale, map[string]string{"If-Match": "2"})
	if status != 412 {
		t.Fatal("If-Match disagrees", status)
	}
	draftCapture(t, drafts.Save, status, stale, b, "revision_precondition_failed")
	rev2 := int64(2)
	fresh := f.meta(&rev2)
	status, b = f.call(t, drafts.Check, fresh, nil)
	var nowCheck drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(b, &nowCheck) != nil || nowCheck.DraftRevision != 2 {
		t.Fatal(status, string(b))
	}
	var old drafts.ReadinessCheck
	json.Unmarshal(checked, &old)
	if old.ConfigDigest == nowCheck.ConfigDigest {
		t.Fatal("changed configuration kept digest")
	}
	f.counts(t, 4)
}
func TestDraftConcurrentCASAndSameCommand(t *testing.T) {
	for _, same := range []bool{false, true} {
		t.Run(map[bool]string{true: "same_command", false: "conflicting_revisions"}[same], func(t *testing.T) {
			f := newDraftFixture(t)
			f.save(t)
			rev := int64(1)
			inputs := make([]drafts.DraftWrite, 6)
			for i := range inputs {
				inputs[i] = drafts.DraftWrite{Command: f.meta(&rev), Config: f.config()}
			}
			if same {
				for i := range inputs {
					inputs[i] = inputs[0]
				}
			}
			var wg sync.WaitGroup
			statuses := make(chan int, len(inputs))
			bodies := make(chan []byte, len(inputs))
			for _, in := range inputs {
				wg.Add(1)
				go func(in drafts.DraftWrite) {
					defer wg.Done()
					s, b := f.call(t, drafts.Save, in, nil)
					statuses <- s
					bodies <- b
				}(in)
			}
			wg.Wait()
			close(statuses)
			close(bodies)
			success := 0
			for s := range statuses {
				if s == 201 {
					success++
				} else if s != 412 {
					t.Fatal("unexpected CAS status", s)
				}
			}
			if same && success != 6 || !same && success != 1 {
				t.Fatal("CAS winners", success)
			}
			if same {
				var first []byte
				for b := range bodies {
					if first == nil {
						first = b
					} else if !bytes.Equal(first, b) {
						t.Fatal("same command produced different full responses")
					}
				}
			}
			f.counts(t, 2)
		})
	}
}
func TestDraftExpiryReplayDoesNotRenewPositiveReadiness(t *testing.T) {
	f := newDraftFixture(t)
	f.save(t)
	rev := int64(1)
	command := f.meta(&rev)
	status, original := f.call(t, drafts.Check, command, nil)
	var check drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(original, &check) != nil || !check.Passed {
		t.Fatal(status, string(original))
	}
	time.Sleep(600 * time.Millisecond)
	status, b := f.call(t, drafts.Check, command, nil)
	if status != 200 || !bytes.Equal(original, b) || check.ExpiresAt.After(time.Now()) {
		t.Fatal("replay renewed expired readiness", status, string(b))
	}
	draftCapture(t, drafts.Check, status, command, b, "expired_positive_replay_is_historical_not_new_permission")
	fresh := f.meta(&rev)
	status, b = f.call(t, drafts.Check, fresh, nil)
	var again drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(b, &again) != nil || again.CheckID == check.CheckID || !again.ExpiresAt.After(check.ExpiresAt) {
		t.Fatal("fresh independent check missing", status, string(b))
	}
	f.counts(t, 3)
}
func TestDraftMissingCorruptAndForeignContentFailClosed(t *testing.T) {
	for _, mode := range []string{"missing_root", "corrupt_root", "bad_digest", "foreign_ref", "missing_full_response"} {
		t.Run(mode, func(t *testing.T) {
			f := newDraftFixture(t)
			saved, _ := f.save(t)
			db := connect(t)
			tx := begin(t, db, f.workspace)
			var ref, hash string
			var expiry time.Time
			if e := tx.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,prose_expires_at FROM hp_processing.drafts WHERE workspace_id=$1 AND draft_id=$2`, f.workspace, f.id).Scan(&ref, &hash, &expiry); e != nil {
				t.Fatal(e)
			}
			tx.Rollback(ctx)
			switch mode {
			case "missing_root":
				if e := os.Remove(filepath.Join(f.path, ref)); e != nil {
					t.Fatal(e)
				}
			case "corrupt_root":
				if e := os.WriteFile(filepath.Join(f.path, ref), []byte("corrupt"), 0600); e != nil {
					t.Fatal(e)
				}
			case "bad_digest":
				tx = begin(t, db, f.workspace)
				if _, e := tx.Exec(ctx, `UPDATE hp_processing.drafts SET prose_sha256=$2 WHERE workspace_id=$1`, f.workspace, "sha256:"+strings.Repeat("f", 64)); e != nil {
					t.Fatal(e)
				}
				if e := tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
			case "foreign_ref":
				other := newDraftFixture(t)
				other.save(t)
				otherDB := connect(t)
				ot := begin(t, otherDB, other.workspace)
				if e := ot.QueryRow(ctx, `SELECT prose_ref::text,prose_sha256,prose_expires_at FROM hp_processing.drafts WHERE workspace_id=$1`, other.workspace).Scan(&ref, &hash, &expiry); e != nil {
					t.Fatal(e)
				}
				ot.Rollback(ctx)
				raw, e := os.ReadFile(filepath.Join(other.path, ref))
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(f.path, ref), raw, 0600); e != nil {
					t.Fatal(e)
				}
				tx = begin(t, db, f.workspace)
				if _, e = tx.Exec(ctx, `UPDATE hp_processing.drafts SET prose_ref=$2,prose_sha256=$3,prose_expires_at=$4,expires_at=$4 WHERE workspace_id=$1`, f.workspace, ref, hash, expiry); e != nil {
					t.Fatal(e)
				}
				if e = tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
			case "missing_full_response":
				raw, e := f.files.ReadDraftRoot(ctx, ref, hash, expiry, origin(f.workspace, f.id, 1), time.Now())
				if e != nil {
					t.Fatal(e)
				}
				var root drafts.Root
				json.Unmarshal(raw, &root)
				raw, e = f.files.ReadDraft(ctx, *root.Tail, origin(f.workspace, f.id, 1), time.Now())
				if e != nil {
					t.Fatal(e)
				}
				var node drafts.CommandNode
				json.Unmarshal(raw, &node)
				if e = os.Remove(filepath.Join(f.path, node.Response.ID)); e != nil {
					t.Fatal(e)
				}
			}
			status, b := f.call(t, drafts.Save, saved, nil)
			if status != 503 {
				t.Fatal("metadata-only or corrupted replay success", mode, status, string(b))
			}
			if bytes.Contains(b, []byte(saved.Config.Name)) {
				t.Fatal("private content in error")
			}
			f.counts(t, 1)
			draftCapture(t, drafts.Save, status, saved, b, "content_failure_"+mode)
		})
	}
}
func TestDraftFileDurabilityFailureRollsBackMetadata(t *testing.T) {
	for _, step := range []string{"write", "file-fsync", "rename", "dir-fsync"} {
		t.Run(step, func(t *testing.T) {
			f := newDraftFixture(t)
			f.files.Step = func(s string) error {
				if s == step {
					return errors.New("controlled file failure")
				}
				return nil
			}
			input := drafts.DraftWrite{Command: f.meta(nil), Config: f.config()}
			status, _ := f.call(t, drafts.Save, input, nil)
			if status != 503 {
				t.Fatal("fault acknowledged", status)
			}
			db := connect(t)
			tx := begin(t, db, f.workspace)
			for _, table := range []string{"drafts", "user_commands"} {
				var n int
				if e := tx.QueryRow(ctx, "SELECT count(*) FROM hp_processing."+table+" WHERE workspace_id=$1", f.workspace).Scan(&n); e != nil || n != 0 {
					t.Fatal("partial SQL metadata", table, n, e)
				}
			}
			tx.Rollback(ctx)
			f.files.Step = nil
			status, b := f.call(t, drafts.Save, input, nil)
			if status != 201 {
				t.Fatal("retry after rollback", status, string(b))
			}
			f.counts(t, 1)
		})
	}
}

type draftCommitLossDB struct {
	draftTestDB
	crash bool
}
type draftCommitLossTx struct {
	pgx.Tx
	command, crash bool
}

func (d draftCommitLossDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.draftTestDB.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &draftCommitLossTx{Tx: tx, crash: d.crash}, nil
}
func (t *draftCommitLossTx) Exec(c context.Context, q string, a ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "INSERT INTO hp_processing.user_commands") {
		t.command = true
	}
	return t.Tx.Exec(c, q, a...)
}
func (t *draftCommitLossTx) Commit(c context.Context) error {
	e := t.Tx.Commit(c)
	if e == nil && t.command {
		if t.crash {
			os.Exit(24)
		}
		return errors.New("controlled SQL commit ACK loss")
	}
	return e
}
func TestDraftSQLCommitAckLossRestoresFullResponse(t *testing.T) {
	f := newDraftFixture(t)
	f.service.Repository = DraftStore{draftCommitLossDB{}, f.files, draftjson.Codec{}, testClock{time.Now}, f.service.Policy}
	f.start(t)
	input := drafts.DraftWrite{Command: f.meta(nil), Config: f.config()}
	status, b := f.call(t, drafts.Save, input, nil)
	if status != 503 {
		t.Fatal("unknown commit reported success", status, string(b))
	}
	f.counts(t, 1)
	f.service.Repository = f.repo
	f.start(t)
	status, b = f.call(t, drafts.Save, input, nil)
	if status != 201 {
		t.Fatal("lost commit replay", status, string(b))
	}
	f.counts(t, 1)
	draftCapture(t, drafts.Save, status, input, b, "actual_commit_ack_loss_recovered")
}
func TestDraftProcessCrashAfterCommit(t *testing.T) {
	if os.Getenv("HIT100_CRASH_AFTER_COMMIT") == "1" {
		ws := os.Getenv("HIT100_WS")
		id := os.Getenv("HIT100_DRAFT")
		files, e := contentfiles.New(os.Getenv("HIT100_CONTENT"))
		if e != nil {
			os.Exit(21)
		}
		defer files.Close()
		raw, e := os.ReadFile(os.Getenv("HIT100_REQUEST"))
		if e != nil {
			os.Exit(22)
		}
		policy := drafts.Policy{ContentTTL: time.Hour, CheckTTL: time.Second, ReplayNodes: 256}
		clock := testClock{time.Now}
		auth := &draftAuthority{workspace: ws, role: "OWNER", epoch: 7, expiry: time.Now().Add(time.Hour)}
		gate := &draftGate{workspace: ws, epoch: 7}
		svc := drafts.Service{Repository: DraftStore{draftCommitLossDB{crash: true}, files, draftjson.Codec{}, clock, policy}, Authority: auth, Boundary: gate, Bindings: &draftBindings{}, Clock: clock, Codec: draftjson.Codec{}, Policy: policy}
		e = svc.Execute(ctx, drafts.Request{Kind: drafts.Check, WorkspaceID: ws, DraftID: id, Session: "controlled-session", CSRF: strings.Repeat("c", 32), Body: raw}, func([]byte, int) error { return errors.New("should crash before emit") })
		if e != nil {
			os.Exit(23)
		}
		os.Exit(25)
	}
	f := newDraftFixture(t)
	f.save(t)
	rev := int64(1)
	command := f.meta(&rev)
	raw, _ := json.Marshal(command)
	requestPath := filepath.Join(t.TempDir(), "request.json")
	if e := os.WriteFile(requestPath, raw, 0600); e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestDraftProcessCrashAfterCommit$")
	cmd.Env = append(os.Environ(), "HIT100_CRASH_AFTER_COMMIT=1", "HIT100_WS="+f.workspace, "HIT100_DRAFT="+f.id, "HIT100_CONTENT="+f.path, "HIT100_REQUEST="+requestPath)
	e := cmd.Run()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 24 {
		t.Fatal("actual commit crash absent", e)
	}
	f.bindings.unavailable = true
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	f.service.Repository = DraftStore{draftTestDB{}, reopened, draftjson.Codec{}, testClock{time.Now}, f.service.Policy}
	f.start(t)
	status, b := f.call(t, drafts.Check, command, nil)
	var stored drafts.ReadinessCheck
	if status != 200 || json.Unmarshal(b, &stored) != nil || !stored.Passed {
		t.Fatal("full response after process crash", status, string(b))
	}
	againStatus, again := f.call(t, drafts.Check, command, nil)
	if againStatus != 200 || !bytes.Equal(again, b) || f.bindings.calls != 0 {
		t.Fatal("crash replay reexecuted owner check")
	}
	f.counts(t, 2)
	draftCapture(t, drafts.Check, status, command, b, "real_process_exit_after_command_commit_full_response_recovery")
}
func TestDraftApplicationRolePoolAndCancellation(t *testing.T) {
	f := newDraftFixture(t)
	f.save(t)
	db := connect(t)
	repo := DraftStore{db, f.files, draftjson.Codec{}, testClock{time.Now}, f.service.Policy}
	g := drafts.Grant{WorkspaceID: f.workspace, PrincipalID: owner}
	r := drafts.Request{Kind: drafts.Get, WorkspaceID: f.workspace, DraftID: f.id}
	if _, e := repo.Load(ctx, r, drafts.Input{}, g); e != nil {
		t.Fatal(e)
	}
	r.WorkspaceID = owner
	g.WorkspaceID = owner
	if _, e := repo.Load(ctx, r, drafts.Input{}, g); !errors.Is(e, drafts.ErrNotFound) {
		t.Fatal("foreign draft exposed", e)
	}
	var scope *string
	if e := db.QueryRow(ctx, `SELECT NULLIF(current_setting('hp.workspace_id',true),'')`).Scan(&scope); e != nil || scope != nil {
		t.Fatal("scope leaked", e, scope)
	}
	r.WorkspaceID = f.workspace
	g.WorkspaceID = f.workspace
	c, cancel := context.WithCancel(ctx)
	cancel()
	if _, e := repo.Load(c, r, drafts.Input{}, g); e == nil {
		t.Fatal("cancelled read accepted")
	}
	db.Close(ctx)
	if _, e := repo.Load(ctx, r, drafts.Input{}, g); e == nil {
		t.Fatal("closed database accepted")
	}
	if _, e := f.repo.Load(ctx, r, drafts.Input{}, g); e != nil {
		t.Fatal("fresh connection recovery", e)
	}
}
