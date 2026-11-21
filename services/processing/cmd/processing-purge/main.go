// Command processing-purge performs one bounded maintenance batch for a local
// Processing qualification stand.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

var errOptions = errors.New("use --local-stand, an absolute --content-dir, --workspace, and a local HITPIPE_PURGE_DSN")

func localConfig(dsn string) (*pgx.ConnConfig, error) {
	if dsn == "" {
		return nil, errOptions
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, errOptions
	}
	host := net.ParseIP(cfg.Host)
	if (!filepath.IsAbs(cfg.Host) && (host == nil || !host.IsLoopback())) ||
		len(cfg.Fallbacks) != 0 || cfg.Database == "" || cfg.User == "" {
		return nil, errOptions
	}
	cfg.ConnectTimeout = 3 * time.Second
	cfg.RuntimeParams = map[string]string{
		"application_name":                    "hitpipe-processing-purge",
		"search_path":                         "pg_catalog",
		"statement_timeout":                   "5000",
		"lock_timeout":                        "1000",
		"idle_in_transaction_session_timeout": "10000",
	}
	return cfg, nil
}

func run(ctx context.Context, workspace, path string, limit int, local bool) int {
	if !local || !filepath.IsAbs(path) || !readcases.UUID(workspace) || limit < 1 || limit > contentfiles.MaxPurgeBatch || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, errOptions)
		return 2
	}
	cfg, err := localConfig(strings.TrimSpace(os.Getenv("HITPIPE_PURGE_DSN")))
	if err != nil {
		fmt.Fprintln(os.Stderr, errOptions)
		return 2
	}
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	files, err := contentfiles.OpenMaintenance(path)
	if err != nil {
		log.Warn("private Processing content directory unavailable or busy", "reason", "purge_volume_unavailable", "workspace_id", workspace)
		return 1
	}
	defer files.Close()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		log.Warn("local Processing database unavailable", "reason", "purge_postgres_unavailable", "workspace_id", workspace)
		return 1
	}
	defer conn.Close(context.Background())
	result, err := (postgres.PurgeStore{DB: conn, Files: files}).Run(ctx, workspace, limit)
	if err != nil {
		if errors.Is(err, postgres.ErrPurgeScope) {
			log.Warn(postgres.ErrPurgeScope.Error(), "reason", "purge_scope_denied", "workspace_id", workspace)
		} else {
			log.Warn("processing purge incomplete; review local state before repeating", "reason", "purge_incomplete", "workspace_id", workspace)
		}
		return 1
	}
	if err = json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 1
	}
	return 0
}

func main() {
	workspace := flag.String("workspace", "", "explicit authorized local workspace UUID")
	path := flag.String("content-dir", "", "absolute path to the private local Processing content directory")
	limit := flag.Int("limit", 100, "maximum rows held and maximum files unlinked per batch")
	local := flag.Bool("local-stand", false, "confirm isolated local stand")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, *workspace, *path, *limit, *local))
}
