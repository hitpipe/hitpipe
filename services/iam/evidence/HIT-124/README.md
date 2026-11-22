# HIT-124. Протокол приватного IAM API рабочих областей

Проверенный код `c74089f05954e7c974385077523d96c5c3d9ef87`. Изолированная Ubuntu VM, `go version go1.27.2 linux/amd64`, PostgreSQL
18.7, временные сертификаты, сохранённые тестовые сессии и синтетические
данные рабочих областей. База принятых зависимостей `5b35214e5c7a96b01da1148c1df1ddcbe700b78d`.

Проверки зафиксировали HTTP-границы GET и POST через TLS 1.3 mTLS, актуальное
членство и общий барьер IAM, сохранение имени, атомарную запись workspace/OWNER/квитанции/audit/outbox,
изоляцию PostgreSQL и установленный повтор той же команды в известной области.
Стенд заранее связывает разрешённую команду с выделенным ID области. Без принятого
допуска POST закрывается. Без точного ID области квитанция не считается обнаруженной.
`ONBOARDING` не выдаётся в публичной проекции. Рабочий механизм выдачи допуска,
доставка CSRF, GET session, logout, BFF и обработчик локального outbox остаются вне стенда.

Прошедшие группы новой интеграции:

- `TestPostgresWorkspacesPrivateListener`
- `TestPostgresWorkspacesUnknownCommitRecoversDurableReceipt`
- `TestPostgresWorkspacesAtomicCreateAndReplay`
- `TestPostgresWorkspacesRollbackAndApplicationRole`
- `TestPostgresWorkspacesCurrentOwnFactsAndLocks`

- [Полный вывод HIT-124](workspaces-full.log) и [исходный тестовый вывод IAM](iam/postgres-race.log).
- [Протокол PostgreSQL и IAM](iam/verification.json), [Go race, vet и сборка](go.log), [регрессии общего барьера](platform/verification.json).
- [Окружение](runtime.log) и [очистка стенда](cleanup.log).
- [Границы реализации и воспроизведение](https://github.com/hitpipe/hitpipe/blob/c74089f05954e7c974385077523d96c5c3d9ef87/services/iam/docs/WORKSPACES_RU.md).
- [Фиксированный OAS](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json), [IAM SQL](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/iam.sql) и [архитектура](https://github.com/hitpipe/project-os/blob/b2461564e4355ea5a72a55a30a635655397c463b/contracts/hitpipe/mvp-2026-10-31/SYSTEM_DESIGN_MVP_RU.md).
