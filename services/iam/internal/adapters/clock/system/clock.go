// Package system wraps the host clock without inventing an attestation of it.
package system

import (
	"context"
	"errors"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
)

// HealthProbe must establish trust for the supplied host observation in the
// deployment environment. No such real probe is shipped by this offline slice.
type HealthProbe func(context.Context, time.Time) bool

type Clock struct{ probe HealthProbe }

// New can be given nil, which deliberately yields unavailable, never Trusted.
func New(probe HealthProbe) *Clock { return &Clock{probe: probe} }

func (c *Clock) Now(ctx context.Context) (sessionpolicy.Observation, error) {
	if c == nil || c.probe == nil || ctx == nil || ctx.Err() != nil {
		return sessionpolicy.Observation{}, errors.New("clock_unavailable")
	}
	now := time.Now().UTC()
	if !c.probe(ctx, now) || ctx.Err() != nil {
		return sessionpolicy.Observation{}, errors.New("clock_unavailable")
	}
	return sessionpolicy.Observation{At: now, Trusted: true}, nil
}

var _ authenticatesession.Clock = (*Clock)(nil)
