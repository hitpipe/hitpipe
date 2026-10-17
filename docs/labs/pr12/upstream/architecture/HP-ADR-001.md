# HP-ADR-001 — Микросервисные границы и пять направлений разработки

**ID ADR:** HP-ADR-001. **Рабочее состояние revision:** In Review / Candidate. **Статус архитектурного выбора:** Accepted by delegated autonomous architect. Основание полномочия — пользователь поручил полную Architecture Design и делегировал остальные архитектурные решения; это действительное принятие выбора исполнителем в пределах делегирования, **не** человеческая Project OS source acceptance, не Accepted Architecture baseline и не разрешение live execution.

## 1. Контекст и область решения

Нужно независимо распределить разработку между пятью людьми, сохранив сквозную безопасность одной сопровождаемой операции. Пользователь предпочёл Go, Python для AI, Next.js/TS/Tailwind и четыре backend-границы: identity, приём, обработка, отправка. Разделение по процессам не должно создавать несколько владельцев case state, credentials или неизвестного внешнего эффекта.

Контекст — точный Product candidate `sha256:e2a5ab63b35ce807c4de7bd79b30858fa0e54161eae252d022882fc8ceb7cd71`, HP-VER-001. Драйверы: HP-RULE-001/004/005/008/010/011, HP-NFR-001/005/006/007/009/014, HP-VIS-D-009. Продуктовые capabilities и сервисы не приравниваются; пять направлений — ролевые слоты, не утверждение о найденных/назначенных сотрудниках.

## 2. Выбранный вариант и rationale

Четыре Go runtime: HP-PART-001 IAM/profile,002 Intake/Stripe,003 Processing,004 Delivery/ClickUp. Дополнительно HP-PART-005 Python AI Interaction и006 Next.js UX/BFF. PostgreSQL007, Keycloak008 и edge009 — инфраструктурные части с отдельными trust boundaries.

IAM владеет identity/membership/epochs, Intake — приёмом и source facts, Processing — конфигурацией/операцией/единственной case machine/ответственностью, Delivery — credentials назначения и effect lineage. Python предлагает конфигурацию, а не выполняет бизнес-правила. BFF обслуживает UX и same-origin session surface, не становится привилегированным backend. Общее runtime-правило и wire source принадлежат HP-BP-007 и HP-FORMAL-001.

Пять workstreams: IAM+platform; Intake/source; Processing/core; Delivery/destination; UX+BFF+PythonAI. Владелец общей интеграции контролирует схемы и сквозной acceptance. Core сложнее остальных; его изменение review-ят source/destination owners. Разделение позволяет параллельную разработку после фиксации общих контрактов, но не выдаёт сервисы за полностью независимые по безопасности/данным.

## 3. Рассмотренные альтернативы

| Вариант | Соответствие и trade-offs | Решение |
| --- | --- | --- |
| Модульный монолит | Меньше distributed states и локально проще; хуже соответствует явному пожеланию распределить отдельные backend microservices | Не выбран для данного поручения; остаётся допустимой альтернативой при существенном изменении команды/стоимости эксплуатации |
| Выбранные четыре Go домена + PythonAI + NextBFF | Соответствует полномочиям/языкам, отделяет provider side effects и сохраняет одного владельца case | Выбран; цена — строгие HTTP/outbox contracts, barriers и распределённый review |
| Отдельный сервис на каждую capability, provider endpoint или экран | Размножает совместные transactions, credentials и повседневные deployments без доказанной необходимости | Не выбран; разделение по экрану не является доменной ответственностью |
| Единый универсальный workflow/LLM agent backend | Быстрый прототип ценой смешения generation и execution и лишнего LLM в штатном пути | Не выбран: противоречит выбранной независимости исполнения от LLM |

Численных оценок производительности/скорости команды не заявляется: данное сравнение — инженерное решение по известным ограничениям, не проведённый командный эксперимент.

## 4. Последствия и канонические технические источники

| Следствие | Канонический источник смысла | Согласованный контекст |
| --- | --- | --- |
| Единственный владелец каждого DATA и PART | HP-DOC-AD-001; HP-BP-001–007 | Все сервисы и SQL assets; cross-service SQL запрещён |
| Пять рабочих потоков и общая интеграция | HP-DOC-AD-001; HP-BP-006 | Общие schemas/gates первыми, затем service mocks и реализация |
| Межсервисная доставка, а не shared tables | HP-BP-007; HP-ADR-002 | Durable intent/ACK/state, eventual propagation не заменяет security revoke |
| AI не подключается к production case/credential store | HP-BP-005 | HP-NFR-007; принятая operation исполняется Go-путём |

Риски: более дорогой integration/debug, Processing bottleneck по сложности, центральная IAM/gate зависимость, общая начальная БД как fault domain. Они приняты как цена выбранного организационного разделения; механизмы и проверки принадлежат соответствующим BP, не этой таблице. Издание документа не доказывает runtime capacity или фактическую staffing готовность.

## 5. Пересмотр, исторические связи и изменения

Пересмотр нужен, если реальная команда/операционная стоимость не поддерживает шесть runtime, ownership постоянно требует синхронных распределённых записей или измеренная нагрузка/новые packs требуют другого разделения. Новый сервис требует существенного нового DATA/effect/trust boundary, а не просто нового endpoint. Первый целевой ADR этого выбора; superseded accepted product architecture не заявляется. Legacy код — технический baseline, а не автоматически принятая новая архитектура.

Миграция к иному разбиению требует нового ADR, согласованных AD/BP/wire revisions и impact review активных configs/cases, security gates и frozen work. Новая страница не переключает исторические pins.

## 6. Проверка перед source review и faithful-проекцией

Выбор один и явен; ownership и пять направлений определены; реальные альтернативы сопоставлены; новая capability не добавлена. Проверка полного source set должна подтвердить, что ни один state/credential/effect не получил второго владельца и все микросервисы включены в revoke/restore. Общая independent source review и точные source revisions закрепляются в Architecture package. Содержательное adoption архитектурного выбора отделено от формальной source acceptance и ещё не выполненных implementation/E2E проверок.
