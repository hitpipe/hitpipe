package iamprocess

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errDatabaseConfig = errors.New("IAM_DATABASE_CONFIG_INVALID")
var errDatabaseAuthority = errors.New("IAM_DATABASE_AUTHORITY_UNAVAILABLE")

type databaseInput struct {
	DSNFile        string `json:"dsn_file"`
	MaxConnections *int32 `json:"max_connections"`
}

func (raw databaseInput) load() (*pgxpool.Config, error) {
	max := int32(5)
	if raw.MaxConnections != nil {
		max = *raw.MaxConnections
	}
	// pgx reads the selected service file before applying overrides from an
	// explicit connection URI. The service file is not a source for IAM.
	if max < 1 || max > 10 || os.Getenv("PGSERVICE") != "" {
		return nil, errDatabaseConfig
	}
	data, err := readFile(raw.DSNFile, maxSecretFile, true)
	if err != nil {
		return nil, errDatabaseConfig
	}
	dsn, err := url.Parse(strings.TrimSpace(string(data)))
	if err != nil || (dsn.Scheme != "postgres" && dsn.Scheme != "postgresql") ||
		dsn.User == nil || dsn.User.Username() == "" || dsn.Hostname() == "" ||
		dsn.Path == "" || dsn.Path == "/" || strings.ContainsAny(dsn.Host, ",/") ||
		strings.Contains(dsn.Path[1:], "/") || dsn.Fragment != "" {
		return nil, errDatabaseConfig
	}
	q, err := url.ParseQuery(dsn.RawQuery)
	if err != nil {
		return nil, errDatabaseConfig
	}
	for key, value := range q {
		if len(value) != 1 || (key != "sslmode" && key != "sslrootcert") {
			return nil, errDatabaseConfig
		}
	}

	// Production uses verified PostgreSQL TLS. Plain TCP is reserved for an
	// explicitly selected loopback connection, including the isolated fixture.
	mode := q.Get("sslmode")
	if mode == "" {
		mode = "verify-full"
	}
	if mode == "disable" {
		ip := net.ParseIP(dsn.Hostname())
		if ip == nil || !ip.IsLoopback() || q.Get("sslrootcert") != "" {
			return nil, errDatabaseConfig
		}
	} else if mode != "verify-full" {
		return nil, errDatabaseConfig
	}
	rootFile := q.Get("sslrootcert")
	port := dsn.Port()
	if port == "" {
		port = "5432"
	}
	dsn.Host = net.JoinHostPort(dsn.Hostname(), port)
	user, password := dsn.User.Username(), ""
	if value, supplied := dsn.User.Password(); supplied {
		password = value
	}
	dsn.User = url.UserPassword(user, password)

	// Avoid implicit credential files and TLS settings from the developer
	// environment. Only the settings in the provisioning file authorize peers.
	dsn.RawQuery = url.Values{
		"sslmode": {mode}, "sslkey": {""}, "sslcert": {""}, "sslrootcert": {""},
		"passfile": {""}, "connect_timeout": {"2"}, "sslnegotiation": {"postgres"},
		"target_session_attrs": {"any"}, "sslpassword": {""}, "sslsni": {"1"},
		"min_protocol_version": {"3.0"}, "max_protocol_version": {"3.0"},
		"channel_binding": {"prefer"}, "require_auth": {""},
	}.Encode()
	cfg, err := pgxpool.ParseConfig(dsn.String())
	if err != nil || len(cfg.ConnConfig.Fallbacks) != 0 {
		return nil, errDatabaseConfig
	}
	cfg.ConnConfig.Password = password
	cfg.ConnConfig.RuntimeParams = map[string]string{"application_name": "hitpipe-iam"}
	cfg.ConnConfig.Tracer = nil
	if mode == "verify-full" {
		tls := cfg.ConnConfig.TLSConfig
		if tls == nil || tls.InsecureSkipVerify || tls.ServerName != dsn.Hostname() {
			return nil, errDatabaseConfig
		}
		if rootFile != "" && rootFile != "system" {
			roots, err := loadCA(rootFile)
			if err != nil {
				return nil, errDatabaseConfig
			}
			tls.RootCAs = roots
		}
	}
	cfg.MaxConns, cfg.MinConns, cfg.MinIdleConns = max, 0, 0
	cfg.AfterConnect = checkPostgresAuthority
	return cfg, nil
}

// OpenPool creates the sole IAM application pool. Establishing a database
// connection remains lazy; unavailable PostgreSQL leaves authorization closed.
func (cfg Config) OpenPool(ctx context.Context) (*pgxpool.Pool, error) {
	if cfg.pool == nil || ctx == nil || ctx.Err() != nil {
		return nil, errDatabaseConfig
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg.pool.Copy())
	if err != nil {
		return nil, errDatabaseConfig
	}
	return pool, nil
}

func checkPostgresAuthority(ctx context.Context, conn *pgx.Conn) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	tx, err := conn.BeginTx(bounded, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return errDatabaseAuthority
	}
	defer func() { _ = tx.Rollback(bounded) }()
	var appRole bool
	const installed = authorization.ApplicationAuthoritySQL + " AND current_setting('server_version_num')::integer=180007"
	if err := tx.QueryRow(bounded, installed).Scan(&appRole); err != nil || !appRole {
		return errDatabaseAuthority
	}
	if err := tx.Commit(bounded); err != nil {
		return errDatabaseAuthority
	}
	return nil
}
