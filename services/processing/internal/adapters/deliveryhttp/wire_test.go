package deliveryhttp

import (
	"encoding/json"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"strings"
	"testing"
	"time"
)

const testID = "11111111-1111-4111-8111-111111111111"

func valid(t *testing.T) ([]byte, dispatchintent.Claim) {
	t.Helper()
	now := time.Now().UTC()
	p := preparecase.DeliveryIntent{MessageId: testID, WorkspaceId: testID, CaseId: testID, OperationId: testID, ConfigId: testID, ConfigDigest: "sha256:" + strings.Repeat("a", 64), LineageId: testID, EpisodeId: testID, ConnectionId: testID, ConnectionEpoch: 7, SourceFactsId: testID, SourceFactsDigest: "sha256:" + strings.Repeat("b", 64), FactsValidUntil: now.Add(time.Minute), DeadlineAt: now.Add(time.Minute), DestinationWorkspaceId: "controlled", TargetListId: "list", AssigneeId: "assignee", Title: "Услуга", ServiceLabel: "Услуга", SourceSessionId: "cs_controlled", OrderReference: "HP-controlled", RenderingVersion: "handoff-block-v1", RenderedHandoffBlock: "controlled", CaseMarker: "controlled", Context: []preparecase.ContextField{{Key: "brief", Value: "0 false", AddedAt: now.Add(-time.Second), ExpiresAt: now.Add(time.Minute)}}, SchemaVersion: "1.0"}
	raw, _ := json.Marshal(p)
	var m map[string]any
	json.Unmarshal(raw, &m)
	delete(m, "message_digest")
	raw, _ = json.Marshal(m)
	raw, e := jcs.Transform(raw)
	if e != nil {
		t.Fatal(e)
	}
	p.MessageDigest = intent.Digest(raw)
	raw, _ = json.Marshal(p)
	raw, e = jcs.Transform(raw)
	if e != nil {
		t.Fatal(e)
	}
	c := dispatchintent.Claim{Request: dispatchintent.Request{WorkspaceID: testID, Epoch: 7}, MessageID: testID, CaseID: testID, OperationID: testID, ConfigID: testID, ConfigDigest: p.ConfigDigest, ConnectionID: testID, EpisodeID: testID, LineageID: testID, Digest: p.MessageDigest, DeadlineAt: p.DeadlineAt, Ref: preparecase.Ref{ID: testID, Digest: intent.Digest(raw), Bytes: len(raw), Class: "DELIVERY_INTENT", ExpiresAt: p.FactsValidUntil}}
	return raw, c
}
func TestStrictIntentSchemaDigestAndPins(t *testing.T) {
	b, c := valid(t)
	c.DeadlineAt = c.DeadlineAt.Truncate(time.Microsecond) // actual timestamptz storage precision; original wire bytes remain intact
	m, e := (Codec{}).Decode(b, c, time.Now())
	if e != nil || string(m.Body) != string(b) {
		t.Fatal("canonical rejected", e)
	}
	var obj map[string]any
	json.Unmarshal(b, &obj)
	for key := range obj {
		t.Run("required_"+key, func(t *testing.T) {
			var x map[string]any
			json.Unmarshal(b, &x)
			delete(x, key)
			if validate(x, schemas["DeliveryIntent"]) {
				t.Fatal("required OAS property not enforced")
			}
			bad, _ := json.Marshal(x)
			cc := c
			cc.Ref.Bytes = len(bad)
			cc.Ref.Digest = intent.Digest(bad)
			if _, e := (Codec{}).Decode(bad, cc, time.Now()); e == nil {
				t.Fatal("missing required accepted")
			}
		})
	}
	for name, mutate := range map[string]func(*dispatchintent.Claim){"workspace": func(c *dispatchintent.Claim) { c.WorkspaceID = "22222222-2222-4222-8222-222222222222" }, "epoch": func(c *dispatchintent.Claim) { c.Epoch++ }, "digest": func(c *dispatchintent.Claim) { c.Digest = c.Ref.Digest }, "expired": func(c *dispatchintent.Claim) { c.Ref.ExpiresAt = time.Now() }, "config": func(c *dispatchintent.Claim) { c.ConfigID = "22222222-2222-4222-8222-222222222222" }, "episode": func(c *dispatchintent.Claim) { c.EpisodeID = "" }} {
		t.Run(name, func(t *testing.T) {
			cc := c
			mutate(&cc)
			if _, e := (Codec{}).Decode(b, cc, time.Now()); e == nil {
				t.Fatal("bad pin accepted")
			}
		})
	}
	for _, bad := range []string{`{"a":1,"a":2}`, `{"message_id":"x","MESSAGE_ID":"y"}`, `{} {}`, `{"x":NaN}`, `{"x":null}`, `[]`} {
		v, e := parse([]byte(bad))
		if e == nil && validate(v, schemas["DeliveryIntent"]) {
			t.Fatal("ambiguous message accepted")
		}
	}
}
func TestReceiptCanonicalNamesDuplicatesAndBounds(t *testing.T) {
	for _, disposition := range []string{"APPLIED", "DUPLICATE", "HELD", "CONFLICT"} {
		b := []byte(`{"message_id":"` + testID + `","disposition":"` + disposition + `"}`)
		r, e := decodeReceipt(b, testID)
		if e != nil || r.Disposition != disposition {
			t.Fatal(e)
		}
	}
	for _, b := range []string{`{"message_id":"` + testID + `","MESSAGE_ID":"` + testID + `","disposition":"APPLIED"}`, `{"message_id":"` + testID + `","disposition":"APPLIED","disposition":"DUPLICATE"}`, `{"message_id":"` + testID + `","disposition":"CONFIRMED"}`, `{"message_id":"` + testID + `","disposition":"APPLIED","revision":0}`, `{"message_id":"` + testID + `","disposition":"APPLIED","revision":9007199254740992}`, `{"message_id":"` + testID + `","disposition":"APPLIED","record_id":null}`, `{"message_id":"` + testID + `","disposition":"APPLIED"} {}`} {
		if _, e := decodeReceipt([]byte(b), testID); e == nil {
			t.Fatal("bad receipt accepted", b)
		}
	}
}
