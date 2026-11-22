package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

// TestHIT148Seed prepares synthetic data only. The pinned issuer and the
// Processing maintenance CLI independently perform and verify any deletion.
func TestHIT148Seed(t *testing.T) {
	dir, workspace, neighbor, principal := os.Getenv("HIT148_FIXTURE_VOLUME"), os.Getenv("HIT148_WORKSPACE"),
		os.Getenv("HIT148_NEIGHBOR"), os.Getenv("HIT148_OWNER")
	if dir == "" || os.Getenv("HIT78_ISOLATED_PG") != "1" {
		t.Skip("requires the isolated HIT-148 runner")
	}
	if !filepath.IsAbs(dir) || !strings.HasPrefix(filepath.Base(dir), "hit148-") {
		t.Fatal("invalid disposable volume")
	}
	db, err := pgx.Connect(t.Context(), "host=/var/run/postgresql dbname=hit148_source sslmode=disable user=hp_processing_app")
	if err != nil {
		t.Fatal("connect fixture role:", err)
	}
	defer db.Close(t.Context())
	files, err := contentfiles.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer files.Close()
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	expiry := now.Add(24 * time.Hour)
	id := func() string {
		v, err := newID()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cfg := map[string]any{"pack_id": "PAID_ORDER_TASK_V1", "source_connection_id": config,
		"destination_connection_id": destination, "destination_workspace_id": "cu_synthetic",
		"price_id": "price_fixed", "list_id": "list_synthetic", "assignee_id": "assignee_synthetic", "mode": "TEST",
		"required_context_keys": []string{"brief", "note"}, "title_pattern": "SERVICE_ORDER", "service_label": "Synthetic service"}
	raw, _ := json.Marshal(cfg)
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatal(err)
	}
	type version struct {
		Case, Entry string
		Ref         preparecase.Ref
	}
	out := struct {
		Case, OtherCase, NeighborCase, Lineage, KnownTask string
		History, Context, Shared, Other, Neighbor         version
	}{Case: id(), OtherCase: id(), NeighborCase: id(), Lineage: id(), KnownTask: "synthetic_known_task"}
	setup := func(ws string, cases []string, visit func(func(string, ...any))) {
		tx, err := scoped(t.Context(), db, ws)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(t.Context())
		exec := func(sql string, args ...any) {
			if _, err := tx.Exec(t.Context(), sql, args...); err != nil {
				t.Fatal(err)
			}
		}
		exec("INSERT INTO hp_processing.operations VALUES($1,$2,$3,1,'ACTIVE',$4,NULL,NULL,NULL)", ws, op, principal, now.Add(-time.Hour))
		exec("INSERT INTO hp_processing.accepted_configs VALUES($1,$2,$3,'1.0','paid-order-v1',$4,$5,$6,$7,'synthetic')",
			ws, config, op, string(raw), intent.Digest(canonical), principal, now)
		exec("UPDATE hp_processing.operations SET selected_config_id=$2 WHERE workspace_id=$1", ws, config)
		for _, c := range cases {
			exec(`INSERT INTO hp_processing.case_identity
(workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,
 ever_dispatch_fenced,outcome_knowledge,lineage_id,known_task_id,last_confirmed_at)
VALUES($1,$2,$3,$4,'acct_synthetic',false,$5,$6,$7,'CONFIRMED',1,true,'RESULT_VERIFIED',$8,$9,$6)`,
				ws, c, op, config, "syn_"+c, now, principal, out.Lineage, out.KnownTask)
		}
		visit(exec)
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	write := func(ws, c, class string, body []byte) preparecase.Ref {
		ref, err := files.Write(t.Context(), body, class, preparecase.Origin{WorkspaceID: ws, CaseID: c, Revision: 1}, expiry, now)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	history := func(ws, c string, exec func(string, ...any)) version {
		entry := id()
		body, _ := json.Marshal(struct {
			CaseID   string
			Revision int64
			Decision preparecase.Decision
		}{c, 1, preparecase.Decision{Reason: "DELIVERY_RESULT_VERIFIED", State: "CONFIRMED", At: now}})
		ref := write(ws, c, "HISTORY", body)
		exec(`INSERT INTO hp_processing.safety_history (workspace_id,entry_id,case_id,actor_kind,at,reason_code,old_state,new_state,proof_ref)
VALUES($1,$2,$3,'SYSTEM',$4,'DELIVERY_RESULT_VERIFIED','WORKING','CONFIRMED',$5)`, ws, entry, c, now, ref.ID)
		exec("INSERT INTO hp_processing.history_content VALUES($1,$2,$3,$4,$5,$6,$7)",
			ws, entry, ref.ID, ref.Digest, ref.Bytes, now, expiry)
		return version{c, entry, ref}
	}
	contextRow := func(exec func(string, ...any), ws, c, key string, ref preparecase.Ref) version {
		entry := id()
		exec("INSERT INTO hp_processing.case_content VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)",
			ws, entry, c, key, ref.ID, ref.Digest, ref.Bytes, principal, now, expiry)
		return version{c, entry, ref}
	}
	setup(workspace, []string{out.Case, out.OtherCase}, func(exec func(string, ...any)) {
		out.History = history(workspace, out.Case, exec)
		out.Other = history(workspace, out.OtherCase, exec)
		out.Context = contextRow(exec, workspace, out.Case, "brief", write(workspace, out.Case, "CASE_CONTEXT", []byte(`"synthetic private target"`)))
		out.Shared = contextRow(exec, workspace, out.Case, "note", write(workspace, out.Case, "CASE_CONTEXT", []byte(`"synthetic shared target"`)))
		contextRow(exec, workspace, out.OtherCase, "note", out.Shared.Ref)
	})
	setup(neighbor, []string{out.NeighborCase}, func(exec func(string, ...any)) {
		out.Neighbor = history(neighbor, out.NeighborCase, exec)
	})
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ".hit148-synthetic-index"), body, 0600); err != nil {
		t.Fatal(err)
	}
}

// TestHIT148CopySQL uses the accepted HIT-128 app-role recovery projection.
// An independent temporary PostgreSQL database holds the pre-deletion copy.
func TestHIT148CopySQL(t *testing.T) {
	phase := os.Getenv("HIT148_COPY_PHASE")
	if phase == "" || os.Getenv("HIT78_ISOLATED_PG") != "1" {
		t.Skip("requires the isolated HIT-148 runner")
	}
	open := func(db string) *pgx.Conn {
		conn, err := pgx.Connect(t.Context(), "host=/var/run/postgresql "+db+" sslmode=disable user=hp_processing_app")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(context.Background()) })
		return conn
	}
	source := open("dbname=hit148_source")
	backup := open("dbname=hit148_backup")
	restored := open("dbname=hit148restore")
	for _, ws := range []string{os.Getenv("HIT148_WORKSPACE"), os.Getenv("HIT148_NEIGHBOR")} {
		if !readcases.UUID(ws) {
			t.Fatal("invalid synthetic workspace")
		}
		switch phase {
		case "backup":
			if err := applyRecovery(backup, ws, captureRecovery(t, source, ws)); err != nil {
				t.Fatal(err)
			}
		case "restore":
			if err := applyRecovery(restored, ws, captureRecovery(t, backup, ws)); err != nil {
				t.Fatal(err)
			}
		case "verify":
			digest := func(parts []recoveryCopy) string {
				var canonical []string
				for _, part := range parts {
					for _, row := range part.rows {
						raw, err := json.Marshal(row)
						if err != nil {
							t.Fatal(err)
						}
						canonical = append(canonical, part.table.name+":"+string(raw))
					}
				}
				slices.Sort(canonical)
				sum := sha256.Sum256([]byte(strings.Join(canonical, "\n")))
				return hex.EncodeToString(sum[:])
			}
			original := digest(captureRecovery(t, backup, ws))
			if digest(captureRecovery(t, source, ws)) != original || digest(captureRecovery(t, restored, ws)) != original {
				t.Fatal("SQL evidence, case identity, action fence or original expiry changed during deletion")
			}
			t.Logf("source, pre-deletion backup and restored SQL projections match %s", original)
		default:
			t.Fatal("unknown fixture phase")
		}
	}
}
