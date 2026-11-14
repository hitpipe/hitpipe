# Processing PostgreSQL checks — HIT-76

From repository root, with a running **local** Docker daemon and Python 3.10+:

```sh
docker pull docker.io/library/postgres:18.6-bookworm@sha256:afc7e2d441324c0388fa80c3d24f733b4194a4eb7f47dd8ee2b08eb1a24a647c
python3 services/processing/scripts/postgres/verify.py --output /tmp/hit76-check-UNIQUE
```

Output must be a new directory. The runner checks the migration SHA-256, records
its exact commands/stdin/stdout/stderr, timestamps, source SHA, dirty-tree status,
script hash, image identity, server settings and SQLSTATEs. `summary.json` is
written only after all checks and cleanup succeed. Nonzero exit means failure;
inspect the preserved logs. Expected rejection commands exit 3 and must match the
specified SQLSTATE, so a syntax/connection error cannot masquerade as a passed test.

The runner creates exactly one uniquely named/labelled container with network
`none`, no published ports, a read-only root, temporary PGDATA/socket/tmp mounts,
and only the migrations directory mounted read-only. It creates a fresh dedicated
Processing database and removes its container and volumes in `finally`. There is
no DSN option and it never connects to an existing server or database. A nonlocal
Docker context or DOCKER_HOST override is rejected. Initial image download needs
network; the PostgreSQL container does not. No provider/service keys or user data
are used. All identities and payload references are synthetic UUIDs.

PostgreSQL must report server_version_num 180006, fsync/full_page_writes/
synchronous_commit on. The pinned image is PostgreSQL18.6, with ARM64 child digest
sha256:67ba68a22dc7d5af3627b7e0f33d10bbefad07562ff9703b3a6c2de247649129
(AMD64: sha256:9551dd5bf356409a21b31248f49b33bba9bb1877365f3a7e933aecd9af3cf52d).
Image authority: project-os@fedbe40bb597e25f66aeb1172af3ff7459c81b40,
contracts/hitpipe/mvp-2026-10-31/formal/stack-mvp.json. No Go driver is introduced;
psql from the same image executes real PostgreSQL SQL. Existing Go checks use
Go1.27.2 (`go test -race -count=1 ./...` from services/processing).

The test container uses local Unix-socket trust exclusively for synthetic tests,
not production authentication. Temporary memory storage proves SQL/reconnection
semantics, not power-loss durability, deployment capacity or backup/restore.

Checks cover application/worker connections and NO SUPERUSER/BYPASSRLS/ownership/
owner membership, all 12 tenant tables' forced RLS, no-scope visibility, SET LOCAL
scope/reset, denied foreign/no-scope writes, immutable accepted config privileges,
Session identity uniqueness, one active SINGLE operation, every pinned case field,
first activation, episode identity/clock and attempt monotonicity/bounds, atomic
rollback of case+outbox and committed state after a new connection. Every SQL call
opens a new connection except the explicit transaction-scope reset check.

`job_index` intentionally exposes only worker job/workspace/due/claim metadata;
application access is denied. `schema_version` is non-tenant metadata. Neither is
misrepresented as tenant content subject to the 12-table RLS test.

Scope: Processing schema and isolation only. No business event ingestion, HTTP,
activation, IAM scope/epoch authority, file payload store or Stripe→ClickUp E2E.
The caller must establish workspace authority before setting hp.workspace_id;
this database GUC itself is not proof of that authority. Independent review and
human acceptance remain separate.
