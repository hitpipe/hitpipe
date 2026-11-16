// Code generated from exact OAS fedbe40bb597e25f66aeb1172af3ff7459c81b40; DO NOT EDIT.
package drafts

import "time"

type CommandMeta struct {
	ExpectedRevision *int64 `json:"expected_revision,omitempty"`
	RequestID        string `json:"request_id"`
}
type DraftConfig struct {
	AssigneeAlias           *string   `json:"assignee_alias,omitempty"`
	DestinationConnectionID *string   `json:"destination_connection_id,omitempty"`
	DraftID                 string    `json:"draft_id"`
	ListAlias               *string   `json:"list_alias,omitempty"`
	Mode                    *string   `json:"mode,omitempty"`
	Name                    string    `json:"name"`
	PriceAlias              *string   `json:"price_alias,omitempty"`
	RequiredContextKeys     *[]string `json:"required_context_keys,omitempty"`
	Revision                int64     `json:"revision"`
	ServiceLabel            *string   `json:"service_label,omitempty"`
	SourceConnectionID      *string   `json:"source_connection_id,omitempty"`
	TitlePattern            *string   `json:"title_pattern,omitempty"`
}
type DraftWrite struct {
	Command CommandMeta `json:"command"`
	Config  DraftConfig `json:"config"`
}
type Error struct {
	Code              string `json:"code"`
	CurrentRevision   *int64 `json:"current_revision,omitempty"`
	Message           string `json:"message"`
	RequestID         string `json:"request_id"`
	RetryAfterSeconds *int64 `json:"retry_after_seconds,omitempty"`
}
type ReadinessCheck struct {
	CheckID       string    `json:"check_id"`
	CheckedAt     time.Time `json:"checked_at"`
	ConfigDigest  string    `json:"config_digest"`
	DraftID       string    `json:"draft_id"`
	DraftRevision int64     `json:"draft_revision"`
	ExpiresAt     time.Time `json:"expires_at"`
	Issues        []Error   `json:"issues"`
	Passed        bool      `json:"passed"`
}
type ScenarioExample struct {
	AmountMinor           int64    `json:"amount_minor"`
	Duplicate             bool     `json:"duplicate"`
	MissingContextKeys    []string `json:"missing_context_keys"`
	Paid                  bool     `json:"paid"`
	SyntheticSessionAlias string   `json:"synthetic_session_alias"`
}
type RehearsalRequest struct {
	Command       CommandMeta       `json:"command"`
	DraftID       string            `json:"draft_id"`
	DraftRevision int64             `json:"draft_revision"`
	Examples      []ScenarioExample `json:"examples"`
}
type RehearsalResult struct {
	ConfigDigest     string   `json:"config_digest"`
	DraftID          string   `json:"draft_id"`
	DraftRevision    int64    `json:"draft_revision"`
	LiveRequestsMade int64    `json:"live_requests_made"`
	RehearsalID      string   `json:"rehearsal_id"`
	Results          []string `json:"results"`
}
type ActivationCommand struct {
	AcceptanceTextVersion string      `json:"acceptance_text_version"`
	AcceptedConfigDigest  string      `json:"accepted_config_digest"`
	Command               CommandMeta `json:"command"`
	ConfirmActivate       bool        `json:"confirm_activate"`
	DraftID               string      `json:"draft_id"`
	DraftRevision         int64       `json:"draft_revision"`
	ReadinessCheckID      string      `json:"readiness_check_id"`
	RehearsalID           string      `json:"rehearsal_id"`
}
type Operation struct {
	AcceptedConfigDigest *string    `json:"accepted_config_digest"`
	AcceptedConfigID     *string    `json:"accepted_config_id"`
	ConnectionHealth     string     `json:"connection_health"`
	DrainDeadlineAt      *time.Time `json:"drain_deadline_at"`
	FirstActivationAt    *time.Time `json:"first_activation_at"`
	Name                 string     `json:"name"`
	OperationID          string     `json:"operation_id"`
	Revision             int64      `json:"revision"`
	State                string     `json:"state"`
}
