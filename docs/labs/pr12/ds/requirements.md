# HIT-41 R1 — Target readback requirements

Status: `DESK_RESEARCH_BASELINE / CANDIDATE`, not an interview or acceptance record.
Owner: Дмитрий Стацюк. Scope: the offline Delivery TargetReadback slice.

## Sources and boundaries

The design input is the locked Candidate/In Review package `HP-VER-001` and the exact
formal schemas named by HIT-44: `DeliveryIntent`, `ObservedTaskAttributes` and
`TaskObservation`, plus HP-RULE-007/008 and HP-BP-004/T/007,T/008,T/013. The package
does not grant live provider access or acceptance. This local module compares a requested
target with a saved observation; it never performs provider writes, retries, state
transitions or durable publishing.

## Functional requirements

| ID | Requirement | Error/unknown boundary | Acceptance evidence |
|---|---|---|---|
| FR-1 | Compare workspace, list, title, configured assignee, canonical handoff block and context. | Missing/invalid evidence is not a match. | `Compare` and domain regression tests. |
| FR-2 | Require one canonical `[HITPIPE-HANDOFF:<case-UUID>:v1]...[/HITPIPE-HANDOFF:<case-UUID>:v1]` block. | Missing, multiple or malformed blocks are `incomplete`. | canonical and malformed-marker tests. |
| FR-3 | Bind saved evidence to requested target and provenance. | Relabelled target, missing source/time, read error are `unknown`. | FileReader and VerifyTarget DI tests. |
| FR-4 | Decode only strict, valid UTF-8 JSON with canonical key spelling. | Invalid bytes, surrogate escapes, aliases and unsupported fields are rejected. | fixture boundary tests. |

## Non-functional requirements and acceptance criteria

- NFR-1: NFC and CRLF/CR→LF normalization only; meaningful spaces and controls remain significant.
- NFR-2: context serialized evidence budget is 32 KiB and the complete handoff block budget is 36 KiB.
- NFR-3: pure domain code has no provider/network/database/retry side effects; the application preserves provenance.
- AC-1: baseline, mismatch, incomplete and unknown outcomes are distinguishable.
- AC-2: FileReader and a synthetic double run through the unchanged VerifyTarget consumer for normal, boundary and error cases.
- AC-3: exact commands, source SHA, logs and limitations are indexed; human screenshots remain placeholders only.

## Interview boundary

No interview was conducted in this correction pass. Questions and desk-research facts are
kept separate in `interview.md`; no participant answer or signature is inferred.
