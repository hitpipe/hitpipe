package postgres

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/admission"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
	"github.com/jackc/pgx/v5"
)

type crashAdmissionDB struct{ *pgx.Conn }
type crashAdmissionTX struct{ pgx.Tx }

func (d crashAdmissionDB) BeginTx(ctx context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.Conn.BeginTx(ctx, o)
	return crashAdmissionTX{tx}, e
}
func (t crashAdmissionTX) Commit(ctx context.Context) error {
	if e := t.Tx.Commit(ctx); e != nil {
		return e
	}
	os.Exit(24)
	return nil
}

func TestRevision3AdmissionCrashThenHTTPExpiryPurgeAndRestart(t *testing.T) {
	if os.Getenv("HIT95_ADMISSION_HELPER") == "1" {
		files, e := contentfiles.New(os.Getenv("HIT95_ROOT"))
		if e != nil {
			t.Fatal(e)
		}
		raw, e := base64.StdEncoding.DecodeString(os.Getenv("HIT95_WIRE"))
		if e != nil {
			t.Fatal(e)
		}
		_, e = admission.Admit(ctx, raw, established(os.Getenv("HIT95_WORKSPACE")), normalizedsignal.Decoder{}, Store{DB: crashAdmissionDB{connect(t)}, Content: files, HistoryRetention: 2 * time.Second})
		t.Fatalf("actual commit/crash not reached: %v", e)
	}
	f := prepared(t, []string{})
	raw := newSessionSignal(t, f, "cs_process_crash_history")
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	command := exec.Command(exe, "-test.run=^TestRevision3AdmissionCrashThenHTTPExpiryPurgeAndRestart$")
	command.Env = append(os.Environ(), "HIT95_ADMISSION_HELPER=1", "HIT95_ROOT="+f.path, "HIT95_WORKSPACE="+f.request.WorkspaceID, "HIT95_WIRE="+base64.StdEncoding.EncodeToString(raw))
	output, e := command.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(e, &exit) || exit.ExitCode() != 24 {
		t.Fatalf("commit/crash not reached: %v %s", e, output)
	}
	replay, e := admission.Admit(ctx, raw, established(f.request.WorkspaceID), normalizedsignal.Decoder{}, Store{DB: connect(t), Content: f.files})
	if e != nil {
		t.Fatal(e)
	}
	ref, _ := historyRef(t, f.request.WorkspaceID, replay.RecordID)
	roots, serverCert, clientCert := readCertificates(t)
	reader := readStore(t, f, newReadPool(t, 1))
	g := &readBoundary{epoch: 7}
	serve := func(store CaseReadStore) *httptest.Server {
		service := readcases.Service{Authority: readAuthority{readGrant(f.request.WorkspaceID)}, Boundary: g, Clock: testClock{time.Now}, Repository: store}
		s, e := casereadhttp.NewServer("127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{serverCert}, ClientCAs: roots}, service, "bff.private")
		if e != nil {
			t.Fatal(e)
		}
		ts := httptest.NewUnstartedServer(s.Handler)
		ts.TLS = s.TLSConfig
		ts.StartTLS()
		t.Cleanup(ts.Close)
		return ts
	}
	server := serve(reader)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{clientCert}, MinVersion: tls.VersionTLS13}}, Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()
	captures := []map[string]any{}
	get := func(ts *httptest.Server, path, operation, schema, scenario string) []byte {
		req, _ := http.NewRequest("GET", ts.URL+"/api/v1/workspaces/"+f.request.WorkspaceID+"/cases"+path, nil)
		req.Header.Set("X-HitPipe-Session", "controlled-session")
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(res.Body)
		res.Body.Close()
		if e != nil || res.StatusCode != 200 {
			t.Fatalf("%s: %d %s %v", scenario, res.StatusCode, b, e)
		}
		var payload any
		if json.Unmarshal(b, &payload) != nil {
			t.Fatal(string(b))
		}
		captures = append(captures, map[string]any{"operation": operation, "schema": schema, "status": 200, "scenario": scenario, "body": payload, "local_actual_mtls_postgres_content": true})
		return b
	}
	path := "/" + replay.RecordID + "/history"
	b := get(server, path, "getCaseHistory", "History", "actual-admission-process-crash-before-ack")
	var original readcases.HistoryView
	if json.Unmarshal(b, &original) != nil || len(original.Items) != 1 {
		t.Fatal(string(b))
	}
	b = get(server, "?limit=1", "listCases", "Cases", "nonempty-case-keyset-first")
	var first readcases.Cases
	json.Unmarshal(b, &first)
	if len(first.Items) != 1 || first.NextCursor == nil {
		t.Fatal("case first page")
	}
	b = get(server, "?limit=1&cursor="+*first.NextCursor, "listCases", "Cases", "nonempty-case-keyset-next")
	var second readcases.Cases
	json.Unmarshal(b, &second)
	if len(second.Items) != 1 || second.NextCursor != nil || first.Items[0].CaseID == second.Items[0].CaseID {
		t.Fatal("case continuation")
	}
	get(server, "/"+replay.RecordID, "getCase", "CaseDetail", "nonempty-detail-after-real-admission")
	if wait := time.Until(ref.ExpiresAt); wait > 0 {
		time.Sleep(wait + time.Millisecond)
	}
	b = get(server, path, "getCaseHistory", "History", "expired-before-purge")
	var expired readcases.HistoryView
	json.Unmarshal(b, &expired)
	if !reflect.DeepEqual(expired, original) {
		t.Fatal("expiry changed minimum history")
	}
	if e = f.files.PurgeExpired(time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(f.path, ref.ID)); !os.IsNotExist(e) {
		t.Fatal("expired body remains", e)
	}
	b = get(server, path, "getCaseHistory", "History", "purge-before-repeat-read")
	json.Unmarshal(b, &expired)
	if !reflect.DeepEqual(expired, original) {
		t.Fatal("purge changed safety")
	}
	server.Close()
	reopened, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	reader.Content = reopened
	server = serve(reader)
	b = get(server, path, "getCaseHistory", "History", "new-listener-and-content-store-after-process-restart")
	json.Unmarshal(b, &expired)
	if !reflect.DeepEqual(expired, original) {
		t.Fatal("restart changed history")
	}
	replayed, e := admission.Admit(ctx, raw, established(f.request.WorkspaceID), normalizedsignal.Decoder{}, Store{DB: connect(t), Content: reopened})
	if e != nil || replayed != replay {
		t.Fatal(e)
	}
	unchanged, _ := historyRef(t, f.request.WorkspaceID, replay.RecordID)
	if !reflect.DeepEqual(unchanged, ref) {
		t.Fatal("replay extended expiry")
	}
	saveReadCaptures(t, captures)
}
