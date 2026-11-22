package intake

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPrivateContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "content")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	d, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	now := time.Now()
	data := []byte("fixture")
	ref, err := d.Put(data, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ref.ID.String())
	got, err := d.Read(ref, now)
	if err != nil || string(got) != string(data) {
		t.Fatalf("read: %s %v", got, err)
	}
	if _, err = d.Read(ref, ref.Expires); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired content exposed")
	}
	bad := ref
	bad.SHA256 = digest([]byte("different"))
	if _, err = d.Read(bad, now); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Read(ref, now); err == nil {
		t.Fatal("unsafe permissions accepted")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	// The symlink targets another file even though the request uses a valid UUID.
	target := filepath.Join(root, "target")
	if err = os.WriteFile(target, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Read(ref, now); err == nil {
		t.Fatal("symlink accepted")
	}
	if _, err = OpenDirectory(path); err == nil {
		t.Fatal("symlink root accepted")
	}
}

func TestPostgresDurableIntake(t *testing.T) {
	dsn := os.Getenv("HIT91_TEST_APP_DSN")
	adminDSN := os.Getenv("HIT91_TEST_ADMIN_DSN")
	if dsn == "" || adminDSN == "" {
		if os.Getenv("HIT91_REQUIRE_POSTGRES") != "1" {
			t.Skip("isolated PostgreSQL fixture not requested")
		}
		t.Fatal("isolated fixture configuration missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	app, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	defer app.Close()
	var version int
	var role string
	if err = app.QueryRow(ctx, "SELECT current_setting('server_version_num')::int,current_user").Scan(&version, &role); err != nil {
		t.Fatal(err)
	}
	if version != 180007 || role != "hp_delivery_app" {
		t.Fatalf("unexpected fixture %d %s", version, role)
	}

	fixed := time.Now().UTC().Truncate(time.Microsecond)
	intent, err := DecodeIntent(signed(t, testFields(fixed)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(ctx, "INSERT INTO hp_delivery.destination_connections(workspace_id,connection_id,external_workspace_id,security_epoch,token_fingerprint,health) VALUES ($1,$2,$3,$4,decode(repeat('11',32),'hex'),'HEALTHY')", intent.WorkspaceID, intent.ConnectionID, intent.DestinationWorkspace, intent.Epoch); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "content")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	content, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	ledger, err := NewPostgres(app, content)
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return fixed }
	assertCounts := func(expected int) {
		t.Helper()
		for _, table := range []string{"inbox", "effect_lineage", "attempts", "intent_content"} {
			var count int
			if err := admin.QueryRow(ctx, "SELECT count(*) FROM hp_delivery."+table+" WHERE workspace_id=$1", intent.WorkspaceID).Scan(&count); err != nil || count != expected {
				t.Fatalf("%s: count=%d expected=%d, %v", table, count, expected, err)
			}
		}
	}
	assertCounts(0)

	expired := intent
	expired.FactsUntil = fixed
	if _, err = ledger.Accept(ctx, expired, now); !errors.Is(err, ErrStale) {
		t.Fatalf("expired facts: %v", err)
	}
	expired = intent
	expired.Deadline = fixed
	if _, err = ledger.Accept(ctx, expired, now); !errors.Is(err, ErrStale) {
		t.Fatalf("expired deadline: %v", err)
	}
	wrongEpoch := intent
	wrongEpoch.Epoch++
	if _, err = ledger.Accept(ctx, wrongEpoch, now); !errors.Is(err, ErrDenied) {
		t.Fatalf("wrong epoch: %v", err)
	}
	assertCounts(0)

	// Actual openat failure: keep the original object while removing its backing directory.
	faultPath := filepath.Join(t.TempDir(), "fault-content")
	if err = os.Mkdir(faultPath, 0700); err != nil {
		t.Fatal(err)
	}
	broken, err := OpenDirectory(faultPath)
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	if err = os.Remove(faultPath); err != nil {
		t.Fatal(err)
	}
	failFile, err := NewPostgres(app, broken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = failFile.Accept(ctx, intent, now); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("file failure: %v", err)
	}
	assertCounts(0)

	// The fixture's deferred trigger fails at actual COMMIT, after file durability.
	if _, err = admin.Exec(ctx, "INSERT INTO public.hit91_fail_commit VALUES ($1)", intent.MessageID); err != nil {
		t.Fatal(err)
	}
	if got, err := ledger.Accept(ctx, intent, now); !errors.Is(err, ErrUnavailable) || got.RecordID != uuid.Nil {
		t.Fatalf("commit failure returned a receipt: %+v %v", got, err)
	}
	assertCounts(0)
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one unreferenced file: %v %v", files, err)
	}
	if _, err = admin.Exec(ctx, "DELETE FROM public.hit91_fail_commit WHERE message_id=$1", intent.MessageID); err != nil {
		t.Fatal(err)
	}

	// Compete on both message identity and case/effect identity.
	var wg sync.WaitGroup
	competingCase := uuid.New()
	receipts := make(chan Receipt, 8)
	for n := 0; n < 8; n++ {
		wg.Go(func() {
			receipt, err := ledger.Accept(ctx, intent, now)
			if err != nil {
				t.Errorf("same message: %v", err)
				return
			}
			receipts <- receipt
		})
		wg.Go(func() {
			other := intent
			other.LineageID = uuid.New()
			other.MessageID = uuid.New()
			// Separate this case to independently exercise competing lineage inserts.
			other.CaseID = competingCase
			receipt, err := ledger.Accept(ctx, other, now)
			if err == nil {
				if receipt.RecordID == uuid.Nil {
					t.Error("empty competing receipt")
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Errorf("competing message: %v", err)
			}
		})
	}
	wg.Wait()
	close(receipts)
	assertCounts(2)
	var stable Receipt
	for receipt := range receipts {
		if stable.RecordID != uuid.Nil && receipt != stable {
			t.Fatalf("different repeated receipts: %+v %+v", stable, receipt)
		}
		stable = receipt
	}
	if stable.RecordID == uuid.Nil || stable.Disposition != "APPLIED" || stable.MessageID != intent.MessageID {
		t.Fatalf("unexpected original receipt: %+v", stable)
	}

	var ref ContentRef
	var state string
	var number int
	var fence, started bool
	err = admin.QueryRow(ctx, "SELECT c.rendered_payload_ref,c.rendered_payload_sha256,c.rendered_payload_bytes,c.expires_at,l.state,a.attempt_number,l.ever_dispatch_fenced,a.network_start_at IS NOT NULL FROM hp_delivery.intent_content c JOIN hp_delivery.attempts a USING(workspace_id,attempt_id) JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id) WHERE c.attempt_id=$1", stable.RecordID).Scan(&ref.ID, &ref.SHA256, &ref.Size, &ref.Expires, &state, &number, &fence, &started)
	if err != nil || state != "PREPARED" || number != 1 || fence || started {
		t.Fatalf("unexpected durable state %s %d %v %v %v", state, number, fence, started, err)
	}
	payload, err := content.Read(ref, fixed)
	if err != nil || string(payload) != string(intent.Content) {
		t.Fatalf("stored payload did not round-trip: %v", err)
	}
	var target struct {
		Workspace string `json:"destination_workspace_id"`
		List      string `json:"target_list_id"`
		Assignee  string `json:"assignee_id"`
	}
	if err := json.Unmarshal(payload, &target); err != nil {
		t.Fatal(err)
	}
	if target.Workspace != intent.DestinationWorkspace || target.List != intent.List || target.Assignee != intent.Assignee {
		t.Fatal("committed content did not preserve the accepted destination")
	}

	var visible int
	tenantTx, err := app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tenantTx.Rollback(ctx)
	if _, err = tenantTx.Exec(ctx, "SELECT set_config('hp.workspace_id',$1,true)", uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if err = tenantTx.QueryRow(ctx, "SELECT count(*) FROM hp_delivery.intent_content").Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("RLS leaked rows: %d %v", visible, err)
	}
	if err = tenantTx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	fields := testFields(fixed)
	fields["workspace_id"], _ = json.Marshal(intent.WorkspaceID)
	fields["connection_id"], _ = json.Marshal(intent.ConnectionID)
	command := signed(t, fields)
	lostIntent, err := DecodeIntent(command)
	if err != nil {
		t.Fatal(err)
	}
	lost := postThroughMTLS(t, ledger, lostIntent, command, true)
	assertCounts(3)

	// Discard the original ACK, reopen both ports and retry past content expiry.
	app.Close()
	if err = content.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedPool.Close()
	reopenedContent, err := OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	defer reopenedContent.Close()
	restarted, err := NewPostgres(reopenedPool, reopenedContent)
	if err != nil {
		t.Fatal(err)
	}
	got, err := restarted.Accept(ctx, intent, func() time.Time { return fixed.Add(2 * time.Hour) })
	if err != nil || got != stable {
		t.Fatalf("unexpected repeat: %+v %v", got, err)
	}
	if retried := replayInNewProcess(t, ctx, root, command); retried != lost {
		t.Fatalf("HTTP replay changed lost receipt: %+v vs %+v", retried, lost)
	}
	wireReceipt, err := json.Marshal(lost)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("lost HTTP ACK and replay from a new process: %s", wireReceipt)
	var expiry time.Time
	if err = admin.QueryRow(ctx, "SELECT expires_at FROM hp_delivery.intent_content WHERE attempt_id=$1", stable.RecordID).Scan(&expiry); err != nil || !expiry.Equal(ref.Expires) {
		t.Fatalf("replay changed expiry: %s %v", expiry, err)
	}
	changed := intent
	changed.MessageDigest = digest([]byte("other message"))
	if _, err = restarted.Accept(ctx, changed, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed digest: %v", err)
	}
	assertCounts(3)
	t.Logf("receipt=%+v before/after replay; state=%s attempt=%d fence=%t network_started=%t expiry=%s", stable, state, number, fence, started, expiry.Format(time.RFC3339Nano))
}

func replayInNewProcess(t *testing.T, ctx context.Context, root string, command []byte) Receipt {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	path := filepath.Join(fixture, "command.json")
	if err = os.WriteFile(path, command, 0600); err != nil {
		t.Fatal(err)
	}
	replay := exec.CommandContext(ctx, executable, "-test.run=^TestPostgresReplayProcess$")
	replay.Env = append(os.Environ(), "HIT91_REPLAY_FIXTURE="+fixture, "HIT91_REPLAY_CONTENT="+root)
	if out, err := replay.CombinedOutput(); err != nil {
		t.Fatalf("restarted process: %v\n%s", err, out)
	}
	bytes, err := os.ReadFile(filepath.Join(fixture, "receipt.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt Receipt
	if err := json.Unmarshal(bytes, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestPostgresReplayProcess(t *testing.T) {
	fixture := os.Getenv("HIT91_REPLAY_FIXTURE")
	if fixture == "" {
		t.Skip("spawned by the lost HTTP ACK check")
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "command.json"))
	if err != nil {
		t.Fatal(err)
	}
	intent, err := DecodeIntent(raw)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), os.Getenv("HIT91_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	content, err := OpenDirectory(os.Getenv("HIT91_REPLAY_CONTENT"))
	if err != nil {
		t.Fatal(err)
	}
	defer content.Close()
	ledger, err := NewPostgres(pool, content)
	if err != nil {
		t.Fatal(err)
	}
	receipt := postThroughMTLS(t, ledger, intent, raw, false)
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "receipt.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func postThroughMTLS(t *testing.T, ledger Ledger, intent Intent, raw []byte, drop bool) Receipt {
	t.Helper()
	ca := newCertificateAuthority(t)
	s, err := New(authorityFunc(func(context.Context, uuid.UUID, uuid.UUID) (Snapshot, error) {
		return snapshot(intent), nil
	}), gateFunc(func(_ context.Context, _ uuid.UUID, _ int64, run func() error) error {
		return run()
	}), ledger)
	if err != nil {
		t.Fatal(err)
	}
	httpServer, err := NewHTTP(s, ca.issue(t, "localhost", x509.ExtKeyUsageServerAuth), ca.pool, "processing.internal")
	if err != nil {
		t.Fatal(err)
	}
	captured := make(chan Receipt, 1)
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !drop {
			httpServer.Handler.ServeHTTP(w, r)
			return
		}
		buffer := httptest.NewRecorder()
		httpServer.Handler.ServeHTTP(buffer, r)
		var receipt Receipt
		if buffer.Code == http.StatusAccepted && json.Unmarshal(buffer.Body.Bytes(), &receipt) == nil {
			captured <- receipt
		}
		close(captured)
		// Abort the TLS connection after commit but before the response is sent.
		panic(http.ErrAbortHandler)
	}))
	testServer.TLS = httpServer.TLSConfig
	testServer.StartTLS()
	defer testServer.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: ca.pool,
		Certificates: []tls.Certificate{ca.issue(t, "processing.internal", x509.ExtKeyUsageClientAuth)},
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, testServer.URL+route, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", uuid.NewString())
	resp, err := client.Do(req)
	if drop {
		if err == nil {
			resp.Body.Close()
			t.Fatal("lost ACK reached client")
		}
		select {
		case receipt := <-captured:
			if receipt.RecordID == uuid.Nil {
				t.Fatal("connection broken before a durable receipt existed")
			}
			return receipt
		default:
			t.Fatal("connection failed before controlled fault")
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var receipt Receipt
	if err = json.NewDecoder(resp.Body).Decode(&receipt); err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("HTTP retry status %d: %v", resp.StatusCode, err)
	}
	return receipt
}
