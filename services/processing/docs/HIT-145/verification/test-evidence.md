# HIT-145. Сборка и запуск

Проверенный исходник `30385d09962c3fa4aad4808253a5493235559d01`, оба checkout чистые. Команды с cwd, SHA, UTC, exit code и хешами находятся в [сырых журналах](original-runs.tar.gz) и [манифесте](original-runs-manifest.json). [Полные выборочные выводы и команды Docker](image-details.json)

| Запуск | Начало UTC | Конец UTC | Команд |
| --- | --- | --- | ---: |
| Рабочая ветка | 2026-11-21T07:04:34.683860+00:00 | 2026-11-21T07:04:48.302180+00:00 | 82 |
| Новый detached worktree | 2026-11-21T07:04:55.278913+00:00 | 2026-11-21T07:05:03.207553+00:00 | 80 |

В обоих случаях runner и два контейнера Processing завершились с кодом 0. В командах опроса и проверках отсутствия удалённых ресурсов ожидаются ненулевые коды. Контейнеры и тома удалены.

- Go 1.27.2 builder index `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`; PostgreSQL image index `sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`. Реальный `postgres --version`: `postgres (PostgreSQL) 18.7 (Debian 18.7-1.pgdg12+2)`. Настройки: `180007|on|on|on|` (server version, fsync, full-page writes, synchronous commit, пустой listen_addresses).
- `linux/arm64`. Manifest `sha256:972262cf9f8c50dd5a1d9009999b8d6b0177d1df23b5be555e2ccdd4df2f0a33`; config `sha256:7c932292a0cc393e273dfad2cfa246e8a0b24a316be0a4845f35da281e1c592b`; layer `sha256:08289da242c3f6457a39618ab7dcbdf1217f269674e9fc8c535ce7c50f6f9ab2`; binary `sha256:ff48719d2e60df6e24f857f3a2dd75c9fd6b753abe3cd77080056c19aaada511`.
- Архивы `sha256:801a4a4812c0fa81848c8554ee130b19f31d2512662c6398ed2705bb1671a7ed` и `sha256:e1ae2e256b6da0c23d16443276426f8c7363ba07a1d077ebc976e046e5190954`. Дайджесты манифеста, конфигурации, слоя и бинарника совпали. В экспортных tar разные локальные теги. Каждый архив проверен до `docker image load`. Слой содержит только исполняемый `processing`. TLS и конфигурация вне слоя.
- Реальный UID 10001, read-only root, собственный RW content volume, ro config и сокет выделенной PostgreSQL. Сеть выделенной базы `none`; Processing использует её приватный namespace. Сокет Docker и host PID отсутствуют. Проверено `docker inspect` и `docker top` для обоих запусков в каждом стенде.
- Health проверен принятой схемой и разрешённым OPS mTLS. Перезапуск использовал ту же базу и том. Старая регистрация давала CLOSED и отказ, свежая — другой boot ID и READY. Повтор дал побайтно прежнюю квитанцию и прежние непустые элементы истории. Тестовый outbox создан существующим механизмом подготовки. Учитывались SQL-состояние и число HTTP-вызовов управляемого Delivery.

Команда первой сборки (exit 0), cwd `/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit145-processing-oci`:

```sh
docker buildx build --load --platform linux/arm64 --provenance=false --sbom=false --build-arg SOURCE_SHA=30385d09962c3fa4aad4808253a5493235559d01 --build-arg SOURCE_DATE_EPOCH=0 --metadata-file /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/research/HIT-145-20261121/stand-debug/build-metadata.json --file services/processing/Dockerfile --tag hit145-e276949be98d4e758d9a056a46d5dc29:test .
```

Первый запуск, полный stdout:

```text
=== RUN   TestProcessingOCIImageOwnedStand
    image_process_test.go:116: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:04:43.95841876Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:125: Negative HTTP checks: CLOSED business 503, non-OPS Health 403, missing mTLS certificate refused
    image_process_test.go:128: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:04:44.926737844Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:167: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:04:45.848273469Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:170: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:04:46.790196553Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:197: RESTART PASS: case=4e846eb2-619c-4af2-bea5-849ebafc1a6e message=a99bd3d2-ef86-47c9-8a83-25252fce0bc2 fresh_boot=true history_items=1 receipt_sha256=ce062bc07e9141bd742def7aa3c1ac2ff421f2775af1c7d5863b8470eec77df3 durable={Cases:1 Inbound:1 Outbound:1 Attempts:1 DeliveryState:ACKED} delivery_calls=1
--- PASS: TestProcessingOCIImageOwnedStand (3.99s)
PASS
```

Новая копия, полный stdout:

```text
=== RUN   TestProcessingOCIImageOwnedStand
    image_process_test.go:116: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:04:59.068155336Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:125: Negative HTTP checks: CLOSED business 503, non-OPS Health 403, missing mTLS certificate refused
    image_process_test.go:128: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:05:00.052136212Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:167: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:05:00.700462004Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:170: OPS mTLS HTTP 200 {"build_revision":"30385d09962c3fa4aad4808253a5493235559d01","checked_at":"2026-11-21T07:05:01.621207379Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:197: RESTART PASS: case=101d2c02-e313-4853-8917-5f7b0f3d979d message=d32500ea-bde4-4570-8d28-700c952e0ef9 fresh_boot=true history_items=1 receipt_sha256=6aac58cd166102bd52590cf74881acdc31c40273cf17ba50dd22ebfef2c283ad durable={Cases:1 Inbound:1 Outbound:1 Attempts:1 DeliveryState:ACKED} delivery_calls=1
--- PASS: TestProcessingOCIImageOwnedStand (3.71s)
PASS
```

Два прогона использовали один локальный Docker Engine и его кэш сборки. Проверенные контрагенты управляемые, PostgreSQL реальная. [Инструкция и доверенные входы](../RUN_RU.md)
