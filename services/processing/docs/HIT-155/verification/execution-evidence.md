# Изолированная проверка образа и восстановления HIT-155

Codex по делегации Тимофея Патрушева. PostgreSQL 18.7 и управляемые mTLS-соседи в выделенной сети без внешнего доступа. Все идентификаторы, показанные в прогонах, синтетические.

Проверенный исходник `1ed91653a747b47a98f2738ca0d831a38cebc98a`. Архитектура `linux/arm64`. Все восемь стартов использовали `sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51`. Каждый контейнер запущен с UID/GID 10001, read-only root, cap-drop ALL, no-new-privileges и отдельным томом содержимого.

Фактическая идентичность загруженного образа.

```json
{
  "source_sha": "1ed91653a747b47a98f2738ca0d831a38cebc98a",
  "platform": "linux/arm64",
  "manifest_digest": "sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51",
  "manifest_media_type": "application/vnd.docker.distribution.manifest.v2+json",
  "config_digest": "sha256:e08b314237542793014efc46085b97e0d917c098e8f35d6ad8dcb72a706c7d3c",
  "layer_digest": "sha256:9d2bb901d57945c942d472aad8df258ba6b26a268cfe2d431b8d6e6bc3a3e841",
  "files": [
    "processing"
  ],
  "binary_sha256": "sha256:ba34ddd3a919d3880cb43074e0bf020da5d44f9fe264f947a35a19c9705e7a6d",
  "archive_sha256": "sha256:d9d3886b6d0c1353e77e219d9ccabc7a1e4d0d1af6ab98d856b1551d807d6390",
  "config": {
    "User": "10001:10001",
    "ExposedPorts": {
      "8443/tcp": {},
      "9443/tcp": {},
      "9444/tcp": {}
    },
    "Env": [
      "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
    ],
    "Entrypoint": [
      "/processing"
    ],
    "Cmd": [
      "-config",
      "/etc/processing/processing.json"
    ],
    "WorkingDir": "/",
    "Labels": {
      "org.opencontainers.image.revision": "1ed91653a747b47a98f2738ca0d831a38cebc98a",
      "org.opencontainers.image.source": "https://github.com/hitpipe/hitpipe",
      "org.opencontainers.image.title": "HitPipe Processing"
    },
    "ArgsEscaped": true
  },
  "engine_image_id": "sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51"
}
```

Повторная сборка из отдельной чистой копии дала те же манифест, конфигурацию, слой и бинарник. В экспортных tar отличаются только index.json и manifest.json, содержащие разные временные имена тестового тега.

```json
{
  "identity_match": {
    "manifest_digest": true,
    "config_digest": true,
    "layer_digest": true,
    "binary_sha256": true,
    "archive_sha256": false
  },
  "original_archive_sha256": "sha256:d9d3886b6d0c1353e77e219d9ccabc7a1e4d0d1af6ab98d856b1551d807d6390",
  "changed_tar_members": {
    "manifest.json": [
      [
        "4ee4c5c3771a5800f61a3d8daf26d016140172eea2f80322c9fec8a181e616d2",
        0
      ],
      [
        "4b1477d80307cbea7bd4c662c7e137d203ae7c73a29d56e8bfc84e9ddbce44a1",
        0
      ]
    ],
    "index.json": [
      [
        "c179c49154bb1e1afebe9cd45e831c8211ca7e92d4b7b8cc03ce316b775408ee",
        0
      ],
      [
        "7cae3377dbe3dbecb1bf5efe37470e10c9a8d270474a2ad1abb6676ae6304ebf",
        0
      ]
    ]
  },
  "identity": {
    "source_sha": "1ed91653a747b47a98f2738ca0d831a38cebc98a",
    "platform": "linux/arm64",
    "manifest_digest": "sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51",
    "manifest_media_type": "application/vnd.docker.distribution.manifest.v2+json",
    "config_digest": "sha256:e08b314237542793014efc46085b97e0d917c098e8f35d6ad8dcb72a706c7d3c",
    "layer_digest": "sha256:9d2bb901d57945c942d472aad8df258ba6b26a268cfe2d431b8d6e6bc3a3e841",
    "files": [
      "processing"
    ],
    "binary_sha256": "sha256:ba34ddd3a919d3880cb43074e0bf020da5d44f9fe264f947a35a19c9705e7a6d",
    "archive_sha256": "sha256:cd544b781096bf21bab6521fbb76451fb8bdf97967f7f5cd61d71d1ab2274818",
    "config": {
      "User": "10001:10001",
      "ExposedPorts": {
        "8443/tcp": {},
        "9443/tcp": {},
        "9444/tcp": {}
      },
      "Env": [
        "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
      ],
      "Entrypoint": [
        "/processing"
      ],
      "Cmd": [
        "-config",
        "/etc/processing/processing.json"
      ],
      "WorkingDir": "/",
      "Labels": {
        "org.opencontainers.image.revision": "1ed91653a747b47a98f2738ca0d831a38cebc98a",
        "org.opencontainers.image.source": "https://github.com/hitpipe/hitpipe",
        "org.opencontainers.image.title": "HitPipe Processing"
      },
      "ArgsEscaped": true
    }
  }
}
```

[Приложение с исходными stdout/stderr и полными командами](raw-execution.tar.gz), SHA-256 `08b72546ce1a7829c246d3a8d6c8c1c9477f70d5f4330c7629732ce414d944f8`. Стартовые служебные логи PostgreSQL исключены. Контроль версий, состояния, очистки и все значимые журналы включены.

### qualification-4 / build-image

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:13.540797+00:00

Exit code 0.

```json
["docker", "buildx", "build", "--load", "--platform", "linux/arm64", "--provenance=false", "--sbom=false", "--build-arg", "SOURCE_SHA=1ed91653a747b47a98f2738ca0d831a38cebc98a", "--build-arg", "SOURCE_DATE_EPOCH=0", "--metadata-file", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/research/HIT-155-20261122/qualification-4/build-metadata.json", "--file", "services/processing/Dockerfile", "--tag", "hit155-05dbefb631bb49a99cf1edaca86c3c35:test", "."]
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 2124 байт, SHA-256 `a45d553aed9bfd6590cfe0c5a4b0ea5d1fd0000006454d3771584483dbcb433b`.

```text
#0 building with "desktop-linux" instance using docker driver

#1 [internal] load build definition from Dockerfile
#1 transferring dockerfile: 1.68kB done
#1 DONE 0.0s

#2 [internal] load metadata for docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
#2 DONE 0.0s

#3 [build 1/9] FROM docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
#3 resolve docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 done
#3 DONE 0.0s

#4 [internal] load build context
#4 transferring context: 10.91kB done
#4 DONE 0.0s

#5 [build 2/9] WORKDIR /src/services/processing
#5 CACHED

#6 [build 3/9] COPY services/processing/go.mod services/processing/go.sum ./
#6 CACHED

#7 [build 5/9] RUN go mod download && go mod verify
#7 CACHED

#8 [build 6/9] COPY services/platform ../platform
#8 CACHED

#9 [build 7/9] COPY services/processing/cmd/processing ./cmd/processing
#9 CACHED

#10 [build 4/9] COPY services/platform/go.mod ../platform/go.mod
#10 CACHED

#11 [build 8/9] COPY services/processing/internal ./internal
#11 CACHED

#12 [build 9/9] RUN test "${#SOURCE_SHA}" -eq 40 &&     case "$SOURCE_SHA" in *[!0-9a-f]*) exit 1;; esac &&     CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false         -ldflags "-X main.revision=$SOURCE_SHA" -o /out/processing ./cmd/processing &&     touch -d '@0' /out/processing
#12 DONE 4.2s

#13 [stage-1 1/1] COPY --from=build /out/processing /processing
#13 DONE 0.0s

#14 exporting to image
#14 exporting layers
#14 exporting layers 0.3s done
#14 exporting manifest sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51 done
#14 exporting config sha256:e08b314237542793014efc46085b97e0d917c098e8f35d6ad8dcb72a706c7d3c done
#14 naming to docker.io/library/hit155-05dbefb631bb49a99cf1edaca86c3c35:test done
#14 unpacking to docker.io/library/hit155-05dbefb631bb49a99cf1edaca86c3c35:test 0.0s done
#14 DONE 0.4s

#15 resolving provenance for metadata file
#15 DONE 0.0s
```

### qualification-4 / postgres-version

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:20.879316+00:00

Exit code 0.

```json
["docker", "exec", "36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360", "postgres", "--version"]
```

stdout, 52 байт, SHA-256 `605b026879b9a291e0d851478a6f4dc23353423fa6f7035ce9e58a1e3a2e9e7a`.

```text
postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / postgres-settings

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:20.939439+00:00

Exit code 0.

```json
["docker", "exec", "-i", "36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360", "psql", "-X", "-q", "-A", "-t", "-v", "VERBOSITY=verbose", "-v", "ON_ERROR_STOP=1", "-U", "postgres", "-d", "postgres"]
```

stdout, 17 байт, SHA-256 `7ad450c52f1bab5c7f37b83c8bba2ed122e06a1a3e00e706c2ddca3efd95ab08`.

```text
180007|on|on|on|
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / driver-stdout

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:31.270731+00:00

Exit code 0.

```json
["docker", "exec", "36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360", "cat", "/tmp/hit145-control/driver.stdout"]
```

stdout, 1561 байт, SHA-256 `6cd1cf74c4e410cbcfc475b72dface2304e03b9cc65808dd2cb976026424f3a1`.

```text
=== RUN   TestProcessingOCIImageOwnedStand
    image_process_test.go:140: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:27.355087884Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:149: Negative HTTP checks: CLOSED business 503, non-OPS Health 403, missing mTLS certificate refused
    image_process_test.go:152: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:28.348977551Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:168: baseline history captured after the controlled Intake failure was durably recorded
    image_process_test.go:191: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:29.233183677Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:194: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:30.173151761Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:226: RESTART PASS: case=ec1ea50b-66e6-4835-9be3-daabb42ec5ba message=4867e867-c728-4220-b5fe-f064f6854087 fresh_boot=true history_items=2 receipt_sha256=6e1a0f1c92322608075ee7f254a2e7022cf69ee147ad14aff86d387b8a258d20 durable={Cases:1 Inbound:1 Outbound:1 Attempts:1 DeliveryState:ACKED} delivery_calls=1
--- PASS: TestProcessingOCIImageOwnedStand (3.98s)
PASS
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / recovery-stdout

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:39.690007+00:00

Exit code 0.

```json
["docker", "exec", "36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360", "cat", "/tmp/hit145-control/recovery.stdout"]
```

stdout, 8663 байт, SHA-256 `0ca4bdd60016b9b290f678e15e4c0afc8ca40c0d443b6fa84f1e08d5ce2bae1b`.

```text
=== RUN   TestProcessingOCIRecoveryOwnedStand
    oci_recovery_process_test.go:34: original
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:31.852777303Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    oci_recovery_process_test.go:34: original
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:32.802251387Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    oci_recovery_process_test.go:22: SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent
    oci_recovery_process_test.go:34: cold restart, same database
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:33.720147262Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.operations: rows=1
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.accepted_configs: rows=1
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.case_identity: rows=1
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.episodes: rows=1
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.case_content: rows=0
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.safety_history: rows=5
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.history_content: rows=5
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.user_commands: rows=1
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.inbox: rows=3
    oci_recovery_process_test.go:22: scoped SELECT hp_processing.outbox: rows=2
    oci_recovery_process_test.go:22: source intent state=CLAIMED lease=true attempts=1; known identity and fence persisted
    oci_recovery_process_test.go:22: source safety: {Operation:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa Config:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb Digest:sha256:51b2440a0d2ec8d6adc410212231d7749c3d46cd3dbd137799bd8d9500f7a558 Case:4e11c899-643d-4546-a28a-d08fd58bd756 Lineage:d1d213ee-c4d1-49ae-ae03-9a861fac78ff Target:86synthetic99 State:UNKNOWN Knowledge:TARGET_ID_KNOWN Fence:true Cases:1 Commands:1 Inbox:3 Intents:1 Queries:1}
    oci_recovery_process_test.go:22: exit=0 /usr/bin/chown -R postgres:postgres /tmp/hit128-target-76969218
    oci_recovery_process_test.go:22: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/initdb -D /tmp/hit128-target-76969218/data --auth-local=trust --auth-host=trust
    oci_recovery_process_test.go:22: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-76969218/data -l /tmp/hit128-target-76969218/postgres.log -o -k /tmp/hit128-target-76969218 -p 55432 -h 127.0.0.5 -c fsync=on -c full_page_writes=on -c synchronous_commit=on -c ssl=on -c ssl_cert_file=/tmp/hit128-target-76969218/server.crt -c ssl_key_file=/tmp/hit128-target-76969218/server.key -w start
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-76969218 port=55432 dbname=postgres sslmode=disable user=postgres -f /migrations/roles.sql
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-76969218 port=55432 dbname=postgres sslmode=disable user=postgres -c CREATE DATABASE "hit128-restore" OWNER hp_processing_owner
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-76969218 port=55432 dbname=postgres sslmode=disable user=postgres -c REVOKE ALL ON DATABASE "hit128-restore" FROM PUBLIC; GRANT CONNECT ON DATABASE "hit128-restore" TO hp_processing_app,hp_processing_migrator,hp_processing_worker,hp_processing_purge
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-76969218 port=55432 dbname=hit128-restore sslmode=disable user=hp_processing_migrator -f /migrations/apply.sql
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -XAt -U postgres -d postgres -c SELECT system_identifier FROM pg_control_system()
    oci_recovery_process_test.go:22: exit=0 /usr/bin/psql -X -v ON_ERROR_STOP=1 -d host=/tmp/hit128-target-76969218 port=55432 dbname=postgres sslmode=disable user=postgres -At -c SELECT system_identifier FROM pg_control_system()
    oci_recovery_process_test.go:22: different cluster system identifiers: source=7710989721967259698 target=7710989782614000155
    oci_recovery_process_test.go:22: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    oci_recovery_process_test.go:22: restored safety: {Operation:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa Config:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb Digest:sha256:51b2440a0d2ec8d6adc410212231d7749c3d46cd3dbd137799bd8d9500f7a558 Case:4e11c899-643d-4546-a28a-d08fd58bd756 Lineage:d1d213ee-c4d1-49ae-ae03-9a861fac78ff Target:86synthetic99 State:UNKNOWN Knowledge:TARGET_ID_KNOWN Fence:true Cases:1 Commands:1 Inbox:3 Intents:1 Queries:1}
    oci_recovery_process_test.go:22: restored intent: state=HELD lease=false attempts=1 expiry_unchanged=true
    oci_recovery_process_test.go:22: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    oci_recovery_process_test.go:22: restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back
    oci_recovery_process_test.go:34: ordinary configuration without authoritative stand inputs
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:35.17942243Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    oci_recovery_process_test.go:34: target with stale boot snapshot
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:35.859156513Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    oci_recovery_process_test.go:34: target with wrong generation
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:36.779421597Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    oci_recovery_process_test.go:34: target with fresh boot and generation
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:37.669064083Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    oci_recovery_process_test.go:22: fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded
    oci_recovery_process_test.go:34: target, one missing detail
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:38.496935334Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    oci_recovery_process_test.go:22: live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE
    oci_recovery_process_test.go:22: exit=0 /test/processing-purge --local-stand --content-dir /var/lib/hitpipe/processing-content --workspace 5c83a851-14e1-40b0-bd32-79b86a904154 --limit 100
    oci_recovery_process_test.go:22: purge result: {"held":0,"removed":3}
    oci_recovery_process_test.go:34: target after expiry cleanup
    oci_recovery_process_test.go:35: OPS mTLS HTTP 200 {"build_revision":"1ed91653a747b47a98f2738ca0d831a38cebc98a","checked_at":"2026-11-22T13:38:39.097574834Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    oci_recovery_process_test.go:22: purge removed expired bytes while preserving the exact public history metadata and expiry
    oci_recovery_process_test.go:22: RECOVERY PASS: controlled Delivery create calls=1, query calls before/after=0/0, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=7800
    recovery_cluster_test.go:81: exit=0 /usr/sbin/runuser -u postgres -- /usr/lib/postgresql/18/bin/pg_ctl -D /tmp/hit128-target-76969218/data -m fast -w stop
--- PASS: TestProcessingOCIRecoveryOwnedStand (7.93s)
PASS
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / processing-isolation-recovery-1

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:31.764061+00:00

Exit code 0.

```json
["docker", "inspect", "--format", "{{json .Config.User}}\n{{json .HostConfig.ReadonlyRootfs}}\n{{json .HostConfig.PidMode}}\n{{json .HostConfig.Privileged}}\n{{json .HostConfig.NetworkMode}}\n{{json .HostConfig.CapDrop}}\n{{json .HostConfig.SecurityOpt}}\n{{json .Mounts}}\n{{json .Image}}", "hit155-05dbefb631bb49a99cf1edaca86c3c35-processing-recovery-1"]
```

stdout, 990 байт, SHA-256 `d23d33a52ee3163afa2a4558e319ed2545e9bab6d9b4945ef4f03916caf2398a`.

```text
"10001:10001"
true
""
false
"container:36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360"
["ALL"]
["no-new-privileges:true"]
[{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-config","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-config/_data","Destination":"/hit145/config","Driver":"local","Mode":"z","RW":false,"Propagation":""},{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-socket","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-socket/_data","Destination":"/var/run/postgresql","Driver":"local","Mode":"z","RW":false,"Propagation":""},{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-content","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-content/_data","Destination":"/var/lib/hitpipe/processing-content","Driver":"local","Mode":"z","RW":true,"Propagation":""}]
"sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51"
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### qualification-4 / processing-isolation-recovery-6

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-processing-oci-recovery

Начало UTC 2026-11-22T13:38:39.007186+00:00

Exit code 0.

```json
["docker", "inspect", "--format", "{{json .Config.User}}\n{{json .HostConfig.ReadonlyRootfs}}\n{{json .HostConfig.PidMode}}\n{{json .HostConfig.Privileged}}\n{{json .HostConfig.NetworkMode}}\n{{json .HostConfig.CapDrop}}\n{{json .HostConfig.SecurityOpt}}\n{{json .Mounts}}\n{{json .Image}}", "hit155-05dbefb631bb49a99cf1edaca86c3c35-processing-recovery-6"]
```

stdout, 990 байт, SHA-256 `d23d33a52ee3163afa2a4558e319ed2545e9bab6d9b4945ef4f03916caf2398a`.

```text
"10001:10001"
true
""
false
"container:36d83089f68d5f6858051e88bf34040d9bc0ee2c197c98fad54dd6f7cf89a360"
["ALL"]
["no-new-privileges:true"]
[{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-config","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-config/_data","Destination":"/hit145/config","Driver":"local","Mode":"z","RW":false,"Propagation":""},{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-socket","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-socket/_data","Destination":"/var/run/postgresql","Driver":"local","Mode":"z","RW":false,"Propagation":""},{"Type":"volume","Name":"hit155-05dbefb631bb49a99cf1edaca86c3c35-content","Source":"/var/lib/docker/volumes/hit155-05dbefb631bb49a99cf1edaca86c3c35-content/_data","Destination":"/var/lib/hitpipe/processing-content","Driver":"local","Mode":"z","RW":true,"Propagation":""}]
"sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51"
```

stderr, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

### independent-rebuild / build-image

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit155-reproduction

Начало UTC 2026-11-22T13:39:23.069570+00:00

Exit code 0.

```json
["docker", "buildx", "build", "--load", "--platform", "linux/arm64", "--provenance=false", "--sbom=false", "--build-arg", "SOURCE_SHA=1ed91653a747b47a98f2738ca0d831a38cebc98a", "--build-arg", "SOURCE_DATE_EPOCH=0", "--metadata-file", "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/research/HIT-155-20261122/independent-rebuild/build-metadata.json", "--file", "services/processing/Dockerfile", "--tag", "hit155-rebuild-67aad54d93f245f8ad5c797d9dc014e7:test", "."]
```

stdout, 0 байт, SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

stderr, 2114 байт, SHA-256 `46165464afa6e01cbb85d7accec619a6e874efa39afa005967f4daf9a2ad6117`.

```text
#0 building with "desktop-linux" instance using docker driver

#1 [internal] load build definition from Dockerfile
#1 transferring dockerfile: 1.68kB done
#1 DONE 0.0s

#2 [internal] load metadata for docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
#2 DONE 0.0s

#3 [build 1/9] FROM docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
#3 resolve docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 done
#3 DONE 0.0s

#4 [internal] load build context
#4 transferring context: 613.92kB 0.0s done
#4 DONE 0.0s

#5 [build 3/9] COPY services/processing/go.mod services/processing/go.sum ./
#5 CACHED

#6 [build 9/9] RUN test "${#SOURCE_SHA}" -eq 40 &&     case "$SOURCE_SHA" in *[!0-9a-f]*) exit 1;; esac &&     CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false         -ldflags "-X main.revision=$SOURCE_SHA" -o /out/processing ./cmd/processing &&     touch -d '@0' /out/processing
#6 CACHED

#7 [build 8/9] COPY services/processing/internal ./internal
#7 CACHED

#8 [build 4/9] COPY services/platform/go.mod ../platform/go.mod
#8 CACHED

#9 [build 6/9] COPY services/platform ../platform
#9 CACHED

#10 [build 2/9] WORKDIR /src/services/processing
#10 CACHED

#11 [build 5/9] RUN go mod download && go mod verify
#11 CACHED

#12 [build 7/9] COPY services/processing/cmd/processing ./cmd/processing
#12 CACHED

#13 [stage-1 1/1] COPY --from=build /out/processing /processing
#13 CACHED

#14 exporting to image
#14 exporting layers done
#14 exporting manifest sha256:91a2b1b19d0d22fe783b74c3ecf338dd0f3e20a2863a3b3cdb061d4b69312e51 done
#14 exporting config sha256:e08b314237542793014efc46085b97e0d917c098e8f35d6ad8dcb72a706c7d3c done
#14 naming to docker.io/library/hit155-rebuild-67aad54d93f245f8ad5c797d9dc014e7:test done
#14 unpacking to docker.io/library/hit155-rebuild-67aad54d93f245f8ad5c797d9dc014e7:test 0.0s done
#14 DONE 0.1s

#15 resolving provenance for metadata file
#15 DONE 0.0s
```
