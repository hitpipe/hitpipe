# HIT-54 / NB-D1 — source to runtime path

Status: `CANDIDATE_MANUAL_LAB_INPUT`; this diagram describes the actual bounded
candidate and deliberately stops before transport, persistence or financial
acceptance.

```text
untrusted raw bytes + signature header
        |
        v
AuthenticateWebhook.Consumer
  | resolve installation secret through SecretResolver
  | obtain trusted Unix seconds through Clock
  | call StripeSignature.Verify(raw bytes, header, secret, now)
        |
        +--> REJECTED: bounded reason; no VerifiedWebhook handoff
        |
        +--> VERIFIED: VerifiedWebhook{installation_id,event_id,raw_body,timestamp}
                           |
                           v
                 future strict parser / whitelist consumer
                           |
                 [not implemented here: receipt, outbox, HTTP 2xx,
                  payment decision, durable retention]
```

| boundary | source and transformation | artifact/result | evidence |
| --- | --- | --- | --- |
| source | caller supplies opaque raw bytes, header, installation id | no normalization before proof | `services/intake/docs/module-contract.md` |
| dependency | application port resolves secret and clock | bounded dependency error | `internal/application/authenticatewebhook` tests |
| compile | Go module compiles domain, application, adapters and CLI | module/binary | `evidence/hit-51-c1-clean.log`, `evidence/hit-52-c2-local-clean.log` |
| transformation | HMAC over decimal timestamp + `.` + exact raw bytes; constant-time MAC compare | `Outcome` | `internal/domain/stripesignature` tests |
| artifact | diagnostic binary carries revision, dirty flag and build mode | dev/release executable | C1/C2 smoke and release inspection logs |
| runtime | CLI-only synthetic diagnostic composition | safe JSON/text status | C1 CLI log and C2 local log |

## Dev/release same-scenario observation

The same positive and tampered synthetic fixtures were run in dev and release
mode by the wrapper from outside the checkout. Both returned safe status-only
diagnostics; the release binary inspection showed the candidate revision and
`vcs.modified=false` at the clean build point. This observation is limited to
the fixture CLI. It is not evidence of live webhook ingress, production secret
storage or deployment.

## Reproduction and ownership

The application owns `SecretResolver` and `Clock`; the executed substitution
pair in C2 is `FileResolver + FixedClock` versus `MapSecrets + FixedClock`.
`SystemClock` is compiled and documented as an adapter, but it was not executed
in that DI experiment. `FileResolver` is exercised by application tests; the
CLI composition root uses a separate fixture resolver and fixture clock. The
domain imports only the standard library. Any future transport must consume the
existing `Consumer`/`VerifiedWebhook` contract rather than recreate signature
verification.
