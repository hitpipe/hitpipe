//go:build integration

package receipt

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/contentstore"
	hook "github.com/hitpipe/hitpipe/services/intake/internal/adapters/http/stripewebhook"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/receivesignal"
)

const webhookPath = "/webhooks/stripe/"

type testInstallations map[string]hook.Installation

func (m testInstallations) Lookup(_ context.Context, id string) (hook.Installation, error) {
	i, ok := m[id]
	if !ok {
		return i, hook.ErrNotFound
	}
	return i, nil
}

type testStart func(context.Context, hook.Installation, func() error) error

func (g testStart) Start(ctx context.Context, i hook.Installation, run func() error) error {
	return g(ctx, i, run)
}

// This is test authority, independent of headers and event bytes.
func testAuthority(m testInstallations) testStart {
	return func(ctx context.Context, i hook.Installation, run func() error) error {
		a, ok := m[i.EndpointID]
		if !ok || !a.Ready || ctx.Err() != nil {
			return hook.ErrDenied
		}
		if a.Revision != i.Revision || a.Connection.SecurityEpoch != i.Connection.SecurityEpoch {
			return hook.ErrStale
		}
		b, c := a.Connection.Binding, i.Connection.Binding
		if b.Livemode == nil || c.Livemode == nil || *b.Livemode != *c.Livemode {
			return hook.ErrDenied
		}
		b.Livemode, c.Livemode = nil, nil
		if b != c {
			return hook.ErrDenied
		}
		return run()
	}
}

func webInstall(endpoint string, c receivesignal.Connection) hook.Installation {
	return hook.Installation{EndpointID: endpoint, Connection: c, Revision: 1, Ready: true}
}

type webFixture struct {
	url    string
	client *http.Client
	tls    *tls.Config
}

func serveWebhook(t *testing.T, timeout time.Duration, installs testInstallations, gate testStart, consumers map[string]*receivesignal.Consumer) webFixture {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := hook.New(&tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, timeout, installs, gate, consumers)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	tlsClient := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	transport := &http.Transport{TLSClientConfig: tlsClient, DisableKeepAlives: true}
	t.Cleanup(func() {
		transport.CloseIdleConnections()
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		if err := srv.Close(ctx); err != nil {
			t.Error(err)
		}
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	})
	return webFixture{"https://" + listener.Addr().String(), &http.Client{Transport: transport, Timeout: 12 * time.Second}, tlsClient}
}

func (f *fixtures) serve(t *testing.T, consumer *receivesignal.Consumer) webFixture {
	t.Helper()
	m := testInstallations{"stripe": webInstall("stripe", f.connection)}
	return serveWebhook(t, 5*time.Second, m, testAuthority(m), map[string]*receivesignal.Consumer{ws: consumer})
}

func (s webFixture) send(endpoint, signature string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodPost, s.url+webhookPath+endpoint, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", otherWS)
	req.Header.Set("X-Stripe-Account", "acct_untrusted")
	if signature != "" {
		req.Header.Set("Stripe-Signature", signature)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, err
}

func (s webFixture) expect(t *testing.T, endpoint string, d receivesignal.Delivery, status int) {
	t.Helper()
	s.expectBody(t, endpoint, d.Signature, bytes.NewReader(d.RawBody), status)
}

func (s webFixture) expectBody(t *testing.T, endpoint, signature string, body io.Reader, want int) {
	t.Helper()
	status, raw, err := s.send(endpoint, signature, body)
	if err != nil || status != want {
		t.Fatalf("HTTP %d, want %d: %s (%v)", status, want, raw, err)
	}
	if want == 204 {
		if len(raw) != 0 {
			t.Fatal("ACK has a body")
		}
		return
	}
	var failure map[string]string
	if err := json.Unmarshal(raw, &failure); err != nil || len(failure) != 3 ||
		len(failure["request_id"]) != 36 || failure["code"] == "" || failure["message"] == "" ||
		bytes.Contains(raw, []byte("NEVER_STORE")) || bytes.Contains(raw, []byte(fixtureSecret)) {
		t.Fatal("unsafe or invalid error envelope")
	}
}

func TestIntegrationWebhookCommitBeforeACKAndReplay(t *testing.T) {
	f := newFixtures(t)
	commit, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	f.repo.fault = func(stage string) error {
		if stage == "before_commit" {
			close(commit)
			<-release
		}
		return nil
	}
	s := f.serve(t, f.consumer)
	d := f.delivery("evt_http_commit", "checkout.session.completed")
	type reply struct {
		status int
		body   []byte
		err    error
	}
	result := make(chan reply, 1)
	go func() {
		code, body, err := s.send("stripe", d.Signature, bytes.NewReader(d.RawBody))
		result <- reply{code, body, err}
	}()
	select {
	case <-commit:
	case <-time.After(4 * time.Second):
		t.Fatal("no real SQL commit attempt")
	}
	if f.scalar("SELECT count(*) FROM hp_intake.signals") != 0 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 0 {
		t.Fatal("uncommitted rows visible")
	}
	entries, err := os.ReadDir(f.path)
	if err != nil || len(entries) != 1 {
		t.Fatal("content not durable before SQL", err)
	}
	select {
	case <-result:
		t.Fatal("HTTP result before commit")
	default:
	}
	once.Do(func() { close(release) })
	got := <-result
	if got.status != 204 || len(got.body) != 0 || got.err != nil {
		t.Fatalf("bad ACK: %+v", got)
	}
	rows := f.sql(`SELECT s.signal_id::text,s.first_received_at,s.expires_at,o.message_id::text,row_to_json(s)::text||row_to_json(o)::text FROM hp_intake.signals s JOIN hp_intake.outbox o ON o.message_id=s.signal_id`)
	var id, msg, sqlText string
	var expiry, first time.Time
	if !rows.Next() || rows.Scan(&id, &first, &expiry, &msg, &sqlText) != nil || id != msg ||
		expiry.Sub(first) != 72*time.Hour || strings.Contains(sqlText, "NEVER_STORE") {
		t.Fatal("inconsistent persisted receipt")
	}
	rows.Close()
	ref := f.outboxRef(id)
	saved, err := f.files.Read(ref)
	if err != nil || bytes.Contains(saved, []byte("NEVER_STORE")) || bytes.Contains(saved, []byte(fixtureSecret)) ||
		!bytes.Contains(saved, []byte(id)) {
		t.Fatal("inconsistent or unsafe file", err)
	}

	f.clock.unix.Add(301)
	s.expect(t, "stripe", d, 401)
	d = f.sign(d)
	before := f.secrets.calls.Load()
	s.expect(t, "stripe", d, 204)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			code, body, err := s.send("stripe", d.Signature, bytes.NewReader(d.RawBody))
			if code != 204 || len(body) != 0 || err != nil {
				t.Errorf("duplicate: %d %v", code, err)
			}
		})
	}
	wg.Wait()
	if f.secrets.calls.Load()-before < 7 || f.scalar("SELECT count(*) FROM hp_intake.signals") != 1 ||
		f.scalar("SELECT count(*) FROM hp_intake.outbox") != 1 || f.outboxRef(id) != ref {
		t.Fatal("retry skipped authentication or changed identity/expiry")
	}
	clash := f.sign(receivesignal.Delivery{RawBody: bytes.Replace(d.RawBody, []byte("cs_synthetic"), []byte("cs_conflict"), 1)})
	s.expect(t, "stripe", clash, 409)
}

func TestIntegrationWebhookRejectsBeforeStorage(t *testing.T) {
	f := newFixtures(t)
	foreign := f.connection
	foreign.Binding.WorkspaceID = otherWS
	revoked := webInstall("revoked", f.connection)
	revoked.Ready = false
	lookup := testInstallations{
		"stripe":   webInstall("stripe", f.connection),
		"foreign":  webInstall("foreign", foreign),
		"revoked":  revoked,
		"changed":  webInstall("changed", f.connection),
		"unknown":  webInstall("unknown", f.connection),
		"silent":   webInstall("silent", f.connection),
		"mismatch": webInstall("stripe", f.connection),
	}
	authority := testInstallations{"stripe": lookup["stripe"], "changed": webInstall("changed", f.connection)}
	current := authority["changed"]
	current.Revision++
	authority["changed"] = current
	guard := testStart(func(ctx context.Context, i hook.Installation, run func() error) error {
		if i.EndpointID == "silent" {
			return nil
		}
		return testAuthority(authority)(ctx, i, run)
	})
	s := serveWebhook(t, 2*time.Second, lookup, guard, map[string]*receivesignal.Consumer{ws: f.consumer})
	d := f.delivery("evt_http_denied", "checkout.session.completed")
	for _, tc := range []struct {
		endpoint string
		status   int
	}{
		{"missing", 404}, {"foreign", 403}, {"revoked", 403}, {"changed", 412}, {"unknown", 403}, {"silent", 503}, {"mismatch", 403},
		{strings.Repeat("a", 256), 400},
	} {
		s.expect(t, tc.endpoint, d, tc.status)
	}
	for _, tc := range []struct {
		name     string
		delivery receivesignal.Delivery
		status   int
	}{
		{"missing signature", receivesignal.Delivery{RawBody: d.RawBody}, 401},
		{"malformed signature", receivesignal.Delivery{RawBody: d.RawBody, Signature: "broken"}, 401},
		{"tampered raw bytes", receivesignal.Delivery{RawBody: append(bytes.Clone(d.RawBody), ' '), Signature: d.Signature}, 401},
		{"conflicting timestamp", receivesignal.Delivery{RawBody: d.RawBody, Signature: d.Signature + ",t=1800000001"}, 401},
		{"bad JSON before authentication", receivesignal.Delivery{RawBody: []byte("{"), Signature: d.Signature}, 401},
		{"signed bad JSON", f.sign(receivesignal.Delivery{RawBody: []byte("{")}), 400},
		{"untrusted account", f.sign(receivesignal.Delivery{RawBody: bytes.Replace(d.RawBody, []byte("acct_synthetic"), []byte("acct_wrong"), 1)}), 403},
		{"wrong mode", f.sign(receivesignal.Delivery{RawBody: bytes.Replace(d.RawBody, []byte("false"), []byte("true"), 1)}), 403},
		{"unsupported api", f.sign(receivesignal.Delivery{RawBody: bytes.Replace(d.RawBody, []byte("synthetic-version"), []byte("unsupported"), 1)}), 400},
		{"unsupported event", f.delivery("evt_unsupported", "payment_intent.succeeded"), 400},
	} {
		t.Run(tc.name, func(t *testing.T) { s.expect(t, "stripe", tc.delivery, tc.status) })
	}
	over := bytes.Repeat([]byte{'x'}, 262145)
	s.expectBody(t, "stripe", d.Signature, bytes.NewReader(over), 413)
	s.expectBody(t, "stripe", d.Signature, struct{ io.Reader }{bytes.NewReader(over)}, 413)
	s.expectBody(t, "stripe", strings.Repeat("x", 2049), bytes.NewReader(d.RawBody), 413)
	atLimit := append(bytes.Clone(d.RawBody), bytes.Repeat([]byte{' '}, 262144-len(d.RawBody))...)
	s.expect(t, "stripe", receivesignal.Delivery{RawBody: atLimit, Signature: d.Signature}, 401)

	for _, deadline := range []bool{false, true} {
		conn, err := tls.Dial("tcp", strings.TrimPrefix(s.url, "https://"), s.tls)
		if err != nil {
			t.Fatal(err)
		}
		conn.SetDeadline(time.Now().Add(4 * time.Second))
		_, err = fmt.Fprintf(conn, "POST %sstripe HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: 100\r\nStripe-Signature: %s\r\n\r\n{", webhookPath, d.Signature)
		if err != nil {
			conn.Close()
			t.Fatal(err)
		}
		if deadline {
			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil || resp.StatusCode != 503 {
				conn.Close()
				t.Fatalf("body deadline: %v %v", resp, err)
			}
			resp.Body.Close()
		}
		conn.Close()
	}
	if f.scalar("SELECT count(*) FROM hp_intake.signals") != 0 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 0 {
		t.Fatal("rejection reached storage")
	}
	files, err := os.ReadDir(f.path)
	if err != nil || len(files) != 0 {
		t.Fatal("rejection created content", err)
	}
}

func TestIntegrationWebhookQuotaRefusal(t *testing.T) {
	for _, kind := range []string{"count", "bytes"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixtures(t)
			size, count := 1, 999
			if kind == "bytes" {
				size, count = 16777216-900, 1
			}
			f.exec(`INSERT INTO hp_intake.signals(workspace_id,signal_id,connection_id,event_id,event_type,session_id,account_id,livemode,source_created_at,first_received_at,expires_at,serialized_bytes,state,reason_code) SELECT $1, md5(i::text)::uuid,$2,'evt_seed_'||i,'checkout.session.completed','cs_seed','acct_synthetic',false,$3,$3,$4,$5,'HELD','SYNTHETIC_BOUNDARY' FROM generate_series(1,$6) i`, ws, connID, time.Unix(1800000000, 0), time.Unix(1800003600, 0), size, count)
			s := f.serve(t, f.consumer)
			d := f.delivery("evt_quota_http", "checkout.session.completed")
			s.expect(t, "stripe", d, 204)
			s.expect(t, "stripe", f.delivery("evt_over_limit", "checkout.session.completed"), 429)
			s.expect(t, "stripe", d, 204)
			if f.scalar("SELECT count(*) FROM hp_intake.signals") != count+1 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 1 {
				t.Fatal("quota created partial or duplicate rows")
			}
		})
	}
}

func TestIntegrationWebhookStorageFailureAndRestart(t *testing.T) {
	for _, stage := range []string{"file", "before_commit", "after_commit", "file_after_commit", "database", "expired"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixtures(t)
			consumer := f.consumer
			d := f.delivery("evt_http_fault", "checkout.session.completed")
			expected := 0
			switch stage {
			case "file":
				var err error
				consumer, err = receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, f.repo, observingContent{Content: f.files, fail: true})
				if err != nil {
					t.Fatal(err)
				}
			case "before_commit", "after_commit":
				f.repo.fault = func(actual string) error {
					if actual == stage {
						return errors.New("NEVER_STORE")
					}
					return nil
				}
				if stage == "after_commit" {
					expected = 1
				}
			case "file_after_commit", "expired":
				accepted := f.receive("evt_http_fault")
				expected = 1
				if stage == "expired" {
					f.clock.unix.Add(72 * 3600)
					d = f.sign(d)
				} else if err := os.Remove(filepath.Join(f.path, f.outboxRef(accepted.MessageID).ID)); err != nil {
					t.Fatal(err)
				}
			case "database":
				f.app.Close()
			}
			s := f.serve(t, consumer)
			s.expect(t, "stripe", d, 503)
			if f.scalar("SELECT count(*) FROM hp_intake.signals") != expected || f.scalar("SELECT count(*) FROM hp_intake.outbox") != expected {
				t.Fatal("storage failure created an invalid durable result")
			}
			if stage == "database" || stage == "file_after_commit" || stage == "expired" {
				return
			}
			repo, err := New(f.app)
			if err != nil {
				t.Fatal(err)
			}
			restarted, err := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, repo, f.files)
			if err != nil {
				t.Fatal(err)
			}
			f.serve(t, restarted).expect(t, "stripe", d, 204)
			if f.scalar("SELECT count(*) FROM hp_intake.signals") != 1 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 1 {
				t.Fatal("restart produced a partial or duplicate result")
			}
		})
	}
}

func TestIntegrationWebhookLostHTTPResponse(t *testing.T) {
	f := newFixtures(t)
	m := testInstallations{"stripe": webInstall("stripe", f.connection)}
	saved := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	g := testStart(func(ctx context.Context, i hook.Installation, run func() error) error {
		err := testAuthority(m)(ctx, i, run)
		close(saved)
		<-release
		return err
	})
	s := serveWebhook(t, 5*time.Second, m, g, map[string]*receivesignal.Consumer{ws: f.consumer})
	d := f.delivery("evt_lost_http", "checkout.session.completed")
	conn, err := tls.Dial("tcp", strings.TrimPrefix(s.url, "https://"), s.tls)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintf(conn, "POST %sstripe HTTP/1.1\r\nHost: localhost\r\nContent-Type: application/json\r\nContent-Length: %d\r\nStripe-Signature: %s\r\n\r\n%s", webhookPath, len(d.RawBody), d.Signature, d.RawBody); err != nil {
		t.Fatal(err)
	}
	select {
	case <-saved:
	case <-time.After(4 * time.Second):
		t.Fatal("no committed result before disconnect")
	}
	r := f.sql("SELECT signal_id::text FROM hp_intake.signals WHERE event_id='evt_lost_http'")
	var id string
	if !r.Next() || r.Scan(&id) != nil {
		r.Close()
		t.Fatal("no actual committed receipt")
	}
	r.Close()
	ref := f.outboxRef(id)
	if _, err := f.files.Read(ref); err != nil {
		t.Fatal(err)
	}
	// The HTTP server cannot write until the gate callback returns. Here the
	// peer connection is gone while the successful durable receipt still exists.
	conn.Close()
	once.Do(func() { close(release) })
	repo, err := New(f.app)
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := receivesignal.New(ws, f.secrets, f.clock, 72*time.Hour, repo, f.files)
	if err != nil {
		t.Fatal(err)
	}
	f.serve(t, restarted).expect(t, "stripe", d, 204)
	if f.scalar("SELECT count(*) FROM hp_intake.signals") != 1 || f.scalar("SELECT count(*) FROM hp_intake.outbox") != 1 || f.outboxRef(id) != ref {
		t.Fatal("retry changed committed result")
	}
}

func TestIntegrationWebhookScopes(t *testing.T) {
	f := newFixtures(t)
	ctx := context.Background()
	second := f.connection
	second.Binding.ConnectionID = "33333333-3333-4333-8333-333333333333"
	foreign := f.connection
	foreign.Binding.WorkspaceID = otherWS
	f.exec(`INSERT INTO hp_intake.source_connections(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at) VALUES($1,$2,'acct_synthetic',false,1,'READY',$3)`, ws, second.Binding.ConnectionID, time.Unix(1799999900, 0))
	tx, err := f.migrator.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, otherWS); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hp_intake.source_connections(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at) VALUES($1,$2,'acct_synthetic',false,1,'READY',$3)`, otherWS, connID, time.Unix(1799999900, 0)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := contentstore.Open(path, func() time.Time { n, _ := f.clock.Now(ctx); return n })
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	other, err := receivesignal.New(otherWS, f.secrets, f.clock, 72*time.Hour, f.repo, store)
	if err != nil {
		t.Fatal(err)
	}
	m := testInstallations{
		"stripe":  webInstall("stripe", f.connection),
		"second":  webInstall("second", second),
		"foreign": webInstall("foreign", foreign),
	}
	s := serveWebhook(t, 5*time.Second, m, testAuthority(m), map[string]*receivesignal.Consumer{ws: f.consumer, otherWS: other})
	d := f.delivery("evt_scoped_http", "checkout.session.completed")
	for _, endpoint := range []string{"stripe", "second", "foreign"} {
		s.expect(t, endpoint, d, 204)
	}
	if f.scalar("SELECT count(DISTINCT signal_id) FROM hp_intake.signals") != 2 ||
		f.scalar("SELECT count(DISTINCT connection_id) FROM hp_intake.signals") != 2 {
		t.Fatal("connection identities collapsed")
	}
	appTx, err := f.app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer appTx.Rollback(ctx)
	if _, err := appTx.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,true)`, otherWS); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := appTx.QueryRow(ctx, "SELECT count(*) FROM hp_intake.signals WHERE workspace_id=$1 AND event_id='evt_scoped_http'", otherWS).Scan(&n); err != nil || n != 1 {
		t.Fatal("missing independently scoped receipt", err)
	}
	for dir, want := range map[string]int{f.path: 2, path: 1} {
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != want {
			t.Fatal("wrong scoped content", err)
		}
	}
}

func TestIntegrationWebhookExactLimits(t *testing.T) {
	f := newFixtures(t)
	s := f.serve(t, f.consumer)
	d := f.delivery("evt_http_limit", "checkout.session.completed")
	d.RawBody = append(d.RawBody, bytes.Repeat([]byte{' '}, 262144-len(d.RawBody))...)
	d = f.sign(d)
	f.clock.unix.Add(300)
	s.expect(t, "stripe", d, 204)
	f.clock.unix.Add(1)
	s.expect(t, "stripe", d, 401)
	if f.scalar("SELECT count(*) FROM hp_intake.signals") != 1 {
		t.Fatal("boundary delivery did not preserve the original receipt")
	}
}
