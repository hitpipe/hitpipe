//go:build hit146_iam_fixture

// Built by the disposable Processing qualification runner as an overlay on the
// accepted IAM checkout. This is only an assembly for existing IAM components.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
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
	IAM, Processing                                   runtimeaccess.Binding
	CA, Certificate, Key                              string
	ClientCertificate, ClientKey                      string
	Workspace, Revocation, MissingACK, Dir            string
	Owner, Session, Destination, ProcessingControlPin string
}

type clock struct{}

func (clock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	if err := ctx.Err(); err != nil {
		return sessionpolicy.Observation{}, err
	}
	return sessionpolicy.Observation{At: time.Now().UTC(), Trusted: true}, nil
}

// testOwner is the retained resource owner input for the isolated synthetic
// records. The real IAM application still checks caller, session, membership,
// workspace, binding, gate and revocations against its PostgreSQL data.
type testOwner struct {
	f  fixture
	db *pgxpool.Pool
}

func (o testOwner) Prove(ctx context.Context, actor authorizeaction.Actor, request authorizeaction.Request) (authorizeaction.Proof, error) {
	if actor.PrincipalID != o.f.Owner || actor.SessionID != o.f.Session ||
		actor.WorkspaceID != o.f.Workspace || request.WorkspaceID != o.f.Workspace ||
		request.RuntimeID != o.f.Processing.RuntimeID {
		return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
	}
	proof := authorizeaction.Proof{Verified: true, PrincipalID: actor.PrincipalID,
		WorkspaceID: actor.WorkspaceID, Action: request.Action, ResourceID: request.ResourceID}
	switch {
	case request.Action == "CASE_READ" && request.ResourceID != nil:
		proof.Operation = "getCaseHistory"
	case request.Action == "CONNECTOR_WRITE" && request.ResourceID != nil && *request.ResourceID == o.f.Destination:
		proof.Operation, proof.BindingGrantID = "connectService", o.f.Destination
	default:
		return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
	}
	tx, err := o.db.Begin(ctx)
	if err != nil {
		return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", request.WorkspaceID); err != nil {
		return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
	}
	resource := ""
	if request.Action == "CASE_READ" {
		resource = *request.ResourceID
	}
	var verified bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM hp_processing.operations o JOIN hp_processing.accepted_configs c
		  ON c.workspace_id=o.workspace_id AND c.config_id=o.selected_config_id
		 WHERE o.workspace_id=$1 AND o.owner_user_id=$2 AND o.state='ACTIVE'
		   AND c.canonical_config->>'mode'='TEST'
		   AND ($3='' OR c.canonical_config->>'destination_connection_id'=$3)
		   AND ($4='' OR EXISTS (SELECT 1 FROM hp_processing.case_identity k
		        WHERE k.workspace_id=o.workspace_id AND k.operation_id=o.operation_id
		          AND k.case_id::text=$4 AND k.owner_user_id=o.owner_user_id))
	)`, request.WorkspaceID, actor.PrincipalID, proof.BindingGrantID, resource).Scan(&verified); err != nil || !verified {
		return authorizeaction.Proof{}, authorizeaction.ErrUnavailable
	}
	return proof, nil
}

type statusWriter struct {
	http.ResponseWriter
	status   int
	response *bytes.Buffer
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.response != nil {
		w.response.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func trace(h http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(out http.ResponseWriter, req *http.Request) {
		w := &statusWriter{ResponseWriter: out, status: 200}
		var registration, response bytes.Buffer
		if req.URL.Path == privatehttp.RegisterPath && req.Body != nil {
			w.response = &response
			req.Body = struct {
				io.Reader
				io.Closer
			}{io.TeeReader(req.Body, &registration), req.Body}
		}
		h.ServeHTTP(w, req)
		var r runtimeaccess.Registration
		if registration.Len() != 0 && json.Unmarshal(registration.Bytes(), &r) == nil {
			var snapshot struct {
				Gate string `json:"gate"`
			}
			_ = json.Unmarshal(response.Bytes(), &snapshot)
			log.Info("iam-register", "gate", snapshot.Gate, "status", w.status, "runtime_id", r.RuntimeID, "boot_id", r.BootID, "generation", r.Generation)
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
	s.log.Info("coordinator-send", "runtime_id", target.Identity.RuntimeID, "generation", target.Identity.Generation, "ack_received", err == nil, "ack", ack)
	return ack, err
}

func waitFile(ctx context.Context, dir, name string) error {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

func driveRevocation(ctx context.Context, coordinator *revocation.Coordinator, f fixture,
	id string, effective bool, log *slog.Logger) error {
	subject := startgate.Subject{Scope: startgate.Workspace, WorkspaceID: &f.Workspace, SubjectID: f.Workspace}
	if !effective {
		subject = startgate.Subject{Scope: startgate.Session, SubjectID: f.Session}
	}
	accepted, err := coordinator.Begin(ctx, revocation.Intent{ID: id, ReasonCode: "TEST_REVOCATION", Subject: subject})
	if err != nil {
		return err
	}
	if accepted.DeadlineAt.Sub(accepted.AcceptedAt) != 5*time.Second {
		return errors.New("wrong effective-revocation deadline")
	}
	observed, err := coordinator.Drive(ctx, accepted.ID)
	if err != nil {
		return err
	}
	log.Info("coordinator-drive", "status", observed.Status, "accepted_at", observed.AcceptedAt,
		"deadline_at", observed.DeadlineAt, "effective_at", observed.EffectiveAt, "missing", observed.MissingRuntimeIDs(),
		"participants", observed.Participants, "epoch", observed.Epoch)
	if effective {
		if observed.Status != revocation.Effective || observed.EffectiveAt == nil ||
			!observed.EffectiveAt.Before(accepted.DeadlineAt) || len(observed.Participants) != 2 || len(observed.MissingRuntimeIDs()) != 0 {
			return errors.New("exact real participant ACKs did not make the revocation effective on time")
		}
		return os.WriteFile(filepath.Join(f.Dir, "effective-observed"), []byte("EFFECTIVE\n"), 0600)
	}
	missing := observed.MissingRuntimeIDs()
	if observed.Status != revocation.Pending || observed.EffectiveAt != nil || len(missing) != 1 || missing[0] != f.Processing.RuntimeID {
		return errors.New("missing Processing ACK did not retain PENDING")
	}
	wait := time.NewTimer(time.Until(accepted.DeadlineAt) + 80*time.Millisecond)
	defer wait.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wait.C:
	}
	final, err := coordinator.Drive(ctx, accepted.ID)
	if err != nil {
		return err
	}
	log.Info("coordinator-deadline", "status", final.Status, "effective", final.EffectiveAt != nil,
		"deadline_breached", final.DeadlineBreached(), "missing", final.MissingRuntimeIDs(),
		"observed_at", time.Now().UTC(), "elapsed_ms", time.Since(accepted.AcceptedAt).Milliseconds())
	missing = final.MissingRuntimeIDs()
	if final.Status != revocation.Incident || final.EffectiveAt != nil || !final.DeadlineBreached() ||
		len(missing) != 1 || missing[0] != f.Processing.RuntimeID {
		return errors.New("missing ACK did not produce a deadline incident")
	}
	return nil
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
	ownerDB, err := pgxpool.New(ctx, "host=/var/run/postgresql user=hp_processing_app dbname=hit78 sslmode=disable pool_max_conns=2")
	if err != nil {
		return err
	}
	defer ownerDB.Close()
	iam, err := composition.New(pool, clock{}, testOwner{f, ownerDB}, f.IAM.Registration, []runtimeaccess.Binding{f.IAM, f.Processing})
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
	controlHandler, err := iam.ControlHandler(ca, f.IAM.CertificateFingerprint)
	if err != nil {
		return err
	}
	controlTLS, err := barrierhttp.ServerTLS(cert, ca, f.IAM.CertificateFingerprint)
	if err != nil {
		return err
	}
	controlListener, err := net.Listen("tcp", "127.0.0.2:9443")
	if err != nil {
		return err
	}
	defer controlListener.Close()
	control := &http.Server{Handler: trace(controlHandler, log), TLSConfig: controlTLS, ReadHeaderTimeout: time.Second}
	defer control.Close()
	go control.ServeTLS(controlListener, "", "")
	store, err := revocationdb.New(pool)
	if err != nil {
		return err
	}
	target := func(b runtimeaccess.Binding, pin, origin string) barrierhttp.ControlBinding {
		return barrierhttp.ControlBinding{
			Identity:                startgate.Identity{RuntimeID: b.RuntimeID, BootID: b.BootID, Generation: uint64(b.Generation)},
			RegistrationFingerprint: b.CertificateFingerprint, ControlFingerprint: pin, Origin: origin,
		}
	}
	client, err := barrierhttp.NewClientWithControlBindings(clientCert, ca, []barrierhttp.ControlBinding{
		target(f.IAM, fmt.Sprintf("sha256:%x", sha256.Sum256(cert.Certificate[0])), "https://127.0.0.2:9443"),
		target(f.Processing, f.ProcessingControlPin, "https://127.0.0.1:9443"),
	})
	if err != nil {
		return err
	}
	coordinator, err := revocation.New(store, tracedSender{client, log}, time.Now)
	if err != nil {
		return err
	}
	log.Info("iam-listening")
	if err := waitFile(ctx, f.Dir, "open-iam"); err != nil {
		return err
	}
	if err := iam.Start(ctx); err != nil {
		return fmt.Errorf("open registered IAM roster: %w", err)
	}
	log.Info("iam-gate-open")
	if err := waitFile(ctx, f.Dir, "revoke"); err != nil {
		return err
	}
	if err := driveRevocation(ctx, coordinator, f, f.Revocation, true, log); err != nil {
		return err
	}
	if err := waitFile(ctx, f.Dir, "revoke-without-processing"); err != nil {
		return err
	}
	return driveRevocation(ctx, coordinator, f, f.MissingACK, false, log)
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
