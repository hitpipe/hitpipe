# HIT-133 — внутренний процесс Delivery

Из каталога `services/delivery` запустить одну команду на Windows с Docker Desktop
и PowerShell 7:

```powershell
pwsh -NoProfile -File tests/run_hit91.ps1 -RuntimeRevision (git rev-parse HEAD)
```

Для точного повторения сначала открыть проверенный SHA, указанный в
[протоколе запуска](../evidence/HIT-133/verification.txt).
Команда запускает PostgreSQL 18.7 Alpine linux/amd64 с закреплённым digest
`fa1b61f5a4844f4d4b73c9b15716cfc75fe70c931f2d32723630f3b900056366`
в изолированной Docker-сети, применяет неизменённую миграцию Delivery,
собирает два бинарника и поднимает Delivery отдельным процессом
с приватным каталогом содержимого. Выполняются Go race-тесты и vet.
Стенд использует сгенерированные сертификаты, синтетическое поручение и
локальные HTTPS-двойники. Они удаляются вместе с временным окружением.

## Сборка процесса

- Бизнес-слушатель использует существующий mTLS HTTP-адаптер HIT-91
  для `POST /internal/v1/delivery-intents` и HIT-93/HIT-94 для
  `POST /internal/v1/delivery-query`. Обе операции принимают Processing.
- Отдельный OPS mTLS-слушатель обслуживает
  `GET /internal/v1/health/delivery`. Он возвращает OAS Health с
  `READY` только после проверки ролей, схемы и параметров PostgreSQL,
  доступности файлового каталога, действующих областей полномочий и start gate.
  Недоступность зависимости отражается HTTP 503. Первая проверка проходит
  в состоянии CLOSED. Health отражает техническую готовность, а не состояние
  доставки заказа или доступность рабочего ClickUp.
- Один процесс владеет сессийной блокировкой планировщика в своей базе.
  Короткий read-only SQL-опрос по доступным workspace под RLS находит
  готовые попытки, известный ID после create и due outbox. Пакеты ограничены,
  SQL завершён до сети. Существующие Worker, QueryService и Publisher
  самостоятельно перепроверяют полномочия и actual-start gate.
- Create сохраняет fence до передачи запроса. Процесс после рестарта
  восстанавливает чтение известного ID только из durable lineage. Если
  процесс завершился до подтверждения ID, он сохраняет состояние возможного
  эффекта и не пытается искать или создавать задачу повторно.
- Подтверждённое чтение сохраняет наблюдение и сообщение outbox в PostgreSQL.
  Уже сохранённое сообщение отправляет Publisher HIT-93 с тем же
  `message_id`, digest и телом. Он сохраняет bounded backoff и свой lease.
  При недоступном Processing планировщик не заменяет накопленное сообщение.

## Контракты и профили

[Принятый OpenAPI](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json)
определяет DeliveryIntent, Receipt, DeliveryQuery, TaskObservation и Health.
Рабочие порты конфигурации — 8443 для бизнеса и 9444 для OPS.
Стенд использует отдельные динамические loopback-порты. Processing принимает
`POST /internal/v1/delivery-observations` с TLS 1.3, клиентской
идентичностью Delivery, X-Request-ID и каноническим TaskObservation, а
отвечает HTTP 202 с квитанцией после применения.

Проверочный профиль требует отдельной сборки с тегом `delivery_fixture`,
явного `DELIVERY_PROFILE=TEST`, неизменённого синтетического manifest
и loopback-адресов. Он поставляет тестовые реализации authority, start gate
и credential lookup. Обычная сборка не включает эти реализации и завершается
ошибкой до открытия слушателей, если обязательные адаптеры не предоставлены.
В точке сборки `deliveryrun.Run` предусмотрено явное подключение таких
адаптеров владельцем runtime. Рабочее подключение IAM, ClickUp и политика
хранения относятся к последующим карточкам.

Владелец runtime задаёт `DELIVERY_APP_DSN`, `DELIVERY_WORKER_DSN`,
`DELIVERY_CONTENT_DIR`, `DELIVERY_TLS_DIR`, `DELIVERY_BUSINESS_ADDR`,
`DELIVERY_HEALTH_ADDR` и `DELIVERY_PROCESSING_ORIGIN`. TLS-каталог содержит
пары `delivery-server.pem/.key` и `delivery-client.pem/.key` и отдельные
PEM доверия `processing-client-ca`, `processing-server-ca`, `ops-client-ca`.
Приложение и worker используют разные DB-роли. Сервис не применяет миграции
самостоятельно и не использует DB-роль администратора.
