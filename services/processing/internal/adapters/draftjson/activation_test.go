package draftjson

import (
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"strings"
	"testing"
)

func activationJSON(t *testing.T) string {
	t.Helper()
	id := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	b, e := json.Marshal(drafts.ActivationCommand{Command: drafts.CommandMeta{RequestID: id}, DraftID: id, DraftRevision: 1, ReadinessCheckID: id, RehearsalID: id, AcceptedConfigDigest: "sha256:" + strings.Repeat("a", 64), AcceptanceTextVersion: "managed-operation-v1", ConfirmActivate: true})
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestActivationExactWireContract(t *testing.T) {
	good := activationJSON(t)
	codec := ActivationCodec{}
	if _, e := codec.Decode([]byte(good)); e != nil {
		t.Fatal(e)
	}
	var obj map[string]json.RawMessage
	json.Unmarshal([]byte(good), &obj)
	for field := range obj {
		t.Run(field, func(t *testing.T) {
			for _, change := range []string{"absent", "null", "upper", "duplicate", "upper_before", "upper_after"} {
				var m map[string]json.RawMessage
				json.Unmarshal([]byte(good), &m)
				original := string(m[field])
				delete(m, field)
				base, _ := json.Marshal(m)
				body := string(base)
				addition := ""
				switch change {
				case "absent":
				case "null":
					addition = `"` + field + `":null`
				case "upper":
					addition = `"` + strings.ToUpper(field) + `":` + original
				case "duplicate":
					addition = `"` + field + `":` + original + `,"` + field + `":` + original
				case "upper_before":
					addition = `"` + strings.ToUpper(field) + `":` + original + `,"` + field + `":` + original
				case "upper_after":
					addition = `"` + field + `":` + original + `,"` + strings.ToUpper(field) + `":` + original
				}
				if addition != "" {
					body = body[:len(body)-1] + "," + addition + "}"
				}
				if _, e := codec.Decode([]byte(body)); e == nil {
					t.Fatal(field, change, "accepted")
				}
			}
		})
	}
	for _, bad := range []string{good + ` {}`, strings.Replace(good, `"confirm_activate":true`, `"confirm_activate":false`, 1), strings.Replace(good, `"draft_revision":1`, `"draft_revision":0`, 1), strings.Replace(good, `"draft_revision":1`, `"draft_revision":1.5`, 1), strings.Replace(good, `"draft_revision":1`, `"draft_revision":9007199254740992`, 1), strings.Replace(good, `"draft_revision":1`, `"draft_revision":NaN`, 1), strings.Replace(good, `"request_id":`, `"REQUEST_ID":`, 1), strings.Replace(good, `"request_id":`, `"extra":0,"request_id":`, 1), strings.Replace(good, `"command":{`, `"command":{"expected_revision":null,`, 1), strings.Replace(good, `"command":{`, `"command":{"expected_revision":1,"EXPECTED_REVISION":2,`, 1), strings.Replace(good, `"command":{`, `"command":{"EXPECTED_REVISION":2,"expected_revision":1,`, 1)} {
		if _, e := codec.Decode([]byte(bad)); e == nil {
			t.Fatal("invalid accepted", bad)
		}
	}
}
func TestActivationOperationNullableFieldsAndCanonicalDigest(t *testing.T) {
	a := activationJSON(t)
	x, e := (ActivationCodec{}).Decode([]byte(a))
	if e != nil {
		t.Fatal(e)
	}
	var m any
	json.Unmarshal([]byte(a), &m)
	pretty, _ := json.MarshalIndent(m, "", " ")
	y, e := (ActivationCodec{}).Decode(pretty)
	if e != nil || x.Digest != y.Digest {
		t.Fatal("JCS digest changed")
	}
	b, _ := json.Marshal(drafts.Operation{OperationID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Revision: 1, Name: "", State: "DRAFT", ConnectionHealth: "UNCHECKED"})
	if (ActivationCodec{}).ValidateResponse(b) != nil {
		t.Fatal("canonical nullable Operation rejected", string(b))
	}
	if Validate("Operation", []byte(strings.Replace(string(b), `"revision":1`, `"revision":false`, 1))) == nil {
		t.Fatal("false revision accepted")
	}
}
