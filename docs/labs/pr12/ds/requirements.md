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
| FR-2 | Require one canonical `[HITPIPE-HANDOFF:<lowercase-UUID>:v1]` block with exactly one delimiter pair, `Service:`, `Order:` and `Context:` lines, matching opening/closing UUID and no nested marker. | Missing, multiple, malformed, control-containing or structurally incomplete blocks are `incomplete`. | canonical, marker-shape and control-input tests. |
| FR-3 | Bind saved evidence to requested target and provenance. | Relabelled target, missing source/time, read error are `unknown`. | FileReader and VerifyTarget DI tests. |
| FR-4 | Decode only strict, valid UTF-8 JSON with exact decoded keys, no duplicate/mixed/escaped aliases, valid surrogate pairing and mandatory EOF. | Invalid bytes, unpaired surrogates, aliases, duplicates, trailing values and unsupported fields are rejected; valid `\ud000` and `\ud83d\ude00` preserve their decoded value. | fixture and saved-observation boundary tests. |

## Non-functional requirements and acceptance criteria

- NFR-1: NFC and CRLF/CR→LF normalization only; meaningful spaces and controls remain significant.
- NFR-2: the configured-key→normalized-value context projection is measured as escaped JSON/JCS bytes and is limited to 32 KiB; the complete normalized rendered handoff block is limited to 36 KiB. Raw `Context` length alone is not the gate.
- NFR-3: pure domain code has no provider/network/database/retry side effects; the application preserves provenance.
- AC-1: baseline, mismatch, incomplete and unknown outcomes are distinguishable.
- AC-2: FileReader and a test implementation run through the unchanged VerifyTarget consumer for six distinct scenarios: normal, quote-heavy budget boundary and real FileReader read error for each implementation. The CLI fixture reader is reported separately and is not used as FileReader evidence.
- AC-3: exact commands, source SHA, logs and limitations are indexed; human screenshots remain placeholders only.

## Interview boundary

No interview was conducted in this correction pass. Questions and desk-research facts are
kept separate in `interview.md`; no participant answer or signature is inferred.
