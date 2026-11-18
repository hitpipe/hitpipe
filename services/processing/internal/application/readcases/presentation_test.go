package readcases

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func presentationRecord() Record {
	b, _ := json.Marshal(map[string]any{"service_label": "Каталожная услуга"})
	return Record{Summary: CaseSummary{State: "RECEIVED", OutcomeKnowledge: "UNESTABLISHED"}, Config: b, LatestReason: "SIGNAL_ADMITTED", RequiredKeys: []string{}, Context: []ContextField{}}
}
func TestLeadStepPriority(t *testing.T) {
	cases := []struct {
		state, outcome                    string
		fenced, complete, access, recheck bool
		priority                          int
	}{
		{"UNKNOWN", "RESULT_MISMATCH", true, false, true, true, 1},
		{"UNKNOWN", "RESULT_MISMATCH", true, false, true, false, 2},
		{"CONFIRMED", "POSSIBLE_EFFECT", false, false, true, false, 2},
		{"WORKING", "UNESTABLISHED", true, true, false, false, 2},
		{"CONFIRMED", "RESULT_MISMATCH", false, false, true, false, 3},
		{"EXCLUDED", "NO_EFFECT", false, false, true, false, 4},
		{"ATTENTION", "NO_EFFECT", false, false, true, false, 5},
		{"WAITING_DATA", "NO_EFFECT", false, true, true, false, 6},
		{"WAITING_DATA", "NO_EFFECT", false, true, false, false, 7},
		{"WORKING", "NO_EFFECT", false, true, false, false, 8},
		{"ATTENTION", "NO_EFFECT", false, true, false, false, 9},
	}
	for _, c := range cases {
		_, got := nextStep(c.state, c.outcome, c.fenced, c.complete, c.access, c.recheck)
		if got != c.priority {
			t.Fatalf("%+v got priority%d", c, got)
		}
	}
}
func TestPresentationAllStatesKnowledgeAndBounds(t *testing.T) {
	for state := range stateText {
		for _, outcome := range []string{"NO_EFFECT", "POSSIBLE_EFFECT", "TARGET_ID_KNOWN", "RESULT_VERIFIED", "RESULT_MISMATCH", "UNESTABLISHED"} {
			for _, fence := range []bool{false, true} {
				for _, missing := range []bool{false, true} {
					r := presentationRecord()
					r.Summary.State = state
					r.Summary.OutcomeKnowledge = outcome
					r.DispatchFenced = fence
					if missing {
						r.RequiredKeys = []string{"brief"}
					}
					r.LatestReason = "opaque secret=do-not-echo arbitrary/provider/payload"
					l, e := Project(r)
					if e != nil || l.OrderLabel != "Каталожная услуга" || l.Actions == nil || len(l.Actions) != 0 || utf8.RuneCountInString(l.NextAction) > 512 {
						t.Fatal(l, e)
					}
					if !strings.HasPrefix(l.NextAction, stateText[state]) || strings.Contains(l.NextAction, "do-not-echo") || !strings.Contains(l.NextAction, "не расшифрована") || !strings.Contains(l.NextAction, "не подключена") {
						t.Fatal(l.NextAction)
					}
					if state == "UNKNOWN" || outcome == "POSSIBLE_EFFECT" || fence {
						if !strings.Contains(l.NextAction, "Повторное создание задачи") {
							t.Fatal("lost no-create", l.NextAction)
						}
					}
					if outcome == "TARGET_ID_KNOWN" && !strings.Contains(l.NextAction, "подтверждения результата нет") {
						t.Fatal(l.NextAction)
					}
					if outcome == "RESULT_VERIFIED" && !strings.Contains(l.NextAction, "не текущее состояние ClickUp") {
						t.Fatal(l.NextAction)
					}
				}
			}
		}
	}
}
func TestPresentationLabelExactAndInvalidSource(t *testing.T) {
	for _, label := range []string{"Точная метка", strings.Repeat("я", 120), "e\u0301"} {
		r := presentationRecord()
		r.Config, _ = json.Marshal(map[string]any{"service_label": label})
		got, e := Project(r)
		if e != nil || got.OrderLabel != label {
			t.Fatal(got, e)
		}
	}
	for _, value := range []any{"", strings.Repeat("я", 121), "line\nbreak", "tab\t", true, nil} {
		r := presentationRecord()
		r.Config, _ = json.Marshal(map[string]any{"service_label": value})
		if _, e := Project(r); e != ErrUnavailable {
			t.Fatal(value, e)
		}
	}
	r := presentationRecord()
	r.Config = []byte(`{"other_label":"fallback forbidden"}`)
	if _, e := Project(r); e != ErrUnavailable {
		t.Fatal(e)
	}
}
func TestCommandKnowledgeNeverComesFromReadPermission(t *testing.T) {
	r := presentationRecord()
	r.Summary.State = "UNKNOWN"
	r.Summary.OutcomeKnowledge = "POSSIBLE_EFFECT"
	r.DispatchFenced = true
	id := "same-known-task"
	r.KnownTaskID = &id
	statuses := []struct {
		s       commandStatus
		want    string
		actions int
	}{
		{commandStatus{}, "не подключена", 0},
		{commandStatus{implemented: true, sameIdentity: true}, "не установлена", 0},
		{commandStatus{implemented: true, sameIdentity: true, authorityKnown: true, checksKnown: true}, "Прав на повторную проверку нет", 0},
		{commandStatus{implemented: true, sameIdentity: true, authorityKnown: true, authorized: true, checksKnown: true}, "Проверки повторного действия не выполнены", 0},
		{commandStatus{implemented: true, sameIdentity: true, authorityKnown: true, authorized: true, checksKnown: true, checksPassed: true}, "Доступна повторная проверка", 1},
	}
	for _, c := range statuses {
		got, e := project(r, c.s)
		if e != nil || len(got.Actions) != c.actions || !strings.Contains(got.NextAction, c.want) || !strings.Contains(got.NextAction, "Повторное создание задачи запрещено") {
			t.Fatal(got, e)
		}
	}
	got, e := Project(r)
	if e != nil || len(got.Actions) != 0 {
		t.Fatal("runtime advertised unimplemented recovery", got, e)
	}
	// Operation ACTIVE is deliberately absent from this pure read projection.
	// A future qualified manual RECHECK on PAUSED is not blocked by this formatter.
}
