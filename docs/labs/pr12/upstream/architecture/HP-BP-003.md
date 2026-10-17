# Technical Blueprint — Processing, конфигурации и дела

**ID:** HP-BP-003. **Состояние:** In Review. **Ответственный:** делегированный архитектор HitPipe; единственный владелец интеграции — Codex root. Кандидатная нормативная спецификация, не Accepted и не evidence реализации.

## 1. Граница спецификации и применимость

Processing — единственный владелец business admission, operation/configuration, case state/episode/responsibility, разрешённой команды и объясняемой истории. Применимость — HP-VER-001 и четыре SINGLE-профиля/PILOT NFR. Две будущие операции/TEAM delegation/scan не включаются наличием архитектурного поля.

| Сторона | Ответственность |
| --- | --- |
| Processing | Canonical state, atomic guards, immutable config pins, разрешённое продолжение/пауза/история |
| Intake BP002 | Durable authenticated ingress и свежие source observations |
| Delivery BP004 | Единственный provider effect ledger и конкретные target observations |
| Identity BP001 | Actual principal/scope/connection epoch, owner continuity и revoke barriers |
| Web/AI BP005 | Проверяемое proposal/view и явное намерение, не runtime business authority |

AD определяет PART/DATA/IF ownership; Product Shared владеет смыслом восьми состояний, ролями и guard. BP не создаёт альтернативных продуктовых состояний.

## 2. Канонические определения и формальные источники

Полные storage constraints принадлежат `formal/sql/processing.sql`. Реальный context/произвольные labels/AI intent/содержательная history/outbox payload живут только в service-owned durable content files HP-BP-006; SQL хранит opaque ref/hash/byte counter/expiry и minimal safety. Необходимая constrained accepted service_label сохраняется как семантика принятого правила; это не разрешение сохранять brief/contact/AI prose в config. Файл fsync+rename+directory fsync до SQL reference commit; missing file→hold, не выдуманное значение.

HTTP operation selectors/wire schemas принадлежат HP-FORMAL-001: `contracts/hitpipe/architecture-2026-10-16/formal/openapi/*.json`, OpenAPI3.1.0; exact commit/digest/imports разрешает итоговый manifest. `formal/schemas/*.json` — generated views, не второй источник. Здесь канонична семантика транзакций, конкуренции, разрешений и обработки knowledge. Product Contract candidate/pins — exact upstream, не mutable latest. Provider responses не выступают внутренней schema. У service database нет cross-service SQL grants; ссылки на объекты других сервисов разрешаются через authenticated API, не foreign SQL joins.

## 3. Точные технические обязательства

### HP-BP-003/T/001

**Admission, сторона Processing:** для authenticated source observation в разрешённой области atomically проверить существующий case по workspace+source account+mode+Session; при наличии привязать наблюдение к прежним case/config/owner/effect knowledge. Новый case допускается только для установленного подлинного оплаченного кандидата; unpaid/ещё не установленный paid-сигнал остаётся ingress до достаточного основания, но не закрепляет вечное исключение Session. Для нового кандидата блокировать operation/route rows, проверить active admission/уникальный маршрут/временную границу и сохранить case identity, текущую принятую config, owner и initial admission timestamp ДО полной последующей финансовой проверки/эффекта. Уникальный constraint — последний race guard. Неразрешённый вход возвращается/остаётся видимым по RULE101, не теряется из-за отсутствия case. **Критерий:** concurrent events одной Session имеют один case и одну config, pause/admission имеют однозначный порядок; FR102/104,RULE001/003/004.

### HP-BP-003/T/002

**Draft/readiness/rehearsal:** structured draft и его revision хранятся независимо от chat. AI возвращает только предложение; Processing проверяет strict pack schema/unknowns/supported refs, не угадывает отсутствующие поля. Config review view показывает полный смысл и исключения по FR004. Репетиция выполняется pure deterministic evaluator с synthetic adapter, не имеющим production connector capability. Сохранённый digest значимых config values связывает readiness/rehearsal; изменение invalidates affected readiness. Неустановленные обязательные права/owner/route/контекст не разрешают Prepared. **Критерий:** LLM outage не теряет draft, rehearsal не меняет live, stale rehearsal не активирует новое правило; FR001–006/008/009,NFR007/010/012.

### HP-BP-003/T/003

**Явное принятие/изменение:** действительный owner посылает stable idempotent command с expected draft/config revision; одной TX фиксируются immutable accepted config, фактический actor/time, command result и operation activation/selected config. First activation boundary устанавливается один раз; не сдвигается update/restart/reconnect. Изменение текущей операции создаёт отдельный draft/diff, применение проходит pause→проверка/репетиция→принятие→явный resume. Прежняя accepted config может быть явно выбрана текущей, но старые cases не мигрируют. **Критерий:** повтор/потеря ACK queryable без второй активации; старый case остаётся на старой версии; FR007/305/306/310,RULE004.

### HP-BP-003/T/004

**Case reducer:** только Processing transaction применяет разрешённый HP-RULE-002 переход с guard и durable typed history. Observation, attempt, user decision и confirmation различны. Closed/Excluded не меняют state от одного нового GET: соответствующий owner reconsideration/reopen обязателен. Unknown/possible prior effect сохраняется при close/reopen/archive/context purge. Known existing mismatching/deleted target не даёт create replacement. **Критерий:** все transitions/negative guards Shared воспроизводимы, no hidden ninth state/false confirmed; FR106/107/110/201/204/207/210–212,RULE002/007/008/009/013.

### HP-BP-003/T/005

**User commands/authorization:** каждый scoped query/mutation имеет actual subject, workspace/operation/case scope и актуальную проверку BP001. Stable request ID+body digest возвращает прежнее применённое command outcome; mismatch→409. Expected revision/row lock отклоняет stale incompatible command с актуальным разрешённым view. Context patch допускает только configured business fields и provenance, не payment/source/account/config truth. Close/reopen/reconsider и operation control owner-only. Owner continuity использует действительного operation owner; удаление последнего owner запрещено, операторский revoke не оставляет case без ответственности. SINGLE не получает TEAM per-case assignment через owner transfer. **Критерий:** concurrent close/continue, чужой URL и context payment patch не обходят guard; FR202/208/210–212/308,NFR005/008.

### HP-BP-003/T/006

**Permission на новое создание:** Processing получает complete fresh facts BP002 и применяет HP-RULE-006/007/012 непосредственно перед выдачей bounded dispatch permission BP004. Source facts/proof привязаны к case/config/account/context и не редактируются оператором. Нужны current rights/lifecycle/context, отсутствие possible/existing effect, неистёкший episode; истёкший source proof требует нового чтения. Разрешение не означает факт отправки/успеха и не переносится за revoke/pause. **Критерий:** refund/dispute/неполный payment proof/expired proof/revoked connection удерживают создающий command; FR103/105/203/206,RULE006/008.

### HP-BP-003/T/007

**Durable deadlines:** episode start/deadline, automatic write counter, WaitingData since, pause accepted/deadline сохраняются и не сбрасываются техническим retry/restart. Время ожидания internal queue входит в исходный budget. Только реальное qualified manual continuation с новым достаточным основанием создаёт следующий episode; lineage/история остаются. Timer worker after recovery сначала обрабатывает истёкшие границы, затем разрешает новые effects. Unknown write к сроку остаётся Unknown с reason/owner, не трактуется неуспехом без эффекта. **Критерий:** NFR003 границы active/wait/pause соблюдаются без auto-close или бесконечного Working; FR108/203/206/303,NFR001/003/014.

### HP-BP-003/T/008

**Pause/resume/archive:** accepted pause TX закрывает new admissions тем же operation lock, показывает Draining и предел уже разрешённой работы. Поздний queued create без фактического dispatch admission консервативно удерживается; pause не источник нового риска. К установленному сроку каждый episode имеет подтверждение или явное незавершённое состояние. Resume проверяет выбранную accepted config/connection readiness и явное disposition held signals/gaps, сохраняет момент. Уже admitted case остаётся pinned; новый uses current selected config. Archive только после Paused и допустимого explicit решения по каждому unresolved case; не стирает unknown. **Критерий:** race pause/admission/dispatch и v1case/v2resume не дают скрытого нового action; FR302–304/306/309,RULE003/004/013.

### HP-BP-003/T/009

**History/read model/notification:** canonical case query возвращает source link, state, pinned config, effective owner, known/unknown facts, last confirmation boundary и следующий permissible action. Typed events хранят actual human/service principal, previous/new knowledge, causal object refs и UTC time. UI не получает отдельного итогового workflow state. Internal attention notification deduplicated по case+unchanged reason; повтор обновляет одно уведомление, не запускает effects. Content заново permission-scoped при чтении; delivery/read внешнего email не обещаются. **Критерий:** старое подтверждение не выглядит continuous health; notification unreceived не auto-close; FR107/201/209/301/310,NFR002/008/011.

### HP-BP-003/T/010

**Данные/deletion:** case safety identity/version/possible effect/known target/responsibility минимально отделены от90d content/history. Read/retry не продлевает TTL. Content deletion требует заново получить недостающие обязательные поля перед допустимым будущим действием, но не создаёт permission unknown retry. Workspace deletion closes execution first, coordinated purge/restore overlay HP-BP-006 охватывает outbox/inbox/exports/backups; old webhook не resurrect. **Критерий:** replay после context purge не даёт второй task, restore удалённых данных не открывает исполнение; NFR009/014.

### HP-BP-003/T/011

**Effect observations и binding:** consumer атомарно сохраняет Delivery observation ID/digest и каноническую интерпретацию. Task ID/HTTP success не Confirmed без полного RULE007 readback. Existing mismatching task сохраняется как known effect и never create again; empty/incomplete search не доказывает absence. Manual link проходит конкретный target read BP004, не человеческую галочку. Потерянный internal response восстанавливается query command/lineage, не новым provider write. **Критерий:** at-least-once result delivery не меняет количество effects, unknown не сброшен; FR105/106/204–207,RULE007/008,NFR001.

### HP-BP-003/T/012

**Совместимость/восстановление:** immutable config schema version и evaluator version фиксируются в accepted snapshot и поддерживаются пока существуют применимые cases. Неизвестная/несовместимая старая config удерживает affected case с incident; не пересчитывается под новым смыслом. Additive wire change допустимо только при preservation старых errors/guards/default semantics; destructive migration не меняет pinned identity/config/ledger. Restore с неустановленным safety state держит affected execution, возвращает собственные доступные case views и honest uncertainty, а не clean empty operation. **Критерий:** rollback/migration/restore не повторяют возможное создание; NFR001/008/014.

## 4. Взаимодействия, состояния и исходы

| Область | Единственный нормативный локатор |
| --- | --- |
| Admission identity/config/route и conflict | T001/T005 |
| Configuration draft/Prepared/explicit accepted lifecycle | T002/T003 и продуктовый RULE003 |
| Восемь business states, close/reopen/reconsider | T004 со всеми guard RULE002 |
| Permission/dispatch outcome/partial/unknown | T006/T011 и Delivery HP-BP-004 |
| Timeouts/cancel/drain/restore | T007/T008/T012 |
| Authority/security/data lifecycle | T005/T009/T010 и AD |

Таблица навигационная: не авторит дополнительную модель переходов. Cross-service visible lag не разрешает новые effects на stale state; actual start проверяется barrier protocol HP-BP-001.

## 5. Обеспечение качественных и продуктовых ограничений

| Источник | Механизм | Проверка всей границы |
| --- | --- | --- |
| FR001–010/305/306,NFR007/010/012 | T002/T003 | полный user path, outbound AI и synthetic isolation, не одна schema |
| FR101–110/201–212,RULE001–013/101 | T001/T004–T007/T009/T011 | source→case→effect→readback→recovery, включая unknown |
| FR301–310,NFR003/005/006/009/014 | T003/T005/T007–T010/T012 | pause/revoke/restore/content lifecycle вместе, без hidden exclusions |
| NFR002/004/008 | T001/T007/T009 | exact initial cohort, timestamps и совместная нагрузка; queue не исключает slow case |

## 6. Параметризация, версии и совместимость

| Адрес | Допустимый диапазон | Полномочие/фиксация | Сохранённая гарантия |
| --- | --- | --- | --- |
| HP-BP-003/PARAM/TIMER_POLL | 0.1–5s между выборками due durable timers | service/platform owner в versioned deployment config/evidence | исходные product deadlines не меняются |
| HP-BP-003/PARAM/OUTBOX_BATCH | 1–100 records/claim, tenant-fair bounded concurrency | service owner, config pin | один lease не даёт разрешения повторять effect; deadline/cohort неизменны |

Все config/evaluator/wire compatibility — T012; effect freshness budget владеет BP004. Product numbers, authorization/trust/ownership и frozen context не Worker tuning. Первое издание ничего не supersede; изменение требует AD/BP/formal closure и оценки всех старых accepted config/cases. Новая publication автоматически не authorizes Development.

## 7. Проверка соответствия и фактическое evidence

План: AT-IN-02,AT-CONF-01–05,AT-STATE-01–06,AT-PAUSE-01–03,AT-EFFECT-01/04/05,AT-REC-01,AT-DATA-01/02. Scope proof — durable row-lock/uniqueness/outbox/inbox spike, затем полнофункциональные multi-service races, fault injection и E2E на выбранной продуктовой версии. SQL proof не подтверждает весь UI/provider/live authorization/latency. Фактический PASS допустим только из exact implementation/evidence pins; независимый source review — отдельный результат итогового пакета.

## 8. Существенные вопросы и контроль перед приёмкой

Материальные выборы области сделаны. Будущие фактические target/account/user-study измерения остаются продуктовой validation boundary, не неизвестным required behavior. Итоговый package проверяет полный188-obligation closure HP-VER-001, formal assets и hash-bound independent review. Формальная upstream/source acceptance не подделывается; authority_ready=false/execution_authorized=false до действительной процедуры. Нарушение permission/guard → DENY; конфликт canonical sources/несовместимая config → ESCALATE+affected hold; AUTO лишь в bounded параметрах принятой AD.
