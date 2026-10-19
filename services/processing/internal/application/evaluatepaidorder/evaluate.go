// Package evaluatepaidorder is the production application consumer of the
// financial policy. Transport, persistence and source authenticity are adapter
// responsibilities; actor/epoch/lifecycle/lineage/dispatch guards are future work.
package evaluatepaidorder

import (
	"context"
	"github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility"
	"time"
)

// Query carries the pinned scope of exactly one source read.
type Query struct {
	CaseID, SessionID, ConfigID, AccountID, ConnectionID string
	LiveMode                                             bool
}

// Snapshot is the supported policy pin and an immutable domain-facts projection.
type Snapshot struct {
	ConfigID, SchemaVersion, EvaluatorVersion string
	Facts                                     eligibility.Facts
}

// SourceFactsReader owns source access. Return immutable values valid for the
// duration of Evaluate. A failure must return error, never invented financial data.
// Implementations must honor context; the consumer never retries an unknown read.
type SourceFactsReader interface {
	Read(context.Context, Query) (Snapshot, error)
}

// Request is a read-only context for an already pinned case and accepted config.
// No operation here accepts a config or checks runtime user authority.
type Request struct {
	Policy            eligibility.Policy
	CaseID, SessionID string
	Now               time.Time
}

// EvaluatePaidOrder reads once, verifies version/identity, then calls the shared
// pure evaluator. Result is financial knowledge only; callers own all results.
// A failed/canceled read yields INCOMPLETE; no case or provider state is written.
func EvaluatePaidOrder(ctx context.Context, r Request, reader SourceFactsReader) eligibility.Result {
	if !eligibility.ValidatePolicy(r.Policy) || r.CaseID == "" || r.SessionID == "" || r.Now.IsZero() {
		return eligibility.Result{Status: eligibility.Incomplete, Reason: "INVALID_CONTEXT"}
	}
	if ctx.Err() != nil || reader == nil {
		return eligibility.Result{Status: eligibility.Incomplete, Reason: "READER_UNAVAILABLE"}
	}
	q := Query{CaseID: r.CaseID, SessionID: r.SessionID, ConfigID: r.Policy.ConfigID, AccountID: r.Policy.AccountID, ConnectionID: r.Policy.ConnectionID, LiveMode: *r.Policy.LiveMode}
	s, err := reader.Read(ctx, q)
	if err != nil || ctx.Err() != nil {
		return eligibility.Result{Status: eligibility.Incomplete, Reason: "READER_UNAVAILABLE"}
	}
	if s.ConfigID != r.Policy.ConfigID || s.SchemaVersion != r.Policy.SchemaVersion || s.EvaluatorVersion != r.Policy.EvaluatorVersion {
		return eligibility.Result{Status: eligibility.Incomplete, Reason: "PIN_MISMATCH"}
	}
	if s.Facts.CaseID != r.CaseID || s.Facts.SessionID != r.SessionID {
		return eligibility.Result{Status: eligibility.Incomplete, Reason: "IDENTITY_MISMATCH"}
	}
	return eligibility.Evaluate(r.Policy, s.Facts, r.Now)
}
