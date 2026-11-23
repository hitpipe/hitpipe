package intake

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
)

const queryRoute = "/internal/v1/delivery-query"

type Query struct {
	Workspace, Case, Lineage uuid.UUID
	Epoch                    int64
	Deadline                 time.Time
	TaskID                   string
}

type QueryReader interface {
	QueryKnownTask(context.Context, Query) (TaskObservation, error)
}

type TaskObservation struct {
	MessageID        uuid.UUID  `json:"message_id"`
	MessageDigest    string     `json:"message_digest"`
	SchemaVersion    string     `json:"schema_version"`
	Workspace        uuid.UUID  `json:"workspace_id"`
	Case             uuid.UUID  `json:"case_id"`
	Lineage          uuid.UUID  `json:"lineage_id"`
	Episode          uuid.UUID  `json:"episode_id"`
	Observation      uuid.UUID  `json:"observation_id"`
	Knowledge        string     `json:"knowledge"`
	TaskID           *string    `json:"task_id"`
	ObservedAt       time.Time  `json:"observed_at"`
	Complete         bool       `json:"complete"`
	WorkspaceMatches *bool      `json:"workspace_matches"`
	ListMatches      *bool      `json:"list_matches"`
	AssigneeMatches  *bool      `json:"assignee_matches"`
	TitleMatches     *bool      `json:"title_matches"`
	MarkerMatches    *bool      `json:"marker_matches"`
	ContextMatches   *bool      `json:"context_matches"`
	ProofCode        string     `json:"proof_code"`
	ProofDigest      string     `json:"proof_digest"`
	EvidenceRef      *uuid.UUID `json:"evidence_ref"`
	EvidenceExpires  *time.Time `json:"evidence_expires_at"`
}

func DecodeQuery(raw []byte) (Query, error) {
	var q Query
	if !utf8.Valid(raw) {
		return q, ErrDenied
	}
	if _, err := jcs.Transform(raw); err != nil {
		return q, err
	}
	f, err := decodeObject(raw)
	if err != nil || len(f) < 5 || len(f) > 6 {
		return q, invalid{}
	}
	m := map[string]*uuid.UUID{"workspace_id": &q.Workspace, "case_id": &q.Case, "lineage_id": &q.Lineage}
	for key, dst := range m {
		*dst, err = parseUUID(f[key])
		if err != nil {
			return q, err
		}
	}
	q.Epoch, err = exactEpoch(string(f["expected_connection_epoch"]))
	if err != nil {
		return q, err
	}
	q.Deadline, err = dateField(f["deadline_at"])
	if err != nil {
		return q, err
	}
	if len(f) == 6 {
		q.TaskID, err = stringField(f["native_task_id"], 1, 255)
		if err != nil {
			return q, err
		}
	}
	return q, nil
}

// EncodeObservation computes RFC8785 SHA-256 over the complete wire projection
// with message_digest omitted.
func EncodeObservation(o *TaskObservation) ([]byte, error) {
	raw, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	f, err := decodeObject(raw)
	if err != nil {
		return nil, err
	}
	delete(f, "message_digest")
	u, err := canonicalObject(f)
	if err != nil {
		return nil, err
	}
	o.MessageDigest = digest(u)
	raw, err = json.Marshal(o)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(raw)
}
