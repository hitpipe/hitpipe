// Code generated from schema.json (pinned MVP OAS); DO NOT EDIT.
package sourcefacts

import "time"

type Request struct {
	CaseID        string    `json:"case_id"`
	ConnectionID  string    `json:"connection_id"`
	DeadlineAt    time.Time `json:"deadline_at"`
	ExpectedEpoch int64     `json:"expected_epoch"`
	SessionID     string    `json:"session_id"`
	WorkspaceID   string    `json:"workspace_id"`
}
type Facts struct {
	AllCurrenciesEqual  *bool      `json:"all_currencies_equal"`
	AmountCaptured      *int64     `json:"amount_captured"`
	AmountReceived      *int64     `json:"amount_received"`
	AmountRefunded      *int64     `json:"amount_refunded"`
	AmountTotal         *int64     `json:"amount_total"`
	CaseID              string     `json:"case_id"`
	ChargeCaptured      *bool      `json:"charge_captured"`
	ChargeSucceeded     *bool      `json:"charge_succeeded"`
	Complete            bool       `json:"complete"`
	CompletedAt         time.Time  `json:"completed_at"`
	ConnectionID        string     `json:"connection_id"`
	Currency            *string    `json:"currency"`
	Digest              string     `json:"digest"`
	Disputed            *bool      `json:"disputed"`
	FactsID             string     `json:"facts_id"`
	LineItemCount       *int64     `json:"line_item_count"`
	LineItemsComplete   bool       `json:"line_items_complete"`
	Livemode            bool       `json:"livemode"`
	Mode                *string    `json:"mode"`
	OldestObservedAt    time.Time  `json:"oldest_observed_at"`
	PaymentIntentStatus string     `json:"payment_intent_status"`
	PaymentStatus       *string    `json:"payment_status"`
	PriceID             *string    `json:"price_id"`
	Quantity            *int64     `json:"quantity"`
	SessionCreatedAt    *time.Time `json:"session_created_at"`
	SessionID           string     `json:"session_id"`
	StripeAccountID     string     `json:"stripe_account_id"`
	ValidUntil          time.Time  `json:"valid_until"`
}
