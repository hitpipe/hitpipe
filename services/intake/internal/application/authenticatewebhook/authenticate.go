// Package authenticatewebhook owns the production-shaped consumer boundary for
// signature verification. Dependencies are narrow ports selected at composition.
package authenticatewebhook

import (
	"context"
	"errors"

	"github.com/hitpipe/hitpipe/services/intake/internal/domain/stripesignature"
)

// SecretResolver resolves only the installation-scoped secret required for one
// verification. Implementations must not expose the secret in errors or logs.
type SecretResolver interface {
	Resolve(ctx context.Context, installationID string) ([]byte, error)
}

// Clock supplies trusted Unix seconds. The consumer never reads time directly.
type Clock interface {
	NowUnix(ctx context.Context) (int64, error)
}

// Input is the untrusted webhook envelope. RawBody is intentionally opaque.
type Input struct {
	InstallationID string
	EventID        string
	RawBody        []byte
	Signature      string
}

// VerifiedWebhook is handed to a later typed parser only after verification.
// The consumer does not parse JSON or create a durable receipt.
type VerifiedWebhook struct {
	InstallationID string
	EventID        string
	RawBody        []byte
	Timestamp      int64
}

// Result is safe to expose to diagnostics and callers. Sensitive dependency
// errors are mapped to bounded reasons.
type Result struct {
	Outcome stripesignature.Outcome
	Webhook *VerifiedWebhook
}

// Consumer authenticates each delivery. It deliberately has no event-ID cache:
// a repeated event still has to pass signature verification before any future
// deduplication/receipt consumer may inspect it.
type Consumer struct {
	secrets SecretResolver
	clock   Clock
}

// New creates a consumer with both required dependency ports.
func New(secrets SecretResolver, clock Clock) (Consumer, error) {
	if secrets == nil {
		return Consumer{}, errors.New("secret resolver is required")
	}
	if clock == nil {
		return Consumer{}, errors.New("clock is required")
	}
	return Consumer{secrets: secrets, clock: clock}, nil
}

// Authenticate resolves installation scope and trusted time before checking
// exact raw bytes. Dependency errors are fail-closed and never returned with
// their underlying text.
func (c Consumer) Authenticate(ctx context.Context, input Input) Result {
	if input.InstallationID == "" {
		return Result{Outcome: rejected(stripesignature.ReasonSecretUnavailable)}
	}
	secret, err := c.secrets.Resolve(ctx, input.InstallationID)
	if err != nil || len(secret) == 0 {
		return Result{Outcome: rejected(stripesignature.ReasonSecretUnavailable)}
	}
	nowUnix, err := c.clock.NowUnix(ctx)
	if err != nil || nowUnix == 0 {
		return Result{Outcome: rejected(stripesignature.ReasonClockUnavailable)}
	}
	outcome := stripesignature.Verify(input.RawBody, input.Signature, secret, nowUnix)
	if outcome.Status != stripesignature.Verified {
		return Result{Outcome: outcome}
	}
	return Result{
		Outcome: outcome,
		Webhook: &VerifiedWebhook{
			InstallationID: input.InstallationID,
			EventID:        input.EventID,
			RawBody:        input.RawBody,
			Timestamp:      outcome.Timestamp,
		},
	}
}

func rejected(reason stripesignature.Reason) stripesignature.Outcome {
	return stripesignature.Outcome{Status: stripesignature.Rejected, Reason: reason, Version: stripesignature.Version}
}
