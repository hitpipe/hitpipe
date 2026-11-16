// Package authorization reads current IAM-owned authority. Its workspace GUC is
// an SQL isolation selector, not a grant or proof of a user's business rights.
package authorization

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrConfiguration  = errors.New("authorization_reader_configuration_invalid")
	ErrInvalidRequest = errors.New("authorization_reader_request_invalid")
	ErrUnavailable    = errors.New("authorization_reader_unavailable")
)

const queryTimeout = 2 * time.Second

// Inspect the actual runtime login and database. A worker, purger, migrator,
// administrator, SET ROLE impersonation or mixed-purpose database is not the IAM
// application authority. No credentials or connection values leave this check.
const checkAuthority = `SELECT current_user=session_user
 AND (SELECT rolcanlogin AND NOT(rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication)
      FROM pg_roles WHERE rolname=current_user)
 AND pg_has_role(current_user,'hp_iam_app','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_worker','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_purge','MEMBER')
 AND NOT pg_has_role(current_user,'hp_iam_migrator','MEMBER')
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname=current_user)
                AND roleid<>(SELECT oid FROM pg_roles WHERE rolname='hp_iam_app'))
 AND (SELECT count(*)=4 FROM pg_roles WHERE rolname IN('hp_iam_app','hp_iam_worker','hp_iam_purge','hp_iam_migrator')
      AND NOT(rolcanlogin OR rolsuper OR rolbypassrls OR rolcreatedb OR rolcreaterole OR rolreplication))
 AND NOT EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_app'))
 AND NOT has_database_privilege(current_user,current_database(),'CREATE')
 AND NOT has_schema_privilege(current_user,'hp_iam','CREATE')
 AND EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='hp_iam' AND nspowner=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator'))
 AND (SELECT count(*)=17 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
      WHERE n.nspname='hp_iam' AND c.relkind='r' AND c.relowner=(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator'))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
                WHERE n.nspname='hp_iam' AND (c.relkind IN('p','f') OR c.relkind='r' AND c.relowner<>(SELECT oid FROM pg_roles WHERE rolname='hp_iam_migrator')))
 AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
                WHERE c.relkind IN('r','p','f') AND n.nspname NOT IN('hp_iam','pg_catalog','information_schema')
                AND n.nspname NOT LIKE 'pg_toast%' AND n.nspname NOT LIKE 'pg_temp_%')
 AND (SELECT count(*)=1 AND min(version)=1 FROM hp_iam.schema_version)
 AND NOT EXISTS(SELECT 1 FROM unnest(ARRAY['principals','sessions','workspaces','memberships','revocations','runtime_registry','external_bindings']) AS t(name)
                WHERE NOT has_table_privilege(current_user,'hp_iam.'||name,'SELECT'))`

const selectSession = `SELECT s.session_id::text,s.principal_id::text,s.epoch,
 s.created_at,s.expires_at,s.idle_expires_at,s.revoked_at IS NOT NULL,p.disabled_at IS NOT NULL
 FROM hp_iam.sessions s JOIN hp_iam.principals p USING(principal_id) WHERE s.session_hash=$1`
const selectWorkspace = `SELECT workspace_id::text,lifecycle,security_epoch FROM hp_iam.workspaces WHERE workspace_id=$1`
const selectMembership = `SELECT workspace_id::text,principal_id::text,role,status,revision,revoked_at IS NOT NULL
 FROM hp_iam.memberships WHERE workspace_id=$1 AND principal_id=$2`
const selectRuntime = `SELECT runtime_id::text,service,boot_id::text,generation,certificate_fingerprint,status
 FROM hp_iam.runtime_registry WHERE runtime_id=$1`
const selectBinding = `SELECT grant_id::text,workspace_id::text,status,security_epoch
 FROM hp_iam.external_bindings WHERE workspace_id=$1 AND grant_id=$2`

// Preserve every scope/subject/epoch independently. MEMBERSHIP subjects have no
// canonical UUID mapping in this schema; the application conservatively handles
// relevant workspace revocations rather than inventing a principal mapping.
const selectRevocations = `SELECT scope,subject_id::text,coalesce(workspace_id::text,''),epoch,status
 FROM hp_iam.revocations WHERE workspace_id=$1 OR (scope='SESSION' AND subject_id=$2)
 ORDER BY scope,epoch,revocation_id`

// Reader borrows the caller's bounded IAM application pool. It never closes the
// pool, keeps no grant cache and retains no transaction between Read calls. The
// caller owns encrypted transport and must not enable SQL-argument logging.
type Reader struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) (*Reader, error) {
	if pool == nil || pool.Config().MaxConns < 1 || pool.Config().MaxConns > 10 {
		return nil, ErrConfiguration
	}
	return &Reader{pool: pool}, nil
}

// Read uses one fresh READ ONLY REPEATABLE READ snapshot and one two-second
// budget, including pool acquisition, role checks, SQL and commit. The enclosing
// application actual-start gate must hold through these reads and the decision;
// a database snapshot alone is not a distributed revocation barrier.
func (r *Reader) Read(ctx context.Context, request authorizeaction.ReadRequest) (authorizeaction.Snapshot, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return authorizeaction.Snapshot{}, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return authorizeaction.Snapshot{}, err
	}
	digest, ok := referenceDigest(request.Reference)
	if !ok || !canonicalUUID(request.WorkspaceID) || !canonicalUUID(request.RuntimeID) || request.BindingGrantID != "" && !canonicalUUID(request.BindingGrantID) {
		return authorizeaction.Snapshot{}, ErrInvalidRequest
	}
	request.WorkspaceID = strings.ToLower(request.WorkspaceID)
	request.RuntimeID = strings.ToLower(request.RuntimeID)
	request.BindingGrantID = strings.ToLower(request.BindingGrantID)
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	failed := func() (authorizeaction.Snapshot, error) {
		if err := bounded.Err(); err != nil {
			return authorizeaction.Snapshot{}, err
		}
		return authorizeaction.Snapshot{}, ErrUnavailable
	}
	tx, err := r.pool.BeginTx(bounded, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return failed()
	}
	defer func() { _ = tx.Rollback(bounded) }()
	var authorized bool
	if err := tx.QueryRow(bounded, checkAuthority).Scan(&authorized); err != nil || !authorized {
		if bounded.Err() != nil {
			return failed()
		}
		return authorizeaction.Snapshot{}, ErrConfiguration
	}
	// Parameterized, transaction-local scope cannot escape into another pooled
	// request. This selector does not establish membership or resource ownership.
	if _, err := tx.Exec(bounded, `SELECT set_config('hp.workspace_id',$1,true)`, request.WorkspaceID); err != nil {
		return failed()
	}
	var snapshot authorizeaction.Snapshot
	err = tx.QueryRow(bounded, selectSession, digest[:]).Scan(&snapshot.Session.ID, &snapshot.Session.PrincipalID, &snapshot.Session.Epoch, &snapshot.Session.Policy.CreatedAt, &snapshot.Session.Policy.ExpiresAt, &snapshot.Session.Policy.IdleExpiresAt, &snapshot.Session.Policy.Revoked, &snapshot.Principal.Disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Commit(bounded); err != nil {
			return failed()
		}
		if bounded.Err() != nil {
			return failed()
		}
		return snapshot, nil
	}
	if err != nil {
		return failed()
	}
	snapshot.Session.Found, snapshot.Session.Policy.Found, snapshot.Principal.Found = true, true, true
	snapshot.Principal.ID = snapshot.Session.PrincipalID
	err = tx.QueryRow(bounded, selectWorkspace, request.WorkspaceID).Scan(&snapshot.Workspace.ID, &snapshot.Workspace.Lifecycle, &snapshot.Workspace.Epoch)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return failed()
	}
	snapshot.Workspace.Found = err == nil
	err = tx.QueryRow(bounded, selectMembership, request.WorkspaceID, snapshot.Principal.ID).Scan(&snapshot.Membership.WorkspaceID, &snapshot.Membership.PrincipalID, &snapshot.Membership.Role, &snapshot.Membership.Status, &snapshot.Membership.Revision, &snapshot.Membership.Revoked)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return failed()
	}
	snapshot.Membership.Found = err == nil
	err = tx.QueryRow(bounded, selectRuntime, request.RuntimeID).Scan(&snapshot.Runtime.Caller.RuntimeID, &snapshot.Runtime.Caller.Service, &snapshot.Runtime.Caller.BootID, &snapshot.Runtime.Caller.Generation, &snapshot.Runtime.Caller.CertificateFingerprint, &snapshot.Runtime.Status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return failed()
	}
	snapshot.Runtime.Found = err == nil
	if request.BindingGrantID != "" {
		err = tx.QueryRow(bounded, selectBinding, request.WorkspaceID, request.BindingGrantID).Scan(&snapshot.Binding.GrantID, &snapshot.Binding.WorkspaceID, &snapshot.Binding.Status, &snapshot.Binding.Epoch)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return failed()
		}
		snapshot.Binding.Found = err == nil
	}
	rows, err := tx.Query(bounded, selectRevocations, request.WorkspaceID, snapshot.Session.ID)
	if err != nil {
		return failed()
	}
	for rows.Next() {
		var revoke authorizeaction.Revocation
		if err := rows.Scan(&revoke.Scope, &revoke.SubjectID, &revoke.WorkspaceID, &revoke.Epoch, &revoke.Status); err != nil {
			rows.Close()
			return failed()
		}
		snapshot.Revocations = append(snapshot.Revocations, revoke)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return failed()
	}
	if err := tx.Commit(bounded); err != nil {
		return failed()
	}
	if bounded.Err() != nil {
		return failed()
	}
	return snapshot, nil
}

func referenceDigest(value string) ([32]byte, bool) {
	var digest [32]byte
	if len(value) != hex.EncodedLen(len(digest)) {
		return digest, false
	}
	for _, c := range []byte(value) {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return digest, false
		}
	}
	_, err := hex.Decode(digest[:], []byte(value))
	return digest, err == nil
}
func canonicalUUID(value string) bool {
	if len(value) != 36 || value == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	for n, c := range []byte(value) {
		if n == 8 || n == 13 || n == 18 || n == 23 {
			if c != '-' {
				return false
			}
		} else if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

var _ authorizeaction.Reader = (*Reader)(nil)
