# HIT-146 — запуски на 717c954fca4f06f165074a8688f0e94a31b398e7

IAM: e1be0c1a0be577a3626a3e120707b3aeeb579c6f. Начало и конец каждого запуска в UTC. Пакет полных потоков и команд: raw-local.tar.gz (SHA-256 8e813018980f9ca3ea26e934bdc840733b06a42648cc08b3f3bc934c66fad60a).

## docker-version

```json
{"label": "docker-version", "argv": ["docker", "version", "--format", "{{json .}}"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:29.055289+00:00", "finished_at": "2026-11-22T19:39:29.080821+00:00", "exit_code": 0, "stdout": {"file": "001-docker-version.stdout", "bytes": 1042, "sha256": "792a658afd583f48031db1ef0940136d5c31e8c5e8ca14787c6e70c5b5609780"}, "stderr": {"file": "001-docker-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## postgres-version

```json
{"label": "postgres-version", "argv": ["docker", "exec", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e", "postgres", "--version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:29.945139+00:00", "finished_at": "2026-11-22T19:39:29.998061+00:00", "exit_code": 0, "stdout": {"file": "008-postgres-version.stdout", "bytes": 52, "sha256": "605b026879b9a291e0d851478a6f4dc23353423fa6f7035ce9e58a1e3a2e9e7a"}, "stderr": {"file": "008-postgres-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

## go-version

```json
{"label": "go-version", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "version"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:30.590491+00:00", "finished_at": "2026-11-22T19:39:30.621626+00:00", "exit_code": 0, "stdout": {"file": "019-go-version.stdout", "bytes": 33, "sha256": "94e911f9fbf3c118f6a0573ab3a13fa546ad13e89ba40cf53df77e468509c92d"}, "stderr": {"file": "019-go-version.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
go version go1.27.2 darwin/arm64
```

## go-race

```json
{"label": "go-race", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-race", "-count=1", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:30.622186+00:00", "finished_at": "2026-11-22T19:39:35.129482+00:00", "exit_code": 0, "stdout": {"file": "020-go-race.stdout", "bytes": 2895, "sha256": "367f906a69ce228295555760c794659996404cb6052adae5ae1173446d181050"}, "stderr": {"file": "020-go-race.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.150s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.399s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.403s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.418s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.352s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.263s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.243s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	1.739s
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.157s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.223s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.289s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.163s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.128s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.133s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.149s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.131s
?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.138s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.215s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.196s
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.210s
?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
```

## go-vet

```json
{"label": "go-vet", "argv": ["env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "vet", "./..."], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:35.130118+00:00", "finished_at": "2026-11-22T19:39:35.288902+00:00", "exit_code": 0, "stdout": {"file": "021-go-vet.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "021-go-vet.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing

```json
{"label": "build-processing", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "build", "-trimpath", "-ldflags", "-X main.revision=717c954fca4f06f165074a8688f0e94a31b398e7", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-va00npxk/processing", "./cmd/processing"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:35.289349+00:00", "finished_at": "2026-11-22T19:39:35.959925+00:00", "exit_code": 0, "stdout": {"file": "022-build-processing.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "022-build-processing.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-processing-tests

```json
{"label": "build-processing-tests", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation/services/processing", "test", "-c", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-va00npxk/storage.test", "./internal/adapters/postgres"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:35.960427+00:00", "finished_at": "2026-11-22T19:39:36.738738+00:00", "exit_code": 0, "stdout": {"file": "023-build-processing-tests.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "023-build-processing-tests.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-iam-probe

```json
{"label": "build-iam-probe", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92/services/iam", "build", "-trimpath", "-tags", "hit146_iam_fixture", "-overlay", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-va00npxk/iam.probe.overlay.json", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-va00npxk/iam-probe", "./cmd/iam-contractcheck"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:36.739317+00:00", "finished_at": "2026-11-22T19:39:37.119935+00:00", "exit_code": 0, "stdout": {"file": "024-build-iam-probe.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "024-build-iam-probe.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## build-iam-migrate

```json
{"label": "build-iam-migrate", "argv": ["env", "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0", "env", "GOTOOLCHAIN=local", "GOWORK=off", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go", "-C", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92/services/iam", "build", "-trimpath", "-o", "/var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit146-build-va00npxk/iam-migrate", "./cmd/iam-migrate"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:37.120490+00:00", "finished_at": "2026-11-22T19:39:37.355867+00:00", "exit_code": 0, "stdout": {"file": "025-build-iam-migrate.stdout", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}, "stderr": {"file": "025-build-iam-migrate.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## iam-migration

```json
{"label": "iam-migration", "argv": ["docker", "exec", "-e", "HITPIPE_IAM_MIGRATOR_DSN=host=/var/run/postgresql user=hp_hit146_iam_migrate dbname=hit146_iam sslmode=disable options='-c role=hp_iam_migrator'", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e", "/test/iam-migrate"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:37.612600+00:00", "finished_at": "2026-11-22T19:39:37.694689+00:00", "exit_code": 0, "stdout": {"file": "029-iam-migration.stdout", "bytes": 114, "sha256": "d9be125712cd9d9d9a4fdb2cf4cfc83f9806f6828376340315ab9ce93b7fe75f"}, "stderr": {"file": "029-iam-migration.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## real-iam-boundary

```json
{"label": "real-iam-boundary", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT146_IAM_BIN=/test/iam-probe", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestRealIAMRegistrationBoundary$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:37.695291+00:00", "finished_at": "2026-11-22T19:39:46.630504+00:00", "exit_code": 0, "stdout": {"file": "030-real-iam-boundary.stdout", "bytes": 9365, "sha256": "9b39e66c686e8ec0af3186e2478ab95a668626952b2d98a98a974441f1ea2cb0"}, "stderr": {"file": "030-real-iam-boundary.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

```text
=== RUN   TestRealIAMRegistrationBoundary
    real_iam_process_test.go:209: HTTP 403 for unprovisioned boot bea6c462-ead8-49f7-bd65-74ebb9ce7582, protected HTTP 503 while CLOSED
    real_iam_process_test.go:238: actual IAM SYNCING snapshot {"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","generation":2,"boot_id":"6c880884-298e-4afe-aeb0-0a4df69eab7a","snapshot_revision":1795376377901264,"denials":[],"gate":"SYNCING"}
    real_iam_process_test.go:258: old generation: HTTP 403 and stale snapshot ErrIdentity. Wrong principal: mTLS rejected.
    real_iam_process_test.go:266: SYNCING → genuine IAM OPEN → protected HTTP 200; OPS {"build_revision":"717c954fca4f06f165074a8688f0e94a31b398e7","checked_at":"2026-11-22T19:39:38.927826924Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    real_iam_process_test.go:276: synthetic Delivery accepted first HTTP start 05f641b1-dd49-4e9b-9d89-5aa0de8eaafa and held its response
    real_iam_process_test.go:282: protected request after effective revocation: HTTP 403
    real_iam_process_test.go:292: held request after ACK and timeout: outbox=CLAIMED attempts=1 Delivery_HTTP_starts=1
    real_iam_process_test.go:315: reused provision rejected, absent Processing remained missing, IAM recorded INCIDENT
    real_iam_process_test.go:191: actual IAM process trace:
        {"time":"2026-11-22T19:39:37.768438882Z","level":"INFO","msg":"iam-listening"}
        {"time":"2026-11-22T19:39:37.822105716Z","level":"INFO","msg":"iam-register","gate":"","status":403,"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","boot_id":"bea6c462-ead8-49f7-bd65-74ebb9ce7582","generation":2}
        {"time":"2026-11-22T19:39:37.874703216Z","level":"INFO","msg":"iam-register","gate":"SYNCING","status":200,"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","boot_id":"6c880884-298e-4afe-aeb0-0a4df69eab7a","generation":2}
        {"time":"2026-11-22T19:39:37.901384882Z","level":"INFO","msg":"iam-register","gate":"SYNCING","status":200,"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","boot_id":"6c880884-298e-4afe-aeb0-0a4df69eab7a","generation":2}
        {"time":"2026-11-22T19:39:37.902267674Z","level":"INFO","msg":"iam-register","gate":"","status":403,"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","boot_id":"6c880884-298e-4afe-aeb0-0a4df69eab7a","generation":1}
        2026/11/22 19:39:37 http: TLS handshake error from 127.0.0.1:46478: privateiam: client certificate rejected
        {"time":"2026-11-22T19:39:37.910426132Z","level":"INFO","msg":"iam-gate-open"}
        {"time":"2026-11-22T19:39:38.878482841Z","level":"INFO","msg":"iam-register","gate":"OPEN","status":200,"runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","boot_id":"6c880884-298e-4afe-aeb0-0a4df69eab7a","generation":2}
        {"time":"2026-11-22T19:39:38.953722758Z","level":"INFO","msg":"iam-http","status":200,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:39.089023008Z","level":"INFO","msg":"iam-http","status":503,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:39.103326591Z","level":"INFO","msg":"iam-http","status":200,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:39.14133105Z","level":"INFO","msg":"coordinator-send","runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","generation":2,"ack_received":true,"ack":{"RevocationID":"e07ae5e1-0c48-42f4-96c7-5d65381b6265","RuntimeID":"b974bdda-0b40-44b8-9f64-541a0e48326b","BootID":"6c880884-298e-4afe-aeb0-0a4df69eab7a","Subject":{"WorkspaceID":"054279f7-45f0-445e-a628-65a95f2c7d9c","Scope":"WORKSPACE","SubjectID":"054279f7-45f0-445e-a628-65a95f2c7d9c"},"Generation":2,"AppliedEpoch":8,"OldStartSectionsDrained":true,"AcknowledgedAt":"2026-11-22T19:39:39.1411653Z"}}
        {"time":"2026-11-22T19:39:39.141916425Z","level":"INFO","msg":"iam-http","status":200,"path":"/internal/v1/runtime/barrier"}
        {"time":"2026-11-22T19:39:39.143388008Z","level":"INFO","msg":"coordinator-send","runtime_id":"eecf0e48-17d0-459b-bb62-295b3c90785b","generation":2,"ack_received":true,"ack":{"RevocationID":"e07ae5e1-0c48-42f4-96c7-5d65381b6265","RuntimeID":"eecf0e48-17d0-459b-bb62-295b3c90785b","BootID":"ac7ea8c2-c6fe-4fe8-8ceb-df046759c1dc","Subject":{"WorkspaceID":"054279f7-45f0-445e-a628-65a95f2c7d9c","Scope":"WORKSPACE","SubjectID":"054279f7-45f0-445e-a628-65a95f2c7d9c"},"Generation":2,"AppliedEpoch":8,"OldStartSectionsDrained":true,"AcknowledgedAt":"2026-11-22T19:39:39.141863133Z"}}
        {"time":"2026-11-22T19:39:39.1493963Z","level":"INFO","msg":"coordinator-drive","status":"EFFECTIVE","accepted_at":"2026-11-22T19:39:39.134108Z","deadline_at":"2026-11-22T19:39:44.134108Z","effective_at":"2026-11-22T19:39:39.149152Z","missing":[],"participants":[{"Identity":{"RuntimeID":"b974bdda-0b40-44b8-9f64-541a0e48326b","BootID":"6c880884-298e-4afe-aeb0-0a4df69eab7a","Generation":2},"CertificateFingerprint":"sha256:bcaf91b16ecc9af7bbcf1da0a499d64bc0c82f6369ac4001f99ce7cf6dc3b167","AcknowledgedAt":"2026-11-22T19:39:39.141165Z","AppliedEpoch":8},{"Identity":{"RuntimeID":"eecf0e48-17d0-459b-bb62-295b3c90785b","BootID":"ac7ea8c2-c6fe-4fe8-8ceb-df046759c1dc","Generation":2},"CertificateFingerprint":"sha256:fd1ca534935a86dc65cb4b4510e48bbd1e004627c570cc97b469bbe71b515748","AcknowledgedAt":"2026-11-22T19:39:39.141863Z","AppliedEpoch":8}],"epoch":8}
        {"time":"2026-11-22T19:39:39.1685798Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.622590967Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.6288773Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.681063675Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.68859555Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.887492634Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:40.895188717Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:41.085908217Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:41.092228759Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:41.280946592Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:41.287792092Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T19:39:41.516889759Z","level":"INFO","msg":"coordinator-send","runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","generation":2,"ack_received":false,"ack":{"RevocationID":"","RuntimeID":"","BootID":"","Subject":{"WorkspaceID":null,"Scope":"","SubjectID":""},"Generation":0,"AppliedEpoch":0,"OldStartSectionsDrained":false,"AcknowledgedAt":"0001-01-01T00:00:00Z"}}
        {"time":"2026-11-22T19:39:41.518980884Z","level":"INFO","msg":"iam-http","status":200,"path":"/internal/v1/runtime/barrier"}
        {"time":"2026-11-22T19:39:41.519106176Z","level":"INFO","msg":"coordinator-send","runtime_id":"eecf0e48-17d0-459b-bb62-295b3c90785b","generation":2,"ack_received":true,"ack":{"RevocationID":"9aa97289-51f5-4b82-b9ba-170f673cbc11","RuntimeID":"eecf0e48-17d0-459b-bb62-295b3c90785b","BootID":"ac7ea8c2-c6fe-4fe8-8ceb-df046759c1dc","Subject":{"WorkspaceID":null,"Scope":"SESSION","SubjectID":"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"},"Generation":2,"AppliedEpoch":8,"OldStartSectionsDrained":true,"AcknowledgedAt":"2026-11-22T19:39:41.518965426Z"}}
        {"time":"2026-11-22T19:39:41.520627717Z","level":"INFO","msg":"coordinator-drive","status":"PENDING","accepted_at":"2026-11-22T19:39:41.513186Z","deadline_at":"2026-11-22T19:39:46.513186Z","effective_at":null,"missing":["b974bdda-0b40-44b8-9f64-541a0e48326b"],"participants":[{"Identity":{"RuntimeID":"b974bdda-0b40-44b8-9f64-541a0e48326b","BootID":"6c880884-298e-4afe-aeb0-0a4df69eab7a","Generation":2},"CertificateFingerprint":"sha256:bcaf91b16ecc9af7bbcf1da0a499d64bc0c82f6369ac4001f99ce7cf6dc3b167","AcknowledgedAt":null,"AppliedEpoch":0},{"Identity":{"RuntimeID":"eecf0e48-17d0-459b-bb62-295b3c90785b","BootID":"ac7ea8c2-c6fe-4fe8-8ceb-df046759c1dc","Generation":2},"CertificateFingerprint":"sha256:fd1ca534935a86dc65cb4b4510e48bbd1e004627c570cc97b469bbe71b515748","AcknowledgedAt":"2026-11-22T19:39:41.518965Z","AppliedEpoch":8}],"epoch":8}
        {"time":"2026-11-22T19:39:46.610801928Z","level":"INFO","msg":"coordinator-send","runtime_id":"b974bdda-0b40-44b8-9f64-541a0e48326b","generation":2,"ack_received":false,"ack":{"RevocationID":"","RuntimeID":"","BootID":"","Subject":{"WorkspaceID":null,"Scope":"","SubjectID":""},"Generation":0,"AppliedEpoch":0,"OldStartSectionsDrained":false,"AcknowledgedAt":"0001-01-01T00:00:00Z"}}
        {"time":"2026-11-22T19:39:46.612021261Z","level":"INFO","msg":"coordinator-deadline","status":"INCIDENT","effective":false,"deadline_breached":true,"missing":["b974bdda-0b40-44b8-9f64-541a0e48326b"],"observed_at":"2026-11-22T19:39:46.612013178Z","elapsed_ms":5098}
--- PASS: TestRealIAMRegistrationBoundary (8.87s)
PASS
```

## processing-predecessor

```json
{"label": "processing-predecessor", "argv": ["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestProcessingExecutableOwnedStand$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:46.632754+00:00", "finished_at": "2026-11-22T19:39:49.911267+00:00", "exit_code": 0, "stdout": {"file": "031-processing-predecessor.stdout", "bytes": 1308, "sha256": "d6af5a8b3f23deed8b5d3cc876e2bfde415b64652bd0ce993d2eea911a9069d7"}, "stderr": {"file": "031-processing-predecessor.stderr", "bytes": 72, "sha256": "cc45b4077b3c319bbd28a010e1e8c871c0d37b017247ba8283f9d015253d494e"}}
```

```text
=== RUN   TestProcessingExecutableOwnedStand
    runtime_process_test.go:675: {"time":"2026-11-22T19:39:46.763722553Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
        {"time":"2026-11-22T19:39:47.770043804Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
        {"time":"2026-11-22T19:39:47.985436554Z","level":"INFO","msg":"processing outbox transition","message_id":"14cf4fad-b814-4f10-bd99-c0e037fb73fe","state":"ACKED"}
        
    runtime_process_test.go:675: {"time":"2026-11-22T19:39:48.588586262Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
        {"time":"2026-11-22T19:39:49.595437179Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
        {"time":"2026-11-22T19:39:49.892632263Z","level":"INFO","msg":"processing outbox transition","message_id":"9dfba3e0-9ba3-4ee2-8f11-f84bfb41399f","state":"ACKED"}
        {"time":"2026-11-22T19:39:49.892668471Z","level":"INFO","msg":"processing stopped"}
        
    runtime_process_test.go:677: PROCESS PASS: CLOSED→fresh IAM snapshot, mTLS allowlists, activation/digest, intake/history, prepared outbox/ACK, observation, pause/resume, barrier, kill/restart dedup and SIGTERM with in-flight ACK
--- PASS: TestProcessingExecutableOwnedStand (3.18s)
PASS
```

## cleanup

```json
{"label": "cleanup", "argv": ["docker", "rm", "-f", "-v", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:49.945972+00:00", "finished_at": "2026-11-22T19:39:50.001963+00:00", "exit_code": 0, "stdout": {"file": "033-cleanup.stdout", "bytes": 65, "sha256": "72051371f187ccbb5cc9b6db541e6151f83b9b9a5c6fec9f10c6a7fb9033153a"}, "stderr": {"file": "033-cleanup.stderr", "bytes": 0, "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}}
```

## cleanup-probe

```json
{"label": "cleanup-probe", "argv": ["docker", "container", "inspect", "ba1c15f9d3afaf62262ab69edaf0737d2b9e6ac7db92ce7fdbb450c22d04868e"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "717c954fca4f06f165074a8688f0e94a31b398e7", "tree_status_at_start": "", "runner_sha256": "644c124dda4133f03de5f633d2b1222052b2a4b084242bd9190a0239e12836f2", "started_at": "2026-11-22T19:39:50.002701+00:00", "finished_at": "2026-11-22T19:39:50.016000+00:00", "exit_code": 1, "stdout": {"file": "034-cleanup-probe.stdout", "bytes": 3, "sha256": "37517e5f3dc66819f61f5a7bb8ace1921282415f10551d2defa5c3eb0985b570"}, "stderr": {"file": "034-cleanup-probe.stderr", "bytes": 112, "sha256": "d2469e6824d2911105abc1dca3b1215b3c1640f8727db955b3740cb8c5ada153"}}
```

## Дайджесты бинарников

```json
{
  "processing_sha": "717c954fca4f06f165074a8688f0e94a31b398e7",
  "iam_sha": "e1be0c1a0be577a3626a3e120707b3aeeb579c6f",
  "platform": "linux/arm64",
  "fixture_main_sha256": "eeade981552382b9f6b71a88ecb8f03dd846b7c887bea064d3e9f4c881b846aa",
  "binaries": {
    "processing": "489d67a1a83705c17362208fd72749a91acced609f8daba651dd476d21802246",
    "storage.test": "a37496836e15454c4db21885af1708f74aa945a318400c3ec11c6a613de1c1ea",
    "iam-probe": "ffd558bf6aa3a7c29bc6626a56be442e3a3feabc14e976083b68f4b835366c9a",
    "iam-migrate": "73e353c9657e196e37ef3c48f6432ca4bca2a53957cb493bc574a3530c960b08"
  }
}
```
