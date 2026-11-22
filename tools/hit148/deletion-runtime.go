//go:build hit148_probe && integration && linux

package casedeletion_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	httpreg "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/casedeletion"
	pgowner "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/casedeletion"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	pm "github.com/hitpipe/hitpipe/services/processing/maintenance"
)

type deletionRuntimeStand struct {
	config, stand, revision, session, workspace string
	ops, bff                                    *http.Client
	registrations, delivery                     *atomic.Int32
}

func newDeletionRuntimeStand(t *testing.T, keys labTLS, owner *pgowner.OwnerAuthority, db fixtureDB,
	fixture seededFiles, volume string, write func(string, []byte) string, ca string) deletionRuntimeStand {
	t.Helper()
	requireOK(t, os.Mkdir(filepath.Join(filepath.Dir(volume), "queue"), 0700))
	workspace := db.workspaces[0]
	serial := int64(20)
	leaf := func(name string, usage x509.ExtKeyUsage) tls.Certificate {
		serial++
		return keys.issue(serial, name, usage)
	}
	pemPair := func(name string, leaf tls.Certificate) (string, string) {
		private, err := x509.MarshalPKCS8PrivateKey(leaf.PrivateKey)
		requireOK(t, err)
		return write(name+".pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Certificate[0]})),
			write(name+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}))
	}
	// HIT-154 authority is the real pinned issuer in the other listener.
	// This ordinary runtime IAM neighbor provides only a controlled OPEN
	// membership snapshot, usable solely for the designated TEST incarnation.
	// CASE_READ still checks the actual IAM session and current OWNER.
	var registrations, delivery atomic.Int32
	id := fixtureUUID()
	serviceClient := leaf("processing-runtime.test", x509.ExtKeyUsageClientAuth)
	serve := func(addr, name string, h http.HandlerFunc) {
		t.Helper()
		cert := leaf(name, x509.ExtKeyUsageServerAuth)
		s := &http.Server{Handler: h, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13,
			MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, ClientCAs: keys.ca,
			ClientAuth: tls.RequireAndVerifyClientCert}, ReadHeaderTimeout: time.Second}
		l, err := net.Listen("tcp", addr)
		requireOK(t, err)
		t.Cleanup(func() { _ = s.Close() })
		go s.ServeTLS(l, "", "")
	}
	serve("127.0.0.2:8443", "runtime-iam.test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.TLS == nil || len(r.TLS.PeerCertificates) == 0 ||
			r.TLS.PeerCertificates[0].VerifyHostname("processing-runtime.test") != nil {
			w.WriteHeader(403)
			return
		}
		var body struct {
			Runtime    string  `json:"runtime_id"`
			Boot       string  `json:"boot_id"`
			Generation int     `json:"generation"`
			Workspace  string  `json:"workspace_id"`
			Action     string  `json:"action"`
			Resource   *string `json:"resource_id"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 16384)).Decode(&body) != nil || body.Runtime != id {
			w.WriteHeader(403)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/internal/v1/runtime/register":
			if body.Generation != 2 || body.Boot == "" {
				w.WriteHeader(403)
				return
			}
			registrations.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"runtime_id": id, "boot_id": body.Boot,
				"generation": 2, "snapshot_revision": time.Now().UnixMicro(), "denials": []any{}, "gate": "OPEN"})
		case "/internal/v1/authorize":
			if body.Workspace != workspace || body.Action != "CASE_READ" || body.Resource == nil ||
				(*body.Resource != fixture.Case && *body.Resource != fixture.OtherCase) {
				w.WriteHeader(403)
				return
			}
			err := owner.WithCurrentOwner(r.Context(), app.OwnerCheck{WorkspaceID: body.Workspace,
				Session: r.Header.Get("X-HitPipe-Session")}, func(_ context.Context, actual app.OwnerIdentity) error {
				return json.NewEncoder(w).Encode(map[string]any{"allowed": true, "epoch": 7, "checked_at": time.Now().UTC(),
					"revocation_pending": false, "principal_id": actual.PrincipalID, "session_id": db.sessions[0],
					"workspace_id": workspace, "role": "OWNER"})
			})
			if err != nil {
				w.WriteHeader(403)
			}
		default:
			w.WriteHeader(404)
		}
	})
	serve("127.0.0.4:8443", "delivery.test", func(w http.ResponseWriter, r *http.Request) {
		delivery.Add(1)
		w.WriteHeader(503)
	})
	serverCert, serverKey := pemPair("processing-server", leaf("processing-server.test", x509.ExtKeyUsageServerAuth))
	clientCert, clientKey := pemPair("processing-runtime", serviceClient)
	config, err := json.Marshal(map[string]any{
		"restore_case_deletion": true, "runtime_id": id, "generation": 2, "database_name": "hit148restore",
		"pool_connections": 4, "business_listen": "127.0.0.1:8443", "control_listen": "127.0.0.1:9443", "health_listen": "127.0.0.1:9444",
		"application_dsn_file": write("app-dsn", []byte(restoreDSN+"hp_processing_app")),
		"worker_dsn_file":      write("worker-dsn", []byte(restoreDSN+"hp_processing_worker")),
		"content_directory":    volume, "cursor_key_file": write("cursor", []byte(strings.Repeat("s", 32))),
		"bff_name": "bff.test", "ops_name": "ops.test",
		"iam":      map[string]string{"origin": "https://127.0.0.2:8443", "name": "runtime-iam.test"},
		"intake":   map[string]string{"origin": "https://127.0.0.3:8443", "name": "intake.test"},
		"delivery": map[string]string{"origin": "https://127.0.0.4:8443", "name": "delivery.test"},
		"tls": map[string]string{"server_certificate": serverCert, "server_key": serverKey,
			"client_certificate": clientCert, "client_key": clientKey, "inbound_ca": ca, "outbound_ca": ca},
	})
	requireOK(t, err)
	stand, err := json.Marshal(map[string]any{"workspaces": []any{map[string]any{
		"workspace_id": workspace, "owner_user_id": db.owners[0], "session_id": db.sessions[0],
		"session_file": write("session", []byte(db.session[0])), "csrf_file": write("csrf", []byte(strings.Repeat("s", 43))),
		"source_connection_id": "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "destination_connection_id": "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
		"source_account_id": "acct_synthetic", "destination_workspace_id": "cu_synthetic", "mode": "TEST", "price_alias": "price_alias",
		"list_alias": "list_alias", "assignee_alias": "assignee_alias", "price_id": "price_fixed", "list_id": "list_synthetic",
		"assignee_id": "assignee_synthetic", "service_label": "Synthetic service",
		"queue_snapshot_file": filepath.Join(filepath.Dir(volume), "queue", "snapshot"), "queue_lock_file": write("queue-lock", nil),
	}}})
	requireOK(t, err)
	client := func(name string) *http.Client {
		cert := leaf(name, x509.ExtKeyUsageClientAuth)
		return &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{DisableKeepAlives: true,
			TLSClientConfig: &tls.Config{RootCAs: keys.ca, Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13}}}
	}
	revision := os.Getenv("HIT148_PROCESSING_SOURCE")
	if len(revision) != 40 {
		t.Fatal("missing exact Processing build revision")
	}
	return deletionRuntimeStand{config: write("runtime.json", config), stand: write("stand.json", stand),
		revision: revision, session: db.session[0], workspace: workspace, ops: client("ops.test"),
		bff: client("bff.test"), registrations: &registrations, delivery: &delivery}
}

func (s deletionRuntimeStand) health(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		res, err := s.ops.Get("https://127.0.0.1:9444/internal/v1/health/processing")
		if err == nil {
			var h struct {
				Status   string `json:"status"`
				Revision string `json:"build_revision"`
			}
			err = json.NewDecoder(res.Body).Decode(&h)
			res.Body.Close()
			if err == nil && res.StatusCode == 200 && h.Status == want && h.Revision == s.revision {
				t.Logf("actual Processing mTLS Health 200 %s build_revision=%s", want, h.Revision)
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("actual Processing did not reach %s", want)
}

func (s deletionRuntimeStand) read(t *testing.T, c, suffix string, want int) []byte {
	t.Helper()
	q, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		"https://127.0.0.1:8443/api/v1/workspaces/"+s.workspace+"/cases/"+c+suffix, nil)
	requireOK(t, err)
	q.Header.Set("X-HitPipe-Session", s.session)
	resp, err := s.bff.Do(q)
	requireOK(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	requireOK(t, err)
	if resp.StatusCode != want {
		t.Fatalf("read %s returned %d instead of %d: %s", suffix, resp.StatusCode, want, raw)
	}
	return raw
}

func (s deletionRuntimeStand) checkClosedStandalone(t *testing.T) {
	t.Helper()
	checkedCommand(t, nil, nil, "/test/processing", "-config", s.config, "-check-config")
	cmd := exec.CommandContext(t.Context(), "/test/processing", "-config", s.config, "-stand-config", s.stand)
	var log bytes.Buffer
	cmd.Stderr = &log
	requireOK(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("standalone Processing: %s", log.Bytes())
		}
	}()
	s.health(t, "CLOSED")
	if s.registrations.Load() != 0 {
		t.Fatal("uncoordinated restored Processing registered the gate")
	}
}

func (s deletionRuntimeStand) checkRestored(t *testing.T, local pm.CaseAuthority, issuer *app.Registry,
	consumer *httpreg.Client, stale app.Snapshot, workspace string, fixture seededFiles, outage *atomic.Bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	exit := make(chan error, 1)
	var fresh atomic.Bool
	var staleDenied, sourceDenied atomic.Bool
	guard := func(c context.Context, verifyAndOpen func(context.Context, []pm.DeletionReceipt) error) error {
		snapshot := stale
		if fresh.Load() {
			var err error
			snapshot, err = consumer.Snapshot(c, workspace, 2)
			if err != nil {
				sourceDenied.Store(true)
				return err
			}
		}
		open := func(c context.Context) error {
			var receipts []pm.DeletionReceipt
			for _, d := range snapshot.Payload.Decisions {
				for _, r := range d.Receipts {
					receipts = append(receipts, pm.DeletionReceipt(r))
				}
			}
			return verifyAndOpen(c, receipts)
		}
		err := issuer.WithCurrent(c, workspace, 2, snapshot, open)
		if !fresh.Load() && errors.Is(err, app.ErrConflict) {
			staleDenied.Store(true)
		}
		return err
	}
	go func() {
		exit <- local.ServeRestored(ctx, s.config, s.stand, s.revision, guard, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	}()
	defer func() {
		cancel()
		select {
		case err := <-exit:
			requireOK(t, err)
		case <-time.After(6 * time.Second):
			t.Error("restored runtime did not stop")
		}
	}()
	s.health(t, "CLOSED")
	s.read(t, fixture.Case, "/history", 503)
	deadline := time.Now().Add(2 * time.Second)
	for !staleDenied.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !staleDenied.Load() || s.registrations.Load() != 0 {
		t.Fatal("stale issuer snapshot did not hold real Processing CLOSED")
	}
	outage.Store(true)
	fresh.Store(true)
	deadline = time.Now().Add(3 * time.Second)
	for !sourceDenied.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !sourceDenied.Load() || s.registrations.Load() != 0 {
		t.Fatal("unavailable publisher did not hold Processing closed")
	}
	s.health(t, "CLOSED")
	s.read(t, fixture.Case, "/history", 503)
	t.Log("actual Processing stayed CLOSED on stale revision and when the live publisher refused its snapshot")
	outage.Store(false)
	s.health(t, "READY")
	raw := s.read(t, fixture.Case, "/history", 200)
	var actual struct {
		Items []struct {
			Entry    string    `json:"entry_id"`
			Case     string    `json:"case_id"`
			NewState string    `json:"new_state"`
			Reason   string    `json:"reason_code"`
			Expiry   time.Time `json:"expires_at"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &actual) != nil || len(actual.Items) != 1 || actual.Items[0].Entry != fixture.History.Entry ||
		actual.Items[0].Case != fixture.Case || !actual.Items[0].Expiry.Equal(fixture.History.Ref.ExpiresAt) ||
		actual.Items[0].Reason != "DELIVERY_RESULT_VERIFIED" || actual.Items[0].NewState != "CONFIRMED" {
		t.Fatalf("deleted history did not preserve original metadata: %s", raw)
	}
	t.Logf("actual mTLS readback of removed history detail returned 200, entry=%s original expires_at=%s",
		actual.Items[0].Entry, actual.Items[0].Expiry.Format(time.RFC3339Nano))
	s.read(t, fixture.OtherCase, "/history", 200)
	detail := s.read(t, fixture.Case, "", 200)
	var fields struct {
		Context         []json.RawMessage `json:"context"`
		ContextComplete bool              `json:"context_complete"`
		ContextExpired  bool              `json:"context_expired"`
	}
	if json.Unmarshal(detail, &fields) != nil || fields.Context == nil || len(fields.Context) != 0 ||
		fields.ContextComplete || fields.ContextExpired {
		t.Fatal("deleted context was returned or presented as expired")
	}
	t.Log("actual mTLS case detail returned 200 without either selected context version")
	if s.delivery.Load() != 0 || s.registrations.Load() != 1 {
		t.Fatal("unexpected protected re-registration or external effect")
	}
}
