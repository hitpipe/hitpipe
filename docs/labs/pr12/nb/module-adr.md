# HIT-52 / NB-C2 — personal implementation ADR

Status: `CANDIDATE_MANUAL_LAB_INPUT`. Author: Никита Будыкин. This is a personal
bounded implementation decision, not a new canonical `HP-ADR-*` and not a phase
contract change.

## Context

The signature rule needs an installation-scoped secret and trusted time, but the
pure policy must remain reusable and testable without a provider, filesystem or
clock. The consumer must not confuse verification with durable acceptance.

## Options considered

1. **Read environment and `time.Now` inside the domain function.** Lowest initial
   wiring cost, but couples policy to secrets/clock, weakens deterministic tests,
   and makes scope/error ownership ambiguous. Rejected.
2. **Pass concrete file and system implementations directly to the domain.**
   Makes diagnostics easy, but couples domain/application to adapters and prevents
   a clean replacement or secure-store migration. Rejected.
3. **Application-owned `SecretResolver` and `Clock` ports, selected at composition,
   with file/system adapters and deterministic doubles.** Adds two small port types
   and explicit wiring, but keeps the consumer stable and preserves fail-closed
   dependency errors. Chosen.

## Decision and consequences

`authenticatewebhook.Consumer` owns both narrow interfaces and calls the pure
`StripeSignature` policy only after successful resolution. The executed substitution
pair is `FileResolver + FixedClock` and `MapSecrets + FixedClock`; `SystemClock` is
compiled/documented but was not executed in this experiment. A secure production
store and trusted service clock remain future adapters. A verified result is a typed handoff only; no receipt,
2xx or financial admission is inferred.

Cost: one extra composition boundary, copies/ownership of raw bytes at the caller,
and a future integration step for real secure storage/clock. Benefit: unchanged
consumer tests can exercise two dependency implementations and all errors without
provider credentials.

## Verification

`TestAuthenticateWebhookContractWithFixtureAndDouble` runs normal/error/boundary
scenarios through the same consumer with file-backed and in-memory resolvers;
`TestAuthenticateWebhookRejectsRepeatWithoutSignature` proves event ID is not a
signature bypass; dependency failures are safe and bounded. Local/CI commands and
full SHA are recorded in the evidence index/handoff.
