package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gowebpki/jcs"
	"github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/activation"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/jackc/pgx/v5"
	"reflect"
	"time"
)

// ActivationStore reuses the same draft root, command ledger and owner files.
// No operation payload or full response is encoded into user_commands or SQL.
type ActivationStore struct{ Drafts DraftStore }

func (a ActivationStore) read(ctx context.Context, tx pgx.Tx, r activation.Request, in activation.Input, g drafts.Grant, lock bool) (activation.Snapshot, error) {
	d := a.Drafts
	var s activation.Snapshot
	req := drafts.Request{Kind: activation.Kind, WorkspaceID: r.WorkspaceID, DraftID: in.Command.DraftID}
	meta, e := loadDraftCommand(ctx, tx, r.WorkspaceID, in.Command.Command.RequestID)
	if e != nil {
		return s, e
	}
	if meta.exists && (meta.actor != g.PrincipalID || meta.digest != in.Digest || !sameInt(meta.expected, in.Command.Command.ExpectedRevision)) {
		return s, drafts.ErrConflict
	}
	row, e := loadDraftRow(ctx, tx, r.WorkspaceID, req.DraftID, lock)
	if e != nil {
		return s, e
	}
	if !row.exists {
		return s, drafts.ErrNotFound
	}
	s.Draft, e = d.root(ctx, req, row)
	if e != nil {
		return s, e
	}
	var operation *string
	if e = tx.QueryRow(ctx, `SELECT operation_id::text FROM hp_processing.drafts WHERE workspace_id=$1 AND draft_id=$2`, r.WorkspaceID, req.DraftID).Scan(&operation); e != nil {
		return s, e
	}
	if operation != nil {
		s.OperationID = *operation
		q := `SELECT owner_user_id::text,revision,state,first_activation_at FROM hp_processing.operations WHERE workspace_id=$1 AND operation_id=$2`
		if lock {
			q += ` FOR UPDATE`
		}
		e = tx.QueryRow(ctx, q, r.WorkspaceID, s.OperationID).Scan(&s.OperationOwner, &s.OperationRevision, &s.OperationState, &s.FirstActivation)
		if e != nil {
			return s, drafts.ErrUnavailable
		}
	}

	if !meta.exists {
		if s.Draft.Config.Revision != in.Command.DraftRevision {
			return s, drafts.ErrStale
		}
		selected, e := d.selectedCheck(ctx, tx, req, s.Draft, in.Command.ReadinessCheckID, g)
		if e != nil {
			return s, e
		}
		s.Check = selected.Check
		s.Binding = selected.Proof
	}
	ref := s.Draft.Root.Tail
	for n := 0; ref != nil && n < d.Policy.ReplayNodes; n++ {
		raw, e := d.readFile(ctx, *ref, r.WorkspaceID, req.DraftID, s.Draft.Config.Revision, d.Clock.Now())
		if e != nil {
			return s, drafts.ErrUnavailable
		}
		var node drafts.CommandNode
		if frame(raw, &node) != nil || node.Format != "draft-command-v1" || node.WorkspaceID != r.WorkspaceID || node.DraftID != req.DraftID || node.ResultRevision != ref.Revision || node.AuthorityEpoch < 1 {
			return s, drafts.ErrUnavailable
		}
		if meta.exists && node.RequestID == in.Command.Command.RequestID {
			if node.Kind != activation.Kind || node.Digest != in.Digest || node.ActorID != g.PrincipalID || node.OperationID != meta.aggregate || meta.result == nil || node.OperationRevision != *meta.result || meta.disposition != "APPLIED" || !sameInt(meta.expected, node.ExpectedRevision) || node.OperationID != s.OperationID {
				return s, drafts.ErrConflict
			}
			body, e := d.readFile(ctx, node.Response, r.WorkspaceID, req.DraftID, s.Draft.Config.Revision, d.Clock.Now())
			if e != nil || draftjson.Validate("Operation", body) != nil {
				return s, drafts.ErrUnavailable
			}
			var op drafts.Operation
			final := node.AcceptedConfigDigest
			if final == "" {
				final = node.ConfigDigest
			} // unchanged historical exact receipt only
			if json.Unmarshal(body, &op) != nil || op.OperationID != node.OperationID || op.Revision != node.OperationRevision || op.AcceptedConfigDigest == nil || *op.AcceptedConfigDigest != final || final != in.Command.AcceptedConfigDigest || op.State != "ACTIVE" || op.FirstActivationAt == nil {
				return s, drafts.ErrUnavailable
			}
			s.Replay = body
			return s, nil
		}
		if !meta.exists && node.Kind == drafts.Rehearse {
			body, e := d.readFile(ctx, node.Response, r.WorkspaceID, req.DraftID, s.Draft.Config.Revision, d.Clock.Now())
			if e != nil {
				return s, drafts.ErrUnavailable
			}
			var rr drafts.RehearsalResult
			if json.Unmarshal(body, &rr) != nil {
				return s, drafts.ErrUnavailable
			}
			if rr.RehearsalID == in.Command.RehearsalID {
				if d.Codec.ValidateResponse(drafts.Rehearse, body) != nil || !responsePins(drafts.Rehearse, body, req.DraftID, node.ResultRevision, node.ConfigDigest) || node.Response.Revision != node.ResultRevision {
					return s, drafts.ErrUnavailable
				}
				if node.ResultRevision != in.Command.DraftRevision {
					return s, drafts.ErrStale
				}
				if node.ConfigDigest != s.Draft.ConfigDigest || node.ReadinessCheckID != s.Check.CheckID || node.AcceptedConfigDigest != in.Command.AcceptedConfigDigest || node.BindingCheck == nil || node.ActorID != g.PrincipalID || node.AuthorityEpoch != g.Epoch || !drafts.SameBinding(s.Binding, *node.BindingCheck) {
					return s, drafts.ErrConflict
				}
				original := drafts.RehearsalRequest{Command: drafts.CommandMeta{RequestID: node.RequestID, ExpectedRevision: node.ExpectedRevision}, DraftID: node.DraftID, DraftRevision: node.ResultRevision, Examples: node.Examples, ReadinessCheckID: node.ReadinessCheckID}
				reconstructed, e := json.Marshal(original)
				if e != nil {
					return s, drafts.ErrUnavailable
				}
				reconstructed, e = jcs.Transform(reconstructed)
				if e != nil || draftjson.Hash(reconstructed) != node.Digest {
					return s, drafts.ErrUnavailable
				}
				check, e := loadDraftCommand(ctx, tx, r.WorkspaceID, node.RequestID)
				if e != nil || !check.exists || check.disposition != "APPLIED" || check.actor != node.ActorID || check.aggregate != req.DraftID || check.digest != node.Digest || check.result == nil || *check.result != node.ResultRevision || !sameInt(check.expected, node.ExpectedRevision) {
					return s, drafts.ErrUnavailable
				}
				s.Rehearsal = rr
				s.Examples = node.Examples
				s.RehearsalDigest = draftjson.Hash(body)
				s.RehearsalReadinessID = node.ReadinessCheckID
				s.RehearsalFinalDigest = node.AcceptedConfigDigest
				s.RehearsalBinding = node.BindingCheck
				return s, nil
			}
		}
		ref = node.Previous
	}
	if ref != nil || meta.exists {
		return s, drafts.ErrUnavailable
	}
	return s, drafts.ErrConflict
}

func (a ActivationStore) Load(ctx context.Context, r activation.Request, in activation.Input, g drafts.Grant) (activation.Snapshot, error) {
	tx, e := a.Drafts.tx(ctx, r.WorkspaceID)
	if e != nil {
		return activation.Snapshot{}, e
	}
	defer tx.Rollback(ctx)
	// Read the command/root pair under the same short lock order as writers.
	// No owner/network lookup is made while either lock is held.
	for _, key := range []string{r.WorkspaceID + ":command:" + in.Command.Command.RequestID, r.WorkspaceID + ":draft:" + in.Command.DraftID} {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); e != nil {
			return activation.Snapshot{}, e
		}
	}
	s, e := a.read(ctx, tx, r, in, g, false)
	if e != nil {
		return s, e
	}
	if e = tx.Commit(ctx); e != nil {
		return s, e
	}
	return s, nil
}
func (a ActivationStore) Commit(ctx context.Context, r activation.Request, in activation.Input, g drafts.Grant, expected activation.Snapshot, p drafts.BindingProof) ([]byte, error) {
	d := a.Drafts
	tx, e := d.tx(ctx, r.WorkspaceID)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	for _, key := range []string{r.WorkspaceID + ":command:" + in.Command.Command.RequestID, r.WorkspaceID + ":draft:" + in.Command.DraftID, r.WorkspaceID + ":activation"} {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); e != nil {
			return nil, e
		}
	}
	s, e := a.read(ctx, tx, r, in, g, true)
	if e != nil {
		return nil, e
	}
	if s.Replay != nil {
		if e = tx.Commit(ctx); e != nil {
			return nil, e
		}
		return s.Replay, nil
	}
	now := d.Clock.Now().UTC().Truncate(time.Microsecond)
	var dbNow time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow); e != nil {
		return nil, e
	}
	if dbNow.After(now) {
		now = dbNow.UTC().Truncate(time.Microsecond)
	}
	if !now.Before(g.ExpiresAt) {
		return nil, drafts.ErrDenied
	}
	if s.Draft.Config.Revision != expected.Draft.Config.Revision || s.Draft.ConfigDigest != expected.Draft.ConfigDigest || s.OperationID != expected.OperationID || s.OperationRevision != expected.OperationRevision {
		return nil, drafts.ErrStale
	}
	if e = activation.CheckSaved(in, s, g, now); e != nil {
		return nil, e
	}
	if !activation.FreshBinding(expected, g, p, now) {
		return nil, drafts.ErrConflict
	}
	if s.Check.CheckID != expected.Check.CheckID || !s.Check.ExpiresAt.Equal(expected.Check.ExpiresAt) || s.RehearsalDigest != expected.RehearsalDigest {
		return nil, drafts.ErrConflict
	}
	var n int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM hp_processing.operations WHERE workspace_id=$1 AND state IN ('ACTIVE','DRAINING')`, r.WorkspaceID).Scan(&n); e != nil {
		return nil, e
	}
	if n != 0 {
		return nil, drafts.ErrConflict
	}
	c := s.Draft.Config
	shown := s.Check.ActivationPreview
	if shown == nil {
		return nil, drafts.ErrConflict
	}
	// Rebuild only to verify the selected immutable snapshot; never replace it.
	rebuilt, e := d.Codec.Preview(c, p)
	if e != nil || !reflect.DeepEqual(rebuilt, *shown) || shown.AcceptedConfigDigest != in.Command.AcceptedConfigDigest {
		return nil, drafts.ErrConflict
	}
	raw, e := json.Marshal(shown.CanonicalConfig)
	if e != nil {
		return nil, e
	}
	raw, e = jcs.Transform(raw)
	if e != nil || draftjson.Hash(raw) != shown.AcceptedConfigDigest {
		return nil, drafts.ErrUnavailable
	}
	// SQL's safe_config_valid remains authoritative in addition to application checks.
	var safe bool
	if e = tx.QueryRow(ctx, `SELECT hp_processing.safe_config_valid($1::jsonb)`, string(raw)).Scan(&safe); e != nil || !safe {
		return nil, drafts.ErrInvalid
	}
	id := s.OperationID
	rev := s.OperationRevision + 1
	if id == "" {
		id, e = drafts.NewID()
		if e != nil {
			return nil, e
		}
		rev = 1
	}
	if rev > 9007199254740991 {
		return nil, drafts.ErrStale
	}
	configID, e := drafts.NewID()
	if e != nil {
		return nil, e
	}
	first := s.FirstActivation
	if first == nil {
		first = &now
	}
	view := drafts.Operation{OperationID: id, Revision: rev, Name: c.Name, State: "ACTIVE", FirstActivationAt: first, AcceptedConfigID: &configID, AcceptedConfigDigest: &shown.AcceptedConfigDigest, ConnectionHealth: "READY"}
	body, e := json.Marshal(view)
	if e != nil {
		return nil, e
	}
	body, e = jcs.Transform(body)
	if e != nil || draftjson.Validate("Operation", body) != nil {
		return nil, drafts.ErrUnavailable
	}
	// Store full response and command envelope BEFORE publishing SQL refs/ACTIVE.
	response, e := d.writeFile(ctx, body, r.WorkspaceID, c.DraftID, c.Revision, s.Draft.ConfigExpiry, now)
	if e != nil {
		return nil, e
	}
	node := drafts.CommandNode{ReadinessCheckID: s.Check.CheckID, AcceptedConfigDigest: shown.AcceptedConfigDigest, Format: "draft-command-v1", WorkspaceID: r.WorkspaceID, DraftID: c.DraftID, RequestID: in.Command.Command.RequestID, ActorID: g.PrincipalID, Digest: in.Digest, Kind: activation.Kind, ExpectedRevision: in.Command.Command.ExpectedRevision, ResultRevision: c.Revision, ConfigDigest: s.Draft.ConfigDigest, AuthorityEpoch: g.Epoch, BindingCheck: &p, Response: response, Previous: s.Draft.Root.Tail, OperationID: id, OperationRevision: rev}
	rawNode, e := json.Marshal(node)
	if e != nil {
		return nil, e
	}
	tail, e := d.writeFile(ctx, rawNode, r.WorkspaceID, c.DraftID, c.Revision, s.Draft.ConfigExpiry, now)
	if e != nil {
		return nil, e
	}
	root := s.Draft.Root
	root.Tail = &tail
	rawRoot, e := json.Marshal(root)
	if e != nil {
		return nil, e
	}
	rootRef, e := d.writeFile(ctx, rawRoot, r.WorkspaceID, c.DraftID, c.Revision, s.Draft.ConfigExpiry, now)
	if e != nil {
		return nil, e
	}
	if s.OperationID == "" {
		_, e = tx.Exec(ctx, `INSERT INTO hp_processing.operations(workspace_id,operation_id,owner_user_id,revision,state,first_activation_at) VALUES($1,$2,$3,$4,'ACTIVE',$5)`, r.WorkspaceID, id, g.PrincipalID, rev, *first)
	} else {
		_, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET revision=$3,state='ACTIVE',first_activation_at=COALESCE(first_activation_at,$4) WHERE workspace_id=$1 AND operation_id=$2`, r.WorkspaceID, id, rev, *first)
	}
	if e != nil {
		return nil, activationSQLError(e)
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.accepted_configs(workspace_id,config_id,operation_id,schema_version,evaluator_version,canonical_config,digest,accepted_by,accepted_at,rehearsal_digest) VALUES($1,$2,$3,'1.0','paid-order-v1',$4,$5,$6,$7,$8)`, r.WorkspaceID, configID, id, string(raw), shown.AcceptedConfigDigest, g.PrincipalID, now, s.RehearsalDigest)
	if e != nil {
		return nil, activationSQLError(e)
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.operations SET selected_config_id=$3 WHERE workspace_id=$1 AND operation_id=$2`, r.WorkspaceID, id, configID)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `UPDATE hp_processing.drafts SET operation_id=$3,prose_ref=$4,prose_sha256=$5,prose_expires_at=$6 WHERE workspace_id=$1 AND draft_id=$2`, r.WorkspaceID, c.DraftID, id, rootRef.ID, rootRef.Digest, rootRef.ExpiresAt)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.user_commands(workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,'APPLIED',$8)`, r.WorkspaceID, in.Command.Command.RequestID, g.PrincipalID, in.Digest, id, in.Command.Command.ExpectedRevision, rev, now)
	if e != nil {
		return nil, e
	}
	if !d.Clock.Now().Before(s.Check.ExpiresAt) || !d.Clock.Now().Before(p.ExpiresAt) || !d.Clock.Now().Before(g.ExpiresAt) {
		return nil, drafts.ErrUnavailable
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return bytes.Clone(body), nil
}
func activationSQLError(e error) error {
	var constraint interface{ SQLState() string }
	if errors.As(e, &constraint) && constraint.SQLState() == "23505" {
		return drafts.ErrConflict
	}
	return e
}
