# HIT-95 — карта полей, CCR редакция3

Основание: решение Lead [12640](https://hitpipe.atlassian.net/browse/HIT-95?focusedCommentId=12640)
и CCR [12646](https://hitpipe.atlassian.net/browse/HIT-95?focusedCommentId=12646).
Проверенная кодовая версия `15ed1e567907bee951715b3b025c657e2009044d`.
Схема OAS и formal SQL не изменены. Каждое чтение проходит fresh authority,
общий actual-start/epoch gate, scoped owner transaction и повторный gate ответа.

## Cases

| Поле | Источник и правило |
|---|---|
| items | case_identity + case-pinned accepted_configs + собственный context; порядок admitted_at,case_id; непустые200 проверены |
| next_cursor | HMAC token workspace/kind/case/filter/snapshot/позиция; null в конце, максимум100 на страницу |
| snapshot_at | transaction_timestamp() read-only repeatable-read транзакции, UTC |

Курсор ограничивает admission cohort; state читается заново в каждой транзакции.
Один долгоживущий MVCC snapshot между HTTP-запросами не обещается. History
использует неизменяемые at/entry_id с snapshot upper bound. Ключ явно задаётся
композицией, в production-коде/логах нет default/секрета. Ротация отвергает старые
курсоры. Курсор не предоставляет полномочий.

## CaseSummary

| Поле | Источник и правило |
|---|---|
| case_id | case_identity.case_id того же workspace |
| operation_id | case_identity.operation_id |
| config_id | case_identity.config_id, без миграции на новый selected_config |
| config_digest | accepted_configs.digest после workspace/operation/config join и JCS проверки canonical_config |
| state | case_identity.state, без переходов от GET |
| revision | case_identity.revision, safe integer1..9007199254740991 |
| owner_user_id | case_identity.owner_user_id, не browser/service identity header |
| order_label | точный canonical_config.service_label принятого config_id дела:1–120 символов, без control; отсутствует/повреждён→503; без усечения, подстановок или нормализации |
| received_at | case_identity.admitted_at, UTC |
| updated_at | at последней safety_history по at,entry_id; new_state должен соответствовать сохранённому state, дата не раньше admission и не из будущего; иначе503 |
| next_action | русский базовый текст сохранённого state + первый подходящий шаг1–9 решения Lead + безопасное объяснение реального producer code/knowledge/отсутствия команды, до512 символов |
| due_at | deadline_at незавершённого episodes либо null; не выдуманный WAITING_DATA timer |
| target_url | null без установленного источника verified Delivery URL; known_task_id не превращается в ссылку |
| outcome_knowledge | case_identity.outcome_knowledge без усиления финансовым VALID/outbox commit |

order_label — каталожное название, не уникальное имя покупки. Его конфигурация не
истекает при expiry context/history. Проверено изменение selected config/PAUSED:
старое дело сохраняет label/config_digest своей версии.

Приоритеты next_action реализованы в presentation.go буквально по Lead:
qualified RECHECK → UNKNOWN/possible send → mismatch → terminal → incomplete
context → установленное препятствие доступа → WAITING_DATA → RECEIVED/WORKING →
разбор причины. Запрет повторного create сохраняется при UNKNOWN/POSSIBLE_EFFECT
или dispatch fence, даже при RECHECK. TARGET_ID_KNOWN не подтверждает результат;
RESULT_VERIFIED объясняется как последняя сохранённая проверка, не текущее здоровье
ClickUp. UNESTABLISHED без отправки остаётся неизвестным знанием.

Каталог содержит только реальные codes текущих producers. Ни READER_UNAVAILABLE,
ни IDENTITY_MISMATCH не выдаются за доказанный отказ доступа к прежнему подключению.
Специального persisted access-denial code эти producers пока не записывают; условный
приоритет6 проверен чистой таблицей условий, а runtime не выдумывает такой факт.
Неизвестная причина отмечается фиксированным русским текстом без raw diagnostics.

## CaseDetail

| Поле | Источник и правило |
|---|---|
| case | полная CaseSummary выше |
| context | case_content + CASE_CONTEXT files; только configured keys; проверяются class/hash/bytes/expiry и causal workspace/case/revision; строки сохраняются точно |
| context_complete | все configured keys представлены пригодным контекстом, нет истёкшего обязательного содержания |
| context_expired | достоверная expiry scoped SQL metadata; expired body не читается/не отдаётся, даже после purge |
| last_observation_at | сохранённый last_confirmed_at в UTC только при CONFIRMED/RESULT_VERIFIED; иначе null; это историческая граница проверки |
| observation_complete | false без пригодных полных Delivery readback attributes; одна дата/ID/финансовый результат их не заменяют |
| available_actions | в этом срезе []: recovery handler и command-authority не подключены; это полный валидный ответ, не503 |
| history_cursor | null для начального отдельного GET history; продолжение возвращает сам history endpoint |

available_actions — пересечение реально подключённой команды, fresh command grant,
той же установленной task identity и выполненных проверок. Право чтения не является
таким grant. Единственный будущий кандидат RECHECK. Чистые unit-таблицы различают
отсутствующую функцию, неизвестную доступность и установленный отказ; они не
подключают recovery/разрешающий runtime port. ACTIVE не навязывается чтению или
будущим допустимым проверкам PAUSED. Закрытые/исключённые дела не переоткрываются.

## History / HistoryEntry

| Поле | Источник и правило |
|---|---|
| items | safety_history + retained history_content metadata; пока body пригодно — integrity/owner checked HISTORY envelope HIT-79 |
| next_cursor | scoped token по at,entry_id и snapshot; null в конце |
| entry_id | safety_history.entry_id |
| case_id | safety_history.case_id, сверка с URL и scoped case |
| at | safety_history.at UTC; live envelope Decision.At сверяется с SQL precision (microseconds), без изменения сохранённой даты |
| actor_kind | safety_history.actor_kind |
| actor_id | safety_history.actor_id, SQL NULL→JSON null |
| old_state | safety_history.old_state, SQL NULL→JSON null |
| new_state | safety_history.new_state, обязательный допустимый enum; NULL/неизвестный→503 |
| reason_code | safety_history.reason_code; для live body совпадает с Decision.Reason |
| evidence_ref | safety_history.proof_ref, nullable opaque causal UUID, не provider URL |
| episode_id | до expiry: сохранённый Decision.EpisodeID, принадлежность проверяется по scoped episodes; после expiry:null означает, что подробность не представлена, а не что эпизода не было |
| expires_at | настоящий history_content.expires_at; проверены ref/hash/bytes/created_at/interval≤90d; никогда не вычисляется reader-ом как at+90d |

Первый SIGNAL_ADMITTED теперь создаётся настоящим admission producer: одно DB
clock_timestamp для case admission/event/content creation; durable HISTORY file
fsync→rename→directory fsync до одного commit inbox/case/safety/history_content.
Retention по умолчанию90d, явно заданная меньшая внутренняя retention допустима,
но не задаётся через HTTP/signal. Повторы не меняют время/expiry и не пишут новый
файл; ошибка или неизвестный commit не подтверждаются ACK.

После достоверного now>=expires_at минимальный ответ использует только сохранённые
safety/expiry metadata, без чтения body. Purge удаляет body, не metadata/safety.
At/reason/actor/old/new/evidence/expiry сохраняются при повторе/перезапуске.
Старые строки без metadata, повреждённая metadata и пропажа/коррупция ещё
неистёкшего body дают503; старые данные не дополняются выдуманным сроком.

## Error и проверка

Error соответствует OAS: code/message/request_id; UUID создаётся случайно.
Нет payload/session/SQL diagnostics/чужого resource existence. Проверены401/403/
400/404/503, mTLS failure до handler и readonly/no cross-tenant leakage.

См. actual captures и протоколы в verification-r3/, матрицу9 DoD в README.md.
Сохранённые verification/ и supplemental/ относятся к прежней редакции2 и не
переименованы в результаты текущей версии.
