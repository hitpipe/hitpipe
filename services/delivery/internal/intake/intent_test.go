package intake

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func testFields(now time.Time) map[string]json.RawMessage {
	sum := "sha256:" + strings.Repeat("a", 64)
	values := map[string]any{
		"schema_version": "1.0", "rendering_version": "handoff-block-v1",
		"connection_epoch": 1, "config_digest": sum, "source_facts_digest": sum,
		"deadline_at":              now.Add(time.Hour).Format(time.RFC3339Nano),
		"facts_valid_until":        now.Add(time.Minute).Format(time.RFC3339Nano),
		"destination_workspace_id": "ws", "target_list_id": "list", "assignee_id": "person",
		"title": "service — order", "service_label": "service", "source_session_id": "session",
		"order_reference": "order", "case_marker": "case", "rendered_handoff_block": "opaque contents",
		"context": []map[string]string{{"key": "note", "value": "Клиент <&>",
			"added_at": now.Format(time.RFC3339Nano), "expires_at": now.Add(time.Hour).Format(time.RFC3339Nano)}},
	}
	for _, field := range []string{"message_id", "workspace_id", "case_id", "operation_id", "config_id",
		"lineage_id", "episode_id", "connection_id", "source_facts_id"} {
		values[field] = uuid.New()
	}
	raw, _ := json.Marshal(values)
	fields, _ := decodeObject(raw)
	return fields
}

func signed(t *testing.T, fields map[string]json.RawMessage) []byte {
	t.Helper()
	delete(fields, "message_digest")
	unsigned, err := canonicalObject(fields)
	if err != nil {
		t.Fatal(err)
	}
	fields["message_digest"], _ = json.Marshal(digest(unsigned))
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestIntentSchema(t *testing.T) {
	now := time.Now().UTC()
	raw := signed(t, testFields(now))
	if _, err := DecodeIntent(raw); err != nil {
		t.Fatalf("valid command: %v", err)
	}
	for name, change := range map[string]func(map[string]json.RawMessage){
		"unknown":          func(f map[string]json.RawMessage) { f["Credential"] = json.RawMessage("\"redacted\"") },
		"case alias":       func(f map[string]json.RawMessage) { f["TITLE"] = f["title"]; delete(f, "title") },
		"wrong version":    func(f map[string]json.RawMessage) { f["schema_version"] = json.RawMessage("\"2.0\"") },
		"wrong rendering":  func(f map[string]json.RawMessage) { f["rendering_version"] = json.RawMessage("\"unsupported\"") },
		"null":             func(f map[string]json.RawMessage) { f["case_id"] = json.RawMessage("null") },
		"missing":          func(f map[string]json.RawMessage) { delete(f, "context") },
		"null item":        func(f map[string]json.RawMessage) { f["context"] = json.RawMessage("[null]") },
		"unsafe epoch":     func(f map[string]json.RawMessage) { f["connection_epoch"] = json.RawMessage("9007199254740992") },
		"fractional epoch": func(f map[string]json.RawMessage) { f["connection_epoch"] = json.RawMessage("1.25") },
		"block bytes": func(f map[string]json.RawMessage) {
			f["rendered_handoff_block"], _ = json.Marshal(strings.Repeat("🙂", maxBlockBytes/4+1))
		},
		"oversized context": func(f map[string]json.RawMessage) {
			f["context"], _ = json.Marshal([]map[string]string{{"key": "note", "value": strings.Repeat("x", maxContextBytes), "added_at": now.Format(time.RFC3339), "expires_at": now.Add(time.Hour).Format(time.RFC3339)}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			fields := testFields(now)
			change(fields)
			if _, err := DecodeIntent(signed(t, fields)); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	for name, data := range map[string]string{
		"duplicate":        strings.Replace(string(raw), "\"title\":", "\"tit\\u006ce\":\"first\",\"title\":", 1),
		"nested duplicate": strings.Replace(string(raw), "\"key\":", "\"key\":\"first\",\"key\":", 1),
		"unpaired Unicode": strings.Replace(string(raw), "\"opaque contents\"", "\"\\ud800\"", 1),
		"bad digest":       strings.Replace(string(raw), "opaque contents", "changed", 1),
		"trailing JSON":    string(raw) + " {}",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeIntent([]byte(data)); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
}
