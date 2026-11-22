package privateiam

import (
	"context"

	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

// TechnicalOpen observes the shared gate and the same boot's durable registration.
// It authorizes no request and establishes no user or connector readiness.
func (s *Server) TechnicalOpen(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	state, err := s.State(ctx)
	if err != nil || state != startgate.Open {
		return false
	}
	_, err = s.activeSnapshot(ctx)
	return err == nil
}
