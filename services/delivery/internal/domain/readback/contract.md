# TargetReadback module contract

Status: candidate refinement for HIT-42, MANUAL_LAB_PR12. This document is local to
`services/delivery`; it does not change the canonical wire schema, SQL, trust, or
retention rules.

## Inputs and ownership

The application consumer owns the comparison port. The comparator receives:

- an expected target attribute set, represented as UTF-8 text values with their
  original byte representation retained for normalization checks;
- one readback result, which is either retrieved evidence, an explicitly incomplete
  response, or an unknown retrieval failure.

The comparator is pure: it performs no network, database, retry, logging, or provider
write. Adapters own retrieval and map provider failures into the readback result.

## Result precedence

The result is one of `match`, `mismatch`, `incomplete`, or `unknown`.

1. `unknown` wins whenever retrieval failed or the adapter cannot establish whether
   the returned evidence is authoritative. The consumer must not treat it as success
   and must not retry implicitly.
2. `incomplete` wins when retrieval succeeded but any expected key is absent or the
   provider explicitly reports a partial readback. Partial evidence is never a match.
3. `mismatch` applies when the evidence is complete but a normalized expected value
   differs, or when an unexpected returned key is present under exact-set mode.
4. `match` requires complete evidence and byte-stable equality for the normalized
   expected set.

## Normalization and boundaries

Normalization is deterministic and documented by the implementation: UTF-8 validity is
checked first; text is compared after the agreed Unicode normalization; bytes are not
silently decoded or lossy-converted. A malformed or non-UTF-8 value is `unknown`,
because the comparator cannot establish an authoritative comparison.

The comparator does not decide provider ownership, retention, authorization, or retry
policy. It returns structured evidence and a reason code for the application consumer.
No secrets or customer payloads may be included in diagnostics.

## Verification contract (Given / When / Then)

- Given identical complete expected and observed sets, when compared, then result is
  `match`.
- Given one differing value, when compared, then result is `mismatch`.
- Given a missing expected key or partial marker, when compared, then result is
  `incomplete`.
- Given retrieval failure or malformed evidence, when compared, then result is
  `unknown` and no provider write or implicit retry is requested.
