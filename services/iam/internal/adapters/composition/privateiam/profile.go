package privateiam

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/content/profilefiles"
	transport "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/privateiam"
	profilehttp "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/profile"
	profiledb "github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/profile"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionlookup"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/authorizeaction"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/getprofile"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/iam/internal/domain/sessionpolicy"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

// Profile mounts the accepted owner handler with the same pool and start gate
// as authorization. The returned closer owns only the profile content reader.
func (s *Server) Profile(root string) (http.Handler, io.Closer, error) {
	files, err := profilefiles.New(root)
	if err != nil {
		return nil, nil, err
	}
	handler, err := s.profile(files)
	if err != nil {
		_ = files.Close()
		return nil, nil, err
	}
	return handler, files, nil
}

func (s *Server) profile(files *profilefiles.Reader) (http.Handler, error) {
	lookup, err := sessionlookup.New(s.pool)
	if err != nil {
		return nil, err
	}
	reader, err := profiledb.New(s.pool)
	if err != nil {
		return nil, err
	}
	service, err := getprofile.New(lookup, authorizeaction.ReferenceFunc(sessionlookup.Reference), s.clock, reader, files, profileGate{s, reader})
	if err != nil {
		return nil, err
	}
	handlers := make(map[runtimeaccess.Peer]http.Handler)
	for peer, registration := range s.peers {
		if registration.Service != "BFF" {
			continue
		}
		handler, err := profilehttp.New(service, profilehttp.Config{BFFCaller: registration.Caller(), BFFDNSName: peer.DNSName})
		if err != nil {
			return nil, err
		}
		handlers[peer] = handler
	}
	if len(handlers) == 0 {
		return nil, getprofile.ErrConfiguration
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if peer, err := transport.TransportPeer(r.TLS); err == nil {
			if handler := handlers[peer]; handler != nil {
				handler.ServeHTTP(w, r) // accepted handler validates the actual chain and identity
				return
			}
		}
		http.NotFound(w, r)
	}), nil
}

type profileGate struct {
	server *Server
	reader *profiledb.Reader
}

func (g profileGate) Within(ctx context.Context, binding getprofile.Binding, use func(context.Context, getprofile.Admission) error) error {
	if binding.Caller.Service != "BFF" || use == nil {
		return getprofile.ErrForbidden
	}
	var caller runtimeaccess.Registration
	for _, registration := range g.server.peers {
		if registration.Caller() == binding.Caller {
			caller = registration
			break
		}
	}
	if !caller.Valid() {
		return getprofile.ErrForbidden
	}
	snapshot, err := g.server.currentGate(ctx)
	if err != nil {
		return getprofile.ErrUnavailable
	}
	remote, err := g.server.store.Read(ctx, caller)
	if err != nil {
		return getprofile.ErrUnavailable
	}
	if !remote.Complete || remote.Gate != string(startgate.Open) {
		return getprofile.ErrForbidden
	}
	subject, err := g.reader.SessionSubject(ctx, binding.Reference)
	if err != nil {
		return err
	}
	if sessionDenied(snapshot.Denials, subject.ID) {
		return getprofile.ErrUnauthenticated
	}
	now, err := g.server.clock.Now(ctx)
	if err != nil || !now.Usable() {
		return getprofile.ErrUnavailable
	}
	if !sessionpolicy.Evaluate(subject.Policy, now).IsValid() {
		return getprofile.ErrUnauthenticated
	}
	ctx, cancel := context.WithTimeout(ctx, subject.Policy.IdleExpiresAt.Sub(now.At))
	defer cancel()
	err = g.server.gate.Start(ctx, globalSession(subject.ID, subject.Epoch), func(work context.Context) error {
		permit := &profileAdmission{server: g.server, reader: g.reader, binding: binding, subject: subject}
		defer func() {
			if permit.release != nil {
				permit.release()
			}
		}()
		return use(work, permit)
	})
	if errors.Is(err, startgate.ErrDenied) {
		return getprofile.ErrUnauthenticated
	}
	return err
}

// A durable row from an old process cannot open a freshly closed local gate.
func (s *Server) currentGate(ctx context.Context) (runtimeaccess.Snapshot, error) {
	state, err := s.State(ctx)
	if err != nil || state != startgate.Open {
		return runtimeaccess.Snapshot{}, runtimeaccess.ErrUnavailable
	}
	return s.refreshGate(ctx)
}

func globalSession(id string, epoch int64) startgate.StartRequest {
	return startgate.StartRequest{Authorities: []startgate.Authority{{
		Subject: startgate.Subject{Scope: startgate.Session, SubjectID: id},
		Epoch:   uint64(epoch), Allowed: true,
	}}}
}

func sessionDenied(denials []runtimeaccess.Denial, id string) bool {
	for _, denial := range denials {
		if denial.Scope == string(startgate.Session) && denial.SubjectID == id {
			return true
		}
	}
	return false
}

type profileAdmission struct {
	server  *Server
	reader  *profiledb.Reader
	binding getprofile.Binding
	subject authorizeaction.Session
	release func()
}

func (p *profileAdmission) Binding() getprofile.Binding { return p.binding }

func (p *profileAdmission) Confirm(ctx context.Context, facts getprofile.Facts) error {
	if p.release != nil {
		return getprofile.ErrUnavailable
	}
	release, err := p.reader.HoldCurrent(ctx, p.server.local.Caller(), p.binding, p.subject, facts)
	if err != nil {
		return getprofile.ErrUnavailable
	}
	p.release = release
	return nil
}
