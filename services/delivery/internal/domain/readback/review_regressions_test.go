package readback

import (
	"strings"
	"testing"
)

func reviewFixtureJSON() string {
	return `{"expected":{"workspace":"Ops","list":"Delivery","title":"Deploy","assignee":"dima","handoff_block":"[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]","context":"ctx"},"observation":{"target_id":"target-1","retrieved":true,"workspace":"Ops","list":"Delivery","title":"Deploy","assignees":["dima"],"handoff_block":"[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]","context":"ctx"}}`
}

func TestReviewJSONBoundaryRejectsAliasesAndMalformedUnicode(t *testing.T) {
	for name, raw := range map[string]string{
		"case alias":     strings.Replace(reviewFixtureJSON(), `"retrieved":true`, `"retrieved":false,"Retrieved":true`, 1),
		"lone surrogate": strings.ReplaceAll(reviewFixtureJSON(), `"ctx"`, `"\ud800"`),
		"invalid bytes":  strings.ReplaceAll(reviewFixtureJSON(), `"ctx"`, "\""+string([]byte{0xff})+"\""),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := EvaluateFixture([]byte(raw))
			if err == nil && result.Outcome == Match {
				t.Fatalf("malformed input matched: %#v", result)
			}
		})
	}
}

func TestReviewComparatorRejectsInvalidTitleAndRetrievalError(t *testing.T) {
	e, o := expected(), observed()
	e.Title, o.Title = string([]byte{0xff}), string([]byte{0xff})
	if result := Compare(e, o); result.Outcome == Match {
		t.Fatalf("invalid title matched: %#v", result)
	}
	o = observed()
	o.RetrievalError = "permission denied"
	if result := Compare(expected(), o); result.Outcome == Match {
		t.Fatalf("retrieval error matched: %#v", result)
	}
}

func TestReviewComparatorNormalizesLoneCRWithoutChangingSpaces(t *testing.T) {
	e, o := expected(), observed()
	e.Context, o.Context = "a\nb", "a\rb"
	if result := Compare(e, o); result.Outcome != Match {
		t.Fatalf("CR normalization failed: %#v", result)
	}
	o.Context = "a  b"
	e.Context = "a b"
	if result := Compare(e, o); result.Outcome != Mismatch {
		t.Fatalf("meaningful spaces were collapsed: %#v", result)
	}
}
