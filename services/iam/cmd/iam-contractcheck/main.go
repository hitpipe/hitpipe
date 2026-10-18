// iam-contractcheck exercises the real policy with synthetic data. It is not an auth server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	systemclock "github.com/hitpipe/hitpipe/services/iam/internal/adapters/clock/system"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/fixtures/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
)

var version = "pr12"
var commit = "unknown"
var dirty = "unknown"
var buildMode = "source"

func run(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "iam-contractcheck event=start")
	fail := func() int { fmt.Fprintln(stderr, "iam-contractcheck event=end status=invalid_config"); return 2 }
	flags := flag.NewFlagSet("iam-contractcheck", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("fixture", "", "synthetic fixture path")
	format := flags.String("format", "text", "text or json")
	showVersion := flags.Bool("version", false, "print build metadata")
	lookupMode := flags.String("lookup", "file", "file or fixed synthetic lookup")
	clockMode := flags.String("clock", "fixture", "fixture or system (requires real trust probe)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, "iam-contractcheck --fixture PATH --format text|json [--lookup file|fixed] [--clock fixture|system] | --version")
			fmt.Fprintln(stderr, "iam-contractcheck event=end status=help")
			return 0
		}
		return fail()
	}
	if flags.NArg() != 0 || (*format != "text" && *format != "json") ||
		(*lookupMode != "file" && *lookupMode != "fixed") || (*clockMode != "fixture" && *clockMode != "system") {
		return fail()
	}
	if *showVersion {
		_, err := fmt.Fprintf(stdout, "version=%s commit=%s dirty=%s buildMode=%s\n", version, commit, dirty, buildMode)
		if err != nil {
			return fail()
		}
		fmt.Fprintln(stderr, "iam-contractcheck event=end status=version")
		return 0
	}
	if *path == "" {
		return fail()
	}
	file, err := os.Open(*path)
	if err != nil {
		return fail()
	}
	defer file.Close()
	snapshot, observation, err := sessionlookup.Decode(file)
	if err != nil {
		return fail()
	}
	var lookup authenticatesession.SessionLookup
	if *lookupMode == "fixed" {
		lookup = sessionlookup.NewFixed(snapshot, false)
	} else {
		lookup, err = sessionlookup.NewFile(*path)
		if err != nil {
			return fail()
		}
	}
	var clock authenticatesession.Clock = sessionlookup.NewFixedClock(observation)
	if *clockMode == "system" {
		clock = systemclock.New(nil)
	}
	service, err := authenticatesession.New(lookup, clock)
	if err != nil {
		return fail()
	}
	decision := service.Execute(context.Background(), "diagnostic")
	if *format == "json" {
		if json.NewEncoder(stdout).Encode(decision) != nil {
			return fail()
		}
	} else if _, err := fmt.Fprintf(stdout, "state=%s reason=%s\n", decision.State, decision.Reason); err != nil {
		return fail()
	}
	fmt.Fprintf(stderr, "iam-contractcheck event=end status=%s\n", decision.State)
	if decision.State == "unavailable" {
		return 3
	}
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
