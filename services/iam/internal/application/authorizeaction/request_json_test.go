package authorizeaction

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestJSONRequiresNullResourceAndRejectsAmbiguity(t *testing.T) {
	valid := `{"workspace_id":"` + workspaceID + `","action":"CASE_READ","resource_id":null,"runtime_id":"` + runtimeID + `"}`
	var r Request
	if e := json.Unmarshal([]byte(valid), &r); e != nil || r.ResourceID != nil || r.WorkspaceID != workspaceID {
		t.Fatal("required null rejected")
	}
	encoded, e := json.Marshal(r)
	if e != nil || !strings.Contains(string(encoded), `"resource_id":null`) {
		t.Fatal("null became absent")
	}
	for _, tc := range []struct{ name, value string }{
		{"missing resource", strings.Replace(valid, `"resource_id":null,`, "", 1)},
		{"missing action", strings.Replace(valid, `"action":"CASE_READ",`, "", 1)},
		{"unknown field", strings.Replace(valid, `"action":"CASE_READ"`, `"action":"CASE_READ","principal_id":"`+principalID+`"`, 1)},
		{"duplicate field", strings.Replace(valid, `"action":"CASE_READ"`, `"action":"CASE_READ","action":"CONFIG_WRITE"`, 1)},
		{"unknown action", strings.Replace(valid, "CASE_READ", "UNKNOWN", 1)},
		{"null workspace", strings.Replace(valid, `"`+workspaceID+`"`, "null", 1)},
		{"bad resource", strings.Replace(valid, `"resource_id":null`, `"resource_id":"bad"`, 1)},
		{"array resource", strings.Replace(valid, `"resource_id":null`, `"resource_id":[]`, 1)},
		{"number action", strings.Replace(valid, `"CASE_READ"`, "42", 1)},
		{"trailing object", valid + `{}`}, {"top null", `null`}, {"top array", `[]`}, {"truncated", valid[:len(valid)-1]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parsed := request("CASE_READ")
			if json.Unmarshal([]byte(tc.value), &parsed) == nil {
				t.Fatal("ambiguous request accepted")
			}
		})
	}
	var upper Request
	if e := json.Unmarshal([]byte(strings.ReplaceAll(valid, workspaceID, strings.ToUpper(workspaceID))), &upper); e != nil || upper.WorkspaceID != workspaceID {
		t.Fatal("valid UUID case rejected")
	}
}
