package postgres

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// These projections are the local qualification inventory. They deliberately
// omit drafts, notifications, runtime leases and the rebuildable job index.
// Content columns are opaque refs and their original integrity/expiry metadata.
type recoveryTable struct {
	name, columns, read string
}

var recoveryTables = []recoveryTable{
	{"operations", "workspace_id,operation_id,owner_user_id,revision,state,first_activation_at,selected_config_id,pause_accepted_at,drain_deadline_at", ""},
	{"accepted_configs", "workspace_id,config_id,operation_id,schema_version,evaluator_version,canonical_config,digest,accepted_by,accepted_at,rehearsal_digest", ""},
	{"case_identity", "workspace_id,case_id,operation_id,config_id,source_account_id,livemode,session_id,admitted_at,owner_user_id,state,revision,ever_dispatch_fenced,outcome_knowledge,lineage_id,known_task_id,last_confirmed_at", ""},
	{"episodes", "workspace_id,episode_id,case_id,started_at,deadline_at,automatic_writes,qualified_manual_command_id,completed_at", ""},
	{"case_content", "workspace_id,content_id,case_id,field_key,value_ref,value_sha256,serialized_value_bytes,author_user_id,added_at,expires_at", ""},
	{"safety_history", "workspace_id,entry_id,case_id,actor_kind,actor_id,at,reason_code,old_state,new_state,proof_ref", ""},
	{"history_content", "workspace_id,entry_id,detail_ref,detail_sha256,detail_bytes,created_at,expires_at", ""},
	{"user_commands", "workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at", ""},
	{"inbox", "workspace_id,message_id,digest,disposition,record_id,received_at", ""},
	{
		"outbox",
		"workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at,state,claim_token,lease_until,delivery_count",
		"workspace_id,message_id,kind,payload_ref,payload_sha256,payload_bytes,digest,created_at,next_attempt_at,expires_at," +
			"CASE WHEN state IN ('PENDING','CLAIMED') THEN 'HELD' ELSE state END,NULL::uuid,NULL::timestamptz,delivery_count",
	},
}

const recoveryRowLimit = 1000 // bounded, local fixture only

type recoveryCopy struct {
	table recoveryTable
	rows  [][]any
}

func captureRecovery(t *testing.T, source *pgx.Conn, workspace string) []recoveryCopy {
	t.Helper()
	tx, err := source.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", workspace); err != nil {
		t.Fatal(err)
	}
	result := make([]recoveryCopy, 0, len(recoveryTables))
	for _, table := range recoveryTables {
		projection := table.read
		if projection == "" {
			projection = table.columns
		}
		// Tables and projections above are fixed in the test. RLS permits only
		// the explicit scope set for this transaction.
		rows, err := tx.Query(ctx, "SELECT "+projection+" FROM hp_processing."+table.name+" LIMIT $1", recoveryRowLimit+1)
		if err != nil {
			t.Fatalf("%s: %v", table.name, err)
		}
		data, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([]any, error) {
			return row.Values()
		})
		if err != nil || len(data) > recoveryRowLimit {
			t.Fatalf("%s: scope exceeds the local qualification or cannot be read (%v)", table.name, err)
		}
		result = append(result, recoveryCopy{table, data})
		t.Logf("scoped SELECT hp_processing.%s: rows=%d", table.name, len(data))
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return result
}

var errRecoveryNotEmpty = errors.New("target workspace is not empty")

func applyRecovery(target *pgx.Conn, workspace string, archive []recoveryCopy) error {
	tx, err := target.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(128, 1)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", workspace); err != nil {
		return err
	}
	// The restore is only for the new isolated target. Refuse a second import
	// or a target that already has live safety/expiry state in this scope.
	for _, table := range recoveryTables {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM hp_processing."+table.name+")").Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("%w: %s", errRecoveryNotEmpty, table.name)
		}
	}
	// Insert through ordinary RLS and table constraints, in a single
	// transaction. The selected_config FK is already deferred by the schema.
	for _, part := range archive {
		table := part.table
		markers := make([]string, len(strings.Split(table.columns, ",")))
		for i := range markers {
			markers[i] = "$" + strconv.Itoa(i+1)
		}
		insert := "INSERT INTO hp_processing." + table.name + " (" + table.columns + ") VALUES (" + strings.Join(markers, ",") + ")"
		for _, row := range part.rows {
			tag, err := tx.Exec(ctx, insert, row...)
			if err != nil {
				return fmt.Errorf("restore %s: %w", table.name, err)
			}
			if tag.RowsAffected() != 1 {
				return fmt.Errorf("restore %s: row not inserted", table.name)
			}
		}
	}
	return tx.Commit(ctx)
}
