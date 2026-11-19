package intakeruntime

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	factshttp "github.com/hitpipe/hitpipe/services/intake/internal/adapters/http/sourcefacts"
	web "github.com/hitpipe/hitpipe/services/intake/internal/adapters/http/stripewebhook"
	factsdb "github.com/hitpipe/hitpipe/services/intake/internal/adapters/postgres/facts"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/postgres/outbox"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/postgres/receipt"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/processing"
	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/stripe"
	facts "github.com/hitpipe/hitpipe/services/intake/internal/application/readsourcefacts"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
	out "github.com/hitpipe/hitpipe/services/intake/internal/application/sendoutbox"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errRuntime = errors.New("Intake sandbox could not run")

const factsPath = "/internal/v1/source-facts/read"

func pool(ctx context.Context, port int, role string, max int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(fmt.Sprintf("postgres://%s@127.0.0.1:%d/intake_test?sslmode=disable", role, port))
	if err != nil {
		return nil, errRuntime
	}
	cfg.ConnConfig.Fallbacks = nil
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "hp_intake,pg_catalog", "application_name": "hitpipe-intake-sandbox"}
	cfg.ConnConfig.ConnectTimeout = 2 * time.Second
	cfg.MaxConns = max
	cfg.MinConns = 0
	cfg.MinIdleConns = 0
	cfg.BeforeConnect = nil
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errRuntime
	}
	var ok bool
	err = p.QueryRow(ctx, `SELECT current_user=$1 AND current_database()='intake_test'
		AND current_setting('server_version_num')='180007'
		AND NOT rolsuper AND NOT rolbypassrls AND NOT pg_has_role(current_user,'hp_intake_migrator','member')
		FROM pg_roles WHERE rolname=current_user`, role).Scan(&ok)
	if err != nil || !ok {
		p.Close()
		return nil, errRuntime
	}
	return p, nil
}

func ca(dir, name string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || len(raw) > 65536 {
		return nil, ErrConfig
	}
	p := x509.NewCertPool()
	if !p.AppendCertsFromPEM(raw) {
		return nil, ErrConfig
	}
	return p, nil
}
func certificate(dir, stem string) (tls.Certificate, error) {
	key := filepath.Join(dir, stem+".key")
	info, err := os.Lstat(key)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return tls.Certificate{}, ErrConfig
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, stem+".pem"), key)
	if err != nil {
		return tls.Certificate{}, ErrConfig
	}
	return cert, nil
}

// Run only admits the explicitly isolated profile. Ordinary startup cannot
// install the sandbox's synthetic grants or start a listener.
func Run(parent context.Context, c Config, logOut io.Writer) error {
	if c.Profile != "sandbox" || c.dir == "" {
		return ErrClosed
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	init, stopInit := context.WithTimeout(ctx, 5*time.Second)
	defer stopInit()

	roots, err := ca(c.dir, "service-ca.pem")
	if err != nil {
		return err
	}
	stripeRoots, err := ca(c.dir, "stripe-ca.pem")
	if err != nil {
		return err
	}
	serverCert, err := certificate(c.dir, "intake-server")
	if err != nil {
		return err
	}
	clientCert, err := certificate(c.dir, "intake-client")
	if err != nil {
		return err
	}
	signing, err := loadSecret(c.dir, "webhook.secret")
	if err != nil {
		return err
	}
	key, err := loadSecret(c.dir, "stripe.secret")
	if err != nil {
		return err
	}
	s := &sandbox{ctx: ctx, gate: make(chan struct{}, 1), signing: signing, key: key}
	appPool, err := pool(init, c.DatabasePort, "hp_intake_app", 5)
	if err != nil {
		return err
	}
	defer appPool.Close()
	workerPool, err := pool(init, c.DatabasePort, "hp_intake_worker", 1)
	if err != nil {
		return err
	}
	defer workerPool.Close()

	store, err := contentstore.Open(filepath.Join(c.dir, "content"), time.Now)
	if err != nil {
		return errRuntime
	}
	defer store.Close()
	receipts, err := receipt.New(appPool)
	if err != nil {
		return errRuntime
	}
	inbound, err := receivesignal.New(Workspace, s, clock{}, 72*time.Hour, receipts, store)
	if err != nil {
		return errRuntime
	}
	webhook, err := web.NewHandler(2*time.Second, s, webhookGate{s}, map[string]*receivesignal.Consumer{Workspace: inbound})
	if err != nil {
		return errRuntime
	}

	fr, err := factsdb.FromPool(init, appPool)
	if err != nil {
		return errRuntime
	}
	provider, err := stripe.New(c.StripeOrigin, &http.Transport{TLSClientConfig: &tls.Config{RootCAs: stripeRoots,
		MinVersion: tls.VersionTLS13}}, s, time.Now, stripe.Bounds{ResponseBytes: 65536, Pages: 4, Items: 300})
	if err != nil {
		return errRuntime
	}
	factService, err := facts.New(s, fr, content{store}, provider, time.Now, 90*24*time.Hour)
	if err != nil {
		return errRuntime
	}
	factHandler, err := factshttp.NewHandler(ProcessingName, factService, 8192)
	if err != nil {
		return errRuntime
	}

	repo, err := outbox.New(workerPool)
	if err != nil {
		return errRuntime
	}
	client, err := processing.New(c.ProcessingAddress, ProcessingName, roots, clientCert)
	if err != nil {
		return errRuntime
	}
	worker, err := out.New(Workspace, repo, senderGate{s}, store, client, out.Policy{Batch: c.Batch, Attempts: 5,
		Lease: 5 * time.Second, Timeout: 2 * time.Second, BaseBackoff: time.Second, MaxBackoff: 4 * time.Second})
	if err != nil {
		return errRuntime
	}
	listener, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return errRuntime
	}
	defer listener.Close()

	server := business(ctx, serverCert, roots, webhook, factHandler)
	done := make(chan error, 1)
	workerDone := make(chan struct{})
	go func() { done <- server.ServeTLS(listener, "", "") }()
	go func() {
		defer close(workerDone)
		poll(ctx, worker, time.Duration(c.PollMillis)*time.Millisecond)
	}()
	fmt.Fprintln(logOut, "intake sandbox listening")
	var serveErr error
	served := false
	select {
	case <-ctx.Done():
	case serveErr = <-done:
		served = true
	}
	cancel()
	drain, stop := context.WithTimeout(context.Background(), 4*time.Second)
	defer stop()
	shutdownErr := server.Shutdown(drain)
	if shutdownErr != nil {
		_ = server.Close()
	}
	if !served {
		select {
		case serveErr = <-done:
		case <-drain.Done():
			return errRuntime
		}
	}
	select {
	case <-workerDone:
	case <-drain.Done():
		return errRuntime
	}
	if shutdownErr != nil || (!errors.Is(serveErr, http.ErrServerClosed) && serveErr != nil) {
		return errRuntime
	}
	fmt.Fprintln(logOut, "intake sandbox stopped")
	return nil
}

func poll(ctx context.Context, worker *out.Worker, period time.Duration) {
	timer := time.NewTimer(period)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if ctx.Err() != nil {
				return
			}
			_, _ = worker.Run(ctx, scope) // The worker persists retry eligibility; the loop only polls.
			timer.Reset(period)
		}
	}
}

func business(ctx context.Context, cert tls.Certificate, roots *x509.CertPool, webhook, facts http.Handler) *http.Server {
	limit := make(chan struct{}, 8)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ctx.Err() != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		select {
		case limit <- struct{}{}:
			defer func() { <-limit }()
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		bounded, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		r = r.WithContext(bounded)
		switch {
		case r.URL.Path == factsPath:
			facts.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/webhooks/stripe/"):
			webhook.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	server := &http.Server{
		Handler: handler, Protocols: protocols, BaseContext: func(net.Listener) context.Context { return ctx },
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, ClientCAs: roots,
			ClientAuth: tls.VerifyClientCertIfGiven, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second, WriteTimeout: 8 * time.Second,
		MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0),
	}
	server.SetKeepAlivesEnabled(false)
	return server
}
