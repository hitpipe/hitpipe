package observations

import (
	"testing"
	"time"
)

func TestKnowledgeDecisionTableAndHistoricalConfirmation(t *testing.T) {
	now := time.Now().UTC()
	yes, no := true, false
	id, ref := "task_synthetic", "11111111-1111-4111-8111-111111111111"
	expiry := now.Add(time.Minute)
	base := TaskObservation{Knowledge: "RESULT_VERIFIED", TaskID: &id, Complete: true, WorkspaceMatches: &yes, ListMatches: &yes, AssigneeMatches: &yes, TitleMatches: &yes, MarkerMatches: &yes, ContextMatches: &yes, ObservedAt: now, EvidenceRef: &ref, EvidenceExpiresAt: &expiry}
	for _, tc := range []struct {
		name                        string
		mutate                      func(*TaskObservation)
		state, outcome, disposition string
	}{
		{"verified", func(o *TaskObservation) {}, "CONFIRMED", "RESULT_VERIFIED", "APPLIED"},
		{"ID only", func(o *TaskObservation) { o.Knowledge = "TARGET_ID_OBSERVED"; o.Complete = false }, "UNKNOWN", "TARGET_ID_KNOWN", "APPLIED"},
		{"possible", func(o *TaskObservation) { o.Knowledge = "POSSIBLE_EFFECT"; o.TaskID = nil }, "UNKNOWN", "POSSIBLE_EFFECT", "APPLIED"},
		{"incomplete", func(o *TaskObservation) { o.Knowledge = "INCOMPLETE"; o.TaskID = nil }, "UNKNOWN", "POSSIBLE_EFFECT", "APPLIED"},
		{"mismatch", func(o *TaskObservation) { o.Knowledge = "RESULT_MISMATCH"; o.ListMatches = &no }, "ATTENTION", "RESULT_MISMATCH", "APPLIED"},
		{"no effect", func(o *TaskObservation) {
			o.Knowledge = "KNOWN_NO_EFFECT"
			o.TaskID = nil
			o.WorkspaceMatches = nil
			o.ListMatches = nil
			o.AssigneeMatches = nil
			o.TitleMatches = nil
			o.MarkerMatches = nil
			o.ContextMatches = nil
		}, "ATTENTION", "NO_EFFECT", "APPLIED"},
		{"missing match", func(o *TaskObservation) { o.MarkerMatches = nil }, "WORKING", "UNESTABLISHED", "HELD"},
		{"one false", func(o *TaskObservation) { o.ContextMatches = &no }, "WORKING", "UNESTABLISHED", "HELD"},
		{"incomplete verification", func(o *TaskObservation) { o.Complete = false }, "WORKING", "UNESTABLISHED", "HELD"},
		{"expired proof", func(o *TaskObservation) { x := now.Add(-time.Second); o.EvidenceExpiresAt = &x }, "WORKING", "UNESTABLISHED", "HELD"},
		{"missing proof", func(o *TaskObservation) { o.EvidenceRef = nil }, "WORKING", "UNESTABLISHED", "HELD"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := base
			tc.mutate(&o)
			c := Decide(Snapshot{State: "WORKING", Outcome: "UNESTABLISHED"}, o, now)
			if c.State != tc.state || c.Outcome != tc.outcome || c.Disposition != tc.disposition {
				t.Fatal(c)
			}
		})
	}
	confirmed := Snapshot{State: "CONFIRMED", Outcome: "RESULT_VERIFIED", KnownTaskID: &id, LastConfirmedAt: &now, Fenced: true, LatestObservationAt: now.Add(-time.Second)}
	for _, kind := range []string{"INCOMPLETE", "POSSIBLE_EFFECT", "TARGET_ID_OBSERVED"} {
		o := base
		o.Knowledge = kind
		c := Decide(confirmed, o, now)
		if c.Outcome != "RESULT_VERIFIED" || c.LastConfirmedAt != &now || !c.Fenced || c.KnownTaskID == nil || c.State != "UNKNOWN" {
			t.Fatal("knowledge regressed", kind, c)
		}
	}
	o := base
	o.Knowledge = "RESULT_MISMATCH"
	o.TitleMatches = &no
	c := Decide(confirmed, o, now)
	if c.State != "ATTENTION" || c.Outcome != "RESULT_MISMATCH" || c.LastConfirmedAt != &now {
		t.Fatal(c)
	}
	o = base
	other := "different"
	o.TaskID = &other
	c = Decide(confirmed, o, now)
	if c.Disposition != "HELD" || *c.KnownTaskID != id || !c.Fenced {
		t.Fatal("identity changed", c)
	}
	confirmed.LatestObservationAt = now.Add(time.Second)
	c = Decide(confirmed, base, now)
	if c.State != "CONFIRMED" || c.Disposition != "HELD" {
		t.Fatal("late observation regressed", c)
	}
}
