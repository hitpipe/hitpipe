// Code generated from pinned mvp-openapi.json fedbe40bb597e25f66aeb1172af3ff7459c81b40; DO NOT EDIT.
package preparecase

import "time"

type ContextField struct {
	AddedAt   time.Time `json:"added_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
}
type DeliveryIntent struct {
	AssigneeId             string         `json:"assignee_id"`
	CaseId                 string         `json:"case_id"`
	CaseMarker             string         `json:"case_marker"`
	ConfigDigest           string         `json:"config_digest"`
	ConfigId               string         `json:"config_id"`
	ConnectionEpoch        int64          `json:"connection_epoch"`
	ConnectionId           string         `json:"connection_id"`
	Context                []ContextField `json:"context"`
	DeadlineAt             time.Time      `json:"deadline_at"`
	DestinationWorkspaceId string         `json:"destination_workspace_id"`
	EpisodeId              string         `json:"episode_id"`
	FactsValidUntil        time.Time      `json:"facts_valid_until"`
	LineageId              string         `json:"lineage_id"`
	MessageDigest          string         `json:"message_digest"`
	MessageId              string         `json:"message_id"`
	OperationId            string         `json:"operation_id"`
	OrderReference         string         `json:"order_reference"`
	RenderedHandoffBlock   string         `json:"rendered_handoff_block"`
	RenderingVersion       string         `json:"rendering_version"`
	SchemaVersion          string         `json:"schema_version"`
	ServiceLabel           string         `json:"service_label"`
	SourceFactsDigest      string         `json:"source_facts_digest"`
	SourceFactsId          string         `json:"source_facts_id"`
	SourceSessionId        string         `json:"source_session_id"`
	TargetListId           string         `json:"target_list_id"`
	Title                  string         `json:"title"`
	WorkspaceId            string         `json:"workspace_id"`
}
