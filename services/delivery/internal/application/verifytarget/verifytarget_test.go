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
	provenance  Provenance
	err         error
	calls       int
}

func (f *fakeReader) Read(context.Context, string) (readback.Observation, Provenance, error) {
	f.calls++
	return f.observation, f.provenance, f.err
}

func TestVerifierUsesReaderAndPreservesProvenance(t *testing.T) {
	block := "[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]"
	expected := readback.Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignee: "dima", HandoffBlock: block, Context: "ctx"}
	reader := &fakeReader{observation: readback.Observation{TargetID: "target-1", Retrieved: true, Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignees: []string{"dima"}, HandoffBlock: block, Context: "ctx"}, provenance: Provenance{ObservedAt: time.Unix(10, 0), Source: "double"}}
	result := New(reader).VerifyTarget(context.Background(), "target-1", expected)
	if result.Comparison.Outcome != readback.Match || reader.calls != 1 || result.Provenance.Source != "double" {
		t.Fatalf("got %#v calls=%d", result, reader.calls)
	}
}

func TestVerifierDoesNotConvertReadErrorToNoEffect(t *testing.T) {
	reader := &fakeReader{err: errors.New("404")}
	result := New(reader).VerifyTarget(context.Background(), "target-1", readback.Expected{})
	if result.Comparison.Outcome != readback.Unknown {
		t.Fatalf("got %#v", result)
	}
}

func TestVerifierRequiresBoundEvidence(t *testing.T) {
	block := "[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]"
	expected := readback.Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignee: "dima", HandoffBlock: block, Context: "ctx"}
	base := readback.Observation{TargetID: "target-1", Retrieved: true, Workspace: "Ops", List: "Delivery", Title: "Deploy", Assignees: []string{"dima"}, HandoffBlock: block, Context: "ctx"}
	cases := []struct {
		name        string
		observation readback.Observation
		provenance  Provenance
		err         bool
		want        readback.Outcome
	}{
		{"normal", base, Provenance{ObservedAt: time.Unix(10, 0), Source: "double"}, false, readback.Match},
		{"boundary", func() readback.Observation { o := base; o.Context = "\r"; return o }(), Provenance{ObservedAt: time.Unix(10, 0), Source: "double"}, false, readback.Mismatch},
		{"error", readback.Observation{}, Provenance{}, true, readback.Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{observation: tc.observation, provenance: tc.provenance}
			if tc.err {
				reader.err = errors.New("read failed")
			}
			got := New(reader).VerifyTarget(context.Background(), "target-1", expected)
			if got.Comparison.Outcome != tc.want {
				t.Fatalf("got %#v want %s", got, tc.want)
			}
		})
	}
}
