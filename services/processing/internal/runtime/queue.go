package runtime

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"syscall"
	"time"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
)

// standQueue is enabled only by an explicit loopback stand configuration.
// The controlled owner publishes a complete immutable generation under the
// same lock. An absent owner file is unavailable, including when it would be
// safe to report a known-empty generation after a genuine owner check.
type standQueue struct{ workspaces map[string]installed }

func (q standQueue) with(ctx context.Context, g drafts.Grant, scope lifecycle.QueueScope, f func(lifecycle.QueueEvidence) error) error {
	w, ok := q.workspaces[scope.WorkspaceID]
	if !ok || g.WorkspaceID != scope.WorkspaceID || g.PrincipalID != w.config.Owner || g.SessionID != w.config.SessionID ||
		!time.Now().Before(g.ExpiresAt) || ctx.Err() != nil {
		return drafts.ErrDenied
	}
	info, err := os.Lstat(w.config.QueueLock)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return drafts.ErrUnavailable
	}
	lock, err := os.OpenFile(w.config.QueueLock, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return drafts.ErrUnavailable
	}
	defer lock.Close()
	stat, err := lock.Stat()
	if err != nil || !os.SameFile(info, stat) || syscall.Flock(int(lock.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) != nil {
		return drafts.ErrUnavailable
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	data, err := readSecret(w.config.QueueSnapshot, 64<<10)
	var proof lifecycle.QueueEvidence
	if err != nil || json.Unmarshal(data, &proof) != nil || !proof.Valid(scope, time.Now()) || ctx.Err() != nil {
		return drafts.ErrUnavailable
	}
	return f(proof)
}

func (q standQueue) Capture(ctx context.Context, g drafts.Grant, scope lifecycle.QueueScope) (lifecycle.QueueEvidence, error) {
	var proof lifecycle.QueueEvidence
	err := q.with(ctx, g, scope, func(e lifecycle.QueueEvidence) error { proof = e; return nil })
	return proof, err
}

func (q standQueue) WithCurrent(ctx context.Context, g drafts.Grant, e lifecycle.QueueEvidence, f func(context.Context) error) error {
	return q.with(ctx, g, e.Scope, func(current lifecycle.QueueEvidence) error {
		if !reflect.DeepEqual(e, current) {
			return drafts.ErrConflict
		}
		return f(ctx)
	})
}
