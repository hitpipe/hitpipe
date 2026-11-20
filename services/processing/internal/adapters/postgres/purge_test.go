package postgres

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
)

func TestProcessingPurgeOwnedStand(t *testing.T) {
	binary := os.Getenv("HIT126_PURGE_BIN")
	if binary == "" {
		t.Skip("requires the owned HIT-126 isolated PostgreSQL runner and purge binary")
	}
	f := prepared(t, []string{})
	r := f.request
	create, err := f.service.Prepare(ctx, r)
	if err != nil || create.MessageID == "" {
		t.Fatal(create, err)
	}
	app := connect(t)
	before := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Hour)
	deadline := before.Add(time.Minute)
	origin := preparecase.Origin{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Revision: 1}
	write := func(class string, scope preparecase.Origin, expires time.Time) preparecase.Ref {
		t.Helper()
		ref, err := f.files.Write(ctx, []byte(`"synthetic"`), class, scope, expires, before)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	payload := write("DELIVERY_INTENT", origin, deadline)
	oldContext := write("CASE_CONTEXT", origin, deadline)
	activeRef := write("CASE_CONTEXT", origin, deadline)
	detail := write("HISTORY", origin, deadline)
	oldDraft := write("DRAFT_DATA", origin, deadline)
	orphan := write("HISTORY", origin, deadline)
	pending := write("HISTORY", origin, deadline)
	fresh := write("HISTORY", origin, before.Add(2*time.Hour))
	other := origin
	other.WorkspaceID, _ = newID()
	foreign := write("HISTORY", other, deadline)

	if err = os.Rename(filepath.Join(f.path, pending.ID), filepath.Join(f.path, pending.ID+".pending")); err != nil {
		t.Fatal(err)
	}
	tx := begin(t, app, r.WorkspaceID)
	stmt := func(q string, args ...any) {
		t.Helper()
		if _, err = tx.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	stmt(`UPDATE hp_processing.case_identity SET ever_dispatch_fenced=true,
 outcome_knowledge='POSSIBLE_EFFECT' WHERE case_id=$1`, r.CaseID)
	stmt(`UPDATE hp_processing.outbox SET payload_ref=$2,payload_sha256=$3,payload_bytes=$4,
 created_at=$5,expires_at=$6,state='CLAIMED',claim_token=$1,lease_until=clock_timestamp()+interval'1 min',
 delivery_count=1 WHERE message_id=$1`, create.MessageID, payload.ID, payload.Digest, payload.Bytes, before, deadline)
	for _, spec := range []struct {
		key    string
		ref    preparecase.Ref
		expiry time.Time
	}{
		{"old_field", oldContext, deadline}, {"still_protected", activeRef, before.Add(2 * time.Hour)},
	} {
		id, _ := newID()
		stmt(`INSERT INTO hp_processing.case_content VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			r.WorkspaceID, id, r.CaseID, spec.key, spec.ref.ID, spec.ref.Digest, spec.ref.Bytes, owner, before, spec.expiry)
	}
	// MATCHED opaque proof metadata survives the removal of expired detail.
	historyID, _ := newID()
	stmt(`INSERT INTO hp_processing.safety_history
 (workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state,proof_ref)
 VALUES($1,$2,$3,'SYSTEM',$4,'PURGE_FIXTURE','WORKING',$5)`, r.WorkspaceID, historyID, r.CaseID, before, detail.ID)
	stmt(`INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,$5,$6,$7)`,
		r.WorkspaceID, historyID, detail.ID, detail.Digest, detail.Bytes, before, deadline)
	stmt(`INSERT INTO hp_processing.drafts VALUES($1,$2,$3,1,'{}',$4,$5,$6,$7,$6)`,
		r.WorkspaceID, r.CaseID, op, oldDraft.ID, oldDraft.Digest, deadline, before)
	stmt(`INSERT INTO hp_processing.user_commands(workspace_id,request_id,actor_id,digest,aggregate_id,disposition,recorded_at)
 VALUES($1,$2,$3,$4,$5,'APPLIED',$6)`, r.WorkspaceID, create.MessageID, owner, payload.Digest, r.CaseID, before)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	dsn := func(role string) string {
		return "host=/var/run/postgresql dbname=hit78 sslmode=disable user=" + role
	}
	call := func(role string, extra ...string) (PurgeResult, error, string) {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"--local-stand", "--content-dir", f.path, "--workspace", r.WorkspaceID, "--limit", "1"}, extra...)...)
		cmd.Env = append(os.Environ(), "HITPIPE_PURGE_DSN="+dsn(role))
		raw, err := cmd.CombinedOutput()
		var result PurgeResult
		if err == nil {
			if err = json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err, string(raw))
			}
		}
		return result, err, string(raw)
	}
	wantFailure := func(role, message string, flags ...string) {
		t.Helper()
		result, err, raw := call(role, flags...)
		if err == nil || result != (PurgeResult{}) || !strings.Contains(raw, message) {
			t.Fatalf("wanted %q: %+v %v %s", message, result, err, raw)
		}
	}
	wantFailure("hp_processing_purge", "busy")
	if err = f.files.Close(); err != nil {
		t.Fatal(err)
	}
	wantFailure("hp_processing_app", "role, schema or workspace rejected")
	wantFailure("hp_processing_worker", "role, schema or workspace rejected")
	wantFailure("postgres", "role, schema or workspace rejected")
	wantFailure("hp_processing_purge", "use --local-stand", "--local-stand=false")

	purgeDB, err := pgx.Connect(ctx, dsn("hp_processing_purge"))
	if err != nil {
		t.Fatal(err)
	}
	defer purgeDB.Close(ctx)
	withoutScope := []string{"hp_processing.case_content", "hp_processing.outbox", "hp_processing.history_content"}
	for _, name := range withoutScope {
		var count int
		if err = purgeDB.QueryRow(ctx, "SELECT count(*) FROM "+name).Scan(&count); err != nil || count != 0 {
			t.Fatal("purge can bypass workspace RLS", name, count, err)
		}
	}
	// Take a concurrent write lock on one source. The committed outbox hold is
	// recoverable but the command cannot positively acknowledge any removal.
	tx = begin(t, app, r.WorkspaceID)
	stmt(`UPDATE hp_processing.case_content SET value_ref=value_ref WHERE field_key='still_protected'`)
	wantFailure("hp_processing_purge", "incomplete")
	_ = tx.Rollback(ctx)
	if _, err = os.Stat(filepath.Join(f.path, payload.ID)); err != nil {
		t.Fatal("body unlinked without a recheck barrier", err)
	}

	count := 0
	for i := 0; i <= 6; i++ {
		result, err, raw := call("hp_processing_purge")
		if err != nil || result.Held != 0 || result.Removed > 1 {
			t.Fatal(result, err, raw)
		}
		count += result.Removed
	}
	if count != 6 {
		t.Fatal("physical removal count", count)
	}
	for _, id := range []string{payload.ID, oldContext.ID, detail.ID, oldDraft.ID, orphan.ID, pending.ID + ".pending"} {
		if _, err = os.Stat(filepath.Join(f.path, id)); !os.IsNotExist(err) {
			t.Fatal("expired body remains", err)
		}
	}
	for _, id := range []string{activeRef.ID, fresh.ID, foreign.ID} {
		if _, err = os.Stat(filepath.Join(f.path, id)); err != nil {
			t.Fatal("protected body removed", err)
		}
	}

	tx = begin(t, app, r.WorkspaceID)
	defer tx.Rollback(ctx)
	var state, proof, session, disposition string
	var fenced, claimsCleared bool
	var attempts int
	if err = tx.QueryRow(ctx, `SELECT state,claim_token IS NULL AND lease_until IS NULL,delivery_count
 FROM hp_processing.outbox WHERE message_id=$1`, create.MessageID).Scan(&state, &claimsCleared, &attempts); err != nil ||
		state != "HELD" || !claimsCleared || attempts != 1 {
		t.Fatal("wrong outbox state", state, claimsCleared, attempts, err)
	}
	if err = tx.QueryRow(ctx, `SELECT ever_dispatch_fenced,session_id FROM hp_processing.case_identity
 WHERE case_id=$1 AND lineage_id=$2 AND outcome_knowledge='POSSIBLE_EFFECT'`, r.CaseID, create.MessageID).Scan(&fenced, &session); err != nil || !fenced || session == "" {
		t.Fatal("identity/fence lost", fenced, err)
	}
	if err = tx.QueryRow(ctx, `SELECT proof_ref::text FROM hp_processing.safety_history WHERE entry_id=$1`, historyID).Scan(&proof); err != nil || proof != detail.ID {
		t.Fatal("safety metadata lost", err)
	}
	if err = tx.QueryRow(ctx, `SELECT disposition FROM hp_processing.user_commands WHERE request_id=$1`, create.MessageID).Scan(&disposition); err != nil || disposition != "APPLIED" {
		t.Fatal("command replay key lost", err)
	}
	_ = tx.Rollback(ctx)
	reopened, err := contentfiles.New(f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err = reopened.Read(ctx, payload, time.Now()); !errors.Is(err, preparecase.ErrContent) {
		t.Fatal("missing proof was accepted", err)
	}
	f.service.Repository = DecisionStore{connect(t), reopened}
	f.service.Content = reopened
	f.service.Readers = fixtureReaders{after: func() { t.Error("replay attempted a new evaluation") }}
	replay, err := f.service.Prepare(ctx, r)
	if err == nil && replay.MessageID != create.MessageID {
		t.Fatal("replay issued a new command", replay)
	}
	var rows int
	tx = begin(t, app, r.WorkspaceID)
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.outbox WHERE kind='DELIVERY_INTENT'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatal("duplicate create outbox", rows, err)
	}
}
