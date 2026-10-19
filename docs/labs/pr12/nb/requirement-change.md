# HIT-50 / NB-R2 — significant contract refinement

Status: `CANDIDATE_MANUAL_LAB_INPUT`, authored by Никита Будыкин on
2026-10-19 Europe/Moscow. This is a local refinement PR; it does not alter the
canonical Candidate source, create a formal HP-ADR, or grant runtime authority.

## Selected gap from R1

R1 established the raw-body HMAC rule and listed malformed headers, freshness,
safe errors and no-business-admission as acceptance oracles. It did not make the
precedence and duplicate-token behavior explicit enough for two independent
implementations to produce the same result. That ambiguity would affect both
the domain verifier and the future application consumer.

## Before / after

| concern | R1 wording/state | refined local contract | rationale |
| --- | --- | --- | --- |
| dependency/body precedence | secret/clock/body/header were listed, but ordering was implicit | empty secret or unavailable clock rejects first; then empty body; then strict header parsing | prevents sensitive work and makes safe error precedence deterministic |
| timestamp duplicates | “contradictory timestamp rejects” | equal duplicate `t` is harmless; any different `t` rejects with `timestamp_conflict`, regardless of token order | prevents first-token wins and parser-order disagreement |
| version/unknown keys | malformed/missing/v0/unsupported were listed | at least one valid `v1` is required; `v0` never substitutes; every unknown key rejects | prevents legacy-only or unknown input from becoming verified |
| freshness boundary | absolute 300 seconds | `abs(now-t) <= 300` passes; 301 fails in either direction; tolerance is not caller-configurable | makes inclusive boundary testable and prevents a disabled check |
| business side effects | verification is not paid/accepted/2xx | `VERIFIED` is only a typed proof handoff; no receipt, outbox, 2xx or payment decision | prevents accidental expansion of module ownership |

## Refined acceptance criteria

1. For fixed `body`, `header`, `secret` and `now`, the verifier returns the same
   bounded `Outcome`; it never parses or rewrites raw bytes.
2. Error precedence is observable with synthetic fixtures: dependency failure,
   empty body, malformed header, missing/conflicting timestamp, missing `v1`,
   stale timestamp, and MAC mismatch produce their specified safe reasons.
3. The timestamp boundary matrix passes for past/future 299 and 300 seconds and
   rejects past/future 301 seconds.
4. Repeated equal `t` values and multiple `v1` candidates are accepted when a
   candidate MAC matches; conflicting `t`, v0-only, unknown keys and altered
   bytes reject.
5. No result or diagnostic exposes secret, raw body, header or provider payload;
   no verification result is named `accepted`, `paid`, or `2xx`.
6. A candidate implementation demonstrates the contract with the named tests in
   `lab-component.json`; actual results are recorded by HIT-51/HIT-52/HIT-53,
   not asserted by this refinement document.

## Traceability and verification

The refinement is grounded in R1's pinned sources: HP-BP-002 T/001/T/002,
HP-BP-007 T/010, and the local implementation convention. The exact candidate
inputs remain the context-lock digests recorded in
`requirements-trace.json`; source status remains `Candidate/In Review`.

Applicable local document checks for this PR:

```sh
git diff --check
python3 -m json.tool docs/labs/pr12/nb/requirements-trace.json >/dev/null
rg -n 'CANDIDATE|NOT_CONDUCTED|PLANNED|UNKNOWN' docs/labs/pr12/nb
```

The refinement can be consumed exactly by C1/C2 candidate artifacts at their
recorded SHAs, but until human review/merge those artifacts remain In Review.
The code-level verification and CI results are linked in the handoff after the
actual runs; this document does not promote any external gate.
