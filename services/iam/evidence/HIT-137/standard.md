# HIT-137. Стандартная проверка IAM на PostgreSQL 18.7

Проверенный коммит `28b9e6a8beecb2e9d1952b0af363c33c687e5d4e`. Основа `main` `ce771d37505e7770003d0796ae932556d06b7bab`.
Закреплённые входы [PR #51](https://github.com/hitpipe/hitpipe/pull/51) `284fa8a5cfe8092bc65c508de6c067b31fd7df94` · [PR #55](https://github.com/hitpipe/hitpipe/pull/55) `60013536b69b4b8d7f99e7bddf714e6432f09a11` · [PR #56](https://github.com/hitpipe/hitpipe/pull/56) `a46d9d40169105356e61200a2a2b0cec17e5efb3`.

Изолированный Ubuntu KVM-стенд, `go version go1.27.2 linux/amd64`. Сервер вернул `SHOW server_version` = `18.7`.
Закреплённый образ `postgres:18.7-alpine@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73`. Тестовая база создавалась заново, контейнер и сеть удалены.
Контрольная сумма формального IAM SQL и миграции `604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7` совпала с принятой.

Обычный запуск с кодом завершения 0. Время UTC `2026-11-21T05:28:19Z` — `2026-11-21T05:29:15Z`.

```text
$ python3 services/iam/tools/check_postgres_session_lookup.py --go /work/hit106/go/bin/go --evidence-dir /work/hit137/28b9e6a8be.Lc5hm3gz/standard
$ go test -p 1 -tags=integration -race -count=1 -v ./...
$ go vet -p 1 -tags=integration ./...
$ /work/hit106/go/bin/go test -race -count=1 ./...
$ /work/hit106/go/bin/go vet ./...
```

Все 11 шагов стандартной проверки завершились кодом 0. Установка вернула `installed`, повторная миграция вернула `already_installed` с тем же хешем SQL.
В проверке роли PostgreSQL зафиксирован `current_user=hp_iam_migrator` при `session_user=hp_hit81_migration`.
В журнале подтверждены 17 проверок схемы, владения и RLS и 9 проверок чтения сессий на реальной базе.

```text
--- PASS: TestIAMSchemaMigrationRepeatOnFreshConnection (0.79s)
--- PASS: TestIAMSchemaMigrationAtomicDDLFailure (0.61s)
--- PASS: TestIAMSchemaMigrationRefusesInvalidExistingState (3.09s)
--- PASS: TestIAMSchemaMigrationRefusesRuntimeAndOpenTransaction (0.20s)
--- PASS: TestIAMSchemaMigrationRejectsUnsafeDefaultPrivileges (0.20s)
--- PASS: TestIAMSchemaMigrationConcurrentInstall (0.98s)
--- PASS: TestIAMSchemaRoleSeparationAndOwnership (0.14s)
--- PASS: TestIAMSchemaRuntimePrivilegeMatrix (0.10s)
--- PASS: TestIAMSchemaTenantRLSReadAndWriteIsolation (1.39s)
--- PASS: TestIAMSchemaOwnerCannotBypassForcedRLS (0.13s)
--- PASS: TestIAMSchemaSetLocalDoesNotLeakAcrossTransactions (0.17s)
--- PASS: TestIAMSchemaGlobalTablesAreIAMAuthorityWithoutTenantFiction (0.28s)
--- PASS: TestIAMSchemaJobIndexIsWorkerAndPurgeOnly (0.20s)
--- PASS: TestIAMSchemaGlobalUniquenessAndForeignKeys (0.14s)
--- PASS: TestIAMSchemaSessionAndRevocationConstraints (0.21s)
--- PASS: TestIAMSchemaAppendOnlyAuditAndCommandReceipts (0.29s)
--- PASS: TestIAMSchemaCommitIsDurableAndFailedTransactionLeavesNoPartialWrite (0.20s)
--- PASS: TestPostgresExactSnapshotAndExistingConsumer (0.05s)
--- PASS: TestPostgresMissingSessionIsNotFound (0.03s)
--- PASS: TestPostgresNextReadObservesRevocation (0.05s)
--- PASS: TestPostgresReadsNeverModifyOrRenewSession (0.09s)
--- PASS: TestPostgresErrorsAreSanitizedAndNeverValid (0.09s)
--- PASS: TestPostgresInFlightCancellationFailsClosed (0.17s)
--- PASS: TestPostgresInternalDeadlineBoundsBlockedRead (2.05s)
--- PASS: TestPostgresConcurrentReadsReturnIndependentValues (0.17s)
--- PASS: TestPostgresCancelledContextAndClosedPool (0.20s)
```

Блок `unit` — обычные Go-тесты без требования живой базы. Доказательства реального PostgreSQL находятся в `postgres-race`.
Опциональный сбор выборок `TestHIT82LatencySamples` штатно пропущен в стандартном режиме. Отдельная команда включила его в прогоне измерений.

[Протокол стандартного запуска](standard/verification.json) · [Полный PostgreSQL-журнал](standard/postgres-race.log) · [Очистка](cleanup.log)
