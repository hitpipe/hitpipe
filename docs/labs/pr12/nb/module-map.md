# HIT-52 / NB-C2 — actual Intake module map

Status: `IMPLEMENTED_CANDIDATE`; source contracts remain Candidate/In Review.
Arrow `A -> B` means source code in A imports or compiles against B's public
contract; it does not claim runtime call direction or production deployment.

```text
cmd/intake-contractcheck -> internal/application/authenticatewebhook
internal/application/authenticatewebhook -> internal/domain/stripesignature
internal/application/authenticatewebhook <- internal/adapters/fixtures/signingsecrets
internal/application/authenticatewebhook <- internal/adapters/clock/system
internal/application/authenticatewebhook_test -> adapters + fixed test doubles
```

| Module | Responsibility | Public contract | Path | Dependencies | Status | Author |
| --- | --- | --- | --- | --- | --- | --- |
| StripeSignature | exact raw-body HMAC/timestamp rule | `Verify(...) Outcome` | `services/intake/internal/domain/stripesignature` | stdlib only | works | Никита Будыкин |
| AuthenticateWebhook | secret/clock resolution then verification handoff | `Consumer.Authenticate(...) Result` | `services/intake/internal/application/authenticatewebhook` | domain + owned ports | works | Никита Будыкин |
| FixtureSecretResolver | synthetic installation-scoped secret lookup | `Resolve(ctx, id)` | `services/intake/internal/adapters/fixtures/signingsecrets` | file/JSON | works for lab | Никита Будыкин |
| SystemClock | system Unix second adapter | `NowUnix(ctx)` | `services/intake/internal/adapters/clock/system` | time | works for lab | Никита Будыкин |
| Durable receipt / HTTP route | persistence/transport/2xx | future port | not present in this slice | future service-owned adapters | plan | Lead/future owner |

The application package owns the ports. Concrete adapters are not imported by the
domain, and domain/application do not read env, network, database or framework
state. The CLI is an offline diagnostic composition root; it is not a production
webhook endpoint.

Actual import check and build evidence: `docs/labs/pr12/nb/evidence/hit-51-c1-clean.log`
and the C2 local check log. Future composition wiring must preserve this map.
