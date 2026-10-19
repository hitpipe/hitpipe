# Delivery module map

Status: candidate / manual laboratory. Owner: Дмитрий Стацюк.

| Module | Layer | Owns | Imports / arrows | Status |
|---|---|---|---|---|
| internal/domain/readback | domain | pure exact comparison and typed outcomes | standard library + x/text normalization | implemented, checks PASS |
| internal/application/verifytarget | application | VerifyTarget orchestration and TargetReader port | domain/readback | implemented, checks PASS |
| internal/adapters/fixtures/targetreader | adapter | file-backed saved-observation read | application port + domain DTO | implemented, checks PASS |
| cmd/delivery-contractcheck | composition/diagnostic | synthetic fixture entrypoint | application/domain | implemented, release smoke PASS |

The comparator has no provider, database, network, retry, state-transition, or durable-message
dependency. The adapter returns observation provenance; the application preserves it and maps
read errors to unknown. This map is local implementation evidence, not a change to canonical
Architecture/Product contracts.
