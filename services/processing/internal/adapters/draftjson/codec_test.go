package draftjson

import (
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"strings"
	"testing"
)

const id = "11111111-1111-4111-8111-111111111111"

func TestStrictCanonicalNamesBothOrdersAndAllLevels(t *testing.T) {
	valid := `{"command":{"request_id":"` + id + `"},"config":{"draft_id":"` + id + `","revision":1,"name":"draft"}}`
	if _, e := (Codec{}).Decode(drafts.Save, []byte(valid)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{
		`{"COMMAND":{"request_id":"` + id + `"},"command":{"request_id":"` + id + `"},"config":{"draft_id":"` + id + `","revision":1,"name":"draft"}}`,
		strings.Replace(valid, `"command":`, `"command":{},"COMMAND":`, 1),
		strings.Replace(valid, `"revision":1`, `"revision":1,"REVISION":2`, 1), strings.Replace(valid, `"revision":1`, `"REVISION":2,"revision":1`, 1),
		strings.Replace(valid, `"revision":1`, `"revision":1,"revision":2`, 1), strings.Replace(valid, `"name":"draft"`, `"name":"draft","NAME":"other"`, 1),
		strings.Replace(valid, `"request_id":"`+id+`"`, `"request_id":"`+id+`","REQUEST_ID":"`+id+`"`, 1), valid + ` {}`,
		strings.Replace(valid, `"name":"draft"`, `"name":null`, 1), strings.Replace(valid, `"name":"draft"`, `"unknown":"draft","name":"draft"`, 1),
		strings.Replace(valid, `"revision":1`, `"revision":9007199254740991.1`, 1), strings.Replace(valid, `"revision":1`, `"revision":9007199254740992`, 1), strings.Replace(valid, `"revision":1`, `"revision":NaN`, 1),
		strings.Replace(valid, `"name":"draft"`, `"name":"sk_live_synthetic_for_negative_test"`, 1),
	} {
		if _, e := (Codec{}).Decode(drafts.Save, []byte(raw)); e == nil {
			t.Fatal("ambiguous/unsafe request accepted", raw)
		}
	}
	for _, number := range []string{"1.0", "1e0", "9007199254740991"} {
		raw := strings.Replace(valid, `"revision":1`, `"revision":`+number, 1)
		if _, e := (Codec{}).Decode(drafts.Save, []byte(raw)); e != nil {
			t.Fatal("valid integer rejected", number, e)
		}
	}
}
func TestRehearsalKnownZeroFalseAndArrayLimits(t *testing.T) {
	v := drafts.RehearsalRequest{ReadinessCheckID: id, Command: drafts.CommandMeta{RequestID: id}, DraftID: id, DraftRevision: 1, Examples: []drafts.ScenarioExample{{SyntheticSessionAlias: "example_zero", AmountMinor: 0, Paid: false, Duplicate: false, MissingContextKeys: []string{}}}}
	b, _ := json.Marshal(v)
	in, e := (Codec{}).Decode(drafts.Rehearse, b)
	if e != nil || len(in.Examples) != 1 || in.Examples[0].Paid || in.Examples[0].AmountMinor != 0 {
		t.Fatal(in, e)
	}
	for _, n := range []int{0, 11} {
		v.Examples = make([]drafts.ScenarioExample, n)
		for i := range v.Examples {
			v.Examples[i] = drafts.ScenarioExample{SyntheticSessionAlias: "example_x", MissingContextKeys: []string{}}
		}
		b, _ = json.Marshal(v)
		if _, e = (Codec{}).Decode(drafts.Rehearse, b); e == nil {
			t.Fatal("array bounds accepted", n)
		}
	}
}
func TestGeneratedSchemaRequiredSensitivityAndNull(t *testing.T) {
	b := []byte(`{"draft_id":"` + id + `","revision":1,"name":"Договор","required_context_keys":[]}`)
	v, e := parse(b)
	if e != nil || !validate(v, schemas["DraftConfig"]) {
		t.Fatal(e)
	}
	for _, field := range []string{"draft_id", "revision", "name"} {
		var x map[string]any
		json.Unmarshal(b, &x)
		delete(x, field)
		if validate(x, schemas["DraftConfig"]) {
			t.Fatal("required ignored", field)
		}
	}
	for _, extra := range []string{`"mode":null`, `"required_context_keys":["key","key"]`, `"required_context_keys":["Upper"]`, `"mode":"live"`} {
		bad := `{"draft_id":"` + id + `","revision":1,"name":"x",` + extra + `}`
		if Validate("DraftConfig", []byte(bad)) == nil {
			t.Fatal("invalid config allowed", extra)
		}
	}
}
