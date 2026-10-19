# HIT-49 / NB-R1 — StripeSignature security baseline

Status: prepared from pinned Candidate sources; empirical interview and live
provider validation are `NOT_CONDUCTED`/`NOT_RUN`.

| Required question | Concrete answer for this module | Evidence/status |
| --- | --- | --- |
| What data/resources are protected? | Installation-scoped signing secret, exact raw request bytes, signature/timestamp, provider account/mode, and the downstream admission boundary. | HP-BP-002 lines 14, 28, 48; source content checked; candidate |
| Where is untrusted input? | Raw HTTP body, `Stripe-Signature` header and opaque endpoint route are external inputs. JSON parsing must occur after signature verification. | HP-BP-002 T/001; candidate |
| Who can access sensitive actions? | The Intake secret adapter resolves an installation-scoped secret; the application consumer receives narrow secret/clock ports; pure domain code has no filesystem, network, env or secret-store access. | `IMPLEMENTATION_CONVENTION.md`; design constraint |
| What realistic harm follows? | A bypass or raw-body substitution can produce a false verified event and downstream receipt/action; secret or raw payload logging can leak provider/customer data; timestamp/replay bypass weakens freshness. | HP-BP-002 T/001/T/003; threat analysis |
| Where are secrets, and what must not be logged? | Production secrets belong to the Intake-owned secure adapter/store. The lab uses synthetic fixtures only. Never log secret, raw body, full signature header, provider credential, customer payload or claim `paid`/`accepted` from verification alone. | HP-BP-002 T/001/T/003; `HIT-51` limitations |

## Derived controls

- Compare MACs with a constant-time operation.
- Use `timestamp + "." + raw body bytes`; do not parse and reserialize JSON first.
- Require `v1`, reject v0-only and contradictory timestamps, and apply absolute
  300-second tolerance without an opt-out parameter.
- Return a safe machine reason; include module version but not sensitive values.
- Keep secret/clock lookup behind application-owned ports and fail closed on
  unavailable dependency.
- Keep verification separate from durable receipt, financial proof and HTTP 2xx.

## Lab data handling

Fixtures must be synthetic, deterministic and committed under the owned module's
`testdata`. Do not copy Stripe customer payloads, real endpoint secrets, cookies,
private account identifiers or local credential files. Before human screenshots,
inspect terminal output for accidental secrets and redact only irrelevant PII; do
not hide a material result.
