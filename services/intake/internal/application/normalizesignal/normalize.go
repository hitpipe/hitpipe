// Package normalizesignal transforms authenticated bytes into the pinned minimal
// wire signal. It neither persists nor acknowledges a delivery.
package normalizesignal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/intake/internal/application/authenticatewebhook"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/stripesignature"
)

const MaxBodyBytes = 256 * 1024

// Binding must come from the endpoint owner's checked installation, not from
// headers or JSON. Nil Livemode and empty APIVersion are unknown, never defaults.
// ConnectedAccount requires Event.account; DirectAccount requires it absent.
type AccountScope string

const (
	DirectAccount    AccountScope = "DIRECT"
	ConnectedAccount AccountScope = "CONNECTED"
)

type Binding struct {
	InstallationID  string
	WorkspaceID     string
	ConnectionID    string
	StripeAccountID string
	Livemode        *bool
	APIVersion      string
	AccountScope    AccountScope
}

// Receipt is explicit caller-owned identity/first-age metadata. On redelivery the
// owner must supply the SAME saved values. This package has no dedup/storage port
// and cannot establish that they were saved or that the binding is current.
type Receipt struct {
	MessageID       string
	ReceiptID       string
	FirstReceivedAt time.Time
	ExpiresAt       time.Time
}
type Input struct {
	RawBody   []byte
	Signature string
	Binding   Binding
	Receipt   Receipt
}

// Signal mirrors NormalizedSignal at the exact pinned OpenAPI revision.
// No raw body, metadata, contact, brief or payment decision is retained.
type Signal struct {
	MessageID       string `json:"message_id"`
	MessageDigest   string `json:"message_digest"`
	SchemaVersion   string `json:"schema_version"`
	WorkspaceID     string `json:"workspace_id"`
	ConnectionID    string `json:"connection_id"`
	StripeAccountID string `json:"stripe_account_id"`
	Livemode        bool   `json:"livemode"`
	SessionID       string `json:"session_id"`
	EventID         string `json:"event_id"`
	EventType       string `json:"event_type"`
	SourceCreatedAt string `json:"source_created_at"`
	FirstReceivedAt string `json:"first_received_at"`
	ExpiresAt       string `json:"expires_at"`
	ReceiptID       string `json:"receipt_id"`
}
type Consumer struct {
	auth  authenticatewebhook.Consumer
	clock authenticatewebhook.Clock
	ready bool
}

func New(secrets authenticatewebhook.SecretResolver, clock authenticatewebhook.Clock) (Consumer, error) {
	if nilPort(secrets) || nilPort(clock) {
		return Consumer{}, errors.New("required_port")
	}
	a, err := authenticatewebhook.New(secrets, clock)
	return Consumer{auth: a, clock: clock, ready: err == nil}, err
}

// Normalize always authenticates each bounded delivery before parsing, even a
// repeated event. Errors are bounded constants without input or dependency text.
func (c Consumer) Normalize(ctx context.Context, in Input) (*Signal, error) {
	if !c.ready {
		return nil, errors.New("unconfigured")
	}
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	if len(in.RawBody) == 0 || len(in.RawBody) > MaxBodyBytes || len(in.Signature) > 2048 {
		return nil, errors.New("input_size")
	}
	// Own one snapshot across authentication/parsing. Callers must not mutate input
	// concurrently while this initial copy is being made.
	raw := append([]byte(nil), in.RawBody...)
	a := c.auth.Authenticate(ctx, authenticatewebhook.Input{InstallationID: in.Binding.InstallationID, RawBody: raw, Signature: in.Signature})
	if a.Webhook == nil || a.Outcome.Status != stripesignature.Verified {
		return nil, errors.New("authentication_failed")
	}
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	b, r := in.Binding, in.Receipt
	now, clockErr := c.clock.NowUnix(ctx)
	if clockErr != nil || now <= 0 || now > 253402300799 {
		return nil, errors.New("clock_unavailable")
	}
	observed := time.Unix(now, 0)
	if !uuid(b.WorkspaceID, false) || !uuid(b.ConnectionID, false) || !opaque(b.StripeAccountID) || b.Livemode == nil || b.APIVersion == "" || (b.AccountScope != DirectAccount && b.AccountScope != ConnectedAccount) {
		return nil, errors.New("binding_unknown")
	}
	if !uuid(r.MessageID, true) || !uuid(r.ReceiptID, false) || !validTime(r.FirstReceivedAt) || !validTime(r.ExpiresAt) || !r.ExpiresAt.After(r.FirstReceivedAt) || r.ExpiresAt.Sub(r.FirstReceivedAt) > 72*time.Hour || r.FirstReceivedAt.After(observed) || !r.ExpiresAt.After(observed) {
		return nil, errors.New("receipt_invalid")
	}
	// Pinned RFC8785 implementation rejects duplicate keys, invalid UTF-8,
	// unpaired surrogates, trailing JSON and nonfinite numbers before projection.
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return nil, errors.New("event_shape")
	}
	if _, err := jcs.Transform(raw); err != nil {
		return nil, errors.New("event_shape")
	}
	event, err := object(raw)
	if err != nil {
		return nil, err
	}
	if !allowed(event, eventFields) {
		return nil, errors.New("event_fields")
	}
	id, ok := textField(event, "id")
	if !ok || !opaque(id) {
		return nil, errors.New("event_id")
	}
	kind, ok := textField(event, "object")
	if !ok || kind != "event" {
		return nil, errors.New("event_shape")
	}
	version, ok := textField(event, "api_version")
	if !ok || version != b.APIVersion {
		return nil, errors.New("api_version")
	}
	eventType, ok := textField(event, "type")
	if !ok || !supported(eventType) {
		return nil, errors.New("event_type")
	}
	mode, ok := boolField(event, "livemode")
	if !ok || mode != *b.Livemode {
		return nil, errors.New("event_context")
	}
	account, present := event["account"]
	if b.AccountScope == ConnectedAccount {
		var value string
		if !present || json.Unmarshal(account, &value) != nil || value != b.StripeAccountID {
			return nil, errors.New("event_context")
		}
	} else if present {
		return nil, errors.New("event_context")
	}
	// Organization context is not an account binding supported by this boundary.
	if v, exists := event["context"]; exists && string(v) != "null" {
		return nil, errors.New("event_context")
	}
	if _, ok := unixField(event, "created"); !ok {
		return nil, errors.New("event_created")
	}
	data, err := object(event["data"])
	if err != nil || !allowed(data, "object previous_attributes") {
		return nil, errors.New("event_shape")
	}
	session, err := object(data["object"])
	if err != nil || !allowed(session, sessionFields) {
		return nil, errors.New("session_shape")
	}
	objectType, ok := textField(session, "object")
	if !ok || objectType != "checkout.session" {
		return nil, errors.New("session_shape")
	}
	sessionID, ok := textField(session, "id")
	if !ok || !opaque(sessionID) {
		return nil, errors.New("session_id")
	}
	sessionMode, ok := boolField(session, "livemode")
	if !ok || sessionMode != mode {
		return nil, errors.New("session_context")
	}
	// HIT-78 comment 12171: ONLY Session.created, never Event.created/receipt time.
	created, ok := unixField(session, "created")
	if !ok {
		return nil, errors.New("session_created")
	}
	s := &Signal{MessageID: r.MessageID, SchemaVersion: "1.0", WorkspaceID: b.WorkspaceID, ConnectionID: b.ConnectionID, StripeAccountID: b.StripeAccountID, Livemode: mode, SessionID: sessionID, EventID: id, EventType: eventType, SourceCreatedAt: time.Unix(created, 0).UTC().Format(time.RFC3339), FirstReceivedAt: r.FirstReceivedAt.UTC().Format(time.RFC3339Nano), ExpiresAt: r.ExpiresAt.UTC().Format(time.RFC3339Nano), ReceiptID: r.ReceiptID}
	// Omit the digest property entirely, not an empty digest string.
	encoded, err := json.Marshal(s)
	if err != nil {
		return nil, errors.New("signal_encoding")
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(encoded, &payload) != nil {
		return nil, errors.New("signal_encoding")
	}
	delete(payload, "message_digest")
	encoded, err = json.Marshal(payload)
	if err != nil {
		return nil, errors.New("signal_encoding")
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return nil, errors.New("signal_encoding")
	}
	digest := sha256.Sum256(canonical)
	s.MessageDigest = "sha256:" + hex.EncodeToString(digest[:])
	if ctx.Err() != nil {
		return nil, errors.New("cancelled")
	}
	return s, nil
}
func supported(s string) bool {
	return s == "checkout.session.completed" || s == "checkout.session.async_payment_succeeded" || s == "checkout.session.async_payment_failed"
}
func object(raw []byte) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &m) != nil || m == nil {
		return nil, errors.New("event_shape")
	}
	return m, nil
}
func textField(m map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := m[key]
	var v string
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &v) != nil {
		return "", false
	}
	return v, true
}
func boolField(m map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := m[key]
	var v bool
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &v) != nil {
		return false, false
	}
	return v, true
}
func unixField(m map[string]json.RawMessage, key string) (int64, bool) {
	raw, ok := m[key]
	var v int64
	if !ok || string(raw) == "null" || json.Unmarshal(raw, &v) != nil || v <= 0 || v > 253402300799 {
		return 0, false
	}
	return v, true
}
func validTime(t time.Time) bool { return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999 }
func opaque(s string) bool {
	return utf8.ValidString(s) && len([]rune(s)) >= 1 && len([]rune(s)) <= 255
}
func uuid(s string, v4 bool) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return !v4 || (s[14] == '4' && strings.ContainsRune("89abAB", rune(s[19])))
}
func allowed(m map[string]json.RawMessage, fields string) bool {
	for key := range m {
		found := false
		for _, field := range strings.Fields(fields) {
			if key == field {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func nilPort(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}

const eventFields = "id object api_version created data livemode account context pending_webhooks request type"

// Stripe provider fields are validated for spelling at the projection boundary;
// only the five consumed Session fields are interpreted. Ignored documented
// provider fields (including metadata/contact) never become internal wire data.
const sessionFields = "id object created livemode adaptive_pricing after_expiration allow_promotion_codes allowed_payment_method_types amount_subtotal amount_total automatic_tax billing_address_collection branding_settings cancel_url client_reference_id client_secret collected_information consent consent_collection currency currency_conversion custom_fields custom_text customer customer_account customer_creation customer_details customer_email discounts excluded_payment_method_types expires_at invoice invoice_creation line_items locale managed_payments metadata mode name_collection optional_items origin_context payment_intent payment_link payment_method_collection payment_method_configuration_details payment_method_data payment_method_options payment_method_types payment_status permissions phone_number_collection presentment_details recovered_from redirect_on_completion return_url saved_payment_method_options setup_intent shipping_address_collection shipping_cost shipping_details shipping_options status submit_type subscription success_url tax_id_collection total_details ui_mode url wallet_options"
