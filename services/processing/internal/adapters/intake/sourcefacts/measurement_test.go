package sourcefacts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
)

// All data is synthetic. Construction/IO is outside benchmark timing. The same
// corpus and harness must be used on both revisions; Decode is not substituted.
type measuredCase struct {
	name     string
	raw      []byte
	expected Expected
	policy   eligibility.Policy
	accept   bool
}

func measurementCases(tb testing.TB) []measuredCase {
	tb.Helper()
	raw, err := os.ReadFile("testdata/valid.json")
	if err != nil {
		tb.Fatal(err)
	}
	live := false
	e := Expected{CaseID: "11111111-1111-4111-8111-111111111111", SessionID: "cs_synthetic", AccountID: "acct_synthetic", ConnectionID: "22222222-2222-4222-8222-222222222222"}
	p := eligibility.Policy{ConfigID: "local-config", SchemaVersion: "1.0", EvaluatorVersion: eligibility.EvaluatorVersion, AccountID: e.AccountID, ConnectionID: e.ConnectionID, LiveMode: &live, PriceID: "price_fixed", ActivationAt: time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)}
	cases := []measuredCase{}
	add := func(name string, b []byte, ok bool) { cases = append(cases, measuredCase{name, b, e, p, ok}) }
	replace := func(k, v string) []byte {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			tb.Fatal(err)
		}
		if v == "MISSING" {
			delete(m, k)
		} else {
			m[k] = json.RawMessage(v)
		}
		b, err := json.Marshal(m)
		if err != nil {
			tb.Fatal(err)
		}
		return b
	}
	add("valid", raw, true)
	add("nullable", replace("currency", "null"), true)
	add("known_zero", replace("amount_refunded", "0"), true)
	add("known_false", replace("disputed", "false"), true)
	add("integer_decimal", replace("quantity", "1.0"), true)
	add("integer_exponent", replace("quantity", "1e0"), true)
	add("boundary_max", replace("amount_total", "9007199254740991"), true)
	add("boundary_over", replace("amount_total", "9007199254740992"), false)
	add("fraction", replace("amount_total", "9007199254740991.1"), false)
	add("unknown", replace("unknown", "0"), false)
	for i, pair := range []string{`"amount_refunded":1,"AMOUNT_REFUNDED":0`, `"AMOUNT_REFUNDED":0,"amount_refunded":1`} {
		add(fmt.Sprintf("alias_order_%d", i), bytes.Replace(raw, []byte(`"amount_refunded": 0`), []byte(pair), 1), false)
	}
	add("duplicate", append([]byte(`{"amount_refunded":1,`), raw[1:]...), false)
	keys := make([]string, 0, len(wireFields))
	for k := range wireFields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var original map[string]json.RawMessage
	_ = json.Unmarshal(raw, &original)
	for _, k := range keys {
		r := wireFields[k]
		add(k+"/missing", replace(k, "MISSING"), false)
		add(k+"/null", replace(k, "null"), r.nullable)
		add(k+"/wrong_shape", replace(k, `[1]`), false)
		add(k+"/alias", replace(strings.ToUpper(k), string(original[k])), false)
		add(k+"/duplicate_first", append([]byte(`{"`+k+`":`+string(original[k])+`,`), raw[1:]...), false)
		add(k+"/duplicate_last", append(append([]byte{}, bytes.TrimSpace(raw)[:len(bytes.TrimSpace(raw))-1]...), []byte(`,"`+k+`":`+string(original[k])+`}`)...), false)
		if r.kind == "integer" {
			for _, v := range []string{"0", "-0.0", "1", "1.0", "1e0", "9007199254740991", "9007199254740991.0", "9.007199254740991e15"} {
				add(k+"/integer/"+v, replace(k, v), true)
			}
			for _, v := range []string{"-1", "1.2", "1.0000000000000000001", "1e-10000", "1e100", "9007199254740992", "9223372036854775808", "true", `"1"`} {
				add(k+"/invalid_integer/"+v, replace(k, v), false)
			}
		}
	}
	for i, b := range [][]byte{[]byte(`null`), []byte(`[]`), []byte(`{}`), []byte(`{"case_id":`), append(append([]byte{}, raw...), []byte(` {}`)...), append(append([]byte{}, raw...), []byte(` false`)...), {0xff, '}'}} {
		add(fmt.Sprintf("malformed/%d", i), b, false)
	}
	for k, values := range map[string][]string{
		"currency":           {`"USD"`, `"us"`, `"u$d"`, `"usd\n"`},
		"digest":             {`"sha256:bad"`, `"sha256:` + strings.Repeat("A", 64) + `"`, `"sha256:` + strings.Repeat("a", 64) + `\n"`},
		"oldest_observed_at": {`"2026-02-30T10:00:00Z"`, `"2026-10-18"`, `"2026-10-18T10:00:00+24:00"`},
		"session_id":         {`""`, `"` + strings.Repeat("s", 256) + `"`},
	} {
		for i, v := range values {
			add(fmt.Sprintf("format/%s/%d", k, i), replace(k, v), false)
		}
	}
	add("provenance_precision", replace("oldest_observed_at", `"2026-10-18T12:00:00.000+02:00"`), true)
	for _, k := range []string{"case", "session", "account", "connection", "mode"} {
		c := measuredCase{"expected_scope/" + k, raw, e, p, false}
		switch k {
		case "case":
			c.expected.CaseID = "other"
		case "session":
			c.expected.SessionID = "other"
		case "account":
			c.expected.AccountID = "other"
		case "connection":
			c.expected.ConnectionID = "other"
		case "mode":
			c.expected.LiveMode = true
		}
		cases = append(cases, c)
	}
	bad := p
	bad.LiveMode = nil
	cases = append(cases, measuredCase{"policy/nil_mode", raw, e, bad, false})
	sort.Slice(cases, func(i, j int) bool { return cases[i].name < cases[j].name })
	return cases
}

var measuredProjection Projection
var measuredError error

func BenchmarkDecode(b *testing.B) {
	// Representative full Decode paths; the differential corpus additionally
	// covers every generated field. No reflection or fixture mutation is timed.
	selected := map[string]bool{"valid": true, "nullable": true, "known_zero": true, "known_false": true, "integer_decimal": true, "integer_exponent": true, "boundary_max": true, "boundary_over": true, "fraction": true, "unknown": true, "alias_order_0": true, "alias_order_1": true, "duplicate": true}
	for _, c := range measurementCases(b) {
		if !selected[c.name] {
			continue
		}
		b.Run(c.name, func(b *testing.B) {
			_, err := Decode(c.raw, c.expected, c.policy)
			if (err == nil) != c.accept {
				b.Fatal("unexpected benchmark acceptance", c.name, err)
			}
			b.ReportAllocs()
			b.SetBytes(int64(len(c.raw)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				measuredProjection, measuredError = Decode(c.raw, c.expected, c.policy)
			}
		})
	}
}

func TestDecodeDifferentialCorpus(t *testing.T) {
	type record struct {
		Name, InputSHA256, Error string
		Projection               Projection
		Financial                []eligibility.Result
	}
	var records []record
	for _, c := range measurementCases(t) {
		t.Run(c.name, func(t *testing.T) {
			got, err := Decode(c.raw, c.expected, c.policy)
			if (err == nil) != c.accept {
				t.Fatalf("unexpected acceptance: %v", err)
			}
			r := record{Name: c.name, Projection: got}
			sum := sha256.Sum256(c.raw)
			r.InputSHA256 = hex.EncodeToString(sum[:])
			if err != nil {
				r.Error = err.Error()
				if r.Error != "INVALID_SOURCE_FACTS" || !reflect.DeepEqual(got, Projection{}) {
					t.Fatalf("partial/error result: %+v %v", got, err)
				}
			} else {
				for _, at := range []time.Time{time.Date(2026, 10, 18, 10, 0, 5, 0, time.UTC), time.Date(2026, 10, 18, 10, 0, 10, 0, time.UTC)} {
					r.Financial = append(r.Financial, eligibility.Evaluate(c.policy, got.Snapshot.Facts, at))
				}
			}
			records = append(records, r)
		})
	}
	if out := os.Getenv("HIT80_CORPUS_OUTPUT"); out != "" {
		b, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(out, append(b, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%d deterministic synthetic records; full values, provenance, financial outcomes, and zero-on-error", len(records))
}

// The optimized rules share compiled regexes. Exercise simultaneous reads under
// -race and independently mutate returned facts to catch shared output storage.
func TestDecodeConcurrentIsolatedProjections(t *testing.T) {
	cases := measurementCases(t)
	var valid measuredCase
	for _, c := range cases {
		if c.name == "valid" {
			valid = c
			break
		}
	}
	for worker := 0; worker < 32; worker++ {
		t.Run(fmt.Sprint(worker), func(t *testing.T) {
			t.Parallel()
			for i := 0; i < 20; i++ {
				p, err := Decode(valid.raw, valid.expected, valid.policy)
				if err != nil || p.Snapshot.Facts.AmountRefunded == nil || *p.Snapshot.Facts.AmountRefunded != 0 {
					t.Fatalf("shared or invalid projection: %v", err)
				}
				*p.Snapshot.Facts.AmountRefunded = int64(i + 1)
			}
		})
	}
}
