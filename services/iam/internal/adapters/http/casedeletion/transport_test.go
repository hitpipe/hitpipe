package casedeletion

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/casedeletion"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const (
	workspaceA = "13faf8b1-1c15-44d3-b09e-5d0e571bf101"
	workspaceB = "13faf8b1-1c15-44d3-b09e-5d0e571bf102"
	deletionID = "13faf8b1-1c15-44d3-b09e-5d0e571bf103"
	processing = "processing.hit154.test"
)

func sampleReceipt() app.Receipt {
	return app.Receipt{
		Environment: "test", WorkspaceID: workspaceA, DeletionID: deletionID,
		ScopeDigest: "sha256:" + strings.Repeat("a", 64), Generation: 1, Revision: 2,
		AppliedAt:      time.Date(2026, 1, 1, 1, 1, 0, 0, time.UTC),
		ApplicationRef: "sha256:" + strings.Repeat("b", 64),
	}
}

type recordingBackend struct {
	sync.Mutex
	snapshots, receipts int
	lastPeer            string
	receipt             app.Receipt
	snapshot            app.Snapshot
	snapshotErr         error
	receiptErr          error
}

func (b *recordingBackend) Snapshot(context.Context, string, uint64) (app.Snapshot, error) {
	b.Lock()
	defer b.Unlock()
	b.snapshots++
	return b.snapshot, b.snapshotErr
}

func (b *recordingBackend) AcceptReceipt(_ context.Context, peer string, receipt app.Receipt) error {
	b.Lock()
	defer b.Unlock()
	b.receipts++
	b.lastPeer, b.receipt = peer, receipt
	return b.receiptErr
}

type testCA struct {
	cert *x509.Certificate
	key  ed25519.PrivateKey
}

func makeCA(t *testing.T, name string) testCA {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	spec := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, spec, spec, pub, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return testCA{cert, key}
}

func (ca testCA) roots() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(ca.cert)
	return pool
}

func (ca testCA) issue(t *testing.T, dns []string, ips []net.IP, usage x509.ExtKeyUsage, expired bool) tls.Certificate {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	end := now.Add(time.Hour)
	if expired {
		end = now.Add(-time.Minute)
	}
	spec := &x509.Certificate{
		SerialNumber: big.NewInt(2), DNSNames: dns, IPAddresses: ips,
		NotBefore: now.Add(-time.Hour), NotAfter: end, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	der, err := x509.CreateCertificate(rand.Reader, spec, ca.cert, pub, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.cert.Raw}, PrivateKey: key, Leaf: leaf}
}

type testTLS struct {
	publisherCA, processingCA testCA
	publisher, processing     tls.Certificate
	inventory                 []Peer
}

func provision(t *testing.T) testTLS {
	t.Helper()
	publisherCA, processingCA := makeCA(t, "publisher test CA"), makeCA(t, "Processing test CA")
	publisher := publisherCA.issue(t, []string{"issuer.hit154.test"}, []net.IP{net.ParseIP("127.0.0.1")}, x509.ExtKeyUsageServerAuth, false)
	proc := processingCA.issue(t, []string{processing}, nil, x509.ExtKeyUsageClientAuth, false)
	peer := Peer{ID: "processing-test", Transport: runtimeaccess.Peer{
		DNSName: processing, CertificateFingerprint: fingerprint(proc.Leaf),
	}, Workspaces: []string{workspaceA}}
	return testTLS{publisherCA, processingCA, publisher, proc, []Peer{peer}}
}

func (cfg testTLS) serve(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(h)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	var err error
	server.TLS, err = TLSConfig(cfg.publisher, cfg.processingCA.roots(), cfg.inventory)
	if err != nil {
		t.Fatal(err)
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func (cfg testTLS) raw(t *testing.T, certificate *tls.Certificate, version uint16) *http.Client {
	t.Helper()
	tlsConfig := &tls.Config{RootCAs: cfg.publisherCA.roots(), MinVersion: version, MaxVersion: version}
	if certificate != nil {
		tlsConfig.Certificates = []tls.Certificate{*certificate}
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, Timeout: time.Second}
}

func requestStatus(t *testing.T, c *http.Client, url, method, body string, expected int) {
	t.Helper()
	var source io.Reader
	if body != "" {
		source = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, url, source)
	if err != nil {
		t.Fatal(err)
	}
	if source != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Peer-ID", "untrusted")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("status=%d, want %d", resp.StatusCode, expected)
	}
	if expected != http.StatusOK {
		out, err := io.ReadAll(resp.Body)
		if err != nil || len(out) != 0 {
			t.Fatal("unexpected response diagnostics/body")
		}
	}
}

func TestProcessingTLSAndWorkspaceScope(t *testing.T) {
	cfg := provision(t)
	b := &recordingBackend{snapshotErr: app.ErrUnavailable}
	handler, err := New(b, cfg.inventory)
	if err != nil {
		t.Fatal(err)
	}
	server := cfg.serve(t, handler)
	endpoint := server.URL + SnapshotPath + "?workspace_id=" + workspaceA + "&generation=1"
	good := cfg.raw(t, &cfg.processing, tls.VersionTLS13)
	requestStatus(t, good, endpoint, http.MethodGet, "", http.StatusServiceUnavailable)
	requestStatus(t, good, strings.Replace(endpoint, workspaceA, workspaceB, 1), http.MethodGet, "", http.StatusForbidden)
	requestStatus(t, good, endpoint+"&workspace_id="+workspaceA, http.MethodGet, "", http.StatusBadRequest)
	requestStatus(t, good, endpoint+"&generation=1", http.MethodGet, "", http.StatusBadRequest)
	requestStatus(t, good, endpoint+"&want=private", http.MethodGet, "", http.StatusBadRequest)
	requestStatus(t, good, endpoint+"&", http.MethodGet, "unexpected", http.StatusBadRequest)
	requestStatus(t, good, strings.Replace(endpoint, "=1", "=01", 1), http.MethodGet, "", http.StatusBadRequest)
	requestStatus(t, good, endpoint, http.MethodPost, "", http.StatusMethodNotAllowed)
	requestStatus(t, good, server.URL+"/internal/maintenance/v1/case-deletion/cancel", http.MethodPost, "{}", http.StatusNotFound)
	requestStatus(t, good, server.URL+"/internal/maintenance/v1/case-deletion/confirm", http.MethodPost, "{}", http.StatusNotFound)
	requestStatus(t, good, server.URL+ReceiptPath, http.MethodGet, "", http.StatusMethodNotAllowed)

	untrusted := makeCA(t, "unauthorized CA").issue(t, []string{processing}, nil, x509.ExtKeyUsageClientAuth, false)
	cloneSAN := cfg.processingCA.issue(t, []string{processing}, nil, x509.ExtKeyUsageClientAuth, false)
	ambiguousSAN := cfg.processingCA.issue(t, []string{processing, "other.hit154.test"}, nil, x509.ExtKeyUsageClientAuth, false)
	expired := cfg.processingCA.issue(t, []string{processing}, nil, x509.ExtKeyUsageClientAuth, true)
	for name, c := range map[string]*tls.Certificate{
		"absent": nil, "different issuer": &untrusted, "different certificate same SAN": &cloneSAN,
		"ambiguous SAN": &ambiguousSAN, "expired leaf": &expired,
	} {
		t.Run(name, func(t *testing.T) {
			client := cfg.raw(t, c, tls.VersionTLS13)
			if resp, err := client.Get(endpoint); err == nil {
				resp.Body.Close()
				t.Fatal("unexpected authenticated connection")
			}
		})
	}
	if response, err := cfg.raw(t, &cfg.processing, tls.VersionTLS12).Get(endpoint); err == nil {
		response.Body.Close()
		t.Fatal("legacy TLS reached maintenance")
	}
	b.Lock()
	defer b.Unlock()
	if b.snapshots != 1 || b.receipts != 0 {
		t.Fatalf("unauthorized calls reached backend: %d reads, %d receipts", b.snapshots, b.receipts)
	}
}

func TestReceiptChannelUsesOnlyThePinnedPeerAndExactWire(t *testing.T) {
	cfg := provision(t)
	b := &recordingBackend{}
	handler, err := New(b, cfg.inventory)
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.serve(t, handler)
	c := cfg.raw(t, &cfg.processing, tls.VersionTLS13)
	endpoint := srv.URL + ReceiptPath
	receipt := sampleReceipt()
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	valid := string(raw)
	requestStatus(t, c, endpoint, http.MethodPost, valid, http.StatusNoContent)

	otherScope := strings.Replace(valid, workspaceA, workspaceB, 1)
	requestStatus(t, c, endpoint, http.MethodPost, otherScope, http.StatusForbidden)
	requestStatus(t, c, endpoint+"?mutation=unexpected", http.MethodPost, valid, http.StatusBadRequest)
	requestStatus(t, c, endpoint, http.MethodPost, valid+strings.Repeat(" ", MaxReceiptBytes), http.StatusRequestEntityTooLarge)

	cases := map[string]string{
		"unknown":       strings.Replace(valid, "{", "{\"status\":\"APPLIED\",", 1),
		"spoofed-peer":  strings.Replace(valid, "{", "{\"peer_id\":\"owner\",", 1),
		"duplicate":     strings.Replace(valid, "{", "{\"generation\":1,", 1),
		"folded":        strings.Replace(valid, "\"generation\"", "\"Generation\"", 1),
		"bad-digest":    strings.Replace(valid, receipt.ScopeDigest, "payload", 1),
		"missing":       strings.Replace(valid, "\"generation\":1,", "", 1),
		"null":          strings.Replace(valid, "\"generation\":1", "\"generation\":null", 1),
		"trailing-json": valid + "{}",
		"invalid-utf8":  strings.Replace(valid, "\"test\"", "\""+string([]byte{0xff})+"\"", 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			requestStatus(t, c, endpoint, http.MethodPost, body, http.StatusBadRequest)
		})
	}
	b.Lock()
	if b.snapshots != 0 || b.receipts != 1 || b.lastPeer != "processing-test" || b.receipt != receipt {
		t.Fatalf("unexpected receipt invocation: %d %d %q", b.snapshots, b.receipts, b.lastPeer)
	}
	b.receiptErr = app.ErrConflict
	b.Unlock()
	requestStatus(t, c, endpoint, http.MethodPost, valid, http.StatusConflict)
}

func TestProcessingClientPinsPublisherAndDoesNotRedirectOrRetry(t *testing.T) {
	cfg := provision(t)
	var requests atomic.Int32
	srv := cfg.serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != ReceiptPath {
			t.Error("unexpected follow-up destination")
		}
		requests.Add(1)
		w.Header().Set("Location", "/different-publisher")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	receipt := sampleReceipt()
	c, err := NewClient(srv.URL, cfg.processing, cfg.publisherCA.roots(), fingerprint(cfg.publisher.Leaf))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitReceipt(t.Context(), receipt); !errors.Is(err, errTransport) {
		t.Fatal("redirect was accepted as a receipt")
	}
	wrongPin := "sha256:" + strings.Repeat("1", 64)
	c, err = NewClient(srv.URL, cfg.processing, cfg.publisherCA.roots(), wrongPin)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitReceipt(t.Context(), receipt); !errors.Is(err, errTransport) {
		t.Fatal("wrong publisher fingerprint was accepted")
	}
	parsed, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// TLS still needs the ordinary SAN check. Use the same endpoint and CA but
	// trust only a leaf issued for a different address at the server.
	wrongSAN := cfg
	wrongSAN.publisher = cfg.publisherCA.issue(t, []string{"wrong.hit154.test"}, []net.IP{net.ParseIP("127.0.0.2")}, x509.ExtKeyUsageServerAuth, false)
	other := wrongSAN.serve(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("wrong publisher SAN reached maintenance")
	}))
	c, err = NewClient(other.URL, cfg.processing, cfg.publisherCA.roots(), fingerprint(wrongSAN.publisher.Leaf))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SubmitReceipt(t.Context(), receipt); !errors.Is(err, errTransport) {
		t.Fatal("wrong publisher SAN was accepted")
	}
	if _, err := NewClient("http://"+parsed.Host, cfg.processing, cfg.publisherCA.roots(), fingerprint(cfg.publisher.Leaf)); err == nil {
		t.Fatal("plaintext origin was accepted")
	}
	if requests.Load() != 1 {
		t.Fatalf("outbound receipt unexpectedly replayed %d times", requests.Load())
	}
}

func TestJSONReceiptAndSnapshotRejectAmbiguity(t *testing.T) {
	for _, b := range [][]byte{
		[]byte(`{"payload":{"revision":1,"revision":2},"signature":"x"}`),
		[]byte(`{"Payload":{},"signature":"x"}`),
		[]byte(`{"payload":{"ſignature":"x"}}`),
		[]byte(`{"payload":{},"signature":"x"}{}`),
		[]byte(`{"payload":[{"x":1,"x":2}]}`),
		bytes.Repeat([]byte("["), 33),
	} {
		if checkJSON(b) == nil {
			t.Fatalf("ambiguous snapshot accepted: %q", b)
		}
	}
	if err := checkJSON([]byte(`{"payload":{"decisions":[],"full":true,"revision":2},"signature":"eA=="}`)); err != nil {
		t.Fatal("strict JSON envelope rejected", err)
	}
}

type fixtureStore struct{ state app.State }

func (s *fixtureStore) Read(_ context.Context, f func(app.State) error) error {
	return f(s.state)
}

func (s *fixtureStore) Change(_ context.Context, f func(*app.State) error) error {
	return f(&s.state)
}

type noWrites struct{}

func (noWrites) WithCurrentOwner(context.Context, app.OwnerCheck, func(context.Context, app.OwnerIdentity) error) error {
	return app.ErrUnauthorized
}

func (noWrites) WithExactTargets(context.Context, app.Scope, func(context.Context) error) error {
	return app.ErrUnauthorized
}

func (noWrites) WithApplied(context.Context, app.Receipt, func(context.Context) error) error {
	return app.ErrUnauthorized
}

func TestAuthenticFullEmptySnapshotDoesNotStandInForUnknownOrOldGeneration(t *testing.T) {
	cfg := provision(t)
	state, err := app.NewState("test")
	if err != nil {
		t.Fatal(err)
	}
	verifyKey, signKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const publisherID = "13faf8b1-1c15-44d3-b09e-5d0e571bf104"
	reg, err := app.New(app.Config{Environment: "test", PublisherID: publisherID, ProcessingPeerID: "processing-test", SigningKey: signKey},
		&fixtureStore{state: state}, noWrites{}, noWrites{}, noWrites{})
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.ProvisionWorkspace(t.Context(), workspaceA, 1); err != nil {
		t.Fatal(err)
	}
	// Authorize reading this exact address at the transport, but there is
	// deliberately no authoritative scope yet for workspaceB.
	cfg.inventory[0].Workspaces = []string{workspaceA, workspaceB}
	h, err := New(reg, cfg.inventory)
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.serve(t, h)
	c, err := NewClient(srv.URL, cfg.processing, cfg.publisherCA.roots(), fingerprint(cfg.publisher.Leaf))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Snapshot(t.Context(), workspaceA, 1)
	scope := app.SnapshotScope{Environment: "test", PublisherID: publisherID, WorkspaceID: workspaceA, Generation: 1}
	if err != nil || got.Payload.Decisions == nil || len(got.Payload.Decisions) != 0 || app.VerifySnapshot(got, verifyKey, scope) != nil {
		t.Fatalf("signed full empty snapshot was not usable: %v", err)
	}
	if _, err := c.Snapshot(t.Context(), workspaceB, 1); !errors.Is(err, errTransport) {
		t.Fatal("an unknown workspace substituted an empty snapshot")
	}
	if _, err := c.Snapshot(t.Context(), workspaceA, 2); !errors.Is(err, errTransport) {
		t.Fatal("a foreign generation substituted an empty snapshot")
	}
	opened := false
	open := func(context.Context) error { opened = true; return nil }
	if err := reg.WithCurrent(t.Context(), workspaceA, 1, got, open); err != nil || !opened {
		t.Fatal("current, complete and verified empty scope could not open", err)
	}
	if err := reg.AdvanceGeneration(t.Context(), workspaceA, 1, 2); err != nil {
		t.Fatal(err)
	}
	if err := app.VerifySnapshot(got, verifyKey, scope); err != nil {
		t.Fatal("fixture old snapshot is no longer authentically signed", err)
	}
	opened = false
	if err := reg.WithCurrent(t.Context(), workspaceA, 1, got, open); !errors.Is(err, app.ErrConflict) || opened {
		t.Fatal("an authentically signed old snapshot opened the new state")
	}
	if _, err := c.Snapshot(t.Context(), workspaceA, 1); !errors.Is(err, errTransport) {
		t.Fatal("old generation remained available from the current publisher")
	}
}

func TestPublisherCannotSendForeignOrIncompleteSnapshotThroughAdapter(t *testing.T) {
	cfg := provision(t)
	empty := app.Snapshot{Payload: app.SnapshotPayload{
		SchemaVersion: app.SchemaVersion, Environment: "test",
		PublisherID: "13faf8b1-1c15-44d3-b09e-5d0e571bf104", WorkspaceID: workspaceB,
		Generation: 1, Full: true, Revision: 2, Decisions: []app.Decision{},
	}, Signature: make([]byte, ed25519.SignatureSize)}
	b := &recordingBackend{snapshot: empty}
	h, err := New(b, cfg.inventory)
	if err != nil {
		t.Fatal(err)
	}
	srv := cfg.serve(t, h)
	endpoint := srv.URL + SnapshotPath + "?workspace_id=" + workspaceA + "&generation=1"
	c := cfg.raw(t, &cfg.processing, tls.VersionTLS13)
	requestStatus(t, c, endpoint, http.MethodGet, "", http.StatusServiceUnavailable)
	b.Lock()
	b.snapshot.Payload.WorkspaceID = workspaceA
	b.snapshot.Payload.Decisions = nil
	b.Unlock()
	requestStatus(t, c, endpoint, http.MethodGet, "", http.StatusServiceUnavailable)
	malformed := cfg.serve(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "null")
	}))
	narrow, err := NewClient(malformed.URL, cfg.processing, cfg.publisherCA.roots(), fingerprint(cfg.publisher.Leaf))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := narrow.Snapshot(t.Context(), workspaceA, 1); !errors.Is(err, errTransport) {
		t.Fatal("an absent HTTP snapshot was treated as an empty current scope")
	}
}
