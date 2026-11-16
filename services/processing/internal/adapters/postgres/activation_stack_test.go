package postgres

// Integration-only test kept on the separate experimental stack branch.
import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestActivationExperimentalStackOnePrivateListener(t *testing.T) {
	f := newActivationFixture(t)
	reader := readcases.Service{Authority: readAuthority{readGrant(f.workspace)}, Boundary: &readBoundary{epoch: 7}, Clock: testClock{time.Now}, Repository: readStore(t, preparedFixture{files: f.files}, newReadPool(t, 1))}
	s, e := casereadhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{f.pki.Leaf(t, "processing.test", true, false)}, ClientCAs: f.pki.Root}, reader, "bff.test")
	if e != nil {
		t.Fatal(e)
	}
	obs := newObservation(t)
	oh, e := observationhttp.New(obs.service, "delivery.test", 256<<10)
	if e != nil {
		t.Fatal(e)
	}
	previous := s.Handler
	s.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/v1/delivery-observations" {
			oh.ServeHTTP(w, r)
		} else {
			previous.ServeHTTP(w, r)
		}
	})
	pool := newObservationPool(t, 4)
	gate := &intakeGate{epoch: 7}
	source := &intakeContexts{pool: pool, installed: receivesignals.Identity{WorkspaceID: f.workspace, ConnectionID: config, AccountID: "acct_controlled"}, epoch: 7, gate: gate}
	receive := receivesignals.Service{Contexts: source, Boundary: gate, Clock: testClock{time.Now}, Decoder: normalizedsignal.Decoder{}, Store: Store{DB: pool, Content: f.files}}
	if e = intakehttp.Attach(s, receive, "intake.test"); e != nil {
		t.Fatal(e)
	}
	if e = drafthttp.Attach(s, f.service, "bff.test", 256<<10); e != nil {
		t.Fatal(e)
	}
	if e = activationhttp.Attach(s, f.act, "bff.test", 256<<10); e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", s.Addr)
	if e != nil {
		t.Fatal(e)
	}
	go s.ServeTLS(l, "", "")
	t.Cleanup(func() { s.Close(); l.Close() })
	f.url = "https://" + l.Addr().String()
	f.client = &http.Client{Transport: f.pki.Transport(t), Timeout: 4 * time.Second}
	command := f.ready(t)
	status, body := f.activate(t, command, nil)
	if status != 201 {
		t.Fatal("activation on shared existing read listener", status, string(body))
	}
	var operation drafts.Operation
	json.Unmarshal(body, &operation)
	source.operation = operation.OperationID
	signal := intakeWire(t, f.workspace, func(m map[string]any) {
		m["stripe_account_id"] = "acct_controlled"
		m["source_created_at"] = operation.FirstActivationAt.Format(time.RFC3339Nano)
		m["first_received_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		m["expires_at"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	})
	tr := f.pki.Transport(t)
	tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "intake.test", false, false)}
	intake := &http.Client{Transport: tr, Timeout: 4 * time.Second}
	req, _ := http.NewRequest("POST", f.url+"/internal/v1/intake-signals", bytes.NewReader(signal))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", config)
	res, e := intake.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 202 {
		t.Fatal("Intake after actual activation", res.StatusCode, string(body))
	}
	req, _ = http.NewRequest("GET", f.url+"/api/v1/workspaces/"+f.workspace+"/cases", nil)
	req.Header.Set("X-HitPipe-Session", "synthetic-session")
	req.Header.Set("X-Request-ID", config)
	res, e = f.client.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatal("BFF list after actual activation/admission", res.StatusCode, string(body))
	}
	tr = f.pki.Transport(t)
	tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, false)}
	delivery := &http.Client{Transport: tr, Timeout: time.Second}
	req, _ = http.NewRequest("POST", f.url+"/internal/v1/delivery-observations", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", config)
	res, e = delivery.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	body, _ = io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatal("Delivery route did not survive attachment", res.StatusCode, string(body))
	}
	activationCapture(t, 201, command, func() []byte { _, b := f.activate(t, command, nil); return b }(), "experimental_shared_listener_activation_intake202_read200_delivery400")
}
