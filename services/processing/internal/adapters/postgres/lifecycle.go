package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle"
	"github.com/jackc/pgx/v5"
)

type LifecycleStore struct{ Drafts DraftStore }

type lifecycleRow struct {
	op    lifecycle.State
	draft drafts.Snapshot
	owner string
	now   time.Time
}

type lifecycleNode struct {
	Format, WorkspaceID, OperationID, ActorID, RequestID, Action, Digest string
	ExpectedRevision, ResultRevision, Epoch                              int64
	Previous                                                             *drafts.Ref
	Response                                                             json.RawMessage
	Prepared                                                             *lifecycle.SavedSnapshot
	Accepted                                                             *lifecycle.SavedSnapshot
	Disposition                                                          string
	Reason                                                               *string
}

func (l LifecycleStore) operation(ctx context.Context, tx pgx.Tx, ws, id string, lock bool) (lifecycleRow, error) {
	var x lifecycleRow
	op := &x.op.Operation
	var paused *time.Time
	var config []byte
	q := `SELECT o.operation_id::text,o.owner_user_id::text,o.revision,o.state,o.first_activation_at,
  o.selected_config_id::text,a.digest,a.canonical_config,o.pause_accepted_at,o.drain_deadline_at,clock_timestamp()
 FROM hp_processing.operations o LEFT JOIN hp_processing.accepted_configs a
 ON (a.workspace_id,a.operation_id,a.config_id)=(o.workspace_id,o.operation_id,o.selected_config_id)
 WHERE o.workspace_id=$1 AND o.operation_id=$2`
	if lock {
		q += ` FOR UPDATE OF o`
	}
	err := tx.QueryRow(ctx, q, ws, id).Scan(&op.OperationID, &x.owner, &op.Revision, &op.State, &op.FirstActivationAt,
		&op.AcceptedConfigID, &op.AcceptedConfigDigest, &config, &paused, &op.DrainDeadlineAt, &x.now)
	if errors.Is(err, pgx.ErrNoRows) {
		return x, drafts.ErrNotFound
	}
	if err != nil {
		return x, err
	}
	if now := l.Drafts.Clock.Now(); now.After(x.now) {
		x.now = now
	}
	x.now = x.now.UTC().Truncate(time.Microsecond)
	op.ConnectionHealth = "UNCHECKED"
	x.op.Scope.WorkspaceID, x.op.Scope.OperationID = ws, id
	if op.AcceptedConfigID != nil {
		canonical, err := jcs.Transform(config)
		if err != nil || op.AcceptedConfigDigest == nil || op.FirstActivationAt == nil ||
			draftjson.Hash(canonical) != *op.AcceptedConfigDigest || draftjson.Validate("CanonicalAcceptedConfig", canonical) != nil {
			return x, drafts.ErrUnavailable
		}
		x.op.Scope.ConfigDigest, x.op.Scope.FirstActivationAt = *op.AcceptedConfigDigest, *op.FirstActivationAt
	}
	if paused != nil {
		x.op.Scope.PauseAcceptedAt = *paused
	}
	return x, nil
}

func lifecycleDraftID(ctx context.Context, tx pgx.Tx, ws, op string) (string, error) {
	rows, err := tx.Query(ctx, `SELECT draft_id::text FROM hp_processing.drafts WHERE workspace_id=$1 AND operation_id=$2 LIMIT 2`, ws, op)
	if err != nil {
		return "", err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowTo[string])
}

func (l LifecycleStore) materialize(ctx context.Context, tx pgx.Tx, x *lifecycleRow, draftID string, lock bool) error {
	d := l.Drafts
	ws, op := x.op.Scope.WorkspaceID, &x.op.Operation
	row, err := loadDraftRow(ctx, tx, ws, draftID, lock)
	if err != nil || !row.exists {
		return drafts.ErrUnavailable
	}
	x.draft, err = d.root(ctx, drafts.Request{WorkspaceID: ws, DraftID: draftID}, row)
	if err != nil {
		return err
	}
	if op.AcceptedConfigID == nil {
		op.Name = x.draft.Config.Name
		return nil
	}
	ref := x.draft.Root.Tail
	for n := 0; ref != nil && n < d.Policy.ReplayNodes; n++ {
		raw, err := d.readFile(ctx, *ref, ws, draftID, x.draft.Config.Revision, x.now)
		if err != nil {
			return err
		}
		var node drafts.CommandNode
		if frame(raw, &node) != nil || node.Format != "draft-command-v1" || node.WorkspaceID != ws ||
			node.DraftID != draftID || node.ResultRevision != ref.Revision {
			return drafts.ErrUnavailable
		}
		if node.Kind == activation.Kind && node.OperationID == op.OperationID {
			if node.BindingCheck == nil || node.ActorID != x.owner || node.AcceptedConfigDigest != *op.AcceptedConfigDigest {
				return drafts.ErrUnavailable
			}
			meta, err := loadDraftCommand(ctx, tx, ws, node.RequestID)
			if err != nil || !meta.exists || meta.actor != node.ActorID || meta.aggregate != op.OperationID ||
				meta.result == nil || *meta.result != node.OperationRevision || meta.digest != node.Digest || meta.disposition != "APPLIED" {
				return drafts.ErrUnavailable
			}
			body, err := d.readFile(ctx, node.Response, ws, draftID, x.draft.Config.Revision, x.now)
			var initial drafts.Operation
			if err != nil || draftjson.Validate("Operation", body) != nil || json.Unmarshal(body, &initial) != nil ||
				initial.OperationID != op.OperationID || initial.Revision != node.OperationRevision ||
				initial.AcceptedConfigID == nil || *initial.AcceptedConfigID != *op.AcceptedConfigID ||
				initial.AcceptedConfigDigest == nil || *initial.AcceptedConfigDigest != *op.AcceptedConfigDigest ||
				initial.FirstActivationAt == nil || !initial.FirstActivationAt.Equal(*op.FirstActivationAt) {
				return drafts.ErrUnavailable
			}
			x.op.Binding = *node.BindingCheck
			op.Name = initial.Name
			return nil
		}
		ref = node.Previous
	}
	return drafts.ErrUnavailable
}

func (l LifecycleStore) List(ctx context.Context, r lifecycle.Request, g drafts.Grant) (drafts.Operations, error) {
	out := drafts.Operations{Items: make([]drafts.Operation, 0)}
	tx, err := l.Drafts.tx(ctx, r.WorkspaceID)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT operation_id::text FROM hp_processing.operations WHERE workspace_id=$1 ORDER BY operation_id LIMIT 6`, r.WorkspaceID)
	if err != nil {
		return out, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(ids) > 5 {
		return out, drafts.ErrUnavailable
	}
	for _, id := range ids {
		x, err := l.operation(ctx, tx, r.WorkspaceID, id, false)
		if err != nil {
			return out, err
		}
		did, err := lifecycleDraftID(ctx, tx, r.WorkspaceID, id)
		if err != nil {
			return out, drafts.ErrUnavailable
		}
		if err = l.materialize(ctx, tx, &x, did, false); err != nil {
			return out, err
		}
		out.Items = append(out.Items, x.op.Operation)
	}
	return out, tx.Commit(ctx)
}

func equivalentSnapshot(s *lifecycle.SavedSnapshot) bool {
	if s == nil || s.View.SignalCount != s.Evidence.SignalCount || s.View.SerializedBytes != s.Evidence.SerializedBytes ||
		!reflect.DeepEqual(s.View.Gaps, s.Evidence.Gaps) || s.View.ExpiresAt.After(s.Evidence.ExpiresAt) ||
		s.View.CreatedAt.Before(s.Evidence.ObservedAt) {
		return false
	}
	a, b := s.View.OldestReceivedAt, s.Evidence.OldestReceivedAt
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
}

func (l LifecycleStore) readLedger(ctx context.Context, tx pgx.Tx, x *lifecycleRow, r lifecycle.Request, in lifecycle.Input, g drafts.Grant) error {
	d := l.Drafts
	var meta draftCommand
	var err error
	if r.Kind == lifecycle.Change {
		meta, err = loadDraftCommand(ctx, tx, r.WorkspaceID, r.RequestID)
		if err != nil {
			return err
		}
		if meta.exists && (meta.actor != g.PrincipalID || meta.aggregate != r.OperationID || meta.digest != in.Digest ||
			!sameInt(meta.expected, in.Command.ExpectedRevision) || meta.result == nil || meta.disposition != "APPLIED") {
			return drafts.ErrConflict
		}
	}
	ref := x.draft.Root.Lifecycle
	for n := 0; ref != nil && n < d.Policy.ReplayNodes; n++ {
		raw, err := d.readFile(ctx, *ref, r.WorkspaceID, x.draft.Config.DraftID, x.draft.Config.Revision, x.now)
		if err != nil {
			return err
		}
		var node lifecycleNode
		if frame(raw, &node) != nil || node.Format != "lifecycle-command-v1" || node.WorkspaceID != r.WorkspaceID ||
			node.OperationID != r.OperationID || node.ResultRevision <= node.ExpectedRevision || node.Epoch < 1 ||
			node.ResultRevision > x.op.Operation.Revision {
			return drafts.ErrUnavailable
		}
		if (n == 0 && node.Prepared != nil && node.ResultRevision == x.op.Operation.Revision) || meta.exists && node.RequestID == r.RequestID {
			m, err := loadDraftCommand(ctx, tx, r.WorkspaceID, node.RequestID)
			if err != nil || !m.exists || m.aggregate != r.OperationID || m.actor != node.ActorID ||
				m.digest != node.Digest || m.expected == nil || *m.expected != node.ExpectedRevision ||
				m.result == nil || *m.result != node.ResultRevision || m.disposition != "APPLIED" {
				return drafts.ErrUnavailable
			}
			var op drafts.Operation
			if draftjson.Validate("Operation", node.Response) != nil || json.Unmarshal(node.Response, &op) != nil ||
				op.OperationID != r.OperationID || op.Revision != node.ResultRevision {
				return drafts.ErrUnavailable
			}
			if n == 0 && node.Prepared != nil && node.ResultRevision == x.op.Operation.Revision {
				p := node.Prepared
				if node.Action != lifecycle.Prepare || !equivalentSnapshot(p) ||
					p.View.Revision != node.ResultRevision || p.View.OperationID != node.OperationID ||
					p.ActorID != node.ActorID || p.Epoch != node.Epoch {
					return drafts.ErrUnavailable
				}
				x.op.Snapshot = p
			}
			if meta.exists && node.RequestID == r.RequestID {
				if node.Action != in.Action {
					return drafts.ErrConflict
				}
				x.op.Replay = node.Response
				return nil
			}
		}
		if !meta.exists {
			return nil
		}
		ref = node.Previous
	}
	if meta.exists {
		return drafts.ErrUnavailable
	}
	return nil
}

func (l LifecycleStore) Load(ctx context.Context, r lifecycle.Request, in lifecycle.Input, g drafts.Grant) (lifecycle.State, error) {
	tx, err := l.Drafts.tx(ctx, r.WorkspaceID)
	if err != nil {
		return lifecycle.State{}, err
	}
	defer tx.Rollback(ctx)
	did, err := lifecycleDraftID(ctx, tx, r.WorkspaceID, r.OperationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return lifecycle.State{}, drafts.ErrNotFound
	}
	if err != nil {
		return lifecycle.State{}, drafts.ErrUnavailable
	}
	keys := []string{r.WorkspaceID + ":draft:" + did}
	if r.Kind == lifecycle.Change {
		keys = append([]string{r.WorkspaceID + ":command:" + r.RequestID}, keys...)
	}
	for _, key := range keys {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
			return lifecycle.State{}, err
		}
	}
	x, err := l.operation(ctx, tx, r.WorkspaceID, r.OperationID, false)
	if err != nil {
		return x.op, err
	}
	if x.owner != g.PrincipalID {
		return x.op, drafts.ErrNotFound
	}
	if err = l.materialize(ctx, tx, &x, did, false); err != nil {
		return x.op, err
	}
	if err = l.readLedger(ctx, tx, &x, r, in, g); err != nil {
		return x.op, err
	}
	return x.op, tx.Commit(ctx)
}

func (l LifecycleStore) Commit(ctx context.Context, r lifecycle.Request, in lifecycle.Input, g drafts.Grant, expected lifecycle.State, c lifecycle.Candidate) ([]byte, error) {
	d := l.Drafts
	tx, err := d.tx(ctx, r.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	did, err := lifecycleDraftID(ctx, tx, r.WorkspaceID, r.OperationID)
	if err != nil {
		return nil, drafts.ErrUnavailable
	}
	for _, key := range []string{r.WorkspaceID + ":command:" + r.RequestID, r.WorkspaceID + ":draft:" + did, r.WorkspaceID + ":activation"} {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
			return nil, err
		}
	}
	x, err := l.operation(ctx, tx, r.WorkspaceID, r.OperationID, true)
	if err != nil {
		return nil, err
	}
	if x.owner != g.PrincipalID {
		return nil, drafts.ErrNotFound
	}
	if err = l.materialize(ctx, tx, &x, did, true); err != nil {
		return nil, err
	}
	if err = l.readLedger(ctx, tx, &x, r, in, g); err != nil {
		return nil, err
	}
	if x.op.Replay != nil {
		return x.op.Replay, tx.Commit(ctx)
	}
	if err = lifecycle.Check(x.op, in, g, x.now); err != nil {
		return nil, err
	}
	if !x.now.Before(g.ExpiresAt) || x.op.Operation.Revision != expected.Operation.Revision ||
		x.op.Scope.ConfigDigest != expected.Scope.ConfigDigest || x.op.Operation.Revision >= 9007199254740991 {
		return nil, drafts.ErrStale
	}
	if in.Action != lifecycle.Pause && (c.Binding == nil || !lifecycle.BindingValid(x.op.Binding, *c.Binding, g, x.now)) {
		return nil, drafts.ErrConflict
	}
	op := x.op.Operation
	op.Revision++
	node := lifecycleNode{Format: "lifecycle-command-v1", WorkspaceID: r.WorkspaceID, OperationID: r.OperationID,
		ActorID: g.PrincipalID, RequestID: r.RequestID, Action: in.Action, Digest: in.Digest, ExpectedRevision: *in.Command.ExpectedRevision,
		ResultRevision: op.Revision, Epoch: g.Epoch, Previous: x.draft.Root.Lifecycle, Reason: in.Reason}
	pauseAt := &x.op.Scope.PauseAcceptedAt
	switch in.Action {
	case lifecycle.Pause:
		pauseAt = &x.now
		deadline := x.now.Add(lifecycle.DrainLimit)
		op.DrainDeadlineAt, op.State = &deadline, "DRAINING"
	case lifecycle.Prepare:
		if c.Queue == nil || !c.Queue.Valid(x.op.Scope, x.now) {
			return nil, drafts.ErrConflict
		}
		id, err := drafts.NewID()
		if err != nil {
			return nil, err
		}
		for _, gap := range c.Queue.Gaps {
			if !draftjson.LifecycleText(gap) {
				return nil, drafts.ErrUnavailable
			}
		}
		expires := c.Queue.ExpiresAt.UTC().Truncate(time.Microsecond)
		if x.draft.ConfigExpiry.Before(expires) {
			expires = x.draft.ConfigExpiry
		}
		node.Prepared = &lifecycle.SavedSnapshot{ActorID: g.PrincipalID, Epoch: g.Epoch, Evidence: *c.Queue,
			View: drafts.ResumeSnapshot{SnapshotID: id, OperationID: r.OperationID, Revision: op.Revision,
				SignalCount: c.Queue.SignalCount, SerializedBytes: c.Queue.SerializedBytes, OldestReceivedAt: c.Queue.OldestReceivedAt,
				Gaps: c.Queue.Gaps, CreatedAt: x.now, ExpiresAt: expires}}
		if _, err := (draftjson.Codec{}).EncodeLifecycle(lifecycle.Snapshot, node.Prepared.View); err != nil {
			return nil, err
		}
		op.ConnectionHealth = "READY"
	case lifecycle.Accept:
		if expected.Snapshot == nil || !reflect.DeepEqual(expected.Snapshot, x.op.Snapshot) ||
			in.HeldDisposition == nil || (*in.HeldDisposition != lifecycle.Process && *in.HeldDisposition != lifecycle.Discard) {
			return nil, drafts.ErrConflict
		}
		var occupied bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id<>$2 AND state IN ('ACTIVE','DRAINING'))`, r.WorkspaceID, r.OperationID).Scan(&occupied); err != nil {
			return nil, err
		}
		if occupied {
			return nil, drafts.ErrConflict
		}
		node.Accepted, node.Disposition = x.op.Snapshot, *in.HeldDisposition
		pauseAt, op.DrainDeadlineAt, op.State, op.ConnectionHealth = nil, nil, "ACTIVE", "READY"
	}
	body, err := (draftjson.Codec{}).EncodeLifecycle(lifecycle.Change, op)
	if err != nil {
		return nil, err
	}
	node.Response = body
	raw, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	tail, err := d.writeFile(ctx, raw, r.WorkspaceID, did, x.draft.Config.Revision, x.draft.ConfigExpiry, x.now)
	if err != nil {
		return nil, err
	}
	root := x.draft.Root
	root.Lifecycle = &tail
	raw, err = json.Marshal(root)
	if err != nil {
		return nil, err
	}
	ref, err := d.writeFile(ctx, raw, r.WorkspaceID, did, x.draft.Config.Revision, x.draft.ConfigExpiry, x.now)
	if err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `UPDATE hp_processing.operations SET revision=$3,state=$4,pause_accepted_at=$5,drain_deadline_at=$6 WHERE workspace_id=$1 AND operation_id=$2 AND revision=$7`,
		r.WorkspaceID, r.OperationID, op.Revision, op.State, pauseAt, op.DrainDeadlineAt, node.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, drafts.ErrStale
	}
	if _, err = tx.Exec(ctx, `UPDATE hp_processing.drafts SET prose_ref=$3,prose_sha256=$4,prose_expires_at=$5 WHERE workspace_id=$1 AND draft_id=$2`,
		r.WorkspaceID, did, ref.ID, ref.Digest, ref.ExpiresAt); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO hp_processing.user_commands(workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,'APPLIED',$8)`, r.WorkspaceID, r.RequestID, g.PrincipalID, in.Digest, r.OperationID, node.ExpectedRevision, node.ResultRevision, x.now); err != nil {
		return nil, err
	}
	var final time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&final); err != nil {
		return nil, err
	}
	if now := d.Clock.Now(); now.After(final) {
		final = now
	}
	if !final.Before(g.ExpiresAt) || !final.Before(x.draft.ConfigExpiry) ||
		c.Binding != nil && !lifecycle.BindingValid(x.op.Binding, *c.Binding, g, final) ||
		node.Prepared != nil && !final.Before(node.Prepared.View.ExpiresAt) ||
		node.Accepted != nil && !lifecycle.SnapshotValid(x.op, g, final) {
		return nil, drafts.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return bytes.Clone(body), nil
}
