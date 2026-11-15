package observationjson

import (
	"encoding/json"
	"github.com/gowebpki/jcs"
	"strings"
	"testing"
)

func testWire(t *testing.T) []byte {
	t.Helper()
	m := map[string]any{"message_id": "11111111-1111-4111-8111-111111111111", "schema_version": "1.0", "workspace_id": "11111111-1111-4111-8111-111111111111", "case_id": "11111111-1111-4111-8111-111111111111", "lineage_id": "11111111-1111-4111-8111-111111111111", "episode_id": "11111111-1111-4111-8111-111111111111", "observation_id": "11111111-1111-4111-8111-111111111111", "knowledge": "INCOMPLETE", "task_id": nil, "observed_at": "2026-11-15T10:00:00Z", "complete": false, "workspace_matches": nil, "list_matches": nil, "assignee_matches": nil, "title_matches": nil, "marker_matches": nil, "context_matches": nil, "proof_code": "CONTROLLED_UNREADABLE", "proof_digest": "sha256:" + strings.Repeat("a", 64), "evidence_ref": nil, "evidence_expires_at": nil}
	b, _ := json.Marshal(m)
	b, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = Digest(b)
	b, _ = json.Marshal(m)
	return b
}
func TestExactRequiredNullDigestAndCanonicalNames(t *testing.T) {
	raw := testWire(t)
	o, e := (Codec{}).Decode(raw)
	if e != nil || o.Complete || o.TaskID != nil || o.ListMatches != nil {
		t.Fatal(o, e)
	}
	var m map[string]json.RawMessage
	json.Unmarshal(raw, &m)
	for k := range m {
		bad := map[string]json.RawMessage{}
		for x, v := range m {
			if x != k {
				bad[x] = v
			}
		}
		b, _ := json.Marshal(bad)
		if Validate("TaskObservation", b) == nil {
			t.Fatal("required omission accepted", k)
		}
	}
	for _, b := range [][]byte{append(raw, []byte(` {}`)...), []byte(strings.Replace(string(raw), `"complete":false`, `"complete":false,"COMPLETE":true`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"COMPLETE":true,"complete":false`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"complete":true,"complete":false`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"complete":null`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"complete":0`, 1)), []byte(strings.Replace(string(raw), `"knowledge":"INCOMPLETE"`, `"knowledge":"incomplete"`, 1)), []byte(strings.Replace(string(raw), `"schema_version":"1.0"`, `"schema_version":"2.0"`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"extra":0,"complete":false`, 1)), []byte(strings.Replace(string(raw), `"complete":false`, `"complete":true`, 1))} {
		if _, e = (Codec{}).Decode(b); e == nil {
			t.Fatal("invalid/ambiguous/digest mismatch accepted", string(b))
		}
	}
}

func TestOriginalWireRejectsUnpairedUnicodeSurrogatesBeforeDigest(t *testing.T) {
	raw := testWire(t)
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["proof_code"] = "controlled_replacement_�"
	delete(m, "message_digest")
	unsigned, _ := json.Marshal(m)
	canonical, e := jcs.Transform(unsigned)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = Digest(canonical)
	valid, _ := json.Marshal(m)
	for _, escape := range []string{`\ud800`, `\udfff`} {
		bad := strings.Replace(string(valid), "controlled_replacement_�", "controlled_replacement_"+escape, 1)
		if _, e = (Codec{}).Decode([]byte(bad)); e == nil {
			t.Fatal("unpaired surrogate accepted after lossy JSON decoding", escape)
		}
	}
}

func TestValidSurrogatePairAndLiteralUnicodeHaveSameDigest(t *testing.T) {
	raw := testWire(t)
	var m map[string]any
	json.Unmarshal(raw, &m)
	m["proof_code"] = "controlled_😀"
	delete(m, "message_digest")
	unsigned, _ := json.Marshal(m)
	canon, e := jcs.Transform(unsigned)
	if e != nil {
		t.Fatal(e)
	}
	m["message_digest"] = Digest(canon)
	valid, _ := json.Marshal(m)
	escaped := strings.Replace(string(valid), "controlled_😀", `controlled_\ud83d\ude00`, 1)
	for _, b := range []string{string(valid), escaped} {
		if _, e = (Codec{}).Decode([]byte(b)); e != nil {
			t.Fatal("valid Unicode rejected", e)
		}
	}
}
