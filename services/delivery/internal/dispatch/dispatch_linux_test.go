package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/intake"
	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type authorize func(context.Context, Job) (Authorization, error)

func (f authorize) Current(ctx context.Context, j Job) (Authorization, error) { return f(ctx, j) }

type allowGate func(context.Context, uuid.UUID, int64, func() error) error

func (g allowGate) WithOpen(ctx context.Context, w uuid.UUID, e int64, f func() error) error {
	return g(ctx, w, e, f)
}

type credentialSource func(context.Context, uuid.UUID, uuid.UUID, int64) (Credential, error)

func (f credentialSource) ForConnection(ctx context.Context, w, c uuid.UUID, e int64) (Credential, error) {
	return f(ctx, w, c, e)
}

type fixture struct {
	admin  *pgxpool.Pool
	app    *intake.Postgres
	worker *Postgres
	dir    *intake.Directory
	root   string
}

func connect(t *testing.T, env string) *pgxpool.Pool {
	t.Helper()
	p, err := pgxpool.New(t.Context(), os.Getenv(env))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func setup(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("HIT92_TEST_WORKER_DSN") == "" {
		if os.Getenv("HIT92_REQUIRE_POSTGRES") == "1" {
			t.Fatal("worker fixture not configured")
		}
		t.Skip("isolated worker fixture not requested")
	}
	admin := connect(t, "HIT91_TEST_ADMIN_DSN")
	var version int
	if err := admin.QueryRow(t.Context(), "SELECT current_setting('server_version_num')::int").Scan(&version); err != nil || version != 180007 {
		t.Fatalf("expected PostgreSQL 18.7: %d %v", version, err)
	}
	root := filepath.Join(t.TempDir(), "content")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := intake.OpenDirectory(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	app, err := intake.NewPostgres(connect(t, "HIT91_TEST_APP_DSN"), dir)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := NewPostgres(connect(t, "HIT92_TEST_WORKER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{admin, app, worker, dir, root}
}

const fixtureToken = "test-only-token"
const exactAssignee = "9007199254740993"
const fixtureTitle = "Example — order"
const fixtureBlock = "Canonical handoff\nContext: пример"

func (f *fixture) budget(t *testing.T, limit int) (fp [32]byte) {
	t.Helper()
	fp = sha256.Sum256([]byte(uuid.NewString()))
	if _, err := f.admin.Exec(t.Context(), "INSERT INTO hp_delivery.credential_budget VALUES ($1,clock_timestamp(),$2,0,clock_timestamp())", fp[:], limit); err != nil {
		t.Fatal(err)
	}
	return fp
}

func (f *fixture) prepare(t *testing.T, fp [32]byte, legacy bool) (Job, Authorization) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	fields := map[string]any{"schema_version": "1.0", "rendering_version": "handoff-block-v1", "connection_epoch": 1,
		"config_digest": "sha256:" + strings.Repeat("a", 64), "source_facts_digest": "sha256:" + strings.Repeat("b", 64),
		"facts_valid_until": now.Add(time.Minute), "deadline_at": now.Add(2 * time.Minute),
		"destination_workspace_id": "fixture-workspace", "target_list_id": "456", "assignee_id": exactAssignee,
		"title": fixtureTitle, "service_label": "Example", "source_session_id": "fixture-session", "order_reference": "order",
		"case_marker": "fixture-case", "rendered_handoff_block": fixtureBlock, "context": []any{}}
	for _, k := range []string{"message_id", "workspace_id", "case_id", "operation_id", "config_id", "lineage_id",
		"episode_id", "connection_id", "source_facts_id"} {
		fields[k] = uuid.New()
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		t.Fatal(err)
	}
	fields["message_digest"] = fmt.Sprintf("sha256:%x", sha256.Sum256(canonical))
	raw, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	i, err := intake.DecodeIntent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if legacy {
		var saved map[string]json.RawMessage
		if err := json.Unmarshal(i.Content, &saved); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"config_digest", "target_list_id", "assignee_id", "destination_workspace_id"} {
			delete(saved, k)
		}
		i.Content, err = json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.admin.Exec(t.Context(), "INSERT INTO hp_delivery.destination_connections(workspace_id,connection_id,external_workspace_id,security_epoch,token_fingerprint,health) VALUES($1,$2,$3,1,$4,'HEALTHY')", i.WorkspaceID, i.ConnectionID, i.DestinationWorkspace, fp[:]); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.app.Accept(t.Context(), i, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	job := Job{i.WorkspaceID, receipt.RecordID}
	a := Authorization{
		Workspace: job.Workspace, Attempt: job.Attempt, Case: i.CaseID, Connection: i.ConnectionID, Config: i.ConfigID,
		Lineage: i.LineageID, Episode: i.EpisodeID, Epoch: i.Epoch, ConfigDigest: i.ConfigDigest, FactsDigest: i.FactsDigest,
		FactsUntil: i.FactsUntil, Deadline: i.Deadline, TargetWorkspace: i.DestinationWorkspace, List: i.List, Assignee: i.Assignee,
	}
	return job, a
}

func acceptGate(ctx context.Context, _ uuid.UUID, _ int64, run func() error) error {
	if ctx.Err() != nil {
		return ErrHeld
	}
	return run()
}

func (f *fixture) newWorker(t *testing.T, client *ClickUp, a Authorization, fp [32]byte) *Worker {
	t.Helper()
	w, err := NewWorker(f.worker, f.dir, authorize(func(context.Context, Job) (Authorization, error) { return a, nil }),
		allowGate(acceptGate), credentialSource(func(context.Context, uuid.UUID, uuid.UUID, int64) (Credential, error) {
			return Credential{fixtureToken, fp}, nil
		}), client)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func fakeClickUp(t *testing.T, h http.Handler) *ClickUp {
	t.Helper()
	s := httptest.NewUnstartedServer(h)
	s.TLS = &tls.Config{MinVersion: tls.VersionTLS13}
	s.StartTLS()
	t.Cleanup(s.Close)
	roots := x509.NewCertPool()
	roots.AddCert(s.Certificate())
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	return clickUpAt(*u, roots)
}

func (f *fixture) state(t *testing.T, job Job) (state, knowledge, taskID string, fenced, started bool) {
	t.Helper()
	err := f.admin.QueryRow(t.Context(), `
		SELECT l.state,COALESCE(o.knowledge,''),COALESCE(l.known_task_id,''),l.ever_dispatch_fenced,a.network_start_at IS NOT NULL
		FROM hp_delivery.attempts a JOIN hp_delivery.effect_lineage l USING(workspace_id,lineage_id)
		LEFT JOIN hp_delivery.effect_observations o ON o.workspace_id=a.workspace_id AND o.attempt_id=a.attempt_id
		WHERE a.workspace_id=$1 AND a.attempt_id=$2`, job.Workspace, job.Attempt).Scan(&state, &knowledge, &taskID, &fenced, &started)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestPostgresSuccessAndSharedBudget(t *testing.T) {
	f := setup(t)
	fp := f.budget(t, 1)
	job, a := f.prepare(t, fp, false)
	var sends atomic.Int64
	client := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends.Add(1)
		body, err := io.ReadAll(r.Body)
		var got struct {
			Name        string
			Description string
			Assignees   []json.Number
		}
		if err != nil || json.Unmarshal(body, &got) != nil || r.URL.Path != "/api/v2/list/456/task" ||
			r.Method != http.MethodPost || r.Host == "" || r.ProtoMajor != 1 || !r.Close ||
			r.Header.Get("Authorization") != fixtureToken || r.Header.Get("Idempotency-Key") != "" ||
			got.Name != fixtureTitle || got.Description != fixtureBlock || len(got.Assignees) != 1 || got.Assignees[0] != exactAssignee {
			t.Error("unexpected HTTPS request")
		}
		var state string
		var fenced, started bool
		tx, err := f.admin.Begin(r.Context())
		if err != nil {
			t.Error("read fence: ", err)
			return
		}
		defer tx.Rollback(context.Background())
		err = tx.QueryRow(r.Context(), `SELECT l.state,l.ever_dispatch_fenced,a.network_start_at IS NOT NULL
			FROM hp_delivery.effect_lineage l JOIN hp_delivery.attempts a USING(workspace_id,lineage_id)
			WHERE a.workspace_id=$1 AND a.attempt_id=$2 AND l.lineage_id=$3 FOR UPDATE OF l,a NOWAIT`,
			a.Workspace, job.Attempt, a.Lineage).Scan(&state, &fenced, &started)
		if err != nil || state != "DISPATCHING" || !fenced || !started {
			t.Errorf("fence or row lock at HTTP boundary: %s %t %t %v", state, fenced, started, err)
		}
		io.WriteString(w, `{"id":"opaque-test-task"}`)
	}))
	worker := f.newWorker(t, client, a, fp)
	var wg sync.WaitGroup
	results := make(chan Result, 8)
	for i := 0; i < 8; i++ {
		wg.Go(func() {
			got, err := worker.Run(t.Context(), job)
			if err == nil {
				results <- got
			} else if !errors.Is(err, ErrFenced) {
				t.Errorf("unexpected concurrent result: %v", err)
			}
		})
	}
	wg.Wait()
	close(results)
	var count int
	for x := range results {
		count++
		if x.Disposition != TargetIDObserved || x.TaskID != "opaque-test-task" {
			t.Errorf("unexpected outcome: %+v", x)
		}
	}
	state, knowledge, id, fenced, started := f.state(t, job)
	if sends.Load() != 1 || count != 1 || state != "RESULT_ID_OBSERVED" || knowledge != "TARGET_ID_OBSERVED" ||
		id != "opaque-test-task" || !fenced || !started {
		t.Fatal("one ID-only observation expected")
	}
	other, b := f.prepare(t, fp, false)
	second := f.newWorker(t, client, b, fp)
	result, err := second.Run(t.Context(), other)
	if err != nil || result.Disposition != Ambiguous || sends.Load() != 1 {
		t.Fatal("shared budget did not hold the second workspace")
	}
	var used int
	if err := f.admin.QueryRow(t.Context(), "SELECT used_count FROM hp_delivery.credential_budget WHERE token_fingerprint=$1", fp[:]).Scan(&used); err != nil || used != 1 {
		t.Fatal("shared budget was charged more than once")
	}
	t.Logf("HTTPS sends=%d; state=%s; knowledge=%s; opaque task ID=%s; shared budget used=%d", sends.Load(), state, knowledge, id, used)
}

func TestPostgresUnknownAndDenied(t *testing.T) {
	f := setup(t)
	for _, name := range []string{"redirect", "401", "403", "429", "503", "malformed", "oversize", "abort", "timeout", "gate", "epoch", "revoked", "expired-facts", "deadline", "cancelled", "credential", "lost-content", "legacy", "crash"} {
		t.Run(name, func(t *testing.T) {
			fp := f.budget(t, 1)
			job, a := f.prepare(t, fp, name == "legacy")
			var sends atomic.Int64
			client := fakeClickUp(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sends.Add(1)
				switch name {
				case "redirect":
					w.Header().Set("Location", "https://unreachable.invalid/redirected")
					w.WriteHeader(307)
				case "401", "403", "429", "503":
					var status int
					fmt.Sscan(name, &status)
					w.WriteHeader(status)
				case "malformed":
					io.WriteString(w, `{"id":`)
				case "oversize":
					w.Write(bytes.Repeat([]byte(" "), maxResponse+1))
				case "abort":
					panic(http.ErrAbortHandler)
				case "timeout":
					_, _ = io.Copy(io.Discard, r.Body)
					select {
					case <-r.Context().Done():
					case <-time.After(9 * time.Second):
					}
				default:
					t.Error("unexpected provider request")
				}
			}))
			w := f.newWorker(t, client, a, fp)
			ctx := t.Context()
			switch name {
			case "gate":
				w.gate = allowGate(func(context.Context, uuid.UUID, int64, func() error) error { return ErrHeld })
			case "epoch":
				if _, err := f.admin.Exec(ctx, "UPDATE hp_delivery.destination_connections SET security_epoch=2 WHERE workspace_id=$1", job.Workspace); err != nil {
					t.Fatal(err)
				}
			case "revoked":
				if _, err := f.admin.Exec(ctx, "UPDATE hp_delivery.destination_connections SET revoked_at=clock_timestamp() WHERE workspace_id=$1", job.Workspace); err != nil {
					t.Fatal(err)
				}
			case "expired-facts", "deadline":
				if name == "deadline" {
					a.Deadline = time.Now().Add(-time.Minute)
				} else {
					a.FactsUntil = time.Now().Add(-time.Minute)
				}
				w = f.newWorker(t, client, a, fp)
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "credential":
				w.credentials = credentialSource(func(context.Context, uuid.UUID, uuid.UUID, int64) (Credential, error) { return Credential{}, ErrHeld })
			case "lost-content":
				rec, err := f.worker.load(ctx, job)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(f.root, rec.ref.ID.String())); err != nil {
					t.Fatal(err)
				}
			case "crash":
				fenceInCrashedProcess(t, job)
			}
			result, err := w.Run(ctx, job)
			state, knowledge, id, fenced, started := f.state(t, job)
			switch name {
			case "redirect", "401", "403", "429", "503", "malformed", "oversize", "abort", "timeout":
				if err != nil || result.Disposition != Ambiguous || sends.Load() != 1 || state != "AMBIGUOUS" ||
					knowledge != "POSSIBLE_EFFECT" || id != "" || !fenced || !started {
					t.Fatalf("not a fenced unknown outcome: %v %s %s", err, state, knowledge)
				}
				restart := f.newWorker(t, client, a, fp)
				if _, err := restart.Run(t.Context(), job); !errors.Is(err, ErrFenced) || sends.Load() != 1 {
					t.Fatal("restart repeated create")
				}
				if name == "abort" {
					t.Logf("lost response: state=%s, knowledge=%s, initial sends=1, repeated sends=0", state, knowledge)
				}
			case "crash":
				if !errors.Is(err, ErrFenced) || sends.Load() != 0 || state != "DISPATCHING" || !fenced || started {
					t.Fatal("crash lost its durable fence")
				}
				t.Log("process exited after SQL fence; restarted worker sent 0 requests")
			default:
				if !errors.Is(err, ErrHeld) || sends.Load() != 0 || state != "PREPARED" || fenced || started {
					t.Fatalf("protected request was not held: %v %s", err, state)
				}
			}
		})
	}
}

func fenceInCrashedProcess(t *testing.T, job Job) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), binary, "-test.run=^TestFenceCrashProcess$")
	cmd.Env = append(os.Environ(), "HIT92_CRASH_WORKSPACE="+job.Workspace.String(), "HIT92_CRASH_ATTEMPT="+job.Attempt.String())
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 42 {
		t.Fatalf("fence process failed: %v %s", err, out)
	}
}

func TestFenceCrashProcess(t *testing.T) {
	workspace := os.Getenv("HIT92_CRASH_WORKSPACE")
	if workspace == "" {
		t.Skip("used by the restart fixture")
	}
	job := Job{uuid.MustParse(workspace), uuid.MustParse(os.Getenv("HIT92_CRASH_ATTEMPT"))}
	db, err := NewPostgres(connect(t, "HIT92_TEST_WORKER_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := db.load(t.Context(), job)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.fence(t.Context(), rec); err != nil {
		t.Fatal(err)
	}
	os.Exit(42)
}
