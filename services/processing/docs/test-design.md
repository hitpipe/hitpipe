# Financial policy test design

Oracle fixed before execution in HIT-25 requirements.md and HIT-26 requirement-change.md, derived from RULE006, BP002/T004, BP003/T006, BP007/T005/010 and canonical SourceFacts description. It is independent of actual program output.

Techniques: equivalence partitioning (known valid, known disallowed, unknown, malformed, foreign scope); boundary values (0/1, 11999/12000/12001, 2^53-1/+1, activation ±1ns, expiry ±1ns, proof oldest/completed order); decision table with exactly one changed predicate plus explicit multi-violation precedence. No lifecycle state-table claim: this stateless object does not own case state.

Checklist: payment/mode/paid; positive exact total; received and capture equal total; Charge success and capture; partial refund and dispute; cross-object currencies; one line/quantity/fixed Price; account/connection/test-live scope; Session activation boundary; complete pagination and every nullable mandatory field; schema/evaluator/config and case/Session pins; explicit oldest proof freshness; canceled/failed reader without retries; safe strict JSON diagnostics; deterministic read-only 100-repeat result. Fixtures all synthetic, fixed time, no external data or random seed. Reset: fresh fixture per test; adapter tests use isolated t.TempDir; no network/DB/secret state.

TestReaderSubstitution runs one unchanged consumer with File and finite double: normal/error/expiry boundary, denial and unavailable-read. Double does not test authentication, TLS, provenance/digest or Intake/provider read behavior. File is retained diagnostic implementation, never production source authority. Hidden teacher suite: NOT PROVIDED / NOT RUN.

Coverage command: go test -count=1 -coverprofile=... ./...; go tool cover -func=... . Scope and concrete gaps/results retained in personal evidence/c3. Adapter code may execute in a consumer test without default package coverage attribution; direct adapter tests provide own-package coverage. Statement coverage is not branch or semantic completeness.

Mutations: temporary detached Git worktree at exact baseline; compile each mutated source before tests; six one-at-a-time behavioral changes, each diff/digest/build/test log. Compile failure INVALID, never KILLED. Survivors require explanation, added test and preserved initial/retest; none are forced. Restore original bytes, verify clean diff, rerun green. Separate sensitivity check invokes exactly the CI wrapper against first compiled mutant and must fail.

Risk selection uses the teacher's qualitative matrix (low×severe HIGH; high×small MEDIUM). Financial false permission, wrong tenant/scope and unknown/proof failures are HIGH, run every change. Malformed/oversized synthetic input HIGH, run every change. Real provider authority/revoke/lineage is HIGH with future integration tests and explicit human gates; no fake E2E. All local/CI counts and natural-defect count are recorded from actual runs only.
