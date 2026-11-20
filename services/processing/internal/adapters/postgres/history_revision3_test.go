package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

func newSessionSignal(t *testing.T, f preparedFixture, session string) []byte {
	t.Helper()
	var first time.Time
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	if e := tx.QueryRow(ctx, `SELECT first_activation_at FROM hp_processing.operations WHERE workspace_id=$1`, f.request.WorkspaceID).Scan(&first); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	raw := wire(t, f.request.WorkspaceID, first.Add(time.Minute).Format(time.RFC3339Nano), "checkout.session.completed")
	var m map[string]any
	json.Unmarshal(raw, &m)
	delete(m, "message_digest")
	m["session_id"] = session
	b, _ := json.Marshal(m)
	canonical, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = intent.Digest(canonical)
	b, _ = json.Marshal(m)
	return b
}
func historyRef(t *testing.T, ws, caseID string) (preparecase.Ref, time.Time) {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, ws)
	defer tx.Rollback(ctx)
	ref := preparecase.Ref{Class: "HISTORY"}
	var at, admitted time.Time
	if e := tx.QueryRow(ctx, `SELECT k.admitted_at,h.at,c.detail_ref::text,c.detail_sha256,c.detail_bytes,c.expires_at FROM hp_processing.case_identity k JOIN hp_processing.safety_history h USING(workspace_id,case_id) JOIN hp_processing.history_content c USING(workspace_id,entry_id) WHERE k.workspace_id=$1 AND k.case_id=$2 AND h.reason_code='SIGNAL_ADMITTED'`, ws, caseID).Scan(&admitted, &at, &ref.ID, &ref.Digest, &ref.Bytes, &ref.ExpiresAt); e != nil {
		t.Fatal(e)
	}
	if !admitted.Equal(at) {
		t.Fatal("producer used different admission/history times")
	}
	return ref, at
}
func TestRevision3ActualAdmissionHistoryReplayExpiryPurgeRestart(t *testing.T) {
	f := prepared(t, []string{})
	db := connect(t)
	writer := Store{DB: db, Content: f.files, HistoryRetention: time.Second}
	raw := newSessionSignal(t, f, "cs_real_history")
	receipt, e := admission.Admit(ctx, raw, established(f.request.WorkspaceID), normalizedsignal.Decoder{}, writer)
	if e != nil {
		t.Fatal(e)
	}
	ref, at := historyRef(t, f.request.WorkspaceID, receipt.RecordID)
	if !ref.ExpiresAt.Equal(at.Add(time.Second)) {
		t.Fatal("wrong explicit retention")
	}
	before, _ := os.ReadDir(f.path)
	repeated, e := admission.Admit(ctx, raw, established(f.request.WorkspaceID), normalizedsignal.Decoder{}, writer)
	if e != nil || repeated != receipt {
		t.Fatal(repeated, e)
	}
	again, _ := historyRef(t, f.request.WorkspaceID, receipt.RecordID)
	after, _ := os.ReadDir(f.path)
	if !reflect.DeepEqual(ref, again) || len(before) != len(after) {
		t.Fatal("replay rewrote history or extended expiry")
	}
	r := readcases.Request{Kind: readcases.History, WorkspaceID: f.request.WorkspaceID, CaseID: receipt.RecordID, Session: "synthetic", Limit: 100}
	g := readGrant(r.WorkspaceID)
	reader := readStore(t, f, newReadPool(t, 1))
	value, e := reader.Read(ctx, r, g)
	if e != nil {
		t.Fatal(e)
	}
	original := value.(readcases.HistoryView).Items[0]
	// Exact boundary uses the actual stored expiry as a controlled read clock on
	// the real scoped SQL data. Public wall-clock reads are exercised below too.
	for _, offset := range []time.Duration{-time.Nanosecond, 0} {
		tx, e := connect(t).BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if e != nil {
			t.Fatal(e)
		}
		tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, r.WorkspaceID)
		probe := reader
		spy := &countOwned{OwnedContent: f.files}
		probe.Content = spy
		v, e := probe.readHistory(ctx, tx, r, readcases.Page{Snapshot: time.Now().UTC()}, ref.ExpiresAt.Add(offset))
		tx.Rollback(ctx)
		if e != nil {
			t.Fatal(e)
		}
		entry := v.(readcases.HistoryView).Items[0]
		if !reflect.DeepEqual(entry, original) {
			t.Fatal("minimal history changed stored fields")
		}
		if offset == 0 && spy.calls != 0 {
			t.Fatal("expired body was read")
		}
		if offset < 0 && spy.calls != 1 {
			t.Fatal("unexpired body not integrity checked")
		}
	}
	if wait := time.Until(ref.ExpiresAt); wait > 0 {
		time.Sleep(wait + time.Millisecond)
	}
	// Minimal200 must not depend on the expired file, whether purge happens first
	// or after the first read, and duplicate ACK must not regenerate it.
	if e = expireFixtureFiles(f.path, time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(f.path, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("purge did not remove expired body", e)
	}
	for i := 0; i < 2; i++ {
		value, e = reader.Read(ctx, r, g)
		if e != nil || !reflect.DeepEqual(value.(readcases.HistoryView).Items[0], original) {
			t.Fatal(value, e)
		}
	}
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	reader.Content = reopened
	value, e = reader.Read(ctx, r, g)
	if e != nil || !reflect.DeepEqual(value.(readcases.HistoryView).Items[0], original) {
		t.Fatal("restart lost minimal history", value, e)
	}
	writer.Content = reopened
	repeated, e = admission.Admit(ctx, raw, established(r.WorkspaceID), normalizedsignal.Decoder{}, writer)
	if e != nil || repeated != receipt {
		t.Fatal(e)
	}
	latest, _ := historyRef(t, r.WorkspaceID, r.CaseID)
	if !reflect.DeepEqual(latest, ref) {
		t.Fatal("expired replay extended history")
	}
	if _, e = os.Stat(filepath.Join(f.path, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("replay resurrected expired file")
	}
}

type countOwned struct {
	OwnedContent
	calls int
}

func (c *countOwned) ReadOwned(ctx context.Context, r preparecase.Ref, o preparecase.Origin, now time.Time) ([]byte, error) {
	c.calls++
	return c.OwnedContent.ReadOwned(ctx, r, o, now)
}

func TestRevision3AdmissionFileAndCommitFaultsNoACK(t *testing.T) {
	for _, mode := range []string{"write", "file-fsync", "rename", "directory-fsync", "commit", "lost-ack", "missing-content", "invalid-retention"} {
		t.Run(mode, func(t *testing.T) {
			f := prepared(t, []string{})
			raw := newSessionSignal(t, f, "cs_fault_"+mode)
			writer := Store{DB: connect(t), Content: f.files}
			ws := f.request.WorkspaceID
			switch mode {
			case "commit":
				writer.DB = badCommitDB{connect(t), ws}
			case "lost-ack":
				writer.DB = lostAdmissionACK{connect(t)}
			case "missing-content":
				writer.Content = nil
			case "invalid-retention":
				writer.HistoryRetention = 91 * 24 * time.Hour
			default:
				f.files.Step = func(step string) error {
					if step == mode {
						return errors.New("controlled durable file failure")
					}
					return nil
				}
			}
			got, e := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, writer)
			if e == nil || got != (admission.Receipt{}) {
				t.Fatal("failed/unknown commit acknowledged", got, e)
			}
			f.files.Step = nil
			if mode == "lost-ack" {
				retry, e := admission.Admit(ctx, raw, established(ws), normalizedsignal.Decoder{}, Store{DB: connect(t), Content: f.files})
				if e != nil {
					t.Fatal(e)
				}
				historyRef(t, ws, retry.RecordID)
				counts(t, ws, 2, 2, 2)
			} else {
				counts(t, ws, 1, 1, 1)
			}
		})
	}
}

type lostAdmissionACK struct{ *pgx.Conn }

func (d lostAdmissionACK) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.Conn.BeginTx(ctx, o)
	return lostAdmissionTx{tx}, e
}

type lostAdmissionTx struct{ pgx.Tx }

func (t lostAdmissionTx) Commit(ctx context.Context) error {
	if e := t.Tx.Commit(ctx); e != nil {
		return e
	}
	return errors.New("controlled lost actual commit ACK")
}

func TestRevision3LegacyMissingExpiryAndCorruptMetadataFail(t *testing.T) {
	f := prepared(t, []string{})
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	id, _ := newID()
	entry, _ := newID()
	at := time.Now().UTC().Add(-time.Second).Truncate(time.Microsecond)
	_, e := tx.Exec(ctx, `INSERT INTO hp_processing.case_identity(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,outcome_knowledge)VALUES($1,$2,$3,$4,'acct_synthetic',false,'legacy_fixture',$5,$6,'RECEIVED',1,'UNESTABLISHED')`, f.request.WorkspaceID, id, op, config, at, owner)
	if e != nil {
		t.Fatal(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.safety_history(workspace_id,entry_id,case_id,actor_kind,at,reason_code,new_state)VALUES($1,$2,$3,'SYSTEM',$4,'SIGNAL_ADMITTED','RECEIVED')`, f.request.WorkspaceID, entry, id, at)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	reader := readStore(t, f, newReadPool(t, 1))
	r := readRequest(f, readcases.History)
	r.CaseID = id
	if _, e = reader.Read(ctx, r, readGrant(r.WorkspaceID)); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("backfilled fabricated expiry", e)
	}
	// An invalid but syntactically insertable metadata interval is not a valid
	// expiry proof. No app UPDATE/DELETE privileges are introduced for this test.
	tx = begin(t, db, r.WorkspaceID)
	refID, _ := newID()
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,10,$5,$6)`, r.WorkspaceID, entry, refID, "sha256:"+strings.Repeat("a", 64), at, at.Add(-time.Second))
	if e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	if _, e = reader.Read(ctx, r, readGrant(r.WorkspaceID)); !errors.Is(e, readcases.ErrUnavailable) {
		t.Fatal("corrupt expiry metadata treated as normal purge", e)
	}
}
