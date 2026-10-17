# Technical Blueprint — UX, BFF и ограниченное AI-проектирование

**ID:** HP-BP-005. **Рабочее состояние:** In Review. **Ответственный:** владелец потока UX/AI; интеграция единственной окончательной редакции — Codex root. Спецификация содержательно определена; статус страницы не означает формальную приёмку или соответствие реализации.

## 1. Граница спецификации и применимость

Область — Next.js browser UX/BFF (HP-PART-006) и Python AI interaction (HP-PART-005): подготовка ограниченного operation draft, безопасная AI-передача, конкретная проверка/репетиция/принятие, отображение дел и допустимых действий. Первое исполнение — HP-VER-001 с четырьмя SINGLE-профилями. TEAM/PUBLIC не включаются наличием расширяемой схемы. Python не исполняет события и не является универсальным агентным runtime.

| Сторона | Ответственность |
| --- | --- |
| Browser / Next BFF | Достоверное представление, same-origin boundary, передача реального инициатора, защищённый editor, транспорт пользовательских команд |
| Python AI interaction | Только ограниченные предложения/уточнения на разрешённом подготовительном контексте; нормализованный отказ при недоступности |
| Identity, HP-BP-001 | Аутентификация, opaque session, текущая membership/role authority, отзыв и scoped внутренний доступ |
| Processing, HP-BP-003 | Единственный владелец draft/configuration/version/rehearsal/admission/case и окончательного принятия команд |
| Intake / Delivery, HP-BP-002/004 | Подключения и внешние credential custody; Python никогда не получает business credentials; browser/BFF имеют только узкий transient credential-entry/handoff путь T/023 и не могут прочитать сохранённый secret |
| Provider | Внешнее AI-вычисление в явно установленном контуре обработки; не authority над продуктовым правилом |

Глобальные POL/RULE/NFR сохраняются. Локальная удобная UI-операция не расширяет полномочия и не заменяет обязательный backend guard.

## 2. Канонические определения и формальные источники

HP-FORMAL-001 — единственный владелец OpenAPI3.1.0 wire definitions: `contracts/hitpipe/architecture-2026-10-16/formal/openapi/*.json`, операции HP-PART-005/006 и используемые upstream commands/queries. Точные component/DATA/IF selectors, Git commit, digests и весь reference closure разрешаются AD и итоговым source manifest. Generated JSON Schema views и Go/TS types — проверяемые производные, не вторая редакция wire-правил.

Этот BP канонически определяет семантику подготовки/подтверждения, data minimization, отображение неизвестности и UX/BFF failure boundary. Business states, permission semantics и configuration ownership не переопределяются. Версии инструментов, compatible lockfiles, type-only bridge и реальные bounded spikes находятся в supporting stack evidence; последняя утверждённая подборка пакета закрепляет их exact source bytes. API shape не восстанавливается по тестовой fixture.

## 3. Точные технические обязательства

### HP-BP-005/T/001

**Сторона/условие:** BFF при каждом защищённом запросе/SSR. **Действие:** получить реального субъекта через HP-BP-001, проверить актуальный доступ к запрошенному workspace и передать подтверждённый actor context в целевой owner service. Browser-supplied role/workspace headers не создают полномочий; права BFF service account не подменяют пользователя. **Результат/предел:** отказ не раскрывает чужой объект, а отсутствие авторизации не превращается в пустой успешный список. **Критерий:** прямая ссылка, повтор и смена workspace проходят ту же отрицательную матрицу HP-NFR-005.

### HP-BP-005/T/002

**Обязательство BFF:** browser business traffic идёт через same-origin API. Cookies/tokens, service credentials и provider access не попадают в client bundle, local/session storage, URL, source map или `NEXT_PUBLIC_*`. Cookie/CSRF/Origin/session revocation механизмы принадлежат HP-BP-001; BFF применяет их на каждой изменяющей команде и не создаёт альтернативный обход. **Критерий:** собранные статические файлы и network traces не содержат контрольных секретов; forged Origin/actor/tenant command отвергнут до owner mutation. HP-POL-001/003, HP-NFR-005/006.

### HP-BP-005/T/003

**Обязательство Next:** конфигурации, дела, роли, история и результаты команд не разделяются через prerender, shared fetch/data/full-route cache, CDN или service worker. Для защищённых ответов — private/no-store и request-time authorized read; публичные неизменяемые assets отделены. Контекст одного SSR запроса не остаётся module-level mutable state следующего. **Критерий:** два параллельных workspace с одинаковыми путями/именами и повтор после revoke не получают предыдущую защищённую выдачу. HP-NFR-005/009.

### HP-BP-005/T/004

**Обязательство BFF/Python перед каждым исходящим AI-запросом:** построить payload только из явного подготовительного editor text, описаний разрешённых полей, ограниченной схемы и синтетических примеров. До пользовательского Send видны фактический editor content, назначение и получатель. В очередном ходе повторно добавляется только разрешённый подготовительный контекст; прежнее согласие не разрешает подключение реальных дел. Не требуется отдельный modal на каждое обычное сообщение, но скрытой отправки нового текста/получателя нет. **Критерий:** packet capture показывает ровно установленный allowlist; HP-RULE-012, HP-NFR-007, HP-FR-001/002.

### HP-BP-005/T/005

**AI minimization guard:** реальные заказы/кейсы, external account/task/person identifiers, customer contacts, брифы, raw events, credentials и журналы не извлекаются в provider context. Это относится также к headers, trace baggage, request names, metric labels, exception attachments, SDK callbacks и provider-side metadata. Workspace/actor IDs для внутренней авторизации не отправляются провайдеру «для удобства корреляции»; там допускается случайный несемантический request ID. **Предел:** свободный editor может содержать введённые человеком сведения; универсальное распознавание/очистка PII не обещается, пользователь видит actual content. Контроль known secret markers и запрет автоматического enrichment обязательны. **Критерий:** adversarial payload/log/trace tests соответствуют HP-NFR-006/007.

### HP-BP-005/T/006

**Python boundary:** сервис не получает DB credentials к case/intake/delivery/identity stores, provider business credentials, произвольные HTTP tools, shell/code execution или возможность активировать правило. Business/model egress допускается только к явно сконфигурированному AI endpoint; private control egress отдельно разрешён только к фиксированным IAM authorize/register endpoints по T/021; URL/model/headers не выбираются из пользовательского текста. Provider response — недоверенные данные. **Результат:** prompt injection не меняет trust boundary и не выполняет внешнюю бизнес-операцию. **Критерий:** injected tool/role/action/URL instructions дают только текст/отклонённое предложение; network/DB policy подтверждает отсутствие обхода. HP-POL-001/003/004, HP-VIS-D-009.

### HP-BP-005/T/007

**Python при ответе:** декодировать только bounded response, проверить schema, enum/size/format и extra-field rejection; не выполнять присланный код и не конвертировать непонятное в «успешную» конфигурацию. Неполные/противоречивые значения остаются unresolved. Proposal содержит только допустимые поля/абстрактные варианты, а реальные подключения выбирает доверенный путь подготовки у Processing. Неизвестный вариант или stale mapping revision отклоняется, не сопоставляется эвристически с похожим названием. **Критерий:** недопустимое `activate`, secret, неизвестный pack или подмена destination не изменяют draft/operation. HP-FR-002/004/008, HP-POL-004.

### HP-BP-005/T/008

**Processing/BFF при сохранении предложения:** AI-result сам по себе не становится принятой конфигурацией. Processing проверяет schema и текущие права/подключения/pack boundaries, связывает предложение с конкретным draft revision и применяет optimistic concurrency. Более старый ответ после новой правки не перезаписывает её. В UI различаются предложенное, сохранённое, неразрешённое и действующее. **Критерий:** две вкладки, задержанный AI-ответ и сменившийся target сохраняют новые значения и явный conflict; HP-FR-002/004/009, HP-RULE-003/004.

### HP-BP-005/T/009

**Принятие/репетиция:** BFF показывает конкретную owner-validated конфигурацию и результат синтетической репетиции, scope подключения, начало live-области и нерешённые условия. Отдельное явное действие владельца отправляет version/digest-bound command в Processing. Общий chat request, provider response, displayed preview и успешная connection check не являются consent. Изменившийся draft требует новой проверки показанного правила; BFF не подставляет silently новую revision. **Критерий:** без действительного принятия нет live state/effect; HP-FR-005/006/007 и HP-RULE-003/004/012.

### HP-BP-005/T/010

**Повторы пользовательской команды:** UI выделяет pending state и повторяет тот же transport command ID только для запроса установленного результата/безопасной идемпотентной доставки по owner protocol. Lost response не даёт локального success или нового действия с новым ID. Owner service определяет существующий результат; body mismatch одного ID — integrity error. Для неизвестного task-effect generic Retry/Reset control не существует: UI показывает только разрешённые backend actions HP-BP-003/004. **Критерий:** double-click, reconnect, потерянный HTTP ответ и две вкладки не создают дополнительный эффект; HP-RULE-001/008, HP-NFR-008.

### HP-BP-005/T/011

**AI timeout/cancel/outage:** один explicit Send имеет конечный deadline, один provider attempt и типизированный error/result. Автоматического replay через SDK/proxy/HTTP redirect нет. Cancel прекращает локальное ожидание и дальнейшую передачу; не обещает, что provider уже не получил предыдущий запрос. Поздний response не применяется после отмены, logout/revoke/deletion или смены revision. Пользователь может явно повторить подготовительный запрос, зная отсутствие нового результата; business execution при этом не повторяется. **Критерий:** timeout, invalid JSON,429,provider outage и late response не меняют active configuration; HP-FR-009, HP-NFR-001/007.

### HP-BP-005/T/012

**Сохранность без AI:** установленные значения draft/configuration принадлежат Processing и доступны через прямую форму независимо от chat transcript/модели. Отказ Python не блокирует чтение/правку допустимых полей, установленные case actions, pause/resume и ordinary deterministic execution. Нельзя читать AI transcript как единственный backup действующего правила. **Критерий:** отключение всей AI части оставляет owner-backed функции четырёх SINGLE capabilities доступными; HP-FR-009, HP-VIS-D-009, HP-NFR-001/014.

### HP-BP-005/T/013

**Отображение состояния:** UI читает case/operation/revision/observation time из авторитетного query, сохраняет восемь бизнес-состояний HP-RULE-002 и не принимает transport ACK за «Подтверждено». Последняя проверка назначения имеет видимое время и отличается от свежести UI. Unknown, stale view, failed query и empty result различаются; поздний ответ не понижает уже показанную monotonic revision. Видимые разрешённые действия берутся из актуальной authority и повторно проверяются backend. **Критерий:** сломанный readback/удалённая task/потеря соединения не показывают ложный успех; HP-RULE-002/007/009, HP-NFR-002/008/013.

### HP-BP-005/T/014

**UI refresh:** после mutation и при открытии/возврате фокуса запрашивать свежий authorized snapshot. Пока открыт наблюдаемый незавершённый случай, ограниченный polling заменяет обязательный WebSocket; interval выбирается только в PARAM/REFRESH. Failed/stale refresh сохраняет последнее известное с меткой непроверенности, не меняет business state. На logout/workspace change очищаются защищённые client caches, pending displays и in-flight response bindings. **Критерий:** нет cross-workspace flash; повторный просмотр отражает сохранённое состояние в границе HP-NFR-002; failures входят в соответствующий знаменатель проверки.

### HP-BP-005/T/015

**Доступность:** английский UI предоставляет полные основные пути клавиатурой, семантические headings/forms/buttons, связанные labels/errors, управляемый focus и ненавязчивые status announcements. Цвет не является единственным носителем case-state/error/blocked смысла; pending и error не пропадают из-за анимации. Zoom/reader/contrast проверяются по HP-NFR-011. AI text и business content отображаются escaped text, без raw HTML/исполняемого Markdown. **Критерий:** все применимые пути настройки/репетиции/принятия/паузы/разрешения дела проходят product accessibility criteria, а не только автоматический score.

### HP-BP-005/T/016

**Browser security/SSR:** Next BFF является единственным владельцем CSP для UI documents; per-response nonce покрывает необходимые inline framework scripts без production `unsafe-eval`/общего `unsafe-inline` для script. Trusted same-origin resources и назначенные connection endpoints ограничены; произвольные AI-provided ссылки/embeds/scripts не исполняются. Защищённый SSR с nonce остаётся request-time, не shared prerender. Edge сохраняет этот policy, не добавляя конфликтующую permissive копию. **Критерий:** стандартная hydration работает при production policy, stored/reflected XSS fixture не выполняется, secrets отсутствуют в CSP/reporting URL. Это обязательство реализации, не утверждение, что маленькая stack fixture уже проверила весь CSP.

### HP-BP-005/T/017

**Жизненный цикл данных:** Python не сохраняет prompts/completions как server transcript и не пишет их в обычные logs/APM; transient memory ограничена request lifetime и освобождается при завершении. Сохранение draft values у Processing следует их data class, не продлевается AI retry. Browser не создаёт неуправляемую постоянную копию editor/business text. Deletion/revoke overlay запрещает позднее применение результата; внешняя provider processing/retention область должна иметь подтверждённые допустимые условия до live включения. Нельзя обещать удаление чужих систем фактом очистки local cache. **Критерий:** markers не остаются в logs/metric labels/trace payloads/storage/backups AI сервиса; HP-NFR-007/009.

### HP-BP-005/T/018

**Контролируемая конфигурация AI:** provider endpoint/model/system-prompt/schema revision/token budgets определяются versioned deployment configuration, проходят совместную проверку и фиксируются в evidence без secret. Provider/model не является свободным per-request URL. Новая model/provider revision проверяется на поддерживаемые intents, unknown-field/unsafe-action rejection, privacy и latency до rollout; failure сохраняет manual fallback. Смена модели не переписывает принятые operation versions. **Критерий:** воспроизводимый model qualification record, rollback и отсутствие provider-dependent изменений обычного execution; HP-NFR-001/007/012.

Первая конкретная конфигурация определена T022, не оставлена выбором Worker. До live нет inference без подтверждённых account/processing условий; coding/mocked contract implementation не требует оплаченного вызова.

### HP-BP-005/T/019

**Wire/tooling:** генерировать TypeScript из HP-FORMAL-001 с pin tooling и diff-gate, использовать strict typechecking и отдельную runtime boundary validation; успешная TS компиляция не доказывает допустимость received JSON. Для Go consumers AI-схемы absent/null сохраняются `nullable-type:true`, если wire различает их. Числа за пределом safe integer передаются формально установленным string-типом, даты — в однозначном offset. Нет ручной второй schema в BFF. **Критерий:** исходные formal bytes, generated diff и boundary fixtures для null/absent/unknown enum/extra fields согласованы. HP-NFR-008, ADR005.

### HP-BP-005/T/020

**Пять потоков:** владелец UX/AI отвечает за HP-PART-005/006 и UI/BFF/provider proposal integration; четыре Go владельца сохраняют authority своих сервисов. Shared formal source имеет единственного интегратора. Параллельная работа начинается со stable contract fixtures; frontend mocks явно помечены synthetic и не становятся фактическим evidence end-to-end. Worker не устраняет interface conflict самостоятельной альтернативной schema или чтением чужой таблицы. **Критерий:** contract matrix owner/consumer замыкается до integrated slice, архитектурный дефект исправляется у владельца. AD allocation, HP-POL-005.

### HP-BP-005/T/021

**Внутренний TLS boundary:** Next BFF обращается к Go owners/Python через нативный `node:https.request` с client cert/key, доверенным server CA, ожидаемым server name и `rejectUnauthorized:true`; browser cookies не становятся service credentials. Python business listener использует Uvicorn0.54 TLS context с `CERT_REQUIRED`, trust store исключительно dedicated BFF client CA и minimum TLS1.3. Issuance этого CA допускает только BFF workload/его контролируемую ротацию; общий platform CA, ops health CA и чужой service certificate не дают доступа к business port. Отдельный health listener не маршрутизирует бизнес-методы. Поэтому runtime не рассчитывает на отсутствующий универсальный ASGI peer-certificate parser: service principal устанавливается выделенной issuer boundary; actor/membership context и действия всё равно проверяются по T001/HP-BP-001. Forwarded headers не заменяют mTLS. **Критерий:** без certificate, с сертификатом чужого issuer, с неверным server SAN и при попытке business path на health port нет business dispatch; валидный BFF client работает; expiry/rotation не приводят к отключению проверки. Ключи/CA mount/rotation принадлежат HP-BP-006/007.

**IAM control:** тот же Python runtime/process содержит второй отдельный mTLS listener с IAM-only client CA и только `applyRevocationBarrier`; business BFF issuer на нём не принимается. Health listener не принимает control/business methods. Оба listener используют один deny snapshot, boot identity и gate/task registry. Python при старте обращается только к fixed IAM register и перед запросом к IAM authorize; эти private обращения не являются разрешением обращаться к другим DB/business API. Внутренний `AuthorizedAIRequest` несёт минимальный control context workspace/principal/session/epoch и opaque session только для fresh IAM check; это не model input. После проверки subject/epoch Python привязывает pending request к scope. Ни control IDs, ни cookie/headers не копируются в provider payload/trace/logs. Actual-start/cancellation barrier языка Python определён HP-BP-007/T/007. **Проверка:** IAM→control работает с собственным сертификатом, BFF→control и IAM→business запрещены, оба listener видят один revoke epoch; stale session deny до provider call.

**Next server/control wiring:** Node24 entry использует официальный custom server API `next({dev:false})`, `await app.prepare()`, `getRequestHandler()`. Next output — обычный build, НЕ standalone: эти режимы несовместимы. Entry — native ES module JS под выбранным Node, не рассчитывает на transpile Next compiler. В одном процессе private HTTPS business listener8443 принимает только EDGE issuer, IAM-only control9443 — только barrier, OPS-only health9444 — только health. Один process-wide registry `globalThis[Symbol.for('hitpipe.runtime.gates.v1')]` доступен wrapper и Next bundled handlers; startup assertion проверяет identity/generation единственного объекта, дублированный registry failclosed. Ни отдельный процесс, ни копия module-local lock не считаются общим gate. До первых защищённых response bytes wrapper и owner-backed commands применяют BP007 actual-start правила. Динамические защищённые пути не static optimized. **Evidence boundary:** ранее выполненный standalone fixture подтвердил framework/build/browser subset, но не этот custom control wiring. До live требуется именно custom-entry production fixture с cross-chunk identity и freeze/revoke гонкой. [Официальный custom server contract](https://nextjs.org/docs/app/guides/custom-server).

### HP-BP-005/T/022

**Первый внешний AI adapter и обязательный routing:** Python/httpx делает один non-streaming POST к `https://openrouter.ai/api/v1/chat/completions`, model=`openai/gpt-6-luna`, provider.only и provider.order=`["azure/eu"]`, `allow_fallbacks:false`, `require_parameters:true`, `zdr:true`, `data_collection:"deny"`. `response_format` — строгий `json_schema` для разрешённого proposal; `reasoning_effort:"none"`, `max_completion_tokens:2048`; tools/plugins/web-search и fallback model list отсутствуют. Это preparation-only параметр, не Hermes reasoning setting. HTTPX deadlines/redirect/retry boundary — T011. Structured output проверяется локально даже при заявленной поддержке провайдером; refusal/truncation/non-JSON не становятся proposal.

**Область и предел:** public endpoint inventory на2026-10-16 подтверждает Azure/EU GPT-6 Luna в ZDR list, structured-output parameters и отсутствие implicit caching; это не доказательство account entitlement, качества, legal basis или фактического размещения каждого auxiliary OpenRouter процесса. UI называет обоих получателей OpenRouter/Azure(EU); обещание полного EU-only processing всей цепочки не делается. Private input/output logging и improvement opt-ins выключены, prompts не записываются в broadcast/APM. Невозможность соблюсти все фильтры, изменившийся observed upstream model/endpoint или отсутствие маршрута даёт bounded unavailable и manual fallback, а не отключение ZDR/переезд в другой region. Provider/model смена требует T018 qualification с новым config digest. Бесплатный Nemotron не выбран: его official free endpoint запрещает confidential/PII input, сохраняет данные для своих целей и не поддерживает `response_format`. **Критерий:** request-byte fixture проверяет все routing flags/recipient declarations и отсутствие fallback при404/429; no real paid/model call этим документом не утверждается. Источники — official OpenRouter chat/routing/ZDR, public endpoints inventory и official OpenAI GPT-6 Luna model documentation в supporting research.

### HP-BP-005/T/023

**Узкое credential entry/handoff исключение:** только отдельная owner-authorized форма подключения принимает введённый владельцем provider token/signing secret в password field. Browser держит собственный ввод в памяти до единственного bounded submit, не сохраняет в local storage/transcript/autofill/analytics и очищает после ответа/ухода; невозможно обещать отсутствие собственного ввода в памяти/DevTools браузера. BFF получает plaintext только transiently на designated `connectService`, проверяет session/CSRF/owner и передаёт mTLS соответствующему connector. Request/response bodies и capture/APM на этом пути выключены, exception snapshots не содержат ввод. BFF не хранит, не читает обратно и не использует provider secret для собственного API-доступа; Python/AI путь полностью исключён. Connector возвращает только Connection metadata. Generic GET/export/logging никогда не возвращает сохранённый credential. Повтор неизвестного setup command сверяет тот же request ID/digest у владельца; никакого вывода о business effect из setup. **Критерий:** secret-canary отсутствует во всех persist/log/cache/AI/response surfaces, кроме разрешённого connector secret file; неназначенный caller/роль/endpoint отвергает ввод. NFR005/006/007.

## 4. Взаимодействия, состояния и исходы

| Установленный исход | Адресуемая семантика |
| --- | --- |
| AI proposal валиден / неполон / недопустим | T004–T008; только предложение или unresolved, без принятия/эффекта |
| Draft conflict / поздний ответ / сменившиеся права | T001/T008/T011/T017; сохранённый новый authoritative state не затирается |
| Rehearsal показан / activation pending / lost response | T009/T010; различаются preview, consent и результат owner command |
| AI unavailable | T011/T012; manual supported preparation и ordinary execution сохраняются |
| Case confirmed / unknown / stale view | T013/T014; data authority и last-confirmed time не подменяются UI свежестью |
| Revoke/logout/deletion | T001/T002/T014/T017 и HP-BP-001; новые чтения/команды запрещены, старый response не возвращает данные в использование |

Identity/конкуренция/повторы: T008/T010; timeout/cancel/unknown: T010/T011; authority: T001/T008/T013 и AD; жизненный цикл: T003/T014/T017. Таблица является навигацией к единственным нормам.

## 5. Обеспечение качественных и продуктовых ограничений

| Каноническое основание | Механизм | Сквозная проверка / предел |
| --- | --- | --- |
| HP-FR-001–009; HP-RULE-003/004/012 | T004/T007–T012 | От intent до конкретного rehearsal/consent; model quality не доказывается schema alone |
| HP-POL-001/003; HP-NFR-005/006/007 | T001–T007/T016/T017 | Browser→BFF→Python→provider и metadata/trace/logs; реальные access controls проверяются отдельно |
| HP-RULE-001/002/008/009; HP-NFR-002/008 | T010/T013/T014 | Потерянный ответ, старые revision и unknown external effect на полном owner path |
| HP-NFR-001/014 | T011/T012 | Полное отключение AI и безопасные owner-backed функции; не общий platform availability PASS |
| HP-NFR-009/011/012 | T015/T017/T018 | Retention поверхности, реальные assistive paths и продуктовый usability-denominator; synthetic UI spike недостаточен |

Числа продуктовых SLO/ёмкости/удаления не переопределяются. Внутренние бюджеты ниже — технические ограничения подготовки, не новый обещанный результат или тариф.

## 6. Параметризация, версии и совместимость

| Адрес / норма | Разрешённый диапазон и default | Выбор / фиксация | Сохраняемая гарантия |
| --- | --- | --- | --- |
| HP-BP-005/PARAM/AI_DEADLINE, T011 | 5–30s total одного AI preparation; default20s; connect не более5s и не позже remaining total | UX/AI owner в versioned deploy config, фактическое значение в qualification evidence | Конечный отказ и manual fallback; не продление product execution episode |
| HP-BP-005/PARAM/REFRESH, T014 | 1–2s при видимой странице незавершённого дела; default2s; скрытая вкладка может остановить polling и обязана fresh-read при возврате | UX owner, config/build pin и network evidence | Honest stale state; совместная проверка HP-NFR-002; параметр не отменяет обязательный fresh request |
| HP-BP-005/PARAM/MODEL, T018/T022 | Начальное единственное pair: OpenRouter `openai/gpt-6-luna` через `azure/eu`; URL/routing/defaults T022. Arbitrary runtime provider/URL не допускается | UX/AI owner совместно с privacy/security owner; изменение только qualification и согласованным config digest | HP-NFR-007 и typed proposal boundary; ни один выбранный фильтр не снимается ради доступности |

Не разрешено менять trust/authority, scope AI inputs, action semantics, product states или mandatory acceptance как локальную настройку. Body/schema/token limits каноничны в formal/config qualification; их изменение проходит совместную совместимость и не скрывает truncation как успешный parse.

Совместимость определяется T008/T018/T019 и HP-FORMAL-001. Первое издание не supersede прежний нормативный BP. Новые page/source/API/model/build/Product versions не переключают frozen context автоматически. Rollout несовместимой draft schema удерживает affected preparation, оставляя доказанно независимое deterministic execution; миграция сохранённых draft делается только Processing owner.

## 7. Проверка соответствия и фактическое evidence

| Нормы | Метод | Граница |
| --- | --- | --- |
| T001–T003/T014/T016 | Два tenants, прямые URL/Origin/CSRF, concurrent revoke, nonce CSP, SSR/cache/XSS tests | Нужны настоящий IAM/BFF и production policy; schema/fixture не заменяют |
| T004–T008/T011/T017 | Контрольные secret/PII markers во всех outgoing surfaces, injected proposal/endpoint, timeout/cancel/late result | Проверить provider request bytes и логи/trace metadata, не только тело UI |
| T009/T010/T012/T013 | Synthetic rehearsal, digest/revision conflict, lost command response, AI shutdown и case owner fixtures | End-to-end state/effect устанавливается owner service; UI mock ограничен |
| T015/T018 | Полные клавиатурные/reader/zoom пути и HP-NFR-012 user study | Реальные исследования пока не выполнены; product gate не объявлен закрытым |
| T019 | Pinned clean install, Go/TS/Python codegen, strict build и nullable/extra-field fixtures | Совместимость инструментов и узкого wire subset, не весь продукт |

Фактические bounded checks: supporting `checks/stack/results` с exact argv/timestamps/log digests; Python4 tests PASS, Go8 tests и vet PASS, OpenAPI3.0/3.1→Go/TS codegen PASS, Next16.4.2 production Turbopack build и TS5.9.3 `skipLibCheck:false` PASS после проверенного type-only URLPattern bridge. Это синтетические fixtures без реальных аккаунтов/provider calls. Полный runtime/CSP/tenant/retention/user-study product acceptance не заявлен. Итоговые обновлённые результаты и source hashes берутся из evidence manifest, не реконструируются из этого абзаца.

Независимая проверка — отдельный root-coordinated exact-source review AD/BP/formals. Авторская проверка и успешный build не называются независимым review.

## 8. Существенные вопросы и контроль перед приёмкой

| Адрес | Тип / область | Условие закрытия |
| --- | --- | --- |
| HP-BP-005/Q/001 | Внешний qualification gate выбранного T022 live adapter, не неизвестный выбор модели | Реальные доступные terms/account/processing/retention условия и bounded intent/privacy evaluation до включения; начальный provider/model/routing и failure semantics уже определены |
| HP-BP-005/Q/002 | Implementation conformance evidence | Реализация T001–T020 и соответствующие integration/security/accessibility tests; отсутствие готовой реализации не выдаётся за пробел нормативной спецификации |

Для authoring package проверены: область, единственный owner wire, addressable norms, обе стороны lost/late/conflict исходов, bounded параметры, прямой fallback, metadata minimization и независимость обычного исполнения. Exact formal closure/ссылки/результаты независимого review закрепляются в итоговом пакете. Нерешённых архитектурных вариантов в этой границе не оставлено; внешний live qualification не подменяется гипотезой. Действуют AUTO/ESCALATE/DENY границы AD: конфликт, отсутствующий обязательный source или новый trust grant удерживает affected path. Документ не создаёт acceptance receipt, Development admission или доказанное соответствие продукта.
