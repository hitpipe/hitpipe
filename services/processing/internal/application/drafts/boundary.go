package drafts

import (
	"context"
	"sync/atomic"
	"time"
)

// Protect admits a single bounded action through the current runtime gate.
func Protect(ctx context.Context, boundary Boundary, grant Grant, phase string, valid func() bool, work func(context.Context) error) error {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var called, active atomic.Bool
	active.Store(true)
	defer active.Store(false)
	var actionErr error
	err := boundary.Start(bounded, grant, phase, func(c context.Context) error {
		if !active.Load() || !called.CompareAndSwap(false, true) || c == nil || c.Err() != nil || bounded.Err() != nil || !valid() {
			return ErrDenied
		}
		w, stop := context.WithCancel(bounded)
		defer stop()
		end := context.AfterFunc(c, stop)
		defer end()
		actionErr = work(w)
		return actionErr
	})
	if actionErr != nil {
		return actionErr
	}
	if err != nil {
		return err
	}
	if !called.Load() || bounded.Err() != nil {
		return ErrDenied
	}
	return nil
}
