package draftjson

import (
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"strings"
	"testing"
	"time"
)

func previewFixture(t *testing.T) (drafts.DraftConfig, drafts.BindingProof, drafts.ActivationPreview) {
	t.Helper()
	str := func(s string) *string { return &s }
	keys := []string{}
	c := drafts.DraftConfig{DraftID: id, Revision: 1, Name: "Синтетический черновик", SourceConnectionID: str(id), DestinationConnectionID: str(id), Mode: str("TEST"), PriceAlias: str("PRICE_A"), ListAlias: str("LIST_A"), AssigneeAlias: str("ASSIGNEE_A"), ServiceLabel: str("Каталог услуг"), TitlePattern: str("SERVICE_ORDER"), RequiredContextKeys: &keys}
	p := drafts.BindingProof{WorkspaceID: id, PrincipalID: id, Epoch: 7, Granted: true, SourceConnectionID: id, DestinationConnectionID: id, Mode: "TEST", PriceAlias: "PRICE_A", ListAlias: "LIST_A", AssigneeAlias: "ASSIGNEE_A", SourceAccountID: "acct_controlled", DestinationWorkspaceID: "workspace_controlled", PriceID: "price_controlled", ListID: "list_controlled", AssigneeID: "assignee_controlled", ServiceLabel: "Каталог услуг"}
	preview, e := (Codec{}).Preview(c, p)
	if e != nil {
		t.Fatal(e)
	}
	return c, p, preview
}
func TestReadinessPreviewCorrelationHashAndStrictCanonicalFields(t *testing.T) {
	c, p, preview := previewFixture(t)
	codec := Codec{}
	draftDigest, e := codec.ConfigDigest(c)
	if e != nil || draftDigest == preview.AcceptedConfigDigest {
		t.Fatal("two distinct representations conflated", e)
	}
	now := time.Now().UTC()
	check := drafts.ReadinessCheck{CheckID: id, DraftID: id, DraftRevision: 1, ConfigDigest: draftDigest, Passed: true, CheckedAt: now, ExpiresAt: now.Add(time.Minute), Issues: []drafts.Error{}, ActivationPreview: &preview}
	positive, e := codec.Encode(drafts.Check, check)
	if e != nil {
		t.Fatal(e)
	}
	var obj map[string]any
	json.Unmarshal(positive, &obj)
	for _, variant := range []string{"true_null", "false_object", "hash_mismatch", "missing_required", "extra_private"} {
		copy := check
		switch variant {
		case "true_null":
			copy.ActivationPreview = nil
		case "false_object":
			copy.Passed = false
		case "hash_mismatch":
			altered := preview
			altered.AcceptedConfigDigest = "sha256:" + strings.Repeat("0", 64)
			copy.ActivationPreview = &altered
		case "missing_required":
			var m map[string]any
			json.Unmarshal(positive, &m)
			delete(m, "activation_preview")
			b, _ := json.Marshal(m)
			if codec.ValidateResponse(drafts.Check, b) == nil {
				t.Fatal(variant)
			}
			continue
		case "extra_private":
			b := strings.Replace(string(positive), `"activation_preview":{`, `"activation_preview":{"BindingProof":{},`, 1)
			if codec.ValidateResponse(drafts.Check, []byte(b)) == nil {
				t.Fatal(variant)
			}
			continue
		}
		b, _ := json.Marshal(copy)
		if codec.ValidateResponse(drafts.Check, b) == nil {
			t.Fatal(variant)
		}
	}
	check.Passed = false
	check.ActivationPreview = nil
	if _, e = codec.Encode(drafts.Check, check); e != nil {
		t.Fatal("canonical false/null rejected", e)
	}
	raw, _ := json.Marshal(preview.CanonicalConfig)
	var canonical map[string]any
	json.Unmarshal(raw, &canonical)
	if len(canonical) != 11 {
		t.Fatal("not exact full config")
	}
	for field := range canonical {
		for _, kind := range []string{"missing", "null", "upper"} {
			var m map[string]any
			json.Unmarshal(raw, &m)
			v := m[field]
			delete(m, field)
			if kind == "null" {
				m[field] = nil
			}
			if kind == "upper" {
				m[strings.ToUpper(field)] = v
			}
			b, _ := json.Marshal(m)
			if Validate("CanonicalAcceptedConfig", b) == nil {
				t.Fatal(field, kind)
			}
		}
	}
	p.ServiceLabel = "Other catalog meaning"
	if _, e = codec.Preview(c, p); e == nil {
		t.Fatal("free text substituted for catalog label")
	}
}
func TestRehearsalRequiresSelectedCheckIDAndRejectsCaseConflicts(t *testing.T) {
	raw := `{"command":{"request_id":"` + id + `"},"draft_id":"` + id + `","draft_revision":1,"readiness_check_id":"` + id + `","examples":[{"synthetic_session_alias":"example_zero","amount_minor":0,"paid":false,"duplicate":false,"missing_context_keys":[]}]}`
	in, e := (Codec{}).Decode(drafts.Rehearse, []byte(raw))
	if e != nil || in.ReadinessCheckID != id {
		t.Fatal(e)
	}
	for _, bad := range []string{strings.Replace(raw, `"readiness_check_id":"`+id+`",`, "", 1), strings.Replace(raw, `"readiness_check_id":"`+id+`"`, `"readiness_check_id":null`, 1), strings.Replace(raw, `"readiness_check_id":`, `"READINESS_CHECK_ID":"`+id+`","readiness_check_id":`, 1), strings.Replace(raw, `"readiness_check_id":"`+id+`"`, `"readiness_check_id":"`+id+`","READINESS_CHECK_ID":"`+id+`"`, 1)} {
		if _, e = (Codec{}).Decode(drafts.Rehearse, []byte(bad)); e == nil {
			t.Fatal("invalid selected check accepted")
		}
	}
}
