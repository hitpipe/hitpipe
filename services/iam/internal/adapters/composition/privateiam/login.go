package privateiam

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	httpauth "github.com/hitpipe/hitpipe/services/iam/internal/adapters/http"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/oidc"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/runtimeaccess"
	"github.com/hitpipe/hitpipe/services/platform/startgate"
)

// Login mounts the accepted browser endpoints. Issuance needs an independently
// provisioned authority as well as the same registered gate used by authorize.
func (s *Server) Login(config oidc.Config, authority issuesession.IssuanceGuard, policy issuesession.Policy, successPath string) (http.Handler, error) {
	value := reflect.ValueOf(authority)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			authority = nil
		}
	}
	return httpauth.NewAdmittedOIDC(config, s.pool, s.clock, registeredIssuance{s, authority}, policy, successPath, issuanceGate{s})
}

type registeredIssuance struct {
	server    *Server
	authority issuesession.IssuanceGuard
}

func (g registeredIssuance) Check(ctx context.Context, identity issuesession.Identity) (issuesession.GuardDecision, error) {
	if g.authority == nil {
		return issuesession.GuardDecision{}, issuesession.ErrGuardUnavailable
	}
	// This runs before admission and again inside its callback, so do not
	// re-enter the gate here. The durable row is necessary, never sufficient.
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	if _, err := g.server.activeSnapshot(ctx); err != nil {
		return issuesession.GuardDecision{}, issuesession.ErrGuardUnavailable
	}
	return g.authority.Check(ctx, identity)
}

type issuanceGate struct{ server *Server }

func (g issuanceGate) Within(ctx context.Context, sessionID string, epoch int64, use func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, startBudget)
	defer cancel()
	snapshot, err := g.server.currentGate(ctx)
	if err != nil || sessionDenied(snapshot.Denials, sessionID) {
		return issuesession.ErrGuardUnavailable
	}
	err = g.server.gate.Start(ctx, globalSession(sessionID, epoch), func(work context.Context) error {
		latest, err := g.server.activeSnapshot(work)
		if err != nil || !reflect.DeepEqual(snapshot.Denials, latest.Denials) {
			return issuesession.ErrGuardUnavailable
		}
		return use(work)
	})
	if errors.Is(err, startgate.ErrDenied) || errors.Is(err, runtimeaccess.ErrDenied) {
		return issuesession.ErrIssuanceDenied
	}
	return err
}
