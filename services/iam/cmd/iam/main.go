// iam serves the admitted IAM routes on its private business and OPS listeners.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/iamprocess"
)

// buildRevision is set by a trusted build from an exact source archive when
// Go VCS metadata is unavailable.
var buildRevision string

func executableRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildRevision
	}
	var revision string
	dirty := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if dirty || revision != "" && buildRevision != "" && revision != buildRevision {
		return ""
	}
	if revision != "" {
		return revision
	}
	return buildRevision
}

func execute(args []string, inputPath string, diagnostic io.Writer) int {
	flags := flag.NewFlagSet("iam", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", inputPath, "path to provisioned IAM JSON configuration")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *path == "" {
		fmt.Fprintln(diagnostic, "IAM_CONFIG_PATH_REQUIRED")
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	cfg, err := iamprocess.LoadConfig(*path)
	if err != nil {
		fmt.Fprintln(diagnostic, err.Error())
		return 2
	}
	if executableRevision() != cfg.BuildRevision {
		fmt.Fprintln(diagnostic, "IAM_BUILD_REVISION_INVALID")
		return 2
	}
	pool, err := cfg.OpenPool(ctx)
	if err != nil {
		fmt.Fprintln(diagnostic, "IAM_DATABASE_POOL_UNAVAILABLE")
		return 3
	}
	defer pool.Close()
	if err := iamprocess.Run(ctx, cfg, pool); err != nil && !(ctx.Err() != nil && errors.Is(err, context.Canceled)) {
		fmt.Fprintln(diagnostic, "IAM_PROCESS_UNAVAILABLE")
		return 3
	}
	return 0
}

func main() { os.Exit(execute(os.Args[1:], os.Getenv("HITPIPE_IAM_CONFIG"), os.Stderr)) }
