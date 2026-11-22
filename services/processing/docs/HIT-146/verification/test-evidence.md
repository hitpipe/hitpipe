# Проверки, сборки и прогоны стенда

Изолированная проверка старого Processing с управляемым IAM отделена от прогона с реальным IAM. Успешный прежний тест покрывает сохранение поведения прежнего стенда.

Попытка 1, ed2b037d8f45f51985c5cad829f8fbdbda505bae, build-iam-probe exit 1. Ошибка относилась к стенду.

Попытка 2, c35321e7427bb489da49e710268703594691e225, real-iam-boundary exit 1. Ошибка относилась к стенду.

Попытка 3, 9f6b6170f6e4a2d2b51178c8b3ebf2cae25797c8, real-iam-boundary exit 1. Ошибка относилась к стенду.

Попытка 4, e19c0a40f52a7661c7fda439feb967de1c3baf60. Блокировка воспроизвелась, код выхода теста 0. GitHub CI отклонил расположение IAM-стенда внутри Processing. Тестовая сборка перенесена в tools/hit146 без изменений принятого кода обоих сервисов.

Попытка 5, e19c0a40f52a7661c7fda439feb967de1c3baf60. Блокировка воспроизвелась, код выхода теста 0. GitHub CI отклонил расположение IAM-стенда внутри Processing. Тестовая сборка перенесена в tools/hit146 без изменений принятого кода обоих сервисов.

- qualification-6 · postgres-version · exit 0 · 2026-11-22T15:17:30.654243+00:00.

```json
["docker", "exec", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e", "postgres", "--version"]
```

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

- qualification-6 · postgres-settings · exit 0 · 2026-11-22T15:17:30.717437+00:00.

```json
["docker", "exec", "-i", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e", "psql", "-X", "-q", "-A", "-t", "-v", "VERBOSITY=verbose", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "postgres"]
```

- qualification-6 · go-version · exit 0 · 2026-11-22T15:17:31.308620+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "version"]
```

```text
go version go1.27.2 darwin/arm64
```

- qualification-6 · go-race · exit 0 · 2026-11-22T15:17:31.318143+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-race", "-count=1", "./..."]
```

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.662s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.700s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.770s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.709s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.647s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.571s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.526s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	2.065s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.515s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.552s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.181s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.163s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.124s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.119s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.119s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.099s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.104s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.094s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.150s
?   	github.com/hitpipe/hitpipe/services/processing/internal/runtime	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

- qualification-6 · go-vet · exit 0 · 2026-11-22T15:17:35.117152+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "vet", "./..."]
```

- qualification-6 · build-processing · exit 0 · 2026-11-22T15:17:35.283540+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=ce317193244623c5a464a3014bb39f32875c1e98", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-683dtg03/processing", "./cmd/processing"]
```

- qualification-6 · build-processing-tests · exit 0 · 2026-11-22T15:17:35.910701+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-c", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-683dtg03/storage.test", "./internal/adapters/postgres"]
```

- qualification-6 · build-iam-probe · exit 0 · 2026-11-22T15:17:36.585240+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam/services/iam", "build", "-trimpath", "-tags", "hit146_iam_fixture", "-overlay", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-683dtg03/iam.probe.overlay.json", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-683dtg03/iam-probe", "./cmd/iam-contractcheck"]
```

- qualification-6 · build-iam-migrate · exit 0 · 2026-11-22T15:17:36.924365+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam/services/iam", "build", "-trimpath", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-683dtg03/iam-migrate", "./cmd/iam-migrate"]
```

- qualification-6 · iam-migration · exit 0 · 2026-11-22T15:17:37.419958+00:00.

```json
["docker", "exec", "-e", "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql user=hp_hit146_iam_migrate dbname=hit146_iam sslmode=disable options='-c role=hp_iam_migrator'", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e", "/test/iam-migrate"]
```

```text
{"state":"installed","version":1,"sql_sha256":"604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"}
```

- qualification-6 · processing-predecessor · exit 0 · 2026-11-22T15:17:42.872415+00:00.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestProcessingExecutableOwnedStand$"]
```

- qualification-6 · cleanup · exit 0 · 2026-11-22T15:17:46.160587+00:00.

```json
["docker", "rm", "-f", "-v", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e"]
```

- qualification-6 · cleanup-probe · exit 1 · 2026-11-22T15:17:46.217082+00:00.

```json
["docker", "container", "inspect", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e"]
```

- qualification-7 · postgres-version · exit 0 · 2026-11-22T15:17:48.137749+00:00.

```json
["docker", "exec", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524", "postgres", "--version"]
```

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

- qualification-7 · postgres-settings · exit 0 · 2026-11-22T15:17:48.202758+00:00.

```json
["docker", "exec", "-i", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524", "psql", "-X", "-q", "-A", "-t", "-v", "VERBOSITY=verbose", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "postgres"]
```

- qualification-7 · go-version · exit 0 · 2026-11-22T15:17:48.766501+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "version"]
```

```text
go version go1.27.2 darwin/arm64
```

- qualification-7 · go-race · exit 0 · 2026-11-22T15:17:48.776085+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-race", "-count=1", "./..."]
```

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.132s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.281s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.316s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.276s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.211s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.138s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.112s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	1.741s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.198s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.229s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.206s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.201s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.178s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.103s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.115s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.100s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.106s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.100s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.163s
?   	github.com/hitpipe/hitpipe/services/processing/internal/runtime	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

- qualification-7 · go-vet · exit 0 · 2026-11-22T15:17:52.206388+00:00.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "vet", "./..."]
```

- qualification-7 · build-processing · exit 0 · 2026-11-22T15:17:52.369949+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=ce317193244623c5a464a3014bb39f32875c1e98", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-oxn5nlnz/processing", "./cmd/processing"]
```

- qualification-7 · build-processing-tests · exit 0 · 2026-11-22T15:17:52.961379+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-c", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-oxn5nlnz/storage.test", "./internal/adapters/postgres"]
```

- qualification-7 · build-iam-probe · exit 0 · 2026-11-22T15:17:53.614346+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam/services/iam", "build", "-trimpath", "-tags", "hit146_iam_fixture", "-overlay", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-oxn5nlnz/iam.probe.overlay.json", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-oxn5nlnz/iam-probe", "./cmd/iam-contractcheck"]
```

- qualification-7 · build-iam-migrate · exit 0 · 2026-11-22T15:17:53.896639+00:00.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam/services/iam", "build", "-trimpath", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-oxn5nlnz/iam-migrate", "./cmd/iam-migrate"]
```

- qualification-7 · iam-migration · exit 0 · 2026-11-22T15:17:54.370517+00:00.

```json
["docker", "exec", "-e", "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql user=hp_hit146_iam_migrate dbname=hit146_iam sslmode=disable options='-c role=hp_iam_migrator'", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524", "/test/iam-migrate"]
```

```text
{"state":"installed","version":1,"sql_sha256":"604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7"}
```

- qualification-7 · processing-predecessor · exit 0 · 2026-11-22T15:17:59.850811+00:00.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestProcessingExecutableOwnedStand$"]
```

- qualification-7 · cleanup · exit 0 · 2026-11-22T15:18:03.129448+00:00.

```json
["docker", "rm", "-f", "-v", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524"]
```

- qualification-7 · cleanup-probe · exit 1 · 2026-11-22T15:18:03.186699+00:00.

```json
["docker", "container", "inspect", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524"]
```
