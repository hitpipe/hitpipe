# HIT-42 requirement refinement

Status: CANDIDATE / In Review. Human review, merge, CI, and independent scenario check
are pending. This is not an accepted Architecture or Product contract.

## Dependency state

R1 is referenced as the candidate source by Jira HIT-41. The reported local commit
`0fb84594b18d1f8855f38761331b2ce79d16a97e` is not present in the connected remote
repository, so this refinement is anchored to the published baseline
`94b78199c202a31bcb63326ae7f2a4748aa29360` and records the dependency gap instead of
claiming to consume an unavailable remote artifact.

## Before / after

Before: a target readback comparison could be interpreted as a boolean success check,
without a visible distinction between a missing field, a provider retrieval failure,
malformed bytes, and a value mismatch.

After: `TargetReadback` returns four explicit outcomes. `unknown` and `incomplete`
are non-success outcomes with precedence over any apparent match; malformed or
non-authoritative evidence cannot be converted into a successful confirmation. Exact
set comparison and deterministic Unicode/byte handling are part of the local module
contract.

## Rationale

The refinement closes a false-confirmation boundary while keeping provider access in
adapters and comparison policy in the reusable domain module. It does not alter the
canonical DeliveryIntent, ObservedTaskAttributes, TaskObservation, SQL, or trust and
retention semantics.

## Traceability

- Task: HIT-42
- Feature: HIT-14
- Component: Delivery - Destination Integrations
- Branch: `work/HIT-42-ds-readback-contract`
- Baseline: `94b78199c202a31bcb63326ae7f2a4748aa29360`
- Module contract: `services/delivery/internal/domain/readback/contract.md`
- Human gates: independent blind scenario check, review, merge, and CI remain pending.
