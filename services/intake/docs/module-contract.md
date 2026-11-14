# StripeSignature module contract

Status: implementation candidate for `HIT-51`; source status remains
`Candidate/In Review`. This document describes the bounded verification module,
not a webhook endpoint or a complete Intake service.

## Responsibility

`StripeSignature` proves the authenticity and freshness of an exact Stripe raw
body before JSON parsing. It does not parse business fields, persist a receipt,
decide payment eligibility, emit HTTP 2xx, or trigger a downstream action.

## Public operation

```go
func Verify(rawBody []byte, signatureHeader string, secret []byte, nowUnix int64) Outcome
```

Inputs are owned by the caller. `rawBody` is treated as opaque bytes and is never
re-encoded. `signatureHeader` is parsed strictly for `t`, `v1` and legacy `v0`
segments. `secret` and `nowUnix` are supplied by the future application consumer;
this pure domain package reads no environment, filesystem, network or clock.

## Contract and error precedence

1. Empty secret or unavailable clock rejects without inspecting sensitive values.
2. Empty body rejects.
3. Header shape, timestamp presence/format/conflict, and v1 presence are checked.
4. Absolute timestamp distance must be at most 300 seconds; the tolerance is fixed.
5. HMAC-SHA256 is computed over `decimal(timestamp) + "." + rawBody` and compared
   with constant-time equality against every v1 candidate.
6. Only a matching v1 candidate returns `VERIFIED`; all other outcomes return
   `REJECTED` with a bounded safe reason.

Duplicate equal timestamps are harmless; contradictory timestamps reject. Multiple
v1 values support secret rotation. A v0-only header never verifies. Unknown header
keys, altered bytes, reserialized JSON and malformed values reject.

## Result, ownership and side effects

`Outcome` contains status, safe reason, policy version and timestamp. It never
contains raw body, header, secret, MAC or provider payload. The caller owns the
input memory and result. The operation is deterministic for the same inputs and
has no I/O, persistence, network call or logging side effect.

## Example

```go
outcome := stripesignature.Verify(rawBody, header, installationSecret, trustedNow.Unix())
if outcome.Status != stripesignature.Verified {
    return outcome // do not create receipt or claim paid
}
// A later consumer may parse/normalize only after this proof.
```

## Deliberate limits

The production secret adapter, trusted clock port, strict business parsing,
durable receipt/outbox, source read and financial proof are later application or
integration work. The retained CLI is an offline diagnostic and is not a public
webhook route.
