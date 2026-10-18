// iam-contractcheck exercises the real policy with synthetic data. It is not an auth server.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
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
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, "iam-contractcheck --fixture PATH --format text|json | --version")
			fmt.Fprintln(stderr, "iam-contractcheck event=end status=help")
			return 0
		}
		return fail()
	}
	if flags.NArg() != 0 || (*format != "text" && *format != "json") {
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
	snapshot, observation, err := decodeFixture(file)
	if err != nil {
		return fail()
	}
	decision := sessionpolicy.Evaluate(snapshot, observation)
	if *format == "json" {
		if json.NewEncoder(stdout).Encode(decision) != nil {
			return fail()
		}
	} else if _, err := fmt.Fprintf(stdout, "state=%s reason=%s\n", decision.State, decision.Reason); err != nil {
		return fail()
	}
	fmt.Fprintf(stderr, "iam-contractcheck event=end status=%s\n", decision.State)
	return 0
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
