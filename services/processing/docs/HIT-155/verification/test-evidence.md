# Применимые проверки HIT-155

Исходник `1ed91653a747b47a98f2738ca0d831a38cebc98a`.

Go test -race и vet выполнены на Darwin arm64. OCI, тестовый драйвер и службы в стенде собраны для Linux arm64. Дополнительно прошёл исходный HIT-128 runner с локальным Linux-бинарником. Он служит проверкой совместимости прежнего режима теста.

### qualification-4 / go-version

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:13.491380+00:00

Exit code 0.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery/../go/bin/go", "version"]
```

stdout, 33 байт, SHA-256 `94e911f9fbf3c118f6a0573ab3a13fa546ad13e89ba40cf53df77e468509c92d`.

```text
go version go1.27.2 darwin/arm64
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / go-race

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:21.444464+00:00

Exit code 0.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery/../go/bin/go", "-C", "services/processing", "test", "-race", "-count=1", "./..."]
```

stdout, 3281 байт, SHA-256 `f1b6263f8bbc0f8a4715dc74e10dbe51ae2770eff555008a2129e23657b7d406`.

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.132s
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.291s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.355s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.314s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.218s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.137s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.115s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	1.658s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.124s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.397s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.175s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.162s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.137s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.138s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.155s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.143s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.150s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.106s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.170s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.295s
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / go-vet

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:26.167562+00:00

Exit code 0.

```json
["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery/../go/bin/go", "-C", "services/processing", "vet", "./..."]
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / build-test-driver

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:18.643424+00:00

Exit code 0.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery/../go/bin/go", "-C", "services/processing", "test", "-c", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit155-efuvi3lx/storage.test", "./internal/adapters/postgres"]
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / build-purge

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:26.423904+00:00

Exit code 0.

```json
["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery/../go/bin/go", "-C", "services/processing", "build", "-trimpath", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit155-efuvi3lx/processing-purge", "./cmd/processing-purge"]
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### native-regression / recovery-process-tests

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:39:10.967071+00:00

Exit code 0.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT128_RECOVERY_STAND=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "6e44e2e75e3dec76e6031f376a5bbf1852782f6ba0b91f503c5b76459c91e957", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=110s", "-test.run=^TestProcessingRecoveryOwnedStand$"]
```

stdout, 8629 байт, SHA-256 `6bd3f89e4c70c263e6e16c757aa222be3f31be6b1e45f606fa70392390a7fdf3`.

```text
=== RUN   TestProcessingRecoveryOwnedStand
    recovery_process_test.go:162: original OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:11.106954377Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:162: original OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:12.122310169Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:205: SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:162: cold restart, same database OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:12.388592085Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:205: scoped SELECT hp_processing.operations: rows=1
    recovery_process_test.go:205: scoped SELECT hp_processing.accepted_configs: rows=1
    recovery_process_test.go:205: scoped SELECT hp_processing.case_identity: rows=1
    recovery_process_test.go:205: scoped SELECT hp_processing.episodes: rows=1
    recovery_process_test.go:205: scoped SELECT hp_processing.case_content: rows=0
    recovery_process_test.go:205: scoped SELECT hp_processing.safety_history: rows=5
    recovery_process_test.go:205: scoped SELECT hp_processing.history_content: rows=5
    recovery_process_test.go:205: scoped SELECT hp_processing.user_commands: rows=1
    recovery_process_test.go:205: scoped SELECT hp_processing.inbox: rows=3
    recovery_process_test.go:205: scoped SELECT hp_processing.outbox: rows=2
    recovery_process_test.go:205: source intent state=CLAIMED lease=true attempts=1; known identity and fence persisted
    recovery_process_test.go:205: source safety: {Operation:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa Config:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb Digest:sha256:51b2440a0d2ec8d6adc410212231d7749c3d46cd3dbd137799bd8d9500f7a558 Case:ce7fcce6-c3ac-45d6-a4c3-6e3a1fa8c630 Lineage:206e4cf9-a8aa-4e03-8fc4-89c985bfbaee Target:86synthetic99 State:UNKNOWN Knowledge:TARGET_ID_KNOWN Fence:true Cases:1 Commands:1 Inbox:3 Intents:1 Queries:1}
    recovery_process_test.go:205: exit=0 /usr/bin/chown -R postgres:postgres /tmp/hit128-target-4175273891
    recovery_process_test.go:205: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/initdb -D /tmp/hit128-target-4175273891/data --auth-local=trust --auth-host=trust
    recovery_process_test.go:205: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-4175273891/data -l /tmp/hit128-target-4175273891/postgres.log -o -k /tmp/hit128-target-4175273891 -p 55432 -h 127.0.0.5 -c fsync=on -c full_page_writes=on -c synchronous_commit=on -c ssl=on -c ssl_cert_file=/tmp/hit128-target-4175273891/server.crt -c ssl_key_file=/tmp/hit128-target-4175273891/server.key -w start
    recovery_process_test.go:205: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-4175273891 port=55432 dbname=postgres sslmode=disable user=postgres -f /migrations/roles.sql
    recovery_process_test.go:205: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-4175273891 port=55432 dbname=postgres sslmode=disable user=postgres -c CREATE DATABASE "hit128-restore" OWNER hp_processing_owner
    recovery_process_test.go:205: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-4175273891 port=55432 dbname=postgres sslmode=disable user=postgres -c REVOKE ALL ON DATABASE "hit128-restore" FROM PUBLIC; GRANT CONNECT ON DATABASE "hit128-restore" TO hp_processing_app,hp_processing_migrator,hp_processing_worker,hp_processing_purge
    recovery_process_test.go:205: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-4175273891 port=55432 dbname=hit128-restore sslmode=disable user=hp_processing_migrator -f /migrations/apply.sql
    recovery_process_test.go:205: exit=0 /usr/bin/psql -XAt -U postgres -d postgres -c SELECT system_identifier FROM pg_control_system()
    recovery_process_test.go:205: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-4175273891 port=55432 dbname=postgres sslmode=disable user=postgres -At -c SELECT system_identifier FROM pg_control_system()
    recovery_process_test.go:205: different cluster system identifiers: source=7710989911116935219 target=7710989946299289819
    recovery_process_test.go:205: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:205: restored safety: {Operation:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa Config:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb Digest:sha256:51b2440a0d2ec8d6adc410212231d7749c3d46cd3dbd137799bd8d9500f7a558 Case:ce7fcce6-c3ac-45d6-a4c3-6e3a1fa8c630 Lineage:206e4cf9-a8aa-4e03-8fc4-89c985bfbaee Target:86synthetic99 State:UNKNOWN Knowledge:TARGET_ID_KNOWN Fence:true Cases:1 Commands:1 Inbox:3 Intents:1 Queries:1}
    recovery_process_test.go:205: restored intent: state=HELD lease=false attempts=1 expiry_unchanged=true
    recovery_process_test.go:205: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:205: restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back
    recovery_process_test.go:162: ordinary configuration without authoritative stand inputs OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:12.845255544Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:162: target with stale boot snapshot OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:12.982761127Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:162: target with wrong generation OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:13.98176692Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:162: target with fresh boot and generation OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:14.870442087Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:205: fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:162: target, one missing detail OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:14.960973795Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:205: live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:205: exit=0 /test/processing-purge --local-stand --content-dir /tmp/TestProcessingRecoveryOwnedStand2561844832/001/content --workspace 4d51df89-3f2e-484c-a765-4ebdbeaea092 --limit 100
    recovery_process_test.go:205: purge result: {"held":0,"removed":2}
    recovery_process_test.go:162: target after expiry cleanup OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:39:15.021564878Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:163: Processing stopped by SIGKILL
    recovery_process_test.go:205: purge removed expired bytes while preserving the exact public history metadata and expiry
    recovery_process_test.go:205: RECOVERY PASS: controlled Delivery create calls=1, query calls before/after=0/0, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=3968
    recovery_cluster_test.go:81: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-4175273891/data -m fast -w stop
--- PASS: TestProcessingRecoveryOwnedStand (4.11s)
PASS
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### native-regression / predecessor-process-tests

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:39:15.155910+00:00

Exit code 0.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT126_PURGE_BIN=/test/processing-purge", "6e44e2e75e3dec76e6031f376a5bbf1852782f6ba0b91f503c5b76459c91e957", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=145s", "-test.run=^TestProcessing(Executable|CaseWorker|Recheck|CaseTimeouts|Purge|Diagnostics)OwnedStand$"]
```

stdout, 5614 байт, SHA-256 `58dc847fcc08ca1319be9cd6763c23a4d5ac19e3dfad0f0676858b66d3d68b2a`.

Полный вывод с исходным пробельным форматом находится в архиве.

stderr, 216 байт, SHA-256 `cddb591421f96f5ea3d3fba18470006f5da7debad2d3cb6ca84e59cd244f32f5`.

```text
2026/11/22 13:39:31 http: TLS handshake error from 127.0.0.1:47494: EOF
2026/11/22 13:40:09 http: TLS handshake error from 127.0.0.1:58392: EOF
2026/11/22 13:40:11 http: TLS handshake error from 127.0.0.1:58404: EOF
```
