# HIT-81 — проверенные миграция и защита схемы IAM

Схема IAM теперь устанавливается отдельной командой. Администратор готовит роли,
мигратор создаёт согласованные таблицы, а приложение, обработчик и очистка получают
только свои права. Повторный запуск распознаёт установленную версию.

Ниже описан **реальный успешный запуск 15 ноября 2026 года**, а не обещание будущей
проверки. Исходные протоколы скопированы без изменения байтов; их хеши сверены с
[`verification.json`](verification.json). История неудачных отладочных запусков
сохранена отдельно в журнале оператора. В них исправлено кодирование пробела в
параметре подключения: `%20` вместо `+`; роли и согласованный SQL не менялись.

## Редакция кода и использованные источники

- Проверенная исходная main: `ce771d37505e7770003d0796ae932556d06b7bab`.
- Фактически испытанный код: [`829934b4dab057661f1a63d8cc009645d9f515e9`](https://github.com/hitpipe/hitpipe/commit/829934b4dab057661f1a63d8cc009645d9f515e9).
- SHA-256 дерева `services/iam` без `evidence` и `__pycache__`: `b53318496d30d03bd72622332633832ef3ad5593f4b51e2fdd19bfa5bdf63327`.
- Адаптер `SessionLookup`, его тесты и ядро HIT-68 переиспользованы. Каталог адаптера
  побайтово не менялся между указанной исходной и испытанной редакциями; его девять
  настоящих PostgreSQL-тестов снова прошли на новой базе.
- [`schema/001_iam.sql`](../../internal/adapters/postgres/migrations/schema/001_iam.sql)
  — точные байты [принятой схемы IAM](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/iam.sql).
  SHA-256: `604d693ff49ccde71755d2b7afccaa0e9b96f03fef2d2c0e420c08c7ab59bef7`. Совпадение проверяется также `TestAcceptedMigrationBytes`.
- PostgreSQL **18.7 Alpine**, согласованный образ:
  `postgres:18.7-alpine@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73`.
  Основание замены версии — [принятая поправка безопасности](https://github.com/hitpipe/project-os/blob/d9eb399d9cc0528f1beae6153851ab5020741ea5/contracts/hitpipe/security-2026-11-14/POSTGRES_SECURITY_PATCH_RU.md).
- Использован прежний Go **1.27.2**, `linux/amd64`, pgx **5.11.0**; `go.mod` и
  `go.sum` этой задачей не менялись. Набор зависимостей не обновлялся молча.

Изменённые исходники в испытанной редакции (`A` — добавлено, `M` — изменено):

```text
M	services/iam/README.md
A	services/iam/cmd/iam-migrate/main.go
A	services/iam/cmd/iam-migrate/main_test.go
A	services/iam/internal/adapters/postgres/migrations/README.md
A	services/iam/internal/adapters/postgres/migrations/bootstrap_roles.sql
A	services/iam/internal/adapters/postgres/migrations/migrate.go
A	services/iam/internal/adapters/postgres/migrations/migrate_integration_test.go
A	services/iam/internal/adapters/postgres/migrations/migrate_test.go
A	services/iam/internal/adapters/postgres/migrations/permissions.go
A	services/iam/internal/adapters/postgres/migrations/schema/001_iam.sql
A	services/iam/internal/adapters/postgres/migrations/schema_integration_test.go
A	services/iam/tools/check_iam_schema.py
M	services/iam/tools/check_postgres_session_lookup.py
```

Протоколы в этом каталоге оформляются отдельной следующей редакцией. Поэтому их
коммит закономерно отличается от испытанного коммита кода; `source_commit` и хеш
исходного дерева в JSON остаются фактическими.

## Что произошло при запуске

Проверка началась `2026-11-15T11:13:43Z`, завершилась
`2026-11-15T11:16:12Z`. Итог — **PASSED**. Все 11 записанных этапов
завершились с кодом **0**. Восемь пакетов IAM прошли обычные тесты и проверку с
обнаружением гонок. `go vet` и сборка успешны.

Использовалась собственная временная база с синтетическими данными. PostgreSQL
ограничен 256 МиБ памяти и 0,5 CPU, доступен только через `127.0.0.1`; данные —
в tmpfs. Go: `GOMAXPROCS=1`, `GOGC=50`, параллелизм сборки 1. Существующие базы
и контейнеры не использовались. Подключения `HITPIPE_IAM_*` извне игнорируются.
После проверки контейнер и его отдельная сеть удалены: оба признака очистки в
JSON равны `true`.

Миграционный вход проверен реальным TCP-подключением и выбором `hp_iam_migrator`.
Первый запуск команды вернул `installed`, версия **1**, согласованный хеш SQL;
второй — `already_installed` с той же версией и хешем. Тест на новом соединении
подтвердил, что запись версии и время её установки не пересоздаются.

В протоколе с PostgreSQL выполнены **17 основных `TestIAMSchema*` и 56 вложенных
случаев**, а также **9 существующих `TestPostgres*`**. Число вложенных случаев не
обозначает число независимых исследований безопасности. Пропущенных тестов нет.

| Проверка | Сохранённый настоящий результат |
| --- | --- |
| Подготовка четырёх групповых ролей и отдельных временных входов | [`bootstrap-roles.log`](bootstrap-roles.log), [`test-logins.log`](test-logins.log) |
| TCP-вход мигратора и выбор роли | [`migration-role-tcp.log`](migration-role-tcp.log), [`migration-startup-role.log`](migration-startup-role.log) |
| Установка версии 1 | [`migration-install.log`](migration-install.log): `installed`, код 0 |
| Повтор установки | [`migration-repeat.log`](migration-repeat.log): `already_installed`, код 0 |
| Отдельный вход чтения сессий | [`read-only-lookup.log`](read-only-lookup.log), код 0 |
| Обычные тесты | [`unit.log`](unit.log), восемь пакетов, код 0 |
| Настоящие PostgreSQL-тесты и обнаружение гонок | [`postgres-race.log`](postgres-race.log), восемь пакетов, код 0 |
| Статическая проверка Go | [`vet.log`](vet.log), код 0; пустой вывод означает отсутствие сообщений |
| Сборка | [`build.log`](build.log), код 0; пустой вывод означает отсутствие сообщений |

## Отдельный запуск CI

[GitHub Actions №43708079414](https://github.com/hitpipe/hitpipe/actions/runs/43708079414)
успешно проверил тот же коммит `829934b4dab057661f1a63d8cc009645d9f515e9`.
Из нативного артефакта CI прочитаны протокол и журналы; хеш исходного дерева
совпал с локальным. Итог: PostgreSQL 18.7, 58 основных проверок всех пакетов
(включая 17 проверок схемы и 9 проверок SessionLookup), ноль пропусков, хеши
журналов совпали, временный контейнер и сеть удалены.

Сводка проверки скачанного артефакта — [`ci-proof.json`](ci-proof.json); она
содержит хеш самого ZIP. Локальные сырые журналы этого каталога не выдаются за
журналы CI. Успех GitHub Actions подтверждает технический запуск проверок и
не является вердиктом независимого Reviewer Project OS или приёмкой задачи.

## Что именно проверено

На всех десяти таблицах с рабочей областью проверены права и RLS — защита строк:
три настоящих прикладных входа видят только свою область, без области не видят
строк; чужая вставка/смена области и чужая запись отклоняются. Отдельно проверены
принудительная RLS для владельца и отсутствие переноса `SET LOCAL` в следующую
транзакцию на том же соединении после `COMMIT` и `ROLLBACK`.

Все 17 таблиц принадлежат `hp_iam_migrator`. Прикладные входы не являются
владельцами, суперпользователями или ролями обхода RLS; каждый наследует только
свою группу. Приложение и обработчик не получают права очистки. Проверяется
матрица SELECT/INSERT/UPDATE/DELETE, в том числе отдельные права `job_index`.

Проверены уникальность issuer+subject, хеша сессии и внешнего подключения;
ссылочная целостность сессии; выраженные в принятом SQL ограничения хешей, эпох,
времени сессий и отзыва. Приложение и обработчик могут добавлять записи аудита и
квитанции команд, но не менять или удалять их; право очистки проверено отдельно.

Общие таблицы IAM (`principals`, `sessions`, `runtime_registry`, `revocations`,
`revocation_participants`, версия схемы) сохраняют собственную границу IAM.
Значение `hp.workspace_id` не изображает бизнес-полномочие на эти данные.

Новое физическое соединение видит ранее подтверждённую запись. Ошибка второго
оператора откатывает всю свою транзакцию и не повреждает предыдущий commit.
Отдельная управляемая ошибка DDL проверяет откат **самой миграции** без частичной
схемы и успешный чистый повтор. Проверены отказ при чужом владельце, неполной или
неизвестной версии, ослабленных RLS/политике/правах, опасных default privileges,
попытке миграции прикладной ролью или внутри открытой транзакции. Два одновременных
мигратора дают ровно одну установку и одно распознавание установленной версии.

## Связь с восемью критериями HIT-81

Имена ниже присутствуют со статусом `PASS` в [`postgres-race.log`](postgres-race.log).
Подтверждение границ и исходной редакции дополняется исходниками и этим отчётом;
оно не выдаётся за функциональный тест ещё не реализованного сервиса.

| Критерий и его точный идентификатор | Выполненные проверки и результат |
| --- | --- |
| 1. Исходный SHA, пути, повторное использование HIT-68 — `76ec899d-6c60-4ec8-8bf6-a99cfe39a4ae` | Исходная и испытанная редакции и список путей выше; неизменённый адаптер. `TestPostgresExactSnapshotAndExistingConsumer` и остальные восемь `TestPostgres*` прошли. |
| 2. Точный SQL, отдельные роли, повтор — `e5384e61-a671-442c-9912-1f54adb4f512` | `TestAcceptedMigrationBytes`, `TestIAMSchemaMigrationRepeatOnFreshConnection`, `TestIAMSchemaMigrationConcurrentInstall`; фактические ответы CLI `installed` / `already_installed`. |
| 3. Владелец и разделение ролей — `4460368e-0b04-44cf-a7dc-ba82ae5e0934` | `TestIAMSchemaRoleSeparationAndOwnership`, `TestIAMSchemaRuntimePrivilegeMatrix`, `TestIAMSchemaMigrationRefusesRuntimeAndOpenTransaction`. |
| 4. Изоляция областей и область действия SET LOCAL — `96d8fea0-6ec7-400c-8ca2-9f66a1acd084` | `TestIAMSchemaTenantRLSReadAndWriteIsolation` (30 сочетаний таблица/роль), `TestIAMSchemaOwnerCannotBypassForcedRLS`, `TestIAMSchemaSetLocalDoesNotLeakAcrossTransactions`. |
| 5. Уникальность, время, неизменяемые записи — `4aada482-109b-4169-b504-ee485f53fa09` | `TestIAMSchemaGlobalUniquenessAndForeignKeys`, `TestIAMSchemaSessionAndRevocationConstraints`, `TestIAMSchemaAppendOnlyAuditAndCommandReceipts`. |
| 6. Отдельная общая граница IAM — `48b5498b-26d5-481c-89f7-7db0bdbce889` | `TestIAMSchemaGlobalTablesAreIAMAuthorityWithoutTenantFiction`, `TestIAMSchemaJobIndexIsWorkerAndPurgeOnly`, матрица прикладных прав. |
| 7. Настоящие протоколы, новое соединение, откат — `3ca164e0-5788-4457-b5d6-6d7a6e17c951` | Полные команды/версии/хеши/коды в `verification.json`; `TestIAMSchemaCommitIsDurableAndFailedTransactionLeavesNoPartialWrite`, `TestIAMSchemaMigrationAtomicDDLFailure`, `TestIAMSchemaMigrationRepeatOnFreshConnection`. |
| 8. Честная граница результата — `8f3e243b-abcf-4872-b075-3bed935adf39` | Раздел ниже и [инструкция миграции](../../internal/adapters/postgres/migrations/README.md). Исполняемые проверки охватывают схему и локальный компонент; работа продукта и его развёртывание не заявлены. |

## Как повторить

Из корня репозитория, с Docker и согласованным Go 1.27.2:

```sh
python3 services/iam/tools/check_iam_schema.py --evidence-dir /tmp/hit81-new-check
```

Каталог должен отсутствовать или быть пустым; для каждого повтора выбирайте
новый. При необходимости добавьте `--go /полный/путь/к/go`. Старое имя
`check_postgres_session_lookup.py` запускает тот же актуальный проход для CI.
Команды Go выполняются внутри `services/iam`:

```sh
go run -p 1 ./cmd/iam-migrate  # первый запуск через отдельный временный DSN
go run -p 1 ./cmd/iam-migrate  # повтор через тот же DSN
go test -p 1 -count=1 ./...
go test -p 1 -tags=integration -race -count=1 -v ./...
go vet -p 1 -tags=integration ./...
go build -p 1 ./...
```

DSN задаёт сам одноразовый runner, значения не нужно копировать в рабочую среду.
Полные фактически исполненные команды, включая подготовку ролей, сохранены в JSON;
приведённые выше команды — удобная памятка, а не отдельный новый запуск.

## Границы и ограничения

Готовы миграция и воспроизводимая проверка схемы **во временной базе одного
сервиса IAM**. Эта задача не подтверждает работающий вход, выдачу сессий,
прикладные grants, распределённый отзыв или готовность к эксплуатации. Другие
сервисы, клиент, Keycloak и рабочие базы не менялись. Выбранная рабочая область
сама по себе не даёт прикладных полномочий; общая граница IAM должна соблюдаться
прикладным кодом. Проверялись именно ограничения принятого SQL; отсутствующие в
нём правила не приписываются базе.

Параметры `fsync`, `full_page_writes`, `synchronous_commit` были `on`, таблицы —
обычные журналируемые. Сохранение после нового соединения проверено, но аварийное
восстановление, потеря питания и долговечность рабочего хранилища не испытывались:
данные временного стенда находились в tmpfs.

Сохраняется известное ограничение общей группы сборки **Go 1.27.2**. В
[исторической записи безопасности HIT-68](../HIT-68/security-note.json) зафиксированы
`GO-2026-6859` и `GO-2026-6809`; она остаётся действительным источником ранее
обнаруженных замечаний, а не новым сканированием в HIT-81. Новый поиск уязвимостей,
проверка достижимости уязвимого кода, рабочий HTTP/TLS и безопасность развёртывания
здесь не проводились. Перед эксплуатацией общей группе сборки нужна отдельная
согласованная поправка; версии не менялись в обход принятого порядка.
