package casedeletion

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ApplicationAuthority verifies an attestation of durable application under
// the exact generation and scope. Receiving an authenticated HTTP POST alone
// is not proof that the data changes persisted.
type ApplicationAuthority interface {
	WithApplied(context.Context, Receipt, func(context.Context) error) error
}

type Config struct {
	Environment      string
	PublisherID      string
	ProcessingPeerID string
	SigningKey       ed25519.PrivateKey
	Clock            func() time.Time
}

type Registry struct {
	cfg          Config
	store        Store
	owner        OwnerAuthority
	targets      TargetAuthority
	applications ApplicationAuthority
	public       ed25519.PublicKey
}

func New(cfg Config, store Store, owner OwnerAuthority, targets TargetAuthority, applications ApplicationAuthority) (*Registry, error) {
	if !ValidEnvironment(cfg.Environment) || !ValidID(cfg.PublisherID) ||
		(strings.TrimSpace(cfg.ProcessingPeerID) != cfg.ProcessingPeerID || cfg.ProcessingPeerID == "" || len(cfg.ProcessingPeerID) > 128) ||
		len(cfg.SigningKey) != ed25519.PrivateKeySize || store == nil || owner == nil || targets == nil || applications == nil {
		return nil, ErrInvalid
	}
	cfg.SigningKey = slices.Clone(cfg.SigningKey)
	if !slices.Equal(cfg.SigningKey, ed25519.NewKeyFromSeed(cfg.SigningKey.Seed())) {
		return nil, ErrInvalid
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &Registry{cfg: cfg, store: store, owner: owner, targets: targets, applications: applications, public: slices.Clone(cfg.SigningKey.Public().(ed25519.PublicKey))}, nil
}

type OfferPayload struct {
	Action      string    `json:"action"`
	Environment string    `json:"environment"`
	DeletionID  string    `json:"deletion_id"`
	PublisherID string    `json:"publisher_id"`
	PrincipalID string    `json:"principal_id"`
	Scope       Scope     `json:"scope"`
	IssuedAt    time.Time `json:"issued_at"`
}
type Offer struct {
	Payload   OfferPayload `json:"payload"`
	Signature []byte       `json:"signature"`
}

// ConfirmationCode binds the user's explicit acknowledgement to the entire
// displayed offer, including id, environment and the exact version list. It is
// not a credential; the owner must still present a valid session to Confirm.
func (o Offer) ConfirmationCode() (string, error) { return Digest(o) }

func signBytes(label string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrInvalid
	}
	return append([]byte("HITPIPE-CASE-DELETION/"+label+"\x00"), raw...), nil
}

func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", ErrUnavailable
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func (r *Registry) now() (time.Time, error) {
	now := normalizedTime(r.cfg.Clock())
	if !validTime(now) {
		return time.Time{}, ErrUnavailable
	}
	return now, nil
}

func (r *Registry) validState(s State) error {
	if err := s.Validate(); err != nil || s.Environment != r.cfg.Environment {
		return ErrUnavailable
	}
	for _, w := range s.Workspaces {
		for _, d := range w.Decisions {
			if d.PublisherID != r.cfg.PublisherID || !r.validOwnerProof(d) {
				return ErrUnavailable
			}
		}
	}
	return nil
}

func (r *Registry) validOwnerProof(d Decision) bool {
	proof := d.OwnerProof
	if proof == nil {
		return false
	}
	offer := Offer{Payload: OfferPayload{Action: d.Action, Environment: r.cfg.Environment, DeletionID: d.DeletionID,
		PublisherID: d.PublisherID, PrincipalID: proof.PrincipalID, Scope: d.Scope, IssuedAt: proof.IssuedAt},
		Signature: proof.OfferSignature}
	ref, err := offer.ConfirmationCode()
	wire, wireErr := signBytes("offer-v1", offer.Payload)
	confirmation, consentErr := confirmationBytes(d)
	return err == nil && wireErr == nil && consentErr == nil && ref == d.ConfirmationRef &&
		ed25519.Verify(r.public, wire, proof.OfferSignature) && ed25519.Verify(r.public, confirmation, proof.ConfirmationSignature)
}

// A preview can be issued without consent. Sign this independent envelope
// only while successfully confirming under both the owner and target holds.
func confirmationBytes(d Decision) ([]byte, error) {
	return signBytes("confirmation-v1", struct {
		OfferRef    string    `json:"offer_ref"`
		ScopeDigest string    `json:"scope_digest"`
		ConfirmedAt time.Time `json:"confirmed_at"`
	}{d.ConfirmationRef, d.ScopeDigest, d.ConfirmedAt})
}

// ProvisionWorkspace is an explicit publisher maintenance operation, never
// invoked by the Processing transport and never inferred from a request.
func (r *Registry) ProvisionWorkspace(ctx context.Context, workspace string, generation uint64) error {
	if ctx == nil || !ValidID(workspace) || !ValidVersion(generation) {
		return ErrInvalid
	}
	return r.change(ctx, workspace, func(s *State, w *Workspace, exists bool) (bool, error) {
		if exists {
			if w.Generation != generation {
				return false, ErrConflict
			}
			return false, nil
		}
		if len(s.Workspaces) >= MaxWorkspaces {
			return false, ErrUnavailable
		}
		w.Generation = generation
		w.Decisions = map[string]Decision{}
		return true, nil
	})
}

func (r *Registry) readWorkspace(ctx context.Context, id string, read func(Workspace) error) error {
	return r.store.Read(ctx, func(s State) error {
		if err := r.validState(s); err != nil {
			return err
		}
		w, ok := s.Workspaces[id]
		if !ok {
			return ErrNotFound
		}
		return read(w)
	})
}

func (r *Registry) change(ctx context.Context, id string, action func(*State, *Workspace, bool) (bool, error)) error {
	return r.store.Change(ctx, func(s *State) error {
		if err := r.validState(*s); err != nil {
			return err
		}
		w, ok := s.Workspaces[id]
		changed, err := action(s, &w, ok)
		if err != nil || !changed {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if s.Revision == MaxSafeInteger || w.Revision == MaxSafeInteger {
			return ErrUnavailable
		}
		s.Revision++
		w.Revision++
		s.Workspaces[id] = w
		return r.validState(*s)
	})
}

// Prepare produces the preview shown to the current owner; it does not save a
// decision. The same case, version IDs and minimal-retention consequence must
// be shown together before accepting the owner's distinct confirmation action.
func (r *Registry) Prepare(ctx context.Context, session string, want Scope) (Offer, error) {
	if ctx == nil {
		return Offer{}, ErrInvalid
	}
	scope, err := normalizeScope(want)
	if err != nil {
		return Offer{}, err
	}
	var out Offer
	err = r.owner.WithCurrentOwner(ctx, OwnerCheck{Session: session, WorkspaceID: scope.WorkspaceID},
		func(ownerCtx context.Context, principal OwnerIdentity) error {
			if !ValidID(principal.PrincipalID) {
				return ErrUnauthorized
			}
			return r.targets.WithExactTargets(ownerCtx, scope, func(targetCtx context.Context) error {
				return r.readWorkspace(targetCtx, scope.WorkspaceID, func(_ Workspace) error {
					now, err := r.now()
					if err != nil {
						return err
					}
					id, err := newID()
					if err != nil {
						return err
					}
					out.Payload = OfferPayload{Action: Action, Environment: r.cfg.Environment, DeletionID: id,
						PublisherID: r.cfg.PublisherID, PrincipalID: principal.PrincipalID, Scope: scope, IssuedAt: now}
					wire, err := signBytes("offer-v1", out.Payload)
					if err != nil {
						return err
					}
					out.Signature = ed25519.Sign(r.cfg.SigningKey, wire)
					return nil
				})
			})
		})
	if err != nil {
		return Offer{}, err
	}
	return out, nil
}

func (r *Registry) Confirm(ctx context.Context, session string, offer Offer, explicitConfirmation string) (Decision, error) {
	p := offer.Payload
	if ctx == nil || p.Action != Action || p.Environment != r.cfg.Environment || p.PublisherID != r.cfg.PublisherID ||
		!ValidID(p.DeletionID) || !ValidID(p.PrincipalID) || !validTime(p.IssuedAt) || !ValidDigest(explicitConfirmation) {
		return Decision{}, ErrInvalid
	}
	scope, err := normalizeScope(p.Scope)
	if err != nil || !slices.Equal(scope.Targets, p.Scope.Targets) {
		return Decision{}, ErrInvalid
	}
	wire, err := signBytes("offer-v1", p)
	ref, refErr := offer.ConfirmationCode()
	if err != nil || refErr != nil || !ed25519.Verify(r.public, wire, offer.Signature) ||
		subtle.ConstantTimeCompare([]byte(ref), []byte(explicitConfirmation)) != 1 {
		return Decision{}, ErrUnauthorized
	}
	digest, err := scopeDigest(scope)
	if err != nil {
		return Decision{}, err
	}
	var result Decision
	err = r.owner.WithCurrentOwner(ctx, OwnerCheck{Session: session, WorkspaceID: scope.WorkspaceID},
		func(ownerCtx context.Context, principal OwnerIdentity) error {
			if principal.PrincipalID != p.PrincipalID {
				return ErrUnauthorized
			}
			// Read an exact already confirmed attempt before consulting a version
			// catalog: existing targets may since have been deleted.
			err := r.readWorkspace(ownerCtx, scope.WorkspaceID, func(w Workspace) error {
				saved, ok := w.Decisions[p.DeletionID]
				if !ok {
					return ErrNotFound
				}
				if saved.ConfirmationRef != ref || saved.ScopeDigest != digest {
					return ErrConflict
				}
				result = saved
				return nil
			})
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			return r.targets.WithExactTargets(ownerCtx, scope, func(targetCtx context.Context) error {
				return r.change(targetCtx, scope.WorkspaceID, func(s *State, w *Workspace, exists bool) (bool, error) {
					if !exists {
						return false, ErrNotFound
					}
					for _, other := range s.Workspaces {
						if saved, ok := other.Decisions[p.DeletionID]; ok {
							if saved.Scope.WorkspaceID != scope.WorkspaceID || saved.ConfirmationRef != ref || saved.ScopeDigest != digest {
								return false, ErrConflict
							}
							result = saved
							return false, nil
						}
					}
					now, err := r.now()
					if err != nil {
						return false, err
					}
					if now.Before(p.IssuedAt) {
						return false, ErrUnauthorized
					}
					result = Decision{DeletionID: p.DeletionID, Action: Action, PublisherID: p.PublisherID,
						Scope: scope, ScopeDigest: digest, ConfirmedAt: now, ConfirmationRef: ref,
						OwnerProof: &OwnerProof{PrincipalID: p.PrincipalID, IssuedAt: p.IssuedAt, OfferSignature: slices.Clone(offer.Signature)},
						Status:     Confirmed, Receipts: []Receipt{}}
					confirmation, err := confirmationBytes(result)
					if err != nil {
						return false, err
					}
					result.OwnerProof.ConfirmationSignature = ed25519.Sign(r.cfg.SigningKey, confirmation)
					w.Decisions[p.DeletionID] = result
					return true, nil
				})
			})
		})
	if err != nil {
		return Decision{}, err
	}
	return result, nil
}

func (r *Registry) Cancel(ctx context.Context, session, workspace, deletionID string) error {
	if ctx == nil || !ValidID(workspace) || !ValidID(deletionID) {
		return ErrInvalid
	}
	return r.owner.WithCurrentOwner(ctx, OwnerCheck{Session: session, WorkspaceID: workspace}, func(ownerCtx context.Context, owner OwnerIdentity) error {
		if !ValidID(owner.PrincipalID) {
			return ErrUnauthorized
		}
		return r.change(ownerCtx, workspace, func(_ *State, w *Workspace, exists bool) (bool, error) {
			d, ok := w.Decisions[deletionID]
			if !exists || !ok {
				return false, ErrNotFound
			}
			if d.Status == Cancelled {
				return false, nil
			}
			if d.Status != Confirmed {
				return false, ErrConflict
			}
			d.Status = Cancelled
			w.Decisions[deletionID] = d
			return true, nil
		})
	})
}

// StartApplying is a publisher operation. Processing cannot call it. The
// state is committed before a decision is eligible for destructive work.
func (r *Registry) StartApplying(ctx context.Context, workspace, deletionID string, generation uint64) (Decision, error) {
	if ctx == nil || !ValidID(workspace) || !ValidID(deletionID) || !ValidVersion(generation) {
		return Decision{}, ErrInvalid
	}
	var result Decision
	err := r.change(ctx, workspace, func(_ *State, w *Workspace, exists bool) (bool, error) {
		d, ok := w.Decisions[deletionID]
		if !exists || !ok {
			return false, ErrNotFound
		}
		if generation != w.Generation || d.Status == Cancelled {
			return false, ErrConflict
		}
		if d.Status == Applying || d.Status == Applied {
			result = d
			return false, nil
		}
		now, err := r.now()
		if err != nil {
			return false, err
		}
		d.Status, d.ApplyGeneration, d.ApplyRevision, d.ApplyingAt = Applying, generation, w.Revision+1, &now
		w.Decisions[deletionID] = d
		result = d
		return true, nil
	})
	if err != nil {
		return Decision{}, err
	}
	return result, nil
}

// AdvanceGeneration is called by the trusted restore supervisor before trying
// to reopen recovered storage. Previous applicable decisions are replayable
// under the new generation, retaining their previous minimal receipts.
func (r *Registry) AdvanceGeneration(ctx context.Context, workspace string, expected, next uint64) error {
	if ctx == nil || !ValidID(workspace) || !ValidVersion(expected) || !ValidVersion(next) || next != expected+1 {
		return ErrInvalid
	}
	return r.change(ctx, workspace, func(_ *State, w *Workspace, exists bool) (bool, error) {
		if !exists {
			return false, ErrNotFound
		}
		if w.Generation == next {
			return false, nil
		}
		if w.Generation != expected {
			return false, ErrConflict
		}
		now, err := r.now()
		if err != nil {
			return false, err
		}
		w.Generation = next
		for id, d := range w.Decisions {
			if d.Status == Applied || d.Status == Applying {
				d.Status, d.ApplyGeneration, d.ApplyRevision, d.ApplyingAt = Applying, next, w.Revision+1, &now
				w.Decisions[id] = d
			}
		}
		return true, nil
	})
}

func receiptMatches(r Receipt, w Workspace, d Decision) bool {
	return r.Generation == w.Generation && (d.Status == Applying || d.Status == Applied) &&
		d.DeletionID == r.DeletionID && d.ScopeDigest == r.ScopeDigest &&
		d.ApplyRevision == r.Revision && d.ApplyGeneration == r.Generation &&
		!r.AppliedAt.Before(*d.ApplyingAt)
}

func (r *Registry) AcceptReceipt(ctx context.Context, peerID string, receipt Receipt) error {
	if ctx == nil || receipt.Validate() != nil || receipt.Environment != r.cfg.Environment {
		return ErrInvalid
	}
	if peerID != r.cfg.ProcessingPeerID {
		return ErrUnauthorized
	}
	receipt.AppliedAt = normalizedTime(receipt.AppliedAt)
	now, err := r.now()
	if err != nil {
		return err
	}
	if receipt.AppliedAt.After(now) {
		return ErrConflict
	}
	err = r.readWorkspace(ctx, receipt.WorkspaceID, func(w Workspace) error {
		d, ok := w.Decisions[receipt.DeletionID]
		if !ok {
			return ErrNotFound
		}
		if !receiptMatches(receipt, w, d) {
			return ErrConflict
		}
		if d.Status == Applied {
			if d.Receipts[len(d.Receipts)-1].equal(receipt) {
				return nil
			}
			return ErrConflict
		}
		return errApplyProof
	})
	if err != errApplyProof {
		return err
	}
	return r.applications.WithApplied(ctx, receipt, func(proofCtx context.Context) error {
		return r.change(proofCtx, receipt.WorkspaceID, func(_ *State, w *Workspace, exists bool) (bool, error) {
			d, ok := w.Decisions[receipt.DeletionID]
			if !exists || !ok {
				return false, ErrNotFound
			}
			if !receiptMatches(receipt, *w, d) {
				return false, ErrConflict
			}
			if d.Status == Applied {
				if d.Receipts[len(d.Receipts)-1].equal(receipt) {
					return false, nil
				}
				return false, ErrConflict
			}
			d.Status = Applied
			d.Receipts = append(d.Receipts, receipt)
			w.Decisions[receipt.DeletionID] = d
			return true, nil
		})
	})
}

var errApplyProof = errors.New("case deletion: durable application proof required")
