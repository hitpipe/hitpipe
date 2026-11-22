package casedeletion

import (
	"context"
	"crypto/ed25519"
	"slices"
	"strings"
)

type SnapshotPayload struct {
	SchemaVersion int        `json:"schema_version"`
	Environment   string     `json:"environment"`
	PublisherID   string     `json:"publisher_id"`
	WorkspaceID   string     `json:"workspace_id"`
	Generation    uint64     `json:"generation"`
	Revision      uint64     `json:"revision"`
	Full          bool       `json:"full"`
	Decisions     []Decision `json:"decisions"`
}

type Snapshot struct {
	Payload   SnapshotPayload `json:"payload"`
	Signature []byte          `json:"signature"`
}

type SnapshotScope struct {
	Environment string
	PublisherID string
	WorkspaceID string
	Generation  uint64
}

func payloadFor(environment, publisher, workspace string, w Workspace) SnapshotPayload {
	out := SnapshotPayload{SchemaVersion: SchemaVersion, Environment: environment, PublisherID: publisher, WorkspaceID: workspace,
		Generation: w.Generation, Revision: w.Revision, Full: true, Decisions: make([]Decision, 0, len(w.Decisions))}
	for _, d := range w.Decisions {
		// Prior-generation receipts stay with the issuer and cannot attest
		// that the currently restored copy has already applied the decision.
		d.Receipts = append([]Receipt{}, slices.DeleteFunc(slices.Clone(d.Receipts), func(x Receipt) bool { return x.Generation != w.Generation })...)
		d.OwnerProof = nil
		out.Decisions = append(out.Decisions, d)
	}
	slices.SortFunc(out.Decisions, func(a, b Decision) int { return strings.Compare(a.DeletionID, b.DeletionID) })
	return out
}

func (r *Registry) Snapshot(ctx context.Context, workspace string, generation uint64) (Snapshot, error) {
	if ctx == nil || !ValidID(workspace) || !ValidVersion(generation) {
		return Snapshot{}, ErrInvalid
	}
	var out Snapshot
	err := r.readWorkspace(ctx, workspace, func(w Workspace) error {
		if w.Generation != generation {
			return ErrConflict
		}
		out.Payload = payloadFor(r.cfg.Environment, r.cfg.PublisherID, workspace, w)
		wire, err := signBytes("snapshot-v1", out.Payload)
		if err != nil {
			return err
		}
		out.Signature = ed25519.Sign(r.cfg.SigningKey, wire)
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return out, nil
}

// VerifySnapshot checks origin, completeness and exact declared scope. A
// signature alone does not prove freshness: use WithCurrent before opening.
func VerifySnapshot(snapshot Snapshot, key ed25519.PublicKey, expected SnapshotScope) error {
	p := snapshot.Payload
	if len(key) != ed25519.PublicKeySize || p.SchemaVersion != SchemaVersion ||
		!p.Full || p.Decisions == nil || len(p.Decisions) > MaxDecisions ||
		!ValidEnvironment(p.Environment) || !ValidID(p.PublisherID) || !ValidID(p.WorkspaceID) ||
		!ValidVersion(p.Generation) || !ValidVersion(p.Revision) ||
		p.Environment != expected.Environment || p.PublisherID != expected.PublisherID ||
		p.WorkspaceID != expected.WorkspaceID || p.Generation != expected.Generation {
		return ErrInvalid
	}
	previous := ""
	for _, d := range p.Decisions {
		if d.DeletionID <= previous || d.PublisherID != p.PublisherID || !d.valid(p.Environment, p.WorkspaceID, d.DeletionID, p.Revision, p.Generation, false) {
			return ErrInvalid
		}
		for _, rc := range d.Receipts {
			if rc.Generation != p.Generation {
				return ErrInvalid
			}
		}
		previous = d.DeletionID
	}
	wire, err := signBytes("snapshot-v1", p)
	if err != nil || !ed25519.Verify(key, wire, snapshot.Signature) {
		return ErrUnauthorized
	}
	return nil
}

// WithCurrent is the local maintenance barrier for a trusted restore
// supervisor. It takes the supervisor's provisioned workspace/generation;
// those values are not inferred from a backup, receipt or signed snapshot.
// open must synchronously perform the local CLOSED -> OPEN transition while
// the store's read lock is held. It must not call this registry recursively.
func (r *Registry) WithCurrent(ctx context.Context, workspace string, generation uint64, snapshot Snapshot, open func(context.Context) error) error {
	if ctx == nil || !ValidID(workspace) || !ValidVersion(generation) || open == nil {
		return ErrInvalid
	}
	expected := SnapshotScope{Environment: r.cfg.Environment, PublisherID: r.cfg.PublisherID, WorkspaceID: workspace, Generation: generation}
	if err := VerifySnapshot(snapshot, r.public, expected); err != nil {
		return err
	}
	return r.readWorkspace(ctx, workspace, func(w Workspace) error {
		live := payloadFor(r.cfg.Environment, r.cfg.PublisherID, workspace, w)
		got, err := Digest(snapshot.Payload)
		if err != nil {
			return err
		}
		want, err := Digest(live)
		if err != nil {
			return err
		}
		if got != want {
			return ErrConflict
		}
		for _, d := range w.Decisions {
			if d.Status != Applied && d.Status != Cancelled {
				return ErrConflict
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return open(ctx)
	})
}
