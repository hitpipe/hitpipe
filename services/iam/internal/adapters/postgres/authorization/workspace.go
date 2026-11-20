package authorization

import (
	"context"
	"errors"
	"strings"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/jackc/pgx/v5"
)

// WorkspaceEpoch returns only the current IAM security epoch for choosing the
// policy-read scope of the local gate. It does not read a session, membership or
// business resource, or establish access to the workspace.
func (r *Reader) WorkspaceEpoch(ctx context.Context, workspaceID string) (int64, bool, error) {
	if r == nil || r.pool == nil || ctx == nil || !canonicalUUID(workspaceID) {
		return 0, false, ErrInvalidRequest
	}
	bounded, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	tx, err := r.pool.BeginTx(bounded, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return 0, false, ErrUnavailable
	}
	defer func() { _ = tx.Rollback(bounded) }()
	var authorized bool
	if err := tx.QueryRow(bounded, checkAuthority).Scan(&authorized); err != nil || !authorized {
		return 0, false, ErrUnavailable
	}
	workspaceID = strings.ToLower(workspaceID)
	if _, err := tx.Exec(bounded, `SELECT set_config('hp.workspace_id',$1,true)`, workspaceID); err != nil {
		return 0, false, ErrUnavailable
	}
	var epoch int64
	err = tx.QueryRow(bounded, `SELECT security_epoch FROM hp_iam.workspaces WHERE workspace_id=$1`, workspaceID).Scan(&epoch)
	found := err == nil
	if !found && !errors.Is(err, pgx.ErrNoRows) || found && (epoch < 1 || epoch > authorizeaction.MaxWireInteger) {
		return 0, false, ErrUnavailable
	}
	if err := tx.Commit(bounded); err != nil || bounded.Err() != nil {
		return 0, false, ErrUnavailable
	}
	return epoch, found, nil
}
