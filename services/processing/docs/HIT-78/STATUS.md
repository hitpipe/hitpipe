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

## Independent storage checkpoint (2026-11-14)

Private adapter primitive `record` stages inbox, unique identity and minimal safety
history in a caller-owned transaction. It never commits or acknowledges a message.
There is no exported admission API or permissive authority/boundary implementation.
Its explicit preconditions (validated signal, established binding/authority,
locked accepted operation/config, verified activation boundary) remain the future
application layer's responsibility. Tests use explicitly synthetic accepted state.

Source tested: 8d4f127730c071ad67b8c70efc998624b03ac2c4, clean tree.
`storage-verification-03/commands.json` records exact argv, cwd, UTC times, source,
exit codes and stdout/stderr digests. `digests.json` covers the exported files.

- Reused full HIT-76 real PostgreSQL schema/RLS/constraint/CAS regression runner.
- Go 1.27.2 darwin/arm64: `go test -race -count=1 -v ./...`, `go vet ./...`: exit 0.
  PostgreSQL tests intentionally skip on the host; these skips are not SQL passes.
- Cross-compiled Go 1.27.2 linux/arm64 test binary (CGO_ENABLED=0, no race)
  executed against owned network-none PostgreSQL 18.6 container using private
  Unix socket and non-superuser hp_processing_app. Test binary digest recorded.
- Real storage tests: atomic commit; same message/digest replay; different digest
  conflict without overwrite; second event/message same case; concurrent same and
  distinct messages; different Session/workspace/account/livemode isolation;
  pinned owner/config/operation and revision survive new connections; rollback,
  deferred-FK commit failure, failure after case insert, foreign-workspace RLS.
- All required commands exit 0. Expected SQL rejections and absent-container
  cleanup probe have recorded nonzero codes, checked by the runner.
- Migration remains byte-identical to accepted SQL, SHA256
  a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93.
- Containers removed; no existing database or service used.

Earlier runs are retained locally under research/HIT-78-20261114:
01 failed copying a binary into the read-only container (not an application test
failure); 02 passed after a read-only test-directory mount; 03 adds account/mode
isolation coverage. They are not relabelled as the final source's results.

Lead question repeated verbatim by explicit participant request in comment 12149.
Pending: agreed Session.created source; application admission/authority/binding
checks and post-commit receipt boundary; end-to-end admission tests; PR/CI,
registered result/evidence, handoff-preflight. No full-task PASS/READY claim.
