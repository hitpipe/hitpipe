// Code generated from OAS fedbe40bb597e25f66aeb1172af3ff7459c81b40; DO NOT EDIT.
package observations

import "time"

type TaskObservation struct {
	AssigneeMatches   *bool      `json:"assignee_matches"`
	CaseID            string     `json:"case_id"`
	Complete          bool       `json:"complete"`
	ContextMatches    *bool      `json:"context_matches"`
	EpisodeID         string     `json:"episode_id"`
	EvidenceExpiresAt *time.Time `json:"evidence_expires_at"`
	EvidenceRef       *string    `json:"evidence_ref"`
	Knowledge         string     `json:"knowledge"`
	LineageID         string     `json:"lineage_id"`
	ListMatches       *bool      `json:"list_matches"`
	MarkerMatches     *bool      `json:"marker_matches"`
	MessageDigest     string     `json:"message_digest"`
	MessageID         string     `json:"message_id"`
	ObservationID     string     `json:"observation_id"`
	ObservedAt        time.Time  `json:"observed_at"`
	ProofCode         string     `json:"proof_code"`
	ProofDigest       string     `json:"proof_digest"`
	SchemaVersion     string     `json:"schema_version"`
	TaskID            *string    `json:"task_id"`
	TitleMatches      *bool      `json:"title_matches"`
	WorkspaceID       string     `json:"workspace_id"`
	WorkspaceMatches  *bool      `json:"workspace_matches"`
}
type Receipt struct {
	Disposition string  `json:"disposition"`
	MessageID   string  `json:"message_id"`
	RecordID    *string `json:"record_id,omitempty"`
	Revision    *int64  `json:"revision,omitempty"`
}
type Error struct {
	Code              string `json:"code"`
	CurrentRevision   *int64 `json:"current_revision,omitempty"`
	Message           string `json:"message"`
	RequestID         string `json:"request_id"`
	RetryAfterSeconds *int64 `json:"retry_after_seconds,omitempty"`
}
