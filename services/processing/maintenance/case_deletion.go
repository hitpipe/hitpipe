// Package maintenance provides the local Processing side of the trusted
// maintenance composition. It defines no network routes or issuer operations.
package maintenance

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
)

type DeletionScope = deletionregistry.Scope
type DeletionTarget = deletionregistry.Target
type DeletionReceipt = deletionregistry.Receipt

var ErrScope = errors.New("processing maintenance scope refused")

// CaseAuthority is provisioned by the trusted local maintenance coordinator.
// Its DB is authenticated with the existing limited purge role.
type CaseAuthority struct {
	volume     string
	db         postgres.Beginner
	workspace  string
	generation uint64
}

func NewCaseAuthority(db postgres.Beginner, volume, workspace string, generation uint64) (CaseAuthority, error) {
	if db == nil || !filepath.IsAbs(volume) || workspace == "" || generation == 0 {
		return CaseAuthority{}, ErrScope
	}
	return CaseAuthority{volume: volume, db: db, workspace: workspace, generation: generation}, nil
}

func (a CaseAuthority) WithExactTargets(ctx context.Context, scope DeletionScope, confirm func(context.Context) error) error {
	if scope.WorkspaceID != a.workspace {
		return ErrScope
	}
	files, err := contentfiles.OpenMaintenance(a.volume)
	if err != nil {
		return err
	}
	defer files.Close()
	return (postgres.PurgeStore{DB: a.db, Files: files}).WithExactTargets(ctx, scope, confirm)
}

func (a CaseAuthority) WithApplied(ctx context.Context, receipt DeletionReceipt, confirm func(context.Context) error) error {
	return a.withProofs(ctx, []DeletionReceipt{receipt}, confirm)
}

func (a CaseAuthority) withProofs(ctx context.Context, receipts []DeletionReceipt, confirm func(context.Context) error) error {
	for _, receipt := range receipts {
		if receipt.WorkspaceID != a.workspace || receipt.Generation != a.generation {
			return ErrScope
		}
	}
	files, err := contentfiles.New(a.volume)
	if err != nil {
		return err
	}
	defer files.Close()
	return (postgres.PurgeStore{DB: a.db}).WithApplied(ctx, files, a.workspace, receipts, confirm)
}
