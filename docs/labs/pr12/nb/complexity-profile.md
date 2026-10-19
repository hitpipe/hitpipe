# HIT-52 / NB-C2 — complexity profile

Status: two directions selected for future engineering investigation. These are
not claims of measured bottlenecks.

## 1. Data handling: raw bytes versus parsed/normalized body

- Object: `services/intake/internal/domain/stripesignature` and the handoff to a
  future strict parser.
- Question: how can exact bytes remain available for proof while later parsing and
  whitelist normalization avoid copying sensitive/raw payload beyond the allowed
  boundary?
- Current readiness: the policy accepts `[]byte` and does not parse or log it;
  typed handoff is present. Durable retention, size limits and normalized envelope
  are not implemented in this slice.
- Candidate check: add a bounded fixture that differs only by JSON whitespace or
  non-UTF-8 bytes, then inspect ownership/copies and safe diagnostics.

## 2. Inter-component trust: untrusted ingress and secret/clock failure

- Object: `AuthenticateWebhook` ports and the future transport/receipt consumer.
- Question: how should unknown installation, unavailable secret, clock uncertainty,
  stale timestamps and repeat delivery remain fail-closed without leaking provider
  details or creating a false durable action?
- Current readiness: narrow ports, bounded reasons and repeat-event signature test
  are implemented; transport, durable receipt and revoke/delete barriers are not.
- Candidate check: run failure-injection doubles for each port, then verify that no
  `VerifiedWebhook` is returned and no durable/2xx path is reachable.

The chosen directions are system/data and trust/inter-component boundaries, not
language or library names. Further measurement and real provider credentials are
outside the manual lab scope.
