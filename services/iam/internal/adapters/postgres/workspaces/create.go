package workspaces

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/jackc/pgx/v5"
)

const receiptKind = "WORKSPACE"

const selectReceipt = `SELECT body_digest,result_id::text,result_kind
 FROM hp_iam.command_receipts WHERE workspace_id=$1 AND actor_id=$2 AND request_id=$3`

// Create uses the approved workspace ID supplied by the onboarding authority.
// The command receipt lives in that real scope. A new workspace cannot be
// allocated by interpreting a missing or ambiguous commit response.
func (s *Store) Create(ctx context.Context, who app.Identity, change app.Change, prepare func(context.Context) (app.Material, error)) (app.Row, error) {
	if s == nil || s.pool == nil || ctx == nil || ctx.Err() != nil || prepare == nil {
		return app.Row{}, app.ErrUnavailable
	}
	if !validChange(change) {
		return app.Row{}, app.ErrInvalid
	}
	workspace := strings.ToLower(change.WorkspaceID)
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	tx, err := s.begin(ctx)
	if err != nil {
		return app.Row{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := s.confirmIdentity(ctx, tx, who); err != nil {
		return app.Row{}, err
	}
	guarded, cancelSession, err := s.sessionDeadline(ctx, who)
	if err != nil {
		return app.Row{}, err
	}
	defer cancelSession()
	if err := selectScope(guarded, tx, workspace); err != nil {
		return app.Row{}, err
	}
	// PostgreSQL locks the key even before it has a row. A collision can only
	// cause temporary contention; the receipt is selected by the exact IDs.
	if _, err := tx.Exec(guarded, `SELECT pg_advisory_xact_lock(hashtextextended($1::text,124))`, workspace); err != nil {
		return app.Row{}, app.ErrUnavailable
	}

	var bodyDigest, resultID, kind string
	err = tx.QueryRow(guarded, selectReceipt, workspace, who.PrincipalID, change.RequestID).Scan(&bodyDigest, &resultID, &kind)
	if err == nil {
		if !runtimeaccess.ValidFingerprint(bodyDigest) || resultID != workspace || kind != receiptKind {
			return app.Row{}, app.ErrStale
		}
		row, found, err := readWorkspace(guarded, tx, who.PrincipalID, workspace)
		if err != nil {
			return app.Row{}, err
		}
		if !found || row.Role != "OWNER" || row.Lifecycle != "ACTIVE" {
			return app.Row{}, app.ErrDenied
		}
		if _, err := s.liveNow(guarded, who); err != nil {
			return app.Row{}, err
		}
		if bodyDigest != change.BodyDigest {
			return app.Row{}, app.ErrConflict
		}
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return app.Row{}, app.ErrUnavailable
	}

	var unavailable bool
	err = tx.QueryRow(guarded, `SELECT EXISTS(SELECT 1 FROM hp_iam.workspaces WHERE workspace_id=$1)
 OR EXISTS(SELECT 1 FROM hp_iam.revocations
           WHERE (scope='WORKSPACE' AND (workspace_id=$1 OR subject_id=$1))
              OR (scope='MEMBERSHIP' AND (workspace_id=$1 OR workspace_id IS NULL)))`, workspace).Scan(&unavailable)
	if err != nil {
		return app.Row{}, app.ErrUnavailable
	}
	if unavailable {
		return app.Row{}, app.ErrStale
	}
	now, err := s.liveNow(guarded, who)
	if err != nil {
		return app.Row{}, err
	}
	if change.OwnerConfirmedAt.After(now) || !change.PayloadExpiresAt.After(now) {
		return app.Row{}, app.ErrDenied
	}
	// Prepare durable owner objects only for a new command after its exact
	// scope is locked. A replay or conflict may neither touch nor require them.
	material, err := prepare(guarded)
	if err != nil {
		return app.Row{}, err
	}
	if !validMaterial(material) {
		return app.Row{}, app.ErrUnavailable
	}
	now, err = s.liveNow(guarded, who)
	if err != nil {
		return app.Row{}, err
	}
	if !change.PayloadExpiresAt.After(now) {
		return app.Row{}, app.ErrDenied
	}
	if err := insertCreation(guarded, tx, who.PrincipalID, workspace, change, material, now); err != nil {
		return app.Row{}, err
	}
	commit, cancelCommit, err := s.sessionDeadline(guarded, who)
	if err != nil {
		return app.Row{}, err
	}
	defer cancelCommit()
	if err := classifyCommit(tx.Commit(commit)); err != nil {
		return app.Row{}, err
	}
	return app.Row{WorkspaceID: workspace, NameRef: strings.ToLower(material.NameRef), Role: "OWNER",
		Lifecycle: change.Lifecycle, Revision: 1, SecurityEpoch: 1, MembershipRevision: 1}, nil
}

func insertCreation(ctx context.Context, tx pgx.Tx, principal, workspace string, change app.Change, material app.Material, now time.Time) error {
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO hp_iam.workspaces
 (workspace_id,name_content_ref,revision,security_epoch,lifecycle,created_at,owner_confirmed_at)
 VALUES($1,$2,1,1,$3,$4,$5)`,
		workspace, material.NameRef, change.Lifecycle, now, change.OwnerConfirmedAt)
	batch.Queue(`INSERT INTO hp_iam.memberships
 (workspace_id,principal_id,role,status,revision,accepted_at) VALUES($1,$2,'OWNER','ACTIVE',1,$3)`,
		workspace, principal, change.OwnerConfirmedAt)
	batch.Queue(`INSERT INTO hp_iam.command_receipts
 (workspace_id,actor_id,request_id,body_digest,result_id,result_kind,created_at)
 VALUES($1,$2,$3,$4,$1,$5,$6)`,
		workspace, principal, change.RequestID, change.BodyDigest, receiptKind, now)
	batch.Queue(`INSERT INTO hp_iam.audit_minimal
 (workspace_id,entry_id,actor_id,action_code,resource_id,at,reason_code)
 VALUES($1,$2,$3,$4,$1,$5,$6)`,
		workspace, material.AuditID, principal, change.AuditAction, now, change.AuditReason)
	batch.Queue(`INSERT INTO hp_iam.outbox
 (workspace_id,message_id,digest,kind,payload_content_ref,payload_expires_at,created_at,state,next_attempt_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,'PENDING',$7)`,
		workspace, material.MessageID, material.MessageDigest, change.MessageKind, material.MessageRef, change.PayloadExpiresAt, now)
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return app.ErrUnavailable
	}
	return nil
}

func classifyCommit(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrTxCommitRollback) {
		return app.ErrUnavailable
	}
	// Losing the commit acknowledgment can leave the complete command durable.
	// No SQL or driver errors are disclosed to the caller or logged here.
	return app.ErrUncertain
}

func validChange(change app.Change) bool {
	for _, id := range []string{change.WorkspaceID, change.RequestID} {
		if !runtimeaccess.ValidUUID(id) {
			return false
		}
	}
	for _, label := range []string{change.AuditAction, change.AuditReason, change.MessageKind} {
		if len(label) < 1 || len(label) > 128 {
			return false
		}
	}
	for _, date := range []time.Time{change.OwnerConfirmedAt, change.PayloadExpiresAt} {
		if date.IsZero() || date.UTC().Year() < 1 || date.UTC().Year() > 9999 {
			return false
		}
	}
	return change.Lifecycle == "ACTIVE" && change.PayloadExpiresAt.After(change.OwnerConfirmedAt) &&
		runtimeaccess.ValidFingerprint(change.BodyDigest)
}

func validMaterial(material app.Material) bool {
	for _, id := range []string{material.NameRef, material.AuditID, material.MessageID, material.MessageRef} {
		if !runtimeaccess.ValidUUID(id) || id[14] != '4' || !strings.ContainsRune("89abAB", rune(id[19])) {
			return false
		}
	}
	return runtimeaccess.ValidFingerprint(material.MessageDigest)
}
