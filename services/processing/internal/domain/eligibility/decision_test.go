package eligibility

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestFinancialDecisionTable(t *testing.T) {
	// Each oracle is fixed from RULE006/BP002/T004; never derived from output.
	cases := []struct {
		name, reason string
		s            Status
		change       func(*Policy, *Facts)
	}{
		{"account", "SOURCE_SCOPE", Ineligible, func(p *Policy, f *Facts) { f.AccountID = "foreign" }},
		{"connection", "SOURCE_SCOPE", Ineligible, func(p *Policy, f *Facts) { f.ConnectionID = "foreign" }},
		{"mode_scope", "SOURCE_SCOPE", Ineligible, func(p *Policy, f *Facts) { f.LiveMode = ptr(true) }},
		{"subscription", "MODE", Ineligible, func(p *Policy, f *Facts) { f.Mode = ptr("subscription") }},
		{"setup", "MODE", Ineligible, func(p *Policy, f *Facts) { f.Mode = ptr("setup") }},
		{"unpaid", "PAYMENT_STATUS", Ineligible, func(p *Policy, f *Facts) { f.PaymentStatus = ptr("unpaid") }},
		{"no_payment", "PAYMENT_STATUS", Ineligible, func(p *Policy, f *Facts) { f.PaymentStatus = ptr("no_payment_required") }},
		{"zero", "NON_POSITIVE_TOTAL", Ineligible, func(p *Policy, f *Facts) { f.AmountTotal = ptr(int64(0)) }},
		{"two_lines", "LINE_ITEM_COUNT", Ineligible, func(p *Policy, f *Facts) { f.LineItemCount = ptr(int64(2)) }},
		{"zero_lines", "LINE_ITEM_COUNT", Ineligible, func(p *Policy, f *Facts) { f.LineItemCount = ptr(int64(0)) }},
		{"zero_quantity", "QUANTITY", Ineligible, func(p *Policy, f *Facts) { f.Quantity = ptr(int64(0)) }},
		{"payment_intent", "PAYMENT_INTENT", Ineligible, func(p *Policy, f *Facts) { f.PaymentIntentStatus = ptr("other") }},
		{"charge_failed", "CHARGE_FAILED", Ineligible, func(p *Policy, f *Facts) { f.ChargeSucceeded = ptr(false) }},
		{"over_received", "AMOUNT_RECEIVED", Ineligible, func(p *Policy, f *Facts) { f.AmountReceived = ptr(int64(12001)) }},
		{"over_captured", "AMOUNT_CAPTURED", Ineligible, func(p *Policy, f *Facts) { f.AmountCaptured = ptr(int64(12001)) }},
		{"before_activation", "BEFORE_ACTIVATION", Ineligible, func(p *Policy, f *Facts) { f.SessionCreatedAt = ptr(p.ActivationAt.Add(-time.Nanosecond)) }},
		{"activation_equality", "FINANCIAL_ELIGIBILITY_ONLY", Valid, func(p *Policy, f *Facts) { f.SessionCreatedAt = ptr(p.ActivationAt) }},
		{"max_money", "FINANCIAL_ELIGIBILITY_ONLY", Valid, func(p *Policy, f *Facts) {
			f.AmountTotal = ptr(maxMoney)
			f.AmountCaptured = ptr(maxMoney)
			f.AmountReceived = ptr(maxMoney)
		}},
		{"too_large", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.AmountTotal = ptr(maxMoney + 1) }},
		{"negative_money", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.AmountRefunded = ptr(int64(-1)) }},
		{"negative_quantity", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.Quantity = ptr(int64(-1)) }},
		{"negative_count", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.LineItemCount = ptr(int64(-1)) }},
		{"enum", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.Mode = ptr("invented") }},
		{"payment_enum", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.PaymentStatus = ptr("success") }},
		{"intent_enum", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.PaymentIntentStatus = ptr("success") }},
		{"currency_length", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.Currency = ptr("US") }},
		{"currency_case", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.Currency = ptr("USD") }},
		{"empty_price", "INVALID_FACT", Incomplete, func(p *Policy, f *Facts) { f.PriceID = ptr("") }},
		{"pagination", "UNKNOWN_FACT", Incomplete, func(p *Policy, f *Facts) { f.LineItemsComplete = ptr(false) }},
		{"incomplete", "UNKNOWN_FACT", Incomplete, func(p *Policy, f *Facts) { f.Complete = ptr(false) }},
		{"unknown_mode", "UNKNOWN_FACT", Incomplete, func(p *Policy, f *Facts) { f.Mode = ptr("UNKNOWN") }},
		{"unknown_paid", "UNKNOWN_FACT", Incomplete, func(p *Policy, f *Facts) { f.PaymentStatus = ptr("UNKNOWN") }},
		{"unknown_intent", "UNKNOWN_FACT", Incomplete, func(p *Policy, f *Facts) { f.PaymentIntentStatus = ptr("UNKNOWN") }},
		{"empty_case", "INVALID_INPUT", Incomplete, func(p *Policy, f *Facts) { f.CaseID = "" }},
		{"empty_session", "INVALID_INPUT", Incomplete, func(p *Policy, f *Facts) { f.SessionID = "" }},
		{"unknown_config", "INVALID_INPUT", Incomplete, func(p *Policy, f *Facts) { p.ConfigID = "" }},
		{"old_evaluator", "INVALID_INPUT", Incomplete, func(p *Policy, f *Facts) { p.EvaluatorVersion = "old" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, f, n := example()
			c.change(&p, &f)
			assertResult(t, Evaluate(p, f, n), c.s, c.reason)
		})
	}
}
func TestEveryNullableFact(t *testing.T) {
	fields := []string{"SessionCreatedAt", "Mode", "PaymentStatus", "Currency", "AmountTotal", "LineItemsComplete", "LineItemCount", "PriceID", "Quantity", "PaymentIntentStatus", "AmountReceived", "ChargeSucceeded", "ChargeCaptured", "AmountCaptured", "AmountRefunded", "Disputed", "AllCurrenciesEqual", "Complete"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			p, f, n := example()
			v := reflect.ValueOf(&f).Elem().FieldByName(field)
			v.Set(reflect.Zero(v.Type()))
			assertResult(t, Evaluate(p, f, n), Incomplete, "UNKNOWN_FACT")
		})
	}
}
func TestProofTimeline(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Facts)
	}{
		{"zero_oldest", func(f *Facts) { f.OldestObservedAt = time.Time{} }},
		{"zero_completed", func(f *Facts) { f.CompletedAt = time.Time{} }},
		{"zero_expiry", func(f *Facts) { f.ValidUntil = time.Time{} }},
		{"completed_before_oldest", func(f *Facts) { f.CompletedAt = f.OldestObservedAt.Add(-time.Nanosecond) }},
		{"future_completed", func(f *Facts) { f.CompletedAt = f.OldestObservedAt.Add(6 * time.Second) }},
		{"collection_over_30", func(f *Facts) { f.OldestObservedAt = f.OldestObservedAt.Add(-31 * time.Second) }},
		{"expiry_at_completed", func(f *Facts) { f.ValidUntil = f.CompletedAt }},
		{"expiry_before_completed", func(f *Facts) { f.ValidUntil = f.CompletedAt.Add(-time.Nanosecond) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, f, n := example()
			c.change(&f)
			assertResult(t, Evaluate(p, f, n), RefreshRequired, "PROOF_TIMELINE")
		})
	}
	p, f, n := example()
	f.ValidUntil = f.OldestObservedAt.Add(5 * time.Second)
	assertResult(t, Evaluate(p, f, n), RefreshRequired, "PROOF_EXPIRED")
	p, f, n = example()
	f.SessionCreatedAt = ptr(n.Add(time.Nanosecond))
	assertResult(t, Evaluate(p, f, n), Incomplete, "INVALID_FACT")
}
func TestDeterminismAndOwnership(t *testing.T) {
	p, f, n := example()
	before, _ := json.Marshal(struct {
		P Policy
		F Facts
	}{p, f})
	want := Result{Valid, "FINANCIAL_ELIGIBILITY_ONLY"}
	for i := 0; i < 100; i++ {
		if got := Evaluate(p, f, n); got != want {
			t.Fatalf("iteration %d: %+v", i, got)
		}
	}
	after, _ := json.Marshal(struct {
		P Policy
		F Facts
	}{p, f})
	if string(before) != string(after) {
		t.Fatal("mutated input")
	}
}
