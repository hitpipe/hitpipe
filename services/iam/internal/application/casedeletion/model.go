// Package casedeletion issues internal owner-confirmed deletion records. It does
// not delete content and does not authorize the Processing storage adapter.
package casedeletion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	SchemaVersion  = 1
	Action         = "CASE_CONTENT_DELETE"
	MaxSafeInteger = 9007199254740991
	MaxWorkspaces  = 32
	MaxDecisions   = 4096
	MaxTargets     = 128
)

var (
	ErrInvalid      = errors.New("case deletion: invalid input")
	ErrUnauthorized = errors.New("case deletion: authority not established")
	ErrDenied       = ErrUnauthorized
	ErrConflict     = errors.New("case deletion: conflicting state")
	ErrUnavailable  = errors.New("case deletion: current source unavailable")
	ErrNotFound     = errors.New("case deletion: record not found")
)

type Status string

const (
	Confirmed Status = "CONFIRMED"
	Applying  Status = "APPLYING"
	Applied   Status = "APPLIED"
	Cancelled Status = "CANCELLED"
)

type Target struct {
	Class     string `json:"class"`
	VersionID string `json:"version_id"`
}

type Scope struct {
	WorkspaceID string   `json:"workspace_id"`
	CaseID      string   `json:"case_id"`
	Targets     []Target `json:"targets"`
}

type Receipt struct {
	Environment    string    `json:"environment"`
	WorkspaceID    string    `json:"workspace_id"`
	DeletionID     string    `json:"deletion_id"`
	ScopeDigest    string    `json:"scope_digest"`
	Generation     uint64    `json:"generation"`
	Revision       uint64    `json:"revision"`
	AppliedAt      time.Time `json:"applied_at"`
	ApplicationRef string    `json:"application_ref"`
}

// OwnerProof is retained by the issuer only. It lets an auditor reconstruct
// the signed offer and resolve ConfirmationRef without keeping the session.
type OwnerProof struct {
	PrincipalID           string    `json:"principal_id"`
	IssuedAt              time.Time `json:"issued_at"`
	OfferSignature        []byte    `json:"offer_signature"`
	ConfirmationSignature []byte    `json:"confirmation_signature"`
}

// Decision is the issuer-side minimal control record. ScopeDigest binds the
// immutable scope; ApplyRevision is the exact transition to APPLYING in the
// current generation, which may be older than the latest full snapshot.
type Decision struct {
	DeletionID      string      `json:"deletion_id"`
	Action          string      `json:"action"`
	PublisherID     string      `json:"publisher_id"`
	Scope           Scope       `json:"scope"`
	ScopeDigest     string      `json:"scope_digest"`
	ConfirmedAt     time.Time   `json:"confirmed_at"`
	ConfirmationRef string      `json:"confirmation_ref"`
	OwnerProof      *OwnerProof `json:"owner_proof,omitempty"`
	Status          Status      `json:"status"`
	ApplyGeneration uint64      `json:"apply_generation"`
	ApplyRevision   uint64      `json:"apply_revision"`
	ApplyingAt      *time.Time  `json:"applying_at"`
	Receipts        []Receipt   `json:"receipts"`
}

type Workspace struct {
	Generation uint64              `json:"generation"`
	Revision   uint64              `json:"revision"`
	Decisions  map[string]Decision `json:"decisions"`
}

type State struct {
	SchemaVersion int                  `json:"schema_version"`
	Environment   string               `json:"environment"`
	Revision      uint64               `json:"revision"`
	Workspaces    map[string]Workspace `json:"workspaces"`
}

// Store owns the shared read/exclusive write barrier. Read runs the callback
// while the current revision cannot change, including across issuer processes.
type Store interface {
	Read(context.Context, func(State) error) error
	Change(context.Context, func(*State) error) error
}

type OwnerCheck struct {
	Session     string
	WorkspaceID string
}

type OwnerIdentity struct{ PrincipalID string }

// OwnerAuthority holds current IAM authority through the synchronous commit.
// It must not get the user's session from a service identity or header claim.
type OwnerAuthority interface {
	WithCurrentOwner(context.Context, OwnerCheck, func(context.Context, OwnerIdentity) error) error
}

// TargetAuthority belongs to the owner of the case/version inventory. It must
// prove that the exact selected versions belong to the case, holding that
// proof through the synchronous commit. Arbitrary caller IDs are not proof.
type TargetAuthority interface {
	WithExactTargets(context.Context, Scope, func(context.Context) error) error
}

func NewState(environment string) (State, error) {
	if !ValidEnvironment(environment) {
		return State{}, ErrInvalid
	}
	return State{SchemaVersion: SchemaVersion, Environment: environment, Revision: 1, Workspaces: map[string]Workspace{}}, nil
}

var (
	idPattern  = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	envPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	pinPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

func ValidID(s string) bool {
	return idPattern.MatchString(s) && s != "00000000-0000-0000-0000-000000000000"
}

func ValidEnvironment(s string) bool { return envPattern.MatchString(s) }
func ValidDigest(s string) bool      { return pinPattern.MatchString(s) }
func ValidVersion(n uint64) bool     { return n > 0 && n <= MaxSafeInteger }

func validTime(t time.Time) bool {
	return !t.IsZero() && t.Year() >= 1 && t.Year() <= 9999
}

func normalizedTime(t time.Time) time.Time { return t.UTC().Truncate(time.Microsecond) }

func normalizeScope(s Scope) (Scope, error) {
	if !ValidID(s.WorkspaceID) || !ValidID(s.CaseID) || len(s.Targets) == 0 || len(s.Targets) > MaxTargets {
		return Scope{}, ErrInvalid
	}
	s.Targets = slices.Clone(s.Targets)
	slices.SortFunc(s.Targets, func(a, b Target) int {
		if a.Class != b.Class {
			return strings.Compare(a.Class, b.Class)
		}
		return strings.Compare(a.VersionID, b.VersionID)
	})
	seen := make(map[Target]bool, len(s.Targets))
	for _, t := range s.Targets {
		if (t.Class != "CASE_CONTEXT" && t.Class != "HISTORY") || !ValidID(t.VersionID) || seen[t] {
			return Scope{}, ErrInvalid
		}
		seen[t] = true
	}
	return s, nil
}

// Digest identifies a JSON projection with no content bodies or secrets.
func Digest(value any) (string, error) {
	wire, err := json.Marshal(value)
	if err != nil {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(wire)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func scopeDigest(s Scope) (string, error) {
	return Digest(struct {
		Action string `json:"action"`
		Scope  Scope  `json:"scope"`
	}{Action, s})
}

func (r Receipt) Validate() error {
	if !ValidEnvironment(r.Environment) || !ValidID(r.WorkspaceID) || !ValidID(r.DeletionID) ||
		!ValidDigest(r.ScopeDigest) || !ValidDigest(r.ApplicationRef) || !ValidVersion(r.Generation) ||
		!ValidVersion(r.Revision) || !validTime(r.AppliedAt) {
		return ErrInvalid
	}
	return nil
}

func (r Receipt) equal(b Receipt) bool {
	return r.Environment == b.Environment && r.WorkspaceID == b.WorkspaceID && r.DeletionID == b.DeletionID &&
		r.ScopeDigest == b.ScopeDigest && r.Generation == b.Generation && r.Revision == b.Revision &&
		r.AppliedAt.Equal(b.AppliedAt) && r.ApplicationRef == b.ApplicationRef
}

func (d Decision) valid(environment, ws, key string, scopeRevision, generation uint64, internal bool) bool {
	if internal != (d.OwnerProof != nil) {
		return false
	}
	if internal && (!ValidID(d.OwnerProof.PrincipalID) || !validTime(d.OwnerProof.IssuedAt) ||
		d.OwnerProof.IssuedAt.After(d.ConfirmedAt) || (len(d.OwnerProof.OfferSignature) != 64 || len(d.OwnerProof.ConfirmationSignature) != 64)) {
		return false
	}
	if !ValidID(d.DeletionID) || key != d.DeletionID || d.Action != Action || !ValidID(d.PublisherID) ||
		d.Scope.WorkspaceID != ws || !ValidDigest(d.ConfirmationRef) || !validTime(d.ConfirmedAt) || d.Receipts == nil {
		return false
	}
	norm, err := normalizeScope(d.Scope)
	digest, digestErr := scopeDigest(d.Scope)
	if err != nil || digestErr != nil || !slices.Equal(norm.Targets, d.Scope.Targets) || digest != d.ScopeDigest {
		return false
	}
	switch d.Status {
	case Confirmed, Cancelled:
		return d.ApplyGeneration == 0 && d.ApplyRevision == 0 && d.ApplyingAt == nil && len(d.Receipts) == 0
	case Applying, Applied:
	default:
		return false
	}
	if d.ApplyGeneration != generation || !ValidVersion(d.ApplyRevision) || d.ApplyRevision > scopeRevision ||
		d.ApplyingAt == nil || !validTime(*d.ApplyingAt) || d.ApplyingAt.Before(d.ConfirmedAt) || len(d.Receipts) > MaxDecisions {
		return false
	}
	var prior uint64
	for _, r := range d.Receipts {
		if r.Validate() != nil || r.Environment != environment || r.WorkspaceID != ws || r.DeletionID != key ||
			r.ScopeDigest != d.ScopeDigest || r.Revision > scopeRevision || r.Generation <= prior || r.Generation > generation ||
			r.AppliedAt.Before(d.ConfirmedAt) {
			return false
		}
		if r.Generation == generation && (r.Revision != d.ApplyRevision || r.AppliedAt.Before(*d.ApplyingAt)) {
			return false
		}
		prior = r.Generation
	}
	return (d.Status == Applied) == (prior == generation)
}

func (s State) Validate() error {
	if s.SchemaVersion != SchemaVersion || !ValidEnvironment(s.Environment) || !ValidVersion(s.Revision) ||
		s.Workspaces == nil || len(s.Workspaces) > MaxWorkspaces {
		return ErrInvalid
	}
	seen := make(map[string]bool)
	for workspace, w := range s.Workspaces {
		if !ValidID(workspace) || !ValidVersion(w.Generation) || !ValidVersion(w.Revision) ||
			w.Revision > s.Revision || w.Decisions == nil {
			return ErrInvalid
		}
		for key, d := range w.Decisions {
			if seen[key] || len(seen) == MaxDecisions || !d.valid(s.Environment, workspace, key, w.Revision, w.Generation, true) {
				return ErrInvalid
			}
			seen[key] = true
		}
	}
	return nil
}
