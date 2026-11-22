# HIT-138. Протокол запуска одного процесса IAM

Проверенный код `d081e4f5c408b2a94286b1193b244f01f41153d7`. Изолированная Ubuntu VM, `go version go1.27.2 linux/amd64`,
PostgreSQL 18.7. Схема установлена отдельным мигратором.
Стенд использует временные сертификаты и тестовые идентичности. База
принятых зависимостей `734c96b88bdd5ce0d5d95e5b895a6b5c25d1c83d` включает HIT-106, HIT-115 и GET profile
из HIT-85.

Протокол включает настоящий дочерний процесс IAM, раздельные mTLS-входы
8443 и 9444, допуск на маршрутах EDGE, BFF, Runtime и OPS, выполнение штатной
регистрации через PostgreSQL, а также отрицательные сценарии при ошибках
настроек, TLS, базы и после перезапуска. Остановка по SIGTERM и завершение
слушателей проверялись на стенде. Использованные доверенные внешние порты
квалифицированы только для явно ограниченных тестовых удостоверений.
Технический сигнал OPS описывает состояние экземпляра.

Прошедшие группы интеграции:

- `TestPostgresIAMProcess`

- [Полный вывод HIT-138](iam-process-full.log) и [исходный тестовый вывод IAM](iam/postgres-race.log).
- [Протокол PostgreSQL и IAM](iam/verification.json), [Go race, vet и сборка](go.log), [регрессии общего шлюза начала работы](platform/verification.json).
- [Окружение](runtime.log) и [очистка стенда](cleanup.log).
- [Точная команда запуска и границы](https://github.com/hitpipe/hitpipe/blob/d081e4f5c408b2a94286b1193b244f01f41153d7/services/iam/docs/IAM_PROCESS_RU.md).
- [Фиксированный OAS](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json), [IAM SQL](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/iam.sql) и [архитектура](https://github.com/hitpipe/project-os/blob/b2461564e4355ea5a72a55a30a635655397c463b/contracts/hitpipe/mvp-2026-10-31/SYSTEM_DESIGN_MVP_RU.md).
- [Поправка PostgreSQL 18.7](https://github.com/hitpipe/project-os/blob/d9eb399d9cc0528f1beae6153851ab5020741ea5/contracts/hitpipe/security-2026-11-14/POSTGRES_SECURITY_PATCH_RU.md).
