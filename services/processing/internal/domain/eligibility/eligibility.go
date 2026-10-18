// Package eligibility evaluates financial facts under a pinned accepted policy.
// It has no IO, clock, authorization, case lifecycle or dispatch capability.
// Inputs are borrowed read-only for the call; results contain owned value strings.
// Callers must not mutate pointed facts concurrently. Unknown is never permission.
package eligibility

import "time"

// EvaluatorVersion identifies this policy's supported meaning, not a wire version.
const EvaluatorVersion = "paid-order-v1"
const maxMoney int64 = 9007199254740991

// Status is local financial knowledge; these values are not case or wire states.
type Status string

const (
	Valid           Status = "VALID"
	Ineligible      Status = "INELIGIBLE"
	Incomplete      Status = "INCOMPLETE"
	RefreshRequired Status = "REFRESH_REQUIRED"
)

// Result states only financial eligibility. VALID grants no external permission.
type Result struct {
	Status Status `json:"status"`
	Reason string `json:"reason"`
}

// Policy is a read-only projection of an already accepted immutable config.
// It cannot accept, activate or modify a configuration.
type Policy struct {
	ConfigID         string    `json:"config_id"`
	SchemaVersion    string    `json:"schema_version"`
	EvaluatorVersion string    `json:"evaluator_version"`
	AccountID        string    `json:"account_id"`
	ConnectionID     string    `json:"connection_id"`
	LiveMode         *bool     `json:"livemode"`
	PriceID          string    `json:"price_id"`
	ActivationAt     time.Time `json:"activation_at"`
}

// Facts are domain values projected from SourceFacts, not a generated wire DTO.
// Mandatory nullable fields preserve unknown nil separately from known false/zero.
// Future inbound adapters must validate canonical OAS/authenticity before mapping.
type Facts struct {
	CaseID              string     `json:"case_id"`
	SessionID           string     `json:"session_id"`
	AccountID           string     `json:"account_id"`
	ConnectionID        string     `json:"connection_id"`
	LiveMode            *bool      `json:"livemode"`
	SessionCreatedAt    *time.Time `json:"session_created_at"`
	Mode                *string    `json:"mode"`
	PaymentStatus       *string    `json:"payment_status"`
	Currency            *string    `json:"currency"`
	AmountTotal         *int64     `json:"amount_total"`
	LineItemsComplete   *bool      `json:"line_items_complete"`
	LineItemCount       *int64     `json:"line_item_count"`
	PriceID             *string    `json:"price_id"`
	Quantity            *int64     `json:"quantity"`
	PaymentIntentStatus *string    `json:"payment_intent_status"`
	AmountReceived      *int64     `json:"amount_received"`
	ChargeSucceeded     *bool      `json:"charge_succeeded"`
	ChargeCaptured      *bool      `json:"charge_captured"`
	AmountCaptured      *int64     `json:"amount_captured"`
	AmountRefunded      *int64     `json:"amount_refunded"`
	Disputed            *bool      `json:"disputed"`
	AllCurrenciesEqual  *bool      `json:"all_currencies_equal"`
	Complete            *bool      `json:"complete"`
	OldestObservedAt    time.Time  `json:"oldest_observed_at"`
	CompletedAt         time.Time  `json:"completed_at"`
	ValidUntil          time.Time  `json:"valid_until"`
}

func result(s Status, reason string) Result { return Result{s, reason} }

// ValidatePolicy reports unsupported/incomplete immutable policy without guessing.
func ValidatePolicy(p Policy) bool {
	return p.ConfigID != "" && p.SchemaVersion == "1.0" &&
		p.EvaluatorVersion == EvaluatorVersion && p.AccountID != "" &&
		p.ConnectionID != "" && p.LiveMode != nil && p.PriceID != "" && !p.ActivationAt.IsZero()
}

// Evaluate applies shape/version, identity, completeness, proof, then business
// predicates in that order. Expiry is exclusive. A fresh snapshot cannot prove
// atomic Stripe/ClickUp truth, current rights, lineage, or permission to dispatch.
func Evaluate(p Policy, f Facts, now time.Time) Result {
	if !ValidatePolicy(p) || now.IsZero() || f.CaseID == "" || f.SessionID == "" ||
		f.AccountID == "" || f.ConnectionID == "" || f.LiveMode == nil {
		return result(Incomplete, "INVALID_INPUT")
	}
	if f.AccountID != p.AccountID || f.ConnectionID != p.ConnectionID || *f.LiveMode != *p.LiveMode {
		return result(Ineligible, "SOURCE_SCOPE")
	}
	if f.Complete == nil || !*f.Complete || f.LineItemsComplete == nil || !*f.LineItemsComplete ||
		f.SessionCreatedAt == nil || f.SessionCreatedAt.IsZero() ||
		f.Mode == nil || *f.Mode == "UNKNOWN" || f.PaymentStatus == nil || *f.PaymentStatus == "UNKNOWN" ||
		f.Currency == nil || f.AmountTotal == nil || f.LineItemCount == nil || f.PriceID == nil ||
		f.Quantity == nil || f.PaymentIntentStatus == nil || *f.PaymentIntentStatus == "UNKNOWN" ||
		f.AmountReceived == nil || f.ChargeSucceeded == nil || f.ChargeCaptured == nil ||
		f.AmountCaptured == nil || f.AmountRefunded == nil || f.Disputed == nil || f.AllCurrenciesEqual == nil {
		return result(Incomplete, "UNKNOWN_FACT")
	}
	if !oneOf(*f.Mode, "payment", "subscription", "setup") ||
		!oneOf(*f.PaymentStatus, "paid", "unpaid", "no_payment_required") ||
		!oneOf(*f.PaymentIntentStatus, "succeeded", "other") || !currency(*f.Currency) ||
		*f.PriceID == "" || !money(*f.AmountTotal) || !money(*f.AmountReceived) ||
		!money(*f.AmountCaptured) || !money(*f.AmountRefunded) ||
		*f.LineItemCount < 0 || *f.Quantity < 0 {
		return result(Incomplete, "INVALID_FACT")
	}
	if f.OldestObservedAt.IsZero() || f.CompletedAt.IsZero() || f.ValidUntil.IsZero() ||
		f.CompletedAt.Before(f.OldestObservedAt) || f.CompletedAt.After(now) ||
		f.CompletedAt.Sub(f.OldestObservedAt) > 30*time.Second ||
		!f.ValidUntil.After(f.CompletedAt) ||
		f.ValidUntil.After(f.OldestObservedAt.Add(10*time.Second)) {
		return result(RefreshRequired, "PROOF_TIMELINE")
	}
	if !now.Before(f.ValidUntil) || now.Sub(f.OldestObservedAt) >= 10*time.Second {
		return result(RefreshRequired, "PROOF_EXPIRED")
	}
	if f.SessionCreatedAt.After(now) {
		return result(Incomplete, "INVALID_FACT")
	}
	checks := []struct {
		ok     bool
		reason string
	}{
		{!f.SessionCreatedAt.Before(p.ActivationAt), "BEFORE_ACTIVATION"},
		{*f.Mode == "payment", "MODE"},
		{*f.PaymentStatus == "paid", "PAYMENT_STATUS"},
		{*f.AmountTotal > 0, "NON_POSITIVE_TOTAL"},
		{*f.LineItemCount == 1, "LINE_ITEM_COUNT"},
		{*f.Quantity == 1, "QUANTITY"},
		{*f.PriceID == p.PriceID, "PRICE"},
		{*f.PaymentIntentStatus == "succeeded", "PAYMENT_INTENT"},
		{*f.ChargeSucceeded, "CHARGE_FAILED"},
		{*f.ChargeCaptured, "NOT_CAPTURED"},
		{*f.AmountReceived == *f.AmountTotal, "AMOUNT_RECEIVED"},
		{*f.AmountCaptured == *f.AmountTotal, "AMOUNT_CAPTURED"},
		{*f.AmountRefunded == 0, "REFUND"},
		{!*f.Disputed, "DISPUTE"},
		{*f.AllCurrenciesEqual, "CURRENCY"},
	}
	for _, c := range checks {
		if !c.ok {
			return result(Ineligible, c.reason)
		}
	}
	return result(Valid, "FINANCIAL_ELIGIBILITY_ONLY")
}
func money(v int64) bool { return v >= 0 && v <= maxMoney }
func currency(v string) bool {
	if len(v) != 3 {
		return false
	}
	for _, c := range v {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
func oneOf(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}
