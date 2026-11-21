//go:build hit106lab

// The explicit lab build serves only generated credentials on guest loopback.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/clock/system"
	httpauth "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/oidc"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/migrations"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	databaseAddress = "@127.0.0.1:15444/hit106_iam?sslmode=disable"
	idpIssuer       = "https://127.0.0.1:18443/realms/hit106-test"
	iamAddress      = "127.0.0.2:18444"
	completionPath  = "/__hit106/complete"
)

func fixtureSecret(name string) (string, error) {
	value := os.Getenv(name)
	if len(value) != 64 || value != strings.ToLower(value) {
		return "", fmt.Errorf("%s must be a generated hex fixture", name)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", fmt.Errorf("%s must be a generated hex fixture", name)
	}
	return value, nil
}

func migration(ctx context.Context) error {
	adminPassword, err := fixtureSecret("HIT106_IAM_ADMIN_PASSWORD")
	if err != nil {
		return err
	}
	migratorPassword, err := fixtureSecret("HIT106_IAM_MIGRATOR_PASSWORD")
	if err != nil {
		return err
	}
	appPassword, err := fixtureSecret("HIT106_IAM_APP_PASSWORD")
	if err != nil {
		return err
	}
	adminConfig, err := pgx.ParseConfig("postgres://hit106_iam_admin" + databaseAddress)
	if err != nil {
		return errors.New("invalid lab administrator configuration")
	}
	adminConfig.Password = adminPassword
	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		return errors.New("lab administrator connection failed")
	}
	defer admin.Close(ctx)
	var fresh bool
	if err := admin.QueryRow(ctx, `SELECT current_database()='hit106_iam'
		AND current_setting('server_version_num')='180007'
		AND NOT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname='hp_iam')`).Scan(&fresh); err != nil || !fresh {
		return errors.New("migration requires the fresh local PostgreSQL 18.7 lab")
	}
	bootstrap, err := os.ReadFile("internal/adapters/postgres/migrations/bootstrap_roles.sql")
	if err != nil {
		return errors.New("read the existing IAM role bootstrap from the IAM work directory")
	}
	if _, err := admin.PgConn().Exec(ctx, string(bootstrap)).ReadAll(); err != nil {
		return errors.New("IAM role bootstrap failed")
	}
	// Values have already been restricted to 64 lowercase hexadecimal digits.
	// The two login roles exist only in this disposable database.
	roles := "CREATE ROLE hit106_iam_migration LOGIN IN ROLE hp_iam_migrator PASSWORD '" + migratorPassword + "'; " +
		"CREATE ROLE hit106_iam_app LOGIN IN ROLE hp_iam_app PASSWORD '" + appPassword + "';"
	if _, err := admin.PgConn().Exec(ctx, roles).ReadAll(); err != nil {
		return errors.New("generate database fixture logins failed")
	}
	if err := admin.Close(ctx); err != nil {
		return errors.New("close lab administrator connection failed")
	}
	config, err := pgx.ParseConfig("postgres://hit106_iam_migration" + databaseAddress)
	if err != nil {
		return errors.New("invalid lab migrator configuration")
	}
	config.Password = migratorPassword
	config.RuntimeParams["role"] = "hp_iam_migrator"
	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return errors.New("lab migrator connection failed")
	}
	defer conn.Close(ctx)
	for _, expected := range []string{"installed", "already_installed"} {
		result, err := migrations.Apply(ctx, conn)
		if err != nil || result.State != expected || result.Version != 1 || result.SQLSHA256 != migrations.SQLSHA256 {
			return errors.New("existing IAM migration did not produce the expected receipt")
		}
		fmt.Printf("IAM PostgreSQL 18.7 migration: %s, version %d, SHA-256 %s\n", result.State, result.Version, result.SQLSHA256)
	}
	return nil
}

// This guard represents explicit positive authority in the isolated fixture.
// The runtime composition never constructs it. The real PostgreSQL issuer still
// checks the database role, disabled identities and pending session revocations.
type labGuard struct{}

func (labGuard) Check(ctx context.Context, identity issuesession.Identity) (issuesession.GuardDecision, error) {
	if err := ctx.Err(); err != nil {
		return issuesession.GuardDecision{}, err
	}
	if identity.Issuer != idpIssuer || identity.Subject == "" {
		return issuesession.GuardDecision{}, nil
	}
	return issuesession.GuardDecision{State: issuesession.IssuanceAllowed, Epoch: 1}, nil
}

func openLabAppPool(ctx context.Context) (*pgxpool.Pool, error) {
	appPassword, err := fixtureSecret("HIT106_IAM_APP_PASSWORD")
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig("postgres://hit106_iam_app" + databaseAddress)
	if err != nil {
		return nil, errors.New("invalid lab application configuration")
	}
	config.ConnConfig.Password = appPassword
	config.MaxConns = 5
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, errors.New("create local lab pool failed")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, errors.New("connect to local IAM PostgreSQL failed")
	}
	return pool, nil
}

func serve(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("HIT106_LAB_DIR must locate a generated fixture")
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.pem"))
	if err != nil {
		return errors.New("read the generated lab CA failed")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid lab CA")
	}
	clientSecret, err := fixtureSecret("HIT106_CLIENT_SECRET")
	if err != nil {
		return err
	}
	db, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pool, err := openLabAppPool(db)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()
	oidcConfig := oidc.Config{
		Issuer: idpIssuer, AuthorizationEndpoint: idpIssuer + "/protocol/openid-connect/auth",
		TokenEndpoint: idpIssuer + "/protocol/openid-connect/token", JWKSURI: idpIssuer + "/protocol/openid-connect/certs",
		ClientID: "hit106-iam", ClientSecret: clientSecret, RootCAs: roots,
		RedirectURI: "https://" + iamAddress + "/auth/callback",
	}
	clock := system.New(func(ctx context.Context, now time.Time) bool {
		return ctx.Err() == nil && !now.IsZero()
	})
	login, err := httpauth.NewOIDC(oidcConfig, pool, clock, labGuard{}, issuesession.Policy{
		AbsoluteLifetime: labAbsoluteLifetime, IdleLifetime: labIdleLifetime,
	}, completionPath)
	if err != nil {
		return fmt.Errorf("configure the lab login: %w", err)
	}
	lookup, err := sessionlookup.New(pool)
	if err != nil {
		return err
	}
	authenticate, err := authenticatesession.New(lookup, clock)
	if err != nil {
		return err
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/__hit106/health":
				w.WriteHeader(http.StatusNoContent)
				return
			case completionPath:
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, "<!doctype html><html><title>HIT-106</title><p>Sign-in completed.</p></html>")
				return
			case "/__hit106/session":
				cookie, err := r.Cookie("__Host-hp_session")
				if err == nil {
					ref, err := sessionlookup.Reference(cookie.Value)
					if err == nil && authenticate.Execute(r.Context(), ref).IsValid() {
						w.WriteHeader(http.StatusNoContent)
						return
					}
				}
				w.WriteHeader(http.StatusUnauthorized)
				return
			case "/__hit106/identity":
				serveIdentityProof(w, r, pool, authenticate)
				return
			case "/__hit106/count":
				ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
				defer cancel()
				var result struct {
					Principals int `json:"principals"`
					Sessions   int `json:"sessions"`
				}
				err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM hp_iam.principals), (SELECT count(*) FROM hp_iam.sessions)").Scan(&result.Principals, &result.Sessions)
				if err != nil {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(result)
				return
			}
		}
		login.ServeHTTP(w, r)
	})
	srv := &http.Server{
		Addr: iamAddress, Handler: handler, ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout: 5 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second,
		MaxHeaderBytes: 32 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		ErrorLog: log.New(io.Discard, "", 0),
	}
	if err := srv.ListenAndServeTLS(filepath.Join(dir, "server.pem"), filepath.Join(dir, "server.key")); !errors.Is(err, http.ErrServerClosed) {
		return errors.New("serve the local HTTPS fixture failed")
	}
	return nil
}

func run() error {
	const usage = "usage: iam-browser migrate|serve|check-policy"
	if len(os.Args) != 2 {
		return errors.New(usage)
	}
	switch os.Args[1] {
	case "migrate":
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return migration(ctx)
	case "serve":
		return serve(os.Getenv("HIT106_LAB_DIR"))
	case "check-policy":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return checkPolicy(ctx)
	default:
		return errors.New(usage)
	}
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
