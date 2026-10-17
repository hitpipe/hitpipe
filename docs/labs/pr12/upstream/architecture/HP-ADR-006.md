# ADR — AI только в контуре подготовки и ограниченных предложений

**ID:** HP-ADR-006. **Рабочее состояние:** In Review. **Ответственный:** делегированный архитектор, поток UX/AI; source integration — Codex root.

## 1. Контекст и область решения

AI-native естественно-языковая настройка — часть первой полноценной версии HitPipe, а не маркетинговый future feature. Одновременно HP-VIS-D-009, HP-POL-001/003/004/005 и HP-NFR-001/006/007 запрещают обязательное постоянное LLM-суждение для обычного принятого исполнения, неявные полномочия и ненужную передачу реальных заказов. Нужно выбрать архитектурное место AI, сохраняющее минимальную дистанцию от намерения до работающей операции и честную ответственность.

Область — HP-PART-005 Python AI interaction, HP-PART-006 UX/BFF и граница с owner configuration/case authority Processing. Business automation допускается в существующем HP-PACK-001; универсальный tool-using agent, произвольная генерация исполняемого кода, auto-deploy и автономный reroute не добавляются. Actual buyer demand и качество конкретной модели пока не доказаны.

## 2. Выбранный вариант и rationale

AI — изолированный preparation/control-plane helper. Получает разрешённое намерение, описания ограниченных полей и синтетические примеры; выдаёт typed proposal/clarifying questions. Processing проверяет и сохраняет draft, проводит репетицию и принимает explicit owner consent на конкретную revision. Intake→Processing→Delivery обрабатывают принятые правила без новой AI зависимости. Python не обладает credentials, DB authority или tools бизнес-исполнения.

Так естественный язык остаётся первичным доступным входом, а не скрытым generic workflow builder. Пользователь получает конкретное проверяемое правило и возможность прямо поправить поля. Деградация AI не теряет established draft values и не останавливает active operation/case resolution. В пределах ограниченного pack недостаточное качество AI не оправдывает незаметное расширение инструментов/данных.

Для первого adapter конкретно выбран OpenRouter Chat Completions с `openai/gpt-6-luna` и принудительным Azure/EU ZDR routing, без fallback в non-ZDR или другие регионы. Exact routing/structured-output/default budgets — единственная норма HP-BP-005/T/022. Official model docs подтверждают текущий structured-output model; прочитанный public ZDR endpoint inventory содержит выбранный Azure/EU route. GPT-5 Mini не выбран для новой базы из-за official deprecated status; бесплатный Nemotron не выбран из-за его data-use restrictions и отсутствия strict response-format. Это не утверждение, что inference или потребительский usability уже проверены.

**Статус выбора:** Accepted по делегированию, source baseline Candidate, human acceptance не выполнена; прежняя пометка Proposed устранена для формального baseline; решение содержательно выбрано в рамках делегирования. Каноническая семантика — HP-BP-005/T/004–T/012/T/017/T/018 и AD; источник этим ADR не считается формально Accepted.

## 3. Рассмотренные альтернативы

| Вариант | Соответствие и trade-offs | Вывод |
| --- | --- | --- |
| Typed AI proposals, deterministic execution, direct-form fallback | Сохраняет AI-native setup и owner-controlled safety, ограничивает privacy/availability coupling; требует точной schema и honest unresolved handling | Выбран по продуктовой стратегии и проверяемой границе |
| LLM/agent управляет каждой оплаченной операцией с tool access | Может обрабатывать широкий неоднозначный контекст, но добавляет nondeterministic execution, privacy и постоянный provider dependency | Не выбран для текущего product scope; будущий отдельный продуктовый выбор |
| Полностью убрать AI из MVP, оставить form-only | Уменьшает integration surface, но отменяет утверждённый primary natural-language experience | Не соответствует HP-VIS-D-009/HP-FR-001 |
| Отправлять реальные заказы/логи для более точной настройки | Возможна контекстная полезность, но текущие версии этого purpose/recipient processing не разрешают | Не выбран; новые данные требуют отдельной продуктовой нормы и основания |
| Бесплатный Nemotron / любой здоровый provider без privacy filters | Низкая стоимость, но free endpoint запрещает sensitive input и допускает logging; availability fallback может незаметно менять получателя | Не выбран для продуктового freeform editor; Hermes deployment вне scope не изменяется |

## 4. Последствия и канонические технические источники

| Следствие | Где закреплён смысл | Совместимость |
| --- | --- | --- |
| Isolated Python без business write/DB/secret полномочий | AD trust boundaries, HP-BP-005/T/006 | Network/service identity не позволяет обход authority |
| Explicit payload preview, metadata/log minimization | HP-BP-005/T/004/005/017 | Тело, headers, baggage, APM и provider context проверяются совместно |
| Proposal schema и stale revision handling | HP-FORMAL-001 AI operations; HP-BP-005/T/007/008/019 | Никакого auto-apply неподтверждённых значений/маппинга |
| Rehearsal + explicit activation command, fallback без LLM | HP-BP-005/T/009–T/012; HP-BP-003 | Только Processing меняет operation/config authority |
| Конкретный GPT-6 Luna Azure/EU ZDR adapter и конечные request budgets | HP-BP-005/T/011/018/022, config pin | Модель заменяется совместимой qualification, не переопределяет принятую операцию и не снимает privacy constraints |
| Доступ BFF→Python только в dedicated mTLS issuer boundary | HP-BP-005/T/021 и HP-BP-001/007 | Service principal не выводится из forwarded headers и не даёт product role authority |

Риск конкретной модели — корректность ограниченного intent mapping, latency и внешний data-processing контекст. Он не закрывается названием framework или заявлением structured output. Provider refusal/invalid response остаются ошибкой подготовки, а не ошибочно принятой конфигурацией. User-entered free text может содержать сведения; универсальный PII classifier не обещается, actual visible editor + запрет скрытого enrichment сохраняются.

**Вопрос/метод:** HP-BP-005/Q/001 — до live AI подтвердить конкретные provider terms/retention/recipient conditions и провести bounded intent/privacy qualification. Product usability study по HP-NFR-012 остаётся внешней эмпирической проверкой. Это не незаконченный выбор функций.

**Actual evidence:** Python strict-schema fixture отвергает лишнее поле/`activate`; timeout остаётся одной попыткой; FastAPI/Pydantic/httpx/OTel cohort исполняется. Cross-language generated schema/client работают в bounded fixtures. Никаких реальных provider prompts, account data или paid calls не выполнялось. Fixture не доказывает semantic model quality, весь platform IAM или product readiness.

## 5. Пересмотр, исторические связи и изменения

Пересмотр требуется при утверждённом новом purpose AI в live operation, расширении data classes/tools, доказанной непригодности ограниченного proposal опыта или изменении внешних legal/processing условий. Он начинается у канонического Product owner, если меняет норму; не оформляется одним тихим SDK upgrade. Модель/endpoint migration с тем же смыслом проходит qualification и compatibility без изменения business automation scope.

Первый ADR этой границы; исторический research «AI agents» не является прежней принятой технической нормой. Supersession не изобретается. Source/model/build/version pins независимы; новая provider configuration не меняет ранее принятые operation versions и frozen tasks автоматически.

## 6. Проверка перед source review и faithful-проекцией

Указаны один связный выбор, реальные альтернативы, точные последствия и canonical owners. Primary AI experience сохранён, обычное исполнение и подтверждение outcome отделены. Приватность охватывает metadata/logs и automatic context, не только main request body. Empirical qualification, implementation tests, independent review и formal acceptance не смешаны; final source manifest закрепляет Product/AD/BP/formal зависимости.
