# HIT-100 — сохраняемый черновик, готовность и синтетический прогон

Выполнил Codex по делегации Тимофея Патрушева / Tim1hik. Контракт редакции 1, протокол v2 `685e797b`. Реализованы ровно saveDraft/getDraft/checkDraftReadiness/rehearseDraft: частный Processing HTTPS/mTLS обработчик, настоящее сохранение DraftConfig в PostgreSQL + собственных файлах, readiness по текущим проверяемым owner bindings, чистый прогон синтетических примеров без внешних действий.

Проверенный код: `369a553aaddbc581e82d9bcf7203bbf2db9efd20`. База: `3bae6417421465868c432fe18b379d46be6144aa` (OPEN PR52), наблюдаемый main: `ce771d37505e7770003d0796ae932556d06b7bab`. PR45/48/50/52/54/57 остаются OPEN: эта работа и предшествующий стек не объявлены принятыми main. HIT-76 — единственная зависимость карточки, Jira Done; база повторно использует его миграцию/RLS и HIT-79 content store. HIT-95 и HIT-98 сохранены в собственных ветках. Инвентаризация с полными SHA и sources — [source-audit.json](source-audit.json).

## Реальная локальная проверка

Финальный полный запуск verification-06 завершён `2026-11-15T19:52:44.722805+00:00`: **95 команд**, обязательные exit=0; ожидаемые SQL-отказы exit=3/SQLSTATE и отрицательный cleanup probe exit=1 отражены отдельно. Пять дополнительных команд exit=0. [Все точные команды/cwd/env/SHA/время/exits/digests](verification/commands.json), [тот же журнал без повторения контекста](verification/review-commands.json), [summary](verification/summary.json), [дополнительные проверки](supplemental/commands.json).

- Go **1.27.2 darwin/arm64 CGO1**: `go test -race -count=1 -v ./...`, `go vet ./...`, `go build ./...` — exit0. [Полный go-race.stdout](verification/go-race.stdout) доступен ревьюеру. Host DB-тесты явно SKIP: реальные DB-проверки запущены следующим отдельным процессом.
- Настоящая PostgreSQL **18.7**, pinned image `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`. Отдельный **linux/arm64 CGO0** test binary и реальные scoped hp_processing_app транзакции/own content/HTTPS TLS1.3 mTLS — exit0, [полный stdout](verification/postgres-storage-tests.stdout). Это не Linux race.
- **61 фактическая HTTP запись**, четыре операции, положительные и честные отрицательные проверки, все четыре rehearsal исхода. Независимая проверка по pinned OAS и **3911** отрицательных проверок чувствительности схем — PASS ([результат](supplemental/0.stdout), [проверяющее средство](verify_captures.py), [запросы/ответы](verification/api-captures.jsonl)). Проверены обе границы 1/10, 0/11 отвергнуты, canonical zero/false сохранены.
- Raw stdout/stderr/stdin всех команд находятся в [raw-run.tar.gz](verification/raw-run.tar.gz); имена/длины/SHA256 указаны в commands.json. Выделенные полные stdout — точные копии, не пересказ. [evidence-digests.json](evidence-digests.json) позволяет проверить файлы пакета.

## Полноценный пример и честный отказ

`TestDraftHTTPFullRoundtripReadinessRehearsalAndRestart`: HTTPS save полной синтетической конфигурации с русским name/service_label/aliases → 201 DraftConfig revision1; GET возвращает ровно её. Текущий controlled owner check подтверждает связи → 200 passed=true с draft_revision/config_digest/check_id/checked_at/expires_at. Rehearsal даёт для оплаченного положительного примера WOULD_CONFIRM, неоплаченного WOULD_WAIT, известного оплаченного нуля WOULD_EXCLUDE, duplicate WOULD_HOLD_UNKNOWN, отсутствующего контекста WOULD_WAIT; live_requests_made=0. После закрытия файлов и нового HTTPS server полный ответ команды байт-в-байт тот же, без повторного owner check. Полные реальные JSON — api-captures.jsonl.

`TestDraftHonestIncompleteAndUnavailableReadiness`: неполный черновик/недоступный owner порт/неподтверждённая или устаревшая привязка/отсутствующий порт → 200 passed=false + CONFIG_INCOMPLETE/OWNER_CHECK_UNAVAILABLE/OWNER_BINDINGS_UNVERIFIED. Это результат проверки, не 403 допуска и не фиктивная готовность. Ошибки аутентификации/разрешения/невалидный JSON возвращают соответствующие Error и ничего не сохраняют.

## DoD: восемь локально проверенных пунктов

| № | Локальный результат и конкретная проверка |
|---|---|
| 1 | SHA/OPEN стек зафиксирован; migration unchanged; [полная карта полей](FIELD_MAP.md), источники pinned. |
| 2 | Четыре реальные private HTTPS операции; mTLS BFF caller, session, CSRF, role, workspace, current epoch и actual-start. TestDraftHTTPAuthoritySessionCSRFAndActualStart, TestDraftTLSCallerCertificateAndMissingDependencies, TestDraftReadRolesAndForeignDraftScope. Неправильный/просроченный/чужой/отсутствующий сертификат, неверное имя сервера отклонены. |
| 3 | Реальные files/PG restart; повтор полной save/readiness/rehearsal; одинаковый ID с другим телом 409; новая stale версия 412; шесть concurrent updates дают один результат. File faults на четырёх реальных точках откатывают SQL; SQL commit ACK loss и настоящий child-process exit24 после commit/до response восстанавливают прежний полный ответ. Нет метаданных вместо response, corrupted/missing/foreign refs 503; pooled scope/cancel/reconnect проверены. |
| 4 | Strict canonical JSON на всех уровнях, case variants в обоих порядках/точные дубликаты/extra/null/trailing JSON/NaN/нецелое/unsafe integer отвергнуты. Неизвестное не становится нулём. SQL содержит только безопасный subset; name/aliases/label остаются own content. Formal SQL и OAS неизменны. |
| 5 | Версия/digest/expiry связаны с saved config; текущие owner scope/epoch/IDs/time проверены. Изменение версии/конфигурации/epoch/binding не обновляет прежний check; exact replay после expiry сохраняет прежнее время/UUID, новая команда делает новую проверку. Private node удерживает provenance исходного положительного check. |
| 6 | Чистый synthetic adapter к неизменному evaluator, без SourceFacts proof/real order. 1–10 примеров, known0/false, все4 исхода; duplicate консервативно HOLD_UNKNOWN, так как прошлого фактического исхода нет. Provider порта нет, live_requests_made=0. |
| 7 | После save/check/rehearsal таблицы operations/accepted_configs/case_identity/outbox в изолированном workspace остаются пустыми; ни ACTIVE, ни DeliveryIntent. Future activation должна свежо проверить revision/digest/context/expiry; сама здесь отсутствует. |
| 8 | Полный race/vet/build + real PG18.7 suite PASS, независимый OAS/captures PASS; русский пример/отказ и границы квалификации сохранены. |

Идентификаторы frozen DoD: `9ef766ec-944f-4768-b35e-7fbb0c61d948`, `903b21ea-82c7-4d9a-a4b9-653c1b961997`, `4e1e485e-bacc-4728-aa9e-1ea2ac6b37aa`, `525970b4-3184-4005-a8fa-29a2de105b36`, `91f409e2-a8e4-4def-be1b-82e059b4578b`, `f5b2e183-d64a-41b5-91fb-00bdfe5125f0`, `444d03c1-e581-438a-a00c-ab8b0ac2b01c`, `7a2bba80-9cd2-4cb4-b60c-24a721f592b2`.

## Границы квалификации и передача

Authority/session/shared actual-start и owner connection/catalog bindings — явные внедряемые порты, испытанные управляемыми зависимостями, не работающие IAM/BFF/Stripe/ClickUp. HTTP NewServer требует настоящую TLS-конфигурацию и привязку private8443 (loopback0 только тест); никаких permissive production grants или автоматического включения нет. Политика TTL, объём HTTP и bounded replay задаются вызывающей композицией; тестовые 1h/500ms/256 не объявляются продуктовыми порогами. Production wiring/credentials и deployment qualification не входят в контракт.

Сроки и hash/ref проверяются при чтении. После expiry/purge/reopen отсутствующее содержимое даёт 503; metadata не позволяет повторить полный ответ. Предел обхода ledger тоже честно даёт 503. Проверялся crash процесса/IO/SQL ACK, не физическое отключение питания, SLA или нагрузочное испытание. Схема/домен/финансовые правила/чужие сервисы/общий CI/source locks не менялись. Из старого contentfiles/store.go добавлен только класс DRAFT_DATA; новые draft-specific reads вынесены отдельно.

[История запусков](prior-runs/index.json) сохранена: ранние инфраструктурные неуспехи, проход prototype, один отказ нового теста из-за неверного имени injected directory-fsync и последующие успешные версии. Они не переписаны под финальный SHA. Финальная документация имеет следующий commit; tested_code_sha не догоняет собственный SHA документа.

Локальное выполнение всех 8 DoD не является официальным READY, независимым PASS или принятием Lead. Официальная регистрация, evidence, текущий CI и handoff фиксируются отдельными Jira/PR записями после публикации этой версии. Jira переводит человек, PR не сливает агент.
