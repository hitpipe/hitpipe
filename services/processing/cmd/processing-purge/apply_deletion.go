package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/jackc/pgx/v5"
)

func applyDeletionInventory(ctx context.Context, inventory, volume string) error {
	if !filepath.IsAbs(volume) {
		return errOptions
	}
	client, err := inventoryClient(inventory)
	if err != nil {
		return err
	}
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		return err
	}
	cfg, err := localConfig(strings.TrimSpace(os.Getenv("HITPIPE_PURGE_DSN")))
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	handled := 0
	for _, d := range snapshot.Decisions {
		switch d.Status {
		case "APPLYING":
			receipt, err := applyDecision(ctx, conn, volume, d)
			if err != nil {
				return err
			}
			// WithApplied on the publisher side independently reacquires the
			// local lease. No database or volume lock crosses this POST.
			if err := client.Submit(ctx, receipt); err != nil {
				return err
			}
		case "APPLIED":
			if err := verifyDecision(ctx, conn, volume, d); err != nil {
				return err
			}
		default:
			continue
		}
		handled++
	}
	if handled == 0 {
		return postgres.ErrDeletion
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]int{"verified_decisions": handled})
}

func applyDecision(ctx context.Context, conn *pgx.Conn, volume string, d deletionregistry.Decision) (deletionregistry.Receipt, error) {
	files, err := contentfiles.OpenMaintenance(volume)
	if err != nil {
		return deletionregistry.Receipt{}, err
	}
	defer files.Close()
	return (postgres.PurgeStore{DB: conn, Files: files}).ApplyDeletion(ctx, d)
}

func verifyDecision(ctx context.Context, conn *pgx.Conn, volume string, d deletionregistry.Decision) error {
	files, err := contentfiles.New(volume)
	if err != nil {
		return err
	}
	defer files.Close()
	return (postgres.PurgeStore{DB: conn}).WithApplied(ctx, files, d.Scope.WorkspaceID, d.Receipts, func(context.Context) error { return nil })
}

func runDeletion(ctx context.Context, inventory, volume string) int {
	if err := applyDeletionInventory(ctx, inventory, volume); err != nil {
		fmt.Fprintln(os.Stderr, "confirmed case deletion incomplete or refused")
		return 1
	}
	return 0
}
