//go:build !delivery_fixture

package main

import (
	"context"

	"github.com/dkar-dev/hitpipe/services/delivery/internal/deliveryrun"
	"github.com/dkar-dev/hitpipe/services/delivery/internal/dispatch"
)

func launch(ctx context.Context, cfg deliveryrun.Config) error {
	// The owner-supplied IAM/runtime and connector adapters are subsequent
	// work. The standard build cannot activate the verification adapters.
	return deliveryrun.Run(ctx, cfg, deliveryrun.Adapters{}, dispatch.NewClickUp(nil))
}
