package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

var ErrDeletion = errors.New("case deletion scope or durable proof rejected")

type deletionApplication struct {
	DecisionID      string                        `json:"deletion_id"`
	Scope           deletionregistry.Scope        `json:"scope"`
	ScopeDigest     string                        `json:"scope_digest"`
	ConfirmationRef string                        `json:"confirmation_ref"`
	Generation      uint64                        `json:"generation"`
	Revision        uint64                        `json:"apply_revision"`
	AppliedAt       time.Time                     `json:"applied_at"`
	Targets         []contentfiles.DeletionTarget `json:"targets"`
}

func (a deletionApplication) receipt() (deletionregistry.Receipt, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return deletionregistry.Receipt{}, err
	}
	return deletionregistry.Receipt{
		Environment: "test", WorkspaceID: a.Scope.WorkspaceID, DeletionID: a.DecisionID,
		ScopeDigest: a.ScopeDigest, Generation: a.Generation, Revision: a.Revision, AppliedAt: a.AppliedAt,
		ApplicationRef: fmt.Sprintf("sha256:%x", sha256.Sum256(raw)),
	}, nil
}

func (s PurgeStore) deletionScope(ctx context.Context, workspace string) (pgx.Tx, error) {
	if s.DB == nil || !readcases.UUID(workspace) {
		return nil, ErrDeletion
	}
	tx, err := s.begin(ctx, workspace)
	if err != nil {
		return nil, err
	}
	var scoped bool
	err = tx.QueryRow(ctx, `SELECT row_security_active('hp_processing.case_identity')
 AND row_security_active('hp_processing.accepted_configs')`).Scan(&scoped)
	if err == nil && scoped {
		_, err = tx.Exec(ctx, `LOCK TABLE hp_processing.case_identity, hp_processing.case_content, hp_processing.drafts,
 hp_processing.history_content, hp_processing.outbox, hp_processing.safety_history IN SHARE MODE NOWAIT`)
		if err == nil {
			return tx, nil
		}
	}
	_ = tx.Rollback(ctx)
	return nil, ErrDeletion
}

const contextVersion = `SELECT content_id::text,value_sha256,serialized_value_bytes,expires_at FROM hp_processing.case_content
 WHERE workspace_id=$1 AND case_id=$2 AND value_ref=$3 LIMIT 2`

const historyVersion = `SELECT h.entry_id::text,c.detail_sha256,c.detail_bytes,c.expires_at
 FROM hp_processing.history_content c JOIN hp_processing.safety_history h USING(workspace_id,entry_id)
 WHERE h.workspace_id=$1 AND h.case_id=$2 AND c.detail_ref=$3 LIMIT 2`

const hasOtherLiveReference = `SELECT EXISTS (
 SELECT 1 FROM hp_processing.case_content WHERE workspace_id=$1 AND value_ref=$2
   AND ($3<>'CASE_CONTEXT' OR content_id<>$4) AND expires_at>$5
 UNION ALL
 SELECT 1 FROM hp_processing.drafts WHERE workspace_id=$1 AND prose_ref=$2
   AND (prose_expires_at IS NULL OR prose_expires_at>$5 OR expires_at>$5)
 UNION ALL
 SELECT 1 FROM hp_processing.history_content WHERE workspace_id=$1 AND detail_ref=$2
   AND ($3<>'HISTORY' OR entry_id<>$4) AND expires_at>$5
 UNION ALL
 SELECT 1 FROM hp_processing.outbox WHERE workspace_id=$1 AND payload_ref=$2
   AND (expires_at>$5 OR state IN ('PENDING','CLAIMED'))
 UNION ALL
 SELECT 1 FROM hp_processing.safety_history h WHERE h.workspace_id=$1 AND h.proof_ref=$2
   AND NOT ($3='HISTORY' AND h.entry_id=$4)
   AND NOT EXISTS (SELECT 1 FROM hp_processing.history_content c
     WHERE (c.workspace_id,c.entry_id)=(h.workspace_id,h.entry_id)
       AND c.detail_ref=h.proof_ref AND c.expires_at<=$5)
)`

func deletionTargets(ctx context.Context, tx pgx.Tx, scope deletionregistry.Scope) ([]contentfiles.DeletionTarget, error) {
	if !readcases.UUID(scope.CaseID) || len(scope.Targets) == 0 || len(scope.Targets) > 128 {
		return nil, ErrDeletion
	}
	origin := preparecase.Origin{WorkspaceID: scope.WorkspaceID, CaseID: scope.CaseID}
	var cutoff time.Time
	err := tx.QueryRow(ctx, `SELECT k.revision,transaction_timestamp() FROM hp_processing.case_identity k
 JOIN hp_processing.accepted_configs c USING(workspace_id,operation_id,config_id)
 WHERE k.workspace_id=$1 AND k.case_id=$2 AND NOT k.livemode AND c.canonical_config->>'mode'='TEST'`,
		scope.WorkspaceID, scope.CaseID).Scan(&origin.Revision, &cutoff)
	if err != nil {
		return nil, ErrDeletion
	}
	seen := make(map[string]bool, len(scope.Targets))
	targets := make([]contentfiles.DeletionTarget, 0, len(scope.Targets))
	for _, target := range scope.Targets {
		if !readcases.UUID(target.VersionID) || seen[target.VersionID] {
			return nil, ErrDeletion
		}
		seen[target.VersionID] = true
		var query string
		switch target.Class {
		case "CASE_CONTEXT":
			query = contextVersion
		case "HISTORY":
			query = historyVersion
		default:
			return nil, ErrDeletion
		}
		rows, err := tx.Query(ctx, query, scope.WorkspaceID, scope.CaseID, target.VersionID)
		if err != nil {
			return nil, ErrDeletion
		}
		t := contentfiles.DeletionTarget{Origin: origin, Ref: preparecase.Ref{Class: target.Class, ID: target.VersionID}}
		entry, err := pgx.CollectExactlyOneRow(rows, func(row pgx.CollectableRow) (string, error) {
			var id string
			err := row.Scan(&id, &t.Ref.Digest, &t.Ref.Bytes, &t.Ref.ExpiresAt)
			return id, err
		})
		if err != nil {
			return nil, ErrDeletion
		}
		t.Ref.ExpiresAt = t.Ref.ExpiresAt.UTC()
		err = tx.QueryRow(ctx, hasOtherLiveReference, scope.WorkspaceID, t.Ref.ID, t.Ref.Class, entry, cutoff).Scan(&t.Shared)
		if err != nil {
			return nil, ErrDeletion
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// WithExactTargets is the issuer's read-only ownership proof. It holds the
// tenant SQL barriers and the volume lease through the issuer's decision.
func (s PurgeStore) WithExactTargets(ctx context.Context, scope deletionregistry.Scope, confirm func(context.Context) error) error {
	if confirm == nil || s.Files == nil {
		return ErrDeletion
	}
	tx, err := s.deletionScope(ctx, scope.WorkspaceID)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	targets, err := deletionTargets(ctx, tx, scope)
	if err != nil {
		return err
	}
	for _, t := range targets {
		if err := s.Files.PreflightDeletion(t, true); err != nil {
			return err
		}
	}
	return confirm(ctx)
}

// ApplyDeletion consumes only a decision already verified by the registry
// client. The publisher persists APPLYING before making that decision visible.
func (s PurgeStore) ApplyDeletion(ctx context.Context, d deletionregistry.Decision) (deletionregistry.Receipt, error) {
	zero := deletionregistry.Receipt{}
	if s.Files == nil || d.Status != "APPLYING" || d.ApplyingAt == nil || d.ApplyGeneration == 0 || d.ApplyRevision == 0 {
		return zero, ErrDeletion
	}
	tx, err := s.deletionScope(ctx, d.Scope.WorkspaceID)
	if err != nil {
		return zero, err
	}
	defer tx.Rollback(ctx)
	var a deletionApplication
	found, err := s.Files.ReadApplication(d.DeletionID, d.ApplyGeneration, &a)
	if err != nil {
		return zero, err
	}
	if found {
		if !a.matches(d) {
			return zero, ErrDeletion
		}
		return validateApplication(ctx, tx, s.Files, a)
	}
	targets, err := deletionTargets(ctx, tx, d.Scope)
	if err != nil {
		return zero, err
	}
	for _, target := range targets {
		if err := s.Files.PreflightDeletion(target, false); err != nil {
			return zero, err
		}
	}
	for _, target := range targets {
		if err := s.Files.DeleteVersion(ctx, target); err != nil {
			return zero, err
		}
	}
	var now time.Time
	if err := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&now); err != nil || now.Before(*d.ApplyingAt) {
		return zero, ErrDeletion
	}
	a = deletionApplication{DecisionID: d.DeletionID, Scope: d.Scope, ScopeDigest: d.ScopeDigest,
		ConfirmationRef: d.ConfirmationRef, Generation: d.ApplyGeneration, Revision: d.ApplyRevision,
		AppliedAt: now.UTC(), Targets: targets}
	receipt, err := validateApplication(ctx, tx, s.Files, a)
	if err != nil {
		return zero, err
	}
	if err := s.Files.WriteApplication(d.DeletionID, d.ApplyGeneration, a); err != nil {
		return zero, err
	}
	return receipt, nil
}

func (a deletionApplication) matches(d deletionregistry.Decision) bool {
	return a.DecisionID == d.DeletionID && a.Generation == d.ApplyGeneration && a.Revision == d.ApplyRevision &&
		a.ScopeDigest == d.ScopeDigest && a.ConfirmationRef == d.ConfirmationRef &&
		a.Scope.WorkspaceID == d.Scope.WorkspaceID && a.Scope.CaseID == d.Scope.CaseID &&
		slices.Equal(a.Scope.Targets, d.Scope.Targets) && d.ApplyingAt != nil && !a.AppliedAt.Before(*d.ApplyingAt)
}

type ApplicationProofFiles interface {
	ReadApplication(string, uint64, any) (bool, error)
	VerifyDeletion(contentfiles.DeletionTarget) error
}

func validateApplication(ctx context.Context, tx pgx.Tx, files ApplicationProofFiles, a deletionApplication) (deletionregistry.Receipt, error) {
	zero := deletionregistry.Receipt{}
	current, err := deletionTargets(ctx, tx, a.Scope)
	if err != nil || a.AppliedAt.IsZero() || len(current) != len(a.Targets) {
		return zero, ErrDeletion
	}
	for i, target := range current {
		previous := a.Targets[i]
		if previous.Origin.WorkspaceID != target.Origin.WorkspaceID || previous.Origin.CaseID != target.Origin.CaseID ||
			previous.Origin.Revision > target.Origin.Revision || previous.Shared != target.Shared ||
			previous.Ref.ID != target.Ref.ID || previous.Ref.Class != target.Ref.Class ||
			previous.Ref.Digest != target.Ref.Digest || previous.Ref.Bytes != target.Ref.Bytes ||
			!previous.Ref.ExpiresAt.Equal(target.Ref.ExpiresAt) {
			return zero, ErrDeletion
		}
		if err := files.VerifyDeletion(target); err != nil {
			return zero, err
		}
	}
	return a.receipt()
}

// WithApplied verifies local proof under a content lease and one tenant
// SQL barrier while the caller records APPLIED or opens a restored runtime.
func (s PurgeStore) WithApplied(ctx context.Context, files ApplicationProofFiles, workspace string,
	receipts []deletionregistry.Receipt, confirm func(context.Context) error) error {
	if files == nil || confirm == nil {
		return ErrDeletion
	}
	tx, err := s.deletionScope(ctx, workspace)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, want := range receipts {
		if want.Environment != "test" || want.Generation == 0 || want.Revision == 0 || want.WorkspaceID != workspace {
			return ErrDeletion
		}
		var a deletionApplication
		found, err := files.ReadApplication(want.DeletionID, want.Generation, &a)
		if err != nil || !found || a.Scope.WorkspaceID != workspace ||
			a.DecisionID != want.DeletionID || a.Generation != want.Generation {
			return ErrDeletion
		}
		actual, err := validateApplication(ctx, tx, files, a)
		if err != nil || !actual.AppliedAt.Equal(want.AppliedAt) {
			return ErrDeletion
		}
		want.AppliedAt = actual.AppliedAt
		if actual != want {
			return ErrDeletion
		}
	}
	return confirm(ctx)
}
