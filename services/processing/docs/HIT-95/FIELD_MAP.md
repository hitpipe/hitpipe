# HIT-95 — источники обязательных полей

Статус: PARTIAL / SOURCE_DECISION_PENDING. Карта описывает источник и текущую
реализацию, а не подтверждает полный успешный CaseDetail. OAS/SQL не изменены.
Все структуры создаются только после fresh authority и scoped owner read.

## Cases

| Поле | Источник/правило | Фактический статус |
|---|---|---|
| items | case_identity + принятая config + собственный context; порядок admitted_at,case_id | Пустой фильтр200 проверен; непустой результат503 до двух решений ниже |
| next_cursor | HMAC-SHA256 keyset token; workspace/kind/case/filter/snapshot/позиция; null в конце | Codec и реальный SQL keyset проверены; непустой Cases API200 не заявлен |
| snapshot_at | transaction_timestamp() read-only repeatable-read транзакции, UTC | Проверен в реальном пустом Cases200 |

## CaseSummary

| Поле | Реальный источник/правило | Ограничение |
|---|---|---|
| case_id | case_identity.case_id | UUID, собственный workspace |
| operation_id | case_identity.operation_id | Связь того же workspace |
| config_id | case_identity.config_id | Старый case остаётся на своей config |
| config_digest | accepted_configs.digest; join по workspace/operation/config; проверка JCS canonical_config | Не текущий selected config другой версии |
| state | case_identity.state, без переходов от GET | VALID/commit outbox не становятся CONFIRMED |
| revision | case_identity.revision; safe integer1..9007199254740991 | Не счётчик времени/источник даты |
| owner_user_id | case_identity.owner_user_id | Не browser header/не technical service principal |
| order_label | Закреплённого источника нет | BLOCKED; нет service_label/SessionID/revision default |
| received_at | case_identity.admitted_at, UTC | Сохранённый admission timestamp |
| updated_at | at последнего safety_history по at,entry_id; требуется его new_state==сохранённому state и at>=admitted_at, at<=snapshot time | При отсутствующей/противоречивой истории503; не clock.Now и не revision |
| next_action | Закреплённого правила нет | BLOCKED; собственная таблица действий не вводится |
| due_at | deadline_at текущего незавершённого episodes; null при отсутствии такой записи | Таймер WAITING_DATA не изобретается |
| target_url | В предшественниках нет qualified Delivery URL producer | Из known_task_id URL не строится; позитивная проекция пока закрыта |
| outcome_knowledge | case_identity.outcome_knowledge | Сохранённое знание, без усиления от финансового результата |

Непустые CaseSummary не выдаются вообще, пока обязательная Presentation не
получит подтверждённый источник order_label и правило next_action. Остальные
описанные источники не делают весь ответ квалифицированным.

## CaseDetail

| Поле | Источник/правило текущего read path | Проверка/остаток |
|---|---|---|
| case | CaseSummary выше | BLOCKED; нет фиктивного полного200 |
| context | case_content refs + реальный CASE_CONTEXT file; owner workspace/case, revision ceiling, class/hash/bytes/expiry | Только configured required_context_keys; посторонний payload не добавляется |
| context_complete | Все configured keys представлены пригодным собственным содержимым | Полный CaseDetail200 ещё не проверен |
| context_expired | Подтверждённая expiry metadata реального файла; просроченный body не возвращается | Expiry-on-read проверен отдельно; пропажа/коррупция503 |
| last_observation_at | В HIT-78/79 нет Delivery observation projection | Не подменяется датой SourceFacts/финансового решения; позитивная проекция закрыта |
| observation_complete | Нет квалифицированного Delivery readback источника в предшественниках | Финансовый VALID не complete observation; позитивная проекция закрыта |
| available_actions | Будущая подтверждённая Presentation, согласованная с next_action и действительными guard/grants | Нет разрешающего default; до завершения CaseDetail200 не заявлен |
| history_cursor | Продолжение отдельного history чтения; начальный GET history не требует cursor | Положительный CaseDetail и различия nullable ещё требуют проверки после решения |

## History

| Поле | Источник/правило | Проверка |
|---|---|---|
| items | safety_history + history_content + реальный HISTORY envelope HIT-79 | Полные controlled records200; 100+2 по HTTPS/PG/content |
| next_cursor | Подписанный scoped token по at,entry_id и snapshot upper bound; null в конце | Проверены оба ответа OAS, отсутствие повторов и подмена scope |

## HistoryEntry

| Поле | Реальный источник/правило | Поведение при недостатке |
|---|---|---|
| entry_id | safety_history.entry_id | UUID |
| case_id | safety_history.case_id, сопоставление с URL/scoped case | Чужой case404, без раскрытия его существования |
| at | safety_history.at, UTC; совпадает с Decision.At в content | Противоречие503 |
| actor_kind | safety_history.actor_kind | SYSTEM/USER/PROVIDER/SUPPORT не переименовываются |
| actor_id | safety_history.actor_id, присутствующий nullable UUID | SQL NULL остаётся JSON null |
| old_state | safety_history.old_state, присутствующий nullable enum | SQL NULL остаётся JSON null |
| new_state | safety_history.new_state, сверка с content Decision.State | NULL/неизвестный state503 |
| reason_code | safety_history.reason_code, сверка с content Decision.Reason | Причина не выдумывается |
| evidence_ref | safety_history.proof_ref, nullable opaque UUID | Это causal ref, а не проверенная provider ссылка |
| episode_id | сохранённый Decision.EpisodeID в HISTORY envelope HIT-79; непустой ID проверяется по episodes того же workspace/case | Пустое записанное значение→null; чужая/неустановленная episode503 |
| expires_at | history_content.expires_at, совпадающий с integrity-checked file metadata | Отсутствие/коррупция/expiry503, без at+90d default |

Старая SIGNAL_ADMITTED из HIT-78 не имеет history_content/expires_at: реальный
negative test подтверждает503. Positive fixtures отдельно и явно создают metadata
в свежем тестовом workspace; это не доказательство исправления старого writer.
Expiry-on-read удаляет только проверенный собственный файл; case/safety_history
и неизвестность внешнего эффекта не стираются.

## Ошибки и области доверия

Error соответствует принятому OAS: code/message/request_id, случайный UUID
request_id, без payload/session/SQL diagnostics/чужого resource existence.
401/403/400/404/503 различают отказ входа, scope и недоступность без выдуманной
полноты. Cursor не даёт полномочий: fresh IAM port и общий epoch/actual-start
gate проверяются для каждого чтения и начала ответа. SET LOCAL — только
defense-in-depth после этой проверки.

Для case keyset snapshot ограничивает admission cohort, а state читается заново
в каждой транзакции. Между HTTP-страницами не обещается один долгоживущий MVCC
snapshot всех изменяемых полей. Для history используются append-only at/entry_id
и тот же upper bound. Ключ курсоров должен быть явно предоставлен deployment;
его нет в коде/логах. Ротация делает старые курсоры невалидными.
