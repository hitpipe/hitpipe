package postgres

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

func TestRevision3PinnedLabelContextExpiryPausedAndNoCommands(t *testing.T) {
	f := prepared(t, []string{"brief"})
	reader := readStore(t, f, newReadPool(t, 1))
	r := readRequest(f, readcases.Detail)
	g := readGrant(r.WorkspaceID)
	get := func() readcases.CaseDetail {
		t.Helper()
		v, e := reader.Read(ctx, r, g)
		if e != nil {
			t.Fatal(e)
		}
		return v.(readcases.CaseDetail)
	}
	initial := get()
	if initial.ContextComplete || initial.ContextExpired || !strings.Contains(initial.Case.NextAction, "Нужны актуальные обязательные данные") {
		t.Fatal(initial)
	}
	db := connect(t)
	tx := begin(t, db, r.WorkspaceID)
	var cfg []byte
	if e := tx.QueryRow(ctx, `SELECT canonical_config FROM hp_processing.accepted_configs WHERE workspace_id=$1 AND config_id=$2`, r.WorkspaceID, config).Scan(&cfg); e != nil {
		t.Fatal(e)
	}
	var m map[string]any
	json.Unmarshal(cfg, &m)
	m["service_label"] = "Другая текущая услуга"
	raw, _ := json.Marshal(m)
	canonical, _ := jcs.Transform(raw)
	otherConfig, _ := newID()
	if _, e := tx.Exec(ctx, `INSERT INTO hp_processing.accepted_configs VALUES($1,$2,$3,'1.0','paid-order-v1',$4,$5,$6,now(),'synthetic')`, r.WorkspaceID, otherConfig, op, string(raw), intent.Digest(canonical), owner); e != nil {
		t.Fatal(e)
	}
	if _, e := tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$2,state='PAUSED',revision=revision+1 WHERE workspace_id=$1`, r.WorkspaceID, otherConfig); e != nil {
		t.Fatal(e)
	}
	if e := tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	d := get()
	if d.Case.OrderLabel != "Synthetic service" || d.Case.ConfigID != config || len(d.AvailableActions) != 0 || d.AvailableActions == nil {
		t.Fatal("pinned label/paused read/no recovery", d)
	}
	// Persist an actual owner-context value using the existing content mechanism.
	added := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	value := []byte(`"0 false сохраняются"`)
	ref, e := f.files.Write(ctx, value, "CASE_CONTEXT", preparecase.Origin{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Revision: 1}, added.Add(time.Hour), added)
	if e != nil {
		t.Fatal(e)
	}
	tx = begin(t, db, r.WorkspaceID)
	cid, _ := newID()
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.case_content VALUES($1,$2,$3,'brief',$4,$5,$6,$7,$8,$9)`, r.WorkspaceID, cid, r.CaseID, ref.ID, ref.Digest, ref.Bytes, owner, added, ref.ExpiresAt)
	if e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	d = get()
	if !d.ContextComplete || d.ContextExpired || len(d.Context) != 1 || d.Context[0].Value != "0 false сохраняются" || d.Case.DueAt != nil || d.Case.TargetURL != nil || d.LastObservationAt != nil || d.ObservationComplete || d.HistoryCursor != nil {
		t.Fatal("required/null/source projection", d)
	}
	// Authenticated expiry metadata survives purge; no stale context is served and
	// the accepted configuration/catalog label does not expire with it.
	expiredAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	oldAdded := expiredAt.Add(-time.Hour)
	expired, e := f.files.Write(ctx, value, "CASE_CONTEXT", preparecase.Origin{WorkspaceID: r.WorkspaceID, CaseID: r.CaseID, Revision: 1}, expiredAt, oldAdded)
	if e != nil {
		t.Fatal(e)
	}
	tx = begin(t, db, r.WorkspaceID)
	_, e = tx.Exec(ctx, `UPDATE hp_processing.case_content SET value_ref=$3,value_sha256=$4,serialized_value_bytes=$5,added_at=$6,expires_at=$7 WHERE workspace_id=$1 AND case_id=$2`, r.WorkspaceID, r.CaseID, expired.ID, expired.Digest, expired.Bytes, oldAdded, expired.ExpiresAt)
	if e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	if e = f.files.PurgeExpired(time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	d = get()
	if !d.ContextExpired || d.ContextComplete || len(d.Context) != 0 || d.Case.OrderLabel != "Synthetic service" || d.Case.State != "RECEIVED" || !strings.Contains(d.Case.NextAction, "Нужны актуальные обязательные данные") {
		t.Fatal("expiry changed saved state/catalog", d)
	}
}

func TestRevision3FinancialValidReadRemainsWorkingAndNullableEvidence(t *testing.T) {
	f := prepared(t, []string{})
	out, e := f.service.Prepare(ctx, f.request)
	if e != nil || out.State != "WORKING" {
		t.Fatal(out, e)
	}
	reader := readStore(t, f, newReadPool(t, 1))
	r := readRequest(f, readcases.Detail)
	v, e := reader.Read(ctx, r, readGrant(r.WorkspaceID))
	if e != nil {
		t.Fatal(e)
	}
	d := v.(readcases.CaseDetail)
	if d.Case.State != "WORKING" || d.Case.Revision != 2 || d.Case.DueAt == nil || d.Case.TargetURL != nil || d.LastObservationAt != nil || d.ObservationComplete || len(d.AvailableActions) != 0 {
		t.Fatal("financial/outbox promoted to confirmed", d)
	}
	r.Kind = readcases.History
	v, e = reader.Read(ctx, r, readGrant(r.WorkspaceID))
	if e != nil {
		t.Fatal("real producer→preparation history", e)
	}
	history := v.(readcases.HistoryView)
	if len(history.Items) != 2 || history.Items[0].OldState != nil || history.Items[1].OldState == nil || *history.Items[1].OldState != "RECEIVED" || history.Items[1].EpisodeID == nil || history.Items[1].NewState != "WORKING" {
		t.Fatal(history)
	}
	decisionCounts(t, r.WorkspaceID, 1, 2, 1)
	// Known task identity without qualified readback/command authority is not a URL
	// or permission. This is explicit controlled persisted state, not a provider.
	db := connect(t)
	tx := begin(t, db, r.WorkspaceID)
	if _, e = tx.Exec(ctx, `UPDATE hp_processing.case_identity SET known_task_id='synthetic-known-task',outcome_knowledge='TARGET_ID_KNOWN',ever_dispatch_fenced=true WHERE workspace_id=$1 AND case_id=$2`, r.WorkspaceID, r.CaseID); e != nil {
		t.Fatal(e)
	}
	tx.Commit(ctx)
	r.Kind = readcases.Detail
	v, e = reader.Read(ctx, r, readGrant(r.WorkspaceID))
	if e != nil {
		t.Fatal(e)
	}
	d = v.(readcases.CaseDetail)
	if d.Case.TargetURL != nil || d.ObservationComplete || len(d.AvailableActions) != 0 || !strings.Contains(d.Case.NextAction, "подтверждения результата нет") || !strings.Contains(d.Case.NextAction, "Повторное создание задачи запрещено") {
		t.Fatal(d)
	}
}
