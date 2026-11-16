# HIT-102 — явная активация проверенного черновика

Выполнил Codex по делегации Тимофея Патрушева / Tim1hik. Контракт v2, редакция 1. **Работа не готова к передаче: интеграционная проверка обнаружила несогласованность digest между подготовкой HIT-100 и принятой SQL-конфигурацией, которую читает HIT-95. По поручению пользователя вариант B проверен по контрактам: отсутствует предактивационный канал получения окончательного хеша клиентом. Конкретный разрыв доказан; согласие пользователя не имитируется. См. [проверку варианта B](CONTRACT_COMPATIBILITY_RU.md). PR остаётся Draft, Jira «В работе».**

Код и собственные тесты: `c3b0f430046d90b0d1614692df819a3c4ec0e720`. База — `48b2829fb37bf6048db83f5586f949dbb8883b1d`, открытый [PR58/HIT-100](https://github.com/hitpipe/hitpipe/pull/58); зависимость HIT-100 в Jira «Готово», что не означает слияния PR. Наблюдаемый main — `ce771d37505e7770003d0796ae932556d06b7bab`. Предшествующие и соседние ветки сохранены. Исправление коллизии тестовых имён касается только PKI черновиков: `DraftPKI`/`NewDraftPKI`, поведение сертификатов не менялось.

## Реализованный локальный путь

`POST /api/v1/workspaces/{workspace_id}/operations/activate` присоединяется до ServeTLS к существующему приватному серверу Processing8443. Сохраняются прежний handler и TLS1.3/mTLS/private bind; отдельного listener не создаётся. `drafthttp.Attach` позволяет использовать существующий общий сервер и для черновиков. Public bind, другой порт, отсутствующая CA/cert/client-auth, TLS1.2/dynamic TLS/insecure setup и отсутствующие существенные порты запрещают сборку обработчика.

Разбор ActivationCommand/CommandMeta использует точные сгенерированные DTO и схемы принятого OAS: канонические имена, отсутствие unknown/duplicate/case variants/null, безопасные целые, UUID/digest/const, запрет лишнего JSON. Обязательны confirm_activate=true, managed-operation-v1, CSRF, X-HitPipe-Session и согласованный X-Request-ID. If-Match сверяется с CommandMeta. draft_revision относится к черновику, expected_revision — к изменяемой существующей операции; для новой операции его нет. Проверены existing operation revision7 при draft revision1 и результат operation revision8.

Сессия, OWNER grant, workspace/principal/epoch и actual-start проверяются явными портами; отсутствие порта закрывает обработчик. Каждый protected SQL/content read, commit и начало ответа проходит общий gate активации, с ограничением2сек и проверкой фактического синхронного вызова callback. Текущая повторная проверка owner bindings выполняется вне SQL locks. Она должна подтвердить те же connection/account/mode/Price/List/Assignee identities, что сохранены при readiness. Неизвестность/истечение/отзыв не превращаются в ACTIVE.

Читаются текущие durable DraftConfig, ReadinessCheck и RehearsalResult той же версии и digest; сравниваются handles, срок готовности, полномочия и исходные синтетические примеры. Для этого существующий частный expiring command node хранит examples репетиции; их точный исходный request digest сверяется с журналом. Старый узел без примеров даёт503, а не выдуманную успешную репетицию. Повторяется существующий чистый evaluator, финансовое ядро не менялось. Ожидаемые WOULD_WAIT/WOULD_EXCLUDE/WOULD_HOLD_UNKNOWN допустимы; RehearsalResult не получил passed/expires_at.

В одной scoped транзакции связываются accepted_configs, operations, selected_config_id, owner/revision, draft.operation_id, command metadata и durable root полного ответа. Первая граница задаётся серверным DB time один раз, существующая сохраняется. Full response → command node → root записываются file fsync/rename/directory fsync перед SQL commit. SINGLE защищён короткой workspace-сериализацией и существующим UNIQUE index; текущая операция/черновик перепроверяются под блокировкой. Сеть под row lock не вызывается. Точный повтор возвращает исторические байты после reconnect/reopen/crash, не создаёт новую операцию, не возобновляет PAUSED и не сдвигает границу/TTL. Другой payload конфликтует. Потеря commit ACK возвращает503; следующий точный повтор восстанавливает исходный ответ. Name/aliases/полный ответ остаются в собственных файлах, user_commands содержит метаданные.

## Обнаруженное препятствие, а не пройденная интеграция

В экспериментальной отдельной ветке совмещены текущая HIT-102 и сохранённые HIT-95/98/99/104. Ни один PR не слит. Сначала обнаружена и устранена коллизия имён тестового PKI HIT-98/HIT-100. Затем реальный один HTTPS listener дал activation201 → Intake202 → BFF listCases503. Прямое чтение scoped SQL подтвердило:

- digest проверенного черновика и записанный accepted_configs.digest: `sha256:f298e500f9e92f632bff94af07e8c752aeb2417165202fb2f867b6e28772dbf0`;
- SHA256 JCS фактически записанного canonical_config: `sha256:b727c133788564854b3c7a169061c7d24437bff203f596564bcee7d360447187`.

HIT-100 считает digest по значимым полям DraftConfig с псевдонимами; принятая SQL конфигурация содержит разрешённые IDs и pack_id. HIT-95 проверяет digest именно фактических JCS bytes canonical_config и обоснованно отказывает. Переключение только SQL-хеша также меняет соответствие входного ActivationCommand.accepted_config_digest и выходного Operation.accepted_config_digest. Самостоятельно вводить эту семантику или ослаблять проверку HIT-95 нельзя.

[Точные факты](DIGEST_DIAGNOSTIC.txt), [вопрос и варианты решения](DIGEST_DECISION_REQUIRED_RU.md), [исходные полные журналы](history/integration-diagnostic-03.tar.gz). Существующий PR содержит текущую незавершённую реализацию, а не разрешение на развёртывание. До согласования, исправления и полного повторного прогона все8 DoD выполненными не считаются. Handoff-preflight ещё не запрашивался; READY/PASS/приёмки нет.

## Фактические проверки

Финальный для текущего кода запуск verification-06: **96 команд**, необходимые exit0; ожидаемые SQL отказы exit3 и probe отсутствующего удалённого контейнера exit1 отражены отдельно. Проверенный чистый SHA `c3b0f430046d90b0d1614692df819a3c4ec0e720`; [summary](verification/summary.json), [полные команды/cwd/env/SHA/время/exits/digests](verification/commands.json), [тот же протокол с общей частью контекста](verification/review-commands.json). **Это успешный локальный прогон компонентов; он не отменяет интеграционный отказ.**

- Go **1.27.2 darwin/arm64 CGO1**: полный `go test -race -count=1 -v ./...`, `go vet ./...`, `go build ./...` — exit0. Полные [race stdout](verification/go-race.stdout) и [vet/build](verification/go-vet.stdout) доступны. DB-тесты на хосте явно SKIP.
- Отдельный **linux/arm64 CGO0** test binary: настоящая временная PostgreSQL **18.7**, hp_processing_app/RLS, собственные файлы и HTTPS/mTLS — exit0, [полный DB stdout](verification/postgres-storage-tests.stdout). Это не Linux race. Образ `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`.
- **81 настоящая HTTP запись** активации: 13×201, 13×400, 2×401, 25×403, 8×409, 5×412, 1×413, 14×503. Независимая проверка всех ответов и положительных запросов по pinned OAS — PASS; [captures](verification/activation-api-captures.jsonl), [проверяющее средство](verify_captures.py), [результат](supplemental/0.stdout).
- Пять дополнительных команд exit0: OAS/captures, реальная Go среда, Python **3.14.2**, неизменность защищённых путей, diff check. Это фактический Python инструмента, не3.14.8 production runtime. [Протокол](supplemental/commands.json).
- [CI предыдущей проверенной редакции ec0eeb8](verification/ci-code-evidence.json):5/5 SUCCESS. PR processing run [43898973014](https://github.com/hitpipe/hitpipe/actions/runs/43898973014), push processing43898954519. PR workflow head — ec0eeb8, реально checkout `refs/pull/63/merge` — `482d0e5454ee67fb9e1e5cc17a4a3f7fbeab2f47`; trees совпадают `cefa1aca407ee09a821c2c23c4d69395b4ec3891`. Это проверочная merge-кандидатная версия, не main. [Полный исходный CI log gzip](verification/ci-code-processing-pr.log.gz), digest распакованных bytes в CI evidence. Общий CI не запускает отдельную owned DB квалификацию.

Пройденные сценарии: все ожидаемые synthetic outcomes; запреты CSRF/session/roles/workspace/caller/epoch/gates; строгие top/nested JSON case conflicts в обоих порядках; точный лимит256KiB допустим, +1 rejected413; версии существующей операции; чужие/изменённые/просроченные handles/bindings; несоответствующие результаты/исходные examples; шесть конкурентных команд; SINGLE; rollback на write/file-fsync/rename/directory-fsync и SQL aborted commit; реальный child process exit24 после commit/до emit; commit ACK loss; exact replay после pause; expiry/purge/corruption/reopen без восстановления полного ответа из одних metadata. Исходные migration/RLS/admission/preparation/draft regressions сохранены.

## Состояние восьми критериев

| № | Фактический статус |
|---|---|
|1|Локально проверено: SHA/PR/reuse/карта полей; рабочие ветки сохранены. Слияния main не заявлены.|
|2|Локально проверено: настоящий HTTPS/mTLS, CSRF/session/current authority/gate; контролируемые owner ports.|
|3|Локально проверено: точные схемы/JSON/UUID/digest/две revisions/If-Match/request_id.|
|4|Частично: проверенные draft/readiness/rehearsal/owner bindings связаны; соответствие digest принятой конфигурации требует решения.|
|5|Частично: реальная атомарность/RLS/SINGLE/first activation проходят; accepted-config digest не совместим с последующим чтением HIT-95.|
|6|Локально проверено: полный replay/reconnect/restart/конфликт/rollback/ACK loss/граница/TTL.|
|7|Не завершено: формат Operation201 проходит OAS, но семантика его digest и SQL digest требует согласования и исправления. Внешних/финансовых действий нет.|
|8|Частично: race/vet/build/PG18.7/OAS/CI проходят; экспериментальный сквозной локальный прогон выявил503 и пока не проходит.|

## Границы и следующий шаг

Порты IAM/сессии/actual-start/Intake/Delivery bindings управляемые, честно обозначенные стендовые зависимости. Нет live IAM/BFF/Stripe/ClickUp, credentials, deployment/SLA/load/power-loss qualification. TTL задаёт существующая явная policy (в тестах1ч содержимое,1мин check); это не новые продуктовые пороги. Full response доступен лишь до expiry своего содержимого; затем503, SQL сохраняет только принятую безопасную конфигурацию/identity. PAUSE/RESUME/ARCHIVE здесь не реализуются. Schema/API/financial domain/чужие сервисы/CI/locks не менялись.

После решения Lead: зафиксировать точное соответствие digest; минимально исправить writer/связь approval/receipt; повторить относящиеся проверки и сквозной сценарий; обновить реальные журналы и CI; зарегистрировать законченный результат/evidence, выполнить свежий официальный handoff-preflight. Только READY позволяет предложить личный перенос In Review. Агент не принимает работу и не сливает PR.
