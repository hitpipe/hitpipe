# HP-DOC-AD-001 — Architecture Design — HitPipe

**Рабочее состояние:** In Review / согласованный архитектурный кандидат. **Владелец интеграции:** делегированный архитектор в настоящем поручении. Архитектурные решения приняты в пределах явно переданного права выбора; это не человеческая приёмка фазы, не admission разработки и не подтверждение существующей реализации.

## 1. Применимый контекст и архитектурный scope

Проектируется HitPipe Managed Business Operations: от явно принятого намерения до проверенного результата либо сопровождаемого исключения. Основание — точный кандидат Product Contract `sha256:e2a5ab63b35ce807c4de7bd79b30858fa0e54161eae252d022882fc8ceb7cd71`, выбранный HP-VER-001; Discovery импортирован только транзитивно через Product. Manifest фиксирует исходные revisions, целиком воспроизводимый source set и formal assets. Смешение latest Product с другим Architecture запрещено.

Нормативный scope: все 42 выбранных FR, их 96 AC, 14 NFR, 15 RULE, 6 gates, 10 strategic dependencies и 5 POL — 188 обязательств замыкания. Полный scope будущих TEAM/следующих версий остаётся в Product; поля расширения не активируют его. Здесь задаётся архитектура MVP SINGLE, с проверяемым маршрутом роста без объявления будущих функций реализованными.

Первая операция: поддерживаемая разовая положительная оплата Stripe Checkout/Payment Link → задача в выбранном ClickUp List с обязательным assignee/контекстом и фактическим readback. Точный pack, восемь состояний и условия исключения принадлежат Shared/CAP. Программируемый arbitrary workflow, платёжные действия, P2P/MCP, любые внешние адреса, самостоятельное принятие AI и непрерывная универсальная сверка не входят в MVP.

Драйверы: невозможность гарантировать exactly-once без внешнего контракта; опасность неизвестного исхода; прежняя конфигурация дела; осмысленное AI-native начало без LLM в цикле исполнения; права/отзыв за 5s; честная очередь/retention/recovery; пять параллельных направлений разработки. Код legacy — контекст, не новая норма.

## 2. Системный контекст и распределение ответственности

Пользователь работает с Next.js Web/BFF в одном origin. IAM устанавливает личность через Keycloak и проверяет актуальные права. Processing единолично владеет принятой конфигурацией и делом. Intake принимает подписанный Stripe webhook и получает финансовые факты; Delivery единолично начинает ClickUp-вызовы и сохраняет след возможного эффекта. Python AI участвует только в явной подготовке предложения; потеря LLM не останавливает принятые операции. Между владельцами — долговечные сообщения через HTTP с outbox/inbox, не общая SQL-модель.

```text
Пользователь → Edge → Next Web/BFF → IAM ↔ Keycloak
                          ├──────→ Processing ───command──→ Delivery → ClickUp
                          │             ↑                       │
                          │        normalized                observation
                          │             │                       ↓
Stripe ──signed webhook──→ Intake ←─source read──────── Processing
                          └─ Stripe API
Next BFF ──explicit minimized request──→ Python AI ──allowlisted model provider
Каждый Go-владелец → только собственная БД PostgreSQL; AI не имеет бизнес-БД.
IAM → actual-start barriers всех зарегистрированных runtime; revoke не eventual cache.
```

| Внешний участник | Граница и предпосылка |
| --- | --- |
| Owner/workspace | Принимает конкретную конфигурацию/ответственность; session не даёт права другого workspace |
| Stripe | Подпись доказывает источник события, не своевременность/полноту оплаты; факты дочитываются по pinned account/mode |
| ClickUp | Разрешённое создание и GET; отсутствие опубликованной idempotency гарантии означает conservative unknown |
| Keycloak | Аутентификация/password/MFA/recovery; бизнес-роли принадлежат IAM, не ID token |
| Model provider | Получает только явно отправленный ограниченный текст/aliases/synthetic samples; не реальные дела/credentials |
| Обслуживающий оператор | Только явный ограниченный support grant с действием/сроком/audit; нет скрытого staff bypass |

### Архитектурные части

| ID | Ответственность и authority | Границы/обеспечение |
| --- | --- | --- |
| HP-PART-001 · Go IAM/Profile | DATA001/007: profile, workspace, membership, opaque sessions, binding reservation, epochs, runtime registry, revoke/deletion coordination | IF002/010; BP001; не владеет case/payment/task truth |
| HP-PART-002 · Go Intake | DATA002/005: Stripe signed ingress, durable receipt/held input, verified account, source read adapter | IF003/004/005/011; BP002; не решает final case state |
| HP-PART-003 · Go Processing | DATA003/006: draft/readiness/rehearsal, immutable configs, operation lifecycle, unique case, episode, history, responsibility, notifications/export | IF004/005/006/007/008; BP003; единственный business-state writer |
| HP-PART-004 · Go Delivery | DATA004/005: ClickUp secret, actual HTTP start, durable effect lineage, attempts/readback/quota | IF006/007/011; BP004; не создаёт вторую case state machine |
| HP-PART-005 · Python AI Interaction | Без долговечного business state: strict proposal из минимального разрешённого входа | IF009; BP005; нет production connectors, raw intake, activation или tools |
| HP-PART-006 · Next.js UX/BFF | Same-origin user journeys, session proxy, CSRF, accessible presentation, bounded AI request | IF001/002/008/009/010; BP005; business authority остаётся Go |
| HP-PART-007 · PostgreSQL | Один cluster допустим, отдельные IAM/Intake/Processing/Delivery/Keycloak databases+roles; миграции принадлежат владельцу | IF012; BP006/007; никаких cross-service SQL reads, shared tables/FK |
| HP-PART-008 · Keycloak | Самостоятельная IdP boundary с отдельной БД/issuer; приложение не получает realm-admin | BP001; сбой не превращает старый JWT в разрешение |
| HP-PART-009 · Edge/TLS | Разрешённые маршруты, TLS, request-size/deadline, internal/private boundaries | BP006/007; никогда не повторяет provider writes и не заменяет authorization |

Это части системы, а не Product Capabilities/Jira Epics. Пять workstreams: IAM+platform; Intake+Stripe; Processing/core; Delivery+ClickUp; UX/BFF+Python AI. Это слоты ответственности без вымышленных персональных назначений. Один integrator владеет совместимостью и замороженной общей формой; service owners владеют своими миграциями/адаптерами. Processing требует обязательного cross-review Intake/Delivery.

## 3. Адресуемые архитектурные обязательства

### HP-ARC-001

**Роль/применимость:** интегральная single-writer authority всех MVP-команд. **Обязательство:** каждой области PART001–004 соответствует ровно один владелец записи; другой сервис обращается только по опубликованному IF. Изменение business state не выводится из transport ACK, health или AI. Межсервисный SQL и второй case writer запрещены. **Критерий:** inventory SQL roles/commands и негативные tests не дают межсервисного чтения/записи; BP001–004/007. Основания RULE001/004/005/008, NFR005/008; rationale ADR001.

### HP-ARC-002

**Обязательство распределённого приёма:** state+outbox владельца и inbox+consumer-state коммитятся атомарно в своих БД; успешный HTTP ACK только после установленного commit. Повторяетcя тот же ID/hash, не бизнес-эффект; конфликт hash блокируется и наблюдаем. Case uniqueness и effect lineage независимы от транспорта. **Критерий:** crash/lost-ACK/race один consumer transition и отсутствие ложного принятия; BP007/T/001–003, BP002/003/004. Основания FR101/104, RULE001/008/101,NFR001/004/008; ADR002.

### HP-ARC-003

**Обязательство identity/config:** Processing фиксирует case identity и accepted config при первом «Получено» до финансовой проверки. Пауза, повтор, новая конфигурация, новое episode и ручное закрытие не создают новую order identity и не стирают safety ledger. **Критерий:** совместная гонка admission/pause/config-change/replay сериализуется, прежнее дело сохраняет исходный pin; BP003, DATA003/004. Основания RULE001/003/004/013, FR102/104/306.

### HP-ARC-004

**Обязательство возможного внешнего эффекта:** Processing/Delivery сохраняют одну lineage на case+effect-kind. После durable DISPATCHING новый create запрещён, пока не получено достаточное доказательство фактического pre-effect отсутствия. Таймаут, crash, 5xx, пустой поиск, удаление task и новый episode сами такого доказательства не дают. **Критерий:** fault matrix во всех местах до/после send/response/commit не даёт второго возможного create; BP004 — единственный владелец точного dispatch/readback поведения. RULE008/013, FR104/208,NFR001/003; ADR002.

### HP-ARC-005

**Обязательство знания результата:** только Processing переводит дело в Confirmed по достаточному полному readback конкретного destination и принятого содержания. Факт POST/task ID не является успехом. Incomplete/unknown/mismatch видимы отдельно; уже подтверждённый исторический факт не переписывается поздним наблюдением. **Критерий:** все required matches и completeness проверены; BP003/004, RULE002/007/009, FR105/106/206.

### HP-ARC-006

**Обязательство целостной текущей authority:** каждый защищённый read/response-start и каждый connector request-start проверяет актуальную scope/epoch и actual-start gate; IAM координирует отзыв всех затронутых runtime. Positive revoke только после целостного barrier/fence, а не истечения JWT/lease. Превышение 5s — FAIL/incident, не исключение. **Критерий:** конкурентный revoke/paused process/new runtime не даёт нового start после positive confirmation; BP001/007. Основания NFR005/006,RULE005/010/011; ADR003/004.

### HP-ARC-007

**Обязательство AI изоляции:** Python model path имеет только разрешённый minimized request и untrusted proposal, без live business authority/credentials/DB; минимальные внутренние authorization IDs служат только IAM/control и никогда не пересылаются model provider. Go повторно валидирует и человек явно принимает фактическую конфигурацию до активации. LLM не участвует в ordinary execution/retry/recovery arbitration. **Критерий:** PII/secret-canary и prompt-injection tests включают payload/headers/metadata/logs; отключение model provider не останавливает принятую операцию; BP005. POL004,NFR007/010/012,FR001/002/005/006/008; ADR006.

### HP-ARC-008

**Обязательство срока/ёмкости:** ingress, scheduler и workers совместно соблюдают Product queue/case/data limits и исходные timestamps; внутреннее ожидание не выводится из end-to-end знаменателя. Quota на реальный token общая для всех его bindings. Reboot не освежает TTL/episode/deadline. **Критерий:** совместная нагрузка выбранной ступени и независимые превышения каждого предела; BP002–004/006/007, NFR002/003/004/013.

### HP-ARC-009

**Обязательство жизненного цикла данных:** expiring raw/envelope/content/history и lifetime minimal safety физически разделены. Дубликат/чтение не продлевает content retention и не удаляет возможный эффект. Deletion overlay применяется до восстановления ingress/egress; внешние Stripe/ClickUp объекты не удаляются HitPipe purge. **Критерий:** controlled-clock expiry, surface inventory и restore-after-delete test; BP006 и DATA002–007. NFR009/014,RULE012/013.

### HP-ARC-010

**Обязательство безопасного восстановления:** до допуска restored runtime закрыты ingress/action gates; schema/version, deletion/revocation overlay и актуальность safety ledger проверены. Потерянный/сомнительный state даёт quarantine/Unknown, а не replay create. **Критерий:** recovery tests наблюдают destination, acknowledged receipts и реальные gates; BP006. RPO0 относится только к явно заданному controlled fault contour; сайт, все копии и внешние системы не становятся бесконечной гарантией. NFR001/014; ADR008.

### HP-ARC-011

**Обязательство observability:** любой case имеет наблюдаемое состояние, ответственного, причину/следующее действие и bounded history; технические queue/dispatch/health факты не маскируются как business success. Metrics/traces используют низкокардинальные безопасные labels; реальные credentials/context не оказываются в них. **Критерий:** реконструкция полученного/unknown/gap/closed case по фактам и negative leak tests; BP003/005/006,NFR008/013.

### HP-ARC-012

**Обязательство версии/изменения:** wire, DB migration, accepted business config, provider API pin и документный source revision — разные версии. Совместимый rollout поддерживает действующие pins/outbox messages; breaking change не включается частичным deploy. **Критерий:** producer/consumer previous-current fixture matrix и staged migration/rollback; BP007, ADR005/009. RULE004,FR302–306,NFR001/014.

### HP-ARC-013

**Обязательство UX:** Next показывает понятие дела/операции и подтверждённое знание, не только список technical runs. Защищённые данные не попадают в shared cache/prerender/client secrets. Клавиатура/reader/320px/200%/contrast проверяются на целых путях, с безопасным explicit acceptance и structured fallback при AI outage. **Критерий:** BP005 matrix и NFR011/012; один browser smoke не доказывает весь путь. ADR007.

### HP-ARC-014

**Обязательство комплекта:** все нормы выбранного upstream имеют действующий технический владелец и проверяемый method. Derived JSON Schema — проекция canonical OAS, не альтернативный wire authority. Незакрытый существенный normative/formal конфликт удерживает затронутый scope, не скрывается completeness=true. **Критерий:** 188-row coverage, full source/formal closure, independent reverse review и unchanged PCF validation; BP007/manifest. ADR009.

### HP-ARC-015

**Обязательство live admission:** до live конкретные account/mode/List/assignee/write-readback rights, privacy/provider recipient и system conformance подтверждаются реальными разрешёнными тестами. Документ, успешная сборка, synthetic rehearsal и архитектурная готовность не являются таким evidence. **Критерий:** Product gates+BP006 qualification checklist; отрицательный gate остаётся блокирующим именно live use, не отменяет содержательный выбор архитектуры. POL001/003/005,NFR010/013, Product gates.

### HP-ARC-016

**Обязательство independent work:** разработчики могут отдельно реализовать сервисы по точным IF/schema/mocks после фиксации общей формы, однако не объявляют общие safety/security/resources независимыми. Мердж затрагивающего shared invariant требует cross-review и совместного conformance test. **Критерий:** integration sequence и owner matrix BP006/007; общий completeness scope полный, не пять ложных partial grants. Основания все глобальные POL/NFR, VER001.

## 4. Данные, состояние и инварианты

| ID | Authority/lifecycle | Гарантии | Точный источник |
| --- | --- | --- | --- |
| HP-DATA-001 | IAM: user/subject/profile/workspace/membership/session/epochs; revoke before access loss confirmation | ARC001/006; no secret in session view; last-owner continuity | BP001; formal/sql/iam.sql |
| HP-DATA-002 | Intake: durable receipts/normalized held input/source observations; first-received TTL bounded | ARC002/008/009; no raw body in ordinary persistence | BP002; formal/sql/intake.sql |
| HP-DATA-003 | Processing: immutable accepted config, operation lifecycle, unique case/minimal identity, durable episodes/timers | ARC003/005/008; no rebinding old case | BP003; formal/sql/processing.sql |
| HP-DATA-004 | Delivery: unique effect lineage, immutable attempt facts, observations/quota counters | ARC004/005/010; possible-effect fence survives age/restart | BP004; formal/sql/delivery.sql |
| HP-DATA-005 | Connector-owned encrypted secret FILE volume; SQL только opaque ref+keyID+epoch; IAM has opaque grant/binding, не secret | ARC006/009; access only adapter's process/role; secret deletion does not erase case facts | BP001/002/004/006 |
| HP-DATA-006 | Processing content/history/config free text, each independently expires by addition/event time | ARC009/011; contact/brief not permanent minimal identity | BP003/006; corresponding content/history SQL |
| HP-DATA-007 | IAM deletion/revocation overlay + platform backup inventory with separate recovery protection | ARC006/009/010; restore applies latest overlay before serving; old backups have finite outer expiry | BP001/006; formal/sql/iam.sql and recovery protocol |

Wire fields and SQL columns/types/indices are deliberately not copied here. Content/raw/source-fact/readback/free-history payloads are not stored in PostgreSQL or WAL: service-owned fsync durable content volumes hold them with exact expiry; SQL has only opaque references/digests/minimal metadata. Logical backups contain only approved minimal safety/config/deletion records; no physical full-cluster PITR, content snapshots or historical content backups in MVP. BP006 defines write-before-reference/expiry/orphan/restore behavior. This is an explicit retention and recovery choice, not a claim of disk-forensic erasure. Within each service `(workspace_id,id)` constraints prevent cross-workspace relations; API role nonowner/NOBYPASSRLS; worker global job locator contains only minimal opaque IDs then opens scoped TX. Secret-store keys are outside DB/backups. Actual SQL role/transaction tests must verify claims; architecture description alone is not proof.

## 5. Взаимодействия и интерфейсы

| ID | Стороны/назначение | Canonical source |
| --- | --- | --- |
| HP-IF-001 | Browser↔NextBFF; same-origin authorized UX commands/reads | OAS `/api/v1/*`; BP005/007, IAM cookie/CSRF BP001 |
| HP-IF-002 | BFF/Go→IAM + IAM↔Keycloak OIDC | OAS `/auth/*`, `/internal/v1/authorize`; BP001 |
| HP-IF-003 | Stripe→Intake signed raw webhook | OAS `/webhooks/stripe/{endpoint_id}`; BP002 signature/ACK semantics |
| HP-IF-004 | Intake→Processing durable normalized signal | OAS `receiveNormalizedSignal`; BP002/003/007 |
| HP-IF-005 | Processing→Intake bounded source-facts read | OAS `readSourceFacts`; BP002/003 |
| HP-IF-006 | Processing→Delivery durable intent/explicit query | OAS `acceptDeliveryIntent`,`queryKnownTask`; BP004 |
| HP-IF-007 | Delivery→Processing durable task observation | OAS `applyTaskObservation`; BP003/004/007 |
| HP-IF-008 | BFF→Processing user domain API | OAS owner PROCESSING operations; BP003/005 |
| HP-IF-009 | BFF→PythonAI explicit proposal | OAS `generateAIProposal`; BP005 |
| HP-IF-010 | IAM↔registered runtime actual-start barrier | OAS `registerRuntime`,`applyRevocationBarrier`; BP001/007 |
| HP-IF-011 | Intake→Stripe / Delivery→ClickUp | fixed providers/versions/fields and no retry in BP002/004/007 |
| HP-IF-012 | Each owner→own DB only | canonical SQL; BP006/007; pgx transaction behavior |
| HP-IF-013 | Platform→health/metrics, nonpublic | OAS health operations; BP006; not product outcome proof |

Основной SCN: durable webhook→normalized outbox/inbox→case+config pin→complete recent source facts→allowed accepted context→one durable effect fence→ClickUp create→specific GET/full match→Processing Confirmed. Missing context→WaitingData; unknown send→Unknown; known mismatch→Attention. Эпизод/TTL/пауза не создают разрешение на новое unsafe действие. Конкретные ветви/доказательства — BP002–004 и Shared, не стрелки диаграммы.

## 6. Обеспечение продуктовых обязательств

Полный адресуемый mapping каждого из 188 obligations содержится в `formal/verification/product-coverage.json` и Architecture Contract. Это projection отношений, не новая копия продуктовых требований. Зависимости поля imported object используют Product namespace, POL — `product/discovery`; покрытие будущих невыбранных FR не выдаётся за MVP.

| Область источников | Обеспечение | Проверка/граница |
| --- | --- | --- |
| FR001–010 / CAP001, POL004 | PART001/003/005/006; ARC006/007/013; BP001/003/005 | intent→clarification→rehearsal→exact acceptance→activation; actual usability gate отдельно |
| FR101–110 / CAP002, RULE001/002/006/008/101 | PART002/003/004; ARC002–005/008; BP002–004 | source identity→verified result plus duplicate/late/unknown/mismatch/refusal |
| FR201–212 / CAP003, RULE005/007/009/010/012/013 | PART001/003/004/006; ARC003–006/009/011; BP001/003–005 | owner/action/time, new data, safe recheck/binding/closure, export/history |
| FR301–310 / CAP004,RULE003/004/011/014 | PART001–004/006; ARC003/006/008/012; BP001–005/007 | exact config diff/rehearsal/new acceptance, pause/drain/resume/reconnect/owner continuity/archive |
| NFR001–004/008/013/014 | ARC002–005/008/010/011; BP002–004/006/007 | joint data/race/load/unknown/recovery tests, не сумма local PASS |
| NFR005–007/009/010 | ARC006/007/009/015; BP001–007 | negative rights/revoke/PII/retention/rehearsal containment |
| NFR011/012, strategic gates and empirical conditions | ARC013/015; BP005/006 | complete UX+actual future user/cohort/live data checks; не выдуманное evidence |

## 7. Безопасность, надёжность и эксплуатация

Trust boundaries: public webhook (HMAC raw bytes), browser (session+CSRF), private interservice mTLS+service allowlist, own DB roles, IdP and external account, minimized model egress. Caller service identity не заменяет user/workspace scope. Управляемый PILOT onboarding с owner-confirmation; secrets вводятся в отдельном credential UI, не AI. Конкретная PKI/session/CSP/secret handling — BP001/005/006/007.

Failure classes: duplicate/reorder/lost ACK; process kill at every commit/send boundary; partial partitions/runtime suspension; database restart/primary loss; revoked provider; rate limit; corrupt/missing/old backup; content expiry/deletion; clock uncertainty; LLM unavailability/malicious proposal. Только подтверждённые внешние причины допускают Product METHOD/HEALTH исключения; внутренняя очередь/unknown/error остаётся в метрике. Arbitrary simultaneous loss of all durable copies is outside declared RPO0 contour and handled as disaster+hold, not concealed success.

Deployment: separate development/staging/live environments and identities; OCI containers, Compose/systemd on dedicated hosts initially, no Kubernetes prerequisite. Current ProjectOS VPS is not the HitPipe environment. Development minimum8GiB RAM/4vCPU/80GiB SSD; live topology/capacity/routing/fencing/storage exact profiles owned BP006. Это проектные начальные границы, не измеренное capacity evidence и не заказ инфраструктуры. Подготовка базы данных/ключей/backup/fencing must precede enabling ingress/external effects. Stage and approved sandbox tests precede live; five service teams can use synthetic adapters first.

## 8. ADR, Blueprints и formal assets

| Source | Владелец определения |
| --- | --- |
| HP-ADR-001–004 | decomposition, durable transport, identity, revocation rationale; consequences reference ARC/BP |
| HP-ADR-005–007 | exact toolchain/API codegen, minimized AI, accessible selfhosted UX choices |
| HP-ADR-008 | data/recovery/controlled fault contour |
| HP-ADR-009 | formal source ownership, versioning, migration and five-workstream compatibility |
| HP-BP-001–007 | respective IAM/Intake/Processing/Delivery/UXAI/platform/common protocol precise T clauses |
| HP-FORMAL-001 | `formal/openapi/hitpipe.v1.json`: exact full HTTP shapes/paths/operations/auth/status; semantics BP refs |
| HP-ASSET-PAYLOADS-001 | `formal/schemas/payloads.schema.json`: generated JSONschema2020-12 payload view, not wire owner |
| HP-FORMAL-SQL-001 | `formal/sql/*.sql`: exact owner-local persistence structures and constraints |
| HP-FORMAL-STACK-001 | `formal/stack/`: exact dependency locks/images/type bridge and compatibility manifests |
| HP-FORMAL-VERIFY-001 | `formal/verification/`: test vectors/coverage/verification inventory; planned tests clearly distinct from actual evidence |

Git source prefix `contracts/hitpipe/architecture-2026-10-16/`; exact actual commit/blob digests and logical versions live in manifest after real commit. Formal local `$id` URI is identifier, not claim of deployed registry. Unchanged ProjectOS PCF currently validates JSONschema2020-12 only. OAS/SQL are complete ordinary native sources with independent validators; the package does not falsely claim unsupported OAS formal validation. Catalogs are navigation, not requirements owners.

## 9. Делегирование локальных решений

| Policy | Разрешение/границы | Кто/запись и proof |
| --- | --- | --- |
| HP-DELEG-001 · AUTO | Local function names/internal package factoring/index addition with same locking/tenant behavior; finite connection-pool tuning1–10 per Go service in PILOT, total budget BP006; CSS component extraction without UX/safety change | service owner; PR with invariant tests, locked versions and query plan/resources as applicable |
| HP-DELEG-002 · ESCALATE | New service/database authority, new provider/path/wire field meaning/state, retry/no-effect rule, scope/limits/retention change, capacity/recovery/fencing contour or breaking dependencies | integrator resolves AD/ADR/BP/formal change and affected tests together, no user micro-approval needed within delegated scope |
| HP-DELEG-003 · DENY | Unsupported live write, blind retry after possible effect, hidden grant/shared tenant cache, raw/secret to AI/log, fabricated human acceptance, unverified source pin | worker refuses unsafe action, preserves facts and records incident; cannot locally waive normative invariant |

Allowed tuning cannot weaken upstream. Parameter exact value/context/author/change reason and measured conformance recorded in deploy manifest. Absence of a prohibition is not a new API grant. Broad architectural discretion of this author does not create an unbounded runtime AI authority.

## 10. Пробелы, полнота и допуск

Адресуемые projection roots: HP-CTX-ARCH-001 — контекст §1–2; HP-SCN-ARCH-001 — последовательность §5; HP-DEPLOY-ARCH-001/HP-SEC-ARCH-001/HP-REC-ARCH-001 — модели §7 и точные нормы BP001/006/007; HP-READY-ARCH-001 — готовность настоящего §10. Это стабильные локаторы существующего содержания, не новые гарантии.

Существенные технические варианты выбранного MVP определены. Реализация, нагрузка, complete platform fault tests, live provider rights, usability and operational observation windows ещё не выполнены: это конкретные verification/admission gates, не пробелы нормативного поведения. Compatibility spikes подтверждают только собственную указанную область.

Техническая спецификационная готовность — весь HP-VER-001, все 42 FR; частичных независимых grants нет, общие security/data/IF dependencies явно сохранены. Формальная upstream/source acceptance не получена. `authority_ready=false`, `execution_authorized=false`, grant=none. Комплект может быть входом к поддерживаемой последующей разработке после законной процедуры admission; самостоятельные chat/PR/page статусы её не подделывают.

## 11. Изменения и контроль готовности

Первое целостное System Design издание новой Operations-концепции; существующие Discovery/Product IDs не заменены. Новый revision не меняет уже frozen context. Любое review-исправление адресует конкретный defect и обновляет все affected dependents; итоговый exact source set повторно читается, валидируется и independently reviewed.

Финальный package содержит document inventory/revisions, source closure, formal Git pins, independent source/projection reviews, actual compatibility logs и неизменяемый contract candidate. Только реальный human-only Lead gate и поддерживаемые доверенные adapters могут дать формальную ACCEPTED/Development authority; это отдельный честно обозначенный внешний шаг.
