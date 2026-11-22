package workspaces

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
)

const maxItems = 50

type Service struct {
	store      Store
	files      Content
	gate       Gate
	candidates Candidates
	onboarding Onboarding
	reference  func(string) (string, error)
}

func absent(v any) bool {
	if v == nil {
		return true
	}
	switch x := reflect.ValueOf(v); x.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return x.IsNil()
	}
	return false
}

// New permits a missing route-specific input only as fail-closed configuration.
func New(store Store, files Content, gate Gate, candidates Candidates, onboarding Onboarding,
	reference func(string) (string, error)) (*Service, error) {
	if absent(store) || absent(files) || absent(gate) || reference == nil {
		return nil, ErrUnavailable
	}
	return &Service{store: store, files: files, gate: gate, candidates: candidates, onboarding: onboarding, reference: reference}, nil
}

func id(value string) bool {
	return value == strings.ToLower(value) && runtimeaccess.ValidUUID(value)
}
func goodName(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) > 0 && utf8.RuneCountInString(value) <= 120
}
func epoch(n int64) bool { return n > 0 && n <= authorizeaction.MaxWireInteger }

var code = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]{0,63}$`)

func (s *Service) authenticate(ctx context.Context, peer runtimeaccess.Peer, session string) (Identity, error) {
	if ctx == nil || ctx.Err() != nil {
		return Identity{}, ErrUnavailable
	}
	caller, err := s.gate.Caller(ctx, peer)
	if err != nil {
		return Identity{}, err
	}
	if !caller.Valid() || caller.Service != "BFF" {
		return Identity{}, ErrDenied
	}
	ref, err := s.reference(session)
	if err != nil || ref == "" {
		return Identity{}, ErrUnauthenticated
	}
	local := s.gate.Local()
	who, err := s.store.Identity(ctx, ref, caller, local)
	if err != nil {
		return Identity{}, err
	}
	if who.Caller != caller || who.Local != local || who.Reference != ref || !id(who.PrincipalID) ||
		!id(who.SessionID) || !epoch(who.SessionEpoch) || who.ValidUntil.IsZero() {
		return Identity{}, ErrUnavailable
	}
	return who, nil
}

func normalizeIDs(ids []string) ([]string, error) {
	out := slices.Clone(ids)
	slices.Sort(out)
	for _, key := range out {
		if !id(key) {
			return nil, ErrUnavailable
		}
	}
	return slices.Compact(out), nil
}

func orderRows(rows []Row, ids []string) ([]Row, error) {
	if len(rows) > len(ids) {
		return nil, ErrUnavailable
	}
	out := append([]Row{}, rows...)
	slices.SortFunc(out, func(a, b Row) int { return strings.Compare(a.WorkspaceID, b.WorkspaceID) })
	for i, row := range out {
		_, found := slices.BinarySearch(ids, row.WorkspaceID)
		if !found || (i > 0 && out[i-1].WorkspaceID == row.WorkspaceID) || !epoch(row.Revision) || !epoch(row.SecurityEpoch) || !id(row.NameRef) ||
			!epoch(row.MembershipRevision) ||
			!IsPublicLifecycle(row.Lifecycle) || (row.Role != "OWNER" && row.Role != "OPERATOR" && row.Role != "READER") {
			return nil, ErrUnavailable
		}
	}
	return out, nil
}

type nameObject struct {
	Schema      string `json:"schema"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
}

const nameSchema = "hitpipe.workspace.name.v1"

func (s *Service) project(ctx context.Context, row Row) (Workspace, error) {
	raw, err := s.files.Read(ctx, row.WorkspaceID, row.NameRef)
	if err != nil {
		return Workspace{}, ErrUnavailable
	}
	var data nameObject
	if json.Unmarshal([]byte(raw), &data) != nil || data.Schema != nameSchema || data.WorkspaceID != row.WorkspaceID || !goodName(data.Name) {
		return Workspace{}, ErrUnavailable
	}
	exact, err := json.Marshal(data)
	if err != nil || string(exact) != raw {
		return Workspace{}, ErrUnavailable
	}
	return Workspace{WorkspaceID: row.WorkspaceID, Name: data.Name, Role: row.Role, Lifecycle: row.Lifecycle,
		Revision: row.Revision, SecurityEpoch: row.SecurityEpoch}, nil
}

// List treats the candidate catalog as a locator. Before any response starts,
// re-read every candidate's authorization under the exact accepted gate.
func (s *Service) List(ctx context.Context, peer runtimeaccess.Peer, session string, start func(context.Context, []Workspace) error) error {
	if start == nil {
		return ErrInvalid
	}
	if absent(s.candidates) {
		return ErrUnavailable
	}
	who, err := s.authenticate(ctx, peer, session)
	if err != nil {
		return err
	}
	var ids []string
	var first []Row
	err = s.gate.Within(ctx, peer, who, nil, func(work context.Context) error {
		candidates, err := s.candidates.WorkspaceIDs(work, who.PrincipalID)
		if err != nil {
			return ErrUnavailable
		}
		ids, err = normalizeIDs(candidates)
		if err != nil {
			return err
		}
		return s.store.WithRows(work, who, ids, func(_ context.Context, read []Row) error {
			first, err = orderRows(read, ids)
			return err
		})
	})
	if err != nil {
		return err
	}
	return s.gate.Within(ctx, peer, who, first, func(work context.Context) error {
		return s.store.WithRows(work, who, ids, func(guard context.Context, current []Row) error {
			rows, err := orderRows(current, ids)
			if err != nil {
				return err
			}
			if !slices.Equal(rows, first) {
				return ErrStale
			}
			out := make([]Workspace, 0, min(maxItems, len(rows)))
			for _, row := range rows[:min(maxItems, len(rows))] {
				item, err := s.project(guard, row)
				if err != nil {
					return err
				}
				out = append(out, item)
			}
			if guard.Err() != nil {
				return ErrUnavailable
			}
			return start(guard, out)
		})
	})
}

func admitted(g Grant, who Identity, requestID string) bool {
	return g.PrincipalID == who.PrincipalID && g.RequestID == requestID && id(g.WorkspaceID) && g.Lifecycle == "ACTIVE" &&
		code.MatchString(g.AuditAction) && code.MatchString(g.AuditReason) && code.MatchString(g.MessageKind) &&
		!g.OwnerConfirmedAt.IsZero() && g.PayloadExpiresAt.After(g.OwnerConfirmedAt)
}

func digest(b []byte) string {
	hash := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func objectID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read fills the buffer or terminates the process.
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	v := hex.EncodeToString(b[:])
	return v[:8] + "-" + v[8:12] + "-" + v[12:16] + "-" + v[16:20] + "-" + v[20:]
}

func canonical(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	raw := jsontext.Value(body)
	if err := raw.Canonicalize(); err != nil {
		return nil, err
	}
	return raw, nil
}

func commandDigest(cmd Command) (string, error) {
	body, err := canonical(map[string]any{
		"command": map[string]string{"request_id": cmd.RequestID},
		"name":    cmd.Name,
	})
	if err != nil {
		return "", ErrInvalid
	}
	return digest(body), nil
}

func (s *Service) stage(ctx context.Context, change Change, displayName string) (Material, error) {
	files := Material{NameRef: objectID(), AuditID: objectID(), MessageID: objectID(), MessageRef: objectID()}
	name, err := json.Marshal(nameObject{nameSchema, change.WorkspaceID, displayName})
	if err != nil {
		return Material{}, ErrInvalid
	}
	// This is the complete local outbox payload. It carries no name or
	// credential. Downstream delivery and its consumer are not wired here.
	event, err := canonical(map[string]string{
		"workspace_id": change.WorkspaceID, "request_id": change.RequestID,
		"message_id": files.MessageID, "kind": change.MessageKind,
	})
	if err != nil {
		return Material{}, ErrInvalid
	}
	files.MessageDigest = digest(event)
	if s.files.Put(ctx, change.WorkspaceID, files.NameRef, string(name)) != nil ||
		s.files.Put(ctx, change.WorkspaceID, files.MessageRef, string(event)) != nil || ctx.Err() != nil {
		return Material{}, ErrUnavailable
	}
	return files, nil
}

// Create is qualified only for a current explicit onboarding grant with an
// exact real workspace and independently established ACTIVE lifecycle. It
// cannot invent a workspace selector or return a public ONBOARDING.
func (s *Service) Create(ctx context.Context, peer runtimeaccess.Peer, session, csrf string, cmd Command, start func(context.Context, Workspace) error) error {
	requestID := strings.ToLower(cmd.RequestID)
	if start == nil || !id(requestID) || !goodName(cmd.Name) || len(csrf) < 32 || len(csrf) > 128 {
		return ErrInvalid
	}
	// Creation has no existing aggregate for a compare-and-swap.
	if cmd.ExpectedRevision != nil {
		return ErrStale
	}
	if absent(s.onboarding) {
		return ErrUnavailable
	}
	who, err := s.authenticate(ctx, peer, session)
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(csrf))
	if subtle.ConstantTimeCompare(hash[:], who.CSRFHash[:]) != 1 {
		return ErrDenied
	}
	bodyDigest, err := commandDigest(cmd)
	if err != nil {
		return err
	}
	return s.onboarding.WithGrant(ctx, who.PrincipalID, requestID, func(admittedCtx context.Context, g Grant) error {
		if !admitted(g, who, requestID) {
			return ErrDenied
		}
		change := Change{WorkspaceID: g.WorkspaceID, RequestID: requestID, BodyDigest: bodyDigest,
			AuditAction: g.AuditAction, AuditReason: g.AuditReason, MessageKind: g.MessageKind, Lifecycle: g.Lifecycle,
			OwnerConfirmedAt: g.OwnerConfirmedAt, PayloadExpiresAt: g.PayloadExpiresAt}
		target := []Row{{WorkspaceID: g.WorkspaceID, SecurityEpoch: 1}}
		var committed Row
		err := s.gate.Within(admittedCtx, peer, who, target, func(work context.Context) error {
			var err error
			committed, err = s.store.Create(work, who, change, func(preparing context.Context) (Material, error) {
				return s.stage(preparing, change, cmd.Name)
			})
			return err
		})
		if err != nil {
			return err
		}
		if committed.WorkspaceID != change.WorkspaceID || !epoch(committed.SecurityEpoch) {
			return ErrUnavailable
		}
		// Recheck current owner access after commit and keep the grant current
		// through the response. Unknown commit outcomes never reach this step.
		return s.startCreationResponse(admittedCtx, peer, who, committed, start)
	})
}

func (s *Service) startCreationResponse(ctx context.Context, peer runtimeaccess.Peer, who Identity, committed Row,
	start func(context.Context, Workspace) error) error {
	return s.gate.Within(ctx, peer, who, []Row{committed}, func(work context.Context) error {
		ids := []string{committed.WorkspaceID}
		return s.store.WithRows(work, who, ids, func(guard context.Context, current []Row) error {
			rows, err := orderRows(current, ids)
			if err != nil {
				return err
			}
			if len(rows) != 1 || rows[0] != committed || rows[0].Role != "OWNER" || rows[0].Lifecycle != "ACTIVE" {
				return ErrStale
			}
			item, err := s.project(guard, rows[0])
			if err != nil {
				return err
			}
			if guard.Err() != nil {
				return ErrUnavailable
			}
			return start(guard, item)
		})
	})
}
