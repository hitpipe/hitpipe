# HIT-45 test design and coverage

Status: actual correction-source test design for the offline TargetReadback slice. The hidden provider set and human gates remain unknown/not conducted.

## Technique 1 — equivalence partitions

The comparator partitions evidence into `match`, field `mismatch`, structural `incomplete`, and retrieval/provenance `unknown`. JSON inputs are partitioned into exact keys, duplicate/mixed/escaped aliases, invalid UTF-8, valid surrogate escapes, unpaired surrogates, and trailing bytes. Handoff inputs are partitioned into one valid lowercase-UUID block, missing/multiple/nested markers, missing labels, mismatched case IDs, and forbidden controls. Each partition maps to an assertion in `review_regressions_test.go`, `readback_test.go`, or `di_contract_test.go`.

## Technique 2 — boundary-value analysis

The byte boundaries are derived from the rules, not an arbitrary percentage: a configured-key→normalized-value JSON projection is limited at 32 KiB, and the complete rendered handoff block at 36 KiB. The quote-heavy context test uses 20,000 quote characters, which expands through JSON escaping and exceeds the 32 KiB projection although the raw string alone does not. The handoff tests cover empty/label-less bodies and the maximum rendered-block guard. Unicode tests cover `\\ud000`, `\\ud83d\\ude00`, unpaired surrogates, invalid UTF-8 and NUL.

## Executed oracle set

`go test ./...` asserts exact typed outcomes and error boundaries. `go test -race ./...` checks the same assertions under the race detector. `go vet ./...` checks the Go implementation. `workspace.py smoke delivery --require` exercises the CLI from a temporary directory outside the checkout using the canonical fixtures. The CLI synthetic reader is intentionally reported as CLI evidence only; FileReader evidence is the six-scenario DI contract test.

## Coverage analysis

The current service coverage is recorded from the actual command in `docs/labs/pr12/ds/evidence/HIT-47-go-test.log`: domain/readback 74.7%, application/verifytarget 77.8%, FileReader 63.2%, CLI 0.0% because the CLI package has no unit functions. The uncovered CLI wiring is covered by the outside-checkout smoke; provider I/O, live permissions, and human protocols are out of scope. The decision is to keep the CLI as a thin composition root and add semantic tests at domain/application/adapter boundaries rather than inflate coverage with wiring-only tests.
