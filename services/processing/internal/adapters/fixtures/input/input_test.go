package input

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictFixture(t *testing.T) {
	for _, s := range []string{
		`{"x":1,"x":2}`, `{"nested":{"x":1,"x":2}}`, `{} {}`, `{"x":1}`, `{"now":null,"now":null}`, `[]`, `{"facts":{"amount_total":1.2}}`,
	} {
		var v Scenario
		if StrictDecode([]byte(s), &v) == nil {
			t.Fatalf("accepted invalid fixture %s", s)
		}
	}
	s, e := Read("../../../../testdata/valid.json")
	if e != nil || s.Facts.AmountTotal == nil || *s.Facts.AmountTotal != 12000 {
		t.Fatalf("fixture %+v %v", s, e)
	}
}
func TestReadFailure(t *testing.T) {
	if _, e := Read(filepath.Join(t.TempDir(), "missing")); e == nil {
		t.Fatal("missing accepted")
	}
	p := filepath.Join(t.TempDir(), "large.json")
	if e := os.WriteFile(p, make([]byte, MaxBytes+1), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Read(p); e == nil {
		t.Fatal("oversized accepted")
	}
}
