package postgres

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/readcases"
)

const imageControl = "/tmp/hit145-control"
const imageContent = "/var/lib/hitpipe/processing-content"

func writeImageSignal(t *testing.T, name string, value any) {
	t.Helper()
	path := filepath.Join(imageControl, name)
	if err := os.WriteFile(path+".tmp", marshalProcess(t, value), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func awaitImageSignal(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(imageControl, name)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("host image controller did not signal", name)
}

func imageHealth(t *testing.T, stand *processStand, wanted, revision string) drafts.Health {
	t.Helper()
	deadline := time.Now().Add(18 * time.Second)
	for time.Now().Before(deadline) {
		code, body, err := stand.call("ops.test", "9444", "GET", "/internal/v1/health/processing", nil, nil)
		var health drafts.Health
		if err == nil && code == 200 && draftjson.Validate("Health", body) == nil &&
			json.Unmarshal(body, &health) == nil && health.Status == wanted {
			if health.BuildRevision != revision {
				t.Fatal("unexpected build revision", health.BuildRevision)
			}
			t.Logf("OPS mTLS HTTP %d %s", code, bytes.TrimSpace(body))
			return health
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("Health did not reach", wanted)
	return drafts.Health{}
}

type imageState struct {
	Cases, Inbound, Outbound, Attempts int
	DeliveryState                      string
}

func awaitImageHistory(t *testing.T, stand *processStand, path string, headers map[string]string) []byte {
	t.Helper()
	// The integrated runtime has a case worker. Its controlled Intake answers
	// 503, so compare history only after the expected durable hold is recorded.
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		body := stand.must("bff.test", "8443", "GET", path, nil, headers, 200)
		var history readcases.HistoryView
		if err := json.Unmarshal(body, &history); err != nil {
			t.Fatal(err)
		}
		for _, item := range history.Items {
			if item.NewState == "WAITING_DATA" && item.ReasonCode == "READER_UNAVAILABLE" {
				t.Log("baseline history captured after the controlled Intake failure was durably recorded")
				return body
			}
		}
		time.Sleep(40 * time.Millisecond)
	}
	t.Fatal("case worker did not persist the expected Intake hold")
	return nil
}

func readImageState(t *testing.T, scope, jobScope, message string) imageState {
	t.Helper()
	db := connect(t)
	tx := begin(t, db, scope)
	defer tx.Rollback(ctx)
	var state imageState
	if e := tx.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM hp_processing.case_identity WHERE workspace_id=$1),
 (SELECT count(*) FROM hp_processing.inbox WHERE workspace_id=$1)`, scope).Scan(&state.Cases, &state.Inbound); e != nil {
		t.Fatal(e)
	}
	if e := tx.Rollback(ctx); e != nil {
		t.Fatal(e)
	}
	tx = begin(t, db, jobScope)
	defer tx.Rollback(ctx)
	if e := tx.QueryRow(ctx, `SELECT (SELECT count(*) FROM hp_processing.outbox WHERE workspace_id=$1),state,delivery_count
 FROM hp_processing.outbox WHERE workspace_id=$1 AND message_id=$2`, jobScope, message).Scan(&state.Outbound, &state.DeliveryState, &state.Attempts); e != nil {
		t.Fatal(e)
	}
	return state
}

// The Linux test driver lives beside PostgreSQL. The local host runs the real
// OCI image twice, without giving this driver or Processing a Docker socket.
func TestProcessingOCIImageOwnedStand(t *testing.T) {
	revision := os.Getenv("HIT145_SOURCE_SHA")
	if revision == "" || os.Getenv("HIT78_ISOLATED_PG") != "1" {
		t.Skip("requires the HIT-145 private image and PostgreSQL runner")
	}

	f := newDraftFixtureBase(t)
	job := prepared(t, []string{})
	files, err := contentfiles.New(imageContent)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = files.Close() })
	job.service.Content = files
	job.service.Repository = DecisionStore{DB: connect(t), Content: files}
	stand := createProcessStand(t, "", imageContent,
		map[string]string{f.workspace: "acct_controlled", job.request.WorkspaceID: "acct_synthetic"})

	writeImageSignal(t, "ready.json", map[string]string{"config": stand.config, "stand": stand.stand})
	imageHealth(t, stand, "CLOSED", revision)
	path := "/api/v1/workspaces/" + f.workspace
	own := map[string]string{"X-HitPipe-Session": stand.token}
	stand.must("bff.test", "8443", "GET", path+"/operations", nil, own, 503)
	stand.must("bff.test", "9444", "GET", "/internal/v1/health/processing", nil, nil, 403)
	noCertificate := stand.client("", false, stand.pki)
	if _, _, err = stand.send(noCertificate, "9444", "GET", "/internal/v1/health/processing", nil, nil); err == nil {
		t.Fatal("Health accepted a client without a certificate")
	}
	t.Log("Negative HTTP checks: CLOSED business 503, non-OPS Health 403, missing mTLS certificate refused")

	stand.iamOpen.Store(true)
	imageHealth(t, stand, "READY", revision)
	_, operation := stand.activate(f)
	input := intakeWire(t, f.workspace, func(m map[string]any) {
		m["stripe_account_id"] = "acct_controlled"
		m["source_created_at"] = operation.FirstActivationAt.Format(time.RFC3339Nano)
		m["first_received_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		m["expires_at"] = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	})
	receipt := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", input, nil, 202)
	var admitted struct {
		RecordID string `json:"record_id"`
	}
	if json.Unmarshal(receipt, &admitted) != nil || !drafts.UUID(admitted.RecordID) {
		t.Fatal("invalid admission receipt")
	}
	historyURL := path + "/cases/" + admitted.RecordID + "/history"
	history := awaitImageHistory(t, stand, historyURL, own)

	result, err := job.service.Prepare(ctx, job.request)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-stand.deliverySeen:
		if id != result.MessageID {
			t.Fatal("unexpected Delivery message")
		}
	case <-time.After(6 * time.Second):
		t.Fatal("immutable outbox did not reach the controlled peer")
	}
	waitProcessOutbox(t, sendFixture{f: job, message: result.MessageID}, "ACKED")
	before := readImageState(t, f.workspace, job.request.WorkspaceID, result.MessageID)
	if before != (imageState{Cases: 1, Inbound: 1, Outbound: 1, Attempts: 1, DeliveryState: "ACKED"}) {
		t.Fatal("wrong first durable state", before)
	}

	stand.iamStale.Store(true)
	writeImageSignal(t, "restart.json", map[string]bool{"restart": true})
	awaitImageSignal(t, "second.json")
	imageHealth(t, stand, "CLOSED", revision)
	stand.must("bff.test", "8443", "GET", historyURL, nil, own, 503)
	stand.iamStale.Store(false)
	imageHealth(t, stand, "READY", revision)
	stand.bootMu.Lock()
	newBoot := stand.old.BootID != "" && stand.current.BootID != stand.old.BootID
	stand.bootMu.Unlock()
	if !newBoot {
		t.Fatal("restart did not establish a fresh boot ID")
	}
	repeated := stand.must("intake.test", "8443", "POST", "/internal/v1/intake-signals", input, nil, 202)
	again := stand.must("bff.test", "8443", "GET", historyURL, nil, own, 200)
	// The view's signed cursor, if present, is part of the accepted history API.
	// Compare its structured history entries, rather than any transport bytes.
	var body, secondBody struct {
		Items []json.RawMessage `json:"items"`
	}
	if !bytes.Equal(receipt, repeated) {
		t.Fatalf("admission receipt changed across restart: before=%x after=%x", sha256.Sum256(receipt), sha256.Sum256(repeated))
	}
	if json.Unmarshal(history, &body) != nil || json.Unmarshal(again, &secondBody) != nil || len(body.Items) == 0 {
		t.Fatal("expected nonempty history before and after the restart")
	}
	if !reflect.DeepEqual(body.Items, secondBody.Items) {
		t.Fatalf("synthetic history changed across restart: before=%s after=%s", marshalProcess(t, body.Items), marshalProcess(t, secondBody.Items))
	}
	select {
	case <-stand.deliverySeen:
		t.Fatal("restart dispatched a second request")
	case <-time.After(900 * time.Millisecond):
	}
	after := readImageState(t, f.workspace, job.request.WorkspaceID, result.MessageID)
	if before != after || stand.deliveryCount.Load() != 1 {
		t.Fatal("restart changed durable state or repeated the external effect", before, after)
	}
	t.Logf("RESTART PASS: case=%s message=%s fresh_boot=%t history_items=%d receipt_sha256=%x durable=%+v delivery_calls=%d",
		admitted.RecordID, result.MessageID, newBoot, len(secondBody.Items),
		sha256.Sum256(repeated), after, stand.deliveryCount.Load())
}
