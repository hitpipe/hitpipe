# Delivery: схема хранения (HIT-70)

`migrations/001_delivery.sql` — исполняемый SQL раздела **HP-MVP-SQL-DELIVERY** из [PERSISTENCE.md, fedbe40](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/PERSISTENCE.md). Байты совпадают с исходным [formal/sql/delivery.sql](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/delivery.sql), SHA-256: `9b23a5a0f8617025f20012f38d0b961010d9d4fbbbc47c3059c1b651a16f155e`. В объединённом Markdown после SQL дополнительно стоит пустая строка оформления.

Применение предназначено только для отдельной базы Delivery. До миграции администратор этой базы создаёт `hp_delivery_migrator`, `hp_delivery_app`, `hp_delivery_worker`, `hp_delivery_purge` с `NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS`. Миграция выполняется от мигратора, владельца базы. Рабочие роли не владеют объектами и не наследуют purge. Сама миграция транзакционная, однократная; повторный запуск не является обновлением версии.

## Воспроизведение проверки

Нужны Docker с Linux-контейнерами и Python 3 (только стандартная библиотека; Python используется для тестового запуска). Из корня репозитория на Windows:

```powershell
py -3 services/delivery/tests/check_schema.py --report services/delivery/evidence/postgresql-18.7.json
```

На Linux та же команда начинается с `python3`. Используется точный PostgreSQL 18.7 Alpine `linux/amd64` по поправке безопасности `HP-CCR-POSTGRES-20261114` к [закреплённому stack-mvp.json](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/stack-mvp.json), digest образа: `sha256:fa1b61f5a4844f4d4b73c9b15716cfc75fe70c931f2d32723630f3b900056366`. Материалы зависимостей: `hitpipe/stack-artifacts@84249ff3ba611358232294c46f6022f030ea223d`.

Версия сервера отдельно проверяется по `server_version_num = 180007`. Старый
протокол `evidence/postgresql.json` относится к первоначальному запуску 18.6 и
сохраняется как история; новый запуск использует отдельный файл. Обновление кода
стенда само по себе не означает, что новый SQL-прогон уже выполнен.

Тест создаёт временный контейнер с `--network none`, без опубликованных портов и host volumes, отдельные базу и роли. Локальная trust-аутентификация действует только в этом одноразовом контейнере. Данные синтетические. Миграция применяется через `SET ROLE hp_delivery_migrator`; обычные запросы проверяются через рабочие роли, администратор используется только для подготовки fixtures и чтения каталогов. Контейнер удаляется в `finally`; неуспех прерывает запуск и не создаёт положительный отчёт.

Проверяются 12 таблиц, их владелец, функция и триггер; девять принудительных RLS-политик; доступ к каждой tenant-таблице в своей и чужой рабочей области для всех трёх ролей; матрица восьми SQL-привилегий на всех таблицах, включая MAINTAIN; права схемы и функции; отклонение записи в чужую область; отсутствие DDL/purge-полномочий у приложения и worker; уникальность lineage и сообщений; доказательство KNOWN_NO_EFFECT; неизменность fence и identity; запрет возможного эффекта → PREPARED.

Отчёт содержит фактический commit проверяемого кода, хеши SQL и тестового файла, команду, среду и результаты каждой проверки. Commit с отчётом может быть следующим: исходный SHA внутри отчёта относится к исполнявшимся миграции и тесту.

## Поиск существующего результата и границы

Исходная версия `hitpipe/hitpipe`: `ce771d37505e7770003d0796ae932556d06b7bab` (main на момент начала). `git ls-tree -r --name-only <SHA> services/delivery` не нашёл файлов. Поиск `CREATE SCHEMA hp_delivery` и `CREATE TABLE effect_lineage` во всём дереве, кроме исторических `docs/labs`, также пуст. Найденный `docs/labs/pr12/upstream/architecture/formal/sql/delivery.sql` относится к выведенной учебной очереди; он не является принятой исполняемой миграцией Delivery. Применимый [отчёт HIT-59](https://hitpipe.atlassian.net/browse/HIT-59?focusedCommentId=11098) использован как исследовательский контекст, его исторические PR не выдаются за текущий main. Открытые PR не используются и не изменяются. До этой задачи применимых существующих тестов Delivery на main нет.

Реестр архитектуры: [systemdesign.json, 0ff88d1](https://github.com/hitpipe/project-os/blob/0ff88d118e39a5dfaa34540c0d621649c34655a7/contracts/hitpipe/mvp-2026-10-31/phase-candidates/systemdesign.json). Этот результат ограничен схемой и локальным SQL. Он не доказывает полномочия пользователя, фактическое начало сетевого запроса, работу ClickUp, полный протокол отзыва, файловое хранилище содержимого или рабочее развёртывание. RLS требует предварительной авторитетной проверки scope в приложении. Внешние сервисы, рабочие базы и другие сервисы не затронуты. Отклонений от принятого SQL нет.

## Текущее замечание CI

В [PR #49](https://github.com/hitpipe/hitpipe/pull/49) старый учебный workflow `pr12-ds.yml` завершился ошибкой до тестов: `lab-component.json is missing`. Он требует manifest выведенной учебной очереди для любой заполненной папки Delivery и не запускает новые SQL-проверки. Полная диагностика: [evidence/ci-status.json](evidence/ci-status.json), [запуск CI](https://github.com/hitpipe/hitpipe/actions/runs/43576499951). Успешные локальные проверки не означают успешный CI. Нужен согласованный владельцем репозитория актуальный маршрут проверок Delivery; workflow и учебные материалы вне области этой задачи не изменены.
