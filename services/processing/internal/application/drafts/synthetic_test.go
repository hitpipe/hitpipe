package drafts

import (
	"testing"
	"time"
)

func completeFixture() DraftConfig {
	keys := []string{"brief"}
	return DraftConfig{DraftID: "11111111-1111-4111-8111-111111111111", Revision: 1, Name: "draft", SourceConnectionID: ptr("22222222-2222-4222-8222-222222222222"), DestinationConnectionID: ptr("33333333-3333-4333-8333-333333333333"), Mode: ptr("TEST"), PriceAlias: ptr("price"), ListAlias: ptr("list"), AssigneeAlias: ptr("assignee"), ServiceLabel: ptr("Услуга"), TitlePattern: ptr("SERVICE_ORDER"), RequiredContextKeys: &keys}
}
func TestPureRehearsalUsesFinancialKernelWithoutEffects(t *testing.T) {
	c := completeFixture()
	for _, tc := range []struct {
		name string
		e    ScenarioExample
		want string
	}{{"paid", ScenarioExample{Paid: true, AmountMinor: 100, SyntheticSessionAlias: "example_paid"}, "WOULD_CONFIRM"}, {"unpaid", ScenarioExample{AmountMinor: 100, SyntheticSessionAlias: "example_unpaid"}, "WOULD_WAIT"}, {"nonpositive", ScenarioExample{Paid: true, AmountMinor: 0, SyntheticSessionAlias: "example_zero"}, "WOULD_EXCLUDE"}, {"duplicate_without_retained_outcome", ScenarioExample{Paid: true, AmountMinor: 100, Duplicate: true}, "WOULD_HOLD_UNKNOWN"}, {"missing", ScenarioExample{Paid: true, AmountMinor: 100, MissingContextKeys: []string{"brief"}}, "WOULD_WAIT"}} {
		t.Run(tc.name, func(t *testing.T) {
			if got := rehearse(c, tc.e, time.Now()); got != tc.want {
				t.Fatal(got, tc.want)
			}
		})
	}
	c.PriceAlias = nil
	if got := rehearse(c, ScenarioExample{Paid: true, AmountMinor: 100}, time.Now()); got != "WOULD_HOLD_UNKNOWN" {
		t.Fatal(got)
	}
}
