//go:build integration

package process_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/intake/internal/adapters/processing"
	rt "github.com/hitpipe/hitpipe/services/intake/internal/intakeruntime"
	"github.com/jackc/pgx/v5/pgxpool"
)

const signKey = "hit119-fixture-webhook-signing-key"
const readKey = "hit119-fixture-stripe-read-key"
const rawMarker = "HIT119_DISCARD_RAW_MARKER"

type pki struct {
	root    *x509.Certificate
	key     ed25519.PrivateKey
	pool    *x509.CertPool
	rootPEM []byte
}

func newPKI(t *testing.T) pki {
	t.Helper()
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	c := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	raw, e := x509.CreateCertificate(rand.Reader, c, c, public, key)
	if e != nil {
		t.Fatal(e)
	}
	root, e := x509.ParseCertificate(raw)
	if e != nil {
		t.Fatal(e)
	}
	p := x509.NewCertPool()
	p.AddCert(root)
	return pki{root, key, p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw})}
}
func (p pki) cert(t *testing.T, serial int64, name string, use x509.ExtKeyUsage) (tls.Certificate, []byte, []byte) {
	t.Helper()
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	c := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: p.root.NotBefore, NotAfter: p.root.NotAfter,
		ExtKeyUsage: []x509.ExtKeyUsage{use}, KeyUsage: x509.KeyUsageDigitalSignature}
	if ip := net.ParseIP(name); ip != nil {
		c.IPAddresses = []net.IP{ip}
	} else {
		c.DNSNames = []string{name}
	}
	raw, e := x509.CreateCertificate(rand.Reader, c, p.root, public, p.key)
	if e != nil {
		t.Fatal(e)
	}
	pk, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key},
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: raw}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk})
}
func write(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(dir, name), data, 0600); e != nil {
		t.Fatal(e)
	}
}
func listenAddress(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	return l.Addr().String()
}

type child struct {
	cmd  *exec.Cmd
	buf  bytes.Buffer
	done chan struct{}
	err  error
}

func startChild(t *testing.T, bin, config string) *child {
	t.Helper()
	c := &child{cmd: exec.Command(bin, "-config", config), done: make(chan struct{})}
	c.cmd.Stdout = &c.buf
	c.cmd.Stderr = &c.buf
	if e := c.cmd.Start(); e != nil {
		t.Fatal(e)
	}
	go func() { c.err = c.cmd.Wait(); close(c.done) }()
	t.Cleanup(func() {
		select {
		case <-c.done:
		default:
			_ = c.cmd.Process.Kill()
			<-c.done
		}
	})
	return c
}
func (c *child) ready(t *testing.T, client *http.Client, url string) {
	t.Helper()
	for until := time.Now().Add(6 * time.Second); time.Now().Before(until); time.Sleep(25 * time.Millisecond) {
		select {
		case <-c.done:
			t.Fatalf("process failed: %v / %s", c.err, c.buf.String())
		default:
		}
		resp, e := client.Get(url + "/no-health-route")
		if e == nil {
			resp.Body.Close()
			if resp.StatusCode != 404 {
				t.Fatal("unexpected extra route", resp.StatusCode)
			}
			return
		}
	}
	t.Fatal("Intake did not open its expected listener")
}
func (c *child) stop(t *testing.T) {
	t.Helper()
	if e := c.cmd.Process.Signal(syscall.SIGTERM); e != nil {
		t.Fatal(e)
	}
	select {
	case <-c.done:
	case <-time.After(6 * time.Second):
		t.Fatal("SIGTERM did not complete")
	}
	log := c.buf.String()
	if c.err != nil || !strings.Contains(log, "intake sandbox stopped") ||
		strings.Contains(log, signKey) || strings.Contains(log, readKey) || strings.Contains(log, rawMarker) {
		t.Fatalf("unsafe or unsuccessful process shutdown: %v / %q", c.err, log)
	}
	t.Logf("SIGTERM exit 0; process log: %q", log)
}
func dial(t *testing.T, p pki, certs ...tls.Certificate) *http.Client {
	t.Helper()
	trans := &http.Transport{DisableKeepAlives: true, TLSClientConfig: &tls.Config{RootCAs: p.pool, Certificates: certs, MinVersion: tls.VersionTLS13}}
	t.Cleanup(trans.CloseIdleConnections)
	return &http.Client{Transport: trans, Timeout: 6 * time.Second}
}
func call(t *testing.T, c *http.Client, url string, raw []byte, signature string, want int) []byte {
	t.Helper()
	req, e := http.NewRequest("POST", url, bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "44444444-4444-4444-8444-444444444444")
	if signature != "" {
		req.Header.Set("Stripe-Signature", signature)
	}
	resp, e := c.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(resp.Body)
	if e != nil || resp.StatusCode != want {
		t.Fatalf("got %d want %d: %s (%v)", resp.StatusCode, want, b, e)
	}
	if bytes.Contains(b, []byte(rawMarker)) || bytes.Contains(b, []byte(signKey)) || bytes.Contains(b, []byte(readKey)) {
		t.Fatal("payload or secret escaped")
	}
	return b
}

func TestIntegrationIntakeExecutable(t *testing.T) {
	bin, dsn := os.Getenv("HIT119_INTAKE_BIN"), os.Getenv("HIT86_MIGRATOR_DSN")
	if bin == "" || dsn == "" {
		t.Fatal("owned runner and compiled binary required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	db, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	pg := func(fn func(context.Context, *pgxpool.Conn)) {
		t.Helper()
		c, e := db.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer c.Release()
		_, e = c.Exec(ctx, `SELECT set_config('hp.workspace_id',$1,false)`, rt.Workspace)
		if e != nil {
			t.Fatal(e)
		}
		fn(ctx, c)
	}
	pg(func(ctx context.Context, c *pgxpool.Conn) {
		for _, q := range []string{
			"TRUNCATE hp_intake.source_connections,hp_intake.ingress_quota,hp_intake.outbox CASCADE",
			"CREATE TABLE IF NOT EXISTS public.hit119_processing_receipts(message_id uuid PRIMARY KEY,digest bytea NOT NULL)",
			"TRUNCATE public.hit119_processing_receipts",
		} {
			if _, e := c.Exec(ctx, q); e != nil {
				t.Fatal(e)
			}
		}
		if _, e := c.Exec(ctx, `INSERT INTO hp_intake.source_connections
			(workspace_id,connection_id,account_id,livemode,security_epoch,health,verified_at)
			VALUES($1,$2,$3,false,1,'READY',clock_timestamp())`, rt.Workspace, rt.Connection, rt.Account); e != nil {
			t.Fatal(e)
		}
	})
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Mkdir(filepath.Join(dir, "content"), 0700); e != nil {
		t.Fatal(e)
	}
	p := newPKI(t)
	stripePKI := newPKI(t)
	write(t, dir, "service-ca.pem", p.rootPEM)
	write(t, dir, "stripe-ca.pem", stripePKI.rootPEM)
	_, crt, key := p.cert(t, 2, "127.0.0.1", x509.ExtKeyUsageServerAuth)
	write(t, dir, "intake-server.pem", crt)
	write(t, dir, "intake-server.key", key)
	_, crt, key = p.cert(t, 3, "intake.fixture", x509.ExtKeyUsageClientAuth)
	write(t, dir, "intake-client.pem", crt)
	write(t, dir, "intake-client.key", key)
	write(t, dir, "webhook.secret", []byte(signKey))
	write(t, dir, "stripe.secret", []byte(readKey))
	procCert, _, _ := p.cert(t, 4, rt.ProcessingName, x509.ExtKeyUsageServerAuth)
	procCaller, _, _ := p.cert(t, 5, rt.ProcessingName, x509.ExtKeyUsageClientAuth)
	otherCaller, _, _ := p.cert(t, 6, "other.fixture", x509.ExtKeyUsageClientAuth)
	stripeCert, _, _ := stripePKI.cert(t, 2, "127.0.0.1", x509.ExtKeyUsageServerAuth)
	var stripeCalls atomic.Int64
	stripe := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stripeCalls.Add(1)
		if r.Method != "GET" || r.Header.Get("Authorization") != "Bearer "+readKey ||
			r.Header.Get("Stripe-Account") != rt.Account || r.Header.Get("Stripe-Version") != rt.Version {
			t.Error("invalid Stripe fixture request")
			w.WriteHeader(403)
			return
		}
		obj := stripeObject(r.URL.Path)
		if obj == nil {
			t.Error("unexpected fixture path", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Stripe-Version", rt.Version)
		_ = json.NewEncoder(w).Encode(obj)
	}))
	stripe.TLS = &tls.Config{Certificates: []tls.Certificate{stripeCert}, MinVersion: tls.VersionTLS13}
	stripe.StartTLS()
	defer stripe.Close()

	first := make(chan []byte, 1)
	second := make(chan []byte, 1)
	drop := make(chan struct{})
	defer func() {
		select {
		case <-drop:
		default:
			close(drop)
		}
	}()
	var deliveries atomic.Int64
	processingServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != processing.Path || r.TLS == nil || len(r.TLS.VerifiedChains) == 0 ||
			r.TLS.PeerCertificates[0].VerifyHostname("intake.fixture") != nil {
			t.Error("invalid Processing request")
			w.WriteHeader(403)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		var sig struct {
			ID string `json:"message_id"`
		}
		if err != nil || json.Unmarshal(body, &sig) != nil || sig.ID == "" || bytes.Contains(body, []byte(rawMarker)) {
			t.Error("invalid signal received")
			w.WriteHeader(400)
			return
		}
		hash := sha256.Sum256(body)
		tx, err := db.Begin(r.Context())
		if err != nil {
			t.Error(err)
			w.WriteHeader(503)
			return
		}
		defer tx.Rollback(context.Background())
		tag, err := tx.Exec(r.Context(), `INSERT INTO public.hit119_processing_receipts VALUES($1,$2) ON CONFLICT DO NOTHING`, sig.ID, hash[:])
		if err != nil {
			t.Error(err)
			w.WriteHeader(503)
			return
		}
		var saved []byte
		if err = tx.QueryRow(r.Context(), `SELECT digest FROM public.hit119_processing_receipts WHERE message_id=$1`, sig.ID).Scan(&saved); err != nil ||
			!bytes.Equal(saved, hash[:]) || tx.Commit(r.Context()) != nil {
			t.Error("Processing fixture could not establish durable receipt")
			w.WriteHeader(503)
			return
		}
		switch deliveries.Add(1) {
		case 1:
			if tag.RowsAffected() != 1 {
				t.Error("first Processing receipt was not new")
			}
			first <- body
			select {
			case <-r.Context().Done():
			case <-drop:
			}
			if c, _, e := http.NewResponseController(w).Hijack(); e == nil {
				c.Close()
			}
			return
		case 2:
			if tag.RowsAffected() != 0 {
				t.Error("duplicate Processing admission")
			}
			second <- body
		default:
			t.Error("unexpected duplicate delivery")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]string{"disposition": "DUPLICATE", "message_id": sig.ID})
	}))
	processingServer.TLS = &tls.Config{Certificates: []tls.Certificate{procCert}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: p.pool, MinVersion: tls.VersionTLS13}
	processingServer.StartTLS()
	defer processingServer.Close()

	addr := listenAddress(t)
	port := int(db.Config().ConnConfig.Port)
	c := rt.Config{Profile: "sandbox", Listen: addr, DatabasePort: port, StripeOrigin: stripe.URL,
		ProcessingAddress: processingServer.Listener.Addr().String(), Batch: 1, PollMillis: 100}
	raw, _ := json.Marshal(c)
	write(t, dir, "sandbox.json", raw)
	configPath := filepath.Join(dir, "sandbox.json")
	for _, bad := range []string{`{}`, `{"profile":"ordinary"}`, `{"profile":"sandbox"}`,
		strings.Replace(string(raw), addr, "0.0.0.0:8443", 1),
		strings.Replace(string(raw), addr, c.ProcessingAddress, 1),
		string(raw[:len(raw)-1]) + `,"profile":"sandbox"}`,
		string(raw[:len(raw)-1]) + `,"password":"` + readKey + `"}`} {
		write(t, dir, "invalid.json", []byte(bad))
		x, e := exec.CommandContext(ctx, bin, "-config", filepath.Join(dir, "invalid.json")).CombinedOutput()
		if e == nil || bytes.Contains(x, []byte(readKey)) || bytes.Contains(x, []byte("listening")) {
			t.Fatalf("invalid launch: %q / %v", x, e)
		}
	}
	t.Log("binary rejected closed ordinary mode, missing fields, non-loopback, conflict, duplicate and unknown secret field")

	public, private, foreign := dial(t, p), dial(t, p, procCaller), dial(t, p, otherCaller)
	base := "https://" + addr
	one := startChild(t, bin, configPath)
	one.ready(t, public, base)
	created := time.Now().Unix()
	payload := []byte(fmt.Sprintf(`{"id":"evt_synthetic_119","object":"event","account":%q,"api_version":%q,"type":"checkout.session.completed","created":%d,"livemode":false,"data":{"object":{"id":%q,"object":"checkout.session","created":%d,"livemode":false,"metadata":{"discard":%q}}}}`, rt.Account, rt.Version, created, rt.Session, created-10, rawMarker))
	h := hmac.New(sha256.New, []byte(signKey))
	fmt.Fprintf(h, "%d.", created)
	h.Write(payload)
	sig := fmt.Sprintf("t=%d,v1=%x", created, h.Sum(nil))
	hook := base + "/webhooks/stripe/" + rt.Endpoint
	call(t, public, hook, payload, "v1=broken", 401)
	if b := call(t, public, hook, payload, sig, 204); len(b) != 0 {
		t.Fatal("webhook ACK body")
	}
	var previous []byte
	select {
	case previous = <-first:
	case <-time.After(6 * time.Second):
		t.Fatal("sender did not reach Processing")
	}
	// A waiting Processing response must not block independent Intake ingress.
	call(t, public, hook, payload, sig, 204)
	factsURL := base + "/internal/v1/source-facts/read"
	factsReq := func() []byte {
		x, _ := json.Marshal(map[string]any{"workspace_id": rt.Workspace, "connection_id": rt.Connection, "case_id": rt.Case,
			"session_id": rt.Session, "expected_epoch": 1, "deadline_at": time.Now().Add(4 * time.Second).UTC()})
		return x
	}
	call(t, public, factsURL, factsReq(), "", 401)
	call(t, foreign, factsURL, factsReq(), "", 401)
	b := call(t, private, factsURL, factsReq(), "", 200)
	var facts struct {
		Complete bool `json:"complete"`
	}
	if json.Unmarshal(b, &facts) != nil || !facts.Complete || stripeCalls.Load() < 6 {
		t.Fatal("source facts were not freshly read")
	}
	state := func() (string, string, string, int) {
		var s, id, ref string
		var n int
		pg(func(ctx context.Context, c *pgxpool.Conn) {
			if e := c.QueryRow(ctx, `SELECT state,message_id::text,payload_ref::text,delivery_count FROM hp_intake.outbox`).Scan(&s, &id, &ref, &n); e != nil {
				t.Fatal(e)
			}
		})
		return s, id, ref, n
	}
	s, id, ref, n := state()
	if s != "CLAIMED" || n != 1 {
		t.Fatal("missing first lease", s, n)
	}
	if saved, e := os.ReadFile(filepath.Join(dir, "content", ref)); e != nil || !bytes.Equal(saved, previous) {
		t.Fatal("wire bytes did not match the exact durable Intake object")
	}
	// Force a true process interruption after the controlled receiver commit
	// but before the sender can receive its HTTP receipt.
	one.stop(t)
	close(drop)
	s, afterID, afterRef, n := state()
	if s != "CLAIMED" || afterID != id || afterRef != ref || n != 1 {
		t.Fatal("shutdown fabricated a receipt or changed identity", s, n)
	}
	two := startChild(t, bin, configPath)
	two.ready(t, public, base)
	select {
	case retry := <-second:
		if !bytes.Equal(previous, retry) {
			t.Fatal("restart changed original bytes")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("lease was not recovered")
	}
	for until := time.Now().Add(4 * time.Second); ; time.Sleep(30 * time.Millisecond) {
		s, newID, newRef, attempts := state()
		if s == "ACKED" && newID == id && newRef == ref && attempts == 2 {
			break
		}
		if time.Now().After(until) {
			t.Fatal("exact receipt not saved on restart", s, attempts)
		}
	}
	pg(func(ctx context.Context, c *pgxpool.Conn) {
		var sigs, facts, boxes, received int
		e := c.QueryRow(ctx, `SELECT (SELECT count(*) FROM hp_intake.signals),(SELECT count(*) FROM hp_intake.source_facts),
			(SELECT count(*) FROM hp_intake.outbox),(SELECT count(*) FROM public.hit119_processing_receipts)`).Scan(&sigs, &facts, &boxes, &received)
		if e != nil || sigs != 1 || facts != 1 || boxes != 1 || received != 1 {
			t.Fatal("incorrect persisted counts", sigs, facts, boxes, received, e)
		}
	})
	time.Sleep(275 * time.Millisecond)
	if deliveries.Load() != 2 {
		t.Fatal("empty poll caused network delivery")
	}
	two.stop(t)
	t.Log("one signed webhook, mTLS-only source facts, SQL/file readback, interrupted receipt and restarted delivery; original ID and bytes preserved")
}

func stripeObject(path string) map[string]any {
	switch path {
	case "/v1/account":
		return map[string]any{"id": "acct_synthetic", "object": "account"}
	case "/v1/checkout/sessions/cs_synthetic":
		return map[string]any{"id": "cs_synthetic", "object": "checkout.session", "livemode": false, "mode": "payment", "payment_status": "paid", "currency": "usd", "amount_total": int64(1200), "created": int64(1700000000), "payment_intent": "pi_synthetic", "customer_email": "RAW_CUSTOMER_MARKER"}
	case "/v1/checkout/sessions/cs_synthetic/line_items":
		return list(map[string]any{"id": "li_synthetic", "object": "item", "currency": "usd", "quantity": int64(1), "price": map[string]any{"id": "price_synthetic", "object": "price"}})
	case "/v1/prices/price_synthetic":
		return map[string]any{"id": "price_synthetic", "object": "price", "livemode": false, "currency": "usd", "type": "one_time", "billing_scheme": "per_unit", "custom_unit_amount": nil, "recurring": nil, "transform_quantity": nil, "unit_amount": int64(1200)}
	case "/v1/payment_intents/pi_synthetic":
		return map[string]any{"id": "pi_synthetic", "object": "payment_intent", "livemode": false, "status": "succeeded", "currency": "usd", "amount_received": int64(1200), "latest_charge": "ch_synthetic"}
	case "/v1/charges/ch_synthetic":
		return map[string]any{"id": "ch_synthetic", "object": "charge", "livemode": false, "currency": "usd", "payment_intent": "pi_synthetic", "paid": true, "status": "succeeded", "captured": true, "amount_captured": int64(1200), "amount_refunded": int64(0), "disputed": false}
	case "/v1/refunds", "/v1/disputes":
		return list()
	}
	return nil
}
func list(items ...map[string]any) map[string]any {
	data := make([]map[string]any, len(items))
	copy(data, items)
	return map[string]any{"object": "list", "data": data, "has_more": false}
}
