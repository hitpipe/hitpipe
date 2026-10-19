# Round-2 correction record — HIT-14 / HIT-41–48

Participant: Дмитрий Стацюк. Mode: `MANUAL_LAB_PR12`. Working PR: [#29](https://github.com/hitpipe/hitpipe/pull/29). Code correction source: `bafc97a0eb5cc2b2aba68c04a74b179b61432d82`.

This record maps the seven still-open findings from the repeat-review prompt to the correction source, executable checks, and evidence. It does not claim independent approval, human-gate completion, or merge readiness.

| DS-REV-ID | Correcting commit / PR | Test or inspection | Evidence | Status |
|---|---|---|---|---|
| DS-REV-001 | `bafc97a0eb5cc2b2aba68c04a74b179b61432d82` / PR #29 | `go test ./...`; boundary and handoff regressions; build/smoke logs | `review_regressions_test.go`, `HIT-48-go-test-round2.log`, four DOCX reports | EVIDENCED_PENDING_INDEPENDENT_REVIEW |
| DS-REV-003 | same / PR #29 | strict JSON aliases, duplicates, EOF, UTF-8 and surrogate tests | `fixtures.go`, `review_regressions_test.go`, module contract, DOCX reports | EVIDENCED_PENDING_INDEPENDENT_REVIEW |
| DS-REV-004 | same / PR #29 | Fixture CLI and FileReader contract tests; six DI scenarios | `di_contract_test.go`, `module-map.md`, `HIT-48-go-test-round2.log` | EVIDENCED_PENDING_INDEPENDENT_REVIEW |
| DS-REV-006 | same / PR #29 | two test-design techniques; risk-register and mutation evidence | `test-design.md`, `risk-register.md`, `0302-pz02-ds.docx` | EVIDENCED_PENDING_INDEPENDENT_REVIEW |
| DS-REV-007 | same / PR #29 | criteria crosswalk, 29 frozen matrix rows, local test/vet/race/smoke logs | `criteria-crosswalk.json`, `index-round2.json`, evidence logs | EVIDENCED_PENDING_INDEPENDENT_REVIEW; fresh CI queued |
| DS-REV-008 | same / PR #29 | four DOCX main tables filled; structural/a11y audit; renderer limitation recorded | all four DOCX and screenshot instruction files | EVIDENCED_PENDING_INDEPENDENT_REVIEW |
| DS-REV-009 | same / PR #29 | one consumer × FileReader/test implementation × normal/boundary/error = six distinct cases | `di_contract_test.go`, `module-contract.md`, `module-map.md`, `0104-pz02-ds.docx` | EVIDENCED_PENDING_INDEPENDENT_REVIEW |

Fresh Actions runs for the code source are recorded in `criteria-crosswalk.json` and `evidence/index-round2.json`; at preparation time they were still `queued`. Human screenshots, interviews, opposition/peer review, independent repeat review, and final acceptance remain human actions. No PNG was created.
