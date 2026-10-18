# SessionPolicy и AuthenticateSession — локальный контракт v2

Task HIT-18, после R1 commit `0661a3ea64bd9f5243a9a7f5b3e1195c31eef152`. Сначала спецификация, затем реализация HIT-19/20. Это внутренняя проекция IAM, не изменение OAS/SQL. До C1 документ находится в личном пакете; при создании module канонический файл переедет в `services/iam/docs/module-contract.md`, а эта страница станет указателем.

## Ответственность и публичная операция

DK-FR-01/02 → установить пригодность прикладной сессии → `sessionpolicy.Evaluate(snapshot, observation) Decision` → application consumer `authenticatesession.Service.Execute(ctx, reference) Result`.

Domain принимает значения, не обращается к IO/системному времени, не изменяет вход. Snapshot содержит только `Found`, `Revoked`, `CreatedAt`, `ExpiresAt`, `IdleExpiresAt`. Никаких cookie, digest, actor/workspace ID, profile или credentials. Observation содержит `At` и явный `Trusted`. Данные caller-owned; результат — независимое value с State и безопасным Reason.

## DK-RULE-01 — пригодность времени и хронология

Все time values должны быть ненулевыми и представимыми в UTC в диапазоне годов 1..9999. Неопределённое время не равно «нулевой дате». Точность сравнения — наносекунда, сравниваются instants; другой UTC offset сам по себе не меняет решение.

Для известной неотозванной сессии требуется:

- `CreatedAt < ExpiresAt`;
- `CreatedAt < IdleExpiresAt <= ExpiresAt`;
- `ExpiresAt - CreatedAt <= 24h`;
- `observation.At >= CreatedAt`.

Сравнения корректности выполняются до проверки дедлайнов; арифметика не использует `UnixNano()` или непроверенное сложение. Разница time.Sub может насыщаться на дальних датах, но такой положительный интервал заведомо больше 24h и отвергается. Не происходит принятия переполненного значения как короткого интервала.

Владелец сессии вычисляет idle deadline по своей последней активности с пределом 8h и absolute cap. У минимального snapshot нет last-activity timestamp; эта policy проверяет предоставленный deadline, а не выдаёт доказательство правильности выдачи/продления. Для синтетических исходных данных первоначальный idle=created+8h, absolute=created+24h. Продление, SQL migration и обновление активности здесь не реализуются. Нельзя ограничить idle фиксированными created+8h: это ошибочно запретило бы законно обновлённую сессию.

## DK-RULE-02 — равенство и порядок причин

`now >= deadline` означает истечение; никакого grace period. Valid возможно только при `now < idle` И `now < absolute`.

| Приоритет | Условие | State | Reason |
| --- | --- | --- | --- |
| 1 | Время не Trusted, zero или вне диапазона | denied | clock_uncertain |
| 2 | Found=false | denied | unknown_session |
| 3 | Revoked=true | denied | revoked_session |
| 4 | Некорректные time values, порядок или lifetime >24h | denied | invalid_snapshot |
| 5 | now раньше CreatedAt | denied | not_yet_valid |
| 6 | now >= ExpiresAt | denied | absolute_expired |
| 7 | now >= IdleExpiresAt | denied | idle_expired |
| 8 | Иначе | valid | session_valid |

Все отказы закрывают доступ. При одновременно истёкших дедлайнах reason=`absolute_expired`. При Unknown и Revoked одновременно — `unknown_session`; при Revoked и некорректных сроках — `revoked_session`. Это локальный устойчивый порядок диагностики, а не новая информация о чужом объекте на публичном API. Default/zero Decision не является Valid.

## DK-RULE-03 — application ports и свежесть

Потребитель владеет двумя узкими ports:

1. `SessionLookup.Lookup(context.Context, string) (sessionpolicy.Snapshot, error)`;
2. `Clock.Now(context.Context) (sessionpolicy.Observation, error)`.

Constructor отвергает nil и typed-nil реализации. Execute также безопасен на нулевом/неинициализированном Service. Reference — внутренний непрозрачный ключ длиной 1..128 ASCII `[A-Za-z0-9_-]`, первая буква/цифра. Это не схема cookie, digest или публичного Session UUID; production transport adapter остаётся отдельной обязанностью.

Порядок: проверить состав зависимостей → context/reference → получить snapshot через Lookup → проверить отмену context → получить Clock observation → проверить отмену и доверенность времени → вызвать policy. **Время считывается после Lookup**: задержка получения snapshot не должна оставить caller с прежним now. Сам Lookup обязан возвращать текущую authority; файловый fixture этого не доказывает.

| Сбой | Application Status | Безопасная причина / действия |
| --- | --- | --- |
| Неинициализированный Service | unavailable | dependencies_unavailable; IO нет |
| nil context / неверный reference | denied | invalid_request; IO нет |
| context cancelled/deadline | unavailable | request_cancelled |
| Lookup error | unavailable | lookup_unavailable; Clock не вызывается |
| Clock error / недоверенное или непригодное время | unavailable | clock_unavailable |
| Корректный ответ Lookup с Found=false | denied | unknown_session после пригодного Clock |
| Policy denied | denied | фиксированный Reason policy |
| Policy valid | valid | session_valid, **без бизнес-гранта** |

Ошибки адаптера не пробрасываются в response/log: там могут оказаться пути, PII или секреты. Ошибка одного port не заменяется cache, прежним результатом или default allow. Context cancellation не является доказательством глобального revoke/fencing.

## Диагностический fixture и CLI

Версия локального JSON fixture — целое `1`; верхний объект содержит только `version`, `session`, `clock`. `session` содержит `found`, `revoked`, `created_at`, `expires_at`, `idle_expires_at`; `clock` содержит `trusted`, `now`. Это отдельная синтетическая форма диагностического adapter, не HP-FORMAL-001 wire.

Для known session и clock требуемые поля не принимают null/отсутствие. В unknown projection timestamps могут отсутствовать; присутствующие timestamp всё равно должны разбираться. Boolean presence отличается от false. Timestamp — RFC3339Nano с offset. Строгий decoder отклоняет duplicate/extra keys, unknown version, неверные типы, trailing document, malformed JSON и input >64 KiB; глубина JSON ограничена 16. Значения из ошибок parser не печатаются.

`iam-contractcheck --fixture PATH --format text|json`, `--version`. Путь и формат влияют на выполнение; отсутствующий/неверный путь, формат и broken fixture дают exit 2. Структурно корректное решение policy, включая безопасный denied, даёт exit 0: отказ сессии — ожидаемый бизнес-результат, не неисправность CLI. После C2 `--lookup file|fixed` меняет adapter в одной точке сборки, без правки Execute. `--clock fixture|system` выбирает фиксированное синтетическое время либо host clock adapter; последний остаётся unavailable без реального clock-health probe, а не объявляет `time.Now()` доказанно доверенным.

На stderr только фиксированные коды старта/исхода; stdout — state/reason либо build metadata. Профили, reference, fixture body и canary не выводятся. Повторный запуск не меняет исходные данные, не создаёт sessions, не выполняет внешних команд.

## Примеры oracle до реализации

При created=00:00Z, idle=08:00Z, absolute=следующие 00:00Z: now=idle−1ns→valid; now=idle→denied/idle_expired; revoked=true→denied/revoked_session. При idle=absolute и now=absolute→denied/absolute_expired. ExpiresAt=created+24h+1ns→denied/invalid_snapshot, даже если now ещё до deadline. Clock failure после успешного Lookup→unavailable/clock_unavailable.

## Границы вывода

Не реализованы OIDC/PKCE/CSRF, cookies, SQL/RLS/durability, membership/tenant/business authorization, session renewal, global revoke/epoch/start barrier или production endpoint. File-backed и fixed adapters синтетические. Их взаимозаменяемость доказывает contract потребителя, а не работу настоящего IAM authority. Нормы HP-BP-001/T/001–014 и HP-NFR целиком этим module не закрываются.
