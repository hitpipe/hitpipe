package readback

import (
	"strings"
	"testing"
)

const testBlock = "[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\nContext: context\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]"

func expected() Expected {
	return Expected{Workspace: "Ops", List: "Delivery", Title: "Deploy é", Assignee: "dima", HandoffBlock: testBlock, Context: "context"}
}

func observed() Observation {
	e := expected()
	return Observation{TargetID: "target-1", Retrieved: true, Workspace: e.Workspace, List: e.List, Title: "Deploy é", Assignees: []string{e.Assignee, "extra"}, HandoffBlock: e.HandoffBlock, Context: e.Context}
}

func TestCompareMatchWithExtraAssignee(t *testing.T) {
	if got := Compare(expected(), observed()); got.Outcome != Match {
		t.Fatalf("got %#v", got)
	}
}

func TestCompareNormalizesNFCAndCRLF(t *testing.T) {
	o, e := observed(), expected()
	o.Context = "a\r\nb"
	e.Context = "a\nb"
	if got := Compare(e, o); got.Outcome != Match {
		t.Fatalf("got %#v", got)
	}
}

func TestCompareMismatchEachRequiredField(t *testing.T) {
	fields := []struct {
		name   string
		mutate func(*Observation)
		want   Outcome
	}{
		{"workspace", func(o *Observation) { o.Workspace = "Other" }, Mismatch},
		{"list", func(o *Observation) { o.List = "Other" }, Mismatch},
		{"title", func(o *Observation) { o.Title = "Other" }, Mismatch},
		{"assignee", func(o *Observation) { o.Assignees = []string{"other"} }, Mismatch},
		{"handoff", func(o *Observation) { o.HandoffBlock = testBlock + "x" }, Incomplete},
		{"context", func(o *Observation) { o.Context = "Other" }, Mismatch},
	}
	for _, tc := range fields {
		t.Run(tc.name, func(t *testing.T) {
			o := observed()
			tc.mutate(&o)
			if got := Compare(expected(), o); got.Outcome != tc.want {
				t.Fatalf("got %#v want %s", got, tc.want)
			}
		})
	}
}

func TestCompareIncompleteAndUnknownPrecedence(t *testing.T) {
	o := observed()
	o.Retrieved = false
	if got := Compare(expected(), o); got.Outcome != Unknown {
		t.Fatalf("got %#v", got)
	}
	o = observed()
	o.Title = ""
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("got %#v", got)
	}
	o = observed()
	o.HandoffBlock = "bad"
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("got %#v", got)
	}
}

func TestCompareMeaningfulWhitespaceMismatch(t *testing.T) {
	o := observed()
	o.Title = "Deploy  é"
	if got := Compare(expected(), o); got.Outcome != Mismatch {
		t.Fatalf("got %#v", got)
	}
}

func TestCompareBoundaries(t *testing.T) {
	o := observed()
	o.Context = strings.Repeat("x", maxContextBytes+1)
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("got %#v", got)
	}
	o = observed()
	o.HandoffBlock = strings.Repeat("x", maxBlockBytes+1)
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("got %#v", got)
	}
}

func TestCompareNoTaskIDShortcut(t *testing.T) {
	o := observed()
	o.Workspace = ""
	o.Context = ""
	o.HandoffBlock = ""
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("got %#v", got)
	}
}
