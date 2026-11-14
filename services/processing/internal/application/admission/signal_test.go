package admission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/gowebpki/jcs"
	"strings"
	"testing"
)

// Synthetic values only; digest is computed independently of Decode.
func fixture(t *testing.T, edit func(map[string]any)) string {
	t.Helper()
	m := map[string]any{
		"message_id":     "11111111-1111-4111-8111-111111111111",
		"workspace_id":   "22222222-2222-4222-8222-222222222222",
		"connection_id":  "33333333-3333-4333-8333-333333333333",
		"receipt_id":     "44444444-4444-4444-8444-444444444444",
		"schema_version": "1.0", "stripe_account_id": "acct_synthetic", "livemode": false,
		"session_id": "cs_synthetic", "event_id": "evt_synthetic",
		"event_type":        "checkout.session.completed",
		"source_created_at": "2026-11-14T00:00:00Z", "first_received_at": "2026-11-14T00:00:01Z", "expires_at": "2026-11-15T00:00:00Z",
	}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	canonical, e := jcs.Transform(b)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(canonical)
	m["message_digest"] = "sha256:" + hex.EncodeToString(sum[:])
	b, _ = json.Marshal(m)
	return string(b)
}
func TestCanonicalAndJCS(t *testing.T) {
	raw := fixture(t, nil)
	s, e := Decode([]byte(raw))
	if e != nil || s.Live {
		t.Fatalf("false must remain valid: %v", e)
	}
	// JCS canonicalizes escaped keys/values and insignificant whitespace.
	escaped := strings.ReplaceAll(raw, `cs_synthetic`, `\u0063s_synthetic`)
	if _, e = Decode([]byte(" \n" + escaped + "\t")); e != nil {
		t.Fatal(e)
	}
	for _, event := range []string{"checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		if _, e = Decode([]byte(fixture(t, func(m map[string]any) { m["event_type"] = event }))); e != nil {
			t.Fatal(e)
		}
	}
}
func TestRejectInvalidSignal(t *testing.T) {
	good := fixture(t, nil)
	cases := map[string]string{
		"trailing": good + ` {}`, "array": "[" + good + "]",
		"digest_mismatch":   strings.Replace(good, "evt_synthetic", "evt_other", 1),
		"exact_duplicate":   strings.Replace(good, `"livemode":false`, `"livemode":false,"livemode":false`, 1),
		"case_first":        strings.Replace(good, `"livemode":false`, `"LIVEMODE":true,"livemode":false`, 1),
		"case_last":         strings.Replace(good, `"livemode":false`, `"livemode":false,"LIVEMODE":true`, 1),
		"escaped_duplicate": strings.Replace(good, `"livemode":false`, `"livemode":false,"\u006civemode":false`, 1),
		"surrogate":         strings.Replace(good, `cs_synthetic`, `\ud800`, 1),
		"utf8":              strings.Replace(good, `cs_synthetic`, string([]byte{255}), 1),
	}
	for name := range fields {
		var m map[string]json.RawMessage
		json.Unmarshal([]byte(good), &m)
		delete(m, name)
		b, _ := json.Marshal(m)
		cases["missing_"+name] = string(b)
		json.Unmarshal([]byte(good), &m)
		m[name] = json.RawMessage("null")
		b, _ = json.Marshal(m)
		cases["null_"+name] = string(b)
	}
	for name, value := range map[string]any{"schema_version": "2.0", "event_type": "other", "livemode": 0, "workspace_id": "not-uuid", "event_id": "", "session_id": strings.Repeat("x", 256), "source_created_at": "not-a-time", "unknown": true} {
		cases["schema_"+name] = fixture(t, func(m map[string]any) { m[name] = value })
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, e := Decode([]byte(raw)); e == nil {
				t.Fatal("accepted invalid signal")
			}
		})
	}
}
