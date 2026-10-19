# HIT-49 / NB-R1 — requirements baseline для StripeSignature

Статус подготовки: `CONTENT_READY_IMAGES_PENDING` для этой карточки; это ручной
`MANUAL_LAB_PR12` результат, не formal Development admission и не принятие
Candidate-контрактов. Автор: Никита Будыкин (`nb`). Дата фиксации: 2026-10-19,
Europe/Moscow. Jira/Project OS state для этой offline-копии: `UNKNOWN`.

## 1. Граница и положение в MVP

Участок `services/intake` принимает синтетически воспроизводимый Stripe webhook
как недоверенный ingress. Собственный модуль `StripeSignature` отвечает только за
проверку точных входных bytes до JSON parsing:

```text
untrusted HTTP bytes + Stripe-Signature + installation-scoped secret + trusted clock
    -> VERIFIED / rejected (safe reason)
```

`VERIFIED` означает только криптографически проверенный вход. Он не означает
`ACCEPTED`, `paid`, durable receipt, normalized envelope, 2xx, financial eligibility
или downstream business action. Durable receipt/outbox/admission/transport остаются
будущими границами и не входят в HIT-49/HIT-51.

В общей кандидатной цепочке это ранняя часть пути:

```text
source ingress -> raw signature proof [этот участок]
               -> strict parse/whitelist -> durable receipt -> financial proof
               -> owner path/readback
```

Канонический Product/Architecture статус по lock: `Candidate/In Review`,
`grant=none`, `authority_ready=false`, `execution_authorized=false`. Поэтому эта
baseline — design input для учебного среза, а не запись о принятом HP-VER-001.

## 2. Источники, pins и ограничения

Точные локальные источники проверены из `docs/labs/pr12` по
`context-lock.json`/`source-files.json` (`missing=0`, `mismatch=0`).

| Источник | Что использовано | Статус |
| --- | --- | --- |
| `upstream/architecture/HP-BP-002.md` | T/001: webhook signature до parsing; v1; HMAC-SHA256; `timestamp.raw-body`; constant-time compare; absolute 300 s; reject unknown/contradictory inputs | Candidate/In Review |
| `upstream/architecture/HP-BP-007.md` | boundary errors, safe diagnostics, unknown не превращается в разрешение, raw Stripe bytes не заменяются JCS | Candidate/In Review |
| `upstream/product/HP-DOC-PVL-001.md` | место в полном single-operation пути; этот модуль не равен полной операции | Candidate/In Review |
| `upstream/product/HP-DOC-NFR-001.md` | измерения и эксплуатационные NFR не объявляются локальным unit-test результатом | Candidate/In Review |
| formal input pin | `NormalizedSignal` и `/webhooks/stripe/{endpoint_id}` из указанного OpenAPI пути | Candidate/In Review; native file pinned but not re-authenticated here |
| `context-lock.json` | Architecture digest `18f94e1c…e418b`, Product digest `e2a5ab63…cd71`, Discovery digest `39e808…76fd`, formal commit `84249ff3…a223d` | immutable package pin |

Точный набор учебных источников: четыре методички МДК01.04 ПР1/ПР2 и МДК03.02
ПР1/ПР2 из `inputs/`; их bytes и SHA закреплены в `source-files.json`.

## 3. Обследование и происхождение сведений

### Реальное интервью

`NOT_CONDUCTED` на момент этой фиксации. Реальные ответы пользователя или
согласованного преподавателем представителя не выдумывались и не заменены
ответами Lead, продуктовым текстом или выводом агента. Human gate `HIT-55` остаётся
открытым.

Подготовленные вопросы для записи человеком:

1. Какие Stripe event types и account/mode считаются допустимыми для первой
   операции, и кто подтверждает это для конкретной установки?
2. Что должен увидеть оператор при отсутствующем, просроченном или
   противоречивом `Stripe-Signature`?
3. Должны ли повторные доставки того же event ID проходить криптографическую
   проверку каждый раз до deduplication?
4. Какой следующий шаг разрешён при verified signature, но не установленном
   durable receipt/financial proof?
5. Какие диагностические поля разрешены, а какие bytes, secrets и provider data
   запрещено писать?

Протокол, который должен заполнить человек: дата/часовой пояс, роль и имя
собеседника по согласованному разрешению, точные ответы, источник каждого ответа,
новые решения и расхождения с текущей baseline. До этого ответы имеют статус
`UNKNOWN`, а не PASS.

### Факты, решения, гипотезы и открытые вопросы

| Категория | Запись | Происхождение/статус |
| --- | --- | --- |
| Факт источника | Intake владеет webhook receipt/Stripe adapter/secret boundary | HP-BP-002, Candidate/In Review |
| Факт источника | Проверка подписи идёт до parsing и до business action | HP-BP-002/T/001 |
| Факт пакета | В окружении нет live Stripe deliveries и credentials | HIT-51 limitations / shared package |
| Учебное решение | `StripeSignature` — pure domain policy; secret/clock — consumer-owned ports | `IMPLEMENTATION_CONVENTION.md`, применимо к этой задаче |
| Учебное решение | CLI печатает только safe verdict/reason/version и не называет результат paid/accepted | HIT-51 AC3 |
| Гипотеза | Установка передаст installation scope как непротиворечивый opaque route | не проверено без интервью/реального integration fixture |
| Гипотеза | Фиксированные synthetic vectors достаточно покрывают локальную rule-логику | будет проверено C1/C3; не доказывает provider behavior |
| Открытый вопрос | Точный provider event/Stripe API version для конкретной установки | human/product confirmation |
| Открытый вопрос | Требуемый durable receipt port и 2xx policy за пределами C1 | future component work |
| Открытый вопрос | Допустимый объём и retention normalized envelope | formal/product resolution; не менять в этой карточке |

## 4. Проверяемые требования и acceptance oracles

Оракул ниже записан до реализации: это ожидаемый исход, вычисляемый из pinned
правил и synthetic inputs, а не переписанный фактический вывод программы.

### FR-SIG-01 — raw-body proof

**Given** exact raw bytes `B`, Unix timestamp `t`, installation secret `K` и
header с хотя бы одной допустимой `v1` подписью, **when** вычисляется
`HMAC-SHA256(K, decimal(t) + "." + B)` и сравнивается constant-time,
**then** результат `VERIFIED` только при совпадении MAC и прохождении freshness.

Оракул: известный offline HMAC vector и независимый расчёт test fixture; изменение
одного byte, повторная сериализация JSON или неверный MAC дают `REJECTED`.

### FR-SIG-02 — strict header and timestamp rules

**Given** `Stripe-Signature`, **when** parser видит malformed segment, отсутствие
`v1`, пустую подпись, отсутствующий timestamp, конфликтующие timestamp или только
`v0`, **then** он отклоняет вход без попытки сделать его verified.

**Given** `now=t0`, **when** `abs(t-t0)` равен 299 s или 300 s, **then** вход
остаётся допустимым по этому правилу; при 301 s — отклоняется. Нельзя передать
`tolerance=0` для отключения проверки.

Оракул: таблица классов/границ и decision table в C3; expected outcome не зависит
от порядка заголовочных токенов или первого найденного conflicting timestamp.

### FR-SIG-03 — safe failure

**Given** missing installation secret, unavailable trusted clock, unsupported header
shape или unknown installation, **when** consumer проверяет вход, **then** исход
отказной и диагностическое сообщение содержит только safe reason/version.

Оракул: отсутствие `secret`, `raw body`, полного signature header и provider
credentials в stdout/stderr/log fields; проверяется текстовым assertion и review
журнала.

### FR-SIG-04 — no false business admission

**Given** `VERIFIED` signature, **when** ещё нет durable receipt/financial proof,
**then** модуль не выдаёт `ACCEPTED`, `paid`, `2xx` или downstream action.

Оракул: CLI contractcheck и application consumer имеют только typed verification
result; отсутствие route/receipt/outbox в owned scope проверяется import/build
границей и документацией.

### NFR-SIG-01 — secret/data minimization

Для каждого отказа диагностика не раскрывает secret, raw body, signature header или
provider payload; допустимы machine-safe reason, module version и bounded metadata.
Проверка: negative fixtures + grep журнала на forbidden markers.

### NFR-SIG-02 — deterministic bounded verification

На фиксированном synthetic fixture при фиксированном clock result детерминированен;
правило не зависит от JSON parser/re-serializer. Local test reports command, SHA,
tool version and exit; performance target не придумывается без измерения.

### NFR-SIG-03 — reproducible clean-room input

Fixtures содержат synthetic bytes/secret и не содержат реальные customer data,
credentials или private files. Команда запуска из clean clone не требует
неявного рабочего файла.

## 5. Security baseline — five required questions

Полный первичный анализ вынесен в `security-baseline.md`; краткая трасса здесь:

1. **Что защищаем?** Installation-scoped secret, exact raw webhook bytes,
   signature/timestamp, provider account/mode и границу downstream action.
2. **Откуда недоверенный ввод?** HTTP webhook bytes, headers и route identifier от
   внешнего Stripe ingress; raw body нельзя считать trusted JSON.
3. **Кто имеет доступ?** Intake adapter и узкий application consumer по
   installation scope; domain получает value inputs и не читает environment/filesystem.
4. **Какой реалистичный ущерб?** False verified event может привести к ложному
   receipt/business action; утечка secret/body раскрывает provider scope или
   customer data; replay/clock bypass может обойти freshness.
5. **Где секреты и что запрещено логировать?** В production secret принадлежит
   Intake secret adapter/secure store; в лаборатории только synthetic fixture.
   Нельзя логировать secret, raw body, signature header, credential, customer
   payload или выдавать verified как paid/accepted.

## 6. Planned checks и границы результата

| Проверка | Ожидаемый oracle | Карточка | Статус на R1 |
| --- | --- | --- | --- |
| known HMAC vector | `VERIFIED` | HIT-51/HIT-53 | `PLANNED` |
| one-byte body alteration | `REJECTED` | HIT-51/HIT-53 | `PLANNED` |
| 299/300/301 past/future | 299/300 accepted by freshness, 301 rejected | HIT-51/HIT-53 | `PLANNED` |
| malformed/missing/conflicting header | `REJECTED`, safe reason | HIT-51/HIT-53 | `PLANNED` |
| unavailable secret/clock | `REJECTED`, no sensitive output | HIT-52/HIT-53 | `PLANNED` |
| clean-room/CI/mutation | actual logs and exact SHA required | HIT-53 | `NOT_RUN` |

Эта карточка не заявляет, что перечисленные проверки выполнены. Human review,
interview, clarity check, opposition, merge and screenshot gates остаются pending.

## 7. Personal contribution and limitations

Мой вклад R1: выделение собственного raw-body signature slice, формулировка
проверяемых FR/error/NFR, fixture/security baseline и traceability. Общие charter,
MVP, stack и Candidate sources не выдаются за личное обследование или accepted
contract.

Не реализуются: live Stripe account/delivery, provider credential setup, durable
normalized receipt/outbox, financial eligibility, 2xx webhook route, source-read
proof, production deployment и acceptance HP-VER-001.
