package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/hitpipe/hitpipe/services/delivery/internal/application/verifytarget"
	"github.com/hitpipe/hitpipe/services/delivery/internal/domain/readback"
)

var (
	version   = "dev"
	commit    = "unknown"
	dirty     = "unknown"
	buildMode = "dev"
)

func main() {
	versionFlag := flag.Bool("version", false, "print build identity")
	fixturePath := flag.String("fixture", "", "synthetic fixture JSON")
	format := flag.String("format", "json", "diagnostic output format")
	config := flag.String("config", "default", "diagnostic config name")
	flag.Parse()
	if *config != "default" {
		fmt.Fprintln(os.Stderr, "unsupported config:", *config)
		os.Exit(2)
	}
	if *format != "json" {
		fmt.Fprintln(os.Stderr, "unsupported format:", *format)
		os.Exit(2)
	}
	if *versionFlag {
		fmt.Printf("delivery-contractcheck version=%s commit=%s dirty=%s buildmode=%s go=%s\n", version, commit, dirty, buildMode, runtime.Version())
		return
	}
	if *fixturePath == "" {
		fmt.Fprintln(os.Stderr, "usage: delivery-contractcheck --fixture PATH")
		os.Exit(2)
	}
	data, err := os.ReadFile(*fixturePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fixture, err := readback.ParseFixture(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fixture.Observation.TargetID = "fixture-target"
	result := verifytarget.New(fixtureReader{observation: fixture.Observation}).VerifyTarget(context.Background(), "fixture-target", fixture.Expected).Comparison
	encoded, _ := json.Marshal(result)
	fmt.Println(string(encoded))
	if result.Outcome != readback.Match {
		os.Exit(1)
	}
	if info, ok := debug.ReadBuildInfo(); ok && info == nil {
		os.Exit(2)
	}
}

type fixtureReader struct{ observation readback.Observation }

func (r fixtureReader) Read(context.Context, string) (readback.Observation, verifytarget.Provenance, error) {
	return r.observation, verifytarget.Provenance{ObservedAt: time.Unix(1, 0).UTC(), Source: "synthetic-fixture"}, nil
}
