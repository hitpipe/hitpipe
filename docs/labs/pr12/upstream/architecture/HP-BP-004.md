# Technical Blueprint — Delivery и достоверное знание о task effect

**ID:** HP-BP-004. **Состояние:** In Review. **Ответственный:** делегированный архитектор HitPipe; интегратор — Codex root. Нормативный кандидат для review, не formal Accepted и не доказательство работающего connector.

## 1. Граница спецификации и применимость

Область — единственный ClickUp read/write adapter, connection-specific egress, create-effect lineage, durable attempt knowledge, bounded readback и manual binding. Применимость — HP-PACK-001/HP-VER-001, SINGLE/PILOT. Это не универсальный executor, financial compensation, автоматический PATCH/удаление задачи или намеренная replacement task.

| Сторона | Ответственность |
| --- | --- |
| Delivery | Единственный фактический ClickUp HTTP и ledger внешнего knowledge |
| Processing BP003 | Единственный case/config/episode/state owner; разрешение конкретного действия |
| Identity BP001 | Subject/connection epochs, revoke coordinator и actual start barriers |
| ClickUp | Внешний результат и readback в доступных пределах; не участник SQL-транзакции |

## 2. Канонические определения и формальные источники

Storage constraints принадлежат `formal/sql/delivery.sql`. Provider request/actual target attributes и ciphertext секретов живут только в service-owned durable content files HP-BP-006, не SQL/WAL/backup. SQL хранит opaque ref/hash/size/expiry и минимальную lineage/knowledge; содержательная копия не прячется в outbox JSON. Fsync+rename+directory fsync до SQL ref commit; missing proof/payload file→hold/incomplete, не retry possible effect.

HP-FORMAL-001 владеет внутренними HTTP command/result/observation схемами и operation selectors: `contracts/hitpipe/architecture-2026-10-16/formal/openapi/*.json`, OpenAPI3.1.0; exact Git commit/digest/refs — итоговый manifest. `formal/schemas/*.json` — generated views, не второй источник. BP определяет effect safety/dispatch/readback semantics, не вторую таблицу wire fields. AD владеет PART/DATA/IF ownership; provider API v2 Create Task/Get Task — внешний versioned contract. Наличие API reference не является live permission evidence или exactly-once обещанием.

## 3. Точные технические обязательства

### HP-BP-004/T/001

**Connection scope, сторона Delivery:** секрет ClickUp хранится отдельно шифрованным, используется только designated API origin и не попадает в business command/config/log/AI/URL/export. Установка/reconnect доказывает external Workspace/List context и required available reads; другой аккаунт/назначение требует configuration change, не переадресует прежние cases. Отсутствующее доказательство права записи не выдаётся за установленное из одного GET. Every provider call проходит действительный connection epoch/local dispatch barrier HP-BP-001. **Критерий:** secret-marker isolation, same-scope reconnect и отсутствие new start после positive revoke; FR003/307,RULE011,NFR005/006/013.

### HP-BP-004/T/002

**Idempotent internal command:** одной TX сохранить consumer inbox и единственную create-effect lineage по workspace+case+effect_kind. Episode/config/attempt не создают новую lineage. Повтор command ID+digest возвращает прежний outcome; другое содержание того же ID —409/integrity incident. Новая command identity того же case не обходит unique lineage/prior knowledge. Lost ACK допускает повтор внутреннего RPC, никогда повтор ClickUp POST по умолчанию. **Критерий:**32одновременных duplicates одного case дают не более одного dispatch admission; FR104/105/208,RULE001/008,NFR001.

### HP-BP-004/T/003

**Pre-dispatch gate:** до каждой новой создающей attempt нужны current authoritative scope/epoch, exact pinned case/config/episode, fresh complete source proof из BP003/BP002, lifecycle permission, достаточный контекст и no possible/existing prior effect. Каждый обязательный наблюдённый source fact должен быть не старше10s на dispatch admission (возраст по самому раннему обязательному чтению, не последнему полю); ожидание quota/очереди не продлевает его — возвращает refresh-required без отправки. Episode deadline и NFR write counter проверяются atomically с ledger transition. Pause не разрешает новую attempt; до actual dispatch не начатый queued create удерживается. **Критерий:** expired proof/revoke/pause/duplicate/unknown не отправляют POST; FR103/203/206,RULE003/004/006/008,NFR003.

### HP-BP-004/T/004

**Durable effect fence:** единственный CAS PREPARED→DISPATCHING фиксируется до передачи HTTP request sender. Только выигравший CAS может начать эту attempt. Перед фактическим network start проверяется сериализованный runtime barrier; stale sender не обходит его. После DISPATCHING crash/lease expiry/response loss не возвращает PREPARED: запрос мог иметь эффект даже при отсутствии task ID. Crash после fence до фактической отправки законно приводит к консервативной неизвестности, не слепому новому созданию. **Критерий:** после process kill/restart нет второго POST без достаточного no-effect proof; FR105/108/109,RULE008,NFR001/014.

### HP-BP-004/T/005

**Knowledge/повтор:** KNOWN_NO_EFFECT требует сохранённого конкретного достаточного proof. Безусловно разрешён только локальный доказанный отказ ДО network start/без возможности передачи запроса; provider-specific whitelist не вводится через общий HTTP4xx. В этом baseline общий5xx, timeout/reset, неверный/непрочитанный ответ после отправки и неизвестный provider code → AMBIGUOUS. Если реализация добавляет конкретный documented provider rejection в whitelist, это совместное review технического источника с exact schema/official basis/contract evidence, не Worker choice. Отказ, для которого proof отсутствует, не объявляется no-effect. Число automatic attempts не превосходит NFR003; достаточный новый manual continue не стирает предыдущие attempts/lineage. **Критерий:** empty search/age/AI/ручное“нет задачи” не дают POST, valid pre-effect repair может продолжить тот же case; FR206,RULE008/013.

### HP-BP-004/T/006

**HTTP safety:** все внешние запросы, включая GET, используют common one-shot HTTP/1.1 policy HP-BP-007: новое connection, DisableKeepAlives=true, HTTP/2 отключён, redirects и generic application/SDK/proxy retry выключены. Это исключает скрытый повтор GET на reused connection после revoke. Actual-start gate удерживается до WroteRequest либо доказанного no-send, in-flight регистрируется перед barrier ACK. ClickUp non-idempotent POST имеет конечный overall deadline; В Create Task не добавляются `Idempotency-Key` или `X-Idempotency-Key`, включая пустую запись header map: опубликованный provider contract не даёт оснований idempotent-create, а Go Transport при таких headers может повторять network request. Ошибка после возможной отправки обрабатывается T004/T005. Business marker в description не является retry-safe transport flag. **Критерий:** target stub считает actual sends при stale connection/reset/redirect; runtime никогда не считает вымышленный header гарантией; NFR003,RULE008.

### HP-BP-004/T/007

**Readback:** после task ID observation выполняется GET конкретной task в разрешённой области. Проверяется принятый List/Workspace, наличие согласованного assignee, детерминированное service/order title, stable case link/marker и все required context fields стандартного task/description. Description marker детерминированный, без paid custom-field зависимости; система не использует AI fuzzy matching. Unknown/inaccessible field — не совпадение. Результат включает наблюдённые значения, source/version, completeness, read time, mismatches; только Processing применяет RULE007 и case state. Успешный POST/task ID не Confirmed. **Критерий:** любое существенное mismatch/incomplete не даёт ложный handoff; FR106/204/207,RULE007/009,NFR008/013.

### HP-BP-004/T/008

**Manual binding/search:** разрешённый участник предлагает native task ID либо каноническую ClickUp task URL; сервер извлекает ID и строит фиксированный API URL, не fetch пользовательского host/path/query. Проверяется вся область и T007 до признания связи. Несколько candidates/неоднозначный marker не дают автоматического выбора. Empty/incomplete search не доказывает absence. Существующий неправильный/удалённый target сохраняет knowledge; HitPipe не PATCH/delete/create replacement. Для Closed/Excluded новый GET сам не меняет state, owner action BP003 обязателен. **Критерий:** SSRF/misbound task/fake“готово” не дают confirmation/new POST; FR204–207,RULE002/007/008.

### HP-BP-004/T/009

**Quota/здоровье:** limiter сериализует реальные tokens по защищённому fingerprint совместно для bindings/replicas, не по operation. Rate-limit headers/reset обновляют durable next eligible time; ожидание ограничено исходным episode deadline и не держит SQL locks. Safe health reads имеют каноническую частоту NFR013; observed deny закрывает новые затронутые permissions, не ждёт следующего cron. Максимумы workspace проверяются совместно с реальными provider limits. Право записи отдельно неизвестно там, где API его не доказывает. **Критерий:**429 не busy loop, quota wait не stale proof resend и не internal reason для исключения case из NFR002 denominator; NFR002/003/004/013.

### HP-BP-004/T/010

**Outcome delivery/сохранность:** observed response/target facts и result outbox commit до positive ACK Processing. Replayed result ID/digest даёт один consumer transition; нет права повторить provider write из-за потерянного ACK. Минимальная lineage/possible effect/known target/reference сохраняется до workspace deletion отдельно от90d business payload. Logs не сохраняют task description/secret по умолчанию. Restore сначала применяет deletion/revocation overlays и latest safety knowledge; потерянный ledger interval→affected execution quarantine/Unknown, не resend. **Критерий:** crash/replay/content purge/old backup не создаёт дополнительной task; NFR001/008/009/014.

### HP-BP-004/T/011

**Внутренняя техническая модель knowledge:** PREPARED означает intent без разрешённого network start; DISPATCHING — durable fence с возможным эффектом; KNOWN_NO_EFFECT — T005 proof; RESULT_ID_OBSERVED — ID без полного readback; AMBIGUOUS — possible effect без достаточного knowledge; TARGET_OBSERVED — конкретный read с matching/mismatch/incomplete. Переход из DISPATCHING только к наблюдаемому response/no-effect proof/AMBIGUOUS; из AMBIGUOUS только к достоверному target observation/no-effect proof, не по timer. История append, не переписывается. Эти значения — внутренний ledger, не альтернативные business case states. **Критерий:** все recovery guards проверяемы и никакая административная команда“reset” не стирает safety fence; RULE002/008/013.

### HP-BP-004/T/012

**Совместимость:** accepted effect payload/config/marker evaluator version сохраняются, пока существуют связанные cases. Schema migration не меняет lineage key, meaning of DISPATCHING/AMBIGUOUS, old target refs или proof. New provider API behavior/SDK retry policy требует revalidation до rollout; unknown response fails closed. Формально additive wire field не считается совместимым, если меняет permission/error/retry semantics. **Критерий:** old inflight/recovered effect safe при обновлении/rollback; NFR001/006/014.

### HP-BP-004/T/013

**Каноническое отображение и сравнение, сторона Processing producer / Delivery consumer:** правило rendering версии `handoff-block-v1` — единственный нормативный владелец текстового handoff. Unicode строка валидна UTF-8; выполняются NFC и CRLF/CR→LF без trim/case-fold/collapse пробелов. Предложенное преобразование видимо в configuration/context preview до сохранения; нельзя менять смысл пользователя невидимым удалением символов. Недопустимый Unicode/control input и контекст с отдельной строкой, начинающейся `[HITPIPE-HANDOFF:` или `[/HITPIPE-HANDOFF:`, отклоняется с понятным действием исправления, не экранируется молча. Все сравнения работают с тем же normalized text.

`service_label` — явно принятая строка услуги из выбранного Price/конфигурации, без customer PII, максимум120 Unicode scalar values. Если исходная каталоговая строка длиннее, proposal использует первые119 scalar values плюс `…`; полный источник и факт сокращения видны при принятии, владелец может выбрать другую ограниченную подпись. Никакого изменения уже принятой подписи при следующем запуске нет. `order_public_ref` = `HP-` + uppercase RFC4648 base32 всех16bytes case UUID без padding; это стабильная opaque ссылка без контакта/имени клиента. Точное `SERVICE_ORDER` имя task: `service_label + " — " + order_public_ref`, максимум152scalar values и wire255. Task title сравнивается точно после указанной normalization; другая строка — mismatch.

Описание содержит ровно один следующий блок. Ключи контекста берутся только из `required_context_keys` принятой config и сортируются по ASCII key ascending; значения выводятся полностью, включая normalized переводы строк. Обозначения в угловых скобках ниже — specification metavariables, не буквальный UI text:

```text
[HITPIPE-HANDOFF:<canonical-lowercase-case-UUID>:v1]
Service: <service_label>
Order: <Stripe Checkout Session ID>
Context:
<key_1>: <normalized_value_1>
<key_2>: <normalized_value_2>
[/HITPIPE-HANDOFF:<canonical-lowercase-case-UUID>:v1]
```

При пустом required context список строк после `Context:` пуст; отсутствие обязательного значения блокирует create. Required content serialized UTF-8 byte budget определяется NFR004 и проверяется ДО provider send; нормализация/JSON-escaping не позволяют обойти byte limit. Максимальный rendered block —36KiB UTF-8 включая fixed labels/order/keys; превышение удерживает действие с actionable size error. Это дополнительный производный bound, не разрешение превысить32KiB case context.

Readback находит ровно одну точную пару delimiter lines текущего case и сравнивает извлечённый полный блок с заново построенным expected block; multiple/missing/malformed blocks — incomplete/mismatch, не поиск “похожего”. Дополнительные пользовательские заметки снаружи блока допустимы и не подтверждают/не изменяют обязательный контекст. Внутренний новый line в value не разбирается как источник поля: whole-block equality устанавливает все required values, порядок и source linkage. Assignee check — configured assignee присутствует среди task assignees; дополнительные внешние assignees сами по себе не стирают его присутствие. List check — exact configured native List ID в выбранном Workspace. Manual binding использует те же expected title/block/List/assignee checks, без ослабления из-за человеческой ссылки. Известная server-side трансформация провайдера, не проходящая этот contract, удерживает live-допуск до exact compatible adapter revision, не разрешает fuzzy matching. **Критерий:** cross-language fixtures, multiline/delimiter/Unicode/byte-boundary/multiple-block tests и actual provider readback устанавливают это поведение; FR105/106/205,RULE007/012,NFR004/008.

**Budget serialization:** предел32KiB измеряется как UTF8 bytes RFC8785 JCS object `{configured_key: normalized_value}` только смысловых key/value, без timestamps/provenance. `serialized_value_bytes` означает полную JSON string value с кавычками/escaping, не размер исходного текста. ASCII keys берутся из принятого schema; пустой object занимает2bytes, непустой object — `sum(JSON_value_bytes + key_ASCII_bytes +4)+1`. Нормализованный текст не обрезается молча ради бюджета.

## 4. Взаимодействия, состояния и исходы

| Исход | Нормативный локатор |
| --- | --- |
| Повтор внутреннего command/result | T002/T010; прежнее durable knowledge |
| Отказ до возможного network effect | T003/T005; concrete proof, bounded permitted continuation |
| Ответ task ID | T007/T011; observation, не confirmation |
| Потеря ответа/неизвестный5xx/crash | T004/T005/T011; AMBIGUOUS без новогоPOST |
| Readback mismatch/incomplete/несколькоtask | T007/T008; видимое препятствие/unknown по Processing |
| Revoke/deletion/ledger restore gap | T001/T010; deny/hold и honest in-flight uncertainty |

Authority, identity/конкуренция, timeout/cancel/recovery и lifecycle имеют адресуемых владельцев T001–T012; таблица не вводит второй protocol. Общая source-state модель — AD/Product Shared, не этот перечень knowledge.

## 5. Обеспечение качественных и продуктовых ограничений

| Источник | Механизм | Сквозная проверка |
| --- | --- | --- |
| FR104–106/108/204–208,RULE001/007/008/009/013 | T002–T008/T010/T011 | actual send counter + Processing state + concrete target observation |
| NFR001/003/008/009/014 | T004/T005/T010/T012 | crash/restore/purge/replay всех копий, не один mockresponse |
| FR307,NFR005/006/013 | T001/T003/T009 | actual start barrier всех runtime; >5s всегда incident/FAIL, нет hidden availability exclusion |
| NFR002/004 | T003/T006/T009 | выбранный полный E2E load/cohort; local latency не полный PASS |

## 6. Параметризация, версии и совместимость

| Адрес | Значения/контекст | Владелец/фиксация | Гарантии |
| --- | --- | --- | --- |
| HP-BP-004/PARAM/HTTP_DEADLINE | 1–10s на provider call, никогда позже episode deadline | service/platform owner, versioned config и evidence | no unbounded request; timeout не no-effect |
| HP-BP-004/PARAM/HEALTH_INTERVAL | 1–15min active connection | service/platform owner, config pin | NFR013 без ослабления |

Source proof freshness10s — фиксированное производное обязательство T003, не свободное tuning. Максимумы writes/episode/retention принадлежат NFR; service owner не вправе менять. Совместимость — T012; первая редакция ничего не supersede. Source/page/wire/provider/config/Product/build versions различны; новая публикация не переключает frozen work и не authorizes execution.

## 7. Проверка соответствия и фактическое evidence

План: AT-EFFECT-01–07,AT-SEC-03/05,AT-PAY-01,AT-PAUSE-01/02,AT-DATA-01/02,AT-REC-01. До live необходимы contract tests с actual send counting/HTTP reset/redirect/crash и конкретный authorized ClickUp readback. Reference SQL/CAS/transport spike подтверждает только проверенный паттерн/версию, не весь distributed connector или live permission. Фактические evidence/digests и независимый review — отдельные записи итогового package; авторский текст не PASS.

## 8. Существенные вопросы и контроль перед приёмкой

Unknown outcome — предусмотренный результат, а не обязательная “дырка”, которую Worker закрывает новым POST. Baseline не нуждается в provider idempotency; никакого необоснованного exactly-once. Реальные customer permissions/readback completeness/provider quota остаются external live gates. Полный formal closure и независимый review нужны до принятия source set. Formal acceptance/admission не выдумываются: authority_ready=false/execution_authorized=false. Прямой permission/retry-fence violation→DENY; incompatible source/proof→ESCALATE+affected hold; AUTO ограничено адресованными параметрами и принятой AD.
