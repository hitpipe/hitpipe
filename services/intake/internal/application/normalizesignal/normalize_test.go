package normalizesignal

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
)

const testNow int64 = 1800000000
const secret = "synthetic-only-signing-secret"

type secrets struct{ calls int }

func (s *secrets) Resolve(context.Context, string) ([]byte, error) {
	s.calls++
	return []byte(secret), nil
}

type clock struct{}

func (clock) NowUnix(context.Context) (int64, error) { return testNow, nil }
func fixture() Input {
	mode := false
	return Input{Binding: Binding{InstallationID: "synthetic-endpoint", WorkspaceID: "11111111-1111-4111-8111-111111111111", ConnectionID: "22222222-2222-4222-8222-222222222222", StripeAccountID: "acct_synthetic", Livemode: &mode, APIVersion: "synthetic-version", AccountScope: ConnectedAccount}, Receipt: Receipt{MessageID: "33333333-3333-4333-8333-333333333333", ReceiptID: "44444444-4444-4444-8444-444444444444", FirstReceivedAt: time.Unix(testNow-1, 123).UTC(), ExpiresAt: time.Unix(testNow-1, 123).Add(72 * time.Hour).UTC()}, RawBody: []byte(`{"id":"evt_synthetic","object":"event","account":"acct_synthetic","api_version":"synthetic-version","type":"checkout.session.completed","created":1800000000,"livemode":false,"data":{"object":{"id":"cs_synthetic","object":"checkout.session","created":1700000000,"livemode":false,"metadata":{"brief":"DO_NOT_RETAIN"},"customer_details":{"email":"DO_NOT_RETAIN"},"payment_status":"unpaid"}}}`)}
}
func sign(raw []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(h, "%d.", testNow)
	h.Write(raw)
	return fmt.Sprintf("t=%d,v1=%s", testNow, hex.EncodeToString(h.Sum(nil)))
}
func execute(t *testing.T, in Input) (*Signal, error) {
	t.Helper()
	c, e := New(&secrets{}, clock{})
	if e != nil {
		t.Fatal(e)
	}
	in.Signature = sign(in.RawBody)
	return c.Normalize(context.Background(), in)
}
func TestSupportedEventsIdentityTimeAndDigest(t *testing.T) {
	for _, kind := range []string{"checkout.session.completed", "checkout.session.async_payment_succeeded", "checkout.session.async_payment_failed"} {
		t.Run(kind, func(t *testing.T) {
			in := fixture()
			in.RawBody = []byte(strings.ReplaceAll(string(in.RawBody), "checkout.session.completed", kind))
			s, err := execute(t, in)
			if err != nil {
				t.Fatal(err)
			}
			if s.EventID != "evt_synthetic" || s.SessionID != "cs_synthetic" || s.StripeAccountID != in.Binding.StripeAccountID || s.Livemode || s.WorkspaceID != in.Binding.WorkspaceID || s.ConnectionID != in.Binding.ConnectionID || s.MessageID != in.Receipt.MessageID || s.ReceiptID != in.Receipt.ReceiptID || s.EventType != kind || s.SchemaVersion != "1.0" {
				t.Fatalf("identity changed: %+v", s)
			}
			if s.SourceCreatedAt != time.Unix(1700000000, 0).UTC().Format(time.RFC3339) || s.FirstReceivedAt != in.Receipt.FirstReceivedAt.Format(time.RFC3339Nano) || s.ExpiresAt != in.Receipt.ExpiresAt.Format(time.RFC3339Nano) {
				t.Fatalf("time changed: %+v", s)
			}
			out, _ := json.Marshal(s)
			if strings.Contains(string(out), "DO_NOT_RETAIN") || strings.Contains(string(out), "unpaid") {
				t.Fatal("provider content retained")
			}
			var fields map[string]json.RawMessage
			json.Unmarshal(out, &fields)
			if len(fields) != 14 {
				t.Fatalf("wire field count=%d", len(fields))
			}
			delete(fields, "message_digest")
			payload, _ := json.Marshal(fields)
			canon, e := jcs.Transform(payload)
			if e != nil {
				t.Fatal(e)
			}
			h := sha256.Sum256(canon)
			if s.MessageDigest != "sha256:"+hex.EncodeToString(h[:]) {
				t.Fatal("digest does not exclude digest field")
			}
			// Formatting differences preserve signed semantic projection and digest.
			in.RawBody = []byte(" \n" + string(in.RawBody) + "\t")
			again, e := execute(t, in)
			if e != nil || *again != *s {
				t.Fatal("format changes semantic signal", e)
			}
		})
	}
}
func TestRejectMalformedAndAmbiguousSignedInput(t *testing.T) {
	cases := map[string]string{
		"empty": "", "null": "null", "array": "[]", "trailing": string(fixture().RawBody) + " {}", "duplicate": strings.Replace(string(fixture().RawBody), `"id":"evt_synthetic"`, `"id":"evt_synthetic","id":"other"`, 1),
		"escaped-duplicate":          strings.Replace(string(fixture().RawBody), `"id":"evt_synthetic"`, `"id":"evt_synthetic","\u0069d":"other"`, 1),
		"unknown":                    strings.Replace(string(fixture().RawBody), `"id":"evt_synthetic"`, `"unknown":true,"id":"evt_synthetic"`, 1),
		"alias":                      strings.Replace(string(fixture().RawBody), `"id":"evt_synthetic"`, `"ID":"evt_synthetic"`, 1),
		"nested-unknown":             strings.Replace(string(fixture().RawBody), `"id":"cs_synthetic"`, `"brief":"no","id":"cs_synthetic"`, 1),
		"unknown-event":              strings.Replace(string(fixture().RawBody), "checkout.session.completed", "invoice.paid", 1),
		"wrong-account":              strings.Replace(string(fixture().RawBody), "acct_synthetic", "acct_other", 1),
		"null-account":               strings.Replace(string(fixture().RawBody), `"account":"acct_synthetic"`, `"account":null`, 1),
		"wrong-api":                  strings.Replace(string(fixture().RawBody), "synthetic-version", "other-version", 1),
		"wrong-object":               strings.Replace(string(fixture().RawBody), `"object":"checkout.session"`, `"object":"payment_intent"`, 1),
		"event-mode":                 strings.Replace(string(fixture().RawBody), `"livemode":false`, `"livemode":true`, 1),
		"session-mode":               strings.Replace(string(fixture().RawBody), `"created":1700000000,"livemode":false`, `"created":1700000000,"livemode":true`, 1),
		"session-time-missing":       strings.Replace(string(fixture().RawBody), `"created":1700000000,`, "", 1),
		"session-time-null":          strings.Replace(string(fixture().RawBody), `"created":1700000000`, `"created":null`, 1),
		"fraction":                   strings.Replace(string(fixture().RawBody), `"created":1700000000`, `"created":1700000000.1`, 1),
		"negative":                   strings.Replace(string(fixture().RawBody), `"created":1700000000`, `"created":-1`, 1),
		"zero":                       strings.Replace(string(fixture().RawBody), `"created":1700000000`, `"created":0`, 1),
		"overflow":                   strings.Replace(string(fixture().RawBody), `"created":1700000000`, `"created":9007199254740992`, 1),
		"surrogate":                  strings.Replace(string(fixture().RawBody), "cs_synthetic", `\ud800`, 1),
		"invalid-utf8":               strings.Replace(string(fixture().RawBody), "cs_synthetic", string([]byte{255}), 1),
		"duplicate-ignored-metadata": strings.Replace(string(fixture().RawBody), `"brief":"DO_NOT_RETAIN"`, `"brief":"DO_NOT_RETAIN","brief":"x"`, 1),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			in := fixture()
			in.RawBody = []byte(raw)
			s, e := execute(t, in)
			if e == nil || s != nil {
				t.Fatalf("accepted malformed input: %+v", s)
			}
		})
	}
}
func TestRequiredFieldsAbsentNullWrongType(t *testing.T) {
	for _, scope := range []string{"event", "session"} {
		for _, key := range []string{"id", "object", "created", "livemode"} {
			for _, value := range []string{"absent", "null", "{}"} {
				t.Run(scope+"/"+key+"/"+value, func(t *testing.T) {
					in := fixture()
					var root map[string]any
					json.Unmarshal(in.RawBody, &root)
					m := root
					if scope == "session" {
						m = root["data"].(map[string]any)["object"].(map[string]any)
					}
					if value == "absent" {
						delete(m, key)
					} else if value == "null" {
						m[key] = nil
					} else {
						m[key] = map[string]any{}
					}
					in.RawBody, _ = json.Marshal(root)
					s, e := execute(t, in)
					if e == nil || s != nil {
						t.Fatal("required-field violation accepted")
					}
				})
			}
		}
	}
	for _, key := range []string{"api_version", "type", "data"} {
		t.Run(key, func(t *testing.T) {
			in := fixture()
			var root map[string]any
			json.Unmarshal(in.RawBody, &root)
			delete(root, key)
			in.RawBody, _ = json.Marshal(root)
			s, e := execute(t, in)
			if e == nil || s != nil {
				t.Fatal("missing required event field accepted")
			}
		})
	}
}
func TestUnknownBindingsAndReceiptRejected(t *testing.T) {
	cases := map[string]func(*Input){
		"installation": func(i *Input) { i.Binding.InstallationID = "" }, "workspace": func(i *Input) { i.Binding.WorkspaceID = "" }, "connection": func(i *Input) { i.Binding.ConnectionID = "bad" }, "account": func(i *Input) { i.Binding.StripeAccountID = "" }, "unknown-mode": func(i *Input) { i.Binding.Livemode = nil }, "unknown-api": func(i *Input) { i.Binding.APIVersion = "" }, "unknown-scope": func(i *Input) { i.Binding.AccountScope = "" }, "missing-message": func(i *Input) { i.Receipt.MessageID = "" }, "non-v4-message": func(i *Input) { i.Receipt.MessageID = "33333333-3333-1333-8333-333333333333" }, "missing-receipt": func(i *Input) { i.Receipt.ReceiptID = "" }, "missing-first": func(i *Input) { i.Receipt.FirstReceivedAt = time.Time{} }, "missing-expiry": func(i *Input) { i.Receipt.ExpiresAt = time.Time{} }, "expiry-boundary": func(i *Input) { i.Receipt.ExpiresAt = i.Receipt.FirstReceivedAt }, "ttl-exceeded": func(i *Input) { i.Receipt.ExpiresAt = i.Receipt.ExpiresAt.Add(time.Nanosecond) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			in := fixture()
			change(&in)
			s, e := execute(t, in)
			if e == nil || s != nil {
				t.Fatal("unknown binding/receipt accepted")
			}
		})
	}
}
func TestEachRepeatAuthenticatesBeforeParsing(t *testing.T) {
	sec := &secrets{}
	c, e := New(sec, clock{})
	if e != nil {
		t.Fatal(e)
	}
	in := fixture()
	in.Signature = sign(in.RawBody)
	first, e := c.Normalize(context.Background(), in)
	if e != nil {
		t.Fatal(e)
	}
	again, e := c.Normalize(context.Background(), in)
	if e != nil || *first != *again || sec.calls != 2 {
		t.Fatal("repeat changed identity or skipped authentication", e)
	}
	in.Signature = "t=1800000000,v1=" + strings.Repeat("0", 64)
	s, e := c.Normalize(context.Background(), in)
	if e == nil || s != nil || e.Error() != "authentication_failed" || sec.calls != 3 {
		t.Fatal("forged repeat accepted")
	}
	in.RawBody = []byte(`not json`)
	in.Signature = "invalid"
	_, e = c.Normalize(context.Background(), in)
	if e == nil || e.Error() != "authentication_failed" {
		t.Fatal("parsed before authentication")
	}
}
func TestDirectAccountRequiresExplicitBindingAndAbsentAccount(t *testing.T) {
	in := fixture()
	in.Binding.AccountScope = DirectAccount
	s, e := execute(t, in)
	if e == nil || s != nil {
		t.Fatal("direct accepted foreign account field")
	}
	in.RawBody = []byte(strings.Replace(string(in.RawBody), `"account":"acct_synthetic",`, "", 1))
	s, e = execute(t, in)
	if e != nil || s.StripeAccountID != in.Binding.StripeAccountID {
		t.Fatal("explicit direct binding rejected", e)
	}
	in.Binding.AccountScope = ConnectedAccount
	s, e = execute(t, in)
	if e == nil || s != nil {
		t.Fatal("connected account inferred")
	}
}
func TestSizeLimitAndCancellation(t *testing.T) {
	in := fixture()
	base := string(in.RawBody)
	in.RawBody = []byte(base + strings.Repeat(" ", MaxBodyBytes-len(base)))
	if _, e := execute(t, in); e != nil {
		t.Fatal("exact limit rejected", e)
	}
	in.RawBody = append(in.RawBody, ' ')
	if s, e := execute(t, in); e == nil || s != nil {
		t.Fatal("oversize accepted")
	}
	c, e := New(&secrets{}, clock{})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	in = fixture()
	in.Signature = sign(in.RawBody)
	if s, e := c.Normalize(ctx, in); e == nil || s != nil {
		t.Fatal("cancelled accepted")
	}
}
func TestIdentifiersAreOpaqueButBounded(t *testing.T) {
	for _, n := range []int{0, 255, 256} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			in := fixture()
			in.RawBody = []byte(strings.Replace(string(in.RawBody), "cs_synthetic", strings.Repeat("x", n), 1))
			s, e := execute(t, in)
			if n == 255 {
				if e != nil || s == nil {
					t.Fatal(e)
				}
			} else if e == nil || s != nil {
				t.Fatal("invalid length accepted")
			}
		})
	}
}
func TestFailClosedConfigurationAndDependencyErrors(t *testing.T) {
	var unconfigured Consumer
	in := fixture()
	if s, e := unconfigured.Normalize(context.Background(), in); e == nil || s != nil {
		t.Fatal("zero consumer accepted")
	}
	var sec *secrets
	if _, e := New(sec, clock{}); e == nil {
		t.Fatal("typed nil accepted")
	}
	c, e := New(errorSecrets{}, clock{})
	if e != nil {
		t.Fatal(e)
	}
	in.Signature = sign(in.RawBody)
	if s, e := c.Normalize(context.Background(), in); e == nil || s != nil || strings.Contains(e.Error(), "DO_NOT_LEAK") {
		t.Fatal("dependency error exposed")
	}
	for _, change := range []func(*Input){func(i *Input) { i.Receipt.FirstReceivedAt = time.Unix(testNow+1, 0) }, func(i *Input) {
		i.Receipt.FirstReceivedAt = time.Unix(testNow-100, 0)
		i.Receipt.ExpiresAt = time.Unix(testNow, 0)
	}} {
		in = fixture()
		change(&in)
		if s, e := execute(t, in); e == nil || s != nil {
			t.Fatal("future or expired receipt accepted")
		}
	}
}

type errorSecrets struct{}

func (errorSecrets) Resolve(context.Context, string) ([]byte, error) {
	return nil, fmt.Errorf("DO_NOT_LEAK")
}
func TestLiveModeAndUnknownFieldSpelling(t *testing.T) {
	in := fixture()
	mode := true
	in.Binding.Livemode = &mode
	in.RawBody = []byte(strings.ReplaceAll(string(in.RawBody), `"livemode":false`, `"livemode":true`))
	if s, e := execute(t, in); e != nil || !s.Livemode {
		t.Fatal("live rejected", e)
	}
	for _, key := range []string{"id object", "", "ID", "brief"} {
		in = fixture()
		var m map[string]any
		json.Unmarshal(in.RawBody, &m)
		m[key] = "excluded"
		in.RawBody, _ = json.Marshal(m)
		if s, e := execute(t, in); e == nil || s != nil {
			t.Fatal("unknown field accepted", key)
		}
	}
}

// The exact schema is a drift oracle, not a new authority.
func TestOutputAgainstPinnedSchema(t *testing.T) {
	raw, e := os.ReadFile("normalized_signal.schema.json")
	if e != nil {
		t.Fatal(e)
	}
	var schema struct {
		AdditionalProperties bool     `json:"additionalProperties"`
		Required             []string `json:"required"`
		Properties           map[string]struct {
			Type      string   `json:"type"`
			Format    string   `json:"format"`
			Pattern   string   `json:"pattern"`
			Const     string   `json:"const"`
			Enum      []string `json:"enum"`
			MinLength int      `json:"minLength"`
			MaxLength int      `json:"maxLength"`
		} `json:"properties"`
	}
	if e = json.Unmarshal(raw, &schema); e != nil {
		t.Fatal(e)
	}
	s, e := execute(t, fixture())
	if e != nil {
		t.Fatal(e)
	}
	out, _ := json.Marshal(s)
	var m map[string]any
	json.Unmarshal(out, &m)
	for _, k := range schema.Required {
		if _, ok := m[k]; !ok {
			t.Fatal("required output missing", k)
		}
	}
	if schema.AdditionalProperties {
		t.Fatal("unexpected contract expansion")
	}
	for k, v := range m {
		p, ok := schema.Properties[k]
		if !ok {
			t.Fatal("unknown output", k)
		}
		if p.Type == "boolean" {
			if _, ok := v.(bool); !ok {
				t.Fatal("wrong bool", k)
			}
			continue
		}
		str, ok := v.(string)
		if !ok {
			t.Fatal("wrong string", k)
		}
		n := len([]rune(str))
		if n < p.MinLength || (p.MaxLength > 0 && n > p.MaxLength) {
			t.Fatal("length", k)
		}
		if p.Const != "" && str != p.Const {
			t.Fatal("const", k)
		}
		if len(p.Enum) > 0 {
			found := false
			for _, x := range p.Enum {
				if str == x {
					found = true
				}
			}
			if !found {
				t.Fatal("enum", k)
			}
		}
		if p.Pattern != "" && !regexp.MustCompile(p.Pattern).MatchString(str) {
			t.Fatal("pattern", k)
		}
		if p.Format == "uuid" && !uuid(str, false) {
			t.Fatal("uuid", k)
		}
		if p.Format == "date-time" {
			if _, e := time.Parse(time.RFC3339Nano, str); e != nil {
				t.Fatal("date-time", k)
			}
		}
	}
}
