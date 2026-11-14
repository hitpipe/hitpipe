# HIT-79 — финансовое решение и надёжно сохранённый intent

Codex по поручению Тимофея Патрушева / Tim1hik. План подтверждён участником
«Перенёс, план подтверждаю», переход в In Progress проверен. Это результат
локальной реализации и её проверок, не независимый вердикт и не приёмка MVP.

## Результат

Добавлен прикладной путь `preparecase.Service.Prepare`: сохранённое дело → свежие
SourceFacts через существующий reader/evaluator → решение и история → один
неизменяемый DeliveryIntent и ожидающий outbox при выполненных guards.

Финансовый VALID сам по себе ничего не разрешает. Обязательны текущие полномочия
и bindings/epoch в общем runtime Boundary, допустимое состояние операции/дела,
прежние config/owner/source identity, актуальные revisions и свежесть facts/context.
Unknown, отмена, отзыв, несовпадение, истечение, известный/возможный эффект,
недоступный content и CAS-конфликт не создают новое разрешение. Отсутствующие
зависимости не заменяются разрешающими реализациями.

Unpaid completion сохраняет WAITING_DATA с причиной, а не вечное EXCLUDED.
Последующая оплата оценивает то же дело, прежнюю конфигурацию и первую границу
активации. Доказанная другая финансовая неприменимость остаётся результатом
существующего evaluator; его предикаты не менялись.

Операция подготовки не вызывает Delivery или ClickUp и не устанавливает CONFIRMED.
`PENDING_OUTBOX` означает только локально сохранённое сообщение, не внешнее действие.

## База и переиспользование

- HIT-72 / PR44 MERGED: прежний Decode SourceFacts и финансовая проекция сохранены.
- HIT-76 / PR45 OPEN, `15e684e1c3f78bb7851bc1d4335c9a9f883f5dc1`: точная SQL-схема,
  роли, мигратор и реальные RLS/constraint регрессии переиспользованы.
- HIT-77 / PR48 OPEN, `b31a3adae4fac625c97fd903e13f9aad8d6ca771`: только исходные
  Processing-коммиты `d9bb781` и `204fabc` перенесены с `cherry-pick -x`.
  Чужие IAM/CI-изменения из ancestry этой ветки не переносились.
- HIT-78 / PR50 OPEN, `c218eb3d87c1df51d999d955d263b9d410f77bd7`: база ветки,
  durable admission и согласованный PostgreSQL18.7.

Интеграционная база этих исходников: `4c21dc3`. Это локальная зависимая композиция,
не слияние указанных PR в main. Наблюдавшийся main — `ce771d37505e7770003d0796ae932556d06b7bab`.
Собственные изменения находятся в application/preparecase, adapters/postgres,
owner adapters/contentfiles, относящихся тестах/инструментах/доказательствах Processing.
`source-audit.json` подтверждает точные байты reused reader/Decode/evaluator,
неизменность SQL, финансового ядра, соседних сервисов и общего CI.

## Контракты и согласованная версия PostgreSQL

Основные источники: замороженный manifest SystemDesign `0ff88d118e39a5dfaa34540c0d621649c34655a7`,
MVP OAS/SQL `fedbe40bb597e25f66aeb1172af3ff7459c81b40`, архитектура
`b2461564e4355ea5a72a55a30a635655397c463b`.

[Поправка HIT-79 №12258](https://hitpipe.atlassian.net/browse/HIT-79?focusedCommentId=12258)
и [пояснение №12271](https://hitpipe.atlassian.net/browse/HIT-79?focusedCommentId=12271)
применяют PostgreSQL18.7 без изменения SQL/API/DoD. Снимки авторства/текста в
`security-adoption.json`. Точный Bookworm-образ:

`docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`.

Полные технические правила content и handoff-block прочитаны; использованные
версии/выдержки BP006/T006 и BP004/T013 сохранены в `technical-source-excerpts.json`
как подробные пояснения, не новая authority поверх замороженного MVP.

Типы ContextField/DeliveryIntent воспроизводятся `tools/intent/generate.py` из
точных OAS bytes (SHA256 проверяется). Генерация и отсутствие diff записаны в
`supplemental-commands.json`. Не добавлены поля wire или таблицы/колонки SQL.

## Композиция, полномочия и факты

Owner обязан явно передать Repository, Content, Readers, Renderer, Boundary и Clock.
Boundary — текущие authority/bindings/epoch и actual-start, один общий runtime объект
для защищённых SQL операций и источника. Здесь нет production IAM resolver,
сертификатов, секретов, permissive default или пользовательского HTTP-входа.

`intent.Readers` создаёт существующий HIT-77 `sourcefacts.NewReader` из pinned case/config,
явно заданного HTTPS origin, mTLS transport, общего Boundary, deadline и частного
buffer bound. Decoder и transport HIT-72/77 не переписаны. Соединение с Intake
выполняется вне SQL-транзакции и row lock; это отдельно проверено настоящим HTTPS
fixture, который получает case row `FOR UPDATE NOWAIT` во время чтения источника.
Внедрённый owner Boundary в тестах явно синтетический и не доказывает runtime IAM.

Факты и provenance принадлежат Intake. ID/digest фактов и точные времена из SourceProof
сохраняются как сообщённая источником provenance, не заменяются локальным hash
потерявшей wire-представление domain-проекции и не называются независимой проверкой
подлинности Stripe. Общий evaluator повторно проверяется после получения SQL lock
на текущем времени БД; финансовый VALID отделён от остальных guards.

## Атомарность, CAS и содержание

Repository повторно проверяет case/operation revisions, pinned config/digest/owner,
ACTIVE lifecycle, absence of possible/existing effect/lineage и обязательные context refs.
Один CAS под row lock выделяет lineage и неизменяемое сообщение; `message_id=lineage_id`
— стабильное внутреннее сопоставление первой подготовки, не provider idempotency key.
Первый episode сохраняется один раз с пределом300s, существующий не продлевается.

В одной SQL-транзакции сохраняются state/revision/lineage, minimal safety history,
history content ref и outbox metadata. Outbox хранит content UUID/hash/bytes/expiry
и wire digest, не payload JSON. `payload_sha256` — hash полных сохранённых bytes;
`digest` — SHA256 RFC8785 JCS сообщения без message_digest. Это разные величины.

Renderer реализует SERVICE_ORDER / handoff-block-v1: NFC/LF, фиксированный opaque
public ref из case UUID, точный title и delimiters, ASCII-порядок обязательных ключей,
полные multiline значения,32KiB JCS context и36KiB rendered block. Непрошедший
нормализацию сохранённый ввод удерживается для видимого исправления/preview,
не изменяется молча. Общий payload также обязан помещаться в уже принятый SQL
предел65536 bytes; превышение удерживается без обрезания и ослабления constraints.

Файл содержит durable metadata header (class, opaque workspace/case/revision origin,
expiry, digest, size) и body. Directory0700/file0600, случайное opaque UUID имя,
запрет traversal/symlink, file fsync → atomic rename → directory fsync до SQL commit.
Срок метаданных округляется вниз до микросекунд PostgreSQL, не продлевая hard expiry.
Intent expires не позже facts proof, episode и самого раннего required context.
Подробная история живёт отдельно≤90d; SQL не получает customer context/proof body.

На ошибке/неизвестном commit положительный Result не возвращается. Файлы не удаляются
как «сироты» по предположению: повтор сначала читает durable SQL refs и проверяет
тот же content. Payload/ID/digest/lineage не пересоздаются. До hard expiry возможный
committed orphan сохраняется; `PurgeExpired` работает по durable header без БД,
включая pending-файлы аварийного завершения. Повреждённый header сообщает incident,
а не становится разрешением. Автоматический GC unexpired unknown refs не реализован.
Владелец обязан вызывать expiry purge; эксплуатационный scheduler — отдельная интеграция.

## Фактические проверки

Проверенный исходник: `4af9683cc2173476549fc3e1bd0515612719dd3f`, чистое дерево.
Все mandatory проверки последнего запуска завершились exit0:

- Go1.27.2 darwin/arm64: `go test -race -count=1 -v ./...`, `go vet ./...`.
- Реальный PostgreSQL18.7 Linux/arm64 в изолированном network-none контейнере,
  fsync/full_page_writes/synchronous_commit ON, app-role без SUPERUSER/BYPASSRLS.
- Go1.27.2 linux/arm64 CGO=0 test binary: полные HIT-76/HIT-78 регрессии и новые
  preparation tests. Это отдельный Linux DB прогон, не Linux Go race.
- Процессы действительно завершаются `os.Exit` на write/fsync/rename/dir-fsync
  и после реального SQL commit до ответа; повтор/новое соединение сохраняют intent.
- Контейнер удалён, отсутствие подтверждено. Existing DB/DSN и внешние аккаунты не используются.

Точные argv/cwd/UTC/env/exit/source/stream digests: `verification/commands.json`.
Полные оригинальные stream-файлы с этими именами — в `verification/raw-run.tar.gz`;
plain aliases полных Go race/vet и PostgreSQL test stdout/stderr доступны рядом.
`summary.json` и `test-binary.json` фиксируют среду/образ/границы и hash binary.
Ожидаемые SQLSTATE отказы, readiness probes и проверка отсутствующего контейнера
имеют собственные проверенные ненулевые exit codes, не переименованные в успехи.

Воспроизведение из корня, локальный Docker и точный Go1.27.2:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_preparation.py --go /absolute/path/to/go1.27.2/bin/go --output /new/empty/output/path
```

Первый прогон выявил отсутствие fixture-файла у переносимого Linux binary —
исправлено embed точных синтетических bytes. Второй выявил расхождение точности
expiry с PostgreSQL — исправлено до успешного повторного прогона. Прежние
verification-01…05 сохранены локально с собственными SHA и результатами;
последний verification-06 публикуется без переписывания исторических журналов.

## Матрица DoD

| Критерий | Проверка и результат |
|---|---|
| Reuse HIT-72/76/77/78, исходный SHA | source-audit.json; byte-identical reused files, unchanged financial kernel/SQL |
| Свежие допустимые facts + все guards; unknown/expired/mismatch/cancel/revoke/lifecycle | TestPreparationUnknownExpiredMismatchedAndCanceled, TestPreparationContentAvailabilityAndGuards; нет нового intent |
| Unpaid → paid, прежняя identity/config/activation | TestPreparationUnpaidThenPaidSamePins; WAITING_DATA → WORKING одного дела |
| Конкурентные оценки / CAS / один intent | TestPreparationConcurrentCASOneIntent; обе оценки имеют исходный snapshot, принят один |
| OAS, accepted mappings, provenance/JCS, неизменность повтора | TestGoldenRenderingAndDigest, TestRenderingRejectsAmbiguityAndExpiry, paid/restart/process-crash tests |
| Durable content, permissions, symlink/expiry/corruption | contentfiles tests, TestPreparationContentAvailabilityAndGuards, short_context bound; недоступное удерживает действие |
| File/SQL failure/crash/restart, без сети под lock, race/vet/PG регрессии | TestCrashBetweenFileSteps, TestPreparationFileFaultsBeforeSQL, TestPreparationCommitFailureAndLostAck, TestPreparationProcessCrashAfterSQLCommit, TestPreparationActualReaderNoNetworkUnderRowLock; последний прогон PASS |
| Honest local outcome / pending outbox | README и result JSON: нет dispatch/ClickUp readback/CONFIRMED/MVP claims |

## Границы результата

Проверена локальная композиция с тестовыми owner grants/epochs и controlled HTTPS
Intake fixture. Нет runtime IAM/Intake/Stripe интеграции, Delivery dispatch worker,
ClickUp create/readback, применения observations, пользовательских API и lifecycle
activation/pause/resume. Сетевой запрос в тесте относится только к fixture чтения
источника, не к Delivery. Нет утверждения о production fencing, физической потере
питания, полной backup/restore/erasure эксплуатации, нагрузке или всём MVP.

После регистрации существующего результата/evidence выполняется handoff-preflight.
Только его свежий официальный READY позволяет предложить личный перенос в In Review.
Независимое review, решение Lead и обычная интеграция PR остаются внешними шагами;
этот документ их не подменяет и не выставляет собственное принятие.
