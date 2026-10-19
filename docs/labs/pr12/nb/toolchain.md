# HIT-54 / NB-D1 — instrumented toolchain

Status: `CANDIDATE_MANUAL_LAB_INPUT`; source contracts remain Candidate/In
Review. This is the tool route for the owned Intake module, not a claim that
Project OS or a production deployment is active.

| stage | pinned tool/route | purpose | actual status/evidence |
| --- | --- | --- | --- |
| build | Go `go1.27.2`, module `github.com/hitpipe/hitpipe/services/intake`, `go 1.27.2` | compile the module and diagnostic binary | `PASS` locally; C1 log records archive SHA-256 `76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5` and exact version |
| deps | `go mod tidy`/module graph inspection, no network dependency in this slice | detect undeclared or accidental dependencies | `PASS` through the clean wrapper; `go.sum` is empty because only stdlib is used; no provider credentials |
| test | `go test -count=1 ./...` and named tests from `lab-component.json` | verify domain and application behavior | `PASS` in C1/C2 local logs; fixture-only deterministic tests, no real provider delivery |
| static | `gofmt -l`, `go vet ./...`, `git diff --check` | formatting, compiler/static diagnostics and patch hygiene | `PASS` in C1/C2 logs |
| dynamic | `go test -race -count=1 ./...`, wrapper `PR12_RACE=1 ... check intake` | race-sensitive execution and CI-sensitivity | `PASS` on restored baseline; controlled temporary failing test produced wrapper exit 1 and was removed; see `evidence/hit-53-ci-sensitivity.log` |
| coverage | `go test -coverprofile=... ./...`, `go tool cover -func=...` | identify tested and untested statement paths | Actual test coverage: domain 87.7%, application 83.3%; CLI/adapters 0% unit profile, exercised by smoke; see `coverage-analysis.md`. This is not CPU/memory profiling. |
| CPU/memory profiling | Go `pprof` route: `go test -cpuprofile=.pr12-build/intake.cpu.prof -run '^$' ./...` or a future benchmark/diagnostic run; inspect with `go tool pprof` | measure performance only when an explicit bounded workload and owner-approved run exist | `PLANNED`: no CPU or memory profile was executed in this lab; no performance claim is made |
| CI | `.github/workflows/pr12-nb.yml` with Go setup and `PR12_RACE=1` named check | reproduce local checks in GitHub Actions | workflow change is limited to the owned PR12 workflow; run URL/status is recorded in the handoff after GitHub completes; human review/merge remains pending |

## Clean-room route

From a clean checkout at the candidate commit:

```sh
export PATH="/tmp/hitpipe-go-1.27.2/go/bin:$PATH"
go version
git status --porcelain
python3 tools/pr12/workspace.py validate intake
PR12_RACE=1 python3 tools/pr12/workspace.py check intake
python3 tools/pr12/workspace.py smoke intake --require
python3 tools/pr12/workspace.py inspect intake --mode release
```

The wrapper builds and checks from `services/intake`; smoke invokes the
diagnostic binary with synthetic fixtures outside the checkout; release
inspection checks embedded revision/build metadata. Logs record absolute cwd,
commit, tool versions, exit code and the limitation that this is not a public
webhook route.
