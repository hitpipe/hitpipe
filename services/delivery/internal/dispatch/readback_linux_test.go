package dispatch

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
)

type readAuth func(context.Context, intake.Query) (Authorization, error)

func (f readAuth) AuthorizeRead(c context.Context, q intake.Query) (Authorization, error) {
	return f(c, q)
}

type unusedAdmission struct{}

func (unusedAdmission) Current(context.Context, uuid.UUID, uuid.UUID) (intake.Snapshot, error) {
	return intake.Snapshot{}, intake.ErrDenied
}

type testCerts struct {
	cert  *x509.Certificate
	key   *ecdsa.PrivateKey
	roots *x509.CertPool
}

func newReadCA(t *testing.T) *testCerts {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := certTemplate()
	tmpl.IsCA, tmpl.BasicConstraintsValid = true, true
	tmpl.KeyUsage = x509.KeyUsageCertSign
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return &testCerts{cert, key, roots}
}
func certTemplate() *x509.Certificate {
	return &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
}
func (ca *testCerts) issue(t *testing.T, san string, usage x509.ExtKeyUsage) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := certTemplate()
	tmpl.ExtKeyUsage = []x509.ExtKeyUsage{usage}
	if san == "127.0.0.1" {
		tmpl.IPAddresses = append(tmpl.IPAddresses, []byte{127, 0, 0, 1})
	} else {
		tmpl.DNSNames = []string{san}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func canonicalTestBlock(id uuid.UUID) string {
	return fmt.Sprintf("[HITPIPE-HANDOFF:%s:v1]\nService: Example\nOrder: fixture-order\nContext:\nnote: α\ncontinued\n[/HITPIPE-HANDOFF:%s:v1]", id, id)
}

func knownFixture(t *testing.T, f *fixture, fp [32]byte) (Job, Authorization, intake.Query) {
	t.Helper()
	job, a := f.prepareWithBlock(t, fp, false, canonicalTestBlock)
	ctx := t.Context()
	rec, err := f.worker.load(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.worker.fence(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err = f.worker.markStart(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err = f.worker.finish(ctx, rec, outcome{Result{TargetIDObserved, "fixture-task"}, "PROVIDER_TASK_ID_RETURNED", checksum([]byte("fixture identity"))}); err != nil {
		t.Fatal(err)
	}
	return job, a, intake.Query{Workspace: a.Workspace, Case: a.Case, Lineage: a.Lineage, Epoch: a.Epoch, Deadline: time.Now().Add(time.Minute)}
}

func queryFixture(t *testing.T, f *fixture, a Authorization, fp [32]byte, target *ClickUp) *QueryService {
	t.Helper()
	q, err := NewQueryService(f.worker, f.dir, readAuth(func(context.Context, intake.Query) (Authorization, error) { return a, nil }),
		allowGate(acceptGate), credentialSource(func(context.Context, uuid.UUID, uuid.UUID, int64) (Credential, error) {
			return Credential{fixtureToken, fp}, nil
		}), target)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func targetJSON(t *testing.T, a Authorization, id string, mode string) []byte {
	t.Helper()
	attrs := map[string]any{"id": id, "team_id": "fixture-workspace", "list": map[string]any{"id": 456},
		"name": fixtureTitle, "description": "Note before\n" + canonicalTestBlock(a.Case) + "\nNote after",
		"assignees": []map[string]any{{"id": json.Number(exactAssignee)}}}
	switch mode {
	case "incomplete":
		delete(attrs, "team_id")
	case "mismatch":
		attrs["name"] = "Wrong title"
	case "wrong-id":
		attrs["id"] = "different-task"
	case "content":
		attrs["description"] = strings.Replace(canonicalTestBlock(a.Case), "continued", "changed", 1)
	}
	raw, err := json.Marshal(attrs)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestReadbackAndDurableOutbox(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 25)
	_, auth, q := knownFixture(t, f, fp)
	var reads atomic.Int64
	target := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v2/task/fixture-task" || r.URL.RawQuery != "" || r.Header.Get("Authorization") != fixtureToken {
			t.Error("unexpected ClickUp request")
		}
		reads.Add(1)
		w.Write(targetJSON(t, auth, "fixture-task", ""))
	}))
	service := queryFixture(t, f, auth, fp, target)
	ca := newReadCA(t)
	serverTLS := ca.issue(t, "127.0.0.1", x509.ExtKeyUsageServerAuth)
	caller := ca.issue(t, "processing.internal", x509.ExtKeyUsageClientAuth)
	intakeService, err := intake.New(unusedAdmission{}, allowGate(acceptGate), f.app)
	if err != nil {
		t.Fatal(err)
	}
	httpConfig, err := intake.NewHTTPWithQuery(intakeService, service, serverTLS, ca.roots, "processing.internal")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(httpConfig.Handler)
	server.TLS = httpConfig.TLSConfig
	server.StartTLS()
	defer server.Close()
	request := func(cert tls.Certificate, body []byte) (int, []byte, error) {
		client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs: ca.roots, MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert},
		}}}
		defer client.CloseIdleConnections()
		req, err := http.NewRequest(http.MethodPost, server.URL+"/internal/v1/delivery-query", bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("X-Request-ID", uuid.NewString())
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		return resp.StatusCode, b, err
	}
	wire, err := json.Marshal(map[string]any{"workspace_id": q.Workspace, "case_id": q.Case, "lineage_id": q.Lineage,
		"expected_connection_epoch": q.Epoch, "deadline_at": q.Deadline})
	if err != nil {
		t.Fatal(err)
	}
	bad := ca.issue(t, "incorrect.internal", x509.ExtKeyUsageClientAuth)
	if status, _, err := request(bad, wire); err != nil || status != 401 {
		t.Fatalf("wrong caller: %d %v", status, err)
	}
	if status, _, err := request(tls.Certificate{}, wire); err == nil {
		t.Fatalf("missing certificate: %d", status)
	}
	if status, _, err := request(caller, append([]byte("{\"lineage_id\":null,"), wire[1:]...)); err != nil || status != 400 {
		t.Fatalf("duplicate or null field: %d %v", status, err)
	}
	status, data, err := request(caller, wire)
	var o intake.TaskObservation
	if err != nil || status != 200 || json.Unmarshal(data, &o) != nil || o.Knowledge != "RESULT_VERIFIED" || !o.Complete ||
		o.EvidenceRef == nil || o.EvidenceExpires == nil || o.ObservedAt.IsZero() || reads.Load() != 1 {
		t.Fatalf("query failed: status=%d err=%v", status, err)
	}
	sum := o.MessageDigest
	encoded, err := intake.EncodeObservation(&o)
	if err != nil || sum != o.MessageDigest {
		t.Fatal("invalid RFC8785 digest")
	}
	var attrs intake.ContentRef
	var observedState, id string
	var fence bool
	err = f.admin.QueryRow(t.Context(), `SELECT c.actual_attributes_ref,c.actual_attributes_sha256,c.actual_attributes_bytes,c.expires_at,
		l.state,l.known_task_id,l.ever_dispatch_fenced FROM hp_delivery.observation_content c
		JOIN hp_delivery.effect_observations o USING(workspace_id,observation_id)
		JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id) WHERE c.observation_id=$1`, o.Observation).
		Scan(&attrs.ID, &attrs.SHA256, &attrs.Size, &attrs.Expires, &observedState, &id, &fence)
	if err != nil || attrs.ID != *o.EvidenceRef || attrs.SHA256 != o.ProofDigest || !attrs.Expires.Equal(*o.EvidenceExpires) ||
		observedState != "TARGET_OBSERVED" || id != "fixture-task" || !fence {
		t.Fatalf("durable observation incorrect: %v", err)
	}
	contents, err := f.dir.Read(attrs, time.Now())
	var projection map[string]any
	if err != nil || json.Unmarshal(contents, &projection) != nil || projection["source"] != observationSource ||
		projection["handoff_block"] != canonicalTestBlock(auth.Case) {
		t.Fatal("missing verified provider provenance or bounded block")
	}

	fenceClaimCrash(t, o.Workspace, o.MessageID)
	if _, err = f.admin.Exec(t.Context(), "UPDATE hp_delivery.outbox SET lease_until=clock_timestamp()-interval '1 second' WHERE message_id=$1", o.MessageID); err != nil {
		t.Fatal(err)
	}
	var deliveries atomic.Int64
	receiver := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || !bytes.Equal(body, encoded) || r.Method != http.MethodPost || r.URL.Path != applyPath ||
			len(r.TLS.VerifiedChains) == 0 || r.TLS.PeerCertificates[0].DNSNames[0] != "delivery.internal" {
			t.Error("invalid Processing delivery")
		}
		switch deliveries.Add(1) {
		case 1:
			panic(http.ErrAbortHandler)
		case 2:
			w.Header().Set("Retry-After", "360")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(202)
		json.NewEncoder(w).Encode(map[string]any{"message_id": o.MessageID, "disposition": "DUPLICATE"})
	}))
	receiver.TLS = &tls.Config{Certificates: []tls.Certificate{serverTLS}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: ca.roots, MinVersion: tls.VersionTLS13}
	receiver.StartTLS()
	defer receiver.Close()
	deliveryCert := ca.issue(t, "delivery.internal", x509.ExtKeyUsageClientAuth)
	pub, err := NewPublisher(f.worker, f.dir, allowGate(acceptGate), receiver.URL, ca.roots, deliveryCert)
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish(t.Context(), o.Workspace, o.MessageID, q.Epoch); !errors.Is(err, intake.ErrUnavailable) {
		t.Fatalf("lost ACK: %v", err)
	}
	if err := pub.Publish(t.Context(), o.Workspace, o.MessageID, q.Epoch); !errors.Is(err, intake.ErrNotFound) {
		t.Fatalf("backoff did not hold: %v", err)
	}
	makeDue := func() {
		t.Helper()
		if _, err := f.admin.Exec(t.Context(), "UPDATE hp_delivery.outbox SET next_attempt_at=clock_timestamp() WHERE message_id=$1", o.MessageID); err != nil {
			t.Fatal(err)
		}
	}
	makeDue()
	reopened, err := NewPostgres(connect(t, "HIT92_TEST_WORKER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	restart, err := NewPublisher(reopened, f.dir, allowGate(acceptGate), receiver.URL, ca.roots, deliveryCert)
	if err != nil {
		t.Fatal(err)
	}
	beforeRateLimit := time.Now()
	if err := restart.Publish(t.Context(), o.Workspace, o.MessageID, q.Epoch); !errors.Is(err, intake.ErrUnavailable) {
		t.Fatalf("Processing rate limit: %v", err)
	}
	var nextAttempt, indexedDue time.Time
	err = f.admin.QueryRow(t.Context(), `SELECT o.next_attempt_at,j.due_at
		FROM hp_delivery.outbox o JOIN hp_delivery.job_index j
		ON j.workspace_id=o.workspace_id AND j.job_id=o.message_id
		WHERE o.message_id=$1 AND o.state='PENDING'`, o.MessageID).Scan(&nextAttempt, &indexedDue)
	if err != nil || nextAttempt.Before(beforeRateLimit.Add(6*time.Minute-time.Microsecond)) || !nextAttempt.Equal(indexedDue) {
		t.Fatalf("Retry-After not persisted to outbox and job index: %v, %v, %v", nextAttempt, indexedDue, err)
	}
	if err := restart.Publish(t.Context(), o.Workspace, o.MessageID, q.Epoch); !errors.Is(err, intake.ErrNotFound) || deliveries.Load() != 2 {
		t.Fatalf("premature delivery during Retry-After: %v, posts=%d", err, deliveries.Load())
	}
	makeDue()
	if err = restart.Publish(t.Context(), o.Workspace, o.MessageID, q.Epoch); err != nil {
		t.Fatal(err)
	}
	var state string
	err = f.admin.QueryRow(t.Context(), "SELECT state FROM hp_delivery.outbox WHERE message_id=$1", o.MessageID).Scan(&state)
	if err != nil || state != "ACKED" || deliveries.Load() != 3 {
		t.Fatalf("outbox replay incorrect: %s %v", state, err)
	}
	next, err := service.QueryKnownTask(t.Context(), q)
	if err != nil || next.Observation == o.Observation || reads.Load() != 2 || next.EvidenceExpires == nil ||
		!next.EvidenceExpires.Equal(*o.EvidenceExpires) {
		t.Fatal("new observation changed identity or retention")
	}
	claim, err := reopened.claimObservation(t.Context(), next.Workspace, next.MessageID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.finishPublication(t.Context(), next.Workspace, claim, false, &claim.ref.Expires); err != nil {
		t.Fatal(err)
	}
	var held, stillScheduled bool
	err = f.admin.QueryRow(t.Context(), `SELECT o.state='HELD',
		EXISTS (SELECT 1 FROM hp_delivery.job_index j WHERE j.workspace_id=o.workspace_id AND j.job_id=o.message_id)
		FROM hp_delivery.outbox o WHERE o.message_id=$1`, next.MessageID).Scan(&held, &stillScheduled)
	if err != nil || !held || stillScheduled {
		t.Fatalf("expired retry window must hold without scheduling: held=%t scheduled=%t err=%v", held, stillScheduled, err)
	}
	t.Logf("native ID=%s knowledge=%s lineage=%s fence=%t; Processing deliveries=%d (same message/digest), Retry-After=360s persisted; outbox=%s", id, o.Knowledge, observedState, fence, deliveries.Load(), state)
}

func fenceClaimCrash(t *testing.T, ws, msg uuid.UUID) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestReadbackClaimProcess$")
	cmd.Env = append(os.Environ(), "HIT93_CRASH_WORKSPACE="+ws.String(), "HIT93_CRASH_MESSAGE="+msg.String())
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 43 {
		t.Fatalf("test crash process: %v %s", err, out)
	}
}

func TestReadbackClaimProcess(t *testing.T) {
	ws := os.Getenv("HIT93_CRASH_WORKSPACE")
	if ws == "" {
		t.Skip("used by the restart fixture")
	}
	p, err := NewPostgres(connect(t, "HIT92_TEST_WORKER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.claimObservation(t.Context(), uuid.MustParse(ws), uuid.MustParse(os.Getenv("HIT93_CRASH_MESSAGE"))); err != nil {
		t.Fatal(err)
	}
	os.Exit(43)
}

func TestReadbackFailures(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 50)
	_, a, q := knownFixture(t, f, fp)
	for _, tc := range []string{"mismatch", "content", "incomplete", "wrong-id", "403", "404", "429", "malformed", "oversize", "timeout", "denied", "new-epoch", "wrong-target", "cancel", "wrong-ca", "budget", "expired-file", "corrupt-file", "lost-file"} {
		t.Run(tc, func(t *testing.T) {
			var gets atomic.Int64
			target := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gets.Add(1)
				if r.Method != http.MethodGet {
					t.Error("a provider write was attempted")
				}
				switch tc {
				case "403", "404", "429":
					var n int
					fmt.Sscan(tc, &n)
					w.WriteHeader(n)
				case "malformed":
					io.WriteString(w, "{")
				case "oversize":
					w.Write(bytes.Repeat([]byte("x"), maxResponse+1))
				case "timeout":
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
				default:
					w.Write(targetJSON(t, a, "fixture-task", tc))
				}
			}))
			service := queryFixture(t, f, a, fp, target)
			query := q
			ctx := t.Context()
			var wantErr error
			switch tc {
			case "denied":
				service.authority = readAuth(func(context.Context, intake.Query) (Authorization, error) { return Authorization{}, intake.ErrDenied })
				wantErr = intake.ErrDenied
			case "new-epoch":
				query.Epoch++
				wantErr = intake.ErrDenied
			case "wrong-target":
				query.TaskID = "different-task"
				wantErr = intake.ErrConflict
			case "cancel":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				wantErr = intake.ErrUnavailable
			case "wrong-ca":
				target.tlsConfig.RootCAs = x509.NewCertPool()
				wantErr = intake.ErrUnavailable
			case "budget":
				if _, err := f.admin.Exec(ctx, "UPDATE hp_delivery.credential_budget SET limit_count=used_count WHERE token_fingerprint=$1", fp[:]); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_, _ = f.admin.Exec(context.Background(), "UPDATE hp_delivery.credential_budget SET limit_count=50 WHERE token_fingerprint=$1", fp[:])
				})
				wantErr = intake.ErrQuota
			case "expired-file", "corrupt-file":
				k, err := f.worker.known(ctx, q, a)
				if err != nil {
					t.Fatal(err)
				}
				if tc == "expired-file" {
					if _, err := f.admin.Exec(ctx, "UPDATE hp_delivery.intent_content SET expires_at=clock_timestamp()-interval '1 minute' WHERE attempt_id=$1", a.Attempt); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if _, err := f.admin.Exec(context.Background(), "UPDATE hp_delivery.intent_content SET expires_at=$2 WHERE attempt_id=$1", a.Attempt, k.ref.Expires); err != nil {
							t.Error(err)
						}
					})
				} else {
					filename := filepath.Join(f.root, k.ref.ID.String())
					original, err := os.ReadFile(filename)
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filename, []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := os.WriteFile(filename, original, 0600); err != nil {
							t.Error(err)
						}
					})
				}
				wantErr = intake.ErrUnavailable
			case "lost-file":
				k, err := f.worker.known(ctx, q, a)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Remove(filepath.Join(f.root, k.ref.ID.String())); err != nil {
					t.Fatal(err)
				}
				wantErr = intake.ErrUnavailable
			}
			o, err := service.QueryKnownTask(ctx, query)
			if wantErr != nil {
				if !errors.Is(err, wantErr) || gets.Load() != 0 {
					t.Fatalf("bad refusal: %v, GET count %d", err, gets.Load())
				}
				return
			}
			want := "INCOMPLETE"
			if tc == "mismatch" || tc == "content" {
				want = "RESULT_MISMATCH"
			}
			if err != nil || o.Knowledge != want || gets.Load() != 1 {
				t.Fatalf("expected %s received %+v %v", want, o, err)
			}
			if tc == "incomplete" && o.WorkspaceMatches != nil {
				t.Fatal("missing provider workspace was substituted")
			}
			if tc == "wrong-id" && o.EvidenceRef != nil {
				t.Fatal("foreign identity linked as evidence")
			}
		})
	}
}

func TestReadbackSharedBudgetAndSQLFailure(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 3)
	_, a, qa := knownFixture(t, f, fp)
	_, b, qb := knownFixture(t, f, fp)
	var reads atomic.Int64
	target := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reads.Add(1)
		w.Write(targetJSON(t, a, "fixture-task", ""))
	}))
	sa := queryFixture(t, f, a, fp, target)
	if ret, err := sa.QueryKnownTask(t.Context(), qa); err != nil || ret.Knowledge != "RESULT_VERIFIED" {
		t.Fatalf("first read failed: %v", err)
	}
	if _, err := queryFixture(t, f, b, fp, target).QueryKnownTask(t.Context(), qb); !errors.Is(err, intake.ErrQuota) || reads.Load() != 1 {
		t.Fatalf("shared budget did not hold second workspace: %v", err)
	}
	if _, err := f.admin.Exec(t.Context(), "UPDATE hp_delivery.credential_budget SET limit_count=10 WHERE token_fingerprint=$1", fp[:]); err != nil {
		t.Fatal(err)
	}
	ddl := fmt.Sprintf(`
		CREATE FUNCTION public.hit93_fail_observation() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.workspace_id='%s'::uuid THEN RAISE EXCEPTION 'fixture deferred failure'; END IF; RETURN NEW; END $$;
		CREATE CONSTRAINT TRIGGER hit93_fail_observation AFTER INSERT ON hp_delivery.outbox
		DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION public.hit93_fail_observation()`, b.Workspace)
	if _, err := f.admin.Exec(t.Context(), ddl); err != nil {
		t.Fatal(err)
	}
	targetB := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Error("provider write attempted")
		}
		w.Write(targetJSON(t, b, "fixture-task", ""))
	}))
	if ret, err := queryFixture(t, f, b, fp, targetB).QueryKnownTask(t.Context(), qb); !errors.Is(err, intake.ErrUnavailable) || ret.Observation != uuid.Nil {
		t.Fatalf("failed SQL commit returned observation: %v", err)
	}
	var observations, outbox, index int
	if err := f.admin.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM hp_delivery.effect_observations WHERE workspace_id=$1 AND knowledge<>'TARGET_ID_OBSERVED'),
		(SELECT count(*) FROM hp_delivery.outbox WHERE workspace_id=$1),
		(SELECT count(*) FROM hp_delivery.job_index WHERE workspace_id=$1)`, b.Workspace).Scan(&observations, &outbox, &index); err != nil || observations+outbox+index != 0 {
		t.Fatalf("failed commit left partial state: %d %d %d %v", observations, outbox, index, err)
	}
	t.Log("shared token budget held second workspace; deferred commit failure left no readback, outbox or job")
}
