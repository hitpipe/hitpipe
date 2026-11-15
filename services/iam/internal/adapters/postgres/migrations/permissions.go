package migrations

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// checkPermissions rejects drift in the access rules of schema v1. It never
// repairs an installed schema or changes the accepted migration bytes.
func checkPermissions(ctx context.Context, conn *pgx.Conn) error {
	tenant := map[string]bool{
		"workspaces": true, "memberships": true, "owner_transfers": true,
		"external_bindings": true, "deletion_overlays": true, "deletion_receipts": true,
		"support_grants": true, "command_receipts": true, "audit_minimal": true, "outbox": true,
	}
	tables := []string{
		"schema_version", "principals", "sessions", "workspaces", "memberships",
		"owner_transfers", "external_bindings", "revocations", "runtime_registry",
		"revocation_participants", "deletion_overlays", "deletion_receipts", "support_grants",
		"command_receipts", "audit_minimal", "outbox", "job_index",
	}
	roles := []string{"hp_iam_app", "hp_iam_worker", "hp_iam_purge"}
	const policyExpression = "(workspace_id = (NULLIF(current_setting('hp.workspace_id'::text, true), ''::text))::uuid)"
	rows, err := conn.Query(ctx, `SELECT c.relname,c.relrowsecurity,c.relforcerowsecurity,
		(SELECT count(*) FROM pg_policy p WHERE p.polrelid=c.oid),
		NOT EXISTS(SELECT 1 FROM pg_policy p WHERE p.polrelid=c.oid AND
			(p.polname<>'tenant' OR p.polcmd<>'*' OR NOT p.polpermissive
			OR p.polroles<>ARRAY[0::oid]
			OR pg_get_expr(p.polqual,p.polrelid) IS DISTINCT FROM $2::text
			OR pg_get_expr(p.polwithcheck,p.polrelid) IS DISTINCT FROM $2::text)),
		NOT EXISTS(SELECT 1 FROM aclexplode(c.relacl) a
			WHERE a.grantee<>c.relowner AND
			(a.is_grantable OR a.grantee NOT IN
				(SELECT oid FROM pg_roles WHERE rolname=ANY($3::text[])))),
		NOT EXISTS(SELECT 1 FROM pg_attribute a WHERE a.attrelid=c.oid
			AND a.attacl IS NOT NULL AND cardinality(a.attacl)>0)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='hp_iam' AND c.relkind='r' AND c.relname=ANY($1::text[])`,
		tables, policyExpression, roles)
	if err != nil {
		return ErrSchema
	}
	count := 0
	for rows.Next() {
		var table string
		var enabled, forced, policyMatches, safeACL, noColumnACL bool
		var policies int
		if err := rows.Scan(&table, &enabled, &forced, &policies, &policyMatches, &safeACL, &noColumnACL); err != nil {
			rows.Close()
			return ErrSchema
		}
		isTenant := tenant[table]
		wantPolicies := 0
		if isTenant {
			wantPolicies = 1
		}
		if enabled != isTenant || forced != isTenant || policies != wantPolicies ||
			!policyMatches || !safeACL || !noColumnACL {
			rows.Close()
			return ErrSchema
		}
		count++
	}
	if rows.Err() != nil || count != len(tables) {
		rows.Close()
		return ErrSchema
	}
	rows.Close()

	var schemaSafe bool
	err = conn.QueryRow(ctx, `SELECT
		NOT EXISTS(SELECT 1 FROM aclexplode(n.nspacl) a WHERE a.grantee<>n.nspowner AND
			(a.is_grantable OR a.grantee NOT IN
				(SELECT oid FROM pg_roles WHERE rolname=ANY($1::text[]))))
		AND (SELECT count(*)=3 FROM pg_roles r WHERE r.rolname=ANY($1::text[])
			AND has_schema_privilege(r.oid,n.oid,'USAGE')
			AND NOT has_schema_privilege(r.oid,n.oid,'CREATE'))
		FROM pg_namespace n WHERE n.nspname='hp_iam'`, roles).Scan(&schemaSafe)
	if err != nil || !schemaSafe {
		return ErrSchema
	}

	// Check effective privileges as well as ACL recipients: PUBLIC grants and
	// inherited privileges must not widen a runtime role's rights. Include all
	// table privileges supported by the accepted PostgreSQL 18 cohort.
	rows, err = conn.Query(ctx, `SELECT c.relname,r.rolname,
		(CASE WHEN has_table_privilege(r.oid,c.oid,'SELECT') THEN 1 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'INSERT') THEN 2 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'UPDATE') THEN 4 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'DELETE') THEN 8 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'TRUNCATE') THEN 16 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'REFERENCES') THEN 32 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'TRIGGER') THEN 64 ELSE 0 END)+
		(CASE WHEN has_table_privilege(r.oid,c.oid,'MAINTAIN') THEN 128 ELSE 0 END)
		FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		CROSS JOIN pg_roles r
		WHERE n.nspname='hp_iam' AND c.relkind='r'
		AND c.relname=ANY($1::text[]) AND r.rolname=ANY($2::text[])`, tables, roles)
	if err != nil {
		return ErrSchema
	}
	defer rows.Close()
	count = 0
	for rows.Next() {
		var table, role string
		var got int
		if err := rows.Scan(&table, &role, &got); err != nil {
			return ErrSchema
		}
		want := 7 // SELECT, INSERT, UPDATE.
		if role == "hp_iam_purge" {
			want = 9 // SELECT, DELETE.
		} else if table == "command_receipts" || table == "audit_minimal" {
			want = 3 // Append-only: SELECT, INSERT.
		}
		if table == "schema_version" {
			want = 1
		} else if table == "job_index" {
			want = 15
			if role == "hp_iam_app" {
				want = 0
			}
		}
		if got != want {
			return ErrSchema
		}
		count++
	}
	if rows.Err() != nil || count != len(tables)*len(roles) {
		return ErrSchema
	}
	return nil
}
