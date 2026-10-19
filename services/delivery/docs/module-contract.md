# TargetReadback module contract

The module owns a read-only comparison of a requested `readback.Expected` projection
against one saved `readback.Observation`. `verifytarget.Verifier.VerifyTarget` is the
public application operation; its consumer owns the `TargetReader` port and does not
depend on FileReader internals.

## Inputs and projection

The local projection maps `DeliveryIntent` to `Expected` fields `workspace`, `list`,
`title`, configured `assignee`, canonical `handoff_block` and `context`. A saved
`ObservedTaskAttributes`/`TaskObservation` projection maps to `Observation` with
`target_id`, `retrieved`, optional `retrieval_error`, attributes, assignee list and
the same handoff/context fields. This is a service-local mapping; it does not modify
the frozen upstream wire schema or claim live provider authority.

Fixture and FileReader JSON use one strict policy: exact lower-case object keys,
no duplicates, mixed/escaped aliases, unknown fields or trailing JSON; UTF-8 must be
valid and surrogate escapes must be paired. `\\ud000` and `\\ud83d\\ude00` are valid.

The handoff block has exactly one canonical delimiter pair with a lower-case UUID,
matching open/close IDs and separate non-empty `Service:`, `Order:` and `Context:`
lines. NUL and other forbidden controls are rejected. NFC and CR/CRLF→LF are the only
normalizations; meaningful spaces remain significant. The configured-key normalized
projection is measured after JSON escaping at 32 KiB, and the rendered handoff block
at 36 KiB.

## Outcomes and precedence

`unknown` is returned for read/provenance/retrieval failure or invalid text; `incomplete`
for missing/structurally invalid/budget-exceeding evidence; `mismatch` for a valid
observation with a differing required attribute; `match` only after every guard passes.
The operation has no provider/network/database/write/retry/message/state-transition
side effects. FileReader is a saved-observation adapter; the CLI fixture reader is a
synthetic diagnostic adapter and is not live provider evidence.

## Replacements and evidence

FileReader and the test `TargetReader` implementation use the unchanged VerifyTarget
consumer for normal, quote-heavy boundary and error scenarios. The six cases are
recorded in `di_contract_test.go`. The CLI invokes the same consumer with a synthetic
fixture adapter and is covered separately by outside-checkout smoke.
