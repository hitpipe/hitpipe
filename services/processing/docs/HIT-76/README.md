# HIT-76 — схема PostgreSQL и изоляция Processing

Выполнено Codex по делегации Тимофея Патрушева / Tim1hik.

Исходный main: 20adc01a037148d20ea8da6c281f52d25cb2d9e3. Проверен весь
Processing и отсутствие готовой миграции/ролей (команда и результат в
source-audit.json). Существующая реализация HIT-72 уже в этом main сохранена.

Код проверен на чистом коммите:
008a6f1b0d8276fbdfc3f8755989c2ff81abf6de.

Добавлены scripts/migrations/{001_processing.sql,roles.sql,apply.sql,README.md}
и scripts/postgres/{verify.py,README.md}. Формальный processing.sql сохранён
байт-в-байт, SHA256 a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93.
Роли создаются отдельно. Миграция исполняется под отдельным владельцем схемы;
повтор сообщает ALREADY_INSTALLED и сохраняет installed_at.

Настоящий PostgreSQL18.6 из закреплённого образа успешно выполнил миграцию и
SQL-проверки. Протокол: sql-verification/commands.json, итог summary.json. Всего
87 вызовов инструментов (включая Docker/setup/readiness/cleanup), из них 31
ожидаемый SQL-отказ с проверенным exit3 и точным SQLSTATE. Это не 87 бизнес-тестов.
SQLSTATE42501 подтверждает запрет прав/RLS, 23505 — unique, 23514 — CHECK,
P0001 — запрет изменения через принятый trigger. Иная ошибка не считается успехом.

Проверены действительные подключения app/worker, отсутствие SUPERUSER,
BYPASSRLS, владения схемой/CREATE и членства owner/migrator/purge. На всех12
tenant tables проверены ENABLE/FORCE RLS. Без workspace tenant rows недоступны;
SET LOCAL показывает только выбранную область и сбрасывается в следующей
транзакции того же соединения. Чужая запись и запись без scope отклоняются.
job_index — отдельно разрешённые worker metadata, schema_version — версия схемы;
они не выдаются за tenant content с RLS.

Отклонены: повтор Session identity, вторая ACTIVE SINGLE operation, изменение
каждого pinned case поля, accepted config (правами), initial activation и
episode identity/clock, уменьшение/превышение attempt count и episode >300s.
Сбой вставки outbox откатывает предшествующий UPDATE case в той же транзакции;
после отдельного успешного COMMIT новое соединение видит обе записи, прежний
clock/counter и действующие ограничения.

Среда SQL: Linux/arm64 PostgreSQL18.6 в Docker Desktop на macOS; fsync,
full_page_writes,synchronous_commit=on. Полные версии клиента/образа/сервера
и digest — в сыром протоколе. Контейнер без сети/портов, временный PGDATA и
собственная новая database; cleanup удалил контейнер/его ресурсы (последние
записи команды), повторный inspect не находит его. Это не подключение к базе
Project OS, соседнего сервиса или пользователя. Синтетические UUID/refs,
без служебных ключей. Unix-socket trust — свойство только изолированного теста.

Go1.27.2 darwin/arm64, GOTOOLCHAIN=local: go test -race -count=1 ./... — все6
пакетов PASS; go vet ./... — exit0. Действительные команды/cwd/env/UTC/выводы/
хеши — go-verification/. SQL на Linux-контейнере и Go на macOS не смешиваются.
Финансовая логика, Go dependencies, соседние сервисы и shared CI не менялись.

Воспроизведение: scripts/postgres/README.md. Журналы каждого запуска имеют свою
версию/время; предыдущие журналы не переписаны. Неудачная ранняя настройка runner
и исправление до проверенного коммита описаны в source-audit.json.

Граница результата: проверены схема и изоляция Processing. Приём событий,
полномочия IAM, runtime epochs, активация, HTTP, payload files и сквозной
Stripe → ClickUp не реализованы и не проверены. SET LOCAL не подтверждает
полномочия установить scope. Временная память PGDATA и новое соединение не
доказывают устойчивость к потере диска/питания, backup/restore или готовый deployment.
Независимый вердикт и человеческая приёмка — отдельные будущие шаги.
