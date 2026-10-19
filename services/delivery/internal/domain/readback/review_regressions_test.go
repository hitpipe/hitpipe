package readback

import (
	"strings"
	"testing"
)

func reviewFixtureJSON() string {
	return `{"expected":{"workspace":"Ops","list":"Delivery","title":"Deploy","assignee":"dima","handoff_block":"[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\nContext: ctx\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]","context":"ctx"},"observation":{"target_id":"target-1","retrieved":true,"workspace":"Ops","list":"Delivery","title":"Deploy","assignees":["dima"],"handoff_block":"[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]\nService: Deploy\nOrder: handoff-123\nContext: ctx\n[/HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]","context":"ctx"}}`
}

func TestReviewJSONBoundaryRejectsAliasesAndMalformedUnicode(t *testing.T) {
	for name, raw := range map[string]string{
		"case alias":      strings.Replace(reviewFixtureJSON(), `"retrieved":true`, `"retrieved":false,"Retrieved":true`, 1),
		"mixed alias":     strings.Replace(reviewFixtureJSON(), `"retrieved":true`, `"retrieved":false,"retRieved":true`, 1),
		"escaped alias":   strings.Replace(reviewFixtureJSON(), `"retrieved":true`, `"retrieved":false,"\u0052etrieved":true`, 1),
		"duplicate key":   strings.Replace(reviewFixtureJSON(), `"retrieved":true`, `"retrieved":true,"retrieved":true`, 1),
		"trailing object": reviewFixtureJSON() + `}`,
		"lone surrogate":  strings.ReplaceAll(reviewFixtureJSON(), `"ctx"`, `"\ud800"`),
		"invalid bytes":   strings.ReplaceAll(reviewFixtureJSON(), `"ctx"`, "\""+string([]byte{0xff})+"\""),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := EvaluateFixture([]byte(raw))
			if err == nil && result.Outcome == Match {
				t.Fatalf("malformed input matched: %#v", result)
			}
		})
	}
}

func TestReviewJSONBoundaryAcceptsValidUnicodeEscapes(t *testing.T) {
	for name, raw := range map[string]string{
		"bmp scalar":         strings.Replace(reviewFixtureJSON(), `"ctx"`, `"\ud000"`, 1),
		"supplementary pair": strings.Replace(reviewFixtureJSON(), `"ctx"`, `"\ud83d\ude00"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			fixture, err := ParseFixture([]byte(raw))
			if err != nil {
				t.Fatalf("valid Unicode rejected: %v", err)
			}
			if fixture.Expected.Context == "" {
				t.Fatal("decoded value was lost")
			}
		})
	}
}

func TestReviewComparatorRejectsControlsAndHandoffShape(t *testing.T) {
	for name, mutate := range map[string]func(*Observation){
		"NUL": func(o *Observation) { o.Title = "Deploy\x00" },
		"uppercase UUID": func(o *Observation) {
			o.HandoffBlock = strings.Replace(o.HandoffBlock, "000000000001", "00000000000A", 1)
		},
		"missing Context label": func(o *Observation) { o.HandoffBlock = strings.Replace(o.HandoffBlock, "\nContext: context", "", 1) },
		"nested marker": func(o *Observation) {
			o.HandoffBlock = strings.Replace(o.HandoffBlock, "Context: context", "Context: context\n[HITPIPE-HANDOFF:00000000-0000-4000-8000-000000000001:v1]", 1)
		},
		"wrong closing case": func(o *Observation) {
			o.HandoffBlock = strings.Replace(o.HandoffBlock, "000000000001:v1]", "000000000002:v1]", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := observed()
			mutate(&o)
			if got := Compare(expected(), o); got.Outcome == Match {
				t.Fatalf("invalid handoff/control matched: %#v", got)
			}
		})
	}
}

func TestReviewContextBudgetUsesEscapedJCSProjection(t *testing.T) {
	o := observed()
	o.Context = strings.Repeat(`"`, 20000)
	if got := Compare(expected(), o); got.Outcome != Incomplete {
		t.Fatalf("quote-heavy context was not bounded: %#v", got)
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
