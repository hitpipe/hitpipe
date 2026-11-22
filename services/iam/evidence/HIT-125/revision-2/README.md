# HIT-125. Протокол сохранения собственного профиля

Проверенный код `622a5848df71d75d2cd5d2a2f9571057fca9911a`. Изолированная Ubuntu VM, `go version go1.27.2 linux/amd64`, PostgreSQL
18.7, временные сертификаты и синтетические профили. База принятых
зависимостей `29ff2eba9d7f32e4a2320a921566970748cf315b`.

Проверены приватный PATCH через TLS 1.3 mTLS, сохранённая сессия и исходный CSRF от issuer,
проверка текущей ревизии, публикация нового файла до смены SQL-ссылок, отзыв и
сохранение совместимости прежнего GET. После неопределённого commit стенд сверяет
зафиксированную ссылку и сохранённую команду.

Файловая квитанция глобальной команды реализована только в испытательном стенде
и переживает повторное открытие хранилища. У принятых IAM command_receipts есть
обязательный workspace ID, а профиль глобальный. Рабочий PATCH требует отдельного
принятого владельцем механизма глобальной команды и пока закрывается без него.
Примеры для HIT-110 и границы выдачи сессии, доставки CSRF и подключения BFF
закреплены в описании API.

Прошедшие группы новой интеграции:

- `TestPostgresProfileUpdatePrivateListener`
- `TestPostgresProfileUpdateUnknownCommitCanReadDurablePointer`
- `TestPostgresProfileUpdateSerializesCASAndKeepsOwner`
- `TestPostgresProfileUpdateRollsBackAndRechecksCurrentIdentity`

- [Полный вывод HIT-125](profile-update-full.log) и [исходный тестовый вывод IAM](iam/postgres-race.log).
- [Протокол PostgreSQL и IAM](iam/verification.json), [Go race, vet и сборка](go.log), [регрессии общего барьера](platform/verification.json).
- [Окружение](runtime.log) и [очистка стенда](cleanup.log).
- [Границы реализации и воспроизведение](https://github.com/hitpipe/hitpipe/blob/622a5848df71d75d2cd5d2a2f9571057fca9911a/services/iam/docs/PROFILE_UPDATE_RU.md).
- [Фиксированный OAS](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json), [IAM SQL](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/iam.sql) и [архитектура](https://github.com/hitpipe/project-os/blob/0ff88d118e39a5dfaa34540c0d621649c34655a7/contracts/hitpipe/mvp-2026-10-31/phase-candidates/systemdesign.json).

Первый запуск IAM в GitHub Actions завершился отказом при открытии каталога профилей интеграционного стенда. В исправленной фикстуре явно задаются права временных каталогов владельцев. Повторный полный локальный прогон проверил исправленный коммит. Исходные результаты сохранены. [Первый запуск GitHub Actions](https://github.com/hitpipe/hitpipe/actions/runs/45050624842) · [Предыдущий локальный протокол](https://github.com/hitpipe/hitpipe/tree/c916f34c13f52f5873c8b8fef8cdffab4e099644/services/iam/evidence/HIT-125).
