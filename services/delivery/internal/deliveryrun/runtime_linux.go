// Package deliveryrun composes the Delivery adapters in one Linux process.
package deliveryrun

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"sync/atomic"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/dispatch"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Scope struct {
	Workspace uuid.UUID
	Epoch     int64
}

// Adapters are provided by the runtime owner. No authority, credential or gate
// is inferred from an inbound message, a TLS identity or a database record.
type Adapters struct {
	Intake      intake.Authority
	Dispatch    dispatch.Authority
	Read        dispatch.ReadAuthority
	Gate        intake.StartGate
	Credentials dispatch.Credentials
	Scopes      func(context.Context) ([]Scope, error)
}

type Config struct {
	AppDSN, WorkerDSN, ContentPath          string
	BusinessAddr, HealthAddr, Revision      string
	ProcessingOrigin, ProcessingSAN, OpsSAN string
	Server, Client                          tls.Certificate
	ProcessingCallerCA, ProcessingPeerCA    *x509.CertPool
	OpsCA                                   *x509.CertPool
}

const (
	healthPath   = "/internal/v1/health/delivery"
	pollInterval = 250 * time.Millisecond
	runtimeLock  = int64(4831059550513427798)
)

var revisionPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type guarded struct {
	ready *atomic.Bool
	next  intake.StartGate
}

func (g guarded) WithOpen(ctx context.Context, ws uuid.UUID, epoch int64, work func() error) error {
	if !g.ready.Load() {
		return intake.ErrUnavailable
	}
	return g.next.WithOpen(ctx, ws, epoch, func() error {
		if !g.ready.Load() || ctx.Err() != nil {
			return intake.ErrUnavailable
		}
		return work()
	})
}

// Run requires explicitly provisioned adapters. It does not apply migrations
// or provision runtime permissions. clickup has a fixed provisioned origin.
func Run(ctx context.Context, cfg Config, adapters Adapters, clickup *dispatch.ClickUp) error {
	if adapters.Intake == nil || adapters.Dispatch == nil || adapters.Read == nil ||
		adapters.Gate == nil || adapters.Credentials == nil || adapters.Scopes == nil || clickup == nil {
		return errors.New("closed: obligatory runtime adapters are unavailable")
	}
	if !revisionPattern.MatchString(cfg.Revision) || cfg.BusinessAddr == "" || cfg.HealthAddr == "" ||
		cfg.OpsSAN == "" || cfg.OpsCA == nil || cfg.BusinessAddr == cfg.HealthAddr {
		return errors.New("closed: mandatory listener configuration is missing")
	}
	startCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if cfg.AppDSN == "" || cfg.WorkerDSN == "" || cfg.ContentPath == "" {
		return errors.New("closed: database or content configuration is missing")
	}
	files, err := intake.OpenDirectory(cfg.ContentPath)
	if err != nil {
		return errors.New("closed: private content directory unavailable")
	}
	defer files.Close()
	app, err := pgxpool.New(startCtx, cfg.AppDSN)
	if err != nil {
		return errors.New("closed: application database configuration invalid")
	}
	defer app.Close()
	workerPool, err := pgxpool.New(startCtx, cfg.WorkerDSN)
	if err != nil {
		return errors.New("closed: worker database configuration invalid")
	}
	defer workerPool.Close()
	if app.Config().MaxConns > 10 || workerPool.Config().MaxConns < 2 || workerPool.Config().MaxConns > 10 {
		return errors.New("closed: database pool limits invalid")
	}
	if err := databaseReady(startCtx, app, "hp_delivery_app"); err != nil {
		return err
	}
	if err := databaseReady(startCtx, workerPool, "hp_delivery_worker"); err != nil {
		return err
	}
	owner, err := workerPool.Acquire(startCtx)
	if err != nil {
		return errors.New("closed: worker lock connection unavailable")
	}
	var locked bool
	if err = owner.QueryRow(startCtx, "SELECT pg_try_advisory_lock($1)", runtimeLock).Scan(&locked); err != nil || !locked {
		owner.Release()
		return errors.New("closed: another Delivery process owns the database scheduler or lock is unavailable")
	}
	// End the owning database session on shutdown, including any failure after
	// the lock was acquired. A pooled connection must never retain the lock.
	defer func() {
		stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		_ = owner.Hijack().Close(stopCtx)
	}()

	appLedger, err := intake.NewPostgres(app, files)
	if err != nil {
		return errors.New("closed: application ledger unavailable")
	}
	workerLedger, err := dispatch.NewPostgres(workerPool)
	if err != nil {
		return errors.New("closed: worker ledger unavailable")
	}
	var ready atomic.Bool
	gate := guarded{ready: &ready, next: adapters.Gate}
	admission, err := intake.New(adapters.Intake, gate, appLedger)
	if err != nil {
		return errors.New("closed: admission adapter unavailable")
	}
	query, err := dispatch.NewQueryService(workerLedger, files, adapters.Read, gate, adapters.Credentials, clickup)
	if err != nil {
		return errors.New("closed: query adapter unavailable")
	}
	worker, err := dispatch.NewWorker(workerLedger, files, adapters.Dispatch, gate, adapters.Credentials, clickup)
	if err != nil {
		return errors.New("closed: dispatch adapter unavailable")
	}
	publisher, err := dispatch.NewPublisher(workerLedger, files, gate, cfg.ProcessingOrigin, cfg.ProcessingPeerCA, cfg.Client)
	if err != nil {
		return errors.New("closed: Processing publisher configuration unavailable")
	}
	business, err := intake.NewHTTPWithQuery(admission, query, cfg.Server, cfg.ProcessingCallerCA, cfg.ProcessingSAN)
	if err != nil {
		return errors.New("closed: Processing caller listener unavailable")
	}
	health := &http.Server{
		Handler: healthHandler(cfg, &ready), TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS13, ClientAuth: tls.RequireAndVerifyClientCert,
			ClientCAs: cfg.OpsCA.Clone(), Certificates: []tls.Certificate{cfg.Server},
		},
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second,
		WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192,
	}
	bl, err := net.Listen("tcp", cfg.BusinessAddr)
	if err != nil {
		return errors.New("closed: business listener could not bind")
	}
	defer bl.Close()
	hl, err := net.Listen("tcp", cfg.HealthAddr)
	if err != nil {
		return errors.New("closed: health listener could not bind")
	}
	defer hl.Close()
	run, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 3)
	go func() { done <- business.ServeTLS(bl, "", "") }()
	go func() { done <- health.ServeTLS(hl, "", "") }()
	s := scheduler{
		pool: workerPool, files: files, worker: worker, query: query, publisher: publisher,
		gate: adapters.Gate, scopes: adapters.Scopes, ready: &ready,
		cursors: make(map[uuid.UUID]uuid.UUID),
	}
	go func() { done <- s.run(run, owner, app) }()

	var failure error
	remaining := 3
	select {
	case <-ctx.Done():
	case err := <-done:
		failure = err
		remaining--
	}
	ready.Store(false)
	stop()
	shutdown, finish := context.WithTimeout(context.Background(), 3*time.Second)
	defer finish()
	_ = business.Shutdown(shutdown)
	_ = health.Shutdown(shutdown)
	_ = business.Close()
	_ = health.Close()
	for range remaining {
		<-done
	}
	if failure != nil && !errors.Is(failure, http.ErrServerClosed) && !errors.Is(failure, context.Canceled) {
		return failure
	}
	return nil
}

func databaseReady(ctx context.Context, pool *pgxpool.Pool, role string) error {
	var ready bool
	err := pool.QueryRow(ctx, `SELECT current_user::text=$1
		AND current_setting('server_version_num')::int=180007
		AND current_setting('fsync')='on' AND current_setting('full_page_writes')='on'
		AND current_setting('synchronous_commit')='on'
		AND EXISTS(SELECT 1 FROM hp_delivery.schema_version WHERE version=1)
		AND EXISTS(SELECT 1 FROM pg_roles WHERE rolname=current_user AND NOT rolsuper AND NOT rolbypassrls)`, role).Scan(&ready)
	if err != nil || !ready {
		return fmt.Errorf("closed: PostgreSQL 18.7, schema, durable settings or role %s unavailable", role)
	}
	return nil
}

func healthHandler(cfg Config, ready *atomic.Bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != healthPath {
			http.NotFound(w, r)
			return
		}
		ops := false
		if r.TLS != nil && len(r.TLS.VerifiedChains) != 0 {
			for _, name := range r.TLS.VerifiedChains[0][0].DNSNames {
				ops = ops || name == cfg.OpsSAN
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if !ops {
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "CALLER_DENIED", "message": "Forbidden", "request_id": uuid.NewString()})
			return
		}
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "DELIVERY_CLOSED", "message": "Delivery dependencies unavailable", "request_id": uuid.NewString()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "READY", "service": "delivery",
			"build_revision": cfg.Revision, "protocol_version": "1.0", "checked_at": time.Now().UTC().Format(time.RFC3339Nano)})
	})
}

func logState(state string) { log.Print("delivery state=", state) }
