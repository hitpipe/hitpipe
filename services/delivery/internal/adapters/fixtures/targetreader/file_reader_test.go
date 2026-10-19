package targetreader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReaderLoadsSavedObservation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "observation.json")
	body := `{"target_id":"target-1","source":"fixture","observation":{"retrieved":true,"workspace":"Ops","list":"Delivery","title":"Deploy","assignees":["dima"],"handoff_block":"[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\nContext: ctx\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]","context":"ctx"},"observed_at":"2026-10-19T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	_, provenance, err := (FileReader{Path: path, Source: "fixture"}).Read(context.Background(), "target-1")
	if err != nil || provenance.Source != "fixture" {
		t.Fatalf("err=%v provenance=%#v", err, provenance)
	}
}
