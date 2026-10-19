package targetreader

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

const diBlock = "[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\nContext: ctx\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]"

type doubleReader struct {
	observation readback.Observation
	provenance  verifytarget.Provenance
	err         error
}

func (r doubleReader) Read(context.Context, string) (readback.Observation, verifytarget.Provenance, error) {
	return r.observation, r.provenance, r.err
}

func TestFileReaderAndDoubleShareVerifyTargetContract(t *testing.T) {
	expected := readback.Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignee: "dima", HandoffBlock: diBlock, Context: "ctx"}
	path := filepath.Join(t.TempDir(), "observation.json")
	body := `{"target_id":"target-1","source":"fixture-file","observation":{"retrieved":true,"workspace":"Ops","list":"Delivery","title":"Deploy","assignees":["dima"],"handoff_block":"` + strings.ReplaceAll(diBlock, "\n", `\n`) + `","context":"ctx"},"observed_at":"2026-10-19T12:00:00Z"}`
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	fileResult := verifytarget.New(FileReader{Path: path}).VerifyTarget(context.Background(), "target-1", expected)
	if fileResult.Comparison.Outcome != readback.Match {
		t.Fatalf("file normal: %#v", fileResult)
	}

	boundaryPath := filepath.Join(t.TempDir(), "boundary.json")
	boundaryContext := strings.Repeat("x", 1000)
	boundaryBlock := strings.Replace(diBlock, "Context: ctx", "Context: "+boundaryContext, 1)
	boundaryBody := `{"target_id":"target-1","source":"fixture-boundary","observation":{"retrieved":true,"workspace":"Ops","list":"Delivery","title":"Deploy","assignees":["dima"],"handoff_block":"` + strings.ReplaceAll(boundaryBlock, "\n", `\n`) + `","context":"` + boundaryContext + `"},"observed_at":"2026-10-19T12:00:00Z"}`
	if err := os.WriteFile(boundaryPath, []byte(boundaryBody), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		reader   verifytarget.TargetReader
		want     readback.Outcome
		expected readback.Expected
	}{
		{"file normal", FileReader{Path: path}, readback.Match, expected},
		{"file boundary quote-safe payload", FileReader{Path: boundaryPath}, readback.Match, readback.Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignee: "dima", HandoffBlock: boundaryBlock, Context: boundaryContext}},
		{"file error malformed saved observation", FileReader{Path: filepath.Join(t.TempDir(), "missing.json")}, readback.Unknown, expected},
		{"double normal", doubleReader{observation: readback.Observation{TargetID: "target-1", Retrieved: true, Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignees: []string{"dima"}, HandoffBlock: diBlock, Context: "ctx"}, provenance: verifytarget.Provenance{ObservedAt: time.Unix(10, 0), Source: "double"}}, readback.Match, expected},
		{"double boundary", doubleReader{observation: readback.Observation{TargetID: "target-1", Retrieved: true, Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignees: []string{"dima"}, HandoffBlock: boundaryBlock, Context: boundaryContext}, provenance: verifytarget.Provenance{ObservedAt: time.Unix(10, 0), Source: "double"}}, readback.Match, readback.Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignee: "dima", HandoffBlock: boundaryBlock, Context: boundaryContext}},
		{"double error", doubleReader{err: os.ErrNotExist}, readback.Unknown, expected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verifytarget.New(tc.reader).VerifyTarget(context.Background(), "target-1", tc.expected)
			if got.Comparison.Outcome != tc.want {
				t.Fatalf("got %#v want %s", got, tc.want)
			}
		})
	}
}
