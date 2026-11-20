package postgres

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
)

const processHost = "127.0.0.1"

type processStand struct {
	t                                                          *testing.T
	dir, binary, config, stand, token, csrf, session, identity string
	pki                                                        *testfixtures.DraftPKI
	cert                                                       string
	files                                                      string
	iamOpen                                                    atomic.Bool
	iamStale                                                   atomic.Bool
	bootMu                                                     sync.Mutex
	current, old                                               drafts.RuntimeRegistration
	deliveryCount                                              atomic.Int32
	deliverySeen                                               chan string
	deliverWait                                                chan struct{}
	waitDelivery                                               atomic.Bool
	workspaces                                                 map[string]string
	pauseHandshake                                             atomic.Bool
	pauseSourceHandshake                                       atomic.Bool
	manualCases                                                map[string]bool
	intakeReply                                                http.HandlerFunc
	queryReply                                                 http.HandlerFunc
	denyRecover                                                atomic.Bool
	handshakeEntered                                           chan struct{}
	handshakeProceed                                           chan struct{}
	denials                                                    []drafts.DenyEpochSnapshot
}

func (s *processStand) file(name string, data []byte) string {
	s.t.Helper()
	path := filepath.Join(s.dir, name)
	if err := os.WriteFile(path, data, 0600); err != nil {
		s.t.Fatal(err)
	}
	return path
}

func (s *processStand) json(name string, value any) string {
	s.t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		s.t.Fatal(err)
	}
	return s.file(name, body)
}

func (s *processStand) keypair(name string, c tls.Certificate) (string, string) {
	s.t.Helper()
	key, err := x509.MarshalPKCS8PrivateKey(c.PrivateKey)
	if err != nil {
		s.t.Fatal(err)
	}
	return s.file(name+".pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Certificate[0]})),
		s.file(name+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
}

func (s *processStand) client(peer string, expired bool, pki *testfixtures.DraftPKI) *http.Client {
	tr := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, RootCAs: s.pki.Root}}
	if peer != "" {
		tr.TLSClientConfig.Certificates = []tls.Certificate{pki.Leaf(s.t, peer, false, expired)}
	}
	return &http.Client{Transport: tr, Timeout: 3 * time.Second}
}

func (s *processStand) peer(address, name string, h http.HandlerFunc) {
	s.t.Helper()
	l, err := net.Listen("tcp", address)
	if err != nil {
		s.t.Fatal(err)
	}
	host, _, _ := net.SplitHostPort(address)
	cfg := &tls.Config{Certificates: []tls.Certificate{s.pki.Issue(s.t, name, net.ParseIP(host), true, false)},
		ClientCAs: s.pki.Root, ClientAuth: tls.RequireAndVerifyClientCert, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		NextProtos: []string{"http/1.1"}}
	if name == "delivery.test" || name == "intake.test" {
		cfg.GetConfigForClient = func(*tls.ClientHelloInfo) (*tls.Config, error) {
			pause := &s.pauseHandshake
			if name == "intake.test" {
				pause = &s.pauseSourceHandshake
			}
			if pause.CompareAndSwap(true, false) {
				close(s.handshakeEntered)
				select {
				case <-s.handshakeProceed:
				case <-time.After(3 * time.Second):
					return nil, fmt.Errorf("controlled handshake timed out")
				}
			}
			return nil, nil
		}
	}
	server := &http.Server{TLSConfig: cfg, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || r.TLS.PeerCertificates[0].VerifyHostname("processing-client.test") != nil {
			w.WriteHeader(403)
			return
		}
		h(w, r)
	})}
	go server.ServeTLS(l, "", "")
	s.t.Cleanup(func() { server.Close(); l.Close() })
}

func (s *processStand) iam(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(io.LimitReader(r.Body, 256<<10))
	if err != nil || r.Method != "POST" || !drafts.UUID(r.Header.Get("X-Request-ID")) {
		w.WriteHeader(400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/internal/v1/runtime/register":
		var in drafts.RuntimeRegistration
		if draftjson.Validate("RuntimeRegistration", b) != nil || json.Unmarshal(b, &in) != nil || in.RuntimeID != s.identity || in.CertificateFingerprint != s.cert || in.Service != "PROCESSING" {
			w.WriteHeader(400)
			return
		}
		s.bootMu.Lock()
		if in.BootID != s.current.BootID {
			s.old = s.current
			s.current = in
		}
		out := in
		if s.iamStale.Load() {
			out = s.old
		}
		denials := append([]drafts.DenyEpochSnapshot{}, s.denials...)
		s.bootMu.Unlock()
		stage, rev := "CLOSED", int64(1)
		if s.iamOpen.Load() {
			stage, rev = "OPEN", 2
		}
		_ = json.NewEncoder(w).Encode(drafts.RuntimeSnapshot{RuntimeID: out.RuntimeID, BootID: out.BootID, Generation: in.Generation, Gate: stage,
			SnapshotRevision: rev, Denials: denials})
	case "/internal/v1/authorize":
		var in drafts.AuthorizeRequest
		if draftjson.Validate("AuthorizeRequest", b) != nil || json.Unmarshal(b, &in) != nil || in.RuntimeID != s.identity {
			w.WriteHeader(400)
			return
		}
		if !s.iamOpen.Load() || s.workspaces[in.WorkspaceID] == "" || r.Header.Get("X-HitPipe-Session") != s.token ||
			(in.Action == "CASE_READ" && s.manualCases[in.WorkspaceID]) || (in.Action == "CASE_RECOVER" && s.denyRecover.Load()) {
			w.WriteHeader(403)
			return
		}
		role := "OWNER"
		_ = json.NewEncoder(w).Encode(drafts.Authorization{Allowed: true, Epoch: 7, CheckedAt: time.Now().UTC(),
			WorkspaceID: &in.WorkspaceID, Role: &role, PrincipalID: draftString(owner), SessionID: &s.session})
	default:
		w.WriteHeader(404)
	}
}

func (s *processStand) delivery(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/internal/v1/delivery-query" && s.queryReply != nil {
		s.queryReply(w, r)
		return
	}
	if r.URL.Path != "/internal/v1/delivery-intents" || r.Method != "POST" {
		w.WriteHeader(404)
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	var in struct {
		MessageID string `json:"message_id"`
	}
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &in) != nil || !drafts.UUID(in.MessageID) || r.Header.Get("X-Request-ID") != in.MessageID {
		w.WriteHeader(400)
		return
	}
	path := filepath.Join(s.dir, "received-"+in.MessageID)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	kind := "APPLIED"
	if os.IsExist(err) {
		prior, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(prior, raw) {
			w.WriteHeader(409)
			return
		}
		kind = "DUPLICATE"
	} else {
		if err != nil {
			w.WriteHeader(503)
			return
		}
		_, err = file.Write(raw)
		if err == nil {
			err = file.Sync()
		}
		file.Close()
		dir, openErr := os.Open(s.dir)
		if err == nil && openErr == nil {
			err = dir.Sync()
		}
		if dir != nil {
			dir.Close()
		}
		if err != nil || openErr != nil {
			w.WriteHeader(503)
			return
		}
	}
	s.deliveryCount.Add(1)
	s.deliverySeen <- in.MessageID
	if s.waitDelivery.Load() {
		select {
		case <-s.deliverWait:
		case <-r.Context().Done():
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(map[string]string{"message_id": in.MessageID, "disposition": kind})
}

func createProcessStand(t *testing.T, binary, files string, workspaces map[string]string) *processStand {
	t.Helper()
	id, _ := newID()
	s := &processStand{t: t, dir: t.TempDir(), binary: binary, files: files, pki: testfixtures.NewDraftPKI(t),
		token: strings.Repeat("s", 48), csrf: strings.Repeat("z", 43), session: config, identity: id, workspaces: workspaces,
		deliverySeen: make(chan string, 8), deliverWait: make(chan struct{}), handshakeEntered: make(chan struct{}), handshakeProceed: make(chan struct{})}
	ca := s.file("ca.pem", s.pki.CAPEM())
	server, key := s.keypair("processing", s.pki.Issue(t, "processing.test", net.ParseIP(processHost), true, false))
	clientLeaf := s.pki.Leaf(t, "processing-client.test", false, false)
	client, clientKey := s.keypair("processing-client", clientLeaf)
	sum := sha256.Sum256(clientLeaf.Certificate[0])
	s.cert = "sha256:" + hex.EncodeToString(sum[:])
	conn := "host=/var/run/postgresql dbname=hit78 sslmode=disable user="
	app := s.file("app.dsn", []byte(conn+"hp_processing_app"))
	worker := s.file("worker.dsn", []byte(conn+"hp_processing_worker"))
	cursor := s.file("cursor.key", bytes.Repeat([]byte{0x55}, 32))
	s.config = s.json("processing.json", map[string]any{
		"business_listen": "127.0.0.1:8443", "control_listen": "127.0.0.1:9443", "health_listen": "127.0.0.1:9444",
		"runtime_id": id, "generation": 1, "database_name": "hit78", "application_dsn_file": app, "worker_dsn_file": worker,
		"cursor_key_file": cursor, "content_directory": s.files, "pool_connections": 5, "bff_name": "bff.test", "ops_name": "ops.test",
		"tls": map[string]string{"server_certificate": server, "server_key": key, "client_certificate": client,
			"client_key": clientKey, "inbound_ca": ca, "outbound_ca": ca},
		"iam":      map[string]string{"origin": "https://127.0.0.2:8443", "name": "iam.test"},
		"intake":   map[string]string{"origin": "https://127.0.0.3:8443", "name": "intake.test"},
		"delivery": map[string]string{"origin": "https://127.0.0.4:8443", "name": "delivery.test"},
	})
	sessionFile := s.file("session", []byte(s.token))
	csrfFile := s.file("csrf", []byte(s.csrf))
	var slots []map[string]string
	for ws, account := range workspaces {
		lock := s.file("queue-lock-"+ws, nil)
		slots = append(slots, map[string]string{"workspace_id": ws, "owner_user_id": owner, "session_id": s.session, "session_file": sessionFile, "csrf_file": csrfFile,
			"source_connection_id": config, "destination_connection_id": destination, "source_account_id": account,
			"destination_workspace_id": "cu_synthetic", "mode": "TEST", "price_alias": "price_alias", "list_alias": "list_alias",
			"assignee_alias": "assignee_alias", "price_id": "price_fixed", "list_id": "list_synthetic", "assignee_id": "assignee_synthetic",
			"service_label": "Synthetic service", "queue_lock_file": lock, "queue_snapshot_file": filepath.Join(s.dir, "queue-"+ws+".json")})
	}
	s.stand = s.json("stand.json", map[string]any{"workspaces": slots})
	s.peer("127.0.0.2:8443", "iam.test", s.iam)
	s.peer("127.0.0.3:8443", "intake.test", func(w http.ResponseWriter, r *http.Request) {
		if s.intakeReply == nil {
			w.WriteHeader(503)
			return
		}
		s.intakeReply(w, r)
	})
	s.peer("127.0.0.4:8443", "delivery.test", s.delivery)
	return s
}

type processingChild struct {
	cmd     *exec.Cmd
	done    chan error
	logPath string
}

func (s *processStand) boot(log string) *processingChild {
	s.t.Helper()
	logPath := filepath.Join(s.dir, log)
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		s.t.Fatal(err)
	}
	cmd := exec.Command(s.binary, "-config", s.config, "-stand-config", s.stand)
	cmd.Stdout, cmd.Stderr = out, out
	if err = cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	p := &processingChild{cmd: cmd, done: make(chan error, 1), logPath: logPath}
	go func() { p.done <- cmd.Wait(); close(p.done); out.Close() }()
	s.t.Cleanup(func() { _ = cmd.Process.Kill(); <-p.done })
	return p
}

func (s *processStand) call(peer, port, method, path string, body []byte, headers map[string]string) (int, []byte, error) {
	client := s.client(peer, false, s.pki)
	return s.send(client, port, method, path, body, headers)
}

func (s *processStand) send(client *http.Client, port, method, path string, body []byte, headers map[string]string) (int, []byte, error) {
	r, err := http.NewRequest(method, "https://127.0.0.1:"+port+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	id, _ := newID()
	r.Header.Set("X-Request-ID", id)
	if method == "POST" {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	res, err := client.Do(r)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	return res.StatusCode, out, err
}

func (s *processStand) must(peer, port, method, path string, b []byte, h map[string]string, want int) []byte {
	s.t.Helper()
	status, out, err := s.call(peer, port, method, path, b, h)
	if err != nil || status != want {
		s.t.Fatalf("%s %s -> %d wanted %d: %s (%v)", method, path, status, want, out, err)
	}
	return out
}

func (s *processStand) pollHealth(process *processingChild, stage string) (int, []byte) {
	s.t.Helper()
	timeout := time.After(8 * time.Second)
	for {
		select {
		case err := <-process.done:
			log, _ := os.ReadFile(process.logPath)
			s.t.Fatalf("processing exited: %v, %s", err, log)
		case <-timeout:
			log, _ := os.ReadFile(process.logPath)
			s.t.Fatalf("health %s did not arrive: %s", stage, log)
		default:
		}
		status, out, err := s.call("ops.test", "9444", "GET", "/internal/v1/health/processing", nil, nil)
		var h drafts.Health
		if err == nil && status == 200 && draftjson.Validate("Health", out) == nil && json.Unmarshal(out, &h) == nil && h.Status == stage {
			return status, out
		}
		time.Sleep(40 * time.Millisecond)
	}
}

func marshalProcess(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func (s *processStand) command(path string, value any, id string, want int) []byte {
	s.t.Helper()
	return s.must("bff.test", "8443", "POST", path, marshalProcess(s.t, value),
		map[string]string{"X-HitPipe-Session": s.token, "X-CSRF-Token": s.csrf, "X-Request-ID": id}, want)
}

func (s *processStand) activate(f draftFixture) (drafts.ActivationCommand, drafts.Operation) {
	return s.activateWithConfig(f, f.config())
}

func (s *processStand) activateWithConfig(f draftFixture, c drafts.DraftConfig) (drafts.ActivationCommand, drafts.Operation) {
	s.t.Helper()
	c.PriceAlias, c.ListAlias, c.AssigneeAlias, c.ServiceLabel = draftString("price_alias"), draftString("list_alias"), draftString("assignee_alias"), draftString("Synthetic service")
	rev := int64(1)
	draft := drafts.DraftWrite{Config: c, Command: f.meta(nil)}
	s.command(f.pathFor(drafts.Save), draft, draft.Command.RequestID, 201)
	meta := f.meta(&rev)
	b := s.command(f.pathFor(drafts.Check), meta, meta.RequestID, 200)
	var check drafts.ReadinessCheck
	if json.Unmarshal(b, &check) != nil || !check.Passed || check.ActivationPreview == nil {
		s.t.Fatal("missing server activation preview", string(b))
	}
	rehearse := drafts.RehearsalRequest{Command: f.meta(&rev), DraftID: f.id, DraftRevision: rev, ReadinessCheckID: check.CheckID,
		Examples: []drafts.ScenarioExample{
			{SyntheticSessionAlias: "example_paid", AmountMinor: 100, Paid: true, MissingContextKeys: []string{}},
			{SyntheticSessionAlias: "example_unpaid", AmountMinor: 100, MissingContextKeys: []string{}},
			{SyntheticSessionAlias: "example_zero", Paid: true, MissingContextKeys: []string{}},
			{SyntheticSessionAlias: "example_duplicate", AmountMinor: 100, Paid: true, Duplicate: true, MissingContextKeys: []string{}},
			{SyntheticSessionAlias: "example_missing", AmountMinor: 100, Paid: true, MissingContextKeys: []string{"brief"}},
		}}
	b = s.command(f.pathFor(drafts.Rehearse), rehearse, rehearse.Command.RequestID, 200)
	var result drafts.RehearsalResult
	if json.Unmarshal(b, &result) != nil {
		s.t.Fatal("invalid rehearsal response", string(b))
	}
	in := drafts.ActivationCommand{Command: f.meta(nil), DraftID: f.id, DraftRevision: rev, ReadinessCheckID: check.CheckID,
		RehearsalID: result.RehearsalID, AcceptedConfigDigest: check.ActivationPreview.AcceptedConfigDigest, AcceptanceTextVersion: "managed-operation-v1", ConfirmActivate: true}
	b = s.command("/api/v1/workspaces/"+f.workspace+"/operations/activate", in, in.Command.RequestID, 201)
	var op drafts.Operation
	if json.Unmarshal(b, &op) != nil || op.AcceptedConfigDigest == nil || *op.AcceptedConfigDigest != in.AcceptedConfigDigest || op.FirstActivationAt == nil {
		s.t.Fatal("accepted hash or activation time missing")
	}
	return in, op
}

func TestProcessingExecutableOwnedStand(t *testing.T) {
	binary := os.Getenv("HIT111_PROCESSING_BIN")
	if binary == "" {
		t.Skip("requires HIT-111 owned PostgreSQL and built executable")
	}
	f := newDraftFixtureBase(t)
	barrierScope := newDraftFixtureBase(t)
	job := prepared(t, []string{})
	other := prepared(t, []string{})
	blocked := prepared(t, []string{})
	// Move the existing decision service onto the same owned content root that
	// the standalone process will open. The seed facts come from the established
	// HIT-79 test fixture. No automatic preparation runs in the executable.
	for _, p := range []*preparedFixture{&job, &other, &blocked} {
		p.service.Content = f.files
		p.service.Repository = DecisionStore{DB: connect(t), Content: f.files}
	}
	stand := createProcessStand(t, binary, f.path, map[string]string{f.workspace: "acct_controlled", barrierScope.workspace: "acct_controlled", job.request.WorkspaceID: "acct_synthetic", other.request.WorkspaceID: "acct_synthetic", blocked.request.WorkspaceID: "acct_synthetic"})
	stand.manualCases = map[string]bool{job.request.WorkspaceID: true, other.request.WorkspaceID: true, blocked.request.WorkspaceID: true}
	// The local database deliberately has no TLS endpoint. Without explicitly
	// enabling the loopback test process it cannot start.
	var refusal *exec.ExitError
	if log, e := exec.Command(binary, "-config", stand.config).CombinedOutput(); !errors.As(e, &refusal) || refusal.ExitCode() != 2 {
		t.Fatalf("insecure production DB startup was not refused: %s %v", log, e)
	}
	invalid := stand.json("invalid.json", map[string]string{"business_listen": "0.0.0.0:8443"})
	if _, e := exec.Command(binary, "-config", invalid, "-check-config").CombinedOutput(); !errors.As(e, &refusal) || refusal.ExitCode() != 2 {
		t.Fatalf("invalid listen configuration accepted: %v", e)
	}
	initial := stand.boot("initial.log")
	stand.pollHealth(initial, "CLOSED")
	path := "/api/v1/workspaces/" + f.workspace
	valid := map[string]string{"X-HitPipe-Session": stand.token}
	stand.must("bff.test", "8443", "GET", path+"/operations", nil, valid, 503)
	stand.must("bff.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 403)
	stand.must("ops.test", "9444", "GET", "/internal/v1/runtime/barrier", nil, nil, 404)
	stand.must("bff.test", "9443", "POST", "/internal/v1/runtime/barrier", []byte("{}"), nil, 403)
	for _, tc := range []struct {
		peer    string
		expired bool
		pki     *testfixtures.DraftPKI
	}{
		{"", false, stand.pki}, {"ops.test", true, stand.pki}, {"ops.test", false, testfixtures.NewDraftPKI(t)},
	} {
		client := stand.client(tc.peer, tc.expired, tc.pki)
		if _, _, err := stand.send(client, "9444", "GET", "/internal/v1/health/processing", nil, nil); err == nil {
			t.Fatal("invalid certificate accepted", tc.peer, tc.expired)
		}
	}
	stand.iamOpen.Store(true)
	stand.pollHealth(initial, "READY")
	cmd, op := stand.activate(f)
	intake := intakeWire(t, f.workspace, func(m map[string]any) {
		m["stripe_account_id"] = "acct_controlled"
		m["source_created_at"] = op.FirstActivationAt.Format(time.RFC3339Nano)
		m["first_received_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		m["expires_at"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	})
	receipt := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", intake, nil, 202)
	stand.must("bff.test", "8443", "POST", "/internal/v1/intake-signals", intake, nil, 403)
	var admit struct {
		RecordID string `json:"record_id"`
	}
	if json.Unmarshal(receipt, &admit) != nil || !drafts.UUID(admit.RecordID) {
		t.Fatal(string(receipt))
	}
	casePath := path + "/cases/" + admit.RecordID
	stand.must("bff.test", "8443", "GET", casePath+"/history", nil, valid, 200)

	result, err := job.service.Prepare(ctx, job.request)
	if err != nil {
		t.Fatal(err)
	}
	finish := sendFixture{f: job, message: result.MessageID}
	select {
	case id := <-stand.deliverySeen:
		if id != result.MessageID {
			t.Fatal(id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stand Delivery did not see prepared immutable outbox")
	}
	waitProcessOutbox(t, finish, "ACKED")
	tx := begin(t, connect(t), job.request.WorkspaceID)
	var episode string
	if err = tx.QueryRow(ctx, `SELECT episode_id::text FROM hp_processing.episodes WHERE workspace_id=$1 AND case_id=$2`, job.request.WorkspaceID, job.request.CaseID).Scan(&episode); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(ctx)
	obs := observationFixture{preparedFixture: job, lineage: result.MessageID, episode: episode}
	observed := observationWire(t, obs.observation(t, "RESULT_VERIFIED"))
	obsReceipt := stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", observed, nil, 202)
	if state, knowledge, _, _, fenced, _ := obs.stored(t); state != "CONFIRMED" || knowledge != "RESULT_VERIFIED" || !fenced {
		t.Fatal(state, knowledge, fenced)
	}

	// A restorable operation is used for PAUSE/RESUME. The untouched admitted
	// case needs a real episode completion before PAUSED is possible.
	tx = begin(t, connect(t), f.workspace)
	if _, err = tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at)
 VALUES($1,gen_random_uuid(),$2,statement_timestamp(),statement_timestamp())`, f.workspace, admit.RecordID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	lifePath := path + "/operations/" + op.OperationID
	pause := drafts.LifecycleCommand{Command: f.meta(&op.Revision), Action: lifecycle.Pause}
	pauseReply := stand.command(lifePath+"/lifecycle", pause, pause.Command.RequestID, 202)
	var draining drafts.Operation
	if json.Unmarshal(pauseReply, &draining) != nil || draining.State != "DRAINING" || draining.DrainDeadlineAt == nil {
		t.Fatal(string(pauseReply))
	}
	var paused drafts.Operation
	stop := time.Now().Add(4 * time.Second)
	for time.Now().Before(stop) {
		raw := stand.must("bff.test", "8443", "GET", path+"/operations", nil, valid, 200)
		var list drafts.Operations
		if json.Unmarshal(raw, &list) == nil && len(list.Items) == 1 && list.Items[0].State == "PAUSED" {
			paused = list.Items[0]
			break
		}
		time.Sleep(35 * time.Millisecond)
	}
	if paused.State != "PAUSED" {
		t.Fatal("runtime drain not scheduled")
	}
	generation, _ := newID()
	queueProof := lifecycle.QueueEvidence{Scope: lifecycle.QueueScope{WorkspaceID: f.workspace, OperationID: op.OperationID,
		ConfigDigest: *op.AcceptedConfigDigest, FirstActivationAt: *op.FirstActivationAt, PauseAcceptedAt: draining.DrainDeadlineAt.Add(-lifecycle.DrainLimit)},
		Generation: generation, Complete: true, Gaps: []string{}, ObservedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute)}
	stand.json("queue-"+f.workspace+".json", queueProof)
	prep := drafts.LifecycleCommand{Command: f.meta(&paused.Revision), Action: lifecycle.Prepare}
	stand.command(lifePath+"/lifecycle", prep, prep.Command.RequestID, 202)
	b := stand.must("bff.test", "8443", "GET", lifePath+"/resume-snapshot", nil, valid, 200)
	var snapshot drafts.ResumeSnapshot
	if json.Unmarshal(b, &snapshot) != nil || snapshot.SignalCount != 0 || snapshot.Gaps == nil {
		t.Fatal(string(b))
	}
	choice := lifecycle.Process
	resume := drafts.LifecycleCommand{Command: f.meta(&snapshot.Revision), Action: lifecycle.Accept, ResumeSnapshotID: &snapshot.SnapshotID, HeldDisposition: &choice}
	b = stand.command(lifePath+"/lifecycle", resume, resume.Command.RequestID, 202)
	var resumed drafts.Operation
	if json.Unmarshal(b, &resumed) != nil || resumed.State != "ACTIVE" || !resumed.FirstActivationAt.Equal(*op.FirstActivationAt) {
		t.Fatal(string(b))
	}

	_, forBarrier := stand.activate(barrierScope)
	checkRuntimeBarrier(t, stand, barrierScope, forBarrier)
	// Pause an already-started TLS handshake, acknowledge the workspace
	// barrier, then let Delivery continue. No old HTTP request may start.
	stand.pauseHandshake.Store(true)
	blockedResult, err := blocked.service.Prepare(ctx, blocked.request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-stand.handshakeEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("controlled TLS-start race did not reach the peer")
	}
	stand.bootMu.Lock()
	boot := stand.current
	stand.bootMu.Unlock()
	revokeID, _ := newID()
	blockedWS := blocked.request.WorkspaceID
	revoke := drafts.BarrierCommand{TargetRuntimeID: stand.identity, BootID: boot.BootID, Generation: boot.Generation,
		RevocationID: revokeID, WorkspaceID: &blockedWS, Scope: "WORKSPACE", SubjectID: blockedWS, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	rawAck := stand.must("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", marshalProcess(t, revoke), nil, 200)
	if draftjson.Validate("BarrierAck", rawAck) != nil {
		t.Fatal(string(rawAck))
	}
	stand.bootMu.Lock()
	stand.denials = append(stand.denials, drafts.DenyEpochSnapshot{WorkspaceID: &blockedWS, Scope: "WORKSPACE", SubjectID: blockedWS, Epoch: 8,
		RevocationID: revokeID, AcceptedAt: time.Now().Add(-time.Second).UTC()})
	stand.bootMu.Unlock()
	close(stand.handshakeProceed)
	select {
	case got := <-stand.deliverySeen:
		t.Fatal("old transport start passed the barrier", got)
	case <-time.After(160 * time.Millisecond):
	}
	blockedRow := sendFixture{f: blocked, message: blockedResult.MessageID}
	if state, _, _, attempt := blockedRow.outbox(t); state != "CLAIMED" || attempt != 1 {
		t.Fatal("denied start became an ACK or retry", state, attempt)
	}

	if err = initial.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-initial.done
	stand.iamStale.Store(true)
	restart := stand.boot("restart.log")
	stand.pollHealth(restart, "CLOSED")
	time.Sleep(120 * time.Millisecond)
	stand.must("bff.test", "8443", "GET", casePath+"/history", nil, valid, 503)
	stand.iamStale.Store(false)
	stand.pollHealth(restart, "READY")
	stand.must("bff.test", "8443", "GET", "/api/v1/workspaces/"+barrierScope.workspace+"/operations", nil, valid, 403)
	stand.bootMu.Lock()
	bootChanged := stand.old.BootID != "" && stand.current.BootID != stand.old.BootID
	stand.bootMu.Unlock()
	if !bootChanged {
		t.Fatal("restart reused boot identity")
	}
	replayed := stand.command(path+"/operations/activate", cmd, cmd.Command.RequestID, 201)
	var prior drafts.Operation
	if json.Unmarshal(replayed, &prior) != nil || prior.OperationID != op.OperationID || prior.Revision != op.Revision {
		t.Fatal("original command receipt was lost", string(replayed))
	}
	if again := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", intake, nil, 202); !bytes.Equal(again, receipt) {
		t.Fatal("intake dedup lost")
	}
	if again := stand.must("delivery.test", "8443", "POST", "/internal/v1/delivery-observations", observed, nil, 202); !bytes.Equal(again, obsReceipt) {
		t.Fatal("observation dedup lost")
	}
	if len(stand.must("bff.test", "8443", "GET", casePath+"/history", nil, valid, 200)) <= len("[]") {
		t.Fatal("history missing")
	}

	stand.waitDelivery.Store(true)
	second, err := other.service.Prepare(ctx, other.request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-stand.deliverySeen:
		if got != second.MessageID {
			t.Fatal("incorrect restart delivery", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown scenario never started delivery")
	}
	if err = restart.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	close(stand.deliverWait)
	select {
	case err = <-restart.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(7 * time.Second):
		t.Fatal("graceful shutdown did not finish")
	}
	waitProcessOutbox(t, sendFixture{f: other, message: second.MessageID}, "ACKED")
	if stand.deliveryCount.Load() != 2 {
		t.Fatal("a committed request was sent twice", stand.deliveryCount.Load())
	}
	for _, p := range []*processingChild{initial, restart} {
		log, err := os.ReadFile(p.logPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{stand.token, stand.csrf, "cu_synthetic", "acct_controlled", "canonical_config", "private_key", "http://"} {
			if bytes.Contains(log, []byte(secret)) {
				t.Fatal("process log leaked owner data")
			}
		}
		t.Log(string(log))
	}
	t.Log("PROCESS PASS: CLOSED→fresh IAM snapshot, mTLS allowlists, activation/digest, intake/history, prepared outbox/ACK, observation, pause/resume, barrier, kill/restart dedup and SIGTERM with in-flight ACK")
}

func waitProcessOutbox(t *testing.T, f sendFixture, wanted string) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		state, _, _, n := f.outbox(t)
		if state == wanted && n == 1 {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	state, _, _, n := f.outbox(t)
	t.Fatal("outbox expected committed send:", state, n)
}

func checkRuntimeBarrier(t *testing.T, s *processStand, f draftFixture, op drafts.Operation) {
	t.Helper()
	c := connect(t)
	block := begin(t, c, f.workspace)
	if _, err := block.Exec(ctx, `SELECT operation_id FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id=$2 FOR UPDATE`, f.workspace, op.OperationID); err != nil {
		t.Fatal(err)
	}
	change := drafts.LifecycleCommand{Command: f.meta(&op.Revision), Action: lifecycle.Pause}
	type result struct {
		status int
		body   []byte
		err    error
	}
	posted := make(chan result, 1)
	acks := make(chan result, 1)
	path := "/api/v1/workspaces/" + f.workspace + "/operations/" + op.OperationID + "/lifecycle"
	body := marshalProcess(t, change)
	go func() {
		status, b, err := s.call("bff.test", "8443", "POST", path, body, map[string]string{"X-HitPipe-Session": s.token, "X-CSRF-Token": s.csrf, "X-Request-ID": change.Command.RequestID})
		posted <- result{status, b, err}
	}()
	read := connect(t)
	deadline := time.Now().Add(time.Second)
	var waiting bool
	for !waiting && time.Now().Before(deadline) {
		if err := read.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name='hitpipe-processing'
 AND wait_event_type='Lock' AND query LIKE '%FOR UPDATE OF o%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if !waiting {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !waiting {
		t.Fatal("protected SQL did not reach controlled row lock")
	}
	s.bootMu.Lock()
	boot := s.current
	s.bootMu.Unlock()
	id, _ := newID()
	command := drafts.BarrierCommand{TargetRuntimeID: s.identity, BootID: boot.BootID, Generation: boot.Generation, RevocationID: id,
		Scope: "WORKSPACE", SubjectID: f.workspace, WorkspaceID: &f.workspace, Epoch: 8, DeadlineAt: time.Now().Add(2 * time.Second)}
	wire := marshalProcess(t, command)
	go func() {
		status, b, err := s.call("iam.test", "9443", "POST", "/internal/v1/runtime/barrier", wire, nil)
		acks <- result{status, b, err}
	}()
	select {
	case v := <-acks:
		t.Fatal("barrier acknowledged before protected SQL exited", v)
	case <-time.After(45 * time.Millisecond):
	}
	if err := block.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	a := <-acks
	p := <-posted
	var ack drafts.BarrierAck
	if a.err != nil || a.status != 200 || json.Unmarshal(a.body, &ack) != nil || !ack.OldStartSectionsDrained || ack.AppliedEpoch != 8 {
		t.Fatalf("invalid durable barrier ACK %d %s %v", a.status, a.body, a.err)
	}
	s.bootMu.Lock()
	s.denials = append(s.denials, drafts.DenyEpochSnapshot{WorkspaceID: command.WorkspaceID, Scope: command.Scope, SubjectID: command.SubjectID,
		Epoch: command.Epoch, RevocationID: command.RevocationID, AcceptedAt: time.Now().UTC().Add(-time.Second)})
	s.bootMu.Unlock()
	if p.err != nil || p.status != 202 && p.status != 403 {
		t.Fatal(fmt.Sprintf("unexpected racing reply %d %v", p.status, p.err))
	}
	s.must("bff.test", "8443", "GET", "/api/v1/workspaces/"+f.workspace+"/operations", nil, map[string]string{"X-HitPipe-Session": s.token}, 403)
}
