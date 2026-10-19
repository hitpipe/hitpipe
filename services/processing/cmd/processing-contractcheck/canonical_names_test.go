package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCLIRejectsRefundShadowing(t *testing.T) {
	for _, fixture := range []string{"refund-shadowed.json", "refund-shadowed-reverse.json"} {
		t.Run(fixture, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			exit := run([]string{"--fixture", "../../testdata/" + fixture, "--format", "json"}, &out, &diagnostics)
			if exit != 2 || out.Len() != 0 || !strings.Contains(diagnostics.String(), "INVALID_JSON") {
				t.Fatalf("ambiguous input: exit=%d stdout=%q diagnostics=%q", exit, out.String(), diagnostics.String())
			}
		})
	}
}
