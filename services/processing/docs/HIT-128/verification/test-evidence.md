# HIT-128. Протокол реального восстановления

Исходник `c426dc400a4bec03fafba72e2a5780c9d85af1e2`, чистое дерево. Среда сборки теста `linux/arm64`, host `go version go1.27.2 darwin/arm64`. Начало UTC 2026-11-21T07:41:26.832101+00:00, завершение UTC 2026-11-21T07:42:39.780684+00:00. Runner завершился с кодом 0. `postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)`; настройки первого кластера `180007|on|on|on|` (версия, fsync, full_page_writes, synchronous_commit, listen_addresses). Второй cluster system identifier отличается, PostgreSQL 18.7 подтверждён запросом через TLS с проверкой сертификата.

| DoD | Фактическое подтверждение |
| --- | --- |
| 1. Источники, состав и приватность | Ограниченный scoped SELECT десяти явно перечисленных таблиц, переход только SQL-метаданных в памяти. Секреты, пользовательское содержимое и файлы не экспортированы. SHA исходника, migration, бинарников и журналов сохранены |
| 2. Состояние и один эффект | Прежние case, canonical config digest, команда, intake/observation inbox, известный target, lineage и fence совпали. Повтор Intake вернул байты исходной квитанции; один create дошёл до управляемого получателя |
| 3. Закрытие и authority | Холодный перезапуск той же базы CLOSED. Обычный режим без доверенных входов CLOSED. Старый boot snapshot и другое поколение CLOSED. Свежий снимок даёт READY, а отозванная область получает 403 при обоих epoch. Старый claim в новом кластере лишён job |
| 4. Содержимое и сроки | Живые refs проверены на оригинальном томе. Отсутствующая или повреждённая деталь даёт 503 READ_UNAVAILABLE; истёкшие данные дают только прежние метаданные. После удаления байтов ответ истории совпадает. Сроки не изменились |
| 5. Прерывание и повтор | SIGKILL после fsync файла и перед SQL COMMIT сохранил только сироту. Незавершённые create и query восстановились HELD без старых leases и без ACKED/CONFIRMED. Повтор восстановления отклонён, чужая область откатилась. Очистка удалила сироту и истёкший файл |
| 6. Исполняемая проверка | Реально собранные Processing и purge, отдельный процесс тестов, PostgreSQL, mTLS и шесть предшествующих процессных регрессий. Host go race/vet и build завершились с кодом 0. Длительность локального сценария не является эксплуатационным RTO |

Параметры стенда и состав переноса описаны в [инструкции](../RUN_RU.md). Повторы не генерируют новое содержимое. Opaque proof ref, сохранённый в истории после expiry, является минимальной SQL-метаинформацией. Он не подтверждает доступность или актуальность истёкшего payload. Возможный эффект инициирован один раз синтетическим получателем, реальный провайдер не использовался.

Точный вызов Linux проверки из `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit128-processing-recovery`, exit 0:

```sh
docker exec -e HIT78_ISOLATED_PG=1 -e HIT128_RECOVERY_STAND=1 -e HIT111_PROCESSING_BIN=/test/processing -e HIT126_PURGE_BIN=/test/processing-purge 55347534a87a58b811a3df2c98dfe69a4b9de1df2006b9a34d060ee6a4a0369f /test/storage.test -test.v -test.count=1 -test.timeout=110s '-test.run=^TestProcessingRecoveryOwnedStand$'
```

Ключевые строки оригинального stdout:

```text
    recovery_process_test.go:201: original OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:36.429584966Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:203: original OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:37.452858383Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:242: SIGKILL after durable file and before SQL COMMIT: durable orphan present, both SQL rows absent
    recovery_process_test.go:266: cold restart, same database OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:37.755637967Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
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
    recovery_process_test.go:290: different cluster system identifiers: source=7710526668357406772 target=7710526714150498525
    recovery_process_test.go:291: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:318: target PostgreSQL server_version_num=180007 fsync=on full_page_writes=on synchronous_commit=on TLS verified
    recovery_process_test.go:322: restore identity/config/command/inbox/effect and fence preserved; create/recheck HELD, old claim has no job, expiry and attempts unchanged; repeat refused; foreign scope rolled back
    recovery_process_test.go:326: ordinary configuration without authoritative stand inputs OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:38.323129217Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:333: target with stale boot snapshot OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:38.452451425Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:338: target with wrong generation OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:39.467235676Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    recovery_process_test.go:340: target with fresh boot and generation OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:40.360508009Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:352: fresh registration required; revoked workspace denied at epochs 7 and 9; command, observation and exact Intake receipt replay succeeded
    recovery_process_test.go:384: target, one missing detail OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:40.45165376Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:393: live refs read successfully; expired detail is metadata-only; missing and corrupt live refs each returned HTTP 503 READ_UNAVAILABLE
    recovery_process_test.go:407: purge result: {"held":0,"removed":2}
    recovery_process_test.go:420: target after expiry cleanup OPS mTLS HTTP 200 {"build_revision":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","checked_at":"2026-11-21T07:41:40.512013885Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    recovery_process_test.go:427: purge removed expired bytes while preserving the exact public history metadata and expiry
    recovery_process_test.go:432: RECOVERY PASS: live provider create calls=1, no promoted ACK/CONFIRMED, cleanup preserved fence, duration_local_ms=4182
--- PASS: TestProcessingRecoveryOwnedStand (4.29s)
```

Регрессии принятых компонентов на этой же сборке:

```text
--- PASS: TestProcessingCaseWorkerOwnedStand (16.40s)
--- PASS: TestProcessingDiagnosticsOwnedStand (14.17s)
--- PASS: TestProcessingPurgeOwnedStand (0.13s)
--- PASS: TestProcessingRecheckOwnedStand (23.68s)
--- PASS: TestProcessingExecutableOwnedStand (3.19s)
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.41s)
```

[Полный stdout восстановления](recovery-process-tests.stdout) · [Полный stdout регрессий (gzip)](predecessor-process-tests.stdout.gz) · [Команды, коды и SHA-256 потоков](commands.json) · [Хеши бинарников](binaries.json) · [Сырые оригиналы](original-run.tar.gz) · [Манифест оригиналов](original-run-manifest.json)

SHA-256 архива `7b2eb8d7adb8f138459fb5bab97cc6c2a1a2267f56e99f6bcc3f028658c56709`.
