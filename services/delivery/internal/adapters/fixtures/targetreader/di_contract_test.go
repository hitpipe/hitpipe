package targetreader

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitpipe/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

const diBlock = "[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]"

type errorReader struct{}

func (errorReader) Read(context.Context, string) (readback.Observation, verifytarget.Provenance, error) {
	return readback.Observation{}, verifytarget.Provenance{}, errors.New("synthetic read failure")
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

	for _, tc := range []struct {
		name   string
		reader verifytarget.TargetReader
		want   readback.Outcome
	}{
		{"boundary", FileReader{Path: path}, readback.Match},
		{"error", errorReader{}, readback.Unknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := verifytarget.New(tc.reader).VerifyTarget(context.Background(), "target-1", expected)
			if got.Comparison.Outcome != tc.want {
				t.Fatalf("got %#v want %s", got, tc.want)
			}
		})
	}
}
