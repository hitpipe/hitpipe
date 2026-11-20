package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/hitpipe/hitpipe/services/intake/internal/intakeruntime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("intake", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	path := flags.String("config", "", "path to Intake configuration")
	if flags.Parse(os.Args[1:]) != nil || flags.NArg() != 0 {
		return intakeruntime.ErrConfig
	}
	config, err := intakeruntime.Load(*path)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return intakeruntime.Run(ctx, config, os.Stdout)
}
