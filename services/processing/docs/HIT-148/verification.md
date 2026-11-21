# Проверка исходной базы HIT-148

Codex по делегации Тимофея Патрушева. Новая запись выполнения, UTC. Проверялся унаследованный код очистки и восстановления. Адресный режим и рабочие данные в прогон не входили.

- Исходный SHA `8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3`. Рабочее дерево на старте чистое.
- Образ `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`. Два отдельных кластера в одном одноразовом контейнере без внешней сети.
- SHA-256 сценария `9898ac32f0b9eea496e6e23ab88a8e0ee8b43b1f49f77830a531f6a67a3f11c1` и миграции `a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93`.
- Завершение `2026-11-21T16:43:31.474262+00:00`. Go 1.27.2, локальный Darwin arm64 для race/vet и Linux arm64 для собранных процессов.
- Полные stdout/stderr приведены для выбранных проверок. Служебные логи начальной настройки контейнера не включены. Идентификаторы в сценариях локальные и синтетические. Две строки с одними пробелами в выводе тестов записаны как ASCII-последовательности \x20, каждая означает один пробел.
- Исходный вызов сделан из чистой рабочей копии. DOCKER_HOST и DOCKER_CONTEXT удалены из окружения вызова, использовался локальный Docker Desktop. Команды Go и Docker ниже содержат установленные для них переменные.

```sh
env -u DOCKER_HOST -u DOCKER_CONTEXT PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/recovery/verify_recovery.py --go ../go/bin/go --output ../../research/HIT-148-20261121/predecessor-stand
```

Хеши собранных Linux-бинарников из фактического прогона.

```json
{
  "source_sha": "8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3",
  "platform": "linux/arm64",
  "sha256": {
    "processing": "0e0ef3c965a405cda3508fa211381211e7fba0b5984b73fef1c2c8ca204c8ceb",
    "processing-purge": "d200b1337c67e8694a9677b668eb30c93e9ea484a9f5e93e15a19e52566236eb",
    "storage.test": "4e5aaf05ab75d00aa4c7f5f8cf2a73309b1af5305cf119b8849e7a370f2aacb0"
  }
}
```


### postgres-version

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:16.533176+00:00` → `2026-11-21T16:42:16.604047+00:00`
- Exit code `0`

```sh
docker exec bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18 postgres --version
```

stdout, 52 байт, SHA-256 `605b026879b9a291e0d851478a6f4dc23353423fa6f7035ce9e58a1e3a2e9e7a`.

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### postgres-settings

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:16.604774+00:00` → `2026-11-21T16:42:16.677754+00:00`
- Exit code `0`

```sh
docker exec -i bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18 psql -X -q -A -t -v VERBOSITY=verbose -v ON_ERROR_STOP=1 -U postgres -d postgres
```

stdout, 17 байт, SHA-256 `7ad450c52f1bab5c7f37b83c8bba2ed122e06a1a3e00e706c2ddca3efd95ab08`.

```text
180007|on|on|on|
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### go-version

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:17.120314+00:00` → `2026-11-21T16:42:17.150658+00:00`
- Exit code `0`

```sh
env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go version
```

stdout, 33 байт, SHA-256 `94e911f9fbf3c118f6a0573ab3a13fa546ad13e89ba40cf53df77e468509c92d`.

```text
go version go1.27.2 darwin/arm64
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### go-race

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:17.151257+00:00` → `2026-11-21T16:42:24.948757+00:00`
- Exit code `0`

```sh
env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing test -race -count=1 ./...
```

stdout, 3281 байт, SHA-256 `90a4d58039c6863a87b9f3e2af04853f04d6b729b4389637fedfec32e453dbbe`.

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.824s
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	2.098s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	3.041s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	2.121s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	2.055s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.977s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.824s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	2.413s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.791s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.375s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.239s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.229s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.205s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.203s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.227s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.259s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.248s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.252s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.237s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.295s
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### go-vet

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:24.949310+00:00` → `2026-11-21T16:42:25.747867+00:00`
- Exit code `0`

```sh
env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing vet ./...
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### container-arch

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:25.748374+00:00` → `2026-11-21T16:42:25.816620+00:00`
- Exit code `0`

```sh
docker exec bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18 uname -m
```

stdout, 8 байт, SHA-256 `5121ae59314a84da7b4d9587374398f1afa2000b243b3e54f491adefaac54f92`.

```text
aarch64
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### build-processing

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:25.817256+00:00` → `2026-11-21T16:42:26.601624+00:00`
- Exit code `0`

```sh
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing build -trimpath -ldflags '-X main.revision=8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3' -o /var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit128-build-unh98mgm/processing ./cmd/processing
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### build-processing-purge

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:26.609648+00:00` → `2026-11-21T16:42:26.891210+00:00`
- Exit code `0`

```sh
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing build -trimpath -ldflags '-X main.revision=8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3' -o /var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit128-build-unh98mgm/processing-purge ./cmd/processing-purge
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### build-recovery-tests

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:26.897384+00:00` → `2026-11-21T16:42:28.441167+00:00`
- Exit code `0`

```sh
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 env GOTOOLCHAIN=local GOWORK=off /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion/services/processing test -c -o /var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit128-build-unh98mgm/storage.test ./internal/adapters/postgres
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### recovery-process-tests

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:28.450338+00:00` → `2026-11-21T16:42:32.707393+00:00`
- Exit code `0`

```sh
docker exec -e HIT78_ISOLATED_PG=1 -e HIT128_RECOVERY_STAND=1 -e HIT111_PROCESSING_BIN=/test/processing -e HIT126_PURGE_BIN=/test/processing-purge bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18 /test/storage.test -test.v -test.count=1 -test.timeout=110s '-test.run=^TestProcessingRecoveryOwnedStand$'
```

stdout, 7630 байт, SHA-256 `35135ac055bf4e0d7323122c3a7df4468834b5a366a92c55b9e7b3becbe69d2b`.

```text
=== RUN   TestProcessingRecoveryOwnedStand
    recovery_process_test.go:201: original OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:28.589334095Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:203: original OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:29.576551513Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:242: SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent
    recovery_process_test.go:263: Processing stopped by SIGKILL
    recovery_process_test.go:266: cold restart, same database OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:29.886438429Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:268: Processing stopped by SIGKILL
    recovery_process_test.go:287: scoped SELECT hp_processing.operations: rows=1
    recovery_process_test.go:287: scoped SELECT hp_processing.accepted_configs: rows=1
    recovery_process_test.go:287: scoped SELECT hp_processing.case_identity: rows=1
    recovery_process_test.go:287: scoped SELECT hp_processing.episodes: rows=1
    recovery_process_test.go:287: scoped SELECT hp_processing.case_content: rows=0
    recovery_process_test.go:287: scoped SELECT hp_processing.safety_history: rows=5
    recovery_process_test.go:287: scoped SELECT hp_processing.history_content: rows=5
    recovery_process_test.go:287: scoped SELECT hp_processing.user_commands: rows=1
    recovery_process_test.go:287: scoped SELECT hp_processing.inbox: rows=3
    recovery_process_test.go:287: scoped SELECT hp_processing.outbox: rows=2
    recovery_process_test.go:288: source intent state=CLAIMED lease=true attempts=1; known identity and fence persisted
    recovery_process_test.go:290: exit=0 /usr/bin/chown -R postgres:postgres /tmp/hit128-target-3517641928
    recovery_process_test.go:290: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/initdb -D /tmp/hit128-target-3517641928/data --auth-local=trust --auth-host=trust
    recovery_process_test.go:290: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-3517641928/data -l /tmp/hit128-target-3517641928/postgres.log -o -k /tmp/hit128-target-3517641928 -p 55432 -h 127.0.0.5 -c fsync=on -c full_page_writes=on -c synchronous_commit=on -c ssl=on -c ssl_cert_file=/tmp/hit128-target-3517641928/server.crt -c ssl_key_file=/tmp/hit128-target-3517641928/server.key -w start
    recovery_process_test.go:290: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-3517641928 port=55432 dbname=postgres sslmode=disable user=postgres -f /migrations/roles.sql
    recovery_process_test.go:290: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-3517641928 port=55432 dbname=postgres sslmode=disable user=postgres -c CREATE DATABASE "hit128-restore" OWNER hp_processing_owner
    recovery_process_test.go:290: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-3517641928 port=55432 dbname=postgres sslmode=disable user=postgres -c REVOKE ALL ON DATABASE "hit128-restore" FROM PUBLIC; GRANT CONNECT ON DATABASE "hit128-restore" TO hp_processing_app,hp_processing_migrator,hp_processing_worker,hp_processing_purge
    recovery_process_test.go:290: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-3517641928 port=55432 dbname=hit128-restore sslmode=disable user=hp_processing_migrator -f /migrations/apply.sql
    recovery_process_test.go:290: exit=0 /usr/bin/psql -XAt -U postgres -d postgres -c SELECT system_identifier FROM pg_control_system()
    recovery_process_test.go:290: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-3517641928 port=55432 dbname=postgres sslmode=disable user=postgres -At -c SELECT system_identifier FROM pg_control_system()
    recovery_process_test.go:290: different cluster system identifiers: source=7710666034747949108 target=7710666094963216601
    recovery_process_test.go:291: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:318: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:322: restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back
    recovery_process_test.go:326: ordinary configuration without authoritative stand inputs OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:30.361605763Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:328: Processing stopped by SIGKILL
    recovery_process_test.go:333: target with stale boot snapshot OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:30.487365305Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:338: target with wrong generation OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:31.499053347Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:340: target with fresh boot and generation OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:32.42019625Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:352: fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded
    recovery_process_test.go:369: Processing stopped by SIGKILL
    recovery_process_test.go:384: target, one missing detail OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:32.513936583Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:393: live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE
    recovery_process_test.go:394: Processing stopped by SIGKILL
    recovery_process_test.go:403: exit=0 /test/processing-purge --local-stand --content-dir /tmp/TestProcessingRecoveryOwnedStand3681771587/001/content --workspace 8eeedff5-55fb-407d-bc25-fc475ee87f44 --limit 100
    recovery_process_test.go:407: purge result: {"held":0,"removed":2}
    recovery_process_test.go:420: target after expiry cleanup OPS mTLS HTTP 200 {"build_revision":"8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3","checked_at":"2026-11-21T16:42:32.577504166Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:426: Processing stopped by SIGKILL
    recovery_process_test.go:427: purge removed expired bytes while preserving the exact public history metadata and expiry
    recovery_process_test.go:432: RECOVERY PASS: live provider create calls=1, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=4064
    recovery_cluster_test.go:81: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-3517641928/data -m fast -w stop
--- PASS: TestProcessingRecoveryOwnedStand (4.18s)
PASS
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### predecessor-process-tests

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:42:32.709455+00:00` → `2026-11-21T16:43:31.472752+00:00`
- Exit code `0`

```sh
docker exec -e HIT78_ISOLATED_PG=1 -e HIT111_PROCESSING_BIN=/test/processing -e HIT126_PURGE_BIN=/test/processing-purge bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18 /test/storage.test -test.v -test.count=1 -test.timeout=145s '-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$'
```

stdout, 5614 байт, SHA-256 `8beba4e70908a088c9b608d61f2f6d83f5a140df91a0bcaf832774965cafd2e2`.

```text
=== RUN   TestProcessingCaseWorkerOwnedStand
    caseworker_process_test.go:258: PROCESS PASS: owned mTLS admission and SourceFacts, fresh proof clock, incomplete/unpaid/unavailable, late payment and kill/restart, ACK dedup, one delivery receipt without confirmation, pause during read and raw-write barrier
--- PASS: TestProcessingCaseWorkerOwnedStand (16.38s)
=== RUN   TestProcessingDiagnosticsOwnedStand
    diagnostics_test.go:389: HTTP PASS: captured actual Health CLOSED/READY/DEGRADED/recovered and mTLS denial; missing each required field and an extra property were rejected by the pinned validator
    diagnostics_test.go:390: Observed live inbox age 2054 ms, outbox due lag 2048 ms, job due lag 2048 ms and committed observation age 708 ms.
    diagnostics_test.go:392: PROCESS PASS: CLOSED/OPEN and exact Health, actual HELD/UNKNOWN, changed outbox/jobs, observation replay, expired lease, content and PostgreSQL faults, purge reason, restart and log privacy
--- PASS: TestProcessingDiagnosticsOwnedStand (14.20s)
=== RUN   TestProcessingPurgeOwnedStand
    purge_test.go:236: expired pending/claimed messages held before unlink; process exit after commit, restart and idempotent bounded sweep; live SQL ref and foreign file preserved; safety identity and command keys retained
--- PASS: TestProcessingPurgeOwnedStand (0.15s)
=== RUN   TestProcessingRecheckOwnedStand
    recheck_process_test.go:312: PROCESS PASS: recoverCase 202 and HIT-99, access/CSRF/mTLS, one queued exact query, SIGKILL/restart, bounded timeout/429/503, incomplete and mismatched responses, late racing observation, revoked raw write and no Create
--- PASS: TestProcessingRecheckOwnedStand (23.31s)
=== RUN   TestProcessingExecutableOwnedStand
    runtime_process_test.go:707: {"time":"2026-11-21T16:43:26.883299428Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
        {"time":"2026-11-21T16:43:26.883563845Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"CLOSED"}
        {"time":"2026-11-21T16:43:27.891079678Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
        {"time":"2026-11-21T16:43:28.116500345Z","level":"INFO","msg":"processing outbox transition","message_id":"b3677972-7e69-4248-b8e9-f48ec2f3b3f5","state":"ACKED","reason":"APPLIED"}
        {"time":"2026-11-21T16:43:28.129163095Z","level":"INFO","msg":"processing case worker held","case_id":"ef1fd561-507c-423c-96bf-1df962ae4c5a"}
        {"time":"2026-11-21T16:43:28.14890272Z","level":"INFO","msg":"processing case worker decision","case_id":"8e8566b4-f452-4a72-9bc4-430db3b591a3","state":"WAITING_DATA"}
        {"time":"2026-11-21T16:43:28.156645929Z","level":"INFO","msg":"processing observation committed","observation_age_ms":5,"disposition":"APPLIED","case_state":"CONFIRMED","skipped_previous_window":0,"workspace_id":"0af8b570-9097-4988-892e-3976916043b4","case_id":"52f0316b-fc40-407e-966a-ec89ea34fd86","message_id":"bf2e2872-4771-4836-a93d-ea713edfde18","effect_id":"b3677972-7e69-4248-b8e9-f48ec2f3b3f5"}
        {"time":"2026-11-21T16:43:28.160892679Z","level":"INFO","msg":"processing case worker held","case_id":"5f9ef24a-54af-4c60-ae22-0e6dfda02788"}
        {"time":"2026-11-21T16:43:28.49360947Z","level":"WARN","msg":"processing diagnostic fault","reason":"barrier_denied","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
        {"time":"2026-11-21T16:43:28.49439197Z","level":"WARN","msg":"processing diagnostic fault","reason":"dispatch_held","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
        {"time":"2026-11-21T16:43:28.495197095Z","level":"WARN","msg":"processing diagnostic fault","reason":"timer_interrupted","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
\x20\x20\x20\x20\x20\x20\x20\x20
    runtime_process_test.go:707: {"time":"2026-11-21T16:43:28.710244845Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
        {"time":"2026-11-21T16:43:28.710307554Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"CLOSED"}
        {"time":"2026-11-21T16:43:29.716031138Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
        {"time":"2026-11-21T16:43:29.941017721Z","level":"WARN","msg":"processing diagnostic fault","reason":"barrier_denied","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
        {"time":"2026-11-21T16:43:29.942190388Z","level":"WARN","msg":"processing diagnostic fault","reason":"dispatch_held","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
        {"time":"2026-11-21T16:43:29.943485263Z","level":"WARN","msg":"processing diagnostic fault","reason":"timer_interrupted","workspace_id":"80499d4f-7d42-476d-95ce-55e5f5643b18"}
        {"time":"2026-11-21T16:43:30.033801263Z","level":"INFO","msg":"processing outbox transition","message_id":"c6fcfbff-bedb-4281-ab92-71da3945584a","state":"ACKED","reason":"APPLIED"}
        {"time":"2026-11-21T16:43:30.033818679Z","level":"INFO","msg":"processing stopped"}
\x20\x20\x20\x20\x20\x20\x20\x20
    runtime_process_test.go:709: PROCESS PASS: CLOSED→fresh IAM snapshot, mTLS allowlists, activation/digest, intake/history, prepared outbox/ACK, observation, pause/resume, barrier, kill/restart dedup and SIGTERM with in-flight ACK
--- PASS: TestProcessingExecutableOwnedStand (3.22s)
=== RUN   TestProcessingCaseTimeoutsOwnedStand
    timeouts_process_test.go:127: PROCESS PASS: actual worker timed out waiting inside five minutes, preserved reason and exactly one notification across kill/restart, held corrupt/expired history and missing episode, owner mTLS views and cross-workspace denial
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.42s)
PASS
```

stderr, 216 байт, SHA-256 `eadd2cf4f10dd28c5e236e9ce7debc0a60abd193d4639f17a10c466c079cdadd`.

```text
2026/11/21 16:42:48 http: TLS handshake error from 127.0.0.1:56916: EOF
2026/11/21 16:43:26 http: TLS handshake error from 127.0.0.1:50076: EOF
2026/11/21 16:43:28 http: TLS handshake error from 127.0.0.1:50096: EOF
```

### cleanup

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:43:31.501713+00:00` → `2026-11-21T16:43:31.545746+00:00`
- Exit code `0`

```sh
docker rm -f -v bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18
```

stdout, 65 байт, SHA-256 `76547746e0d962f0d51d4608d54e8dc6252464440b3071df1d7c1bda81e39186`.

```text
bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### cleanup-probe

- Каталог `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion`
- Время UTC `2026-11-21T16:43:31.546380+00:00` → `2026-11-21T16:43:31.560396+00:00`
- Exit code `1`

```sh
docker container inspect bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18
```

stdout, 3 байт, SHA-256 `37517e5f3dc66819f61f5a7bb8ace1921282415f10551d2defa5c3eb0985b570`.

```text
[]
```

stderr, 112 байт, SHA-256 `3d9925c4ff82cc934a79282b45381278a71a5d7b0a75f768e30091d7e25428d4`.

```text
Error response from daemon: No such container: bef54b5318c7407ead2e3b6d5d764f85b37fe54590c9f5bdc760c9755ca64f18
```
