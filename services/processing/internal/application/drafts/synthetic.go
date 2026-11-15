package drafts

import (
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"time"
)

func ptr[T any](v T) *T { return &v }
func syntheticFacts(p eligibility.Policy, ex ScenarioExample, now time.Time) eligibility.Facts {
	payment := "unpaid"
	pi := "other"
	if ex.Paid {
		payment = "paid"
		pi = "succeeded"
	}
	return eligibility.Facts{CaseID: "synthetic_case", SessionID: ex.SyntheticSessionAlias, AccountID: p.AccountID, ConnectionID: p.ConnectionID, LiveMode: p.LiveMode, SessionCreatedAt: ptr(now.Add(-time.Minute)), Mode: ptr("payment"), PaymentStatus: ptr(payment), Currency: ptr("xxx"), AmountTotal: ptr(ex.AmountMinor), LineItemsComplete: ptr(true), LineItemCount: ptr(int64(1)), PriceID: ptr(p.PriceID), Quantity: ptr(int64(1)), PaymentIntentStatus: ptr(pi), AmountReceived: ptr(ex.AmountMinor), ChargeSucceeded: ptr(ex.Paid), ChargeCaptured: ptr(ex.Paid), AmountCaptured: ptr(ex.AmountMinor), AmountRefunded: ptr(int64(0)), Disputed: ptr(false), AllCurrenciesEqual: ptr(true), Complete: ptr(true), OldestObservedAt: now, CompletedAt: now, ValidUntil: now.Add(time.Second)}
}
