# HIT-53 / NB-C3 — coverage analysis

## Actual run

The baseline run is recorded in
`evidence/hit-53-baseline-v2.log` and `evidence/hit-53-baseline-v2.cover`.
It used commit `835bbb1bc565e9b2786d5ed1a2cac166f816f7a8`, Go `go1.27.2
darwin/arm64`, cwd `services/intake`, `go test -coverprofile=... ./...`,
`go tool cover -func=...`, `go vet ./...`, `go test -race ./...`, and the
workspace wrapper's check/smoke/release inspection.

The unit coverage report gives:

| bounded area | statement coverage | interpretation |
| --- | ---: | --- |
| `internal/domain/stripesignature` | 87.7% on original baseline | parser, raw-byte, timestamp-boundary, rotation and fail-closed paths were exercised; the correction adds checked-overflow and invalid-clock regressions, with updated coverage pending CI |
| `internal/application/authenticatewebhook` | 83.3% | normal path, dependency errors, repeat event and fixed-clock/file-secret composition are exercised; the defensive nil-input branch remains outside the current fixture set |
| CLI/adapters | 0.0% unit profile | exercised by wrapper dev/release smoke and CLI evidence, but not instrumented by package unit tests; this is an explicit coverage limitation, not an inferred PASS |

The remaining CLI/adapter unit-coverage gap is bounded by the actual smoke
checks and is listed as follow-up work rather than silently counted as unit
coverage. The five original behavioral mutations in `MUTATION_PROTOCOL.md` were
compiled and killed, providing sensitivity evidence for the highest-risk
signature and timestamp rules. CPU/memory profiling was not run; a Go `pprof`
route is planned only for a separately bounded performance experiment.
