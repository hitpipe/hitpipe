package admission

import (
	"errors"
	"time"
)

var ErrInvalidSignal = errors.New("invalid normalized signal")

type Signal struct {
	MessageID    string `json:"message_id"`
	Digest       string `json:"message_digest"`
	Version      string `json:"schema_version"`
	WorkspaceID  string `json:"workspace_id"`
	ConnectionID string `json:"connection_id"`
	AccountID    string `json:"stripe_account_id"`
	Live         bool   `json:"livemode"`
	SessionID    string `json:"session_id"`
	EventID      string `json:"event_id"`
	EventType    string `json:"event_type"`
	// SourceCreatedAt is Checkout Session.created (Event.data.object.created),
	// per HIT-78 comment 12171, never Event.created or receipt time.
	SourceCreatedAt time.Time `json:"source_created_at"`
	FirstReceivedAt time.Time `json:"first_received_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	ReceiptID       string    `json:"receipt_id"`
}
