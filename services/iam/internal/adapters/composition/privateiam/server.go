// Package privateiam wires the private IAM endpoints to one runtime gate,
// explicit service inventory, and the existing IAM authorization components.
package privateiam

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/authorization"
	runtimedb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authenticatesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
	"github.com/jackc/pgx/v5/pgxpool"
)

const startBudget = 2 * time.Second

type Server struct {
	local runtimeaccess.Registration
	peers map[runtimeaccess.Peer]runtimeaccess.Registration
	store *runtimedb.Store
	gate  *startgate.Gate
	scope *authorization.Reader
	check authorizeaction.Service
	start sync.Mutex
}

// New takes the approved roster and the current boot's complete identity from
// trusted provisioning. Each allowed leaf represents one exact incarnation.
func New(pool *pgxpool.Pool, clock authenticatesession.Clock, owner authorizeaction.ResourceAuthority, local runtimeaccess.Registration, bindings []runtimeaccess.Binding) (*Server, error) {
	if !local.Valid() || local.Service != "IAM" || len(bindings) == 0 {
		return nil, runtimeaccess.ErrInvalid
	}
	local = local.Normalized()
	allowed := make([]runtimeaccess.Registration, 0, len(bindings))
	peers := make(map[runtimeaccess.Peer]runtimeaccess.Registration, len(bindings))
	fingerprints := make(map[string]struct{}, len(bindings))
	var hasLocal bool
	for _, binding := range bindings {
		if !binding.Valid() || !runtimeaccess.ValidDNSName(binding.DNSName) || net.ParseIP(binding.DNSName) != nil {
			return nil, runtimeaccess.ErrInvalid
		}
		if _, exists := fingerprints[binding.CertificateFingerprint]; exists {
			return nil, runtimeaccess.ErrInvalid
		}
		fingerprints[binding.CertificateFingerprint] = struct{}{}
		reg := binding.Normalized()
		allowed = append(allowed, reg)
		peers[runtimeaccess.Peer{DNSName: strings.ToLower(binding.DNSName), CertificateFingerprint: reg.CertificateFingerprint}] = reg
		hasLocal = hasLocal || reg == local
	}
	if !hasLocal {
		return nil, runtimeaccess.ErrInvalid
	}
	store, err := runtimedb.New(pool, allowed)
	if err != nil {
		return nil, err
	}
	scope, err := authorization.New(pool)
	if err != nil {
		return nil, err
	}
	lookup, err := sessionlookup.New(pool)
	if err != nil {
		return nil, err
	}
	check, err := authorizeaction.New(lookup, authorizeaction.ReferenceFunc(sessionlookup.Reference), clock, admissionPorts{}, scope, owner, admissionPorts{})
	if err != nil {
		return nil, err
	}
	gate, err := startgate.New(startgate.Identity{RuntimeID: local.RuntimeID, BootID: local.BootID, Generation: uint64(local.Generation)})
	if err != nil {
		return nil, err
	}
	return &Server{local: local, peers: peers, store: store, gate: gate, scope: scope, check: check}, nil
}

// Start only opens the process after applying the complete, fresh snapshot for
// its own boot and confirming that exact snapshot in the IAM database.
func (s *Server) Start(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	s.start.Lock()
	defer s.start.Unlock()
	state, err := s.gate.State(ctx)
	if err != nil {
		return gateError(err)
	}
	if state == startgate.Open {
		return nil
	}

	pending, err := s.store.Register(ctx, s.local)
	if err != nil {
		return err
	}
	if !pending.Complete || pending.Gate != string(startgate.Syncing) {
		return runtimeaccess.ErrUnavailable
	}
	if err := s.apply(ctx, pending); err != nil {
		return err
	}
	opened, err := s.store.OpenLocal(ctx, s.local, pending)
	if err != nil {
		return err
	}
	if !opened.Complete || opened.Gate != string(startgate.Open) {
		return runtimeaccess.ErrUnavailable
	}
	return s.apply(ctx, opened)
}

// State observes local gate state. An OPEN observation is never itself a user
// permit or proof of remote fencing.
func (s *Server) State(ctx context.Context) (startgate.State, error) {
	return s.gate.State(ctx)
}

func (s *Server) Register(ctx context.Context, peer runtimeaccess.Peer, input runtimeaccess.Registration) (runtimeaccess.Snapshot, error) {
	if !input.Valid() {
		return runtimeaccess.Snapshot{}, runtimeaccess.ErrInvalid
	}
	bound, ok := s.resolve(peer)
	if !ok || input.Normalized() != bound {
		return runtimeaccess.Snapshot{}, runtimeaccess.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	if bound == s.local {
		state, err := s.gate.State(ctx)
		if err != nil {
			return runtimeaccess.Snapshot{}, gateError(err)
		}
		if state == startgate.Open {
			return s.activeSnapshot(ctx)
		}
	}
	return s.store.Register(ctx, bound)
}

func (s *Server) resolve(peer runtimeaccess.Peer) (runtimeaccess.Registration, bool) {
	peer.DNSName = strings.ToLower(peer.DNSName)
	registration, ok := s.peers[peer]
	return registration, ok
}

func (s *Server) apply(ctx context.Context, snapshot runtimeaccess.Snapshot) error {
	if snapshot.Generation < 1 || snapshot.Revision < 1 {
		return runtimeaccess.ErrUnavailable
	}
	view := startgate.RuntimeSnapshot{
		RuntimeID: snapshot.RuntimeID, BootID: snapshot.BootID, Generation: uint64(snapshot.Generation),
		SnapshotRevision: uint64(snapshot.Revision), Gate: startgate.State(snapshot.Gate),
		Denials: make([]startgate.Denial, 0, len(snapshot.Denials)),
	}
	for _, denial := range snapshot.Denials {
		if denial.Epoch < 1 {
			return runtimeaccess.ErrUnavailable
		}
		view.Denials = append(view.Denials, startgate.Denial{
			Subject: startgate.Subject{WorkspaceID: denial.WorkspaceID, Scope: startgate.Scope(denial.Scope), SubjectID: denial.SubjectID},
			Epoch:   uint64(denial.Epoch), RevocationID: denial.RevocationID, AcceptedAt: denial.AcceptedAt, EffectiveAt: denial.EffectiveAt,
		})
	}
	return gateError(s.gate.ApplySnapshot(ctx, view))
}

func (s *Server) activeSnapshot(ctx context.Context) (runtimeaccess.Snapshot, error) {
	snapshot, err := s.store.Read(ctx, s.local)
	if err != nil {
		return runtimeaccess.Snapshot{}, err
	}
	if !snapshot.Complete || snapshot.Gate != string(startgate.Open) {
		return runtimeaccess.Snapshot{}, runtimeaccess.ErrUnavailable
	}
	return snapshot, nil
}

func (s *Server) refreshGate(ctx context.Context) (runtimeaccess.Snapshot, error) {
	// A concurrent request may apply a higher durable revision first. Read the
	// current source again once in that case instead of applying stale state.
	for range 2 {
		snapshot, err := s.activeSnapshot(ctx)
		if err != nil {
			return runtimeaccess.Snapshot{}, err
		}
		err = s.apply(ctx, snapshot)
		if !errors.Is(err, runtimeaccess.ErrStale) {
			return snapshot, err
		}
	}
	return runtimeaccess.Snapshot{}, runtimeaccess.ErrStale
}

// Authorize reads IAM policy and starts the response under the same gate. The
// first section consults IAM's own policy in the workspace scope to discover
// the trusted session and optional binding subjects, with no response side
// effect. The response section checks those exact subjects in the gate and
// re-runs the current authorization before starting the actual write.
func (s *Server) Authorize(ctx context.Context, peer runtimeaccess.Peer, opaqueSession string, request authorizeaction.Request, emit func(context.Context, authorizeaction.Authorization) error) error {
	caller, ok := s.resolve(peer)
	if !ok || !strings.EqualFold(request.RuntimeID, caller.RuntimeID) {
		return runtimeaccess.ErrDenied
	}
	if emit == nil || !runtimeaccess.ValidUUID(request.WorkspaceID) {
		return runtimeaccess.ErrInvalid
	}

	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	state, err := s.gate.State(ctx)
	if err != nil || state != startgate.Open {
		return runtimeaccess.ErrUnavailable
	}
	snapshot, err := s.refreshGate(ctx)
	if err != nil {
		return err
	}
	if caller != s.local {
		remote, err := s.store.Read(ctx, caller)
		if err != nil {
			return err
		}
		if !remote.Complete || remote.Gate != string(startgate.Open) {
			return runtimeaccess.ErrDenied
		}
	}
	workspace := strings.ToLower(request.WorkspaceID)
	if unknownMembershipDeny(snapshot.Denials, workspace) {
		return runtimeaccess.ErrDenied
	}

	epoch, found, err := s.scope.WorkspaceEpoch(ctx, workspace)
	if err != nil {
		return runtimeaccess.ErrUnavailable
	}
	if !found {
		return runtimeaccess.ErrDenied
	}
	workspaceAuthority := authority(startgate.Workspace, workspace, workspace, epoch)
	var first authorizeaction.ScopeEpochs
	var firstRole string
	err = s.gate.Start(ctx, startgate.StartRequest{Authorities: []startgate.Authority{workspaceAuthority}}, func(work context.Context) error {
		scope := policyWork{caller: caller.Caller(), epoch: epoch}
		result, err := s.check.Execute(scope.withContext(work), opaqueSession, request)
		first, firstRole = scope.facts, result.Role
		return err
	})
	if err != nil {
		return gateError(err)
	}

	authorities := []startgate.Authority{
		workspaceAuthority,
		authority(startgate.Session, workspace, first.SessionID, first.SessionEpoch),
	}
	if first.BindingGrantID != "" {
		authorities = append(authorities, authority(startgate.Connection, workspace, first.BindingGrantID, first.BindingEpoch))
	}
	err = s.gate.Start(ctx, startgate.StartRequest{Authorities: authorities}, func(work context.Context) error {
		scope := policyWork{caller: caller.Caller(), epoch: epoch, expected: &first}
		result, err := s.check.Execute(scope.withContext(work), opaqueSession, request)
		if err != nil {
			return err
		}
		if result.Role != firstRole {
			return runtimeaccess.ErrStale
		}
		latest, err := s.activeSnapshot(work)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(latest.Denials, snapshot.Denials) {
			return runtimeaccess.ErrStale
		}
		latestEpoch, found, err := s.scope.WorkspaceEpoch(work, workspace)
		if err != nil || work.Err() != nil {
			return runtimeaccess.ErrUnavailable
		}
		if !found || latestEpoch != epoch {
			return runtimeaccess.ErrStale
		}
		return emit(work, result)
	})
	return gateError(err)
}

func authority(scope startgate.Scope, workspace, subject string, epoch int64) startgate.Authority {
	return startgate.Authority{Subject: startgate.Subject{WorkspaceID: &workspace, Scope: scope, SubjectID: subject}, Epoch: uint64(epoch), Allowed: true}
}

func unknownMembershipDeny(denials []runtimeaccess.Denial, workspace string) bool {
	for _, denial := range denials {
		if denial.Scope == string(startgate.Membership) && (denial.WorkspaceID == nil || *denial.WorkspaceID == workspace) {
			return true
		}
	}
	return false
}

func gateError(err error) error {
	switch {
	case errors.Is(err, startgate.ErrDenied), errors.Is(err, authorizeaction.ErrDenied):
		return runtimeaccess.ErrDenied
	case errors.Is(err, startgate.ErrStaleSnapshot), errors.Is(err, startgate.ErrConflict):
		return runtimeaccess.ErrStale
	case errors.Is(err, startgate.ErrClosed), errors.Is(err, startgate.ErrIdentity), errors.Is(err, startgate.ErrInvalid),
		errors.Is(err, startgate.ErrUninitialized), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return runtimeaccess.ErrUnavailable
	default:
		return err
	}
}
