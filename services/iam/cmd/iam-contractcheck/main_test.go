package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `{"version":1,"session":{"found":true,"revoked":false,"created_at":"2026-10-18T00:00:00Z","expires_at":"2026-10-19T00:00:00Z","idle_expires_at":"2026-10-18T08:00:00Z"},"clock":{"trusted":true,"now":"2026-10-18T07:59:59Z"}}`

func fixtureFile(t *testing.T, data string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiagnosticValid(t *testing.T) {
	path := fixtureFile(t, fixture)
	for _, format := range []string{"text", "json"} {
		var out, diagnostics bytes.Buffer
		if code := run([]string{"--fixture", path, "--format", format}, &out, &diagnostics); code != 0 {
			t.Fatalf("exit %d", code)
		}
		if !strings.Contains(out.String(), "session_valid") || !strings.Contains(diagnostics.String(), "event=start") || !strings.Contains(diagnostics.String(), "event=end") {
			t.Fatal("missing safe result/diagnostics")
		}
		if format == "json" {
			var data map[string]string
			if json.Unmarshal(out.Bytes(), &data) != nil || data["state"] != "valid" {
				t.Fatal("invalid result JSON")
			}
		}
	}
}

func TestDiagnosticMalformedAndCanary(t *testing.T) {
	const canary = "SYNTHETIC_SECRET_MUST_NOT_APPEAR_9E21"
	cases := map[string]string{
		"case mismatched key":    strings.Replace(fixture, `"version":1`, `"Version":1`, 1),
		"nested case":            strings.Replace(fixture, `"trusted":true`, `"Trusted":true`, 1),
		"unknown null timestamp": strings.Replace(strings.Replace(fixture, `"found":true`, `"found":false`, 1), `"created_at":"2026-10-18T00:00:00Z"`, `"created_at":null`, 1),
		"invalid UTF8":           strings.Replace(fixture, `"version":1`, "\"version\":1,\"bad\xff\":0", 1),
		"broken":                 "{", "trailing": fixture + " {}", "null root": "null",
		"extra":           strings.Replace(fixture, `"version":1`, `"version":1,"secret":"`+canary+`"`, 1),
		"duplicate":       strings.Replace(fixture, `"trusted":true`, `"trusted":false,"trusted":true`, 1),
		"null bool":       strings.Replace(fixture, `"found":true`, `"found":null`, 1),
		"bad version":     strings.Replace(fixture, `"version":1`, `"version":2`, 1),
		"missing now":     strings.Replace(fixture, `,"now":"2026-10-18T07:59:59Z"`, "", 1),
		"bad timestamp":   strings.Replace(fixture, "2026-10-18T07:59:59Z", canary, 1),
		"comma timestamp": strings.Replace(fixture, "07:59:59Z", "07:59:59,1Z", 1),
		"large offset":    strings.Replace(fixture, "07:59:59Z", "07:59:59+24:00", 1),
		"oversize":        strings.Repeat(" ", maxFixtureBytes+1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			var out, diagnostics bytes.Buffer
			if code := run([]string{"--fixture", fixtureFile(t, data), "--format", "json"}, &out, &diagnostics); code != 2 {
				t.Fatalf("expected config exit 2, got %d", code)
			}
			if out.Len() != 0 || strings.Contains(diagnostics.String(), canary) || strings.Contains(diagnostics.String(), data) {
				t.Fatal("input leaked")
			}
		})
	}
}

func TestDiagnosticConfigAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"--fixture", "/does-not-exist"}, {"--format", "xml"}, {"unexpected"}, {"--secret=SYNTHETIC_CANARY"}} {
		var out, diagnostics bytes.Buffer
		if code := run(args, &out, &diagnostics); code != 2 {
			t.Fatalf("want config error, got %d", code)
		}
		if strings.Contains(out.String()+diagnostics.String(), "SYNTHETIC_CANARY") {
			t.Fatal("flag leaked")
		}
	}
	for _, arg := range []string{"--version", "--help"} {
		var out, diagnostics bytes.Buffer
		if code := run([]string{arg}, &out, &diagnostics); code != 0 {
			t.Fatal("metadata/help failed")
		}
		if arg == "--version" && (!strings.Contains(out.String(), "commit=") || !strings.Contains(out.String(), "buildMode=")) {
			t.Fatal("build identity missing")
		}
	}
}

func TestDiagnosticDeniedIsSuccessfulEvaluation(t *testing.T) {
	data := strings.Replace(fixture, `"revoked":false`, `"revoked":true`, 1)
	var out, diagnostics bytes.Buffer
	if code := run([]string{"--fixture", fixtureFile(t, data)}, &out, &diagnostics); code != 0 || !strings.Contains(out.String(), "revoked_session") {
		t.Fatal("expected successful denied decision")
	}
}
