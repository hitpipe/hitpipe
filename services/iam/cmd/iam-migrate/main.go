// iam-migrate installs IAM schema v1; it never creates roles or starts serving.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/migrations"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func execute() int {
	dsn := os.Getenv("HITPIPE_IAM_MIGRATOR_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IAM_MIGRATION_DSN_REQUIRED")
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "IAM_MIGRATION_CONFIG_INVALID")
		return 2
	}
	cfg.ConnectTimeout = 5 * time.Second
	cfg.RuntimeParams["application_name"] = "hitpipe-iam-migrate"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, connectionCode(err))
		return 3
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		_ = conn.Close(cleanup)
	}()
	result, err := migrations.Apply(ctx, conn)
	if err != nil {
		// Apply errors contain fixed codes, never SQL text, DSNs or data.
		fmt.Fprintln(os.Stderr, err.Error())
		return 3
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		return 3
	}
	return 0
}

func connectionCode(err error) string {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		// SQLSTATE is a fixed five-character public code, not a server message.
		valid := len(pg.Code) == 5
		for _, c := range pg.Code {
			valid = valid && (c >= '0' && c <= '9' || c >= 'A' && c <= 'Z')
		}
		if valid {
			return "IAM_MIGRATION_CONNECT_FAILED SQLSTATE=" + pg.Code
		}
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "IAM_MIGRATION_CONNECT_TIMEOUT"
	}
	return "IAM_MIGRATION_CONNECT_FAILED"
}

func main() { os.Exit(execute()) }
