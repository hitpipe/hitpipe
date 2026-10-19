// processing-contractcheck is an offline diagnostic of the reusable policy.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"io"
	"os"
)

var version = "dev"
var commit = "unknown"
var dirty = "true"
var buildMode = "unconfigured"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, diagnostics io.Writer) int {
	fs := flag.NewFlagSet("processing-contractcheck", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fixture := fs.String("fixture", "", "synthetic scenario JSON")
	format := fs.String("format", "text", "text or json")
	showVersion := fs.Bool("version", false, "version metadata")
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		fmt.Fprintln(diagnostics, "INVALID_ARGUMENTS")
		return 2
	}
	if *showVersion {
		fmt.Fprintf(out, "version=%s commit=%s dirty=%s buildmode=%s\n", version, commit, dirty, buildMode)
		return 0
	}
	if *fixture == "" || (*format != "json" && *format != "text") {
		fmt.Fprintln(diagnostics, "INVALID_CONFIGURATION")
		return 2
	}
	fmt.Fprintln(diagnostics, "start processing-contractcheck offline")
	s, err := input.Read(*fixture)
	if err != nil {
		fmt.Fprintln(diagnostics, err)
		return 2
	}
	if !eligibility.ValidatePolicy(s.Policy) || s.Now.IsZero() {
		fmt.Fprintln(diagnostics, "INVALID_POLICY")
		return 2
	}
	r := eligibility.Evaluate(s.Policy, s.Facts, s.Now)
	if *format == "json" {
		if json.NewEncoder(out).Encode(r) != nil {
			return 3
		}
	} else {
		fmt.Fprintf(out, "%s %s\n", r.Status, r.Reason)
	}
	fmt.Fprintf(diagnostics, "result status=%s reason=%s\n", r.Status, r.Reason)
	return 0
}
