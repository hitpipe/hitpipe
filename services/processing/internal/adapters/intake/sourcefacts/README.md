# SourceFacts → financial facts (HIT-72)

`Decode(bytes, Expected, localPolicy)` transforms one already received message into
an owned `Projection`. `Snapshot` composes with the existing `SourceFactsReader`
port / `EvaluatePaidOrder`; `Provenance` separately retains facts_id, digest and
exact source times. An error returns an empty projection and a constant diagnostic.
No source body/identifier is included in error text.

`schema_generated.go` is a generated view, not an independent wire contract:

- source: hitpipe/project-os@fedbe40bb597e25f66aeb1172af3ff7459c81b40,
  contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json;
- file SHA-256: a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e;
- schema: components.schemas.SourceFacts, all 28 required fields;
- generation (from services/processing, Go 1.27.2 on PATH):
  `python3 tools/sourcefacts/generate.py /path/to/exact/mvp-openapi.json`.

The generator verifies the entire OAS digest and fails on unsupported schema
shapes. Runtime validation rejects missing required properties, unknown/noncanonical
names, duplicates, extra JSON, invalid scalar shape, enum, format, bounds and null
where forbidden. JSON integer representations such as 1.0 and 1e0 are normalized
exactly without float rounding. HP-MVP-A-004 additionally limits every wire integer
to the safe JSON range. Known zero/false survive; nullable unknowns remain nil.

Expected case/session/account/connection/test-live must match the message. Expected
source scope must also match the caller's local Policy. Policy config/schema/evaluator
pins are local inputs, not wire fields or remote attestations. Financial rules and
application consumer are unchanged. Provenance digest is preserved, not verified.

Run `go test -race -count=1 ./...` and `go vet ./...` from services/processing.
Tests use synthetic messages and a test-only reader; the adapter itself has no reader,
network, persistence, provider client, wall clock, retry or dispatch capability.

This slice does not establish sender authenticity, authority, workspace/epoch/start
guards, a working Intake endpoint or end-to-end integration. A caller must supply
those trusted boundaries before any production wiring. VALID remains financial
knowledge only, with no permission to act. No live server is connected here.
