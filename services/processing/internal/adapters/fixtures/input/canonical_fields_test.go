package input

import "testing"

func TestRejectsUndeclaredFieldAliases(t *testing.T) {
	for _, body := range []string{
		`{"Policy":{}}`,
		`{"policy":{"SCHEMA_VERSION":"1.0"}}`,
		`{"facts":{"AmountRefunded":0}}`,
		`{"facts":{"amount_refunded":1,"AMOUNT_REFUNDED":0}}`,
		`{"facts":{"AMOUNT_REFUNDED":0,"amount_refunded":1}}`,
	} {
		var scenario Scenario
		if StrictDecode([]byte(body), &scenario) == nil {
			t.Errorf("accepted an undeclared or shadowing field alias: %s", body)
		}
	}
}

func TestCanonicalFieldNamesPreserveKnownZero(t *testing.T) {
	var scenario Scenario
	if err := StrictDecode([]byte(`{"policy":{"schema_version":"1.0"},"facts":{"amount_refunded":0}}`), &scenario); err != nil {
		t.Fatal(err)
	}
	if scenario.Policy.SchemaVersion != "1.0" || scenario.Facts.AmountRefunded == nil || *scenario.Facts.AmountRefunded != 0 {
		t.Fatal("canonical fields or known zero were lost")
	}
}
