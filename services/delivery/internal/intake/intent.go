package intake

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gowebpki/jcs"
)

const maxContextBytes = 32 * 1024
const maxBlockBytes = 36 * 1024
const maxContentBytes = 64 * 1024

type Intent struct {
	MessageID, WorkspaceID, CaseID, OperationID, ConfigID, LineageID, EpisodeID uuid.UUID
	ConnectionID, SourceFactsID                                                 uuid.UUID
	MessageDigest, ConfigDigest, FactsDigest                                    string
	Epoch                                                                       int64
	FactsUntil, Deadline                                                        time.Time
	DestinationWorkspace, List, Assignee                                        string
	Content                                                                     []byte
	ContentUntil                                                                time.Time
}

type invalid struct{ size bool }

func (invalid) Error() string { return "invalid delivery intent" }

func DecodeIntent(data []byte) (Intent, error) {
	if !utf8.Valid(data) {
		return Intent{}, invalid{}
	}
	// JCS validates object names and Unicode scalar values before encoding/json
	// can replace invalid surrogates or overwrite duplicate names.
	wire, err := jcs.Transform(data)
	if err != nil {
		return Intent{}, invalid{}
	}
	fields, err := decodeObject(wire)
	if err != nil || len(fields) != 27 {
		return Intent{}, invalid{}
	}
	var i Intent
	for name, target := range map[string]*uuid.UUID{
		"message_id": &i.MessageID, "workspace_id": &i.WorkspaceID, "case_id": &i.CaseID,
		"operation_id": &i.OperationID, "config_id": &i.ConfigID, "lineage_id": &i.LineageID,
		"episode_id": &i.EpisodeID, "connection_id": &i.ConnectionID, "source_facts_id": &i.SourceFactsID,
	} {
		*target, err = parseUUID(fields[name])
		if err != nil {
			return Intent{}, invalid{}
		}
	}
	if i.MessageID.Version() != 4 || i.MessageID.Variant() != uuid.RFC4122 {
		return Intent{}, invalid{}
	}
	for name, target := range map[string]*string{
		"message_digest": &i.MessageDigest, "config_digest": &i.ConfigDigest,
		"source_facts_digest": &i.FactsDigest,
	} {
		*target, err = stringField(fields[name], 71, 71)
		if err != nil || !validDigest(*target) {
			return Intent{}, invalid{}
		}
	}
	if !bytes.Equal(fields["schema_version"], []byte("\"1.0\"")) ||
		!bytes.Equal(fields["rendering_version"], []byte("\"handoff-block-v1\"")) {
		return Intent{}, invalid{}
	}
	original, err := decodeObject(data)
	if err != nil {
		return Intent{}, invalid{}
	}
	i.Epoch, err = exactEpoch(string(original["connection_epoch"]))
	if err != nil {
		return Intent{}, invalid{}
	}
	i.FactsUntil, err = dateField(fields["facts_valid_until"])
	if err != nil {
		return Intent{}, invalid{}
	}
	i.Deadline, err = dateField(fields["deadline_at"])
	if err != nil {
		return Intent{}, invalid{}
	}
	// The worker must be able to recover the exact accepted target after a restart.
	// The existing private, digest-checked content file holds those values.
	payload := make(map[string]json.RawMessage)
	for name, target := range map[string]*string{
		"destination_workspace_id": &i.DestinationWorkspace, "target_list_id": &i.List,
		"assignee_id": &i.Assignee,
	} {
		*target, err = stringField(fields[name], 1, 255)
		if err != nil {
			return Intent{}, err
		}
		payload[name] = fields[name]
	}

	for name, maximum := range map[string]int{
		"title": 255, "service_label": 120, "source_session_id": 255, "order_reference": 64,
		"case_marker": 255, "rendered_handoff_block": maxBlockBytes,
	} {
		value, err := stringField(fields[name], 1, maximum)
		if err != nil {
			return Intent{}, err
		}
		if name == "rendered_handoff_block" && len(value) > maxBlockBytes {
			return Intent{}, invalid{size: true}
		}
		payload[name] = fields[name]
	}
	if i.ContentUntil, err = validateContext(fields["context"]); err != nil {
		return Intent{}, err
	}
	payload["context"] = fields["context"]
	i.Content, err = canonicalObject(payload)
	if err != nil {
		return Intent{}, invalid{}
	}
	if len(i.Content) > maxContentBytes {
		return Intent{}, invalid{size: true}
	}
	delete(fields, "message_digest")
	unsigned, err := canonicalObject(fields)
	if err != nil || digest(unsigned) != i.MessageDigest {
		return Intent{}, invalid{}
	}
	return i, nil
}

func validateContext(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || raw[0] != '[' {
		return time.Time{}, invalid{}
	}
	if len(raw) > maxContextBytes {
		return time.Time{}, invalid{size: true}
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return time.Time{}, invalid{}
	}
	if len(items) > 20 {
		return time.Time{}, invalid{size: true}
	}
	var earliest time.Time
	for _, item := range items {
		if len(item) != 4 {
			return time.Time{}, invalid{}
		}
		key, err := stringField(item["key"], 1, 64)
		if err != nil || key[0] < 'a' || key[0] > 'z' {
			return time.Time{}, invalid{}
		}
		for _, char := range key[1:] {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_') {
				return time.Time{}, invalid{}
			}
		}
		if _, err := stringField(item["value"], 0, 32768); err != nil {
			return time.Time{}, err
		}
		if _, err := dateField(item["added_at"]); err != nil {
			return time.Time{}, err
		}
		expires, err := dateField(item["expires_at"])
		if err != nil {
			return time.Time{}, err
		}
		if earliest.IsZero() || expires.Before(earliest) {
			earliest = expires
		}
	}
	return earliest, nil
}

func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	var values map[string]json.RawMessage
	err := json.Unmarshal(data, &values)
	return values, err
}

func canonicalObject(values map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(data)
}

func stringField(raw json.RawMessage, minimum, maximum int) (string, error) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return "", invalid{}
	}
	n := utf8.RuneCountInString(value)
	switch {
	case n < minimum:
		return "", invalid{}
	case n > maximum:
		return "", invalid{size: true}
	default:
		return value, nil
	}
}

func parseUUID(raw json.RawMessage) (uuid.UUID, error) {
	value, err := stringField(raw, 36, 36)
	if err != nil {
		return uuid.Nil, err
	}
	id, err := uuid.Parse(value)
	if err != nil || !strings.EqualFold(value, id.String()) {
		return uuid.Nil, invalid{}
	}
	return id, nil
}

func exactEpoch(number string) (int64, error) {
	if len(number) > 64 {
		return 0, invalid{}
	}
	if at := strings.IndexAny(number, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(number[at+1:])
		if err != nil || exponent < -64 || exponent > 64 {
			return 0, invalid{}
		}
	}
	value, ok := new(big.Rat).SetString(number)
	if !ok || !value.IsInt() || !value.Num().IsInt64() {
		return 0, invalid{}
	}
	n := value.Num().Int64()
	if n < 1 || n > 9007199254740991 {
		return 0, invalid{}
	}
	return n, nil
}

func dateField(raw json.RawMessage) (time.Time, error) {
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil {
		return time.Time{}, invalid{}
	}
	return time.Parse(time.RFC3339Nano, value)
}

func digest(data []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

func validDigest(value string) bool {
	if len(value) != 71 || value[:7] != "sha256:" {
		return false
	}
	for _, char := range value[7:] {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}
