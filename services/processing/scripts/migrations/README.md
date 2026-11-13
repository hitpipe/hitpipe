# Exact Processing schema, version 1

`001_processing.sql` is a byte-for-byte copy of:

https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/processing.sql

SHA-256: a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93.
Do not edit that file to accommodate a test. Any material conflict needs the
existing contract/context process. `verify.py` verifies these exact bytes.

Provision roles once using `roles.sql` as the administrator of a new dedicated
Processing test cluster. It creates a NOLOGIN schema owner, a separate NOINHERIT
migrator with membership in that owner, and nonowner application/worker/purge roles.
None is SUPERUSER, BYPASSRLS, CREATEDB, CREATEROLE or REPLICATION. The application
and worker do not inherit migrator/owner/purge privileges. No credentials are
included. Authentication provisioning for live use belongs to its separate scope.

Create the dedicated database owned by hp_processing_owner; revoke PUBLIC access
and grant CONNECT to the explicit Processing roles. As hp_processing_migrator run:

```sh
psql -X -v ON_ERROR_STOP=1 -f services/processing/scripts/migrations/apply.sql
```

Use only an explicitly authorized test connection. For an automated isolated run
with no connection parameters, see ../postgres/README.md. Do not point tests at
Project OS, another service or existing user databases.

`apply.sql` SET ROLEs to the owner, takes a session advisory lock, checks version,
and includes the unchanged source (its own BEGIN/COMMIT). A second execution emits
ALREADY_INSTALLED and preserves installed_at, without recreating tables. Unknown
version, partial schema or SQL failure stops; the tool does not silently repair it.
Session/connection exit releases the lock even on error. Role bootstrap is separate
and deliberately fails on existing role names; do not rerun it on shared clusters.
