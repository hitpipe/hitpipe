package intent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

func fixture(t *testing.T) (preparecase.Case, preparecase.Decision) {
	t.Helper()
	live := false
	activation, _ := time.Parse(time.RFC3339, "2026-10-18T09:00:00Z")
	now, _ := time.Parse(time.RFC3339, "2026-10-18T10:00:02Z")
	policy := eligibility.Policy{ConfigID: "44444444-4444-4444-8444-444444444444", SchemaVersion: "1.0", EvaluatorVersion: "paid-order-v1", AccountID: "acct_synthetic", ConnectionID: "22222222-2222-4222-8222-222222222222", LiveMode: &live, PriceID: "price_fixed", ActivationAt: activation}
	c := preparecase.Case{Request: preparecase.Request{WorkspaceID: "55555555-5555-4555-8555-555555555555", CaseID: "11111111-1111-4111-8111-111111111111", Epoch: 7}, OperationID: "66666666-6666-4666-8666-666666666666", ConfigID: policy.ConfigID, ConfigDigest: "sha256:" + strings.Repeat("a", 64), SessionID: "cs_synthetic", Policy: policy, Mapping: preparecase.Mapping{SourceConnectionID: policy.ConnectionID, DestinationConnectionID: "77777777-7777-4777-8777-777777777777", DestinationWorkspaceID: "cu_workspace", ListID: "list_fixed", AssigneeID: "assignee_fixed", ServiceLabel: "Synthetic service", TitlePattern: "SERVICE_ORDER", RequiredKeys: []string{}}}
	raw, e := os.ReadFile("../../intake/sourcefacts/testdata/valid.json")
	if e != nil {
		t.Fatal(e)
	}
	p, e := sourcefacts.Decode(raw, sourcefacts.Expected{CaseID: c.CaseID, SessionID: c.SessionID, AccountID: policy.AccountID, ConnectionID: policy.ConnectionID}, policy)
	if e != nil {
		t.Fatal(e)
	}
	v := p.Provenance
	p.Snapshot.Proof = &evaluatepaidorder.SourceProof{FactsID: v.FactsID, Digest: v.Digest, SessionCreatedAt: v.SessionCreatedAt, OldestObservedAt: v.OldestObservedAt, CompletedAt: v.CompletedAt, ValidUntil: v.ValidUntil}
	return c, preparecase.Decision{Financial: eligibility.Evaluate(policy, p.Snapshot.Facts, now), Snapshot: p.Snapshot, At: now}
}
func TestGoldenRenderingAndDigest(t *testing.T) {
	c, d := fixture(t)
	c.Mapping.RequiredKeys = []string{"z", "a"}
	values := map[string]string{"a": "first\nline", "z": "é"}
	fields := []preparecase.ContextField{{Key: "z", Value: "é", AddedAt: d.At, ExpiresAt: d.At.Add(time.Hour)}, {Key: "a", Value: "first\nline", AddedAt: d.At, ExpiresAt: d.At.Add(time.Hour)}}
	p, e := (Renderer{}).Build(c, d, values, fields)
	if e != nil {
		t.Fatal(e)
	}
	var got preparecase.DeliveryIntent
	if json.Unmarshal(p.Body, &got) != nil {
		t.Fatal("invalid intent JSON")
	}
	want := "[HITPIPE-HANDOFF:11111111-1111-4111-8111-111111111111:v1]\nService: Synthetic service\nOrder: cs_synthetic\nContext:\na: first\nline\nz: é\n[/HITPIPE-HANDOFF:11111111-1111-4111-8111-111111111111:v1]"
	if got.RenderedHandoffBlock != want {
		t.Fatal(got.RenderedHandoffBlock)
	}
	if got.OrderReference != "HP-CEIRCEIRCFARDAIRCEIRCEIRCE" {
		t.Fatal("public reference", got.OrderReference)
	}
	if got.Title != "Synthetic service — "+got.OrderReference || got.ConnectionEpoch != 7 || got.SourceFactsDigest != d.Snapshot.Proof.Digest || got.SourceFactsId != d.Snapshot.Proof.FactsID {
		t.Fatal("contract/provenance changed")
	}
	var m map[string]json.RawMessage
	json.Unmarshal(p.Body, &m)
	delete(m, "message_digest")
	b, _ := json.Marshal(m)
	b, e = jcs.Transform(b)
	if e != nil || Digest(b) != p.Digest {
		t.Fatal("not RFC8785 unsigned message digest")
	}
}
func TestRenderingRejectsAmbiguityAndExpiry(t *testing.T) {
	for _, bad := range []string{"[HITPIPE-HANDOFF:fake]", "ok\n[/HITPIPE-HANDOFF:fake]", "e\u0301", "a\r\nb", "a\tb", string([]byte{255}), strings.Repeat("x", 32768)} {
		t.Run("context", func(t *testing.T) {
			c, d := fixture(t)
			c.Mapping.RequiredKeys = []string{"brief"}
			_, e := (Renderer{}).Build(c, d, map[string]string{"brief": bad}, []preparecase.ContextField{{Key: "brief", Value: bad, AddedAt: d.At, ExpiresAt: d.At.Add(time.Hour)}})
			if e == nil {
				t.Fatal("unsafe or oversized context allowed")
			}
		})
	}
	c, d := fixture(t)
	d.At = d.Snapshot.Facts.ValidUntil
	if _, e := (Renderer{}).Build(c, d, map[string]string{}, nil); e == nil {
		t.Fatal("expired proof allowed")
	}
	c, d = fixture(t)
	d.Snapshot.Proof = nil
	if _, e := (Renderer{}).Build(c, d, map[string]string{}, nil); e == nil {
		t.Fatal("unknown provenance allowed")
	}
	n, e := Normalized("e\u0301\r\nlast\rline")
	if e != nil || n != "é\nlast\nline" {
		t.Fatal(n, e)
	}
}
