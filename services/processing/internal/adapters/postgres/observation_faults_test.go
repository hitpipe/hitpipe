package postgres

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/observations"
	"github.com/hitpipe/hitpipe/services/processing/internal/testfixtures"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestObservationWireScopeAndEpisodeRejection(t *testing.T) {
	for _, mode := range []string{"digest", "extra", "duplicate", "case_variant_first", "case_variant_last", "required_omission", "forbidden_null", "enum", "version", "wrong_workspace", "foreign_case", "lineage", "episode", "old_episode", "future", "body_limit", "authority_header"} {
		t.Run(mode, func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, "RESULT_VERIFIED")
			want := 400
			headers := map[string]string{}
			switch mode {
			case "wrong_workspace":
				o.WorkspaceID, _ = newID()
				want = 403
			case "foreign_case":
				other := newObservation(t)
				o.CaseID = other.request.CaseID
				want = 404
			case "lineage":
				o.LineageID, _ = newID()
				want = 409
			case "episode":
				o.EpisodeID, _ = newID()
				want = 409
			case "old_episode":
				db := connect(t)
				tx := begin(t, db, f.request.WorkspaceID)
				newEpisode, _ := newID()
				_, e := tx.Exec(ctx, `INSERT INTO hp_processing.episodes(workspace_id,episode_id,case_id,started_at,deadline_at) VALUES($1,$2,$3,clock_timestamp(),clock_timestamp()+interval'60seconds')`, f.request.WorkspaceID, newEpisode, f.request.CaseID)
				if e != nil {
					t.Fatal(e)
				}
				tx.Commit(ctx)
				want = 412
			case "future":
				o.ObservedAt = time.Now().Add(time.Minute)
			case "authority_header":
				headers["X-HitPipe-Role"] = "OWNER"
			}
			raw := observationWire(t, o)
			switch mode {
			case "digest":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"complete":false`, 1))
			case "extra":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"complete":true,"EXTRA":0`, 1))
			case "duplicate":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"complete":false,"complete":true`, 1))
			case "case_variant_first":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"COMPLETE":false,"complete":true`, 1))
			case "case_variant_last":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"complete":true,"COMPLETE":false`, 1))
			case "forbidden_null":
				raw = []byte(strings.Replace(string(raw), `"complete":true`, `"complete":null`, 1))
			case "required_omission":
				var m map[string]any
				json.Unmarshal(raw, &m)
				delete(m, "evidence_ref")
				raw, _ = json.Marshal(m)
			case "enum":
				raw = []byte(strings.Replace(string(raw), "RESULT_VERIFIED", "UNSUPPORTED", 1))
			case "version":
				raw = []byte(strings.Replace(string(raw), `"schema_version":"1.0"`, `"schema_version":"2.0"`, 1))
			case "body_limit":
				raw = bytes.Repeat([]byte(" "), (1<<16)+1)
				want = 413
			}
			status, b := f.call(t, raw, headers)
			if status != want {
				t.Fatal(mode, status, want, string(b))
			}
			var request any
			json.Unmarshal(raw, &request)
			observationCapture(t, mode, "applyTaskObservation", "Error", status, request, b)
			if mode != "old_episode" {
				f.counts(t, 0)
			}
		})
	}
}
func TestObservationRealTLSAndCurrentAuthorityBoundary(t *testing.T) {
	for _, mode := range []string{"denied", "expired", "pending_revoke", "stale_epoch", "gate_closed", "non_invoking_gate", "response_denied", "wrong_service", "expired_client", "foreign_ca", "missing_client", "wrong_server_name"} {
		t.Run(mode, func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, "RESULT_VERIFIED")
			switch mode {
			case "denied":
				f.authority.denied = true
			case "expired":
				f.authority.grant.ExpiresAt = time.Now().Add(-time.Second)
			case "pending_revoke":
				f.authority.grant.RevocationPending = true
			case "stale_epoch":
				f.gate.epoch++
			case "gate_closed":
				f.gate.closed = true
			case "non_invoking_gate":
				f.gate.noCall = true
			case "response_denied":
				f.gate.denyPhase = "OBSERVATION_RESPONSE_START"
			default:
				tr := f.pki.Transport(t)
				tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, false)}
				switch mode {
				case "wrong_service":
					tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "bff.test", false, false)}
				case "expired_client":
					tr.TLSClientConfig.Certificates = []tls.Certificate{f.pki.Leaf(t, "delivery.test", false, true)}
				case "foreign_ca":
					tr = testfixtures.NewPKI(t).Transport(t)
				case "missing_client":
					tr.TLSClientConfig.Certificates = nil
				case "wrong_server_name":
					tr.TLSClientConfig.ServerName = "other.test"
				}
				f.client = &http.Client{Transport: tr, Timeout: time.Second}
				defer f.client.CloseIdleConnections()
			}
			if mode == "expired_client" || mode == "foreign_ca" || mode == "missing_client" || mode == "wrong_server_name" {
				req, _ := http.NewRequest("POST", f.url+"/internal/v1/delivery-observations", bytes.NewReader(observationWire(t, o)))
				res, e := f.client.Do(req)
				if e == nil {
					res.Body.Close()
					t.Fatal("bad TLS accepted")
				}
				f.counts(t, 0)
				return
			}
			b := f.apply(t, o, 403, "security_"+mode)
			if bytes.Contains(b, []byte(*o.TaskID)) {
				t.Fatal("business payload in denial")
			}
			if mode == "response_denied" {
				f.counts(t, 1)
				f.gate.mu.Lock()
				f.gate.denyPhase = ""
				f.gate.mu.Unlock()
				f.apply(t, o, 202, "committed_receipt_after_response_gate_recovery")
				f.counts(t, 1)
			} else {
				f.counts(t, 0)
			}
		})
	}
	f := newObservation(t)
	var authority *observationAuthority
	s := f.service
	s.Authority = authority
	if _, e := observationhttp.New(s, "delivery.test", 1024); e == nil {
		t.Fatal("typed nil authority accepted")
	}
	s = f.service
	s.Boundary = nil
	if _, e := observationhttp.New(s, "delivery.test", 1024); e == nil {
		t.Fatal("missing gate accepted")
	}
}
func TestObservationConcurrentSameMessageAndRevisionCAS(t *testing.T) {
	for _, same := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_message", false: "distinct_messages"}[same], func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, "RESULT_VERIFIED")
			raw := observationWire(t, o)
			var wg sync.WaitGroup
			codes := make(chan int, 6)
			bodies := make(chan []byte, 6)
			start := make(chan struct{})
			for i := 0; i < 6; i++ {
				body := raw
				if !same {
					x := f.observation(t, "RESULT_VERIFIED")
					body = observationWire(t, x)
				}
				wg.Add(1)
				go func(b []byte) {
					defer wg.Done()
					<-start
					status, out := f.call(t, b, nil)
					codes <- status
					bodies <- out
				}(body)
			}
			close(start)
			wg.Wait()
			close(codes)
			close(bodies)
			n := 0
			for code := range codes {
				if code == 202 {
					n++
				} else if code != 412 {
					t.Fatal("concurrent status", code)
				}
			}
			if same {
				if n != 6 {
					t.Fatal(n)
				}
				var original []byte
				for b := range bodies {
					if original == nil {
						original = b
					} else if !bytes.Equal(original, b) {
						t.Fatal("replay receipt changed")
					}
				}
				f.counts(t, 1)
			} else {
				if n < 1 {
					t.Fatal("no progress")
				}
				f.counts(t, n)
				_, _, revision, task, fence, _ := f.stored(t)
				if revision != int64(2+n) || task == nil || !fence {
					t.Fatal(revision, n, task, fence)
				}
			}
		})
	}
}

type observationAckDB struct {
	Beginner
	Crash bool
}
type observationAckTx struct {
	pgx.Tx
	write bool
	crash bool
}

func (d observationAckDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.Beginner.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &observationAckTx{Tx: tx, crash: d.Crash}, nil
}
func (tx *observationAckTx) Exec(c context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "UPDATE hp_processing.case_identity") {
		tx.write = true
	}
	return tx.Tx.Exec(c, q, args...)
}
func (tx *observationAckTx) Commit(c context.Context) error {
	if e := tx.Tx.Commit(c); e != nil {
		return e
	}
	if tx.write {
		if tx.crash {
			os.Exit(24)
		}
		return errors.New("controlled commit ACK loss")
	}
	return nil
}
func TestObservationCommitAckLossAndRealProcessCrash(t *testing.T) {
	if os.Getenv("HIT99_CRASH_HELPER") == "1" {
		files, e := contentfiles.New(os.Getenv("HIT99_ROOT"))
		if e != nil {
			t.Fatal(e)
		}
		raw, e := base64.StdEncoding.DecodeString(os.Getenv("HIT99_WIRE"))
		if e != nil {
			t.Fatal(e)
		}
		o, e := (observationjson.Codec{}).Decode(raw)
		if e != nil {
			t.Fatal(e)
		}
		grant := observations.Grant{WorkspaceID: o.WorkspaceID, PrincipalID: owner, OwnerID: owner, OperationID: op, ConnectionID: destination, Epoch: 7, ExpiresAt: time.Now().Add(time.Minute), Allowed: true}
		service := observations.Service{Repository: ObservationStore{observationAckDB{connect(t), true}, files, time.Hour}, Authority: &observationAuthority{grant: grant}, Boundary: &observationGate{epoch: 7}, Clock: testClock{time.Now}, Codec: observationjson.Codec{}}
		e = service.Execute(ctx, raw, func(observations.Receipt) error { t.Fatal("response emitted before crash"); return nil })
		t.Fatalf("commit/crash not reached %v", e)
	}
	for _, crash := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit_ack_loss", true: "real_process_exit24"}[crash], func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, "RESULT_VERIFIED")
			if crash {
				exe, _ := os.Executable()
				command := exec.Command(exe, "-test.run=^TestObservationCommitAckLossAndRealProcessCrash$")
				command.Env = append(os.Environ(), "HIT99_CRASH_HELPER=1", "HIT99_ROOT="+f.path, "HIT99_WIRE="+base64.StdEncoding.EncodeToString(observationWire(t, o)))
				out, e := command.CombinedOutput()
				var exit *exec.ExitError
				if !errors.As(e, &exit) || exit.ExitCode() != 24 {
					t.Fatalf("real committed crash absent %v %s", e, out)
				}
			} else {
				f.repo.DB = observationAckDB{f.repo.DB, false}
				f.service.Repository = f.repo
				f.start(t)
				f.apply(t, o, 503, "lost_commit_ack_not_positive_response")
			}
			files, e := contentfiles.New(f.path)
			if e != nil {
				t.Fatal(e)
			}
			defer files.Close()
			f.repo.DB = newObservationPool(t, 1)
			f.preparedFixture.files = files
			f.repo.Content = files
			f.service.Repository = f.repo
			f.start(t)
			f.apply(t, o, 202, "full_original_receipt_after_commit_crash_restart")
			f.counts(t, 1)
			state, outcome, revision, _, fence, _ := f.stored(t)
			if state != "CONFIRMED" || outcome != "RESULT_VERIFIED" || revision != 3 || !fence {
				t.Fatal(state, outcome, revision, fence)
			}
			f.readAPI(t, "restart_projection")
		})
	}
}
func TestObservationFileFaultRollbackAndContentCorruption(t *testing.T) {
	for _, step := range []string{"write", "file-fsync", "rename", "directory-fsync"} {
		t.Run(step, func(t *testing.T) {
			f := newObservation(t)
			injected := false
			f.files.Step = func(actual string) error {
				if actual == step {
					injected = true
					return errors.New("controlled observation file fault")
				}
				return nil
			}
			o := f.observation(t, "RESULT_VERIFIED")
			f.apply(t, o, 503, "file_fault_"+step)
			if !injected {
				t.Fatal("fault boundary not hit")
			}
			f.counts(t, 0)
			f.files.Step = nil
			f.apply(t, o, 202, "retry_after_file_fault_rollback")
			f.counts(t, 1)
		})
	}
	f := newObservation(t)
	db := connect(t)
	tx := begin(t, db, f.request.WorkspaceID)
	var ref string
	if e := tx.QueryRow(ctx, `SELECT detail_ref::text FROM hp_processing.history_content c JOIN hp_processing.safety_history h USING(workspace_id,entry_id) WHERE h.workspace_id=$1 AND h.case_id=$2 AND h.new_state='WORKING'`, f.request.WorkspaceID, f.request.CaseID).Scan(&ref); e != nil {
		t.Fatal(e)
	}
	tx.Rollback(ctx)
	if e := os.WriteFile(filepath.Join(f.path, ref), []byte("corrupt"), 0600); e != nil {
		t.Fatal(e)
	}
	f.apply(t, f.observation(t, "RESULT_VERIFIED"), 503, "corrupt_original_binding_history")
	f.counts(t, 0)
}
func TestObservationExpiryPurgeAndMinimalReadAfterRestart(t *testing.T) {
	f := newObservation(t)
	f.repo.HistoryRetention = 300 * time.Millisecond
	f.service.Repository = f.repo
	f.start(t)
	o := f.observation(t, "RESULT_VERIFIED")
	first := f.apply(t, o, 202, "short_owned_history")
	f.readAPI(t, "before_owned_history_expiry")
	time.Sleep(400 * time.Millisecond)
	if e := f.files.PurgeExpired(time.Now()); e != nil {
		t.Fatal(e)
	}
	files, e := contentfiles.New(f.path)
	if e != nil {
		t.Fatal(e)
	}
	defer files.Close()
	f.preparedFixture.files = files
	f.repo.Content = files
	f.service.Repository = f.repo
	f.start(t)
	again := f.apply(t, o, 202, "same_original_receipt_after_history_expiry_purge_restart")
	if !bytes.Equal(first, again) {
		t.Fatal("receipt/TTL renewed")
	}
	detail := f.readAPI(t, "expired_history_minimal_projection")
	if detail.Case.State != "CONFIRMED" || detail.Case.OutcomeKnowledge != "RESULT_VERIFIED" || detail.ObservationComplete {
		t.Fatal("purge lost identity or fabricated full detail", detail)
	}
	f.counts(t, 1)
	fresh := f.observation(t, "INCOMPLETE")
	fresh.TaskID = nil
	fresh.Complete = false
	f.apply(t, fresh, 202, "new_observation_after_prior_details_purged")
	state, _, revision, task, fence, confirmed := f.stored(t)
	if state != "UNKNOWN" || revision != 4 || task == nil || !fence || confirmed == nil {
		t.Fatal("retained safety lost")
	}
	f.readAPI(t, "new_unknown_after_expiry_projection")
}

// Abort the real PostgreSQL transaction immediately before its actual Commit.
// This is distinct from a committed transaction whose ACK was lost.
type observationAbortDB struct{ Beginner }
type observationAbortTx struct{ *observationAckTx }

func (d observationAbortDB) BeginTx(c context.Context, o pgx.TxOptions) (pgx.Tx, error) {
	tx, e := d.Beginner.BeginTx(c, o)
	if e != nil {
		return nil, e
	}
	return &observationAbortTx{&observationAckTx{Tx: tx}}, nil
}
func (tx *observationAbortTx) Commit(c context.Context) error {
	if tx.write {
		_, _ = tx.Tx.Exec(c, `SELECT 1/0`)
	}
	return tx.Tx.Commit(c)
}
func TestObservationActualSQLCommitRollbackAndCommitGate(t *testing.T) {
	for _, mode := range []string{"postgres_aborted_transaction", "commit_gate"} {
		t.Run(mode, func(t *testing.T) {
			f := newObservation(t)
			o := f.observation(t, "RESULT_VERIFIED")
			want := 503
			if mode == "commit_gate" {
				f.gate.denyPhase = "OBSERVATION_COMMIT"
				want = 403
			} else {
				f.repo.DB = observationAbortDB{f.repo.DB}
				f.service.Repository = f.repo
				f.start(t)
			}
			f.apply(t, o, want, mode)
			f.counts(t, 0)
			state, outcome, rev, task, fence, confirmed := f.stored(t)
			if state != "WORKING" || outcome != "UNESTABLISHED" || rev != 2 || task != nil || fence || confirmed != nil {
				t.Fatal("rollback leaked state", state, outcome, rev, task, fence, confirmed)
			}
			f.gate.mu.Lock()
			f.gate.denyPhase = ""
			f.gate.mu.Unlock()
			f.repo.DB = newObservationPool(t, 1)
			f.service.Repository = f.repo
			f.start(t)
			f.apply(t, o, 202, "same_message_after_actual_rollback_or_gate_recovery")
			f.counts(t, 1)
		})
	}
}
