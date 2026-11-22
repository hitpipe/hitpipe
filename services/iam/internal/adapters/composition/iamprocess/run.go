// Package iamprocess assembles the IAM listeners and the one local runtime gate.
package iamprocess

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/clock/system"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
	wire "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/jackc/pgx/v5/pgxpool"
)

const shutdownBudget = 5 * time.Second

// Authorities are explicitly wired, independent inputs. A valid certificate,
// an open local gate and the presence of configuration are not such proofs.
type Authorities struct {
	Clock    authenticatesession.Clock
	Issuance issuesession.IssuanceGuard
	Resource authorizeaction.ResourceAuthority
}

var (
	errComposition = errors.New("IAM composition rejected")
	errBind        = errors.New("IAM listener could not bind")
	errServing     = errors.New("IAM listener stopped unexpectedly")
	errShutdown    = errors.New("IAM listener could not drain")
)

// Run starts the configured IAM process. A caller with an independent authority
// can use RunWithAuthorities; absent authorities deliberately remain closed.
func Run(ctx context.Context, config Config, pool *pgxpool.Pool) error {
	return RunWithAuthorities(ctx, config, pool, Authorities{})
}

// RunWithAuthorities borrows pool until both listeners and the registration
// worker have stopped. The caller owns the pool and closes it afterwards.
func RunWithAuthorities(ctx context.Context, config Config, pool *pgxpool.Pool, authorities Authorities) error {
	if ctx == nil || pool == nil || !privateAddress(config.BusinessAddress, "8443") ||
		!privateAddress(config.OPSAddress, "9444") || !buildSHA.MatchString(config.BuildRevision) ||
		!listenerTLS(config.BusinessTLS) || !listenerTLS(config.OPSTLS) {
		return errComposition
	}
	if authorities.Clock == nil {
		authorities.Clock = system.New(nil)
	}
	if authorities.Resource == nil {
		authorities.Resource = closedResource{}
	}
	iam, err := privateiam.New(pool, authorities.Clock, authorities.Resource, config.Local, config.RuntimeBindings)
	if err != nil {
		return errComposition
	}
	profile, closeProfile, err := iam.Profile(config.ProfileRoot)
	if err != nil {
		return errComposition
	}
	defer closeProfile.Close()
	auth, err := iam.Login(config.OIDC, authorities.Issuance, config.SessionPolicy, config.SuccessPath)
	if err != nil {
		return errComposition
	}
	control, err := wire.New(iam)
	if err != nil {
		return errComposition
	}
	return serve(ctx, iam, config, businessHandler(config, auth, profile, control), opsHandler(config, iam))
}

func listenerTLS(config *tls.Config) bool {
	return config != nil && config.MinVersion == tls.VersionTLS13 && config.MaxVersion == tls.VersionTLS13 &&
		config.ClientAuth == tls.RequireAndVerifyClientCert && config.ClientCAs != nil &&
		config.SessionTicketsDisabled && config.VerifyConnection != nil && config.GetConfigForClient == nil &&
		len(config.Certificates) > 0
}

type closedResource struct{}

func (closedResource) Prove(context.Context, authorizeaction.Actor, authorizeaction.Request) (authorizeaction.Proof, error) {
	return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
}

func serve(ctx context.Context, iam *privateiam.Server, config Config, business, ops http.Handler) error {
	businessListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.BusinessAddress)
	if err != nil {
		return errBind
	}
	defer businessListener.Close()
	opsListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.OPSAddress)
	if err != nil {
		return errBind
	}
	defer opsListener.Close()

	work, cancel := context.WithCancel(ctx)
	defer cancel()
	businessServer := httpServer(work, business, config.BusinessTLS)
	opsServer := httpServer(work, ops, config.OPSTLS)
	exits := make(chan error, 2)
	go func() { exits <- businessServer.ServeTLS(businessListener, "", "") }()
	go func() { exits <- opsServer.ServeTLS(opsListener, "", "") }()
	started := make(chan struct{})
	go func() {
		defer close(started)
		registerLocal(work, iam)
	}()

	var result error
	remaining := 2
	select {
	case <-ctx.Done():
	case <-exits:
		remaining--
		result = errServing
	}
	cancel()
	drain, stop := context.WithTimeout(context.Background(), shutdownBudget)
	defer stop()
	shutdown := make(chan error, 2)
	for _, server := range []*http.Server{businessServer, opsServer} {
		go func() {
			if err := server.Shutdown(drain); err != nil {
				_ = server.Close()
				shutdown <- errShutdown
				return
			}
			shutdown <- nil
		}()
	}
	for range 2 {
		result = errors.Join(result, <-shutdown)
	}
	for range remaining {
		<-exits
	}
	<-started
	return result
}

func registerLocal(ctx context.Context, iam *privateiam.Server) {
	retry := time.NewTicker(time.Second)
	defer retry.Stop()
	for ctx.Err() == nil {
		if iam.Start(ctx) == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-retry.C:
		}
	}
}

func httpServer(ctx context.Context, handler http.Handler, config *tls.Config) *http.Server {
	return &http.Server{
		Handler: handler, TLSConfig: config,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 15 * time.Second,
		MaxHeaderBytes: 8 * 1024,
		ErrorLog:       log.New(io.Discard, "", 0),
	}
}
