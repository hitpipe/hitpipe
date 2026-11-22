# HIT-148 — проверка доверенного снимка

Код Processing: `0eba954c24652655cfc661fc63628b2097ebd33e`. Издатель: `c715a079ab57d4c9e7b56862f7f2584c792b74b0`. Стандартный источник PostgreSQL 18.7 закреплён с `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`. Полные команды, cwd, UTC, статусы, версии и сырые потоки в `inventory-raw.tar.gz`, SHA-256 `9de14e2f05e549b04d836400368d5d24cc7b58a97997934cde7b4eb120bc19bd`. Ниже фактическая выборка, включая полный журнал межпроцессного теста. Первый промежуточный прогон отклонил отличавшийся суффиксом текст PostgreSQL server_version при фактической версии 18.7. В итоговом прогоне сравнивается server_version_num=180007, исторический запуск сохранён отдельно.

Изменение области, версии, получателя и подлинности отклоняются. Запись владельца прошла настоящую процедуру IAM в PostgreSQL, подписана выверенным издателем и прочитана отдельным бинарником Processing. Единственное подтверждение от Processing — чтение. Прогон исходного теста HIT-154 использует его синтетический подчинённый адаптер, поэтому применение и открытие в том тесте относятся только к стенду HIT-154. Остальные тесты — регрессии прежнего Processing.

## postgres-version

```json
{"label": "postgres-version", "argv": ["docker", "exec", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e", "postgres", "--version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:32.641503+00:00", "finished_at": "2026-11-22T19:50:32.710239+00:00", "exit_code": 0, "stdout": {"file": "008-postgres-version.stdout", "bytes": 52, "sha256": "605b026879b9a291e0d851478a6f4dc23353423fa6f7035ce9e58a1e3a2e9e7a"}, "stderr": {"file": "008-postgres-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

## go-version

```json
{"label": "go-version", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:33.279174+00:00", "finished_at": "2026-11-22T19:50:33.288372+00:00", "exit_code": 0, "stdout": {"file": "018-go-version.stdout", "bytes": 33, "sha256": "94e911f9fbf3c118f6a0573ab3a13fa546ad13e89ba40cf53df77e468509c92d"}, "stderr": {"file": "018-go-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
go version go1.27.2 darwin/arm64
```

## go-race

```json
{"label": "go-race", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "test", "-race", "-count=1", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:33.289049+00:00", "finished_at": "2026-11-22T19:50:38.095370+00:00", "exit_code": 0, "stdout": {"file": "019-go-race.stdout", "bytes": 3384, "sha256": "7ad82014d8fe97141d485bef2366984f61cbe3a99f95802548587e9ac910d97f"}, "stderr": {"file": "019-go-race.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.806s
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.902s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.953s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.911s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.779s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.698s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.666s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	2.207s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.664s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.718s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.187s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.180s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.147s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.144s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.131s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.153s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.317s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.298s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.172s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.246s
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

## go-vet

```json
{"label": "go-vet", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "vet", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:38.096204+00:00", "finished_at": "2026-11-22T19:50:38.268056+00:00", "exit_code": 0, "stdout": {"file": "020-go-vet.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "020-go-vet.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing

```json
{"label": "build-processing", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=0eba954c24652655cfc661fc63628b2097ebd33e", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-o4cjwf5r/processing", "./cmd/processing"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:38.330404+00:00", "finished_at": "2026-11-22T19:50:38.980202+00:00", "exit_code": 0, "stdout": {"file": "022-build-processing.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "022-build-processing.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing-purge

```json
{"label": "build-processing-purge", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=0eba954c24652655cfc661fc63628b2097ebd33e", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-o4cjwf5r/processing-purge", "./cmd/processing-purge"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:38.988081+00:00", "finished_at": "2026-11-22T19:50:39.281736+00:00", "exit_code": 0, "stdout": {"file": "023-build-processing-purge.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "023-build-processing-purge.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## recovery-process-tests

```json
{"label": "recovery-process-tests", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT128_RECOVERY_STAND=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=110s", "-test.run=^TestProcessingRecoveryOwnedStand$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:39.979593+00:00", "finished_at": "2026-11-22T19:50:44.213657+00:00", "exit_code": 0, "stdout": {"file": "025-recovery-process-tests.stdout", "bytes": 7627, "sha256": "accd2018c4c381e7015cd880f1db132c8b142940ccb0a9f4251c2845cd3d7280"}, "stderr": {"file": "025-recovery-process-tests.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
=== RUN   TestProcessingRecoveryOwnedStand
--- PASS: TestProcessingRecoveryOwnedStand (4.14s)
PASS
```

## predecessor-process-tests

```json
{"label": "predecessor-process-tests", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=145s", "-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:50:44.215620+00:00", "finished_at": "2026-11-22T19:51:42.550253+00:00", "exit_code": 0, "stdout": {"file": "026-predecessor-process-tests.stdout", "bytes": 5616, "sha256": "d9e505c19bed74ff87a3eb64c7fbb78bc249e3f9f61be00a14ae0b138cbbbb89"}, "stderr": {"file": "026-predecessor-process-tests.stderr", "bytes": 216, "sha256": "db9cbd09176dab507a0b00a662a3417c80839f2be2cf25d9505147db443f1a30"}}
```

```text
=== RUN   TestProcessingCaseWorkerOwnedStand
--- PASS: TestProcessingCaseWorkerOwnedStand (16.41s)
=== RUN   TestProcessingDiagnosticsOwnedStand
--- PASS: TestProcessingDiagnosticsOwnedStand (14.19s)
=== RUN   TestProcessingPurgeOwnedStand
--- PASS: TestProcessingPurgeOwnedStand (0.13s)
=== RUN   TestProcessingRecheckOwnedStand
--- PASS: TestProcessingRecheckOwnedStand (22.89s)
=== RUN   TestProcessingExecutableOwnedStand
--- PASS: TestProcessingExecutableOwnedStand (3.23s)
=== RUN   TestProcessingCaseTimeoutsOwnedStand
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.41s)
PASS
```

## build-iam-deletion-interop

```json
{"label": "build-iam-deletion-interop", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89/services/iam", "test", "-c", "-tags", "integration,hit148_probe", "-overlay", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-overlay-23g7o8kw/overlay.json", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-o4cjwf5r/iam-deletion.test", "./internal/adapters/composition/casedeletion"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:51:42.554196+00:00", "finished_at": "2026-11-22T19:51:42.954161+00:00", "exit_code": 0, "stdout": {"file": "027-build-iam-deletion-interop.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "027-build-iam-deletion-interop.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## iam-migration

```json
{"label": "iam-migration", "argv": ["docker", "exec", "-e", "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=hit148_migrator options='-c role=hp_iam_migrator'", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e", "/test/iam-migrate"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:51:43.431361+00:00", "finished_at": "2026-11-22T19:51:43.504435+00:00", "exit_code": 0, "stdout": {"file": "032-iam-migration.stdout", "bytes": 114, "sha256": "d9be125712cd9d9d9a4fdb2cf4cfc83f9806f6828376340315ab9ce93b7fe75f"}, "stderr": {"file": "032-iam-migration.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## real-iam-inventory

```json
{"label": "real-iam-inventory", "argv": ["docker", "exec", "-e", "HITPIPE_IAM_TEST_ADMIN_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=postgres", "-e", "HITPIPE_IAM_TEST_APP_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=hp_hit81_app", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e", "/test/iam-deletion.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^Test(ProcessingConsumesRealHIT154Inventory|PostgresCaseDeletionQualification)$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:51:43.505162+00:00", "finished_at": "2026-11-22T19:51:43.664793+00:00", "exit_code": 0, "stdout": {"file": "033-real-iam-inventory.stdout", "bytes": 2802, "sha256": "7d6ddb71e9c590f27c91c496c2ea6db43a0749da065fc8bb0d71b32376644d26"}, "stderr": {"file": "033-real-iam-inventory.stderr", "bytes": 211, "sha256": "1cde3ad4d2ee6d12fc1b85fb3014cd4ba07e6f7fe5818c9ad3afb5a104fdd32b"}}
```

```text
=== RUN   TestProcessingConsumesRealHIT154Inventory
    processing_inventory_test.go:30: postgres=180007 role=hp_hit81_app isolated_workspaces=2
    processing_inventory_test.go:128: signed-empty: exit=0 {"workspace_id":"da8784e0-4efb-4abf-9400-975494ff2a2f","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_test.go:129: neighbor-empty: exit=0 {"workspace_id":"d2cd4712-215a-40be-a32b-4ad4f877803f","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_test.go:130: foreign-workspace: exit=1 trusted case deletion inventory unavailable
    processing_inventory_test.go:131: unknown-generation: exit=1 trusted case deletion inventory unavailable
    processing_inventory_test.go:132: wrong-mTLS-client: exit=1 trusted case deletion inventory unavailable
    processing_inventory_test.go:133: wrong-server-pin: exit=1 trusted case deletion inventory unavailable
    processing_inventory_test.go:134: wrong-signer: exit=1 trusted case deletion inventory unavailable
    processing_inventory_test.go:144: owner-confirmed-applying: exit=0 {"workspace_id":"da8784e0-4efb-4abf-9400-975494ff2a2f","generation":1,"revision":3,"decision_count":1,"serving":"CLOSED"}
    processing_inventory_test.go:145: neighbor-stays-empty: exit=0 {"workspace_id":"d2cd4712-215a-40be-a32b-4ad4f877803f","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_test.go:147: no-current-source: exit=1 trusted case deletion inventory unavailable
--- PASS: TestProcessingConsumesRealHIT154Inventory (0.07s)
=== RUN   TestPostgresCaseDeletionQualification
    qualification_integration_test.go:82: postgres=180007 role=hp_hit81_app isolated_workspaces=2
    qualification_integration_test.go:144: real TLS 1.3 publisher read: valid full empty rev=1; other cert and absent source scope denied
    qualification_integration_test.go:170: owner checks: foreign workspace, foreign target, downgraded operator and tampered offer denied without new record
    qualification_integration_test.go:209: committed APPLYING before synthetic mutation id=6e0e2d94-9088-409a-889b-e892742593f5 generation=1 apply_revision=5; pre-proof receipt rejected
    qualification_integration_test.go:237: reopened issuer, reloaded same id and apply revision; durable receipt stored once, published rev=6
    qualification_integration_test.go:307: restored subordinate backup independently; generation=2 old receipt/stale/partial/foreign states denied; applied exact old id and opened under rev=10
    qualification_integration_test.go:336: independent source missing over real TLS: no snapshot; restore stayed closed; registry/publication excluded synthetic content and credentials
--- PASS: TestPostgresCaseDeletionQualification (0.03s)
PASS
```

## cleanup

```json
{"label": "cleanup", "argv": ["docker", "rm", "-f", "-v", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:51:43.693657+00:00", "finished_at": "2026-11-22T19:51:43.739060+00:00", "exit_code": 0, "stdout": {"file": "035-cleanup.stdout", "bytes": 65, "sha256": "6ad40151d923c87c62b7c8eae18da1b7464f000aab9a2080bff7f8560749ce5e"}, "stderr": {"file": "035-cleanup.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## cleanup-probe

```json
{"label": "cleanup-probe", "argv": ["docker", "container", "inspect", "6aebf8e8c9a18b4bf2a2fdfb955385bfa34f11adc53e97819508f0f3c307533e"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e", "tree_status_at_start": "", "runner_sha256": "b65c56dd844d66545859b9ae3216e86f398f822e8d07623af0c51f4fc9f176a4", "started_at": "2026-11-22T19:51:43.739829+00:00", "finished_at": "2026-11-22T19:51:43.753516+00:00", "exit_code": 1, "stdout": {"file": "036-cleanup-probe.stdout", "bytes": 3, "sha256": "37517e5f3dc66819f61f5a7bb8ace1921282415f10551d2defa5c3eb0985b570"}, "stderr": {"file": "036-cleanup-probe.stderr", "bytes": 112, "sha256": "3bdd9ccfd1c569920c967924b3ed3f643eee7e91bc60de679197a47421dd5366"}}
```

## Хеши бинарников

```json
{
  "source_sha": "0eba954c24652655cfc661fc63628b2097ebd33e",
  "platform": "linux/arm64",
  "sha256": {
    "processing": "e3cd35d689547e6f63cf58889af5017a69db858d613ad02db581322c3cfdf588",
    "processing-purge": "759dd266918e86b8e08777409c0e45a599dd1e32482a09605978302972197107",
    "storage.test": "4e5aaf05ab75d00aa4c7f5f8cf2a73309b1af5305cf119b8849e7a370f2aacb0",
    "iam-deletion.test": "1e23416fc56cfbf892f988e76bc8bec74f3fcc7e16a86fe01ab4077d2b6ca4a8",
    "iam-migrate": "3f9f4ac16b723a4848cb93ba689f3eea275f020a4e0ef9cd4e83a98265c5f75a"
  },
  "iam_source": "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
  "iam_schema_sha256": "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7",
  "overlay": {
    "source_fixture_sha256": "4acbc657c4fd125e6cbea44332a56122417a6e58be48a4050de31b9d7fcf0a4b",
    "patched_fixture_sha256": "55c2e72140ae5b9d422289e8a7e4a0c7d06d2aecbccae8cacafd208339b0bd3b",
    "test_sha256": "50bd9705ece0968ecee1dda36377563b5420b386c0a5b17569b6ca4d5e3b16fb"
  }
}
```
