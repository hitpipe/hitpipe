package input

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func validJSON(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../../../testdata/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCanonicalFieldNames(t *testing.T) {
	original := validJSON(t)
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(original, &envelope); err != nil {
		t.Fatal(err)
	}
	// Exercise every declared canonical name at the envelope and both nested objects.
	// A single case alias is invalid even without a colliding canonical key.
	for _, scope := range []string{"", "policy", "facts"} {
		fields := envelope
		if scope != "" {
			fields = map[string]json.RawMessage{}
			if err := json.Unmarshal(envelope[scope], &fields); err != nil {
				t.Fatal(err)
			}
		}
		for name, value := range fields {
			for _, alias := range []string{strings.ToUpper(name), strings.ToUpper(name[:1]) + name[1:]} {
				t.Run(scope+"/"+alias, func(t *testing.T) {
					changed := map[string]json.RawMessage{}
					for k, v := range fields {
						if k != name {
							changed[k] = v
						}
					}
					changed[alias] = value
					b, err := json.Marshal(changed)
					if err != nil {
						t.Fatal(err)
					}
					if scope != "" {
						full := map[string]json.RawMessage{}
						for k, v := range envelope {
							full[k] = v
						}
						full[scope] = b
						b, err = json.Marshal(full)
						if err != nil {
							t.Fatal(err)
						}
					}
					var s Scenario
					if err := StrictDecode(b, &s); err == nil {
						t.Fatalf("accepted noncanonical name %q at %q", alias, scope)
					}
				})
			}
		}
	}
}

func TestConflictingAliasesBothOrders(t *testing.T) {
	original := string(validJSON(t))
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal([]byte(original), &envelope); err != nil {
		t.Fatal(err)
	}
	refunded := strings.Replace(string(envelope["facts"]), `"amount_refunded": 0`, `"amount_refunded": 1`, 1)
	if refunded == string(envelope["facts"]) {
		t.Fatal("refund precondition changed")
	}
	cases := map[string]string{
		"nested_canonical_first": strings.Replace(original, `"amount_refunded": 0`, `"amount_refunded": 1, "AMOUNT_REFUNDED": 0`, 1),
		"nested_alias_first":     strings.Replace(original, `"amount_refunded": 0`, `"AMOUNT_REFUNDED": 0, "amount_refunded": 1`, 1),
		"policy_canonical_first": strings.Replace(original, `"config_id": "config_synthetic"`, `"config_id": "config_synthetic", "CONFIG_ID": "other"`, 1),
		"policy_alias_first":     strings.Replace(original, `"config_id": "config_synthetic"`, `"CONFIG_ID": "other", "config_id": "config_synthetic"`, 1),
		"top_canonical_first":    fmt.Sprintf(`{"policy":%s,"facts":%s,"FACTS":%s,"now":%s}`, envelope["policy"], refunded, envelope["facts"], envelope["now"]),
		"top_alias_first":        fmt.Sprintf(`{"policy":%s,"FACTS":%s,"facts":%s,"now":%s}`, envelope["policy"], envelope["facts"], refunded, envelope["now"]),
		"unicode_fold_alias":     strings.Replace(original, `"facts"`, `"factſ"`, 1),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			var s Scenario
			if err := StrictDecode([]byte(b), &s); err == nil {
				t.Fatal("ambiguous fixture accepted")
			}
		})
	}
}

func TestCanonicalZeroAndStrictRejections(t *testing.T) {
	original := validJSON(t)
	for _, b := range [][]byte{original, []byte(strings.Replace(string(original), `"amount_refunded"`, `"\u0061mount_refunded"`, 1))} {
		var s Scenario
		if err := StrictDecode(b, &s); err != nil {
			t.Fatalf("canonical input: %v", err)
		}
		if s.Facts.AmountRefunded == nil || *s.Facts.AmountRefunded != 0 || s.Policy.LiveMode == nil || *s.Policy.LiveMode {
			t.Fatal("known zero/false was lost")
		}
	}
	cases := map[string]string{
		"exact_duplicate":   strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 1, "amount_refunded": 0`, 1),
		"escaped_duplicate": strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 1, "\u0061mount_refunded": 0`, 1),
		"unknown_nested":    strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 0, "unknown": 0`, 1),
		"unknown_top":       strings.Replace(string(original), `"policy":`, `"unknown": 0, "policy":`, 1),
		"trailing_document": string(original) + " {}",
		"fraction":          strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 0.5`, 1),
		"exponent":          strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 0e0`, 1),
		"integer_overflow":  strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": 9223372036854775808`, 1),
		"number_string":     strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": "0"`, 1),
		"nonfinite":         strings.Replace(string(original), `"amount_refunded": 0`, `"amount_refunded": NaN`, 1),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			var s Scenario
			if err := StrictDecode([]byte(b), &s); err == nil {
				t.Fatal("strict rejection regressed")
			}
		})
	}
}
