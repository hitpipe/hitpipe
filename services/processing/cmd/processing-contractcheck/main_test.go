package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestCLIConfiguration(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		exit     int
		contains string
	}{
		{[]string{"--version"}, 0, "commit="},
		{[]string{}, 2, "INVALID_CONFIGURATION"},
		{[]string{"--fixture", "../../testdata/valid.json", "--format", "invalid"}, 2, "INVALID_CONFIGURATION"},
		{[]string{"--fixture", "../../testdata/valid.json", "--format", "json"}, 0, "FINANCIAL_ELIGIBILITY_ONLY"},
		{[]string{"--fixture", "../../testdata/refunded.json"}, 0, "REFUND"},
		{[]string{"--fixture", "../../testdata/invalid.json"}, 2, "INVALID_JSON"},
		{[]string{"--bad"}, 2, "INVALID_ARGUMENTS"},
	} {
		var out, diag bytes.Buffer
		exit := run(tc.args, &out, &diag)
		if exit != tc.exit || !strings.Contains(out.String()+diag.String(), tc.contains) {
			t.Fatalf("%v exit=%d out=%s diag=%s", tc.args, exit, out.String(), diag.String())
		}
		if strings.Contains(diag.String(), "acct_synthetic") || strings.Contains(diag.String(), "12000") {
			t.Fatal("raw input leaked")
		}
	}
}

type failedWriter struct{}

func (failedWriter) Write(p []byte) (int, error) { return 0, fmt.Errorf("output unavailable") }
func TestOutputFailure(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"--fixture", "../../testdata/valid.json", "--format", "text"}, {"--fixture", "../../testdata/valid.json", "--format", "json"}} {
		var diag bytes.Buffer
		if exit := run(args, failedWriter{}, &diag); exit != 3 {
			t.Errorf("args=%v got exit=%d want 3", args, exit)
		}
	}
}
