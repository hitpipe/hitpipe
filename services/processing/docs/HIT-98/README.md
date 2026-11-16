# HIT-98 — надёжная отправка сохранённого поручения

Реализован локальный отправщик Processing → Delivery. Все8 критериев договора проверены локально; официальный handoff и независимое ревью — отдельные шаги. Выполнено Codex по делегации Тимофея Патрушева / Tim1hik. Это не приёмка задачи и не квалификация всего MVP.

Проверенный код: `4d744101803caa864d94fed01d2ab7eaf854b9f9`. Основа HIT-79: `3bae6417421465868c432fe18b379d46be6144aa`. Наблюдённый main: `ce771d37505e7770003d0796ae932556d06b7bab`. PR45/48/50/52 и PR54 остаются OPEN: подготовленный стек не называется принятым main. Существующие ветки, работа HIT-95 и опубликованная история сохранены.

## Что теперь работает

`dispatchintent.Service.RunScope` получает уже установленную рабочую область и ожидаемую эпоху. Общая внедрённая граница допуска проверяет текущие полномочия и привязки до защищённого чтения и фактического сетевого старта. Отсутствие зависимости/разрешения или не вызванный callback не дают допуска. Режим по умолчанию не запускает отправку; живого IAM и фонового production listener здесь нет.

Рабочая роль в новой scoped transaction ставит ограниченную порцию существующего outbox в уже предусмотренный job_index. Это техническая постановка готового поручения, не повтор финансового решения и не новый intent. Глобальный индекс содержит только непрозрачные ID, вид работы и due/lease metadata. После выбора начинается новая транзакция с SET LOCAL hp.workspace_id. Захват атомарно меняет outbox и индекс; конкурент не получает действительную чужую аренду. Старый claim_token не может подтвердить результат после замены/истечения аренды. Постановка/выбор ограничены, обработка последовательная; новых неограниченных goroutine нет.

Собственные content files читаются через прежний HIT-79 Store: права, целостность, класс, размер и expiry. Codec проверяет точные схемы DeliveryIntent/ContextField/Receipt из закреплённого OAS, canonical имена и required/null, точные дубликаты, unknown поля, enum и числовые границы. Проверяются RFC8785 digest без message_digest и все сохранённые workspace/case/config/connection/epoch/lineage/episode pins. В сеть передаются исходные байты без пересборки JSON. SQL хранит сроки с микросекундной точностью: сравнение учитывает эту точность, исходный wire и исходный срок не изменяются.

Приватный транспорт использует TLS1.3, CA/SAN/expiry и service-specific сертификат. Нет insecure override, HTTP redirect или скрытого replay. Один новый HTTP/1 connection на попытку; GetBody отсутствует. Тот же runtime gate проверяется при connect и каждом raw socket write. До записи повторно проверяются аренда и сохранённые pins в короткой SQL-транзакции, которая завершается до сетевого IO. Критические шаги ограничены2сек; сеть не удерживает SQL row lock.

APPLIED/DUPLICATE с совпавшим message_id дают ACKED. HELD/CONFLICT дают HELD и не переписывают сообщение. Один HTTP202 без корректной квитанции не подтверждает приём. Ошибки, временный отказ и потерянный ACK допускают только ограниченный повтор того же внутреннего сообщения, с backoff, криптографическим jitter и Retry-After. Неизвестный внешний create не повторяется этим кодом: он вообще не выполняется. Повтор не меняет исходные ID/digest/content/deadline/facts expiry; задержка за пределами исходной пригодности удерживает работу. После сбоя старый payload не переиздаётся с новым сроком.

## Реальный пример из артефактов

В `verification/delivery-captures.jsonl` —14 фактических записей стенда:8 пар request/receipt и6 снимков результата/состояния. В сценарии lost_ack_reopen управляемый Delivery сначала durably сохраняет исходные bytes через file fsync/directory fsync, затем соединение закрывается до Receipt. Outbox возвращается в PENDING. Новый клиент/Content store и новый HTTPS peer открывают те же данные; повтор получает DUPLICATE, outbox становится ACKED. Digest и expires_at сохранены. В процессе crash_claim_recovery отдельный дочерний процесс завершается exit24 после настоящего claim commit; новый отправщик восстанавливает просроченную аренду и использует тот же message_id. Эти сценарии не являются production outage или тестом чужого Delivery.

Во всех положительных receipt-сценариях case остаётся WORKING/UNESTABLISHED. ACKED подтверждает только долговременный приём внутреннего поручения. Он не означает CONFIRMED, существование задачи ClickUp, её корректные атрибуты или ссылку на неё.

## Восемь критериев

| № | Проверка и фактическое основание | Локальный итог |
|---|---|---|
|1|Base/main/состояния PR, reuse admission/финансового решения/Renderer/Content; защищённые пути не изменены, supplemental command2|Проверено|
|2|TestDispatchConcurrentClaimStaleLeaseAndWorkspace:8 конкурентных claim, один победитель, замена аренды и запрет старого ACK; реальные worker role/RLS и app-index denial; pool scope reset|Проверено|
|3|Реальный TLS1.3/mTLS, исходные bytes/ID/digest, strict OAS, CA/caller/expiry/server-name/redirect negatives; TestDispatchNoSQLLockDuringActualNetwork и подмена claim между connect и TLS write|Проверено|
|4|TestDispatchHoldsWithoutNetworkStart: закрытый/отсутствующий/не вызванный gate, stale epoch, missing/corrupt/expired content/hash и попытки; handler не вызывается. Fresh gate/pins на каждом write|Проверено|
|5|Все4 Receipt disposition на настоящем HTTPS/SQL, bad ID/JSON/empty202/unknown fields/duplicates/numbers/bounds; case не CONFIRMED|Проверено|
|6|Lost ACK + новые store/peer/transport, реальный process exit24 и lease recovery, stable hash/TTL, Retry-After/no early retry/hard expiry/conflict; maxAttempts|Проверено|
|7|Новые и старые тесты, full host race/vet/build0; real PG18.7, SQL rollback между outbox/index и при finish, lost commit ACK, cancellation/closed DB/new connection и RLS/pooling|Проверено|
|8|Этот русский отчёт и14 captures; работающий локальный sender явно отделён от controlled Delivery, отсутствующих IAM/runtime и ClickUp|Проверено|

## Команды и журналы

Итоговый запуск из `services/processing`:

```sh
PYTHONDONTWRITEBYTECODE=1 GOTOOLCHAIN=local PATH=<pinned-Go1.27.2>/bin:$PATH python3 scripts/postgres/verify_dispatch.py --output <new-output-directory> --go <pinned-Go1.27.2>/bin/go
```

Это только читаемый шаблон повторения. Действительные абсолютные commands/cwd/env/UTC timestamps/exits/source SHA находятся в `verification/commands.json`. Все95 записей сохранены; `review-commands.json` представляет те же записи без потерь, с вынесенным повторяющимся контекстом. Обратное восстановление сверено. Raw stdin/stdout/stderr с SHA256 — `raw-run.tar.gz`; full plain `go-race.stdout`, `go-vet.stdout`, `go-build.stdout`, `postgres-storage-tests.stdout` доступны отдельно. Source/test binary/migration/image hashes — summary/test-binary JSON и source-audit.

Host: Go1.27.2 darwin/arm64 CGO1, полный `go test -race -count=1 -v ./...`, `go vet ./...`, `go build ./...` — exit0. DB-тесты на хосте явно skipped. Отдельный Go1.27.2 linux/arm64 CGO0 binary выполняется на настоящем PostgreSQL18.7 Bookworm по согласованному digest, network-none owned container, fsync/full_page_writes/synchronous_commit on. Это не Linux race. Контейнер удалён; отсутствие контейнера подтверждено отдельным cleanup probe.

Контролируемые SQL отказы exit3, initial readiness probe и cleanup-probe exit1 сохранены и ожидаемы; фраза PASS не означает, что все отрицательные команды вернули0. Прикладные race/vet/build/DB suite завершились0. Четыре supplemental command — exit0: независимая проверка8 request/receipt пар по точному OAS и248 отрицательных чувствительностей required/extra, фактический go env, неизменность защищённых путей, diff --check.

История разработки сохранена отдельными архивами. Первый запуск на3413b55 выявил неверное строгое сравнение наносекунд wire с микросекундами SQL (MESSAGE_INVALID до сети); он не объявлен PASS. После исправления5658e94 полный запуск02 PASS. Итоговый03 на4d74410 добавляет криптографический jitter и actual rollback-regression и PASS. Старые журналы не переписаны и не переименованы в результаты новой версии.

## Границы и остаток

Реальных IAM, готового сервера Delivery другого участника, Stripe/ClickUp credentials, ClickUp create/readback, TaskObservation, активации/паузы/настройки и deployment не добавляли. Нет квалификации физической потери питания, production fencing, нагрузочных/uptime окон или всего MVP. Полный применимый source_manifest и исходные fingerprints сохранены в source-audit; поправка PostgreSQL18.7 применяется без правки formal SQL/OAS.

Для production composition нужны установленная актуальная область/эпоха, текущий owner grant, общий runtime Boundary и реальный ограниченный worker pool/transport/content volume. Эти зависимости не заменены разрешающими заглушками. Значения стенда — batch10, lease2s, request1s, maxAttempts3, backoff40–200ms — явно локальные настройки, не новые продуктовые пороги. Свежие рабочие области передаются вызывающей owner-композицией; глобальный индекс не используется для чтения чужих payload.

После регистрации реального результата/evidence требуется официальный handoff-preflight READY, затем личный перевод Тимофея в In Review, независимый Reviewer и человеческая приёмка. Agent не сливает PR, не переводит Jira и не принимает свою работу. После разрешённой людьми интеграции зависимого стека в main его ветки и CI проверяются заново.
