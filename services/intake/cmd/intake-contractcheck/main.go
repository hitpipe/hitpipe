package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/hitpipe/hitpipe/services/intake/internal/application/authenticatewebhook"
	"github.com/hitpipe/hitpipe/services/intake/internal/domain/stripesignature"
)

var (
	version   = "dev"
	commit    = "unknown"
	dirty     = "unknown"
	buildMode = "dev"
)

type fixture struct {
	RawBodyBase64 string `json:"raw_body_base64"`
	Signature     string `json:"signature_header"`
	Secret        string `json:"synthetic_secret"`
	NowUnix       int64  `json:"now_unix"`
}

type diagnostic struct {
	Module    string `json:"module"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Dirty     string `json:"dirty"`
	BuildMode string `json:"buildmode"`
}

func main() {
	fixturePath := flag.String("fixture", "", "synthetic fixture JSON path")
	format := flag.String("format", "text", "diagnostic format: text or json")
	start := flag.Bool("start", false, "print safe startup diagnostic without input")
	showVersion := flag.Bool("version", false, "print build identity")
	flag.Parse()

	if *format != "text" && *format != "json" {
		fail(*format, "format_invalid")
	}
	if *showVersion {
		fmt.Printf("module=intake-contractcheck version=%s commit=%s dirty=%s buildmode=%s\n", version, commit, dirty, buildMode)
		return
	}
	if *start {
		printDiagnostic(*format, diagnostic{Module: "intake-contractcheck", Status: "READY", Reason: "startup", Version: version, Commit: commit, Dirty: dirty, BuildMode: buildMode})
		return
	}
	if *fixturePath == "" {
		fail(*format, "fixture_required")
	}
	data, err := os.ReadFile(*fixturePath)
	if err != nil {
		fail(*format, "fixture_unavailable")
	}
	var input fixture
	if json.Unmarshal(data, &input) != nil {
		fail(*format, "fixture_invalid")
	}
	rawBody, err := base64.StdEncoding.DecodeString(input.RawBodyBase64)
	if err != nil {
		fail(*format, "fixture_body_invalid")
	}
	consumer, err := authenticatewebhook.New(fixtureSecretResolver{secret: []byte(input.Secret)}, fixtureClock{now: input.NowUnix})
	if err != nil {
		fail(*format, "consumer_unavailable")
	}
	result := consumer.Authenticate(context.Background(), authenticatewebhook.Input{
		InstallationID: "fixture-installation",
		RawBody:        rawBody,
		Signature:      input.Signature,
	})
	outcome := result.Outcome
	printDiagnostic(*format, diagnostic{Module: "intake-contractcheck", Status: string(outcome.Status), Reason: string(outcome.Reason), Version: outcome.Version, Commit: commit, Dirty: dirty, BuildMode: buildMode})
	if outcome.Status != stripesignature.Verified {
		os.Exit(1)
	}
}

type fixtureSecretResolver struct{ secret []byte }

func (r fixtureSecretResolver) Resolve(context.Context, string) ([]byte, error) {
	return r.secret, nil
}

type fixtureClock struct{ now int64 }

func (c fixtureClock) NowUnix(context.Context) (int64, error) { return c.now, nil }

func printDiagnostic(format string, value diagnostic) {
	if format == "json" {
		encoded, _ := json.Marshal(value)
		fmt.Println(string(encoded))
		return
	}
	fmt.Printf("module=%s status=%s reason=%s version=%s commit=%s dirty=%s buildmode=%s\n", value.Module, value.Status, value.Reason, value.Version, value.Commit, value.Dirty, value.BuildMode)
}

func fail(format string, reason string) {
	printDiagnostic(format, diagnostic{Module: "intake-contractcheck", Status: "REJECTED", Reason: reason, Version: stripesignature.Version, Commit: commit, Dirty: dirty, BuildMode: buildMode})
	os.Exit(2)
}
