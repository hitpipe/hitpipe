// Package system provides the production-shaped trusted clock adapter.
package system

import (
	"context"
	"time"
)

// Clock reads Unix seconds from the system clock. A future trusted time service
// can implement the same application-owned port without changing the consumer.
type Clock struct{}

// NowUnix returns the current Unix second unless the context is cancelled.
func (Clock) NowUnix(ctx context.Context) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return time.Now().Unix(), nil
}
