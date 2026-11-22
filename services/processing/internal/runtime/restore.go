package runtime

import (
	"context"
	"log/slog"
)

// RestoreBarrier must invoke the real gate transition synchronously under the
// trusted publisher's live revision check.
type RestoreBarrier func(context.Context, func(context.Context) error) error

func NewRestored(ctx context.Context, cfg Config, revision, stand, workspace string,
	barrier RestoreBarrier, log *slog.Logger) (*Runtime, error) {
	if !cfg.RestoreCaseDeletion || barrier == nil {
		return nil, ErrConfig
	}
	r, err := New(ctx, cfg, revision, stand, log)
	if err != nil {
		return nil, err
	}
	if _, configured := r.a.stand[workspace]; !configured || len(r.a.stand) != 1 {
		r.close()
		return nil, ErrConfig
	}
	r.restoreBarrier = barrier
	return r, nil
}
