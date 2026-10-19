# Local implementation ADR: TargetReadback boundaries

Status: CANDIDATE / In Review. This is a personal module ADR for HIT-46, not a second
HP-ADR-001 and not a formal Architecture acceptance.

## Decision

Use a pure domain comparator behind a consumer-owned TargetReader port. Keep file-backed
saved observations and synthetic doubles as separate adapters. The VerifyTarget consumer
returns typed comparison plus provenance and performs no provider write, retry, state
transition, or durable publish.

## Options

1. Put provider reads and comparison in one adapter. Rejected: couples policy to transport
and makes the false-confirmation boundary hard to test.
2. Put comparison in the application and let adapters return loosely typed maps. Rejected:
expands the consumer boundary and weakens exact Unicode/marker/error semantics.
3. Pure comparator + narrow port + interchangeable adapters. Selected: preserves module
ownership, supports deterministic fixtures, and leaves live provider authority outside
this bounded task.

## Two complexity directions

- Data handling: NFC and CRLF normalization while preserving meaningful whitespace and
rejecting malformed UTF-8.
- Interaction: retrieved evidence, incomplete evidence, and unknown read failures must
remain distinct without implicit retry or provider writes.

## Verification / cost

The selected design is checked by domain tests, application tests, adapter tests, go test,
go vet, and synthetic CLI smoke. It does not prove live destination permissions,
provider GET semantics, effect fencing, or final business state. A later real adapter would
need its own transport and one-shot/common-gate review.
