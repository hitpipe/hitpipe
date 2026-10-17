# HP-ADR-004 — Полный отзыв через epochs и synchronous runtime barriers

**ID ADR:** HP-ADR-004. **Revision:** In Review / Candidate. **Выбор:** Accepted by delegated autonomous architect; основание — явное делегирование архитектурных решений пользователем. Это не подпись пользователя под source set, не формальный ACCEPTED witness и не реализация gate.

## 1. Контекст и область решения

HP-NFR-005/006 требуют применения допустимого отзыва всей области≤5 секунд, positive confirmation только после применения и отсутствие новых чтений/обращений после positive. JWT expiry, TTL cache, eventual event и проверка только в начале handler оставляют check→actual IO race. Приостановленный процесс может продолжить старый вызов после отзыва. Область — каждый runtime, способный начать protected read/response, user proposal или provider call, включая BFF и connector workers.

## 2. Выбранный вариант и rationale

IAM хранит per-scope deny epochs и конечный deployment runtime registry. Revoke сначала durable commits deny+intent и показывает Pending. Каждый boot имеет CLOSED startup gate до актуального полного epoch snapshot. Runtime применяет synchronous barrier, сериализованный с actual read/response/HTTP-start critical section по HP-BP-007. ACK подтверждает не получение сообщения, а невозможность нового old-epoch start. Новая generation не начинает работу, пока старая не fenced.

Positive Applied выдаётся только после всего затронутого roster ACK либо действительного infrastructure fence, исключающего networkingress+egress и доступ старого runtime к DB. Удаление из балансировщика, timeout, истёкшая lease и намерение kill не являются proof. Уже фактически начатые вызовы остаются in-flight и могут иметь эффект; неизвестный effect не превращается в безопасный retry. Pending сверх5 секунд — измеримый NFR violation и incident, не разрешённое исключение или фиктивный успех.

Для стартового контура фиксированный roster и ограниченное число executor instances упрощают проверяемое fencing. Для последующего HA потребуется реальная проверка старого исполнителя, а не lease-only promotion. Низкая latency и устойчивость coordinator должны быть проверены fault matrix; архитектурное описание не является её результатом.

## 3. Рассмотренные альтернативы

| Вариант | Проблема или trade-off | Решение |
| --- | --- | --- |
| Дождаться короткого JWT/cache TTL | Между проверкой/expiry и actual call остаётся окно; нельзя обещать отсутствие start после positive | Не выбран как enforcement |
| Асинхронно распространить revoke event | Нужен для доставки состояния, но молча допускает неответивший/старый runtime | Не достаточен без barrier |
| IAM introspection в начале каждого handler | Хороший current grant check, но остаётся проверка→чтение/отправка и возобновление замороженного процесса | Используется вместе с gate, не заменяет его |
| Один центральный proxy для всех данных и provider calls | Может централизовать часть starts, но добавляет иной data/network bottleneck; требуется всё равно учесть DB и response surfaces | Не выбран для начального ownership; возможен будущий новый ADR |
| Epoch+actual-start barrier+подтверждённый fencing | Явно выражает требуемую границу; сложнее реализация и partition ведёт к fail-closed | Выбран; цена соответствует продуктовому требованию |

## 4. Последствия и канонические технические источники

| Следствие | Единственный владелец смысла | Контекст |
| --- | --- | --- |
| Registry/deny commit/revoke status и полный scope | HP-BP-001/T/009–012 | Membership/session/support/connection revoke различаются по scope, а не по ослаблению гарантии |
| Actual start critical section, ACK и transport запреты | HP-BP-007 | Protected SQL≤2s, network start≤2s; callback/признак фактической отправки не заменяется spawn |
| Физический fence и replacement boot | HP-BP-006/T/004 | Старый runtime не получает новые ingress/egress/DB возможности после возврата |
| Provider possible-effect lineage | HP-BP-004 | Barrier не обещает отменить уже начатый внешний effect |

Риски: временная недоступность затронутой области при partition, сложность правильного Go transport instrumentation, неполный roster, долгое query/start section, потеря последнего control overlay при restore. Их нельзя исключить из NFR измерения задним числом. Нужны реальные suspend/reconnect/race tests и наблюдаемые actual-start timestamps.

## 5. Пересмотр, исторические связи и изменения

Новый runtime, autoscaling, HA/failover, alternate DB driver/transport, long-lived response streams или новый provider расширяют gate surface и требуют compatibility/security review до допуска. Если реализация не укладывается в5 секунд, архитектурный механизм исправляется или затронутая эксплуатация удерживается; ADR не ослабляет канонический NFR. Прежние Accepted source pins и historical effect knowledge сохраняются. Это первый целевой ADR данного механизма, не утверждение о внедрённом gate в legacy коде.

## 6. Проверка перед source review и faithful-проекцией

Независимый reviewer сопоставляет весь roster с AD/IF, все start paths с BP007, физический fence с platform BP и restore с актуальными epochs. Обязательные испытания: freeze между check/start, slow connect/write/SQL, lost barrier ACK, control network partition, old generation resume, implicit HTTP retry, positive-confirm race, already-in-flight completion. Любой новый old-access start после positive или ложный Applied — FAIL. Успех иных тестов не компенсирует этот дефект. Фактическое прохождение ещё не заявлено; source acceptance, content fidelity и runtime conformance различаются.
