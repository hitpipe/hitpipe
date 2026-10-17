# HP-ADR-002 — PostgreSQL outbox/inbox и durable HTTP

**ID ADR:** HP-ADR-002. **Рабочее состояние:** In Review / Candidate. **Выбор:** Accepted by delegated autonomous architect; полномочия из текущего пользовательского поручения. Это не human acceptance и не execution grant.

## 1. Контекст и область решения

Раздельные сервисы должны сохранять принятый intent, не терять подтверждённое состояние при контролируемом отказе и безопасно повторять доставку без повторного ClickUp effect. Продукт требует case identity, неизменной конфигурации и честного неизвестного исхода, а не технологического exactly-once. Scope: HP-IF-004/006/007, внутренние durable команды/события и все service databases. Основания HP-RULE-001/004/008/013, HP-NFR-001/003/004/008/014.

## 2. Выбранный вариант и rationale

Один начальный PostgreSQL cluster с отдельными service databases/SQL roles. Каждая локальная business transaction сохраняет state+outbox; consumer transaction сохраняет inbox+state+свой outbox. Outbox worker передаёт HTTP-сообщение с постоянным ID и content hash; повтор после lost ACK — штатная at-least-once доставка. Получатель применяет неизменный ID идемпотентно и подтверждает только durable результат. Совпавший ID с другим содержанием — конфликт, не новая версия сообщения.

Бизнес-уникальность обеспечивают отдельные owner constraints: Session case identity в Processing и create-effect lineage в Delivery. Inbox dedup не заменяет их. External write start имеет отдельный durable fence, и DISPATCHING/AMBIGUOUS никогда не повторяется по истечению transport lease. Общая реализация protocol принадлежит HP-BP-007; provider-specific safety — HP-BP-002/004. Межсервисный network IO не выполняется под удерживаемым SQL row lock; нет cross-service SQL/foreign key/двухфазного SQL commit.

## 3. Рассмотренные альтернативы

| Вариант | Trade-offs | Решение |
| --- | --- | --- |
| Простые синхронные HTTP-вызовы без durable outbox | Меньше инфраструктуры, но lost response/crash оставляет непереносимое намерение и опасные повторы | Не выбран для принятых фоновых операций |
| PostgreSQL outbox/inbox + HTTP | Уже нужен durable owner state; достаточно для выбранного bounded pack, прозрачно тестируется | Выбран; требуется собственная понятная доставка/lag/poison recovery |
| Kafka/NATS/другой broker | Полезен для большого fan-out/replay/масштаба, но добавляет новый operational domain; не решает внешнюю idempotency сам | Не нужен для MVP без измеренного основания |
| Temporal/универсальная workflow платформа | Даёт workflow tooling; добавляет новый стек и не снимает provider unknown-effect проблему | Не выбран для конечной case machine и небольшого числа durable timers |
| Shared database tables между сервисами | Упрощает отдельные join, но уничтожает ownership и независимость миграций | Не выбран; сервисное API является единственной границей |

Публичные нагрузочные ступени — критерии будущей квалификации, не измеренная ёмкость выбранного PostgreSQL. Отсутствие broker не является обещанием бесконечного throughput.

## 4. Последствия и канонические технические источники

| Следствие | Точный владелец | Контекст |
| --- | --- | --- |
| ID/hash, inbox/outbox/ACK, retry/deadline и incompatibility | HP-BP-007; HP-FORMAL-001; formal SQL | Все durable сообщения, без повторного авторства в каждом BP |
| Session uniqueness и immutable config admission | HP-BP-003 | Один business case независимо от event ID/version/episode |
| Effect lineage и conservative possible-effect fence | HP-BP-004 | Повторяется доставка intent/observation, а не сомнительная provider запись |
| DB durability, backlog visibility и восстановление | HP-BP-006 | RPO0 только в явно выбранном контролируемом fault contour |

Риски: lag/claim contention, poison messages, необходимость поддержать protocol migrations и ограничить backlog. Их conformance и метрики фиксируются в BP; generic HTTP retry не может самовольно создать новую business attempt. Нельзя использовать transport success как подтверждение task readiness.

## 5. Пересмотр, исторические связи и изменения

Пересмотр по фактическим устойчивым lag/contention, потребности независимого fan-out/replay или новому классу workflows, который не помещается в выбранную машину. Само число разработчиков или слово «микросервисы» не требует broker. При смене транспорта сохраняются IDs, consumer dedup, effects и исторические неизвестности; миграция не включает replay всех старых сообщений автоматически. Первый целевой ADR этого решения; исторических принятых pins не заменяет.

## 6. Проверка перед source review и faithful-проекцией

Independent review должен пройти producer/consumer crash до/после commit/send/ACK, повторID/hash conflict, deadline, schema incompatibility и восстановление Delivery fence. Реальные PostgreSQL, network-fault и application E2E проверки ещё не доказаны этим ADR. Контракт сохраняет full BP semantics и formal refs; один ADR summary не заменяет transport Blueprint. Формальная приёмка источников и admission остаются отдельными.
