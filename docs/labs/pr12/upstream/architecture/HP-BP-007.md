# HP-BP-007 — Common HTTP, durable messaging и совместимость

**Рабочее состояние:** In Review / Candidate. **Владелец:** интегратор System Design. Локальная спецификация всех IF, не дополнительный владелец продуктовых правил. Архитектурная норма отличается от фактического implementation evidence и human acceptance.

## 1. Граница и применимость

Применяется к четырём Go-владельцам, Next BFF, Python AI и их ingress/egress/storage boundaries для HP-VER-001. PART/IF/DATA принадлежат AD; этот BP владеет общим точным transport/security-start/idempotency/versioning поведением. Provider-specific financial/readback смыслы принадлежат BP002/004, IAM authority BP001. Внешние Stripe/ClickUp не становятся участниками внутренней атомарной транзакции.

## 2. Канонические источники

`HP-FORMAL-001`, `formal/openapi/hitpipe.v1.json` — canonical OpenAPI3.1.0, каждая операция имеет ID, owner, interface и behavior reference. Это полный назначенный источник HTTP методов/путей/shape/auth/status; generated Go/TS types и JSONschema view не редактируются отдельно. `formal/build_contracts.py` — воспроизводимый авторинг wire source и генератор projection; все полученные bytes закреплены exact Git commit/digest. `formal/schemas/payloads.schema.json` — JSONschema2020-12, generated view только component payload semantics; не утверждение что PCF проверяет HTTP/OAS поведение. SQL владельцев — canonical storage definitions. `formal/verification/*` — trace/vectors/planned checks, не подмена нормы.

Стабильный logical prefix Git: `contracts/hitpipe/architecture-2026-10-16/`. Фактические source locations и imports в manifest; отсутствующий pin удерживает финальную упаковку, не заполняется примером SHA. Payload $id — logical identity, не provisioned network service.

## 3. Адресуемые технические обязательства

### HP-BP-007/T/001

**Стороны/условие:** producer/consumer любого durable internal observation/command. **Действие:** producer одной TX сохраняет свой state и outbox metadata с persistent UUID message_id, schema_version, opaque serialized-payload FILE reference/expiry и content digest по BP006; сам payload не помещается в SQL/WAL. Consumer одной TX сохраняет inbox unique `(producer,message_id)`, сравнивает body digest и применяет допустимое state изменение; ACK только после установленного commit. Повтор того же ID/hash возвращает установленный receipt, не новый transition. Иное тело того же ID →409 integrity incident, не merge. **Критерий:** убийство процесса до/после каждого commit/ACK не теряет принятое обязательство и не создаёт второй consumer business effect; ARC002,NFR001/008.

### HP-BP-007/T/002

**Identity/digest (durable messages Go→Go; PythonAI/Next не их producer/consumer):** UUIDv4 CSPRNG, timestamps RFC3339 UTC с достаточной миллисекундной точностью; provider IDs opaque strings, не числа JavaScript. Wire money/ревизии ≤2^53−1; больший ID остаётся string. Canonicalizer Go — `github.com/gowebpki/jcs v1.0.2`, pin+общие RFC8785 vectors в stack/formal evidence; обычный encoding/json sorted keys не подмена JCS. Digest=`sha256:` lowercase64hex от UTF8 RFC8785 JCS полного message payload БЕЗ поля message_digest; unknown keys/duplicate JSON object keys/nonfinite numbers запрещены. Точный сериализованный payload хранится для повторной передачи; повтор не генерирует новые timestamps/ID. Проверка incoming digest обязательна до inbox ACK. **Критерий:** Go durable boundary соответствует общим RFC8785 expected-byte vectors; Python/TS parsers/types сохраняют wire meaning, но не обязаны иметь второй JCS implementation в MVP, altered body конфликтует; JSON null/absent различаются, не coercion/default permission. Native raw Stripe bytes подписываются по Stripe HMAC, не JCS.

### HP-BP-007/T/003

**Доставка/очередь:** worker кратко claims ready rows `FOR UPDATE SKIP LOCKED`, commit, затем HTTP вне SQL lock. Claimlease30s/heartbeat10s — только liveness внутренних outbox/read jobs. Повтор at-least-once с capped exponential backoff1,2,4,8,16,30s + uniform jitter0–20%, respect Retry-After и исходного deadline; очередной lease/перезапуск не расширяет Product time budgets. Permanent schema/hash/authorization conflict quarantine с owner/reason, не drop. Доставленные outbox payloads удаляются в пределах их data class; lifetime minimal receipt сохраняет только digest/identity/outcome. **Критерий:** expired delivery-dispatch lease НИКОГДА не даёт права на второй provider POST; правила BP004 имеют приоритет над generic outbox retry.

### HP-BP-007/T/004

**User commands:** меняющий запрос несёт command request UUID, точный body и expected_revision для существующего mutable aggregate. Owner atomically сохраняет command receipt+result; тот же actor/scope/ID/hash повторно возвращает прежний установленный результат после fresh authorization, иной hash409. Stale revision/If-Match→412 без изменения; order/lineage не привязаны к request ID. BFF double-click/lost response не создают новый command. UI не помечает business success из202. **Критерий:** одновременные вкладки/потерянный ответ не теряют фактический outcome и не обходят revoke; ARC003/004/006.

### HP-BP-007/T/005

**Boundary validation/errors:** parser отвергает malformed JSON/duplicate keys/extra fields/unknown enum и неподдерживаемую версию; null/absent обрабатываются по OAS. Go strict generated types дополняются runtime semantic validation, Python Pydantic strict/extra=forbid; TS types не runtime validator. Коды ошибок machine-readable по OAS; сообщение не раскрывает чужой resource/secret. 400 invalid,401 no identity,403 denied scope,404 no authorized visible object,409 state/hash conflict,412 revision,413 size,429 refused capacity,503 not established durable acceptance. Provider status не транслируется как готовый business state. **Критерий:** negative fixtures всех boundaries дают объяснённый отказ без побочного разрешения.

### HP-BP-007/T/006

**Private service trust:** Go servers require TLS1.3 client certificate, validate private CA/expiry/exact DNS service SAN and method/path caller allowlist. Exact operation `x-hitpipe-callers` в OAS — canonical method/path allowlist; он действует в дополнение к service SAN и не заменяет user authority. Listener8443 business,9443 IAM-only barrier,9444 OPS-only health; private runtime register и authorize на IAM8443. Own service cert+private key отдельны от user/session и provider secret. Browser headers identity/role удаляются Edge/BFF. BFF использует Node native `https.request` с явными cert/key/ca и `rejectUnauthorized:true`; Go→Go native TLS, не plaintext Docker assumption. Python business Uvicorn SSLContext `CERT_REQUIRED` доверяет выделенному BFF-only client issuer. Второй private IAM-only control listener в том же процессе/shared state принимает только barrier; отдельный health listener не имеет business/control methods. Python outbound control допускается только к фиксированным IAM authorize/register. Полная topology/role семантика BP005/T/021; никакого ASGI assumption о доступности peer certificate. Произвольный другой service cert не получает AI endpoint. Internal service identity сама не даёт user/workspace authority: owner проверяет actual session/current grant в IAM и own epoch/start gate. **Критерий:** wrongcert/service/expiredcert/cross-scope denied, unsafe TLS environment overrides запрещены. Runtime roster generation BP001 обязателен до serving.

### HP-BP-007/T/007

**Actual-start/revoke сериализация:** защищённый start приобретает read section локального runtime gate, проверяет deny epoch/current authority и сохраняет уже необходимый durable effect fence. Для внешнего HTTP section удерживается до фактической записи request (`httptrace.WroteRequest`), либо доказанной отмены без send; запуск goroutine/получение permit/вход RoundTrip не считаются достаточным start proof. DNS/connect/TLS/initial request-write deadline2s; full response deadline10s может продолжаться после фактического start как уже начатый запрос. При error-after-write uncertainty передаётся BP004, не очищает fence. Node BFF удерживает section до события ClientRequest `finish` либо установленного pre-send cancel; создание request не release. Python консервативно удерживает зарегистрированный provider-task section до полного завершения запроса. Получив revoke control, он сначала устанавливает deny для новых tasks, отменяет относящиеся незавершённые tasks и ждёт подтверждённого join/закрытия transport до ACK, с cancellation budget2s; один факт cancel() не ACK и не отмена уже полученного provider эффекта. Невозможность join оставляет Pending/incident, не допускает новый start. Для protected SQL read section удерживается до bounded≤2s completion; перед началом выдачи защищённого ответа проверяется действующий response gate. Revoke writer section запрещает новый старый epoch и ACK только после завершения всех прежних start sections; прерванный/unresponsive процесс не даёт ACK. **Критерий:** paused-between-check-and-write, stale runtime/newgeneration, DB/response/connector starts после positive revoke=0; BP001 whole-roster barrier. Невозможность уложиться в5s — явный NFR005/006 FAIL/incident, не ложный успех. Уже начатый network call не объявляется отозванным внешним эффектом.

### HP-BP-007/T/008

**One-shot provider transport:** Intake/Delivery используют отдельный `http.Transport`, HTTP/1.1, DisableKeepAlives=true, HTTP2 disabled, без proxy/environment route вне deployment allowlist, no redirect (`ErrUseLastResponse`), no SDK retry и без GetBody replay. Для ClickUp POST запрещены `Idempotency-Key`/`X-Idempotency-Key`, включая пустые header entries: опубликованного provider контракта нет, а net/http может считать такой запрос replayable. На каждом GET/POST отдельный actual-start gate; safe GET повторяет только bounded higher-level read protocol, не скрытый transport. **Критерий:** timeout/reset/reused socket/307/proxy scenarios показывают ровно один physical request на attempt; BP004 final authority по допустимости следующей write attempt. Внутренние idempotent durable messages используют отдельный pool/client, не этот provider policy.

### HP-BP-007/T/009

**Tenant SQL/конкуренция:** у каждого Go owner отдельная БД; runtime API role не owner/superuser/BYPASSRLS и не выполняет DDL. Scoped TX устанавливает `SET LOCAL hp.workspace_id`, RLS+composite workspace keys сохраняют tenant relation. Worker global locator содержит только минимальные IDs/срок/jobkind, затем выполняет scoped TX; он не предоставляет user bypass. Мигратор отдельный privileged principal. Порядок locks в Processing workspace→operation→route→case→episode; сетевой вызов не держит row lock. Unique constraints — последняя сериализация identity/lineage, optimistic revision — пользовательский конфликт. **Критерий:** actual role/cross-tenant/duplicate race tests и запрет чужих DB grants; один unit handler test не доказательство multi-tenant безопасности.

### HP-BP-007/T/010

**Timeout/cancel/задачи:** durable absolute deadlines основаны на первоначальном DB/UTC времени; monotonic timer только будит worker. Internal durable/control/short business request total≤3s, connect≤1s; отдельный AI preparation IF009 использует total5–30s BP005, default20s, BFF/Edge whole-response budget40s и не подчиняется short3s; provider total≤10s/initial start≤2s; source snapshot весь набор может быть≤30s, но freshness всех mandatory facts≤10s до permitted dispatch. Старый snapshot перечитывается bounded, не получает новый возраст по последнему полю. Episode≤300s и pauseddrain≤300s не продлеваются retry/reboot. User cancel/HTTP disconnect отменяют только неначатое; committed command/outbox/fence остаются для установленного результата/unknown. **Критерий:** crash/time-jump/deadline tests не создают новый budget/permission; clock uncertainty failclosed для authorization/freshness и наблюдаема по NFR008.

### HP-BP-007/T/011

**Совместимость/rollout:** v1 current и непосредственно предыдущая совместимая minor schema работают в producer/consumer matrix; additive необязательное поле вводится сначала reader capability deployment, затем writer feature. Strict unknown rejection означает нельзя молча отправить новое поле старому consumer. Breaking required/enum/meaning change → новый explicit major path/schema, migration и совместная замена маршрутов; old queued messages должны получить поддерживаемый reader до удаления старой формы. DB expand→dual-read/backfill→verifiedcutover→contract; destructive down migration не автоматический rollback. Accepted configs/ledger identity не переписываются ради wire change. **Критерий:** exact codegen diff+previous/current fixtures+data migration crash/rollback tests; unknownschema quarantined, не drop.

### HP-BP-007/T/012

**Ownership/coverage:** изменение OAS→rebuild generated payload schema/types→diff+validate+consumer build→behavior review соответствующего BP. Весь formal/native reference closure закрепляется фактическими commit/digests; source_manifest не ссылается на mutablebranch вместоcommit. Contract interface operation использует real generated JSONschema pointer для request/response, при этом полные HTTP semantics сохранены в OAS и operation description. PCF не объявляется валидатором OAS/SQL; каждый формат получает свой реальный tool check. **Критерий:** reproducible source/projection bytes, semantic reverse review и отсутствие fabricated pins/acceptance; ARC012/014, ADR009.

## 4. Состояния и исходы

Durable protocol: Pending→Claimed→ConsumerCommitted→Acked, либо Quarantined/ExpiredObserved. Это технические состояния доставки, не девятое case state. DISPATCHING/AMBIGUOUS — BP004 ledger, не outbox retry queue. Known error/refusal, unknown commit, unknown external effect и confirmed observation различаются. T001–005 владеют ACK/hash/revision; T006–008 authority/start; T009 tenantserialization; T010 time/cancel; T011–012 version/provenance.

## 5. Продуктовые бюджеты и трассировка

T001–004/008 обеспечивают NFR001/RULE001/008/013; T006–009 NFR005/006; T010 NFR002/003/004/008; T003/009/011 NFR009/014. Техническое штатное allocation: durable internal transit≤5s, source reads≤30s, create≤10s, readback≤10s, state/UI≤5s. Allocation не подменяет joint95%60s/99%300s denominator. Internalqueue,unknown,error не исключаются из Product measurement. Итоговая нагрузка/полнота проверяется совместно всех workspace; local spike не доказывает capacity.

## 6. Параметры/версии

| ID | Допустимый выбор | Кто/запись/ограничение |
| --- | --- | --- |
| HP-BP-007/PARAM/POOL |1–10 DB connections/service PILOT, default5; общая сумма и резерв Keycloak/мигратора в BP006 | owner+platform deploy manifest; load/queue proof без weakeningNFR |
| HP-BP-007/PARAM/OUTBOX_WORKERS |1–4 per service PILOT, default2 | owner; versionedconfig; no concurrency override effectunique/gate/quota |
| HP-BP-007/PARAM/INTERNAL_IDLE | idleconnection timeout10–60s default30s internal pool only | owner; boundedresourcecheck; provider transport remains one-shot |

TTL/state/security/body limits и provider retry rules не локальная tuning. Первое издание без supersession; semantic changes require exact source-set revision, not latest substitution.

## 7. Проверка и evidence

План включает lostACK/duplicate storms, conflicting hash/unknownkeys, JSON cross-language vectors, race locks/tenantroles, actual wire-start revocation, provider redirects/retries, old/currentcodegen, migrations/restore and timeouts. Фактические совместимые Go/Python/Next fixtures и их logs/locks входят в stack evidence; полная реализация/gates/productionbenchmarks здесь не объявлены PASS. PCFstructural validation, OASvalidation и appconformance различны. Independent review по final exact source set выполняет другой автор, не генератор собственного отчёта.

## 8. Полнота и границы приёмки

Внутри MVP технические варианты выбраны, отложенных обязательных protocol решений нет. Actual implementation/fault/load/liveaccount qualification остаются проверками реализации, не свободой менять API/authority. Missing source/formalpin или semanticconflict удерживает affectedscope. Документ Candidate/InReview, не acceptance/admission. AUTO/ESCALATE/DENY AD действуют на каждую локальную правку.
