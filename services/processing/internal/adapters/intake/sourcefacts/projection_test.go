package sourcefacts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

func inputs(t *testing.T) ([]byte, Expected, eligibility.Policy, time.Time) {
	t.Helper()
	b, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	live := false
	e := Expected{CaseID: "11111111-1111-4111-8111-111111111111", SessionID: "cs_synthetic", AccountID: "acct_synthetic", ConnectionID: "22222222-2222-4222-8222-222222222222", LiveMode: false}
	at := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	p := eligibility.Policy{ConfigID: "local-config", SchemaVersion: "1.0", EvaluatorVersion: eligibility.EvaluatorVersion, AccountID: e.AccountID, ConnectionID: e.ConnectionID, LiveMode: &live, PriceID: "price_fixed", ActivationAt: at}
	return b, e, p, time.Date(2026, 10, 18, 10, 0, 5, 0, time.UTC)
}
func change(t *testing.T, b []byte, k string, v any) []byte {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	m[k] = v
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func reject(t *testing.T, b []byte, e Expected, p eligibility.Policy) {
	t.Helper()
	got, err := Decode(b, e, p)
	if err == nil || got.Snapshot.ConfigID != "" || got.Provenance.FactsID != "" || got.Snapshot.Facts.LiveMode != nil {
		t.Fatalf("accepted or leaked partial result: %+v %v", got, err)
	}
	if err.Error() != "INVALID_SOURCE_FACTS" {
		t.Fatalf("unsafe diagnostic: %v", err)
	}
}
func TestProjectionPreservesFactsAndLocalPins(t *testing.T) {
	b, e, p, now := inputs(t)
	// Preserve original lexical times independently from parsed domain times.
	b = change(t, b, "oldest_observed_at", "2026-10-18T12:00:00.000+02:00")
	got, err := Decode(b, e, p)
	if err != nil {
		t.Fatal(err)
	}
	f := got.Snapshot.Facts
	if f.AmountRefunded == nil || *f.AmountRefunded != 0 || f.Disputed == nil || *f.Disputed || f.LiveMode == nil || *f.LiveMode {
		t.Fatal("lost canonical zero/false")
	}
	if got.Provenance.FactsID != "33333333-3333-4333-8333-333333333333" || got.Provenance.Digest != "sha256:"+strings.Repeat("a", 64) || got.Provenance.OldestObservedAt != "2026-10-18T12:00:00.000+02:00" || got.Provenance.CompletedAt != "2026-10-18T10:00:01Z" || got.Provenance.ValidUntil != "2026-10-18T10:00:10Z" || got.Provenance.SessionCreatedAt == nil || *got.Provenance.SessionCreatedAt != "2026-10-18T09:00:00Z" {
		t.Fatalf("lost provenance: %+v", got.Provenance)
	}
	if got.Snapshot.ConfigID != p.ConfigID || got.Snapshot.SchemaVersion != p.SchemaVersion || got.Snapshot.EvaluatorVersion != p.EvaluatorVersion {
		t.Fatal("lost local pins")
	}
	if r := eligibility.Evaluate(p, f, now); r.Status != eligibility.Valid {
		t.Fatal(r)
	}
	// Each call owns its pointers and does not retain caller bytes or Policy pointer.
	*got.Snapshot.Facts.AmountRefunded = 1
	next, err := Decode(b, e, p)
	if err != nil || *next.Snapshot.Facts.AmountRefunded != 0 {
		t.Fatal("shared projection")
	}
	clear(b)
	if *next.Snapshot.Facts.AmountTotal != 12000 {
		t.Fatal("retained input")
	}
}
func TestEveryRequiredFieldAndNullableValue(t *testing.T) {
	b, e, p, _ := inputs(t)
	for k, r := range wireFields {
		t.Run(k, func(t *testing.T) {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(b, &m)
			delete(m, k)
			missing, _ := json.Marshal(m)
			reject(t, missing, e, p)
			null := change(t, b, k, nil)
			if r.nullable {
				got, err := Decode(null, e, p)
				if err != nil {
					t.Fatal(err)
				}
				if k == "session_created_at" && (got.Provenance.SessionCreatedAt != nil || got.Snapshot.Facts.SessionCreatedAt != nil) {
					t.Fatal("null fabricated")
				}
			} else {
				reject(t, null, e, p)
			}
			wrong := any([]any{1})
			reject(t, change(t, b, k, wrong), e, p)
		})
	}
}
func TestInvalidScalars(t *testing.T) {
	b, e, p, _ := inputs(t)
	for k, r := range wireFields {
		if r.kind == "integer" {
			for _, v := range []string{"-1", "1.2", "1e100", "9007199254740992", "9223372036854775808", "true", "\"1\""} {
				t.Run(k+v, func(t *testing.T) {
					var m map[string]json.RawMessage
					_ = json.Unmarshal(b, &m)
					m[k] = json.RawMessage(v)
					raw, _ := json.Marshal(m)
					reject(t, raw, e, p)
				})
			}
		}
		if len(r.choices) > 0 {
			reject(t, change(t, b, k, "invented"), e, p)
		}
	}
	for k, vs := range map[string][]any{
		"case_id": {"not-uuid"}, "facts_id": {"bad"}, "connection_id": {"bad"}, "session_id": {"", strings.Repeat("s", 256)}, "stripe_account_id": {"", strings.Repeat("a", 256)}, "price_id": {"", strings.Repeat("p", 256)}, "currency": {"USD", "us", "u$d"}, "digest": {"sha256:bad", "sha256:" + strings.Repeat("A", 64)}, "oldest_observed_at": {"2026-02-30T10:00:00Z", "2026-10-18", "2026-10-18T10:00:00+24:00"}, "complete": {"true", 1},
	} {
		for _, v := range vs {
			reject(t, change(t, b, k, v), e, p)
		}
	}
}
func TestStrictCanonicalJSON(t *testing.T) {
	b, e, p, _ := inputs(t)
	for _, raw := range []string{
		`null`, `[]`, `{}`, `{"case_id":`, string(b) + ` {}`, string(b) + ` false`,
		strings.TrimSpace(string(b)) + `!`,
	} {
		reject(t, []byte(raw), e, p)
	}
	// Reject exact duplicates and aliases regardless of order, including refund shadowing.
	for k := range wireFields {
		var m map[string]json.RawMessage
		_ = json.Unmarshal(b, &m)
		prefix := []byte(`"` + k + `":` + string(m[k]) + `,`)
		reject(t, append(append([]byte{'{'}, prefix...), b[1:]...), e, p)
		alias := strings.ToUpper(k)
		reject(t, change(t, b, alias, 0), e, p)
	}
	for _, pair := range []string{`"amount_refunded":1,"AMOUNT_REFUNDED":0`, `"AMOUNT_REFUNDED":0,"amount_refunded":1`} {
		clean := bytes.ReplaceAll(b, []byte(`"amount_refunded": 0`), []byte(pair))
		reject(t, clean, e, p)
	}
	reject(t, change(t, b, "policy", map[string]any{"config_id": "remote"}), e, p)
	reject(t, change(t, b, "account_id", "acct_synthetic"), e, p)
	reject(t, append([]byte{'{'}, []byte{0xff, '}'}...), e, p)
}
func TestExpectedScopeAndPolicy(t *testing.T) {
	b, e, p, _ := inputs(t)
	for _, edit := range []func(*Expected){func(v *Expected) { v.CaseID = "other" }, func(v *Expected) { v.SessionID = "other" }, func(v *Expected) { v.AccountID = "other" }, func(v *Expected) { v.ConnectionID = "other" }, func(v *Expected) { v.LiveMode = true }} {
		wrong := e
		edit(&wrong)
		reject(t, b, wrong, p)
	}
	for k, v := range map[string]any{"case_id": "44444444-4444-4444-8444-444444444444", "session_id": "other", "stripe_account_id": "other", "connection_id": "44444444-4444-4444-8444-444444444444", "livemode": true} {
		reject(t, change(t, b, k, v), e, p)
	}
	p.LiveMode = nil
	reject(t, b, e, p)
}

type testReader struct {
	data   []byte
	policy eligibility.Policy
	calls  int
	err    error
	cancel context.CancelFunc
}

func (r *testReader) Read(ctx context.Context, q evaluatepaidorder.Query) (evaluatepaidorder.Snapshot, error) {
	r.calls++
	if r.cancel != nil {
		r.cancel()
	}
	if r.err != nil {
		return evaluatepaidorder.Snapshot{}, r.err
	}
	if err := ctx.Err(); err != nil {
		return evaluatepaidorder.Snapshot{}, err
	}
	p, err := Decode(r.data, Expected{CaseID: q.CaseID, SessionID: q.SessionID, AccountID: q.AccountID, ConnectionID: q.ConnectionID, LiveMode: q.LiveMode}, r.policy)
	return p.Snapshot, err
}
func TestApplicationComposition(t *testing.T) {
	b, e, p, now := inputs(t)
	for _, tt := range []struct {
		name   string
		data   []byte
		now    time.Time
		status eligibility.Status
		reason string
	}{
		{"normal", b, now, eligibility.Valid, "FINANCIAL_ELIGIBILITY_ONLY"},
		{"refund", change(t, b, "amount_refunded", 1), now, eligibility.Ineligible, "REFUND"},
		{"null", change(t, b, "amount_received", nil), now, eligibility.Incomplete, "UNKNOWN_FACT"},
		{"unknown enum", change(t, b, "payment_status", "UNKNOWN"), now, eligibility.Incomplete, "UNKNOWN_FACT"},
		{"expiry boundary", b, now.Add(5 * time.Second), eligibility.RefreshRequired, "PROOF_EXPIRED"},
		{"absent", []byte(`{}`), now, eligibility.Incomplete, "READER_UNAVAILABLE"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reader := &testReader{data: tt.data, policy: p}
			r := evaluatepaidorder.EvaluatePaidOrder(context.Background(), evaluatepaidorder.Request{Policy: p, CaseID: e.CaseID, SessionID: e.SessionID, Now: tt.now}, reader)
			if r.Status != tt.status || r.Reason != tt.reason || reader.calls != 1 {
				t.Fatalf("%+v calls=%d", r, reader.calls)
			}
		})
	}
	for _, mode := range []string{"error", "cancel-before", "cancel-during"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &testReader{data: b, policy: p}
			calls := 1
			switch mode {
			case "error":
				reader.err = errors.New("synthetic failure")
			case "cancel-before":
				cancel()
				calls = 0
			case "cancel-during":
				reader.cancel = cancel
			}
			r := evaluatepaidorder.EvaluatePaidOrder(ctx, evaluatepaidorder.Request{Policy: p, CaseID: e.CaseID, SessionID: e.SessionID, Now: now}, reader)
			if r.Status != eligibility.Incomplete || r.Reason != "READER_UNAVAILABLE" || reader.calls != calls {
				t.Fatalf("%+v calls=%d", r, reader.calls)
			}
		})
	}
}

func TestEquivalentJSONIntegers(t *testing.T) {
	b, e, p, now := inputs(t)
	for _, pair := range [][2]string{{`"amount_total": 12000`, `"amount_total": 12000.0`}, {`"quantity": 1`, `"quantity": 1e0`}, {`"amount_refunded": 0`, `"amount_refunded": -0.0`}} {
		raw := bytes.Replace(b, []byte(pair[0]), []byte(pair[1]), 1)
		got, err := Decode(raw, e, p)
		if err != nil {
			t.Fatal(err)
		}
		if r := eligibility.Evaluate(p, got.Snapshot.Facts, now); r.Status != eligibility.Valid {
			t.Fatal(r)
		}
	}
	for _, raw := range []string{"1e-10000", "9007199254740991.1", "1.0000000000000000001", "NaN", "1e999999999"} {
		if _, ok := integer([]byte(raw)); ok {
			t.Fatal("invalid integer accepted:", raw)
		}
	}
}

func TestDomainProjectionMatchesIndependentFixture(t *testing.T) {
	b, e, p, _ := inputs(t)
	raw, err := os.ReadFile("../../../../testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Facts eligibility.Facts `json:"facts"`
	}
	if err = json.Unmarshal(raw, &oracle); err != nil {
		t.Fatal(err)
	}
	oracle.Facts.CaseID = e.CaseID
	oracle.Facts.ConnectionID = e.ConnectionID
	got, err := Decode(b, e, p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(oracle.Facts, got.Snapshot.Facts) {
		t.Fatalf("lost/remapped financial fact: want %+v got %+v", oracle.Facts, got.Snapshot.Facts)
	}
}
