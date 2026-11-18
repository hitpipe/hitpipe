# HIT-95 — owner read API и начальная история, CCR редакция3

Выполнено Codex по делегации Тимофея Патрушева / Tim1hik. Локальные проверки всех
9 критериев пройдены; это не независимое review, приёмка или разрешение merge.
Официальный handoff READY определяется отдельным ответом Project OS.

## Что изменено

listCases/getCase/getCaseHistory дают непустые OAS200 через приватный HTTPS owner
adapter. Сохранены BFF mTLS/TLS1.3, fresh authority и общий epoch/actual-start gate
до SQL/content и повторно до начала ответа, scoped readonly transactions/RLS,
подписанные bounded cursors и owner/integrity content reader. Без обязательных
dependency ports/cert/CA нет разрешающей production композиции.

По [Lead12640](https://hitpipe.atlassian.net/browse/HIT-95?focusedCommentId=12640)
и [CCR12646](https://hitpipe.atlassian.net/browse/HIT-95?focusedCommentId=12646):

- order_label — точный service_label принятого config_id дела после scope/digest
  проверки; без fallback/усечения. Это каталожное название, case_id хранит identity.
- next_action — утверждённые русские тексты и приоритеты1–9, bounded reason catalog,
  явное неизвестное знание и запрет повторного create. available_actions=[]
  корректно отражает отсутствие подключённого recovery/command-authority.
- SIGNAL_ADMITTED использует существующие HISTORY envelope/files HIT-79: одно
  действительное время PostgreSQL, fsync→rename→directory fsync до единого commit
  inbox/case_identity/safety_history/history_content. Durable ACK только после
  успешного commit; дубликат не продлевает TTL/не переписывает файл.
- После expiry читается минимальная safety history + достоверная expiry metadata
  без обращения к удалённому body. episode_id=null означает отсутствие подробности
  в ответе, а не отсутствие эпизода в прошлом. Legacy без срока остаётся503.

[Полная карта каждого обязательного поля](FIELD_MAP.md),
[source audit](source-audit.json), [замороженный договор3](contract-r3.json).
Финансовые правила, formal SQL/OAS, другие сервисы, зависимости/common CI не менялись.
Единственный разрешённый writer repair — начальная история этого admission.

## Версии и фактическое состояние стека

Прежний draft head `45c6723543f6597f2b39161fb19545361a51d15e` сохранён в истории.
База `3bae6417421465868c432fe18b379d46be6144aa` (HIT-79/PR52), включающая HIT-78
`c218eb3d87c1df51d999d955d263b9d410f77bd7`. PR50/52 наблюдались OPEN, main —
`ce771d37505e7770003d0796ae932556d06b7bab`; зависимый стек не выдаётся за main.
Отдельный HIT-80/PR53 сохранён, оптимизация не повторялась и ветки не переписывались.

Проверенный текущий код `15ed1e567907bee951715b3b025c657e2009044d`, чистая рабочая
копия на старте запуска. После него публикуются только эти документы/протоколы.
Head→CI фиксируются внешней записью Jira, без коммитов ради собственного SHA.

## Настоящий протокол текущей версии

[96 commands/cwd/env/time/source SHA/exit](verification-r3/commands.json),
[итог](verification-r3/summary.json), [14 actual API captures](verification-r3/api-capture.json),
[полный raw archive](verification-r3/raw-run.tar.gz).

Host Go1.27.2 darwin/arm64: `go test -race -count=1 -v ./...` и `go vet ./...`,
оба exit0. [Полный race stdout](verification-r3/go-race.stdout),
[vet stdout](verification-r3/go-vet.stdout), [Go env](verification-r3/go-env.json).
DB tests в этом host прогоне явно SKIP; он не выдаётся за Linux/DB race.

Отдельно Go1.27.2 linux/arm64 CGO_ENABLED=0 на настоящем PostgreSQL18.7 Bookworm:
приватный Unix socket, network-none, fsync/full_page_writes/synchronous_commit on;
полные миграция/роль/RLS/HIT-78/HIT-79 регрессии и новые API/storage tests exit0.
[Полный PG stdout](verification-r3/postgres-tests.stdout). Образ:
`docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`.
Linux-race, physical power-loss и production fencing не заявлены.

Проверены:

- настоящий TLS1.3 handshake, отсутствующий/чужой/просроченный/untrusted cert и
  TLS1.2; сессия, forged authority headers, stale grant/epoch, deny до SQL и после
  SQL перед response start. До разрешения нет чтения/защищённого200;
- non-owner/NOBYPASSRLS app role, SET LOCAL в READ ONLY REPEATABLE READ, pool reuse
  после commit/error/rollback, параллельные разные workspace, чужие case/cursor,
  scope reset и реальный SQL запрет INSERT внутри readonly transaction;
- history100+2, непустые case страницы с limit1/продолжением, case SQL keyset102
  строк/фильтр/bounds, отсутствие повторов и чужих refs; GET не меняет бизнес-состояние;
- pinned catalog при смене current config/PAUSED, context missing/present/expired
  после purge, nullable due/target/observation/history cursor и nullable history
  fields; известный task ID/финансовый VALID/outbox не становятся confirmed URL/
  readback/CONFIRMED. Сохранённая verification boundary не объявляется live health;
- русский policy: все состояния/виды knowledge, приоритеты, отсутствие/неизвестность/
  отказ команды, запрет create даже при тестовом qualified RECHECK, labels1..120,
  точное сохранение Unicode, отсутствие raw diagnostics и предел512;
- реальные file faults (write/fsync/rename/directory fsync), SQL commit failure,
  lost ACK, actual process exit24 после настоящего admission COMMIT; свежий reader
  и реальный HTTPS читают его историю без ручного создания initial metadata;
- точная граница expiry на реальных сохранённых metadata с контролируемым read clock;
  также actual wall-clock HTTPS после expiry, purge-before-read, повторы, новая
  listener/content композиция после writer process crash, стабильный expiry;
- non-null episode в живой preparation history → null после expiry с неизменными
  остальными минимальными фактами; legacy без срока и повреждённая metadata503;
- missing/corrupt/wrong-hash/symlink/foreign-owner content, traversal, actual SQL
  deadline/cancellation, закрытое DB соединение и rollback до emit.

Все14 captures проверены против exact OAS SHA256:
`a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e`.
[Вывод schema validation](verification-r3/schema-check.stdout). Валидатор покрывает
закрытое используемое подмножество, fail-closed на неизвестных schema keywords и
проверяет sensitivity удалением required поля; общий JSON Schema engine не заявлен.
Включены непустые Cases/CaseDetail/History200, empty filter, Error400/403.

[5 supplemental commands](supplemental-r3/commands.json) exit0: Go, gofmt, unchanged
formal migration/financial/application/dependencies/other services/CI, повтор exact
OAS validation и точный список изменений относительно прежнего PR54.
Все stream-файлы, названные commands.json, находятся как members raw-run.tar.gz;
важные stdout также открываются отдельно. [Digests](evidence-digests.json) не включают
сам manifest. Протоколы хранят реальные ожидаемые отрицательные SQLSTATE exits,
начальные readiness probes и cleanup-probe exit1 (own container отсутствует).
Это не «все процессы exit0», а успешно проверенная группа положительных/отрицательных
условий. Временный контейнер удалён, existing DSN/working data не принимались.

## Воспроизведение

Из корня exact checkout, с Docker local Unix context, закреплённым Go1.27.2 и
указанным образом PostgreSQL18.7, в новую папку результатов:

```sh
GOTOOLCHAIN=local PYTHONDONTWRITEBYTECODE=1 python3 \
  services/processing/scripts/postgres/verify_case_reads.py \
  --output /absolute/new-empty-run \
  --go /absolute/pinned-go1.27.2/bin/go \
  --oas /absolute/exact/mvp-openapi.json
```

Runner создаёт только свой новый изолированный стенд, проверяет exact migration/OAS,
выполняет host race/vet, отдельный Linux DB/API binary, схему и cleanup. Тестовые PKI
keys генерируются в памяти; controlled IAM/epoch обозначены как fixtures, не
разрешающие runtime реализации. Private literal IP:8443, loopback:0 только для теста.

## История и границы

`verification/`, `supplemental/`, `development-*.tar.gz` — неизменённые результаты
прежней редакции2, не текущая qualification. Их исходный пакет доступен на commit
45c6723; прежний source audit сохранён в history/. Запрос Lead теперь разрешён
решением12640/CCR12646. Новые промежуточные r3 прогоны —
[revision3-development-runs.json](revision3-development-runs.json): первый упал до
тестов из-за остановленного Docker, остальные PASS на своих SHA до добавления
последующих регрессий. Они не переписаны как текущая версия.

Нет live IAM/BFF/браузера/Stripe/ClickUp, production credentials/shared runtime
fencing, physical power-loss, полного MVP или коммерческой готовности. Это
локальный owner slice в согласованной области. Legacy history без достоверной
expiry metadata остаётся503 согласно Lead; новых миграций/выдуманного срока нет.
Полная подробность expired history не восстанавливается. Подключение recovery и
реального provider observation consumer — другие задачи, не скрытая часть GET.

## Матрица девяти frozen DoD

| ID | Фактическое локальное evidence |
|---|---|
| b9f41472-fa52-49e4-b4a8-da8cd12059de | FIELD_MAP.md, source-audit.json, contract-r3.json; supplemental-r3/commands.json |
| 3cf87f68-f4b9-46d5-8d51-a321406b698c | verification-r3/api-capture.json; schema-check.stdout; TestReadActualMTLSPostgresContentComposition; revision3 projection tests |
| 85960fe2-8786-4db6-aa52-fe53d72fdfa2 | api-capture.json; TestRevision3PinnedLabelContextExpiryPausedAndNoCommands; FinancialValidRead; StoredConfirmationBoundary; History entries and exact schema |
| 6735d7ae-bf94-408f-82b7-bf77d744981f | casereadhttp TestRealMTLSAndFreshReadBoundaries/ParsingAndUnavailableComposition; actual DB response gate |
| 4655d050-7561-44a3-9ce2-36eee2be78c9 | PG role/RLS regressions; TestReadHistoryParallelWorkspacesAndNoBusinessWrites; ActualStoragePaginationAndGaps; readonly rollback scope |
| a8653ea1-519c-418b-b60f-700e2fdfd308 | CaseSQLKeysetAndReadonly; actual HTTP case limit1 continuation; history100+2 and scoped codec tests |
| 9283c299-11b6-4c6e-8f16-17ed30e70822 | ContentFaultsAndSQLCancellation; ReadOwned integrity/traversal; expiry/minimal history/purge tests |
| 03a2dba3-4e8c-418c-8020-563c5ae4f5d8 | commands.json: host race/vet, Linux actual PG18.7/mTLS/content, schema, old regressions; explicit environment/limits |
| a82f2f3c-5c31-418f-bffd-95d807d38d01 | TestRevision3ActualAdmissionHistoryReplayExpiryPurgeRestart; AdmissionFileAndCommitFaultsNoACK; AdmissionCrashThenHTTPExpiryPurgeAndRestart; LegacyMissingExpiryAndCorruptMetadataFail |

Локальная проверка критериев завершена. Регистрация результата/evidence и официальный
handoff-preflight выполняются после публикации текущего head и CI. Только полный
свежий READY позволяет предложить человеку In Review; независимая проверка и личное
принятие остаются отдельными действиями. Агент не сливает PR и не меняет Jira status.

## Представление evidence для Project OS

Первый handoff-preflight №12702 (официальный ответ №12706) дал `NOT_READY`: `review evidence content exceeds bounded context budget`. Причина — полный `verification-r3/commands.json` (255437 bytes) повторяет одну и ту же среду для каждой команды. Фактические журналы и результаты сохранены без изменений. Для повторной передачи подготовлен `verification-r3/review-commands.json`: те же96 exact commands, cwd/env/source SHA, times/exits и digests, с вынесенными общими контекстами. Обратное восстановление всех записей побайтно по JSON-значениям сверено с исходным журналом; это упаковка существующих результатов, не новый прогон. Реестр требует замены EVID-002 этим компактным TEST evidence. Официальная повторная готовность определяется новым handoff-preflight после подтверждённой замены, а не этим документом.
