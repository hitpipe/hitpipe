package sourcefacts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRequestStrictShapeAndIntegerSemantics(t *testing.T) {
	q := Request{WorkspaceID: "11111111-1111-4111-8111-111111111111", ConnectionID: "22222222-2222-4222-8222-222222222222", CaseID: "33333333-3333-4333-8333-333333333333", SessionID: "opaque_session", ExpectedEpoch: 1, DeadlineAt: time.Now().Add(time.Minute).UTC()}
	b, _ := json.Marshal(q)
	for _, n := range []string{"1", "1.0", "1e0"} {
		s := strings.Replace(string(b), `"expected_epoch":1`, `"expected_epoch":`+n, 1)
		if v, e := DecodeRequest([]byte(s)); e != nil || v.ExpectedEpoch != 1 {
			t.Fatal("mathematical integer rejected", n, e)
		}
	}
	for _, s := range []string{strings.Replace(string(b), `"expected_epoch":1`, `"expected_epoch":1.1`, 1), strings.Replace(string(b), `"case_id":`, `"Case_id":`, 1), strings.Replace(string(b), `"case_id":`, `"unknown":true,"case_id":`, 1), strings.Replace(string(b), `"case_id":`, `"case_id":"duplicate","case_id":`, 1), strings.Replace(string(b), `"session_id":"opaque_session"`, `"session_id":null`, 1), string(b) + `{}`} {
		if _, e := DecodeRequest([]byte(s)); e == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
func TestDigestRoundtripAndTampering(t *testing.T) {
	id, _ := NewID()
	f := Facts{FactsID: id, CaseID: id, ConnectionID: id, SessionID: "opaque", StripeAccountID: "acct_synthetic", PaymentIntentStatus: "UNKNOWN", OldestObservedAt: time.Now().UTC().Truncate(time.Microsecond)}
	f.CompletedAt = f.OldestObservedAt
	f.ValidUntil = f.OldestObservedAt.Add(10 * time.Second)
	f.Digest, _ = FactsDigest(f)
	b, e := Encode(f)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Decode(b); e != nil {
		t.Fatal(e)
	}
	mutated := strings.Replace(string(b), `"complete":false`, `"complete":true`, 1)
	if _, e = Decode([]byte(mutated)); e == nil {
		t.Fatal("tampered digest accepted")
	}
	if strings.Contains(string(b), `"workspace_id"`) {
		t.Fatal("invented wire property")
	}
}

func TestIntegerRejectsStringsAndHugeExponent(t *testing.T) {
	for _, s := range []string{`"1"`, `1e999999999`, `1e-999999999`, `0.1`, `9007199254740992`, `true`, `null`} {
		if _, ok := Integer([]byte(s)); ok {
			t.Fatal("not strict safe integer", s)
		}
	}
	if n, ok := Integer([]byte(`0e999999999`)); !ok || n != 0 {
		t.Fatal("zero exponent")
	}
}
