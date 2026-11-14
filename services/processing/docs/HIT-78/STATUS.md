# HIT-78 — надёжное локальное принятие NormalizedSignal

Реализация подготовлена Codex по поручению Тимофея Патрушева / Tim1hik.
14 ноября по поручению руководителя повторены полные проверки на PostgreSQL 18.7
и обновлены доказательства. Этот отчёт описывает реализацию и тесты. Проверка
готовности, независимое ревью и личная приёмка — отдельные следующие шаги.

## Что сделано и что переиспользовано

Использована схема HIT-76 из `15e684e1c3f78bb7851bc1d4335c9a9f883f5dc1`.
[PR45](https://github.com/hitpipe/hitpipe/pull/45) открыт; проверенная основная ветка
`ce771d37505e7770003d0796ae932556d06b7bab` его не содержит.
[PR50](https://github.com/hitpipe/hitpipe/pull/50) зависит от
`work/HIT-76-processing-postgres`. Схема заново не реализовывалась:
SHA256 миграции сохранён — `a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93`.
Слияние этих PR в основную ветку ещё предстоит.

Пути изменений HIT-78: `internal/application/admission/`,
`internal/adapters/postgres/`, `scripts/postgres/verify_admission_storage.py`,
`go.mod`, `go.sum` и документация Processing.

## Принятое правило времени события

[Решение руководителя 12171](https://hitpipe.atlassian.net/browse/HIT-78?focusedCommentId=12171)
и [уточнение Intake 12172](https://hitpipe.atlassian.net/browse/HIT-73?focusedCommentId=12172)
сохранены с авторами и метаданными в `lead-decision.json`.

`source_created_at` — исходное `Session.created` из подписанного
`Event.data.object.created`, не время Event или получения сообщения.
Новое дело допускается при `Session.created >= operations.first_activation_at`.
Новое поле протокола, вызов readSourceFacts и SQL-колонка не добавлены.
Время Session не подменяется и не синтезируется. Каждый вход проверяется
относительно первой активации; повтор не меняет эту границу и закреплённые данные.
На настоящем PostgreSQL проверены до/точно/после активации и старая Session с поздним
`async_payment_succeeded`: позднее получение события не разрешает старую Session.

## Границы реализации

Точка входа — `admission.Admit(ctx, raw, establishedContext, decoder, store)`.
Декодер из `internal/adapters/postgres/normalizedsignal` подключён через порт
приложения; приложение не зависит от библиотеки JCS напрямую. Проверенные полномочия
вызывающего, привязка источника и принятая редакция операции обязательны.
`AuthorityVerified` — доверенное внутреннее утверждение, не поле запроса.
Контекст нельзя восстановить из NormalizedSignal или заполнить по умолчанию.

`postgres.Store` повторно проверяет контекст; использует READ COMMITTED,
транзакционную RLS-изоляцию рабочего пространства и блокировку операции от смены
состояния/редакции. ACTIVE, конфигурация, владелец и граница первой активации берутся
из локальной БД. Сырой сигнал проверяется до вызова хранилища.

В одной транзакции записываются inbox, уникальная идентичность дела и минимальная
история безопасности. Повтор message_id с тем же digest возвращает исходную
квитанцию, в том числе после паузы; другой digest даёт конфликт. Новые сообщения
известной Session сохраняют дело, конфигурацию, владельца и редакцию. Ошибка commit
не возвращает APPLIED. Потерянное подтверждение commit означает неизвестный исход;
повтор разрешается через сохранённый inbox. Внешних эффектов и повторов здесь нет.

SQL хранит непрозрачные идентификаторы, digest и ограниченные коды безопасности,
без содержимого NormalizedSignal, сырого Stripe payload и бизнес-текста.
Необязательная revision в квитанции опущена: принятый inbox не хранит редакцию
исходного результата. Принятая SQL-схема не менялась.

## Полная актуальная проверка на PostgreSQL 18.7

Измеренный исходный commit — `fa05c7dfd848ba576d2e9be054c9eb791ac96584`;
рабочее дерево перед прогоном чистое. Commit с публикацией журналов не выдаётся
за измеренный исходный код.
Обновление среды согласовано в [HP-CCR-POSTGRES-20261114](https://hitpipe.atlassian.net/browse/HIT-63?focusedCommentId=12229)
и описано в [HP-ADR-010](https://hitpipe.atlassian.net/wiki/spaces/Hitpipe/pages/24608769).

Образ — `postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`,
linux/amd64, фактический `server_version_num=180007`.
Прогон завершён 14 ноября в 13:10 UTC.
[Компактный протокол](admission-verification-pg187-20261114/verification.json)
содержит конкретные команды, итоги, ссылки и SHA256 полных исходных файлов.
Полные argv/cwd/время/коды/stdout/stderr сохранены рядом; `commands.json` не сокращён.

Воспроизведение из корня репозитория с Docker:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_admission_storage.py --go /absolute/path/to/pinned/go --output /new/output/directory
```

Итог: 95 команд, в том числе 75 SQL-команд; 31 ожидаемый отказ проверен по SQLSTATE.
Go 1.27.2 linux/amd64: `go test -race -count=1 -v ./...` и `go vet ./...` — успешно.
Все 11 групп/35 случаев тестов хранилища и принятия отдельно выполнены внутри
настоящего PostgreSQL без пропусков. Отдельный контейнерный двоичный файл собран
с CGO=0 и не инструментирован race. Пропуски DB-тестов на хосте не считаются
их выполнением: они покрыты отдельным настоящим контейнерным прогоном.

Стенд принимает только собственную временную БД, без существующего DSN и сетевых
портов. Прикладная роль без SUPERUSER/BYPASSRLS; fsync/full_page_writes/
synchronous_commit включены. Контейнер удалён, результат очистки сохранён.
Неожиданных ненулевых кодов завершения нет.

| Критерий задачи | Конкретное доказательство |
|---|---|
| Переиспользование HIT-76 и пути изменений | Исходный SHA, сохранённый хеш миграции, пути и зависимость PR45 выше; исходный source-audit.json сохранён |
| Строгая схема и digest | TestCanonicalAndJCS, TestRejectInvalidSignal; некорректные поля/время отклоняются без записи |
| Атомарность, отсутствие преждевременного APPLIED | TestAdmissionCommitFailureNeverAcknowledges, TestRecordRollbackAndCommitFailure; настоящий отказ отложенного ограничения при commit |
| Повтор и конфликт digest | TestAdmissionReplayAfterLifecycleAndNewConnection, TestAdmissionConcurrentAndDigestConflict, TestRecordCommitRepeatConflictAndReconnect |
| Одно дело для Session при конкуренции | TestAdmissionConcurrentAndDigestConflict, TestRecordConcurrentIdentityAndMessage; изоляция Session/workspace/account/mode |
| Контекст, состояние и время активации | TestAdmissionRejectsContextAndLocalState, TestAdmissionActivationBoundary, TestAdmissionTwoSignalsOnePinnedCase |
| Новое соединение, RLS, CAS, регрессии | Переподключение, старая редакция, SQL-регрессии HIT-76, общий Go race/vet |
| Явные ограничения | Следующий раздел и .project-os/results/HIT-78.json |

## Ограничения и следующие действия

Проверены надёжное локальное принятие и тестовая композиция с ранее установленным
контекстом. В тестах этот контекст синтетический. Работающие подключения
IAM/Intake/Stripe, HTTP-сервер, финансовое решение, outbox, Delivery и внешняя
запись этой задачей не реализуются. Испытания потери питания, промышленного
развёртывания и восстановления копий не проводились. Отдельная эксплуатация CVE
не воспроизводилась; новая версия проверена на совместимость.

Старые проверки на 18.6 и прежние редакции остаются историей. Неудачная компиляция
run04, замечание vet run05 и прежний CI-отказ на зависимость приложения не
переименованы в успешные. Декодер исправлен через порт приложения; правила CI не
менялись. Актуальный CI публикуется в PR50 отдельно, без ссылки отчёта на собственный
ещё не созданный commit. После регистрации результата и READY участник лично
переносит карточку в In Review; независимое ревью остаётся следующим шагом.
