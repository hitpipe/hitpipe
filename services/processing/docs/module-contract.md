# PaidOrderEligibility

HIT-27, Тимофей Патрушев при помощи Codex. Pinned Candidate: HP-RULE-006/011/012, BP002/T004, BP003/T002/006/012, BP007/T005/010. Exact source-lock: docs/labs/pr12/context-lock.json. Local engineering contract; canonical OAS/SQL remain unchanged.

Purpose: deterministic financial eligibility under an existing accepted immutable policy. Public operation eligibility.Evaluate(policy, facts, now) → Result. CLI processing-contractcheck is the retained offline diagnostic; EvaluatePaidOrder application consumer follows in HIT-28. No case/config writes, admission, lifecycle, actor authority, epoch/lineage or dispatch permission.

Inputs: read-only borrowed policy and fact values with nullable pointers; supported schema 1.0 and evaluator paid-order-v1; explicit evaluation time. Caller owns inputs and must prevent concurrent mutation during call. Result owns two strings, no borrowed references, no durable validity promise. Source snapshot can age immediately after call.

Error precedence: invalid policy/shape → source scope → unknown/incomplete → invalid financial data → proof timeline/expiry → ordered financial predicates. Local statuses VALID, INELIGIBLE, INCOMPLETE, REFRESH_REQUIRED are not new canonical wire enums. Stable reasons are in GoDoc/source and requirement-change.md. No panic/error-as-unpaid conversion.

Currency must be lowercase ASCII three letters; all_currencies_equal proves cross-object comparison by upstream. Money values exact integers 0..2^53-1; total>0, received=captured=total, refund=0. payment/paid, succeeded/captured Charge, no dispute, one item, one unit, configured fixed Price, matching source account/mode/connection. Session creation equal to activation is supported. Proof expiry exclusive, <=oldest mandatory fact+10s; collection<=30s; future/inconsistent timestamps fail closed. No default conversion for unknown.

Fixtures: testdata/valid.json contains synthetic accepted-policy projection, fixed time, full facts. Change --fixture to a denied scenario to change output. Invalid fixture/format/policy → exit2; valid business denial → exit0 with explicit non-VALID result; output failure exit3. This convention keeps process health separate from business eligibility.

Offline projection is NOT a hand-maintained generated wire DTO. It accepts neither arbitrary provider payload nor customer credentials. Future wire adapter must authenticate source scope, validate full canonical OAS including provenance/digest, then map to domain. Current fixture adapter proves only offline declared domain, not authentic payment. Standard library only; go.sum absent because there are no external modules.

Reproduce from repo root with exact Go 1.27.2: python3 tools/pr12/workspace.py check processing --require; python3 tools/pr12/workspace.py smoke processing --require. All data versioned; reset by rereading fixtures; no database or seed state. CLI reads at most 1MiB synthetic input and rejects duplicate/unknown keys, malformed shape and trailing documents. Logs contain only metadata/status/reason, never bodies.

Example: s := input.Read(fixture); r := eligibility.Evaluate(s.Policy,s.Facts,s.Now). Production application consumer uses the same policy through a consumer-owned port. Synthetic adapters will not replace a future authenticated Intake adapter.

## Application consumer and substitution

HIT-28 adds evaluatepaidorder.EvaluatePaidOrder(ctx, Request, SourceFactsReader). Request pins policy/config ID and versions, case, Session and explicit time. Reader receives scoped Query once; no retry. Consumer verifies returned config/schema/evaluator then case/Session identity before domain evaluation. Unsupported context, missing reader, cancellation/error and returned pin mismatch are INCOMPLETE. Application has no concrete adapter imports. File reader and finite in-memory test double execute identical normal, expiry boundary, nullable error, financial denial and unavailable-read scenarios through this one consumer. Double is test-only, not a live Intake replacement. File adapter reads anew and respects cancellation around bounded IO. No authenticity, TLS, digest or remote latency proof follows from these tests. CLI composition binds File; future service transport will reuse the same consumer with authenticated Intake adapter and all separate owner guards.
