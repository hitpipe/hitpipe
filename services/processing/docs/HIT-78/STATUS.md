# HIT-78 — partial implementation, not ready for handoff

Confirmed by Timofey in Codex: «Подтверждаю». Jira In Progress verified 2026-11-14.
Accepted base HIT-76: 15e684e1c3f78bb7851bc1d4335c9a9f883f5dc1.
PR45 remains open; actual main at start: ce771d37505e7770003d0796ae932556d06b7bab.
No migration bytes changed. No production integration or admission claim.

Implemented: strict NormalizedSignal 1.0 validation and RFC8785 digest, using
pinned github.com/gowebpki/jcs v1.0.2. Real Go race/vet logs for exact source
8ce998b are in parser-verification/commands.json. Runs are Go 1.27.2,
darwin/arm64, not Linux PostgreSQL integration tests. All three exit codes 0.

Unresolved contract question: architecture HP-MVP-A-015 requires Session.created
for the first activation boundary. NormalizedSignal.source_created_at has no
field description defining whether it is Event.created or Session.created.
No separate Session.created exists in that message. Late asynchronous payment
must not cause an old Session to pass the original activation boundary.
Lead clarification requested in HIT-78 comment 12146; no invented source read,
additional wire field, or permissive interpretation is implemented.

Pending: durable PostgreSQL admission; transaction/commit failure, concurrency,
repeat/conflict/reconnect/RLS/CAS tests; PR, CI, registered result/evidence and
handoff-preflight. This partial checkpoint does not satisfy all task criteria.
