# HIT-102 — карта полей, двух хешей и состояния

Редакция2, HP-CCR-ACTIVATION-20261116. Единственный действующий OAS `aa380880c43d8045a851d3c939d0cd0a879b7484`, SHA256 `e10177793312cd80224c461113ccab3fdc3f8af82fe0d08ce77513bcd5a2e31e`. Исходные44 операции и SQL сохраняются; меняются только утверждённые pointer fragments readiness/rehearsal/preview и описания digest. DTO/schema сгенерированы из этих exact bytes.

| Источник | Проверка/назначение | Собственное состояние/выход |
|---|---|---|
|workspace path, BFF mTLS, session/CSRF|private8443/TLS1.3, CA/SAN/expiry, актуальный OWNER/grant/scope/epoch/actual-start|RLS SET LOCAL; токен/CSRF/private grant не в SQL/preview|
|DraftConfig/revision|переиспользуется current saved content, safe SQL subset|name/aliases/private full config только owner files; existing draft root|
|ReadinessCheck.config_digest|JCS SHA256 значимых DraftConfig полей без name/draft_id/revision|**draft hash**, не accepted SQL hash|
|BindingProof|действительные подключения, catalog service_label, account/mode, resolved Price/List/Assignee/destination workspace IDs; инициатор/область/epoch/time/expiry|частный saved proof; не публичный preview и не SQL payload|
|CanonicalAcceptedConfig|один чистый builder: ровно pack_id,source_connection_id,destination_connection_id,destination_workspace_id,price_id,list_id,assignee_id,mode,required_context_keys,title_pattern,service_label|pack=PAID_ORDER_TASK_V1/title=SERVICE_ORDER, строгий OAS и safe_config_valid; secret/private proof/name/aliases нет|
|ActivationPreview.accepted_config_digest|SHA256 RFC8785 JCS полного canonical_config, без исключений и иной нормализации|**final hash** показанного и записываемого объекта|
|ReadinessCheck.activation_preview|passed=true→полный object; passed=false→null; корреляция сверх JSON Schema|полный check+preview+proof/root durable до ответа; ACTIVE/accepted_configs не создаются|
|check_id/expires_at|идентичность/срок именно выбранного preview; scope/principal/draft/version/draft hash|existing durable node/response, без новой таблицы или срока|
|RehearsalRequest.readiness_check_id|обязательный UUID; только этот successful saved check, не latest/default|selectedCheck читается без owner/provider вызовов и повторно проверяется при commit|
|RehearsalResult.config_digest|тот же **draft hash**; synthetic results по выбранному snapshot, live_requests_made=0|публичный результат без новых passed/expires_at|
|Rehearsal private CommandNode|исходные request/examples/JCS identity, check ID, final hash и существенный BindingProof|saved association; диагностические времена proof не определяют конфигурацию|
|ActivationCommand.accepted_config_digest|только **final hash** публичного selected preview; match private rehearsal final hash и check ID|повторное разрешение aliases в другие ID под старой командой запрещено|
|draft_revision / command.expected_revision / If-Match|draft revision отдельно; existing operation expected revision обязательно и If-Match согласован|пример draft1/existing op7→op8; новая operation ожидаемую op revision не выдумывает|
|X-Request-ID/command.request_id|совпадение для активации; UUID и стабильный JCS digest всего тела|existing user_commands PK; same ID/different body409|
|confirm_activate / acceptance_text_version|true / managed-operation-v1 обязательны|save/check/rehearsal/AI не принимают конфигурацию за человека|
|актуальный owner lookup|вне SQL locks; exact same semantic proof, валидность текущего scope/epoch/start gate|новые ID/отзыв/expiry не заменяют сохранённый preview; требуется новый check/rehearsal/показ/команда|
|accepted_configs.canonical_config/digest|записаны ровно показанные canonical bytes после JCS; hash повторно проверен|один **final hash** с Command/Operation, SQL safe_config_valid неизменён|
|accepted_by/at, selected_config_id, owner/revision|действительный principal, server DB time, same workspace/op FK|одна защищённая scoped транзакция; SINGLE/CAS/UNIQUE/RLS сохранены|
|first_activation_at|server time только при отсутствии исходной границы|COALESCE/immutable trigger; повтор/restart/pause не сдвигают|
|Operation201|точная nullable OAS форма, name из saved draft, final digest/accepted config ID, ACTIVE|полный immutable response только в owner files; никакой финансовой обработки/внешней записи|
|rehearsal_digest|hash точного сохранённого RehearsalResult|отдельное назначение SQL поля сохраняется|
|исторический exact replay|после текущего доступа, без повторной активации/owner resolve/TTL renewal|истечение readiness после commit не уничтожает сохранённую квитанцию|
|legacy/missing/corrupt|старые snapshots без нужных pins не достраиваются, metadata не заменяет полный результат|история сохраняется; новая активация требует нового корректного check/rehearsal|

`CanonicalConfig` используется общим путём preview/проверки сохранённого снимка/commit. Проверка replay не нормализует и не заменяет принятый final hash. Файлы response→command node→root завершают file fsync/rename/directory fsync до SQL refs/ACTIVE. Сироты до rollback не объявляются принятыми и очищаются по прежней TTL policy, без угадывания неизвестного commit.

Все protected reads, commit и response start сохраняют bounded gate2сек. Нет внешнего owner/provider запроса под SQL row lock. На commit повторно читаются current draft/version, выбранный check/expiry и saved pair; сохранённая конфигурация сравнивается с тем же common builder и текущими подтверждёнными привязками.

Ошибки:400 INVALID_REQUEST — неверная форма/const/required/null/duplicate/case/число;401/403/404 — сессия/права/видимость;412 REVISION_PRECONDITION_FAILED — revision;409 COMMAND_CONFLICT — неподходящая/истёкшая/изменившаяся pair/config/digest или конфликтующий exact ID;503 ACTIVATION_UNAVAILABLE/соответствующая owner ошибка — недостоверное обязательное чтение, corruption, missing port/content, IO/commit ACK. Body >256KiB сохраняет413. Неизвестное никогда не становится ACTIVE.

Существующие helper имена DraftPKI отделены от Delivery PKI только в тестах, не вводят production сертификаты. Новые proof/association поля принадлежат частному CommandNode/owner port внутри Processing; это не новый межсервисный API. Клиентские изменения HIT-97, реальные подключения и production runtime здесь не выполнялись.
