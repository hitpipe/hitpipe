package maintenance

import (
	"context"
	"log/slog"

	"github.com/hitpipe/hitpipe/services/processing/internal/runtime"
)

// CurrentDeletionInventory is the trusted local issuer coordinator. It calls
// verifyAndOpen synchronously inside its live WithCurrent barrier, supplying
// every current-generation APPLIED receipt. CANCELLED entries are omitted.
type CurrentDeletionInventory func(context.Context, func(context.Context, []DeletionReceipt) error) error

// ServeRestored starts the real Processing runtime closed. Both the issuer
// revision and the matching durable proof in this volume must be held while the
// ordinary IAM snapshot is applied to the actual runtime start gate.
func (a CaseAuthority) ServeRestored(ctx context.Context, config, stand, revision string,
	current CurrentDeletionInventory, logger *slog.Logger) error {
	cfg, err := runtime.LoadConfig(config)
	if err != nil {
		return err
	}
	if current == nil || cfg.Content != a.volume || cfg.Generation != a.generation {
		return ErrScope
	}
	guard := func(ctx context.Context, open func(context.Context) error) error {
		return current(ctx, func(c context.Context, receipts []DeletionReceipt) error {
			return a.withProofs(c, receipts, open)
		})
	}
	process, err := runtime.NewRestored(ctx, cfg, revision, stand, a.workspace, guard, logger)
	if err != nil {
		return err
	}
	return process.Serve(ctx)
}
