package runtime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals"
	"github.com/jackc/pgx/v5/pgxpool"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type Runtime struct {
	a           *access
	servers     []*http.Server
	pool        []*pgxpool.Pool
	files       *contentfiles.Store
	sender      dispatchintent.Service
	lifecycle   postgres.LifecycleStore
	caseReaders sourceReaders
	caseJobs    postgres.CaseJobStore
	timeouts    postgres.CaseTimeoutStore
	logger      *slog.Logger
}

func newPool(ctx context.Context, path, dbName, role string, size int32, stand bool) (*pgxpool.Pool, error) {
	raw, err := readSecret(path, 8<<10)
	if err != nil {
		return nil, err
	}
	c, err := pgxpool.ParseConfig(strings.TrimSpace(string(raw)))
	if err != nil || c.ConnConfig.Database != dbName || c.ConnConfig.User == "" || len(c.ConnConfig.Fallbacks) != 0 {
		return nil, ErrConfig
	}
	cfg := c.ConnConfig
	if stand {
		host := net.ParseIP(cfg.Host)
		if !strings.HasPrefix(cfg.Host, "/") && (host == nil || !host.IsLoopback()) {
			return nil, ErrConfig
		}
	} else if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || cfg.TLSConfig.ServerName == "" {
		return nil, ErrConfig
	}
	c.MaxConns = size
	c.MinConns = 0
	c.MinIdleConns = 0
	cfg.ConnectTimeout = protectedTime
	cfg.RuntimeParams = map[string]string{"application_name": "hitpipe-processing", "statement_timeout": "1800", "lock_timeout": "1600", "idle_in_transaction_session_timeout": "2000"}
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, ErrUnavailable
	}
	var ok bool
	err = p.QueryRow(ctx, `SELECT current_setting('server_version_num')='180007' AND current_database()=$1
 AND NOT rolsuper AND NOT rolbypassrls AND NOT rolcreaterole AND NOT rolcreatedb
 AND pg_has_role(current_user,$2,'member')
 AND NOT pg_has_role(current_user,'hp_processing_purge','member')
 AND CASE WHEN $2='hp_processing_app' THEN NOT pg_has_role(current_user,'hp_processing_worker','member')
 ELSE NOT pg_has_role(current_user,'hp_processing_app','member') END
 AND (SELECT max(version) FROM hp_processing.schema_version)=1 FROM pg_roles WHERE rolname=current_user`, dbName, role).Scan(&ok)
	if err != nil || !ok {
		p.Close()
		return nil, ErrUnavailable
	}
	return p, nil
}

func New(ctx context.Context, c Config, revision, standPath string, logger *slog.Logger) (*Runtime, error) {
	if c.validate() != nil || !commit.MatchString(revision) || logger == nil {
		return nil, ErrConfig
	}
	stand, err := loadStand(standPath, c)
	if err != nil {
		return nil, err
	}
	serverCert, err := keyPair(c.TLS.ServerCertificate, c.TLS.ServerKey)
	if err != nil {
		return nil, err
	}
	clientCert, err := keyPair(c.TLS.ClientCertificate, c.TLS.ClientKey)
	if err != nil {
		return nil, err
	}
	if string(serverCert.Certificate[0]) == string(clientCert.Certificate[0]) {
		return nil, ErrConfig
	}
	inCA, err := caFile(c.TLS.InboundCA)
	if err != nil {
		return nil, err
	}
	outCA, err := caFile(c.TLS.OutboundCA)
	if err != nil {
		return nil, err
	}
	boot, err := drafts.NewID()
	if err != nil {
		return nil, ErrUnavailable
	}
	id := startgate.Identity{RuntimeID: c.Identity, BootID: boot, Generation: c.Generation}
	g, err := startgate.New(id)
	if err != nil {
		return nil, ErrConfig
	}

	rt := &Runtime{logger: logger}
	success := false
	defer func() {
		if !success {
			rt.close()
		}
	}()
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	app, err := newPool(startup, c.AppDSN, c.OwnerDB, "hp_processing_app", c.Connections-c.Connections/2, standPath != "")
	if err != nil {
		return nil, err
	}
	rt.pool = append(rt.pool, app)
	worker, err := newPool(startup, c.WorkerDSN, c.OwnerDB, "hp_processing_worker", c.Connections/2, standPath != "")
	if err != nil {
		return nil, err
	}
	rt.pool = append(rt.pool, worker)
	if app.Config().ConnConfig.Host != worker.Config().ConnConfig.Host || app.Config().ConnConfig.Port != worker.Config().ConnConfig.Port {
		return nil, ErrConfig
	}
	rt.files, err = contentfiles.New(c.Content)
	if err != nil {
		return nil, ErrConfig
	}
	key, err := readSecret(c.CursorKey, 32)
	if err != nil {
		return nil, err
	}
	cursor, err := readcases.NewCursorKey(key)
	if err != nil {
		return nil, ErrConfig
	}
	rt.a = &access{gate: g, iam: newIAM(c.IAM, outbound(c.IAM, outCA, clientCert), id, clientCert), app: app, stand: stand}
	draftsAdapter := draftsAccess{rt.a}
	cases := casesAccess{rt.a}
	intake := intakeAccess{rt.a}
	obs := observationAccess{rt.a}
	dispatch := dispatchAccess{rt.a}
	serviceClock := clock{}
	ttl := 90 * 24 * time.Hour
	policy := drafts.Policy{ContentTTL: ttl, CheckTTL: time.Minute, ReplayNodes: 256}
	draftRepo := postgres.DraftStore{DB: app, Files: rt.files, Clock: serviceClock, Policy: policy, Codec: draftjson.Codec{}}
	rt.lifecycle = postgres.LifecycleStore{Drafts: draftRepo}
	draftService := drafts.Service{Repository: draftRepo, Authority: draftsAdapter, Boundary: draftsAdapter, Bindings: draftsAdapter,
		Clock: serviceClock, Policy: policy, Codec: draftjson.Codec{}}
	activationService := activation.Service{Repository: postgres.ActivationStore{Drafts: draftRepo}, Authority: draftsAdapter,
		Boundary: draftsAdapter, Bindings: draftsAdapter, Clock: serviceClock, Codec: draftjson.ActivationCodec{}}
	life := lifecycle.Service{Repository: rt.lifecycle, Authority: draftsAdapter, Boundary: draftsAdapter, Bindings: draftsAdapter,
		Queue: standQueue{stand}, Clock: serviceClock, Codec: draftjson.Codec{}}
	read := readcases.Service{Repository: postgres.CaseReadStore{DB: app, Content: rt.files, Cursors: cursor},
		Authority: cases, Boundary: cases, Clock: serviceClock}
	receive := receivesignals.Service{Contexts: intake, Boundary: intake, Clock: serviceClock,
		Decoder: normalizedsignal.Decoder{}, Store: postgres.Store{DB: app, Content: rt.files}}
	observe := observations.Service{Repository: postgres.ObservationStore{DB: app, Content: rt.files, HistoryRetention: ttl},
		Authority: obs, Boundary: obs, Clock: serviceClock, Codec: observationjson.Codec{}}
	serveTLS := &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientCAs: inCA,
		ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"http/1.1"}}

	business, err := casereadhttp.NewServer(c.Business, serveTLS, read, c.BFFName)
	if err != nil {
		return nil, err
	}
	if err = drafthttp.Attach(business, draftService, c.BFFName, limitBody); err != nil {
		return nil, err
	}
	if err = activationhttp.Attach(business, activationService, c.BFFName, limitBody); err != nil {
		return nil, err
	}
	if err = lifecyclehttp.Attach(business, life, c.BFFName); err != nil {
		return nil, err
	}
	if err = intakehttp.Attach(business, receive, c.Intake.Name); err != nil {
		return nil, err
	}
	oh, err := observationhttp.New(observe, c.Delivery.Name, limitBody)
	if err != nil {
		return nil, err
	}
	business.ErrorLog = slog.NewLogLogger(slog.DiscardHandler, slog.LevelError)
	previous := business.Handler
	business.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rt.a.stopping.Load() {
			writeError(w, r, 503, "PROCESSING_UNAVAILABLE")
			return
		}
		r = r.WithContext(withPermit(r.Context()))
		w = immediateResponse{ResponseWriter: w}
		if r.URL.Path == "/internal/v1/delivery-observations" {
			oh.ServeHTTP(w, r)
		} else {
			previous.ServeHTTP(w, r)
		}
	})
	rt.servers = []*http.Server{business,
		privateServer(c.Control, serveTLS, rt.barrier(c.IAM.Name)),
		privateServer(c.Health, serveTLS, rt.health(c.OPSName, revision)),
	}
	send, err := deliveryhttp.New(c.Delivery.Origin, outbound(c.Delivery, outCA, clientCert), limitBody, serviceClock)
	if err != nil {
		return nil, err
	}
	rt.sender = dispatchintent.Service{Repository: jobStore{DispatchStore: postgres.DispatchStore{DB: worker}, a: dispatch},
		Content: dispatchContent{Content: rt.files, a: dispatch}, Decoder: deliveryhttp.Codec{}, Sender: send, Boundary: dispatch, Clock: serviceClock,
		Policy: dispatchintent.Policy{Batch: 5, MaxAttempts: 3, Lease: 10 * time.Second, RequestTimeout: 1500 * time.Millisecond, BaseBackoff: time.Second, MaxBackoff: 5 * time.Second}, Jitter: dispatchintent.CryptoJitter}
	rt.caseJobs = postgres.CaseJobStore{DB: worker}
	rt.timeouts = postgres.CaseTimeoutStore{DB: worker, Files: rt.files, History: rt.files, Clock: serviceClock}
	rt.caseReaders = sourceReaders{a: rt.a, origin: c.Intake.Origin, transport: outbound(c.Intake, outCA, clientCert), jobs: rt.caseJobs}
	success = true
	return rt, nil
}

type jobStore struct {
	postgres.DispatchStore
	a dispatchAccess
}

func (j jobStore) Jobs(ctx context.Context, w string, n int) ([]dispatchintent.Job, error) {
	h := scopeOf(ctx)
	if h == nil {
		return nil, dispatchintent.ErrHeld
	}
	var jobs []dispatchintent.Job
	err := j.a.Start(ctx, dispatchintent.Claim{Request: dispatchintent.Request{WorkspaceID: w, Epoch: h.permit.epoch}}, func(c context.Context) error {
		var e error
		jobs, e = j.DispatchStore.Jobs(c, w, n)
		return e
	})
	return jobs, err
}

type dispatchContent struct {
	preparecase.Content
	a dispatchAccess
}

func (d dispatchContent) Read(ctx context.Context, ref preparecase.Ref, at time.Time) ([]byte, error) {
	h := scopeOf(ctx)
	if h == nil {
		return nil, dispatchintent.ErrHeld
	}
	var raw []byte
	err := d.a.a.start(ctx, h.permit.workspace, h.permit.principal, h.permit.epoch, func(c context.Context) error {
		var e error
		raw, e = d.Content.Read(c, ref, at)
		return e
	})
	return raw, err
}

func (rt *Runtime) close() {
	for _, s := range rt.servers {
		_ = s.Close()
	}
	for _, p := range rt.pool {
		p.Close()
	}
	if rt.files != nil {
		_ = rt.files.Close()
	}
}

func (rt *Runtime) Serve(ctx context.Context) error {
	defer rt.close()
	listeners := make([]net.Listener, 0, len(rt.servers))
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	for _, s := range rt.servers {
		l, err := net.Listen("tcp", s.Addr)
		if err != nil {
			return ErrUnavailable
		}
		listeners = append(listeners, l)
	}
	errs := make(chan error, len(listeners))
	for i, s := range rt.servers {
		l := listeners[i]
		go func() {
			err := s.ServeTLS(l, "", "")
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				errs <- ErrUnavailable
			}
		}()
	}
	rt.logger.Info("processing listeners started", "gate", "CLOSED")
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stop := make(chan struct{})
	var bg sync.WaitGroup
	bg.Go(func() { rt.registration(workerCtx, stop) })
	bg.Go(func() { rt.work(workerCtx, stop) })
	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-errs:
	}
	rt.a.stopping.Store(true)
	close(stop)
	shutdown, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	for _, s := range rt.servers {
		if s.Shutdown(shutdown) != nil {
			_ = s.Close()
		}
	}
	done := make(chan struct{})
	go func() { bg.Wait(); close(done) }()
	select {
	case <-done:
	case <-shutdown.Done():
		cancel()
		<-done
	}
	rt.logger.Info("processing stopped")
	return runErr
}

func (rt *Runtime) registration(ctx context.Context, stop <-chan struct{}) {
	if len(rt.a.stand) == 0 {
		return
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		default:
		}
		call, cancel := context.WithTimeout(ctx, protectedTime)
		err := rt.a.iam.sync(call, rt.a.gate)
		state, _ := rt.a.gate.State(call)
		cancel()
		if err == nil && state == startgate.Open {
			rt.logger.Info("processing runtime registered", "gate", "OPEN")
			return
		}
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (rt *Runtime) work(ctx context.Context, stop <-chan struct{}) {
	scopes := make([]string, 0, len(rt.a.stand))
	for w := range rt.a.stand {
		scopes = append(scopes, w)
	}
	sort.Strings(scopes)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		for _, w := range scopes {
			if rt.a.stopping.Load() {
				return
			}
			c := withPermit(ctx)
			p, err := rt.a.local(c, w, "ACTIVATE", nil)
			if err == nil {
				_ = rt.a.start(c, w, p.principal, p.epoch, func(x context.Context) error { return rt.lifecycle.Drain(x, w) })
			}
			rt.advanceCase(ctx, w)
			rt.advanceTimeout(ctx, w)
			c = withPermit(ctx)
			destination := rt.a.stand[w].config.Destination
			p, err = rt.a.local(c, w, "CONNECTOR_WRITE", &destination)
			if err != nil {
				continue
			}
			out, _ := rt.sender.RunScope(c, dispatchintent.Request{WorkspaceID: w, Epoch: p.epoch})
			for _, v := range out {
				rt.logger.Info("processing outbox transition", "message_id", v.MessageID, "state", v.State)
			}
		}
	}
}

// The existing services emit protected responses inside the shared gate.
// net/http can buffer a small response until ServeHTTP returns. Flush it
// before returning from Write so a barrier also orders the socket start.
type immediateResponse struct{ http.ResponseWriter }

func (w immediateResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w immediateResponse) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	if err == nil {
		err = http.NewResponseController(w.ResponseWriter).Flush()
	}
	return n, err
}

func privateServer(addr string, t *tls.Config, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: h, TLSConfig: t.Clone(), ReadHeaderTimeout: protectedTime,
		ReadTimeout: protectedTime, WriteTimeout: protectedTime, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10,
		ErrorLog: slog.NewLogLogger(slog.DiscardHandler, slog.LevelError)}
}

func peerOK(r *http.Request, name string) bool {
	s := r.TLS
	return s != nil && s.Version == tls.VersionTLS13 && len(s.VerifiedChains) > 0 && len(s.PeerCertificates) > 0 && s.PeerCertificates[0].VerifyHostname(name) == nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code string) {
	id := r.Header.Get("X-Request-ID")
	if !drafts.UUID(id) {
		id, _ = drafts.NewID()
	}
	writeJSON(w, status, drafts.Error{Code: code, Message: "Processing request could not be completed", RequestID: id})
}

func routeOK(r *http.Request, path, method string) bool {
	return r.Method == method && r.URL.Path == path && r.URL.RawPath == "" && r.URL.RawQuery == "" && !r.URL.ForceQuery
}

func (rt *Runtime) barrier(peer string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !peerOK(r, peer):
			writeError(w, r, 403, "BARRIER_ACCESS_DENIED")
			return
		case !routeOK(r, "/internal/v1/runtime/barrier", http.MethodPost):
			writeError(w, r, 404, "NOT_FOUND")
			return
		}
		ids := r.Header.Values("X-Request-ID")
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if len(ids) != 1 || !drafts.UUID(ids[0]) || err != nil || media != "application/json" || r.Header.Get("Content-Encoding") != "" {
			writeError(w, r, 400, "INVALID_REQUEST")
			return
		}
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limitBody))
		var command drafts.BarrierCommand
		if err != nil || draftjson.Validate("BarrierCommand", b) != nil || json.Unmarshal(b, &command) != nil {
			writeError(w, r, 400, "INVALID_REQUEST")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), protectedTime)
		defer cancel()
		ack, err := rt.a.gate.ApplyBarrier(ctx, startgate.BarrierCommand{TargetRuntimeID: command.TargetRuntimeID, BootID: command.BootID,
			Generation: uint64(command.Generation), RevocationID: command.RevocationID, Subject: subject(command.Scope, command.SubjectID, command.WorkspaceID),
			Epoch: uint64(command.Epoch), DeadlineAt: command.DeadlineAt})
		if err != nil {
			switch {
			case errors.Is(err, startgate.ErrIdentity), errors.Is(err, startgate.ErrConflict):
				writeError(w, r, 409, "BARRIER_CONFLICT")
			case errors.Is(err, startgate.ErrInvalid):
				writeError(w, r, 400, "INVALID_REQUEST")
			default:
				writeError(w, r, 503, "BARRIER_UNAVAILABLE")
			}
			return
		}
		writeJSON(w, 200, drafts.BarrierAck{RevocationID: ack.RevocationID, RuntimeID: ack.RuntimeID, BootID: ack.BootID,
			WorkspaceID: ack.Subject.WorkspaceID, Scope: string(ack.Subject.Scope), SubjectID: ack.Subject.SubjectID,
			Generation: int64(ack.Generation), AppliedEpoch: int64(ack.AppliedEpoch), OldStartSectionsDrained: ack.OldStartSectionsDrained, AcknowledgedAt: ack.AcknowledgedAt})
	})
}

func (rt *Runtime) health(peer, revision string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !peerOK(r, peer):
			writeError(w, r, 403, "HEALTH_ACCESS_DENIED")
			return
		case !routeOK(r, "/internal/v1/health/processing", http.MethodGet):
			writeError(w, r, 404, "NOT_FOUND")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), protectedTime)
		defer cancel()
		state, e := rt.a.gate.State(ctx)
		status := "CLOSED"
		if state == startgate.Open && !rt.a.stopping.Load() {
			status = "READY"
			if e != nil || len(rt.a.stand) == 0 || rt.pool[0].Ping(ctx) != nil || rt.pool[1].Ping(ctx) != nil {
				status = "DEGRADED"
			}
		}
		writeJSON(w, 200, drafts.Health{BuildRevision: revision, CheckedAt: time.Now().UTC(), ProtocolVersion: "1.0", Service: "PROCESSING", Status: status})
	})
}
