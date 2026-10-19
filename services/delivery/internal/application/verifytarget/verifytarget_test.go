package verifytarget

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

type fakeReader struct {
	observation readback.Observation
	provenance Provenance
	err error
	calls int
}

func (f *fakeReader) Read(context.Context, string) (readback.Observation, Provenance, error) {
	f.calls++
	return f.observation, f.provenance, f.err
}

func TestVerifierUsesReaderAndPreservesProvenance(t *testing.T) {
	block := "<!-- hitpipe:handoff:start -->x<!-- hitpipe:handoff:end -->"
	expected := readback.Expected{Workspace:"Ops", List:"Delivery", Title:"Deploy", Assignee:"dima", HandoffBlock:block, Context:"ctx"}
	reader := &fakeReader{observation:readback.Observation{Retrieved:true, Workspace:"Ops", List:"Delivery", Title:"Deploy", Assignees:[]string{"dima"}, HandoffBlock:block, Context:"ctx"}, provenance:Provenance{ObservedAt:time.Unix(10, 0), Source:"double"}}
	result := New(reader).VerifyTarget(context.Background(), "target-1", expected)
	if result.Comparison.Outcome != readback.Match || reader.calls != 1 || result.Provenance.Source != "double" { t.Fatalf("got %#v calls=%d", result, reader.calls) }
}

func TestVerifierDoesNotConvertReadErrorToNoEffect(t *testing.T) {
	reader := &fakeReader{err:errors.New("404")}
	result := New(reader).VerifyTarget(context.Background(), "target-1", readback.Expected{})
	if result.Comparison.Outcome != readback.Unknown { t.Fatalf("got %#v", result) }
}
