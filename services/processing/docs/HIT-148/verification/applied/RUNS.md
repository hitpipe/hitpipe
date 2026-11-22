# HIT-148. Фактический прогон адресного удаления

Processing `107ed4e3ae0d43300a09dea10b2d43ad4423efc1`, IAM `c715a079ab57d4c9e7b56862f7f2584c792b74b0`, завершение UTC 2026-11-22T20:27:50.380451+00:00. Исходное дерево было чистым. Полный пакет `raw-local.tar.gz` содержит точные команды, cwd, UTC, exit codes, stdout, stderr, SQL-команды стенда и хеши. SHA-256 архива `4a287c032b17cbb7a5d181e9f62f3464824dd8b44dbaa5292205aee2c10be956`.

Go 1.27.2 race и vet выполнены на хосте. Бинарники и процессы квалификации выполнялись на Linux внутри изолированного контейнера PostgreSQL 18.7. Встроенный тест из IAM-зависимости остаётся отдельной регрессией. Основной сценарий `TestProcessingAppliesAndRestoresRealHIT154` использует реальные файлы, SQL-проекции и бинарник обслуживания Processing, настоящий издатель HIT-154 и реальный стартовый gate Processing.

## postgres-version

```json
{"label": "postgres-version", "argv": ["docker", "exec", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39", "postgres", "--version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:34.459696+00:00", "finished_at": "2026-11-22T20:26:34.518876+00:00", "exit_code": 0, "stdout": {"file": "008-postgres-version.stdout", "bytes": 52, "sha256": "605b026879b9a291e0d851478a6f4dc23353423fa6f7035ce9e58a1e3a2e9e7a"}, "stderr": {"file": "008-postgres-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

## go-version

```json
{"label": "go-version", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:35.022264+00:00", "finished_at": "2026-11-22T20:26:35.030553+00:00", "exit_code": 0, "stdout": {"file": "018-go-version.stdout", "bytes": 33, "sha256": "94e911f9fbf3c118f6a0573ab3a13fa546ad13e89ba40cf53df77e468509c92d"}, "stderr": {"file": "018-go-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
go version go1.27.2 darwin/arm64
```

## go-race

```json
{"label": "go-race", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "test", "-race", "-count=1", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:35.031147+00:00", "finished_at": "2026-11-22T20:26:40.921682+00:00", "exit_code": 0, "stdout": {"file": "019-go-race.stdout", "bytes": 3464, "sha256": "2dce291a855f4c740a84bf6e272310cfc5134f985f411e42c6f39ea450d56ba1"}, "stderr": {"file": "019-go-race.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.716s
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.861s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.908s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deletionregistry	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.889s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.823s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.736s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.704s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	2.244s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.564s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.354s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.237s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.226s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.203s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.201s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.287s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.231s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.230s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.237s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.230s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.273s
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/maintenance	[no test files]
```

## go-vet

```json
{"label": "go-vet", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "vet", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:40.922449+00:00", "finished_at": "2026-11-22T20:26:41.454261+00:00", "exit_code": 0, "stdout": {"file": "020-go-vet.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "020-go-vet.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing

```json
{"label": "build-processing", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-zrnzoqhi/processing", "./cmd/processing"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:41.522473+00:00", "finished_at": "2026-11-22T20:26:42.166570+00:00", "exit_code": 0, "stdout": {"file": "022-build-processing.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "022-build-processing.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing-purge

```json
{"label": "build-processing-purge", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-zrnzoqhi/processing-purge", "./cmd/processing-purge"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:42.174307+00:00", "finished_at": "2026-11-22T20:26:42.466232+00:00", "exit_code": 0, "stdout": {"file": "023-build-processing-purge.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "023-build-processing-purge.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-recovery-tests

```json
{"label": "build-recovery-tests", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing", "test", "-c", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-zrnzoqhi/storage.test", "./internal/adapters/postgres"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:42.473153+00:00", "finished_at": "2026-11-22T20:26:43.670433+00:00", "exit_code": 0, "stdout": {"file": "024-build-recovery-tests.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "024-build-recovery-tests.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## recovery-process-tests

```json
{"label": "recovery-process-tests", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT128_RECOVERY_STAND=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=110s", "-test.run=^TestProcessingRecoveryOwnedStand$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:43.679468+00:00", "finished_at": "2026-11-22T20:26:47.916659+00:00", "exit_code": 0, "stdout": {"file": "025-recovery-process-tests.stdout", "bytes": 7630, "sha256": "c974daa2779662b50225002896acf0c8deae2bb0adbdd073cc7a7b102cef4da7"}, "stderr": {"file": "025-recovery-process-tests.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
=== RUN   TestProcessingRecoveryOwnedStand
--- PASS: TestProcessingRecoveryOwnedStand (4.15s)
PASS
```

## predecessor-process-tests

```json
{"label": "predecessor-process-tests", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=145s", "-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:26:47.918414+00:00", "finished_at": "2026-11-22T20:27:46.406596+00:00", "exit_code": 0, "stdout": {"file": "026-predecessor-process-tests.stdout", "bytes": 5071, "sha256": "bb4f5d4e0b87ff471e5c565976fdd8090157199789a13e7297ff44385a7e3a7c"}, "stderr": {"file": "026-predecessor-process-tests.stderr", "bytes": 429, "sha256": "0bbf24ad857819702b739f0ac3a042c7339506a0aa0da48ec54b4804a3631819"}}
```

```text
=== RUN   TestProcessingCaseWorkerOwnedStand
--- PASS: TestProcessingCaseWorkerOwnedStand (16.36s)
=== RUN   TestProcessingDiagnosticsOwnedStand
--- PASS: TestProcessingDiagnosticsOwnedStand (14.19s)
=== RUN   TestProcessingPurgeOwnedStand
--- PASS: TestProcessingPurgeOwnedStand (0.13s)
=== RUN   TestProcessingRecheckOwnedStand
--- PASS: TestProcessingRecheckOwnedStand (23.10s)
=== RUN   TestProcessingExecutableOwnedStand
--- PASS: TestProcessingExecutableOwnedStand (3.20s)
=== RUN   TestProcessingCaseTimeoutsOwnedStand
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.43s)
PASS
```

## build-iam-deletion-interop

```json
{"label": "build-iam-deletion-interop", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89/services/iam", "test", "-c", "-tags", "integration,hit148_probe", "-modfile=/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-overlay-wqztgglq/iam-stand.mod", "-mod=mod", "-overlay", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-overlay-wqztgglq/overlay.json", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit148-build-zrnzoqhi/iam-deletion.test", "./internal/adapters/composition/casedeletion"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:27:46.430072+00:00", "finished_at": "2026-11-22T20:27:46.871213+00:00", "exit_code": 0, "stdout": {"file": "028-build-iam-deletion-interop.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "028-build-iam-deletion-interop.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## iam-migration

```json
{"label": "iam-migration", "argv": ["docker", "exec", "-e", "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=hit148_migrator options='-c role=hp_iam_migrator'", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39", "/test/iam-migrate"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:27:47.347220+00:00", "finished_at": "2026-11-22T20:27:47.412828+00:00", "exit_code": 0, "stdout": {"file": "033-iam-migration.stdout", "bytes": 114, "sha256": "d9be125712cd9d9d9a4fdb2cf4cfc83f9806f6828376340315ab9ce93b7fe75f"}, "stderr": {"file": "033-iam-migration.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

[Полный журнал основного интеграционного запуска](RUNTIME.md).

## cleanup

```json
{"label": "cleanup", "argv": ["docker", "rm", "-f", "-v", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:27:50.407662+00:00", "finished_at": "2026-11-22T20:27:50.456023+00:00", "exit_code": 0, "stdout": {"file": "042-cleanup.stdout", "bytes": 65, "sha256": "2864b4cfb037fbbc0ab002acbabfbbd98544cb083e74fc314020667e2246f642"}, "stderr": {"file": "042-cleanup.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## cleanup-probe

```json
{"label": "cleanup-probe", "argv": ["docker", "container", "inspect", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:27:50.456709+00:00", "finished_at": "2026-11-22T20:27:50.469533+00:00", "exit_code": 1, "stdout": {"file": "043-cleanup-probe.stdout", "bytes": 3, "sha256": "37517e5f3dc66819f61f5a7bb8ace1921282415f10551d2defa5c3eb0985b570"}, "stderr": {"file": "043-cleanup-probe.stderr", "bytes": 112, "sha256": "6b497e03ad10df90f44c1c01f08e5698959b33b3d57ed9ecd66e3524dd0fa5ce"}}
```

## Хеши сборок

```json
{
  "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
  "platform": "linux/arm64",
  "sha256": {
    "processing": "4808c6b5fe840d56b37ccc177b18fd463437423352ea42ef21f6f44ab7418823",
    "processing-purge": "3b9e0bf2d2614c9b566527f9a0468178ef1b76db26532e625902e0de1fa73791",
    "storage.test": "8a5585a1818298b9665b6bf861a41c87f7eb468fa9e9a27e13801ac77e61aeef",
    "iam-deletion.test": "f7027422dcf33f9bb98bf4f5ff07e77e884142687ddc3cdece10350fa154d115",
    "iam-migrate": "3f9f4ac16b723a4848cb93ba689f3eea275f020a4e0ef9cd4e83a98265c5f75a"
  },
  "iam_source": "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
  "iam_schema_sha256": "604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7",
  "overlay": {
    "source_fixture_sha256": "4acbc657c4fd125e6cbea44332a56122417a6e58be48a4050de31b9d7fcf0a4b",
    "patched_fixture_sha256": "50f449b533bb567cd645e423364d06f6335e8e9a597c9bcdc95e0aebe7629c2f",
    "test_sha256": {
      "inventory-probe.go": "50bd9705ece0968ecee1dda36377563b5420b386c0a5b17569b6ca4d5e3b16fb",
      "deletion-flow.go": "9a391e5f6126ed4be9e35ac1c7888942ac5ec02a0402c6276bea052acb082a49",
      "deletion-runtime.go": "01d9d95216fa57d759e752230edbd76e0f9dd60c7f5fa237a855649de9e00d3a"
    },
    "test_mod_sha256": "95cf50ceed4a91a90bde725d34d92ba1af4eadfccd1e85b38cf1e35f545a80b0"
  }
}
```
