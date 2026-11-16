# HIT-102 — карта команды и собственного состояния

Статус: локальная реализация, **согласование digest не завершено**. Источник формы — неизменённый OAS `fedbe40bb597e25f66aeb1172af3ff7459c81b40`; SQL — точная HIT-76 migration. DTO/schema генерируются, новый продуктовый формат не добавлен.

| Вход/источник | Проверка | Собственное хранение/выход |
|---|---|---|
|workspace_id path|canonical UUID, current grant scope, RLS|SET LOCAL workspace; scoped операции/черновики/команды|
|mTLS BFF|TLS1.3/CA/SAN/expiry; private8443|внутренний service caller, не бизнес-права пользователя|
|X-HitPipe-Session/CSRF|authoritative port, OWNER, current epoch/expiry/actual-start; CSRFVerified|grant/инициатор, без записи токена или CSRF в SQL|
|X-Request-ID / command.request_id|совпадение; stable UUID/JCS digest тела|user_commands PK(workspace,request_id); полный ответ через root private ledger|
|command.expected_revision / If-Match|оба согласованы; существующая операция требует её revision|owner operation CAS; не draft_revision|
|draft_id / draft_revision|существующий собственный current draft и точная версия|drafts operation link/root; не новая параллельная конфигурация|
|readiness_check_id|сохранённый полный ответ, command provenance, passed/no issues/expiry/version/digest/owner proof|read-only поиск в existing durable root ledger; нет новой SQL таблицы|
|rehearsal_id|сохранённый результат+examples, исходный request digest, 1–10 результатов, чистый existing evaluator|частные expiring examples в существующем command node; RehearsalResult wire без passed/expires_at|
|accepted_config_digest input|сейчас сравнивается с digest текущего DraftConfig и обоих проверок|**вопрос соответствия SQL canonical_config и Operation digest открыт**; см. DIGEST_DECISION_REQUIRED_RU.md|
|acceptance_text_version|managed-operation-v1 обязательно|идентичность исходной принятой команды через digest|
|confirm_activate|true обязательно; false не разрешает запись|никакого implicit activate из save/check/rehearsal/AI|
|текущие owner bindings|exact same source/destination connection, account/mode/Price/List/Assignee, principal/epoch/time/expiry|только разрешённые IDs/семантика в safe_config_valid accepted subset; alias/name остаются в файлах|
|canonical_config|PAID_ORDER_TASK_V1 + разрешённые IDs/mode/keys/title/service_label; SQL safe_config_valid|accepted_configs canonical_config; **его фактический JCS hash сейчас отличается от записанного draft digest — известный незавершённый дефект**|
|rehearsal_digest|hash точного сохранённого RehearsalResult|accepted_configs.rehearsal_digest|
|accepted_by/at|действительный principal и серверное DB время|accepted_configs в той же транзакции|
|owner/revision/config_id|авторитетный владелец; новая op1 / существующая op+1|operations owner,revision,selected_config_id; accepted config FK same workspace/op|
|first_activation_at|DB time только когда исходной границы нет|COALESCE + существующий immutable SQL trigger; exact replay/pause не сдвигают|
|Operation201 name|полное имя из current draft|только private expiring полный ответ; не SQL/логи|
|Operation201 accepted_config_id/digest|идентичность принятой конфигурации|id связан атомарно; **digest семантически не завершён**|
|connection_health|READY только при свежем совпавшем owner proof в этом bounded локальном пути|не обещание будущей доступности/provider integration|
|drain_deadline_at|null при первичной активации|не объявляется реализованным pause/drain|
|ошибка до/при commit или файлах|без доказанного SQL commit нет201|503 и сохранённая возможность точного повторения; orphan только собственный TTL/purge|

Короткие command→draft→workspace activation locks используются только в своей транзакции; source/network lookup вне locks. Load/Commit повторно читают current root/handles/version. Full response/node/root fsync завершаются перед публикацией SQL refs; общий commit связывает конфигурацию/операцию/первую границу/журнал. Пропавшие, чужие, повреждённые или истёкшие файлы не заменяются metadata-only ответом.

Частные поля CommandNode `Examples`, `OperationID`, `OperationRevision` относятся к своему файловому конверту, не расширяют OAS/DDL. Examples нужны для проверки фактического смысла rehearsal и должны давать тот же исходный command digest. Исторические узлы без них остаются читаемыми для собственных старых повторов, но не дают новую успешную активацию.
