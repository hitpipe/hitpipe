package runtime

import (
	"context"
	"errors"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres"
)

func (rt *Runtime) advanceTimeout(ctx context.Context, workspace string) {
	if rt.a.stopping.Load() {
		return
	}
	ctx = withPermit(ctx)
	p, err := rt.a.local(ctx, workspace, "ACTIVATE", nil)
	if err != nil {
		return
	}
	err = rt.a.start(ctx, workspace, p.principal, p.epoch, func(c context.Context) error {
		const batch = 5
		if err := rt.timeouts.Index(c, workspace, p.principal, batch); err != nil {
			return err
		}
		for range batch {
			claim, err := rt.timeouts.Claim(c, workspace, p.principal)
			if err != nil || claim.CaseID == "" {
				return err
			}
			result, err := rt.timeouts.Apply(c, claim)
			if errors.Is(err, postgres.ErrTimeoutTimeline) {
				rt.logger.Info("processing case timer held", "case_id", claim.CaseID, "reason", postgres.ErrTimeoutTimeline.Error())
				continue
			}
			if err != nil {
				return err
			}
			if result.CaseID != "" {
				rt.logger.Info("processing case timer", "case_id", result.CaseID, "state", result.State, "reason", result.Reason)
			}
		}
		return nil
	})
	if err != nil {
		rt.logger.Warn("processing case timer interrupted")
	}
}
