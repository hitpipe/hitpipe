package intake

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type authorityFunc func(context.Context, uuid.UUID, uuid.UUID) (Snapshot, error)

func (f authorityFunc) Current(ctx context.Context, workspace, id uuid.UUID) (Snapshot, error) {
	return f(ctx, workspace, id)
}

type gateFunc func(context.Context, uuid.UUID, int64, func() error) error

func (f gateFunc) WithOpen(ctx context.Context, workspace uuid.UUID, epoch int64, run func() error) error {
	return f(ctx, workspace, epoch, run)
}

type ledgerFunc func(context.Context, Intent, func() time.Time) (Receipt, error)

func (f ledgerFunc) Accept(ctx context.Context, intent Intent, now func() time.Time) (Receipt, error) {
	return f(ctx, intent, now)
}

func snapshot(i Intent) Snapshot {
	return Snapshot{
		Workspace: i.WorkspaceID, Case: i.CaseID, Operation: i.OperationID, Config: i.ConfigID,
		Lineage: i.LineageID, Episode: i.EpisodeID, Connection: i.ConnectionID, SourceFacts: i.SourceFactsID,
		Epoch: i.Epoch, ConfigDigest: i.ConfigDigest, FactsDigest: i.FactsDigest,
		FactsUntil: i.FactsUntil, Deadline: i.Deadline, DestinationWorkspace: i.DestinationWorkspace,
		List: i.List, Assignee: i.Assignee,
	}
}

func TestHTTPContract(t *testing.T) {
	now := time.Now().UTC()
	raw := signed(t, testFields(now))
	intent, err := DecodeIntent(raw)
	if err != nil {
		t.Fatal(err)
	}
	record := uuid.New()
	tests := []struct {
		name                                      string
		status                                    int
		body                                      []byte
		authorityErr, gateErr, ledgerErr          error
		wrongSAN, wrongRoot, noCert, wrongBinding bool
	}{
		{name: "receipt", status: 202},
		{name: "wrong caller", status: 401, wrongSAN: true},
		{name: "untrusted certificate", wrongRoot: true},
		{name: "missing certificate", noCert: true},
		{name: "unknown field", status: 400, body: append([]byte("{\"extra\":1,"), raw[1:]...)},
		{name: "too big", status: 413, body: []byte(strings.Repeat(" ", maxWireBytes+1))},
		{name: "wrong binding", status: 403, wrongBinding: true},
		{name: "closed gate", status: 403, gateErr: ErrDenied},
		{name: "not visible", status: 404, authorityErr: ErrNotFound},
		{name: "conflict", status: 409, ledgerErr: ErrConflict},
		{name: "expired", status: 412, ledgerErr: ErrStale},
		{name: "quota", status: 429, ledgerErr: ErrQuota},
		{name: "unavailable authority", status: 503, authorityErr: ErrUnavailable},
		{name: "unavailable storage", status: 503, ledgerErr: ErrUnavailable},
	}
	ca := newCertificateAuthority(t)
	untrusted := newCertificateAuthority(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			current := snapshot(intent)
			if tc.wrongBinding {
				current.List = "different"
			}
			s, err := New(authorityFunc(func(context.Context, uuid.UUID, uuid.UUID) (Snapshot, error) {
				return current, tc.authorityErr
			}), gateFunc(func(ctx context.Context, ws uuid.UUID, epoch int64, run func() error) error {
				if tc.gateErr != nil {
					return tc.gateErr
				}
				return run()
			}), ledgerFunc(func(_ context.Context, i Intent, _ func() time.Time) (Receipt, error) {
				if tc.status != 202 && tc.ledgerErr == nil {
					t.Error("unexpected durable write")
				}
				return Receipt{MessageID: i.MessageID, Disposition: "APPLIED", RecordID: record}, tc.ledgerErr
			}))
			if err != nil {
				t.Fatal(err)
			}

			serverConfig, err := NewHTTP(s, ca.issue(t, "localhost", x509.ExtKeyUsageServerAuth), ca.pool, "processing.internal")
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewUnstartedServer(serverConfig.Handler)
			server.TLS = serverConfig.TLSConfig
			server.StartTLS()
			defer server.Close()
			clientTLS := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: "localhost", RootCAs: ca.pool}
			if !tc.noCert {
				issuer, san := ca, "processing.internal"
				if tc.wrongSAN {
					san = "iam.internal"
				}
				if tc.wrongRoot {
					issuer = untrusted
				}
				clientTLS.Certificates = []tls.Certificate{issuer.issue(t, san, x509.ExtKeyUsageClientAuth)}
			}
			transport := &http.Transport{TLSClientConfig: clientTLS}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			body := tc.body
			if body == nil {
				body = raw
			}
			requestID := uuid.New()
			req, err := http.NewRequest(http.MethodPost, server.URL+route, bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", requestID.String())
			resp, err := client.Do(req)
			if tc.status == 0 {
				if err == nil {
					resp.Body.Close()
					t.Fatal("TLS connection accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status || resp.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected status or content type: %v", resp.StatusCode)
			}
			var fields map[string]json.RawMessage
			if err := json.NewDecoder(resp.Body).Decode(&fields); err != nil {
				t.Fatal(err)
			}
			if tc.status == 202 {
				if len(fields) != 3 || string(fields["message_id"]) != quoted(intent.MessageID) ||
					string(fields["record_id"]) != quoted(record) || string(fields["disposition"]) != "\"APPLIED\"" {
					t.Fatalf("unexpected Receipt: %s", fields)
				}
			} else if len(fields) != 3 || string(fields["request_id"]) != quoted(requestID) ||
				len(fields["code"]) < 3 || len(fields["message"]) < 2 {
				t.Fatalf("unexpected Error: %s", fields)
			}
		})
	}
}

func TestUnwiredService(t *testing.T) {
	if s, err := New(nil, nil, nil); s != nil || err == nil {
		t.Fatal("unwired service admitted")
	}
	if s, err := NewHTTP(nil, tls.Certificate{}, nil, ""); s != nil || err == nil {
		t.Fatal("unwired listener admitted")
	}
}

func quoted(id uuid.UUID) string { return "\"" + id.String() + "\"" }

type testCA struct {
	key  *ecdsa.PrivateKey
	cert *x509.Certificate
	pool *x509.CertPool
}

func newCertificateAuthority(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := certificateTemplate()
	template.IsCA = true
	template.BasicConstraintsValid = true
	template.KeyUsage = x509.KeyUsageCertSign
	raw, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &testCA{key, cert, pool}
}

func certificateTemplate() *x509.Certificate {
	id := uuid.New()
	return &x509.Certificate{SerialNumber: new(big.Int).SetBytes(id[:]), Subject: pkix.Name{CommonName: "Delivery fixture"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
}

func (ca *testCA) issue(t *testing.T, san string, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := certificateTemplate()
	template.DNSNames = []string{san}
	template.ExtKeyUsage = []x509.ExtKeyUsage{usage}
	raw, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{raw}, PrivateKey: key}
}
