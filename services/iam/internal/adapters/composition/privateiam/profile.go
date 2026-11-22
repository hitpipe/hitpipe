package privateiam

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"

	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	readHTTP "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/profile"
	writeHTTP "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/profileupdate"
	readDB "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/profile"
	writeDB "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	app "github.com/hitpipe/hitpipe/services/iam/internal/application/profileupdate"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

// ListenProfile adds protected GET and PATCH to the private IAM listener using
// the same runtime gate and database. Without an explicit approved command
// authority PATCH cannot write. GET uses the accepted HIT-85 implementation.
func (s *Server) ListenProfile(ctx context.Context, address string, certificate tls.Certificate, clientCAs *x509.CertPool,
	content app.Content, commands app.CommandAuthority) error {
	store, err := writeDB.New(s.pool, s.clock)
	if err != nil {
		return err
	}
	gate := profileGate{s}
	update, err := app.New(store, content, gate, commands, sessionlookup.Reference)
	if err != nil {
		return err
	}
	get, err := s.profileReader(profileReadGate{gate, store}, content)
	if err != nil {
		return err
	}
	readers := make(map[runtimeaccess.Peer]http.Handler)
	var bff []runtimeaccess.Peer
	var fallback http.Handler
	for peer, registration := range s.peers {
		if registration.Service != "BFF" {
			continue
		}
		h, err := readHTTP.New(get, readHTTP.Config{BFFCaller: registration.Caller(), BFFDNSName: peer.DNSName})
		if err != nil {
			return err
		}
		bff = append(bff, peer)
		readers[peer], fallback = h, h
	}
	patch, err := writeHTTP.New(update, bff)
	if err != nil {
		return err
	}
	existing, err := transport.New(s)
	if err != nil {
		return err
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != writeHTTP.Path {
			existing.ServeHTTP(w, r)
			return
		}
		if r.Method == http.MethodPatch {
			patch.ServeHTTP(w, r)
			return
		}
		peer, err := transport.TransportPeer(r.TLS)
		if h := readers[peer]; err == nil && h != nil {
			h.ServeHTTP(w, r)
			return
		}
		// The original verified GET handler rejects a peer outside the BFF
		// set while preserving its accepted response and method handling.
		fallback.ServeHTTP(w, r)
	})
	return s.listen(ctx, address, certificate, clientCAs, handler)
}

func (s *Server) profileReader(gate getprofile.Gate, content getprofile.Content) (getprofile.Service, error) {
	lookup, err := sessionlookup.New(s.pool)
	if err != nil {
		return getprofile.Service{}, err
	}
	reader, err := readDB.New(s.pool)
	if err != nil {
		return getprofile.Service{}, err
	}
	return getprofile.New(lookup, authorizeaction.ReferenceFunc(sessionlookup.Reference), s.clock, reader, content, gate)
}

type profileGate struct{ server *Server }

func (g profileGate) Local() runtimeaccess.Registration { return g.server.local }

func (g profileGate) Caller(ctx context.Context, peer runtimeaccess.Peer) (runtimeaccess.Registration, error) {
	caller, found := g.server.resolve(peer)
	if !found || caller.Service != "BFF" {
		return runtimeaccess.Registration{}, app.ErrDenied
	}
	snapshot, err := g.server.store.Read(ctx, caller)
	if err != nil {
		return runtimeaccess.Registration{}, profileGateError(err)
	}
	if !snapshot.Complete || snapshot.Gate != string(startgate.Open) {
		return runtimeaccess.Registration{}, app.ErrDenied
	}
	return caller, nil
}

func (g profileGate) Within(ctx context.Context, peer runtimeaccess.Peer, who app.Identity, start func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	caller, err := g.Caller(ctx, peer)
	if err != nil {
		return err
	}
	if caller != who.Caller || g.Local() != who.Local {
		return app.ErrDenied
	}
	if _, err := g.server.refreshGate(ctx); err != nil {
		return profileGateError(err)
	}
	req := startgate.StartRequest{Authorities: []startgate.Authority{{
		Subject: startgate.Subject{Scope: startgate.Session, SubjectID: who.SessionID},
		Epoch:   uint64(who.SessionEpoch), Allowed: true,
	}}}
	return profileGateError(g.server.gate.Start(ctx, req, start))
}

func profileGateError(err error) error {
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

type profileReadGate struct {
	profileGate
	store app.Store
}

func (g profileReadGate) Within(ctx context.Context, binding getprofile.Binding, use func(context.Context, getprofile.Admission) error) error {
	for peer, provisioned := range g.server.peers {
		if provisioned.Caller() != binding.Caller || provisioned.Service != "BFF" {
			continue
		}
		caller, err := g.Caller(ctx, peer)
		if err != nil {
			return profileReadError(err)
		}
		who, err := g.store.Identity(ctx, binding.Reference, caller, g.Local())
		if err != nil {
			return profileReadError(err)
		}
		err = g.profileGate.Within(ctx, peer, who, func(work context.Context) error {
			return use(work, profileAdmission{binding: binding, who: who, store: g.store})
		})
		return profileReadError(err)
	}
	return getprofile.ErrForbidden
}

func profileReadError(err error) error {
	switch {
	case errors.Is(err, app.ErrUnauthenticated):
		return getprofile.ErrUnauthenticated
	case errors.Is(err, app.ErrDenied):
		return getprofile.ErrForbidden
	case errors.Is(err, app.ErrNotFound), errors.Is(err, app.ErrUnavailable), errors.Is(err, app.ErrUncertain), errors.Is(err, app.ErrStale):
		return getprofile.ErrUnavailable
	default:
		return err
	}
}

type profileAdmission struct {
	binding getprofile.Binding
	who     app.Identity
	store   app.Store
}

func (a profileAdmission) Binding() getprofile.Binding { return a.binding }

func (a profileAdmission) Confirm(ctx context.Context, facts getprofile.Facts) error {
	if ctx.Err() != nil || facts.Caller != a.binding.Caller || facts.SessionID != a.who.SessionID ||
		facts.SessionEpoch != a.who.SessionEpoch || facts.PrincipalID != a.who.PrincipalID || len(facts.Revocations) != 0 {
		return getprofile.ErrUnavailable
	}
	return nil
}

func (a profileAdmission) Start(ctx context.Context, facts getprofile.Facts, start func(context.Context) error) error {
	return profileReadError(a.store.WithProfile(ctx, a.who, func(work context.Context, tx app.ProfileTx) error {
		if tx.Pointer() != (app.Pointer{Ref: facts.ProfileRef, Digest: facts.ProfileDigest}) {
			return getprofile.ErrUnavailable
		}
		return start(work)
	}))
}
