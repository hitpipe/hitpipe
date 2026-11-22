package httpauth

import (
	"context"
	"net/url"
	"strings"

	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/oidc"
	"github.com/hitpipe/hitpipe/services/iam/internal/adapters/postgres/sessionissuer"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/issuesession"
	"github.com/hitpipe/hitpipe/services/iam/internal/application/oidclogin"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewOIDC wires the two login routes to the existing IAM PostgreSQL issuer.
// The caller owns the pool and supplies trusted time, issuance authority and
// session lifetimes for its environment. No test authority is selected here.
func NewOIDC(config oidc.Config, pool *pgxpool.Pool, clock issuesession.Clock, guard issuesession.IssuanceGuard, policy issuesession.Policy, successPath string) (*Handler, error) {
	return newOIDC(config, pool, clock, guard, policy, successPath, nil)
}

// NewAdmittedOIDC reuses the accepted adapter and additionally binds session
// creation and its successful cookie response to the configured runtime gate.
func NewAdmittedOIDC(config oidc.Config, pool *pgxpool.Pool, clock issuesession.Clock, guard issuesession.IssuanceGuard, policy issuesession.Policy, successPath string, admission issuesession.StartAdmission) (*Handler, error) {
	if admission == nil {
		return nil, issuesession.ErrConfiguration
	}
	return newOIDC(config, pool, clock, guard, policy, successPath, admission)
}

func newOIDC(config oidc.Config, pool *pgxpool.Pool, clock issuesession.Clock, guard issuesession.IssuanceGuard, policy issuesession.Policy, successPath string, admission issuesession.StartAdmission) (*Handler, error) {
	callback, err := url.Parse(config.RedirectURI)
	if err != nil || callback.EscapedPath() != "/auth/callback" {
		return nil, issuesession.ErrConfiguration
	}
	authorization, err := url.Parse(config.AuthorizationEndpoint)
	if err != nil {
		return nil, issuesession.ErrConfiguration
	}
	// Cookies are scoped by host, not port. Keep browser credentials on IAM.
	if strings.EqualFold(strings.TrimSuffix(callback.Hostname(), "."), strings.TrimSuffix(authorization.Hostname(), ".")) {
		return nil, issuesession.ErrConfiguration
	}
	provider, err := oidc.New(config)
	if err != nil {
		return nil, err
	}
	flow, err := oidclogin.New(provider, clock)
	if err != nil {
		return nil, err
	}
	store, err := sessionissuer.New(pool)
	if err != nil {
		return nil, err
	}
	issuer, err := issuesession.New(flow, clock, guard, store, policy)
	if err != nil {
		return nil, err
	}
	handler, err := New(flow, issuer, successPath)
	if err != nil {
		return nil, err
	}
	if admission != nil {
		handler.issueAtStart = func(ctx context.Context, proof string, emit func(context.Context, issuesession.Result) error) error {
			return issuer.ExecuteAtStart(ctx, proof, admission, emit)
		}
	}
	return handler, nil
}
