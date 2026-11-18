package postgres

import (
	"context"
	"encoding/json"

	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
)

// CommittedResume is the bounded readback for trusted local owner composition.
// It exposes neither a route nor external writes. The authoritative queue
// owner must apply a decision at most once to that exact held generation and
// retain a known gap when discarding.
func (l LifecycleStore) CommittedResume(ctx context.Context, workspace, operation, request, generation string) (lifecycle.ResumeDecision, error) {
	var out lifecycle.ResumeDecision
	if !drafts.UUID(workspace) || !drafts.UUID(operation) || !drafts.UUID(request) || !drafts.UUID(generation) {
		return out, drafts.ErrInvalid
	}
	tx, err := l.Drafts.tx(ctx, workspace)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, workspace+":command:"+request); err != nil {
		return out, err
	}
	meta, err := loadDraftCommand(ctx, tx, workspace, request)
	if err != nil {
		return out, err
	}
	if !meta.exists || meta.aggregate != operation || meta.disposition != "APPLIED" || meta.result == nil || meta.expected == nil {
		return out, drafts.ErrNotFound
	}
	did, err := lifecycleDraftID(ctx, tx, workspace, operation)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, workspace+":draft:"+did); err != nil {
		return out, err
	}
	x, err := l.operation(ctx, tx, workspace, operation, false)
	if err != nil {
		return out, err
	}
	if err = l.materialize(ctx, tx, &x, did, false); err != nil {
		return out, err
	}
	d := l.Drafts
	ref := x.draft.Root.Lifecycle
	for n := 0; ref != nil && n < d.Policy.ReplayNodes; n++ {
		raw, err := d.readFile(ctx, *ref, workspace, did, x.draft.Config.Revision, x.now)
		if err != nil {
			return out, err
		}
		var node lifecycleNode
		if frame(raw, &node) != nil || node.Format != "lifecycle-command-v1" || node.WorkspaceID != workspace || node.OperationID != operation {
			return out, drafts.ErrUnavailable
		}
		ref = node.Previous
		if node.RequestID != request {
			continue
		}
		p := node.Accepted
		if node.Action != lifecycle.Accept || p == nil || p.Evidence.Generation != generation {
			return out, drafts.ErrNotFound
		}
		if node.ActorID != meta.actor || node.Digest != meta.digest || node.ExpectedRevision != *meta.expected ||
			node.ResultRevision != *meta.result || !equivalentSnapshot(p) || p.ActorID != node.ActorID || p.Epoch != node.Epoch ||
			p.View.OperationID != operation || p.View.Revision != node.ExpectedRevision || p.Evidence.Scope.WorkspaceID != workspace ||
			p.Evidence.Scope.OperationID != operation || p.Evidence.Scope.ConfigDigest != x.op.Scope.ConfigDigest ||
			!p.Evidence.Scope.FirstActivationAt.Equal(x.op.Scope.FirstActivationAt) ||
			(node.Disposition != lifecycle.Process && node.Disposition != lifecycle.Discard) {
			return out, drafts.ErrUnavailable
		}
		var op drafts.Operation
		if draftjson.Validate("Operation", node.Response) != nil || json.Unmarshal(node.Response, &op) != nil ||
			op.State != "ACTIVE" || op.OperationID != operation || op.Revision != node.ResultRevision {
			return out, drafts.ErrUnavailable
		}
		out = lifecycle.ResumeDecision{Scope: p.Evidence.Scope, RequestID: request, SnapshotID: p.View.SnapshotID, Generation: generation,
			Disposition: node.Disposition, ResultRevision: node.ResultRevision, SignalCount: p.View.SignalCount, SerializedBytes: p.View.SerializedBytes,
			OldestReceivedAt: p.View.OldestReceivedAt, Gaps: p.View.Gaps, ObservedAt: p.Evidence.ObservedAt}
		if err = tx.Commit(ctx); err != nil {
			return lifecycle.ResumeDecision{}, err
		}
		return out, nil
	}
	return out, drafts.ErrNotFound
}
