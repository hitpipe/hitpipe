package postgres

import (
	"context"
	"encoding/json"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/jackc/pgx/v5"
	"reflect"
)

// selectedCheck reads exactly the caller-selected durable check, not a latest
// check or freshly resolved alias. It never calls an owner or writes anything.
func (d DraftStore) selectedCheck(ctx context.Context, tx pgx.Tx, r drafts.Request, snap drafts.Snapshot, id string, g drafts.Grant) (*drafts.CheckEvidence, error) {
	if !drafts.UUID(id) {
		return nil, drafts.ErrInvalid
	}
	ref := snap.Root.Tail
	for n := 0; ref != nil && n < d.Policy.ReplayNodes; n++ {
		raw, e := d.readFile(ctx, *ref, r.WorkspaceID, r.DraftID, snap.Config.Revision, d.Clock.Now())
		if e != nil {
			return nil, drafts.ErrUnavailable
		}
		var node drafts.CommandNode
		if frame(raw, &node) != nil || node.Format != "draft-command-v1" || node.WorkspaceID != r.WorkspaceID || node.DraftID != r.DraftID || node.ResultRevision != ref.Revision || node.AuthorityEpoch < 1 {
			return nil, drafts.ErrUnavailable
		}
		if node.Kind == drafts.Check {
			body, e := d.readFile(ctx, node.Response, r.WorkspaceID, r.DraftID, snap.Config.Revision, d.Clock.Now())
			if e != nil {
				return nil, drafts.ErrUnavailable
			}
			var check drafts.ReadinessCheck
			if json.Unmarshal(body, &check) != nil {
				return nil, drafts.ErrUnavailable
			}
			if check.CheckID == id {
				// Historical missing preview is not upgraded under a new consent.
				var historical map[string]json.RawMessage
				json.Unmarshal(body, &historical)
				if _, ok := historical["activation_preview"]; !ok {
					return nil, drafts.ErrConflict
				}
				if d.Codec.ValidateResponse(drafts.Check, body) != nil {
					return nil, drafts.ErrUnavailable
				}
				if !check.Passed {
					return nil, drafts.ErrConflict
				}
				if d.Codec.ValidateResponse(drafts.Check, body) != nil || node.BindingCheck == nil || check.ActivationPreview == nil || node.Response.Revision != node.ResultRevision {
					return nil, drafts.ErrUnavailable
				}
				if node.ActorID != g.PrincipalID || node.AuthorityEpoch != g.Epoch {
					return nil, drafts.ErrConflict
				}
				if check.DraftID != r.DraftID || check.DraftRevision != snap.Config.Revision {
					return nil, drafts.ErrStale
				}
				if !responsePins(drafts.Check, body, r.DraftID, node.ResultRevision, node.ConfigDigest) || node.ConfigDigest != snap.ConfigDigest {
					return nil, drafts.ErrConflict
				}
				p := *node.BindingCheck
				now := d.Clock.Now()
				if snap.LoadedAt.After(now) {
					now = snap.LoadedAt
				}
				if !now.Before(check.ExpiresAt) || !now.Before(p.ExpiresAt) || !now.Before(snap.ConfigExpiry) {
					return nil, drafts.ErrConflict
				}
				if p.WorkspaceID != r.WorkspaceID || p.PrincipalID != node.ActorID || p.Epoch != node.AuthorityEpoch || p.CheckedAt.IsZero() || p.CheckedAt.After(check.CheckedAt) || check.CheckedAt.IsZero() || check.CheckedAt.After(now) || check.ExpiresAt.After(p.ExpiresAt) || check.ExpiresAt.After(snap.ConfigExpiry) {
					return nil, drafts.ErrUnavailable
				}
				expected, e := d.Codec.Preview(snap.Config, p)
				if e != nil || !reflect.DeepEqual(expected, *check.ActivationPreview) || node.AcceptedConfigDigest != expected.AcceptedConfigDigest {
					return nil, drafts.ErrUnavailable
				}
				meta, e := loadDraftCommand(ctx, tx, r.WorkspaceID, node.RequestID)
				if e != nil || !meta.exists || meta.actor != node.ActorID || meta.aggregate != r.DraftID || meta.digest != node.Digest || meta.disposition != "APPLIED" || meta.result == nil || *meta.result != node.ResultRevision || !sameInt(meta.expected, node.ExpectedRevision) {
					return nil, drafts.ErrUnavailable
				}
				return &drafts.CheckEvidence{Check: check, Proof: p}, nil
			}
		}
		ref = node.Previous
	}
	if ref != nil {
		return nil, drafts.ErrUnavailable
	}
	return nil, drafts.ErrConflict
}
