# HIT-104 — защищённый HTTP-приём нормализованного сигнала

Codex по делегации Тимофея Патрушева / Tim1hik, 16 ноября 2026 года.
[Задача](https://hitpipe.atlassian.net/browse/HIT-104), [PR62](https://github.com/hitpipe/hitpipe/pull/62).

## Результат и фактическая граница

Готов адаптер `receiveNormalizedSignal`: POST `/internal/v1/intake-signals` на существующем закрытом Processing HTTPS8443/TLS1.3 с обязательным mTLS и отдельным Intake SAN. `intakehttp.Attach` вызывается до ServeTLS и сохраняет прежний Handler как продолжение маршрутизации. Второй listener, новая схема и новое финансовое ядро не создаются. Реальные тесты сохраняют и BFF history read, и Delivery observation handler на том же сервере; сертификат одного сервиса не допускает его в маршрут другого.

Внешний источник установленной привязки и текущих полномочий — обязательный ограниченный порт `receivesignals.ContextSource`. Его отсутствие, typed nil, неизвестность, истечение grant, pending revoke и недействительная эпоха закрывают приём. JSON содержит только подсказки identity для поиска, не полномочия. В стенде явно управляемые установка/текущий grant дополнены настоящим read-only SQL-чтением собственной операции под тем же shared actual-start gate; revision берётся из PostgreSQL, а не из browser headers. Это не подключённый production IAM, Intake sender или внешние провайдеры.

Один исходный decode/JCS через существующий `normalizedsignal.Decoder` передаёт сигнал в прежний admission-путь (`Admit` и `AdmitWithContext` используют одну реализацию), затем в существующий `postgres.Store.Accept`. Успех возвращается после commit inbox/case/safety и durable HISTORY, затем после отдельного допуска начала ответа. Никакого source read, финансовой оценки, outbox dispatch или внешней записи внутри квитанции нет. **HTTP202 означает только надёжный локальный приём.** Это не подтверждение оплаты, ClickUp-задачи или DeliveryACK; новое дело остаётся RECEIVED/UNESTABLISHED.

## Версии и сохранённая работа

Проверенный чистый код: `5dea7c4592f52218d71afc88b1529b5f458be415`; дерево перед run05 чистое. База: PR59/HIT-99 `60f6c4863d91bc5f32812d55afbe01de94557f9e`, сохраняющая HIT-78/79/95. Main при проверке — `ce771d37505e7770003d0796ae932556d06b7bab`. PR50/52/54/57/58/59 открыты; готовность Jira-предшественников не названа слиянием в main. Отдельные HIT-98 и HIT-100 не переписаны и не интегрированы экспериментально. Точные reuse SHA, полный применимый manifest источников и хеши изменённых файлов — [source-audit.json](source-audit.json).

Локальные проверки завершены. CI финального head, регистрация результата/evidence и свежий Project OS READY фиксируются внешними записями в PR/Jira после публикации пакета; документ не коммитится повторно ради собственного SHA. Независимый Reviewer и принятие человеком — отдельные шаги. Сам этот отчёт не выдаёт READY/PASS/Done.

## Точная проверка входа и сохраняемое значение

Обязательны POST, точный путь без query/варианта encoding, один корректный X-Request-ID и application/json; X-HitPipe-* authority headers отклоняются. Лимит входящего сигнала — **256KiB по HP-NFR-004**, проверены ровно262144 байта с корректным JSON/whitespace и отказ на262145. Это не произвольно выбранный новый лимит. Недопустимое тело не оставляет inbox/case/safety и не получает202.

Прежний строгий декодер проверяет все обязательные поля, null, неизвестные/повторные имена, варианты регистра в обоих порядках, вложенный объект вместо scalar, лишний JSON, enum/version, UUID, длины, числа, UTF-8 и непарные Unicode surrogate. JCS проверяет исходные байты и digest без message_digest; сырое сообщение не пересобирается из типизированных значений до проверки digest. Дополнена проверка RFC3339-синтаксиса времени, чтобы расширения Go parser (comma fraction, zone+24:00) не обходили wire-формат. Workspace/connection UUID приводятся к тому же регистру identity, что SQL/content scope, только после проверки исходного digest; message_id/Receipt сохраняют исходное wire-значение. Канонический livemode:false допускается.

[Решение Lead12171](https://hitpipe.atlassian.net/browse/HIT-78?focusedCommentId=12171) прочитано непосредственно в Jira; фактические author/updateAuthor проверены и сохранены в [копии решения](session-created-decision.json). `source_created_at` — `Event.data.object.created`, время **Checkout Session**, не Event.created и не время приёма. Реальные HTTP-тесты: Session до первой активации409; ровно на границе202; после202; старая Session с поздним async_payment_succeeded409. После pause/resume/revision исходная граница остаётся прежней.

`receipt_id` остаётся proof_ref исходного SIGNAL_ADMITTED; минимальная принадлежность Intake и исходные Session.created/first_received_at/expires_at записаны в существующий expiring HISTORY envelope. Это не копия NormalizedSignal: нет event_id/event_type/raw payload/account/session text в новом detail, в SQL только прежние identity/safety и content refs. Времена fixture заданы явно; они не заменяются текущими часами. Собственная HISTORY retention ограничена прежними90сут и не продлевается повтором. Исходный expires_at Intake — provenance владельца, не новое собственное правило финансового допуска или обещание реализации очереди Intake.

## Проверки всех восьми критериев

Все строки ниже — **LOCAL PASS** применимого среза на указанном коде, не независимая приёмка.

| № | Критерий и реальное доказательство |
|---|---|
|1|Reuse/SHA/границы: source-audit.json и ROUTE_MAP.md; supplemental record1 подтверждает неизменность финансового domain/preparecase, старых read/observation handlers, миграций, locks, CI, чужих сервисов. Точный декодер и Store переиспользованы.|
|2|Настоящий mTLS: `TestIntakeHTTPBindingAuthorityLifecycleAndTLS` проверяет wrong service/CA/expiry/no cert/server name, workspace/connection/account/mode, verified context, grant expiry/revoke/epoch/shared gate и отдельный context SQL gate. Неизвестная принадлежность не превращается в глобальный доступ.|
|3|`TestIntakeHTTPStrictWireAndHTTPBoundary`, `TestIntakeHTTPUUIDIdentityAndExistingDeliveryRoute`: обязательность каждого поля/null, exact/case/escaped duplicates, методы/путь/query/headers/body limit, malformed scalar/число/время/Unicode/digest. Квитанции и ошибки независимо проверены по точной принятой OAS.|
|4|`TestIntakeHTTPErrorsCommitAckAndProcessCrash`: настоящий PostgreSQL aborted Commit (SELECT1/0), закрытое соединение, потерянный committed ACK, закрытый собственный content resource, новый процесс exit24 после настоящего commit и до HTTP-квитанции. Негативные ответы403/503 не названы APPLIED; свежая композиция/соединение восстанавливает исходный Receipt.|
|5|`TestIntakeHTTPReplayPauseConcurrencyIdentityAndReadRoute`: одновременные6 доставок одного/разных message_id, digest conflict409, одна Session в точном workspace/account/mode — одно дело; другая Session/разрешённый иной account/mode не склеиваются. Stale revision412, пауза блокирует новый приём409, принятый message_id+digest после pause/revision/restart даёт прежнюю квитанцию без нового допуска.|
|6|Четыре activation boundary HTTP-сценария выше; origin/receipt проверяются по реальной SQL HISTORY и owned file. Replay сохраняет полный Receipt, content ref/expiry; resume не переносит first_activation_at.|
|7|SQL показывает RECEIVED/UNESTABLISHED/revision1 без outbox/CONFIRMED; error body не раскрывает источник ошибки/сырой payload. `TestIntakeHTTPDurableFileFailuresAndExpiryRestart`: write/file-fsync/rename/directory-fsync ошибки дают rollback, нет202; после detail expiry/purge/нового content handle/restart минимальная identity и исходный Receipt сохраняются. Общие RLS/privacy/history регрессии предшественников реально выполнены.|
|8|Финальный run05:101 фактическая команда PASS, включая race/vet/build и весь PostgreSQL storage suite. Дополнительно3 команды exit0. Полные команды/окружение/SHA/коды/времена/hashes сохранены ниже.|

## Настоящие журналы и воспроизведение

Среда: **Go1.27.2 darwin/arm64 CGO1** для полного `go test -race -count=1 -v ./...`, vet и build. Отдельный **Go1.27.2 linux/arm64 CGO0** test binary исполняется в настоящей изолированной PostgreSQL18.7. Linux race не выполнялся и не заявляется. Версии Docker/архитектура/настройки fsync/full_page_writes/synchronous_commit зафиксированы в командах. Python инструмента — фактическая3.14.2, не3.14.8; Python не является runtime этого Go-модуля.

Образ: `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`, принятый HP-CCR-POSTGRES-20261114. Новый временный контейнер без сети и без доступа к прежним DSN/БД, удалён после тестов; cleanup-probe exit1 подтверждает отсутствие контейнера. Ожидаемые SQL-negative exit3 отмечены отдельно; они не скрыты как обычные успешные команды.

[Компактный протокол](verification/review-commands.json) — lossless-представление [полных команд](verification/commands.json): context из таблицы contexts добавляется к каждой команде; обратное восстановление проверено. Внутри — argv/cwd/env, code SHA, clean tree, время, exit code и SHA256 каждого сырого потока. Полные [race stdout](verification/go-race.stdout), [PostgreSQL stdout](verification/postgres-storage-tests.stdout), [vet](verification/go-vet.stdout), [build](verification/go-build.stdout) доступны напрямую. Остальные исходные `.stdin/.stdout/.stderr` с точными именами из commands.json находятся без изменений в [raw-streams.tar.gz](verification/raw-streams.tar.gz).

[133 реальных HTTP-ответа](verification/intake-captures.jsonl), [независимая OAS-проверка](verification/exact-intake-oas-validation.stdout): статусы202/400/403/404/409/412/413/503; **2867 отрицательных проверок чувствительности схемы**. Это не2867 отдельных сетевых тестов. Успешные запросы и ответы проверены по принятым NormalizedSignal/Receipt; ошибки — Error. Негативный HTTP route/header может иметь корректное JSON-тело и всё равно отказ. TLS handshake отказы не выданы за JSON-ответы. Все данные синтетические, настоящих Stripe webhook/клиентских данных/секретов здесь нет.

[Supplemental](supplemental/commands.json): Go env, проверка неизменности защищённых путей, git diff --check; все3 exit0 на том же clean code. [Summary](verification/summary.json), [digests](evidence-digests.json). Прежние прогоны имеют свои SHA и сохранены в [prior-runs](verification/prior-runs.tar.gz) с [индексом](verification/run-history.json): run01 остановился до БД из-за выключенного Docker, run02–04 предварительные версии, run05 окончательный. Журналы HIT-78/79/95/98/99 не переписывались.

Из корня клона с Docker и указанным Go (OAS — точные байты принятого источника):

```sh
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_intake_http.py --output /absolute/new-empty-run --go /absolute/go1.27.2/bin/go --oas /absolute/accepted-mvp-openapi.json
```

Точные фактически исполненные абсолютные команды, а не этот параметризованный пример, находятся в протоколе. В нём cwd корня и `go -C services/processing` различаются явно. Без `HIT78_ISOLATED_PG=1`/выделенного runner DB-тесты пропускаются; один обычный host test не доказывает PostgreSQL/mTLS. В настоящем Linux PostgreSQL-запуске новые TestIntakeHTTP не пропущены.

## Остаток и передача

Нужны CI точного опубликованного head, официальная регистрация результата/evidence, свежий handoff-preflight READY, затем личный перевод Тимофеем в «На проверке», независимый Reviewer и решение Lead. Фактические квитанции/CI ссылки публикуются в PR/Jira отдельно от неизменяемого пакета. Агент Jira не переводит, PR не сливает и собственную работу не принимает.

Граница результата: локальный защищённый сетевой приём с управляемым trusted context. Production transport/credentials/актуальный IAM/Intake sender и полный вертикальный runtime, удержанная очередь Intake, scheduler, внешние Stripe/ClickUp действия, power loss/потеря всего storage, SLA/load и production qualification этой задачей не доказаны. После реальной интеграции открытых веток требуются новые проверки обновлённых веток и CI.
