package privateiam

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"

	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	wire "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	workspaceDB "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/workspaces"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/workspaces"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

// ListenWorkspaces adds the owner methods to the same private HTTPS listener
// and runtime gate. The catalog and onboarding authority require explicit
// owner provisioning. A nil port leaves the corresponding method closed.
func (s *Server) ListenWorkspaces(ctx context.Context, address string, certificate tls.Certificate, clientCAs *x509.CertPool,
	files app.Content, candidates app.Candidates, onboarding app.Onboarding) error {
	store, err := workspaceDB.New(s.pool, s.clock)
	if err != nil {
		return err
	}
	service, err := app.New(store, files, workspaceGate{s}, candidates, onboarding, sessionlookup.Reference)
	if err != nil {
		return err
	}
	var bff []runtimeaccess.Peer
	for peer, registration := range s.peers {
		if registration.Service == "BFF" {
			bff = append(bff, peer)
		}
	}
	handler, err := wire.New(service, bff)
	if err != nil {
		return err
	}
	existing, err := transport.New(s)
	if err != nil {
		return err
	}
	// Exact routing preserves the previous paths and their handling, without
	// ServeMux redirects or normalization of a protected operation.
	return s.listen(ctx, address, certificate, clientCAs, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == wire.Path {
			handler.ServeHTTP(w, r)
			return
		}
		existing.ServeHTTP(w, r)
	}))
}

type workspaceGate struct{ server *Server }

func (g workspaceGate) Local() runtimeaccess.Registration { return g.server.local }

func (g workspaceGate) Caller(ctx context.Context, peer runtimeaccess.Peer) (runtimeaccess.Registration, error) {
	caller, bound := g.server.resolve(peer)
	if !bound || caller.Service != "BFF" {
		return runtimeaccess.Registration{}, app.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	snapshot, err := g.server.store.Read(ctx, caller)
	if err != nil {
		return runtimeaccess.Registration{}, workspaceGateError(err)
	}
	if !snapshot.Complete || snapshot.Gate != string(startgate.Open) {
		return runtimeaccess.Registration{}, app.ErrDenied
	}
	return caller, nil
}

func (g workspaceGate) Within(ctx context.Context, peer runtimeaccess.Peer, who app.Identity, rows []app.Row,
	start func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	caller, err := g.Caller(ctx, peer)
	if err != nil {
		return err
	}
	if caller != who.Caller || who.Local != g.Local() {
		return app.ErrDenied
	}
	snapshot, err := g.server.refreshGate(ctx)
	if err != nil {
		return workspaceGateError(err)
	}
	authorities := make([]startgate.Authority, 0, 1+2*len(rows))
	authorities = append(authorities, startgate.Authority{
		Subject: startgate.Subject{Scope: startgate.Session, SubjectID: who.SessionID},
		Epoch:   uint64(who.SessionEpoch), Allowed: true,
	})
	for _, row := range rows {
		// The pinned IAM table has a composite membership key and no
		// canonical revocation subject UUID. Keep the accepted denial rule.
		if unknownMembershipDeny(snapshot.Denials, row.WorkspaceID) {
			return app.ErrDenied
		}
		authorities = append(authorities,
			authority(startgate.Workspace, row.WorkspaceID, row.WorkspaceID, row.SecurityEpoch),
			authority(startgate.Session, row.WorkspaceID, who.SessionID, who.SessionEpoch))
	}
	// SQL rechecks these exact owners and holds relevant rows and the accepted
	// denial set through the protected operation inside this start section.
	return workspaceGateError(g.server.gate.Start(ctx, startgate.StartRequest{Authorities: authorities}, start))
}

func workspaceGateError(err error) error {
	err = gateError(err)
	switch {
	case errors.Is(err, runtimeaccess.ErrDenied):
		return app.ErrDenied
	case errors.Is(err, runtimeaccess.ErrStale):
		return app.ErrStale
	case errors.Is(err, runtimeaccess.ErrUnavailable), errors.Is(err, runtimeaccess.ErrInvalid):
		return app.ErrUnavailable
	default:
		return err
	}
}
