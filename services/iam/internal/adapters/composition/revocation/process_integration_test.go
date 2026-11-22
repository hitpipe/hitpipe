//go:build integration

package revocation_test

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
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	barrierhttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation"
	revocationdb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/revocation"
	runtimedb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgxpool"
)

const childFile = "HIT116_TEST_PROCESS_FILE"

type targetProcess struct {
	Dir             string
	Address         string
	IAMCA           string
	IAMPin          string
	Leaf            string
	Inventory       []runtimeaccess.Registration
	Own             runtimeaccess.Registration
	Probes          map[string]startgate.StartRequest
	DropRevocation  string
	DelayRevocation string
}

type recoveryProcess struct {
	Dir             string
	Leaf            string
	CAs             string
	Targets         map[string]string
	ObserveIncident string
}

type processConfig struct {
	Target   *targetProcess
	Recovery *recoveryProcess
}

// This entrypoint is re-executed from the race-enabled integration binary.
// It is a no-op during discovery and is never part of a production executable.
func TestHIT116Process(t *testing.T) {
	name := os.Getenv(childFile)
	if name == "" {
		return
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal("read isolated process configuration")
	}
	var cfg processConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal("decode isolated process configuration")
	}
	switch {
	case cfg.Target != nil && cfg.Recovery == nil:
		err = runTarget(t.Context(), *cfg.Target)
	case cfg.Recovery != nil && cfg.Target == nil:
		err = runRecovery(t.Context(), *cfg.Recovery)
	default:
		err = errors.New("unexpected process mode")
	}
	if err != nil {
		t.Fatal(err)
	}
}

func readLeaf(prefix string) (tls.Certificate, error) {
	return tls.LoadX509KeyPair(prefix+".crt", prefix+".key")
}

func readCA(name string) (*x509.CertPool, error) {
	raw, err := os.ReadFile(name)
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(raw) {
		return nil, errors.New("load isolated TLS roots")
	}
	return roots, nil
}

func runRecovery(ctx context.Context, cfg recoveryProcess) error {
	pool, err := childPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := revocationdb.New(pool)
	if err != nil {
		return err
	}
	leaf, err := readLeaf(cfg.Leaf)
	if err != nil {
		return err
	}
	roots, err := readCA(cfg.CAs)
	if err != nil {
		return err
	}
	client, err := barrierhttp.NewClient(leaf, roots, cfg.Targets)
	if err != nil {
		return err
	}
	var transport revocation.Sender = client
	if cfg.ObserveIncident != "" {
		transport = sendFunc(func(ctx context.Context, target revocation.Participant, cmd startgate.BarrierCommand) (startgate.BarrierAck, error) {
			if cmd.RevocationID == cfg.ObserveIncident {
				persisted, err := store.Get(ctx, cmd.RevocationID)
				if err != nil || persisted.Status != revocation.Incident || persisted.EffectiveAt != nil ||
					persisted.IncidentID == nil || persisted.DeadlineBreachedAt == nil || len(persisted.MissingRuntimeIDs()) != 1 {
					return startgate.BarrierAck{}, errors.New("incomplete expired revocation was not persisted as incident before retry")
				}
				raw, err := json.Marshal(persisted)
				if err != nil {
					return startgate.BarrierAck{}, err
				}
				if err := mark(cfg.Dir, "before-replay", raw); err != nil {
					return startgate.BarrierAck{}, err
				}
			}
			return client.Send(ctx, target, cmd)
		})
	}
	coordinator, err := revocation.New(store, transport, time.Now)
	if err != nil {
		return err
	}
	next, err := coordinator.Recover(ctx, "", 16)
	if err != nil {
		return fmt.Errorf("recover in fresh process: %w", err)
	}
	return os.WriteFile(filepath.Join(cfg.Dir, "recovered"), []byte(next), 0600)
}

func childPool(ctx context.Context) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(os.Getenv("HITPIPE_IAM_TEST_APP_DSN"))
	if err != nil || !labDatabaseConfig(cfg, "hp_hit81_app") {
		return nil, errors.New("isolated app config required")
	}
	cfg.MaxConns = 3
	return pgxpool.NewWithConfig(ctx, cfg)
}

func runTarget(ctx context.Context, cfg targetProcess) error {
	leaf, err := readLeaf(cfg.Leaf)
	if err != nil {
		return err
	}
	roots, err := readCA(cfg.IAMCA)
	if err != nil {
		return err
	}
	identity := runtimeIdentity(cfg.Own)
	gate, err := startgate.New(identity)
	if err != nil {
		return err
	}
	handler, err := barrierhttp.NewHandler(gate, roots, cfg.IAMPin)
	if err != nil {
		return err
	}
	serverTLS, err := barrierhttp.ServerTLS(leaf, roots, cfg.IAMPin)
	if err != nil {
		return err
	}
	control, err := net.Listen("tcp4", net.JoinHostPort(cfg.Address, "9443"))
	if err != nil {
		return err
	}
	defer control.Close()
	business, err := net.Listen("tcp4", net.JoinHostPort(cfg.Address, "0"))
	if err != nil {
		return err
	}
	defer business.Close()
	controlSrv := server(traceBarrier(cfg, handler))
	controlSrv.TLSConfig = serverTLS
	businessSrv := server(businessProbe(cfg, gate))
	defer controlSrv.Close()
	defer businessSrv.Close()
	listenerErrors := make(chan error, 2)
	go func() { listenerErrors <- controlSrv.ServeTLS(control, "", "") }()
	go func() { listenerErrors <- businessSrv.Serve(business) }()
	ready, err := json.Marshal(childReady{PID: os.Getpid(), Business: "http://" + business.Addr().String()})
	if err != nil {
		return err
	}
	if err := mark(cfg.Dir, "ready", ready); err != nil {
		return err
	}

	// The process is already listening, but its new boot cannot start any
	// protected action. Mirror the verified registration flow used by HIT-115.
	if err := waitFile(ctx, filepath.Join(cfg.Dir, "register")); err != nil {
		return err
	}
	err = openFromRegistry(ctx, cfg, gate)
	outcome := "open"
	if errors.Is(err, runtimeaccess.ErrConflict) {
		outcome = "conflict"
	} else if err != nil {
		return fmt.Errorf("exact fixture registration failed: %w", err)
	}
	if err := mark(cfg.Dir, "registration", []byte(outcome)); err != nil {
		return err
	}
	select {
	case err := <-listenerErrors:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func openFromRegistry(ctx context.Context, cfg targetProcess, gate *startgate.Gate) error {
	pool, err := childPool(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := runtimedb.New(pool, cfg.Inventory)
	if err != nil {
		return err
	}
	pending, err := store.Register(ctx, cfg.Own)
	if err != nil {
		return err
	}
	if err := applySnapshot(ctx, gate, pending, startgate.Syncing); err != nil {
		return err
	}
	opened, err := store.OpenLocal(ctx, cfg.Own, pending)
	if err != nil {
		return err
	}
	return applySnapshot(ctx, gate, opened, startgate.Open)
}

func applySnapshot(ctx context.Context, gate *startgate.Gate, snapshot runtimeaccess.Snapshot, state startgate.State) error {
	if !snapshot.Complete || snapshot.Gate != string(state) || snapshot.Generation < 1 || snapshot.Revision < 1 {
		return errors.New("fixture registration did not receive a complete snapshot for this state")
	}
	denials := make([]startgate.Denial, len(snapshot.Denials))
	for i, row := range snapshot.Denials {
		if row.Epoch < 1 {
			return errors.New("invalid epoch in fixture snapshot")
		}
		denials[i] = startgate.Denial{
			Subject: startgate.Subject{Scope: startgate.Scope(row.Scope), SubjectID: row.SubjectID, WorkspaceID: row.WorkspaceID},
			Epoch:   uint64(row.Epoch), RevocationID: row.RevocationID, AcceptedAt: row.AcceptedAt, EffectiveAt: row.EffectiveAt,
		}
	}
	return gate.ApplySnapshot(ctx, startgate.RuntimeSnapshot{
		RuntimeID: snapshot.RuntimeID, BootID: snapshot.BootID, Generation: uint64(snapshot.Generation),
		SnapshotRevision: uint64(snapshot.Revision), Gate: state, Denials: denials,
	})
}

func server(handler http.Handler) *http.Server {
	return &http.Server{
		Handler: handler, ReadHeaderTimeout: time.Second, ReadTimeout: time.Second,
		WriteTimeout: 8 * time.Second, IdleTimeout: time.Second, MaxHeaderBytes: 8192,
		ErrorLog: log.New(io.Discard, "", 0),
	}
}

// In the lab, a protected business action has a separately observable start
// and end. Finishing it later must not keep the actual-start section locked.
func businessProbe(cfg targetProcess, gate *startgate.Gate) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path
		req, ok := cfg.Probes[name]
		if !ok || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		err := gate.Start(r.Context(), req, func(ctx context.Context) error {
			if name != "/inflight" {
				return nil
			}
			if err := mark(cfg.Dir, "inflight-entered", []byte(time.Now().UTC().Format(time.RFC3339Nano))); err != nil {
				return err
			}
			budget, cancel := context.WithTimeout(ctx, 1700*time.Millisecond)
			defer cancel()
			return waitFile(budget, filepath.Join(cfg.Dir, "release-start"))
		})
		switch {
		case errors.Is(err, startgate.ErrDenied):
			w.WriteHeader(http.StatusForbidden)
			return
		case errors.Is(err, startgate.ErrClosed):
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		case err != nil:
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if name == "/inflight" {
			budget, cancel := context.WithTimeout(r.Context(), 7*time.Second)
			defer cancel()
			if err := waitFile(budget, filepath.Join(cfg.Dir, "finish-action")); err != nil {
				w.WriteHeader(http.StatusGatewayTimeout)
				return
			}
		}
		w.WriteHeader(http.StatusCreated)
	})
}

// This wrapper is a fixture fault: only after a real successful application
// does it discard the first selected HTTP response and sever the connection.
func traceBarrier(cfg targetProcess, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != barrierhttp.Path {
			next.ServeHTTP(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var command struct {
			ID string `json:"revocation_id"`
		}
		if json.Unmarshal(body, &command) != nil || !runtimeaccess.ValidUUID(command.ID) {
			next.ServeHTTP(w, r)
			return
		}
		tracePath := filepath.Join(cfg.Dir, "wire-"+command.ID)
		f, err := os.OpenFile(tracePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, writeErr := fmt.Fprintf(f, "%x\n", sha256.Sum256(body))
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if command.ID == cfg.DelayRevocation {
			first, err := os.OpenFile(filepath.Join(cfg.Dir, "network-delay-armed"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_ = first.Close()
				timer := time.NewTimer(5200 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-r.Context().Done():
					_ = mark(cfg.Dir, "network-delay-cancelled", nil)
					return
				case <-timer.C:
				}
			} else if !errors.Is(err, os.ErrExist) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		if command.ID != cfg.DropRevocation {
			next.ServeHTTP(w, r)
			return
		}
		wire := httptest.NewRecorder()
		next.ServeHTTP(wire, r)
		if wire.Code == http.StatusOK {
			first, err := os.OpenFile(filepath.Join(cfg.Dir, "ack-dropped"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err == nil {
				_, writeErr := first.Write(wire.Body.Bytes())
				closeErr := first.Close()
				if writeErr != nil || closeErr != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				conn, _, err := http.NewResponseController(w).Hijack()
				if err == nil {
					_ = conn.Close()
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if !errors.Is(err, os.ErrExist) {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		for k, values := range wire.Header() {
			w.Header()[k] = values
		}
		w.WriteHeader(wire.Code)
		_, _ = w.Write(wire.Body.Bytes())
	})
}

func waitFile(ctx context.Context, name string) error {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(name); err == nil {
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

func mark(dir, name string, value []byte) error {
	file, err := os.CreateTemp(dir, ".marker-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(value)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, name))
}
