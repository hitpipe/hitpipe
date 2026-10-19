# HIT-53 / NB-C3 — test-design module

The executable source is `services/intake/docs/test-design.md`; this personal
module records the report-facing result and links to actual evidence. Techniques:
equivalence classes + boundary analysis, and a decision/state table. Fixtures are
fixed synthetic bytes/time and reset per subtest; no private fixture is required.

Required checks are local `go vet`, named `go test`, `go test -race`, wrapper CI
check, dev/release outside-checkout smoke, coverage interpretation, and an exact
same-check negative CI sensitivity run. The oracle is the pinned HP-BP-002/T/001
rule plus the independent HMAC calculation in tests, not application output.
