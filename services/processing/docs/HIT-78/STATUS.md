# HIT-78 — local durable admission, prepared for independent review

Codex по поручению Тимофея Патрушева / Tim1hik. Human consent «Подтверждаю»
and Jira In Progress verified. This document describes implementation/testing,
not independent acceptance or Project OS readiness.

Accepted base HIT-76: `15e684e1c3f78bb7851bc1d4335c9a9f883f5dc1` (PR45 OPEN).
Observed main: `ce771d37505e7770003d0796ae932556d06b7bab` does not contain PR45.
This is a dependent branch targeting `work/HIT-76-processing-postgres`; no merge,
retarget, force push or change to the accepted SQL is performed.

## Resolved source-time decision

[Lead decision HIT-78 comment 12171](https://hitpipe.atlassian.net/browse/HIT-78?focusedCommentId=12171)
and [Intake clarification HIT-73 comment 12172](https://hitpipe.atlassian.net/browse/HIT-73?focusedCommentId=12172)
were read with matching author/updateAuthor Dmitriy Karacheban
`712020:98d613f7-2ada-48c2-9ef5-04f0a11f63b1`. Exact HTML/metadata in
`lead-decision.json`. This resolves the earlier question, not implementation acceptance.

`source_created_at` is Session.created from signed Event.data.object.created,
not Event.created or receipt/current time. Processing accepts `Session.created >=
operations.first_activation_at`. No new wire field, readSourceFacts call or SQL column.
The original Session creation time is neither overwritten nor synthesized; the
schema does not store this timestamp in case_identity. Each input is checked against
first_activation_at; replay does not alter that boundary or any case pin.

| Real PostgreSQL test | Expected | Actual |
|---|---|---|
| Session before first activation | reject, no rows/receipt | passed |
| Session exactly at first activation | admit if other checks pass | passed |
| Session after first activation | admit if other checks pass | passed |
| Old Session, late async_payment_succeeded | reject despite late receipt/new event | passed |

## Composition and transaction boundary

Application entry: `admission.Admit(ctx, raw, establishedContext, store)`.
Mandatory EstablishedContext comes from previously verified caller authority,
installed source binding and accepted operation revision. No defaults/resolver,
HTTP handler, activation, IAM/Intake authentication or financial decision is added.
Tests explicitly supply synthetic established context and previously accepted DB state.
`AuthorityVerified` is a trusted internal composition assertion, never a request field.
The context cannot be reconstructed from NormalizedSignal; unknown/absent values fail.

`postgres.Store{DB: explicitly configured application-role connection source}`
checks context again, uses READ COMMITTED + transaction-local workspace RLS,
locks the operation against lifecycle/CAS changes, checks ACTIVE state/revision,
selected accepted config and original activation boundary. Config/owner come from DB.
Strict raw validation occurs in the application entry before invoking the store;
the Store port accepts an already validated Signal, not raw external input.

A local transaction stages inbox, unique case identity and minimal safety history.
A committed message ID+digest returns its original receipt (including after pause);
different digest conflicts. New messages for a known Session retain case/config/owner
and revision. Receipt contains message/disposition/record ID; optional revision omitted
because accepted inbox does not retain original result revision. No result is returned
on commit error; a lost commit acknowledgment is an error/unknown outcome, not APPLIED.
A retry resolves through the durable inbox. No retries or external effects here.

SQL stores only opaque identifiers, digest and bounded safety codes; no normalized
payload JSON, Stripe raw payload, business text or history contents are persisted/logged.

## Verification, exact current code

Tested source: `2a2a4824ad387c243844f3344e1ee629f70c2629`, clean tree.
Full argv/cwd/time/environment/exit/stdout/stderr/hash records:
`admission-verification-06/commands.json`; final scope in `summary.json`;
file digests in `digests.json`, Linux test-binary digest in `test-binary.json`.

From repository root (requires local Docker):

```sh
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_admission_storage.py --go /absolute/path/to/pinned/go --output /new/output/directory
```

Runner reuses the entire HIT-76 isolated PostgreSQL lifecycle and SQL regressions.
It accepts no existing DB/DSN, exposes no network ports, uses pinned PostgreSQL18.6
Linux/arm64 image and private socket, application role without SUPERUSER/BYPASSRLS,
fsync/full_page_writes/synchronous_commit ON, and removes its container in finally.
Migration SHA256 remains `a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93`.

Go1.27.2 darwin/arm64: module-wide `go test -race -count=1 -v ./...` and `go vet ./...`
exit 0. Host PostgreSQL tests skip explicitly; they are separately executed (not
skipped) as a Go1.27.2 linux/arm64 CGO=0 test binary inside real PG container.
The Linux DB run is not a Go race run. All required commands exit 0; expected
SQLSTATE rejection commands and missing-container cleanup probe have verified
nonzero codes. No claim of crash/power-loss or production deployment qualification.

| DoD | Concrete verification |
|---|---|
| Reuse / scope | source-audit.json; unchanged migration hash; base PR45 explicit |
| Strict schema/digest | TestCanonicalAndJCS, TestRejectInvalidSignal; full entry rejects malformed wire/missing/invalid time with no rows |
| Atomicity/no premature APPLIED | TestAdmissionCommitFailureNeverAcknowledges, TestRecordRollbackAndCommitFailure; real deferred constraint commit failure |
| Replay/conflict | TestAdmissionReplayAfterLifecycleAndNewConnection, TestAdmissionConcurrentAndDigestConflict, TestRecordCommitRepeatConflictAndReconnect |
| One concurrent Session identity | TestAdmissionConcurrentAndDigestConflict, TestRecordConcurrentIdentityAndMessage; distinct Session/workspace/account/mode isolation |
| Context/local state/boundary | TestAdmissionRejectsContextAndLocalState, TestAdmissionActivationBoundary, TestAdmissionTwoSignalsOnePinnedCase |
| New connection/RLS/CAS/regressions | reconnect tests, stale revision test, full HIT-76 SQL regressions + module-wide race/vet |
| Explicit limitations | composition and limitations in this file and result JSON |

Earlier parser/storage logs keep their original SHAs. Local run04 failed a test
fixture compilation; run05 exposed unkeyed cross-package receipt literals via vet.
Both are retained in research/HIT-78-20261114 and superseded by successful run06,
not relabelled as passes. Earlier storage runs01–03 are also historical.

## Remaining human/server steps

Register materialized external result/evidence; successful handoff-preflight is
required before inviting In Review. Independent Reviewer and human acceptance
remain pending. No runtime IAM/Intake/Stripe, financial evaluation, outbox/Delivery
or external write is implemented or claimed. Lead integrates PR45 and this dependent
PR through ordinary review/protected merge; main integration/CI must then be checked.
