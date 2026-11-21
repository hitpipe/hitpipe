package runtime

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
)

func TestDiagnosticsBoundsAndOpaqueIDs(t *testing.T) {
	var out bytes.Buffer
	d := diagnostics{logger: slog.New(slog.NewJSONHandler(&out, nil))}
	c := postgres.CommittedObservation{WorkspaceID: "private-token", CaseID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		Age: 200 * time.Millisecond, Disposition: "APPLIED", State: "UNKNOWN"}
	for range 30 {
		d.observation(c)
		d.fault(contentUnavailable, "private-token", c.CaseID, "external-session")
	}
	// Move to the next interval without a sleeping clock dependency.
	d.observationWindow = time.Now().Add(-sampleInterval)
	d.observation(c)
	if bytes.Contains(out.Bytes(), []byte("private-token")) || bytes.Contains(out.Bytes(), []byte("external-session")) {
		t.Fatal("non-opaque values reached the log")
	}
	var observations, faults int
	for _, row := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		var e map[string]any
		if err := json.Unmarshal(row, &e); err != nil {
			t.Fatal(err)
		}
		if e["case_id"] != c.CaseID {
			t.Fatal("missing safe correlation ID", e)
		}
		if e["reason"] == "content_unavailable" {
			faults++
			continue
		}
		if e["observation_age_ms"] != float64(200) {
			t.Fatal("invalid measured value", e)
		}
		observations++
		if observations == 17 && e["skipped_previous_window"] != float64(14) {
			t.Fatal("sampling loss was not reported", e)
		}
	}
	if observations != 17 || faults != 1 {
		t.Fatal("diagnostic rate limit did not apply", observations, faults)
	}
}
