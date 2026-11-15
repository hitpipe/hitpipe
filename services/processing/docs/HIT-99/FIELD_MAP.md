# HIT-99 — карта полей и переходов

Authority: [точный OAS fedbe40b](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json), [неизменный Processing SQL](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/processing.sql), GLOBAL RULE001/002/007/008/009 и SystemDesign A004/A011–013/A016/A018–020/A025–026 из [manifest](source-audit.json). DTO/schema subset генерируются `tools/observations/generate.py` без правки formal.

| TaskObservation / Receipt | Источник, проверка, место |
|---|---|
| message_id | UUIDv4 внутреннего сообщения; inbox PK(workspace_id,message_id). Регистр UUID допустим, original wire case/digest сохраняет смысл. Не provider ID и не новая identity. |
| message_digest | SHA256 RFC8785 JCS всего исходного сообщения без message_digest. Исходный Unicode/I-JSON валидируется до lossy decode: lone surrogate отклоняется, корректная пара/literal разрешены. Проверка до полномочий/SQL; другой digest того же ID →409. Hash собственных файлов отдельный. |
| schema_version | точная const1.0; неизвестная версия/enum/fields/duplicates/case variants/trailingJSON отвергаются |
| workspace_id/case_id | существующий SQL case, scoped hp_processing_app/RLS; сравнение с независимым current Authority owner/operation/connection, а не разрешение из сообщения |
| lineage_id | неизменная текущая case_identity.lineage_id и существующий DELIVERY_INTENT outbox message; ACKED не требуется, sender state не меняется |
| episode_id | исходный EpisodeID из подлинного собственного HIT79 HISTORY связывает lineage/message/case/config/owner; проверяются свой episode и актуальный episode. Неверная связь409, заменённый episode412; identity не переназначается |
| observation_id | UUID наблюдения Delivery, сохраняется в собственном содержимом; не новое дело. Inbox dedup остаётся по message_id/digest |
| knowledge | один из6 wire enum; явная таблица ниже, не численный рейтинг |
| task_id | обязательный nullable native ID; при установлении хранится case_identity.known_task_id. Отличный ID удерживается; null/неполнота не стирает уже известный |
| observed_at | фактическое время утверждения Delivery; не будущий момент и не раньше подготовки episode. Private HISTORY хранит его отдельно от SQL времени принятия. last_confirmed_at устанавливается только полным подтверждением |
| complete | обязательный bool, не самостоятельное подтверждение |
| workspace_matches/list_matches/assignee_matches/title_matches/marker_matches/context_matches | все6 обязательны и nullable. Для RESULT_VERIFIED каждый должен быть известным true; отсутствие поля400, null допустим как неизвестность, false/unknown не подтверждают. Для полного mismatch нужен известный false, complete/ID/свежий evidence |
| proof_code/proof_digest | bounded code + SHA256 syntax из утверждения уполномоченного Delivery; private HISTORY. Processing не выдумывает proof body и не заявляет независимую проверку этого foreign digest |
| evidence_ref/evidence_expires_at | обязательные nullable поля; ссылка остаётся у Delivery, не локальный path/ref. Для подтверждения/полного mismatch/known-no-effect нужны ненулевые ссылка и будущий срок, позже observed_at. Нет fetch чужого содержимого; missing/expired честно удерживает неподтверждённый результат |
| Receipt.message_id/disposition | исходный ID и APPLIED либо HELD после SQL commit; 202 означает durable acceptance, не успех ClickUp. Optional record_id/revision сознательно не выдаются: эти два обязательных поля полностью восстанавливаются по inbox даже после TTL. Replay возвращает исходный Receipt, не новую revision/время |

## Переходы по принятой модели

Предусловия каждой новой записи: current Authority и общий actual-start допуск; верная case/lineage/episode связь; не закрытая/исключённая identity. Current snapshot читается отдельно, затем под lock/CAS повторно проверяются revision, связи, expiry и допустимый переход. Нет слепого enum max/rank.

| Пришедшее знание | Достаточные признаки | Изменение |
|---|---|---|
| RESULT_VERIFIED | complete, все6 matches=true, ID, неистёкшая evidence provenance | CONFIRMED/RESULT_VERIFIED, known ID, last_confirmed_at=observed_at, fence=true |
| TARGET_ID_OBSERVED | ID известен | UNKNOWN; TARGET_ID_KNOWN только если ещё нет исторического RESULT_VERIFIED/RESULT_MISMATCH; ID сохраняется |
| POSSIBLE_EFFECT / INCOMPLETE | допустимое утверждение owner | UNKNOWN; при отсутствии известной identity и прежнего verified/mismatch — POSSIBLE_EFFECT; имеющееся знание/ID/confirmation boundary сохраняются |
| RESULT_MISMATCH | complete, известный false среди matches, ID, свежая evidence provenance | ATTENTION/RESULT_MISMATCH; прежний last_confirmed_at и identity сохраняются |
| KNOWN_NO_EFFECT | полное утверждение владельца с proof provenance, нет task ID/установленной identity, matches=null; нет прежнего verified/mismatch | ATTENTION/NO_EFFECT. Это не право нового create: fence не очищается, lineage остаётся прежней. Empty search/timeout не должны маркироваться Delivery как доказанный KNOWN_NO_EFFECT |
| неполный/неподтверждённый RESULT_VERIFIED/MISMATCH/NO_EFFECT | недостаточно обязательных признаков | HELD, прежнее state/knowledge/identity/confirmation сохраняются, причина в истории |
| позднее/одновременное наблюдение | observed_at не позже последнего применённого | HELD/LATE, никакого отката. После TTL прежней детали SQL время принятия служит консервативной верхней границей её observed_at, не отображается как provider read time |
| другой native ID | конфликт с уже известным ID | HELD/IDENTITY_CONFLICT, никакого переназначения |
| чужое/неактуальное episode, неверная scope/lineage | связь не подтверждена | Error до durable положительной квитанции, без state/inbox/history изменения |

Более поздний mismatch либо неполный readback после подтверждения может дать ATTENTION/UNKNOWN; прежнее last_confirmed_at остаётся. Это текущая неопределённость и историческое подтверждение, а не откат identity. EXCLUDED/CLOSED_WITHOUT_CONFIRMATION не возобновляются автоматически. Никакая ветка не создаёт новый case/intent, не очищает ever_dispatch_fenced или lineage и не отправляет внешний запрос.

## SQL, собственная история и API чтения

Inbox + CASE state/outcome/revision/native identity/fence/last_confirmed_at + safety_history + history_content refs фиксируются одной транзакцией. Command advisory lock сериализует один message ID; case FOR UPDATE + revision/CAS не теряет concurrent наблюдение. Успех только после Commit. Повтор после потерянного ACK/crash/TTL не пишет вторую историю и не продлевает срок.

HISTORY — существующий собственный fsync/rename protocol: write→file fsync→rename→directory fsync до SQL commit; UUID ref/hash/bytes/origin/expiry проверяются. В приватном HIT79-совместимом конверте CaseID/Revision/OldState/Decision находятся parsed Observation, старое/новое знание, текущая authority provenance и время приёма. SQL хранит минимальную историю и refs, не message body/proof_code/контекст. SQL event at — время принятия, Observation.observed_at — отдельное время readback. Rollback может оставить не опубликованный orphan; без угадывания исхода он удаляется только по expiry. Срок собственной истории задаётся явно, ≤90сут; подробности Delivery имеют отдельный срок и не продлеваются.

Исходная собственная HIT79 HISTORY, а не краткоживущий intent payload, связывает lineage и episode после истечения срока исходящего содержимого. Если собственное доказательство связи отсутствует/испорчено/истекло, новая запись503, а identity и fence остаются. Прочитать чужой диск или выдумать эту связь нельзя.

Готовый HIT95 API и его политика представления сохранены; новый producer добавляет текущие reason codes и узкую проекцию наблюдения. CaseSummary показывает state/outcome/revision/updated_at; CaseDetail — настоящие observed_at и completeness, прежнюю confirmation boundary при наличии. History читает прежний конверт и после expiry/purge возвращает минимальные metadata без восстановленных атрибутов. Ссылка выдаётся только при текущих CONFIRMED/RESULT_VERIFIED, полном сохранённом readback, неистёкшей evidence provenance и том же native ID. Используется документированный ClickUp внутренний short URL для native alphanumeric ID ([первичный источник](clickup-native-link-source.json)); custom/небезопасная для этого route форма не угадывается и даёт null. При unknown/mismatch/expired детали ссылки нет. Ссылка не обещает текущее непрерывное здоровье провайдера или право viewer на доступ.
