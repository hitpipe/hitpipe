# Intake laboratory module

`internal/application/authenticatewebhook` is the retained application consumer
for the `StripeSignature` domain policy. It resolves an installation-scoped secret
and trusted clock through narrow ports, verifies exact raw bytes, and returns a
typed `VerifiedWebhook` only after verification.

This bounded slice intentionally has no public webhook route, no HTTP 2xx, no
durable receipt/outbox, no payment decision and no downstream business action.
`VERIFIED` is a cryptographic result, not `ACCEPTED` or `paid`. A real receipt port
and transport adapter are future `Intake & Source Connectors` work.

The file secret resolver and system clock are diagnostic adapters. Tests use the
same consumer with a file-backed fixture and in-memory/fixed-clock doubles; the
doubles do not claim that a production secure store or clock service was verified.
