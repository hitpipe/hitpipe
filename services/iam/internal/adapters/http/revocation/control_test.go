package revocation

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	app "github.com/hitpipe/hitpipe/services/iam/internal/application/revocation"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

const (
	runtimeID    = "10000000-0000-4000-8000-000000000001"
	bootID       = "10000000-0000-4000-8000-000000000002"
	revocationID = "10000000-0000-4000-8000-000000000003"
	subjectID    = "10000000-0000-4000-8000-000000000004"
	otherID      = "10000000-0000-4000-8000-000000000005"
)

type testCA struct {
	root *x509.Certificate
	key  ed25519.PrivateKey
	pool *x509.CertPool
}

func newCA(t *testing.T) testCA {
	t.Helper()
	now := time.Now()
	key, der := issueCert(t, &x509.Certificate{
		SerialNumber: big.NewInt(1), IsCA: true, BasicConstraintsValid: true,
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		KeyUsage: x509.KeyUsageCertSign,
	}, nil, nil)
	root, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	return testCA{root, key, pool}
}

func issueCert(t *testing.T, template, parent *x509.Certificate, signer ed25519.PrivateKey) (ed25519.PrivateKey, []byte) {
	t.Helper()
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if parent == nil {
		parent, signer = template, key
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, public, signer)
	if err != nil {
		t.Fatal(err)
	}
	return key, der
}

func (ca testCA) leaf(t *testing.T, san string, usage x509.ExtKeyUsage, expired bool) tls.Certificate {
	t.Helper()
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Minute),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{usage},
	}
	if expired {
		template.NotAfter = now.Add(-time.Second)
	}
	if ip := net.ParseIP(san); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{san}
	}
	key, der := issueCert(t, template, ca.root, ca.key)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func certificatePin(cert tls.Certificate) string {
	digest := sha256.Sum256(cert.Certificate[0])
	return "sha256:" + hex.EncodeToString(digest[:])
}

type networkFixture struct {
	iamCA, runtimeCA testCA
	iam, runtime     tls.Certificate
	gate             *startgate.Gate
	command          startgate.BarrierCommand
}

func fixture(t *testing.T) networkFixture {
	t.Helper()
	f := networkFixture{iamCA: newCA(t), runtimeCA: newCA(t)}
	f.iam = f.iamCA.leaf(t, "iam.fixture.test", x509.ExtKeyUsageClientAuth, false)
	f.runtime = f.runtimeCA.leaf(t, "127.0.0.1", x509.ExtKeyUsageServerAuth, false)
	f.command = startgate.BarrierCommand{
		TargetRuntimeID: runtimeID, BootID: bootID, Generation: 5, RevocationID: revocationID, Epoch: 9,
		Subject: startgate.Subject{Scope: startgate.Session, SubjectID: subjectID}, DeadlineAt: time.Now().Add(5 * time.Second),
	}
	var err error
	f.gate, err = startgate.New(f.identity())
	if err != nil {
		t.Fatal(err)
	}
	err = f.gate.ApplySnapshot(t.Context(), startgate.RuntimeSnapshot{
		RuntimeID: runtimeID, BootID: bootID, Generation: 5, SnapshotRevision: 1, Gate: startgate.Open,
	})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f networkFixture) identity() startgate.Identity {
	return startgate.Identity{RuntimeID: runtimeID, BootID: bootID, Generation: 5}
}

func (f networkFixture) participant() app.Participant {
	return app.Participant{Identity: f.identity(), CertificateFingerprint: certificatePin(f.runtime)}
}

func (f networkFixture) handler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := NewHandler(f.gate, f.iamCA.pool, certificatePin(f.iam))
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func (f networkFixture) server(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	cfg, err := ServerTLS(f.runtime, f.iamCA.pool, certificatePin(f.iam))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = cfg
	server.StartTLS()
	t.Cleanup(server.Close)
	return server
}

func (f networkFixture) client(t *testing.T, cert tls.Certificate, origin string) *Client {
	t.Helper()
	client, err := NewClient(cert, f.runtimeCA.pool, map[string]string{runtimeID: origin})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (f networkFixture) admitStart(ctx context.Context, started func(context.Context) error) error {
	return f.gate.Start(ctx, startgate.StartRequest{
		Authorities: []startgate.Authority{{Subject: f.command.Subject, Epoch: f.command.Epoch - 1, Allowed: true}},
	}, started)
}

func TestControlUsesActualStartGate(t *testing.T) {
	f := fixture(t)
	received := make(chan struct{}, 1)
	handler := f.handler(t)
	server := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		handler.ServeHTTP(w, r)
	}))
	client := f.client(t, f.iam, server.URL)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- f.admitStart(t.Context(), func(ctx context.Context) error {
			close(entered)
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("old protected start never entered")
	}
	type outcome struct {
		ack startgate.BarrierAck
		err error
	}
	result := make(chan outcome, 1)
	go func() {
		ack, err := client.Send(t.Context(), f.participant(), f.command)
		result <- outcome{ack, err}
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("mTLS control request did not reach the target")
	}
	select {
	case got := <-result:
		t.Fatalf("barrier responded while the actual start was still in its section: %v", got)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got := <-result
	if got.err != nil || !ackMatches(f.command, got.ack) {
		t.Fatalf("first exact ACK not proved: %v", got)
	}
	if err := f.admitStart(t.Context(), func(context.Context) error {
		t.Error("old-epoch protected work started after ACK")
		return nil
	}); !errors.Is(err, startgate.ErrDenied) {
		t.Fatalf("unexpected old-epoch start result: %v", err)
	}
	replayed, err := client.Send(t.Context(), f.participant(), f.command)
	if err != nil || !ackMatches(f.command, replayed) || !replayed.AcknowledgedAt.Equal(got.ack.AcknowledgedAt) {
		t.Fatalf("exact replay did not return the already-issued ACK: %v", err)
	}
}

func TestControlVerifiesBothTLSIdentitiesWithNoRedirect(t *testing.T) {
	f := fixture(t)
	var reached atomic.Int32
	handler := f.handler(t)
	server := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		handler.ServeHTTP(w, r)
	}))
	wrongCaller := f.iamCA.leaf(t, "different.fixture.test", x509.ExtKeyUsageClientAuth, false)
	expiredCaller := f.iamCA.leaf(t, "iam.fixture.test", x509.ExtKeyUsageClientAuth, true)
	for _, cert := range []tls.Certificate{wrongCaller, expiredCaller} {
		if _, err := f.client(t, cert, server.URL).Send(t.Context(), f.participant(), f.command); err == nil {
			t.Fatal("untrusted caller obtained ACK")
		}
	}
	wrongPin := f.participant()
	wrongPin.CertificateFingerprint = certificatePin(f.iam)
	if _, err := f.client(t, f.iam, server.URL).Send(t.Context(), wrongPin, f.command); err == nil {
		t.Fatal("accepted trusted CA and valid SAN but incorrect registry leaf")
	}
	for _, cert := range []tls.Certificate{
		f.runtimeCA.leaf(t, "wrong.fixture.test", x509.ExtKeyUsageServerAuth, false),
		f.runtimeCA.leaf(t, "127.0.0.1", x509.ExtKeyUsageServerAuth, true),
		newCA(t).leaf(t, "127.0.0.1", x509.ExtKeyUsageServerAuth, false),
	} {
		alternate := f
		alternate.runtime = cert
		server := alternate.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reached.Add(1)
			handler.ServeHTTP(w, r)
		}))
		if _, err := f.client(t, f.iam, server.URL).Send(t.Context(), alternate.participant(), f.command); err == nil {
			t.Fatal("server leaf pin bypassed SAN, validity interval or CA verification")
		}
	}
	redirect := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, server.URL+Path, http.StatusTemporaryRedirect)
	}))
	if _, err := f.client(t, f.iam, redirect.URL).Send(t.Context(), f.participant(), f.command); err == nil || reached.Load() != 0 {
		t.Fatalf("redirect or invalid TLS peer reached control handler: %d", reached.Load())
	}
	if err := f.admitStart(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("an unauthenticated request affected the gate: %v", err)
	}
}

func TestControlUsesIndependentlyProvisionedRuntimeLeaves(t *testing.T) {
	f := fixture(t)
	registration := f.runtimeCA.leaf(t, "processing-client.fixture.test", x509.ExtKeyUsageClientAuth, false)
	var reached atomic.Int32
	handler := f.handler(t)
	server := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		handler.ServeHTTP(w, r)
	}))
	binding := ControlBinding{
		Identity: f.identity(), RegistrationFingerprint: certificatePin(registration),
		ControlFingerprint: certificatePin(f.runtime), Origin: server.URL,
	}
	client, err := NewClientWithControlBindings(f.iam, f.runtimeCA.pool, []ControlBinding{binding})
	if err != nil {
		t.Fatal(err)
	}
	participant := app.Participant{Identity: f.identity(), CertificateFingerprint: certificatePin(registration)}
	for _, field := range []string{"registry pin", "boot", "generation"} {
		t.Run(field, func(t *testing.T) {
			other := participant
			command := f.command
			switch field {
			case "registry pin":
				// The control server leaf also has a valid chain/SAN. It is not
				// the outbound identity configured for this registry entry.
				other.CertificateFingerprint = certificatePin(f.runtime)
			case "boot":
				other.Identity.BootID, command.BootID = otherID, otherID
			case "generation":
				other.Identity.Generation++
				command.Generation++
			}
			if _, err := client.Send(t.Context(), other, command); err == nil {
				t.Fatal("ACK established without an exact control binding")
			}
		})
	}
	wrong := binding
	wrong.ControlFingerprint = binding.RegistrationFingerprint
	untrusted, err := NewClientWithControlBindings(f.iam, f.runtimeCA.pool, []ControlBinding{wrong})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := untrusted.Send(t.Context(), participant, f.command); err == nil || reached.Load() != 0 {
		t.Fatalf("an unprovisioned control peer reached the handler: %d", reached.Load())
	}
	if _, err := NewClientWithControlBindings(f.iam, f.runtimeCA.pool, []ControlBinding{binding, binding}); err == nil {
		t.Fatal("duplicate bindings accepted for the same process incarnation")
	}
	ack, err := client.Send(t.Context(), participant, f.command)
	if err != nil || !ackMatches(f.command, ack) || reached.Load() != 1 {
		t.Fatalf("independently bound control listener did not prove ACK: %v", err)
	}
}

func TestControlRejectsUnprovedAcknowledgements(t *testing.T) {
	f := fixture(t)
	fake := ackWire{
		RevocationID: revocationID, RuntimeID: runtimeID, BootID: bootID, Generation: 5,
		Scope: "SESSION", SubjectID: subjectID, AppliedEpoch: 9,
		OldStartSectionsDrained: true, AcknowledgedAt: time.Now().UTC(),
	}
	for name, change := range map[string]func(*ackWire){
		"runtime":    func(a *ackWire) { a.RuntimeID = otherID },
		"boot":       func(a *ackWire) { a.BootID = otherID },
		"generation": func(a *ackWire) { a.Generation++ },
		"revocation": func(a *ackWire) { a.RevocationID = otherID },
		"subject":    func(a *ackWire) { a.SubjectID = otherID },
		"workspace":  func(a *ackWire) { a.WorkspaceID = &a.SubjectID },
		"epoch":      func(a *ackWire) { a.AppliedEpoch++ },
		"drained":    func(a *ackWire) { a.OldStartSectionsDrained = false },
		"late":       func(a *ackWire) { a.AcknowledgedAt = f.command.DeadlineAt.UTC() },
	} {
		t.Run(name, func(t *testing.T) {
			ack := fake
			change(&ack)
			server := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, r.Header.Get("X-Request-ID"), http.StatusOK, ack)
			}))
			if _, err := f.client(t, f.iam, server.URL).Send(t.Context(), f.participant(), f.command); err == nil {
				t.Fatal("accepted nonmatching ACK over a trusted TLS exchange")
			}
		})
	}
}

func TestControlRejectsInvalidCommandsBeforeApplyingAcceptedCommand(t *testing.T) {
	f := fixture(t)
	server := f.server(t, f.handler(t))
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: f.runtimeCA.pool, Certificates: []tls.Certificate{f.iam}}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	wire := commandValue(f.command)
	data, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{"duplicate", strings.Replace(string(data), `"epoch":9`, `"epoch":9,"\u0065poch":9`, 1), 400},
		{"unknown", strings.Replace(string(data), `"epoch":9`, `"epoch":9,"secret":"test-only"`, 1), 400},
		{"wrong key case", strings.Replace(string(data), `"epoch":9`, `"Epoch":9`, 1), 400},
		{"absent nullable", strings.Replace(string(data), `"workspace_id":null,`, "", 1), 400},
		{"point release", strings.Replace(string(data), `"epoch":9`, `"epoch":9.0`, 1), 400},
		{"overflow", strings.Replace(string(data), `"epoch":9`, `"epoch":9007199254740992`, 1), 400},
		{"null required", strings.Replace(string(data), `"epoch":9`, `"epoch":null`, 1), 400},
		{"wrong boot", strings.Replace(string(data), bootID, otherID, 1), 412},
		{"unsupported scope", strings.Replace(string(data), "SESSION", "CONNECTION", 1), 400},
		{"trailing JSON", string(data) + `{}`, 400},
		{"too large", strings.Repeat(" ", maxBodyBytes+1), 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+Path, strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Request-ID", otherID)
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != test.want || bytes.Contains(body, []byte(revocationID)) {
				t.Fatalf("expected HTTP %d and no ACK or input trace, got %d (%s)", test.want, response.StatusCode, body)
			}
		})
	}
	if err := f.admitStart(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("invalid input changed protected work: %v", err)
	}
	expired := f.command
	expired.DeadlineAt = time.Now().Add(-time.Second)
	if _, err := f.client(t, f.iam, server.URL).Send(t.Context(), f.participant(), expired); err == nil {
		t.Fatal("new expired barrier yielded an ACK")
	}
}

func TestControlWorkspaceIdentityDoesNotAffectAnUnrelatedWorkspace(t *testing.T) {
	f := fixture(t)
	workspace := subjectID
	f.command.Subject = startgate.Subject{Scope: startgate.Workspace, SubjectID: workspace, WorkspaceID: &workspace}
	server := f.server(t, f.handler(t))
	client := f.client(t, f.iam, server.URL)

	conflicting := f.command
	conflicting.Subject.WorkspaceID = nil
	if _, err := client.Send(t.Context(), f.participant(), conflicting); err == nil {
		t.Fatal("unscoped workspace command sent")
	}
	ack, err := client.Send(t.Context(), f.participant(), f.command)
	if err != nil || ack.Subject.WorkspaceID == nil || *ack.Subject.WorkspaceID != workspace {
		t.Fatalf("workspace ACK identity not established: %v", err)
	}
	if err := f.admitStart(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, startgate.ErrDenied) {
		t.Fatalf("workspace barrier did not deny protected start: %v", err)
	}
	unaffected := f
	otherWorkspace := otherID
	unaffected.command.Subject = startgate.Subject{Scope: startgate.Workspace, SubjectID: otherWorkspace, WorkspaceID: &otherWorkspace}
	if err := unaffected.admitStart(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("unrelated workspace did not stay open: %v", err)
	}
}

func TestControlExchangeAllowsTransportHeadroom(t *testing.T) {
	f := fixture(t)
	handler := f.handler(t)
	server := f.server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := time.NewTimer(2100 * time.Millisecond)
		defer delay.Stop()
		select {
		case <-delay.C:
			handler.ServeHTTP(w, r)
		case <-r.Context().Done():
		}
	}))
	ack, err := f.client(t, f.iam, server.URL).Send(t.Context(), f.participant(), f.command)
	if err != nil || !ackMatches(f.command, ack) {
		t.Fatalf("an exchange lasting just over two seconds failed before the five-second command deadline: %v", err)
	}
}
