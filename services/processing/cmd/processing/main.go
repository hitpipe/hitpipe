// Command processing runs the private Processing owner service.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/hitpipe/hitpipe/services/processing/internal/runtime"
)

var revision string // explicit release pin, set by the service-local build command

func main() {
	config := flag.String("config", "", "absolute runtime configuration file")
	stand := flag.String("stand-config", "", "explicit loopback-only qualification inputs")
	check := flag.Bool("check-config", false, "validate configuration syntax")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	c, err := runtime.LoadConfig(*config)
	if err != nil || flag.NArg() != 0 {
		log.Error("processing configuration rejected")
		os.Exit(2)
	}
	if *check {
		return
	}
	if revision == "" {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, x := range info.Settings {
				if x.Key == "vcs.revision" {
					revision = x.Value
				}
			}
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	r, err := runtime.New(ctx, c, revision, *stand, log)
	if err != nil {
		log.Error("processing startup refused")
		os.Exit(2)
	}
	if err = r.Serve(ctx); err != nil {
		log.Error("processing listener unavailable")
		os.Exit(1)
	}
}
