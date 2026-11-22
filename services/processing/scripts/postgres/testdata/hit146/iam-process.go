// Built by the disposable Processing qualification runner as an overlay on the
// accepted IAM checkout. This is only an assembly for existing IAM components.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	composition "github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam"
	privatehttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	barrierhttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation"
	revocationdb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	IAM, Processing              runtimeaccess.Binding
	CA, Certificate, Key         string
	ClientCertificate, ClientKey string
	Workspace, Revocation, Dir   string
}

type clock struct{}

func (clock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	if err := ctx.Err(); err != nil {
		return sessionpolicy.Observation{}, err
	}
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
}

type closedOwner struct{}

func (closedOwner) Prove(context.Context, authorizeaction.Actor, authorizeaction.Request) (authorizeaction.Proof, error) {
	return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func trace(h http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
		w := &statusWriter{ResponseWriter: out, status: 200}
		var registration bytes.Buffer
		if req.URL.Path == privatehttp.RegisterPath && req.Body != nil {
			req.Body = struct {
				io.Reader
				io.Closer
			}{io.TeeReader(req.Body, &registration), req.Body}
		}
		h.ServeHTTP(w, req)
		var r runtimeaccess.Registration
		if registration.Len() != 0 && json.Unmarshal(registration.Bytes(), &r) == nil {
			log.Info("iam-register", "status", w.status, "runtime_id", r.RuntimeID, "boot_id", r.BootID, "generation", r.Generation)
		} else {
			log.Info("iam-http", "status", w.status, "path", req.URL.Path)
		}
	})
}

type tracedSender struct {
	client *barrierhttp.Client
	log    *slog.Logger
}

func (s tracedSender) Send(ctx context.Context, target revocation.Participant, command startgate.BarrierCommand) (startgate.BarrierAck, error) {
	ack, err := s.client.Send(ctx, target, command)
	s.log.Info("coordinator-send", "runtime_id", target.Identity.RuntimeID, "generation", target.Identity.Generation, "ack_received", err == nil)
	return ack, err
}

func observeNoACK(ctx context.Context, pool *pgxpool.Pool, f fixture, leaf tls.Certificate, ca *x509.CertPool, log *slog.Logger) error {
	store, err := revocationdb.New(pool)
	if err != nil {
		return err
	}
	client, err := barrierhttp.NewClient(leaf, ca, map[string]string{
		f.IAM.RuntimeID: "https://127.0.0.2:9443", f.Processing.RuntimeID: "https://127.0.0.1:9443",
	})
	if err != nil {
		return err
	}
	rt, err := revocation.New(store, tracedSender{client, log}, time.Now)
	if err != nil {
		return err
	}
	accepted, err := rt.Begin(ctx, revocation.Intent{ID: f.Revocation, ReasonCode: "TEST_REVOCATION",
		Subject: startgate.Subject{Scope: startgate.Workspace, WorkspaceID: &f.Workspace, SubjectID: f.Workspace}})
	if err != nil {
		return err
	}
	pending, err := rt.Drive(ctx, accepted.ID)
	if err != nil {
		return err
	}
	log.Info("coordinator-pending", "status", pending.Status, "accepted_at", pending.AcceptedAt,
		"deadline_at", pending.DeadlineAt, "effective", pending.EffectiveAt != nil, "missing", len(pending.MissingRuntimeIDs()))
	if pending.Status != revocation.Pending || pending.EffectiveAt != nil || len(pending.MissingRuntimeIDs()) != 2 {
		return errors.New("no-ACK fixture did not retain its mandatory participants as pending")
	}
	wait := time.NewTimer(time.Until(accepted.DeadlineAt) + 80*time.Millisecond)
	defer wait.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait.C:
	}
	final, err := rt.Drive(ctx, accepted.ID)
	if err != nil {
		return err
	}
	log.Info("coordinator-deadline", "status", final.Status, "effective", final.EffectiveAt != nil,
		"deadline_breached", final.DeadlineBreached(), "missing", len(final.MissingRuntimeIDs()),
		"observed_at", time.Now().UTC(), "elapsed_ms", time.Since(accepted.AcceptedAt).Milliseconds())
	if final.Status != revocation.Incident || final.EffectiveAt != nil || !final.DeadlineBreached() || len(final.MissingRuntimeIDs()) != 2 {
		return errors.New("missing ACK did not produce a deadline incident")
	}
	return os.WriteFile(filepath.Join(f.Dir, "incident-observed"), []byte("INCIDENT\n"), 0600)
}

func run(ctx context.Context, path string, log *slog.Logger) error {
	raw, err := os.ReadFile(path)
	var f fixture
	if err != nil || json.Unmarshal(raw, &f) != nil {
		return errors.New("read disposable fixture")
	}
	cert, err := tls.LoadX509KeyPair(f.Certificate, f.Key)
	if err != nil {
		return errors.New("load disposable IAM certificate")
	}
	clientCert, err := tls.LoadX509KeyPair(f.ClientCertificate, f.ClientKey)
	if err != nil {
		return errors.New("load disposable IAM client certificate")
	}
	ca := x509.NewCertPool()
	if raw, err = os.ReadFile(f.CA); err != nil || !ca.AppendCertsFromPEM(raw) {
		return errors.New("load disposable root")
	}
	pool, err := pgxpool.New(ctx, "host=/var/run/postgresql user=hp_hit146_iam_app dbname=hit146_iam sslmode=disable pool_max_conns=4")
	if err != nil {
		return err
	}
	defer pool.Close()
	iam, err := composition.New(pool, clock{}, closedOwner{}, f.IAM.Registration, []runtimeaccess.Binding{f.IAM, f.Processing})
	if err != nil {
		return err
	}
	h, err := privatehttp.New(iam)
	if err != nil {
		return err
	}
	tlsConfig, err := privatehttp.TLSConfig(cert, ca, []runtimeaccess.Peer{
		{DNSName: f.IAM.DNSName, CertificateFingerprint: f.IAM.CertificateFingerprint},
		{DNSName: f.Processing.DNSName, CertificateFingerprint: f.Processing.CertificateFingerprint},
	})
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", "127.0.0.2:8443")
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: trace(h, log), TLSConfig: tlsConfig, ReadHeaderTimeout: time.Second}
	defer server.Close()
	go server.ServeTLS(listener, "", "")
	log.Info("iam-listening")
	for {
		if err = iam.Start(ctx); err == nil {
			break
		}
		if ctx.Err() != nil {
			return errors.New("local IAM failed to open after roster registration")
		}
		time.Sleep(40 * time.Millisecond)
	}
	log.Info("iam-gate-open")
	for {
		if _, err = os.Stat(filepath.Join(f.Dir, "revoke")); err == nil {
			return observeNoACK(ctx, pool, f, clientCert, ca, log)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 36*time.Second)
	defer cancel()
	if err := run(ctx, os.Args[1], log); err != nil {
		log.Error("iam-fixture", "error", err.Error())
		os.Exit(1)
	}
}
