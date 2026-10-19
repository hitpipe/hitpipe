# Correction record — DS-REV-001–009

Correction commit: `7e815aa790e0ebba58e453e786bda45b9266161d`.
Correction evidence/report commit: `c365ad5689da032fb9d53c115fe8f9791bbc031e`.
Correction PR: https://github.com/hitpipe/hitpipe/pull/29. Base: `0a6f2ef3a5cd6db30ff4cef5efbefd185fc47b00`.

| Finding | Correcting commit/PR | Regression/test | Evidence |
|---|---|---|---|
| DS-REV-001 | `7e815aa`; PR #29 | `TestReviewCanonicalHandoffMatches`, canonical v1 validation and budget tests | `HIT-47-go-test.log`, `requirements.md`, `services/delivery/internal/domain/readback/readback.go` |
| DS-REV-002 | `7e815aa`; PR #29 | `TestReviewSavedObservationCannotBeRelabelled`, `TestReviewNoProvenanceCannotMatch`, `TestFileReaderAndDoubleShareVerifyTargetContract` | targetreader/application tests and `correction-index.json` |
| DS-REV-003 | `7e815aa`; PR #29 | `TestReviewJSONBoundaryRejectsAliasesAndMalformedUnicode` | `HIT-47-go-test.log`, strict fixture decoder |
| DS-REV-004 | `7e815aa`; PR #29 | `TestReviewComparatorRejectsInvalidTitleAndRetrievalError` | `HIT-47-go-test.log` |
| DS-REV-005 | `7e815aa`; PR #29 | `TestReviewComparatorNormalizesLoneCRWithoutChangingSpaces` | `HIT-47-go-test.log` |
| DS-REV-006 | `c365ad5`; PR #29 | five compiling mutants M1–M5; final restored suite, race, vet, workspace check/smoke | `HIT-45-mutations-actual.log`, `HIT-45-mutation-matrix.md`, `HIT-47-go-race.log` |
| DS-REV-007 | `7e815aa`/`c365ad5`; PR #29 | R1 artifacts and 29-record crosswalk; interview explicitly NOT CONDUCTED | `requirements.md`, `interview.md`, `security-baseline.md`, `requirements-trace.json`, `criteria-crosswalk.json` |
| DS-REV-008 | `c365ad5`; PR #29 | four template-based DOCX, 21 instructions, ZIP/XML/token/media checks | `docs/labs/pr12/ds/reports/`, `screenshots-INSTRUCTIONS.md`; renderer unavailable, no visual PASS claimed |
| DS-REV-009 | `7e815aa`; PR #29 | same VerifyTarget consumer across normal/boundary/error for file adapter and double; GoDoc and import map | `di_contract_test.go`, `module-map.md`, `go vet` log |

This record is a correction handoff, not an approval. Independent repeat review by the
delegated reviewer remains pending. No merge, tag, human interview, opposition, screenshot,
Feature close or self-approval is claimed.
