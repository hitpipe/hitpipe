package httpauth

import (
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
	return New(flow, issuer, successPath)
}
