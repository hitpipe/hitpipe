package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/gowebpki/jcs"

	"github.com/hitpipe/hitpipe/services/processing/internal/application/drafts"
	"github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase"
	"github.com/jackc/pgx/v5"
	"io"
	"time"
)

type DraftStore struct {
	DB     Beginner
	Files  drafts.FileStore
	Codec  drafts.Codec
	Clock  drafts.Clock
	Policy drafts.Policy
}

func (d DraftStore) tx(ctx context.Context, ws string) (pgx.Tx, error) {
	if d.DB == nil || drafts.Missing(d.Files) || drafts.Missing(d.Codec) || drafts.Missing(d.Clock) || !d.Policy.Valid() {
		return nil, drafts.ErrUnavailable
	}
	tx, e := scoped(ctx, d.DB, ws)
	if e != nil {
		return nil, e
	}
	var valid bool
	e = tx.QueryRow(ctx, `SELECT NOT rolsuper AND NOT rolbypassrls AND NOT rolcreaterole AND NOT rolcreatedb AND pg_has_role(current_user,'hp_processing_app','member') FROM pg_roles WHERE rolname=current_user`).Scan(&valid)
	if e != nil || !valid {
		tx.Rollback(ctx)
		return nil, drafts.ErrUnavailable
	}
	return tx, nil
}
func frame(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return drafts.ErrUnavailable
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return drafts.ErrUnavailable
	}
	return nil
}
func origin(ws, id string, revision int64) preparecase.Origin {
	return preparecase.Origin{WorkspaceID: ws, CaseID: id, Revision: revision}
}
func (d DraftStore) readFile(ctx context.Context, r drafts.Ref, ws, id string, ceiling int64, now time.Time) ([]byte, error) {
	if r.Revision < 1 || r.Revision > ceiling || r.Class != "DRAFT_DATA" {
		return nil, drafts.ErrUnavailable
	}
	return d.Files.ReadDraft(ctx, r, origin(ws, id, r.Revision), now)
}
func (d DraftStore) writeFile(ctx context.Context, b []byte, ws, id string, rev int64, expiry, now time.Time) (drafts.Ref, error) {
	r, e := d.Files.Write(ctx, b, "DRAFT_DATA", origin(ws, id, rev), expiry, now)
	return drafts.Ref{Ref: r, Revision: rev}, e
}

type draftRow struct {
	exists              bool
	revision            int64
	technical           []byte
	rootID, rootDigest  *string
	proseExpiry, expiry *time.Time
	updated, now        time.Time
}

func loadDraftRow(ctx context.Context, tx pgx.Tx, ws, id string, lock bool) (draftRow, error) {
	var x draftRow
	q := `SELECT revision,technical_config,prose_ref::text,prose_sha256,prose_expires_at,expires_at,updated_at,clock_timestamp() FROM hp_processing.drafts WHERE workspace_id=$1 AND draft_id=$2`
	if lock {
		q += ` FOR UPDATE`
	}
	e := tx.QueryRow(ctx, q, ws, id).Scan(&x.revision, &x.technical, &x.rootID, &x.rootDigest, &x.proseExpiry, &x.expiry, &x.updated, &x.now)
	if errors.Is(e, pgx.ErrNoRows) {
		e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&x.now)
		return x, e
	}
	x.exists = e == nil
	return x, e
}

type draftCommand struct {
	exists                                bool
	actor, digest, aggregate, disposition string
	expected, result                      *int64
}

func loadDraftCommand(ctx context.Context, tx pgx.Tx, ws, id string) (draftCommand, error) {
	var x draftCommand
	e := tx.QueryRow(ctx, `SELECT actor_id::text,digest,aggregate_id::text,expected_revision,result_revision,disposition FROM hp_processing.user_commands WHERE workspace_id=$1 AND request_id=$2`, ws, id).Scan(&x.actor, &x.digest, &x.aggregate, &x.expected, &x.result, &x.disposition)
	if errors.Is(e, pgx.ErrNoRows) {
		return x, nil
	}
	x.exists = e == nil
	return x, e
}
func sameInt(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// Only these shape-safe non-prose fields go into SQL. Name, aliases and catalog
// label remain in owner content even when they happen to be ASCII/SQL-valid.
func technical(c drafts.DraftConfig) []byte {
	m := map[string]any{}
	for k, v := range map[string]*string{"source_connection_id": c.SourceConnectionID, "destination_connection_id": c.DestinationConnectionID, "mode": c.Mode, "title_pattern": c.TitlePattern} {
		if v != nil {
			m[k] = *v
		}
	}
	if c.RequiredContextKeys != nil {
		m["required_context_keys"] = *c.RequiredContextKeys
	}
	b, _ := json.Marshal(m)
	return b
}
func (d DraftStore) root(ctx context.Context, r drafts.Request, x draftRow) (drafts.Snapshot, error) {
	z := drafts.Snapshot{}
	now := d.Clock.Now()
	if x.now.After(now) {
		now = x.now
	}
	if !x.exists {
		return z, nil
	}
	if x.rootID == nil || x.rootDigest == nil || x.proseExpiry == nil || x.expiry == nil || !x.expiry.After(now) || !x.proseExpiry.Equal(*x.expiry) {
		return z, drafts.ErrUnavailable
	}
	raw, e := d.Files.ReadDraftRoot(ctx, *x.rootID, *x.rootDigest, *x.proseExpiry, origin(r.WorkspaceID, r.DraftID, x.revision), now)
	if e != nil {
		return z, drafts.ErrUnavailable
	}
	var root drafts.Root
	if frame(raw, &root) != nil || root.Format != "draft-root-v1" || root.WorkspaceID != r.WorkspaceID || root.DraftID != r.DraftID || root.Revision != x.revision || root.Config.Revision != x.revision {
		return z, drafts.ErrUnavailable
	}
	raw, e = d.readFile(ctx, root.Config, r.WorkspaceID, r.DraftID, x.revision, now)
	if e != nil || d.Codec.ValidateResponse(drafts.Get, raw) != nil {
		return z, drafts.ErrUnavailable
	}
	var config drafts.DraftConfig
	if json.Unmarshal(raw, &config) != nil || config.DraftID != r.DraftID || config.Revision != x.revision {
		return z, drafts.ErrUnavailable
	}
	digest, e := d.Codec.ConfigDigest(config)
	if e != nil || digest != root.ConfigDigest || !root.Config.ExpiresAt.Equal(*x.expiry) {
		return z, drafts.ErrUnavailable
	}
	a, e := jcs.Transform(technical(config))
	if e != nil {
		return z, drafts.ErrUnavailable
	}
	b, e := jcs.Transform(x.technical)
	if e != nil || !bytes.Equal(a, b) {
		return z, drafts.ErrUnavailable
	}
	return drafts.Snapshot{Exists: true, Config: config, ConfigDigest: digest, ConfigExpiry: *x.expiry, UpdatedAt: x.updated, LoadedAt: x.now, Root: root}, nil
}
func (d DraftStore) replay(ctx context.Context, r drafts.Request, in drafts.Input, g drafts.Grant, snap drafts.Snapshot, meta draftCommand) ([]byte, error) {
	if !meta.exists {
		return nil, nil
	}
	if meta.actor != g.PrincipalID || meta.digest != in.Digest || meta.aggregate != r.DraftID || !sameInt(meta.expected, in.Command.ExpectedRevision) {
		return nil, drafts.ErrConflict
	}
	if meta.result == nil || *meta.result < 1 || meta.disposition != "APPLIED" || !snap.Exists {
		return nil, drafts.ErrUnavailable
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
		if node.RequestID == in.Command.RequestID {
			if node.ActorID != g.PrincipalID || node.Digest != meta.digest || node.Kind != r.Kind || node.ResultRevision != *meta.result || !sameInt(node.ExpectedRevision, meta.expected) || node.Response.Revision != node.ResultRevision {
				return nil, drafts.ErrUnavailable
			}
			body, e := d.readFile(ctx, node.Response, r.WorkspaceID, r.DraftID, snap.Config.Revision, d.Clock.Now())
			if e != nil || d.Codec.ValidateResponse(r.Kind, body) != nil {
				return nil, drafts.ErrUnavailable
			}
			if r.Kind == drafts.Check {
				var check drafts.ReadinessCheck
				if json.Unmarshal(body, &check) != nil {
					return nil, drafts.ErrUnavailable
				}
				if check.Passed && (node.BindingCheck == nil || node.BindingCheck.Epoch != node.AuthorityEpoch || node.BindingCheck.PrincipalID != node.ActorID || node.BindingCheck.WorkspaceID != node.WorkspaceID) {
					return nil, drafts.ErrUnavailable
				}
			}
			if !responsePins(r.Kind, body, r.DraftID, node.ResultRevision, node.ConfigDigest) {
				return nil, drafts.ErrUnavailable
			}
			return body, nil
		}
		ref = node.Previous
	}
	return nil, drafts.ErrUnavailable
}
func responsePins(k drafts.Kind, b []byte, id string, rev int64, digest string) bool {
	switch k {
	case drafts.Save, drafts.Get:
		var v drafts.DraftConfig
		return json.Unmarshal(b, &v) == nil && v.DraftID == id && v.Revision == rev
	case drafts.Check:
		var v drafts.ReadinessCheck
		return json.Unmarshal(b, &v) == nil && v.DraftID == id && v.DraftRevision == rev && v.ConfigDigest == digest
	case drafts.Rehearse:
		var v drafts.RehearsalResult
		return json.Unmarshal(b, &v) == nil && v.DraftID == id && v.DraftRevision == rev && v.ConfigDigest == digest && v.LiveRequestsMade == 0
	}
	return false
}
func (d DraftStore) Load(ctx context.Context, r drafts.Request, in drafts.Input, g drafts.Grant) (drafts.Snapshot, error) {
	z := drafts.Snapshot{}
	tx, e := d.tx(ctx, r.WorkspaceID)
	if e != nil {
		return z, e
	}
	defer tx.Rollback(ctx)
	var meta draftCommand
	if r.Kind != drafts.Get {
		meta, e = loadDraftCommand(ctx, tx, r.WorkspaceID, in.Command.RequestID)
		if e != nil {
			return z, e
		}
		if meta.exists && (meta.actor != g.PrincipalID || meta.digest != in.Digest || meta.aggregate != r.DraftID || !sameInt(meta.expected, in.Command.ExpectedRevision)) {
			return z, drafts.ErrConflict
		}
	}
	row, e := loadDraftRow(ctx, tx, r.WorkspaceID, r.DraftID, false)
	if e != nil {
		return z, e
	}
	if !row.exists && r.Kind != drafts.Save {
		return z, drafts.ErrNotFound
	}
	snap, e := d.root(ctx, r, row)
	if e != nil {
		return z, e
	}
	if meta.exists {
		snap.Replay, e = d.replay(ctx, r, in, g, snap, meta)
		if e != nil {
			return z, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return z, e
	}
	return snap, nil
}
func (d DraftStore) Commit(ctx context.Context, r drafts.Request, in drafts.Input, g drafts.Grant, expected drafts.Snapshot, c drafts.Candidate) ([]byte, error) {
	if d.Codec.ValidateResponse(r.Kind, c.Response) != nil || c.Config.DraftID != r.DraftID || !responsePins(r.Kind, c.Response, r.DraftID, c.Config.Revision, c.ConfigDigest) {
		return nil, drafts.ErrUnavailable
	}
	sum, e := d.Codec.ConfigDigest(c.Config)
	if e != nil || sum != c.ConfigDigest {
		return nil, drafts.ErrUnavailable
	}
	tx, e := d.tx(ctx, r.WorkspaceID)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	// Command lock precedes draft lock consistently. Creation needs the aggregate
	// lock because there is no row yet. All keys are opaque IDs, not config prose.
	for _, key := range []string{r.WorkspaceID + ":command:" + in.Command.RequestID, r.WorkspaceID + ":draft:" + r.DraftID} {
		if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); e != nil {
			return nil, e
		}
	}
	meta, e := loadDraftCommand(ctx, tx, r.WorkspaceID, in.Command.RequestID)
	if e != nil {
		return nil, e
	}
	row, e := loadDraftRow(ctx, tx, r.WorkspaceID, r.DraftID, true)
	if e != nil {
		return nil, e
	}
	current, e := d.root(ctx, r, row)
	if e != nil {
		return nil, e
	}
	if meta.exists {
		body, e := d.replay(ctx, r, in, g, current, meta)
		if e != nil {
			return nil, e
		}
		if e = tx.Commit(ctx); e != nil {
			return nil, e
		}
		return body, nil
	}
	if current.Exists != expected.Exists || (current.Exists && (current.Config.Revision != expected.Config.Revision || current.ConfigDigest != expected.ConfigDigest)) {
		return nil, drafts.ErrStale
	}
	resultRevision := int64(1)
	if current.Exists {
		resultRevision = current.Config.Revision
		if r.Kind == drafts.Save {
			resultRevision++
		}
	}
	if c.Config.Revision != resultRevision || r.Kind != drafts.Save && (!current.Exists || !c.ContentExpiry.Equal(current.ConfigExpiry) || c.ConfigDigest != current.ConfigDigest) {
		return nil, drafts.ErrStale
	}
	now := row.now
	if d.Clock.Now().After(now) {
		now = d.Clock.Now()
	}
	if !now.Before(g.ExpiresAt) || !now.Before(c.ContentExpiry) || (c.PositiveValidUntil != nil && !now.Before(*c.PositiveValidUntil)) {
		return nil, drafts.ErrUnavailable
	}
	configRef := current.Root.Config
	if r.Kind == drafts.Save {
		raw, e := d.Codec.Encode(drafts.Get, c.Config)
		if e != nil {
			return nil, e
		}
		configRef, e = d.writeFile(ctx, raw, r.WorkspaceID, r.DraftID, resultRevision, c.ContentExpiry, now)
		if e != nil {
			return nil, e
		}
	}
	responseRef, e := d.writeFile(ctx, c.Response, r.WorkspaceID, r.DraftID, resultRevision, c.ContentExpiry, now)
	if e != nil {
		return nil, e
	}
	node := drafts.CommandNode{AuthorityEpoch: g.Epoch, BindingCheck: c.BindingCheck, Format: "draft-command-v1", WorkspaceID: r.WorkspaceID, DraftID: r.DraftID, RequestID: in.Command.RequestID, ActorID: g.PrincipalID, Digest: in.Digest, Kind: r.Kind, ExpectedRevision: in.Command.ExpectedRevision, ResultRevision: resultRevision, ConfigDigest: c.ConfigDigest, Response: responseRef, Previous: current.Root.Tail}
	if r.Kind == drafts.Rehearse {
		node.Examples = in.Examples
	}
	raw, e := json.Marshal(node)
	if e != nil {
		return nil, e
	}
	tail, e := d.writeFile(ctx, raw, r.WorkspaceID, r.DraftID, resultRevision, c.ContentExpiry, now)
	if e != nil {
		return nil, e
	}
	root := drafts.Root{Format: "draft-root-v1", WorkspaceID: r.WorkspaceID, DraftID: r.DraftID, Revision: resultRevision, Config: configRef, ConfigDigest: c.ConfigDigest, Tail: &tail}
	raw, e = json.Marshal(root)
	if e != nil {
		return nil, e
	}
	rootRef, e := d.writeFile(ctx, raw, r.WorkspaceID, r.DraftID, resultRevision, c.ContentExpiry, now)
	if e != nil {
		return nil, e
	}
	updated := current.UpdatedAt
	if r.Kind == drafts.Save {
		updated = now
	}
	if !current.Exists {
		_, e = tx.Exec(ctx, `INSERT INTO hp_processing.drafts(workspace_id,draft_id,revision,technical_config,prose_ref,prose_sha256,prose_expires_at,updated_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$7)`, r.WorkspaceID, r.DraftID, resultRevision, string(technical(c.Config)), rootRef.ID, rootRef.Digest, rootRef.ExpiresAt, updated)
	} else {
		_, e = tx.Exec(ctx, `UPDATE hp_processing.drafts SET revision=$3,technical_config=$4,prose_ref=$5,prose_sha256=$6,prose_expires_at=$7,updated_at=$8,expires_at=$7 WHERE workspace_id=$1 AND draft_id=$2`, r.WorkspaceID, r.DraftID, resultRevision, string(technical(c.Config)), rootRef.ID, rootRef.Digest, rootRef.ExpiresAt, updated)
	}
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO hp_processing.user_commands(workspace_id,request_id,actor_id,digest,aggregate_id,expected_revision,result_revision,disposition,recorded_at) VALUES($1,$2,$3,$4,$5,$6,$7,'APPLIED',$8)`, r.WorkspaceID, in.Command.RequestID, g.PrincipalID, in.Digest, r.DraftID, in.Command.ExpectedRevision, resultRevision, now)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return append([]byte(nil), c.Response...), nil
}
