# Technical Blueprint — Intake и Stripe source

**ID:** HP-BP-002. **Состояние:** In Review. **Ответственный:** делегированный архитектор HitPipe; единственный владелец интеграции — Codex root. Это полная кандидатная техническая спецификация; formal acceptance и соответствие реализации не заявлены.

## 1. Граница спецификации и применимость

Область: подлинный durable ingress, ограниченное удержание до case, source facts для HP-PACK-001, source connection и восстановление. Канонические business states/допуск/конфигурация принадлежат Product Definition и Processing HP-BP-003; внешний task effect — HP-BP-004; полномочия/отзыв — HP-BP-001; общий outbox/эксплуатационный контур — HP-BP-006.

Применимость: HP-VER-001, четыре SINGLE-профиля и PILOT NFR-варианты. Сведения о TEAM source scan — граница будущей совместимости, не включение HP-FR-111/112 в MVP.

| Сторона | Обязанность |
| --- | --- |
| Stripe | Внешний источник подписанных событий и читаемого payment state; вне транзакции HitPipe |
| Intake | Единственный владелец receipt/held input/Stripe adapter и secret; нормы ниже |
| Processing | Единственный admission/case/config owner, потребитель authenticated observations |
| Identity | Binding/tenant/permission epoch authority; доверенный callback не даёт business approval |

## 2. Канонические определения и формальные источники

Полные storage constraints принадлежат `formal/sql/intake.sql`; opaque content refs с hash/expiry ссылаются на service-owned durable files HP-BP-006. Реальные payload/source facts и secret ciphertext не сохраняются в SQL/WAL/backup; fsync+atomic rename+directory fsync до reference commit, отсутствие файла→hold.

Wire schemas/required fields/operation selectors принадлежат HP-FORMAL-001: `contracts/hitpipe/architecture-2026-10-16/formal/openapi/*.json`, OpenAPI3.1.0. Exact Git commit/digest/import closure — итоговому manifest; `formal/schemas/*.json` — только generated views, не второй нормативный источник. BP определяет поведение, которое schema не выражает; повторной wire-таблицы здесь нет. Нативный Stripe payload — внешняя versioned representation, не внутренний event schema. Для v1 read requests обязателен `Stripe-Version: 2026-09-30.endive`; webhook installation настраивается на snapshot events той же версии. Минимально необходимые события: `checkout.session.completed` и `checkout.session.async_payment_succeeded`; unpaid completion не paid admission, Thin Events/v2 preview не включены. Неподдерживаемый signed event type безопасно отмечается неприменимым без создания дела. Точные component/DATA/IF links разрешаются интегральной AD и manifest; файлы provider fixtures — supporting test assets, не новый source of truth.

## 3. Точные технические обязательства

### HP-BP-002/T/001

**Роль/сторона/условие:** обязательный Intake ingress guard на каждом webhook. **Действие:** найти существующий installation по opaque route, прочитать исходные bytes с конечным deadline и лимитом HP-NFR-004; до parsing проверить Stripe signature/timestamp строгим HMAC-SHA256 verifier по официальному Stripe manual verification contract для выбранного secret: signed bytes=`timestamp + "." + raw body`, только signatures схемыv1, constant-time comparison, absolute timestamp tolerance300s. Нулевой tolerance, v0 downgrade, отсутствующий/противоречивый timestamp и неподтверждённая подпись запрещены. Повтор event ID не отменяет signature check. Неподдерживаемый content encoding, повреждённый JSON, неизвестная версия/shape или неподтверждённая область не допускают business action. **Исход/предел:** rejected request не создаёт workspace/case; безопасная диагностика не раскрывает чужую область. **Критерий:** fake paid/изменённые bytes не приводят к downstream action; обеспечивает FR101,RULE011,POL001/003,NFR005/006.

### HP-BP-002/T/002

**Обязательство Intake при положительном приёме:** одной SQL-транзакцией сохранить whitelist normalized envelope, receipt identity, исходный first-received timestamp, quota counters и outbox. 2xx разрешён только после установленного durable commit. Неизвестный commit не считается успехом; повтор supplier delivery разрешается через тот же receipt key. Duplicate event ID не занимает новый слот и не сдвигает возраст. Отличающиеся authenticated events одной Session остаются наблюдениями одной business identity у Processing. **Критерий:** crash между commit/ACK сохраняет сообщение, retry не создаёт дополнительного case/effect; FR101/104,NFR001/004.

### HP-BP-002/T/003

**Outbox-протокол Intake↔Processing:** постоянный message ID и content digest сохраняются до доставки; повтор HTTP возможен после timeout/lost ACK. Consumer обязан atomically фиксировать inbox decision и соответствующий результат, затем ACK. Несовпадающий content одного ID отклоняется как integrity conflict, не дополняется AI. Transport ACK означает приём observation, не handoff. Unresolved/held input имеет owner workspace, причину и связь с исходным receipt; no case не означает no received work. **Критерий:** same-message storm даёт один consumer transition и queryable outcome; RULE101,FR101/102,NFR008.

### HP-BP-002/T/004

**Source-facts read для pinned account/context/Session:** Intake получает доступ только через scoped internal operation; читает Session, необходимое полное line-item множество и связанный PaymentIntent/Charge, различает missing/unknown/false/zero. Результат включает фактический момент, object/version provenance и completeness. Processing применяет финансовый predicate HP-RULE-006; Intake не объявляет его выполненным из одного webhook paid. Для обязательной проверки используются полная amount/currency/capture/refund/dispute семантика: `Charge.refunded=false` не исключает partial refund, `Charge.paid=true` не доказывает capture. Прерванная pagination/недоступный mandatory field → incomplete, не разрешающий default. **Критерий:** partial refund/capture, wrong account/currency/qty/Price и unreadable field удерживают действие у consumer; FR103,NFR013.

### HP-BP-002/T/005

**Held/unresolved lifecycle:** normalized envelope содержит только класс HP-DOC-NFR-001/METHOD/DATA. Raw body штатно не сохраняется, контакты/бриф/arbitrary metadata не копируются в held storage. Счётчики количества/serialized size и expiry проверяются в той же TX, что admission receipt. При паузе действуют канонические пределы NFR004; повтор не освежает TTL. Pre-warning/known overflow/expiration сохраняются с interval/counter/owner и видимым ограничением. Already admitted case не вытесняется. **Критерий:** обе size boundaries, slot1001 и TTL±1s не вызывают ложный2xx или скрытую потерю; RULE003/101,NFR004/009.

### HP-BP-002/T/006

**Connection lifecycle:** после установки secret Intake доказывает внешний Stripe account/mode и передаёт identity claim для atomic binding registry HP-BP-001. Конфликт между workspace даёт безопасный отказ. Credentials шифруются отдельно от business data, выдаются только Stripe adapter, не URL/log/AI/export. Reconnect другого account/mode не восстанавливает старое подключение. Local revoke обязан пройти actual Intake dispatch/read barrier HP-BP-001, не только обновить cached health. **Критерий:** secret marker не раскрыт, same-identity reconnect сохраняет case scope, positive revoke запрещает новые provider requests; FR003/307,RULE011,NFR005/006.

### HP-BP-002/T/007

**Health и finite reads:** внешние чтения используют common one-shot HTTP/1.1 policy HP-BP-007: без keep-alive reuse, скрытого retry и redirect. Каждый physical request start проходит gate; безопасный GET не исключение из revoke. безопасные проверки необходимых чтений выполняются по NFR013; observed denial немедленно закрывает новые затронутые dispatch permissions и виден в установленный срок. Нет вывода write permission из одного GET или старого success. Read retries/429 waits конечны, учитывают provider limits и исходный episode deadline; при недостаточном остатке бюджета возвращают объяснённое препятствие, а не busy loop. **Критерий:** внешняя недоступность не превращается в отсутствие платежа/новое создание; NFR003/013 и FR108.

### HP-BP-002/T/008

**Retention/deletion/restore:** raw logging запрещён; отдельный диагностический raw режим подчиняется NFR009 и не является Worker default. Envelope/content/minimal receipt имеют отдельные storage classes и expiry. Workspace deletion немедленно закрывает route/secret/dispatch через общий deletion protocol, затем purges все управляемые копии в срок. Восстановленный inbox не обходит deletion/revoke overlay. Старый webhook не создаёт новую область; restoration с неустановленной актуальностью binding held. **Критерий:** old signed delivery после full deletion не resurrect workspace и не даёт action; NFR009/014.

### HP-BP-002/T/009

**Future compatibility boundary:** схема поддерживает stable source identity/completeness/cursor references для будущего TEAM scan, но worker HP-VER-001 не запускает его автоматически. Включение FR111/112 требует собственной выбранной Product/Architecture revision. Будущая сверка обязана использовать тот же admission/dedup path; full/partial различаются. **Критерий:** MVP permissions/config не включают scan по наличию поля; VER001 scope сохраняется.

### HP-BP-002/T/010

**Версии/ошибки:** internal schema version и Stripe API version независимы. Неподдерживаемый internal envelope quarantined с durable owner-visible incident; unknown provider enum/mandatory field не coercion к допустимому значению. Миграция inbox не удаляет unacked receipts и не меняет их ID/first_received/expiry. Producer/consumer compatibility проверяется до rollout; новая source revision не перепривязывает pinned case. **Критерий:** replay старой совместимой записи безопасен; несовместимая не вызывает silent drop/action; NFR001/008/014.

## 4. Взаимодействия, состояния и исходы

| Исход | Нормативный владелец |
| --- | --- |
| Rejected unauthenticated/oversize/unsupported input | T001/T002; ничего не выдано за business acceptance |
| Durable receipt, processing ACK lost | T002/T003; повторяется тот же observation |
| Неустановленный маршрут/объект | T003/T005; RULE101, owner, visible reason |
| Paused/expired/overflow | T005; нет нового admission или скрытого восстановления всех оплат |
| Complete/incomplete source facts | T004/T007; решение разрешения у Processing |
| Revoke/deletion/restore uncertainty | T006/T008; deny/hold, не false success |

Identity/конкуренция/повторы определены T002/T003; таймаут/unknown — T004/T007; authority — AD и T003/T006; хранение — T005/T008. Эти ссылки являются навигацией, не второй копией норм.

## 5. Обеспечение качественных и продуктовых ограничений

| Источник | Технические нормы | Сквозной предел |
| --- | --- | --- |
| FR101/102/104,RULE001/101,NFR001/008 | T001–T003 | consumer admission и final case проверяются вместе с BP003 |
| FR103,RULE006,NFR003/013 | T004/T007 | source proof не гарантирует атомарность между провайдерами |
| FR307,NFR005/006 | T006 | barrier всех runtime; breach5s = incident/FAIL, не исключение |
| NFR004/009/014 | T005/T008 | active/backup/log/outbox copies, не только одна таблица |

## 6. Параметризация, версии и совместимость

| Адрес | Диапазон/контекст | Кто выбирает/фиксация | Сохраняемое |
| --- | --- | --- | --- |
| HP-BP-002/PARAM/READ_DEADLINE | 1–10s на отдельное provider reading, не позже оставшегося episode deadline | service owner в versioned deploy config, exact value в evidence | конечность/NFR003; уменьшение не выдаётся за NFR002 PASS |
| HP-BP-002/PARAM/HEALTH_INTERVAL | 1–15min при active operation | platform/service owner, versioned config | NFR013 без продления интервала |

Все остальные body/queue/TTL пределы принадлежат NFR, не свободной Worker tuning. Совместимость — T010; изменение authority/trust/receipt semantics требует совместного AD/BP/formal source change. Первое издание, ничего не supersede; source/page/API/build/Product version — разные измерения. Старые frozen contexts не переключаются новой публикацией.

## 7. Проверка соответствия и фактическое evidence

План: AT-IN-01/02/04,AT-PAY-01,AT-SEC-03/05,AT-DATA-01/02 из supporting adversarial plan. Нужны signature fixtures, реальный transaction fault/replay test, provider stubs и разрешённые integration accounts. SQL-pattern spike может подтвердить atomic persistence/concurrency, но не provider permissions или весь NFR002/005/014. Фактические результаты — только hash-bound evidence inventory после запуска. Независимый review запрашивается по итоговому точному source set; этот авторский текст не объявляет PASS.

## 8. Существенные вопросы и контроль перед приёмкой

Все технические решения области заданы. Внешние реальные account permissions/API compatibility/legal data context остаются live-validation gates Product Definition, не незакрытым выбором Worker. Финальный exact formal asset closure и независимый source review обязательны для пакета. Пока нет formal acceptance/admission — authority_ready=false/execution_authorized=false. Значимый schema/authority conflict → ESCALATE+hold затронутой области; прямое нарушение прав/подписи → DENY; AUTO только внутри опубликованного bounded parameter range и принятого контекста AD.
