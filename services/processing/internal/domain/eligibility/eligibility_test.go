package eligibility

import (
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func example() (Policy, Facts, time.Time) {
	t := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
	p := Policy{ConfigID: "config_synthetic", SchemaVersion: "1.0", EvaluatorVersion: EvaluatorVersion, AccountID: "acct_synthetic", ConnectionID: "connection_synthetic", LiveMode: ptr(false), PriceID: "price_fixed", ActivationAt: t.Add(-24 * time.Hour)}
	f := Facts{CaseID: "case_synthetic", SessionID: "cs_synthetic", AccountID: p.AccountID, ConnectionID: p.ConnectionID, LiveMode: ptr(false), SessionCreatedAt: ptr(t.Add(-time.Hour)), Mode: ptr("payment"), PaymentStatus: ptr("paid"), Currency: ptr("usd"), AmountTotal: ptr(int64(12000)), LineItemsComplete: ptr(true), LineItemCount: ptr(int64(1)), PriceID: ptr(p.PriceID), Quantity: ptr(int64(1)), PaymentIntentStatus: ptr("succeeded"), AmountReceived: ptr(int64(12000)), ChargeSucceeded: ptr(true), ChargeCaptured: ptr(true), AmountCaptured: ptr(int64(12000)), AmountRefunded: ptr(int64(0)), Disputed: ptr(false), AllCurrenciesEqual: ptr(true), Complete: ptr(true), OldestObservedAt: t, CompletedAt: t.Add(time.Second), ValidUntil: t.Add(10 * time.Second)}
	return p, f, t.Add(5 * time.Second)
}
func assertResult(t *testing.T, r Result, s Status, reason string) {
	t.Helper()
	if r.Status != s || r.Reason != reason {
		t.Fatalf("got %+v want %s/%s", r, s, reason)
	}
}
func TestNormal(t *testing.T) {
	p, f, n := example()
	assertResult(t, Evaluate(p, f, n), Valid, "FINANCIAL_ELIGIBILITY_ONLY")
}
func TestUnknownAndZero(t *testing.T) {
	p, f, n := example()
	f.AmountRefunded = nil
	assertResult(t, Evaluate(p, f, n), Incomplete, "UNKNOWN_FACT")
	f.AmountRefunded = ptr(int64(0))
	assertResult(t, Evaluate(p, f, n), Valid, "FINANCIAL_ELIGIBILITY_ONLY")
	f.Disputed = ptr(false)
	assertResult(t, Evaluate(p, f, n), Valid, "FINANCIAL_ELIGIBILITY_ONLY")
	f.Disputed = nil
	assertResult(t, Evaluate(p, f, n), Incomplete, "UNKNOWN_FACT")
}
func TestProofBoundary(t *testing.T) {
	p, f, _ := example()
	assertResult(t, Evaluate(p, f, f.ValidUntil.Add(-time.Nanosecond)), Valid, "FINANCIAL_ELIGIBILITY_ONLY")
	assertResult(t, Evaluate(p, f, f.ValidUntil), RefreshRequired, "PROOF_EXPIRED")
	assertResult(t, Evaluate(p, f, f.ValidUntil.Add(time.Nanosecond)), RefreshRequired, "PROOF_EXPIRED")
	f.ValidUntil = f.ValidUntil.Add(time.Nanosecond)
	assertResult(t, Evaluate(p, f, f.CompletedAt), RefreshRequired, "PROOF_TIMELINE")
}
func TestFinancialDenial(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*Facts)
	}{
		{"refund", "REFUND", func(f *Facts) { f.AmountRefunded = ptr(int64(1)) }},
		{"capture", "NOT_CAPTURED", func(f *Facts) { f.ChargeCaptured = ptr(false) }},
		{"received", "AMOUNT_RECEIVED", func(f *Facts) { f.AmountReceived = ptr(int64(11999)) }},
		{"partial_capture", "AMOUNT_CAPTURED", func(f *Facts) { f.AmountCaptured = ptr(int64(11999)) }},
		{"dispute", "DISPUTE", func(f *Facts) { f.Disputed = ptr(true) }},
		{"currency", "CURRENCY", func(f *Facts) { f.AllCurrenciesEqual = ptr(false) }},
		{"quantity", "QUANTITY", func(f *Facts) { f.Quantity = ptr(int64(2)) }},
		{"price", "PRICE", func(f *Facts) { f.PriceID = ptr("other") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, f, n := example()
			c.change(&f)
			assertResult(t, Evaluate(p, f, n), Ineligible, c.reason)
		})
	}
}
func TestPrecedence(t *testing.T) {
	p, f, n := example()
	f.AccountID = "foreign"
	f.AmountRefunded = nil
	n = f.ValidUntil
	assertResult(t, Evaluate(p, f, n), Ineligible, "SOURCE_SCOPE")
	f.AccountID = p.AccountID
	assertResult(t, Evaluate(p, f, n), Incomplete, "UNKNOWN_FACT")
	f.AmountRefunded = ptr(int64(1))
	assertResult(t, Evaluate(p, f, n), RefreshRequired, "PROOF_EXPIRED")
	n = n.Add(-time.Nanosecond)
	assertResult(t, Evaluate(p, f, n), Ineligible, "REFUND")
}
func TestInvalidPolicy(t *testing.T) {
	p, f, n := example()
	p.SchemaVersion = "2.0"
	assertResult(t, Evaluate(p, f, n), Incomplete, "INVALID_INPUT")
}
