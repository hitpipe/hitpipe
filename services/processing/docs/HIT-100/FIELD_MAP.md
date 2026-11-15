# HIT-100 — карта полей и персистентности

Основа: [точный OAS fedbe40b](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json), [точный SQL](https://github.com/hitpipe/project-os/blob/fedbe40bb597e25f66aeb1172af3ff7459c81b40/contracts/hitpipe/mvp-2026-10-31/formal/sql/processing.sql). DTO/schema subset воспроизводимо генерируются `tools/drafts/generate.py`; formal не изменён. Полный принятый manifest — [source-audit.json](source-audit.json).

| Схема / поле | Источник / смысл | Хранение / проверка |
|---|---|---|
| DraftWrite.command.request_id | UUID команды клиента, не заголовок полномочий | user_commands.request_id, actor из текущей Authority, JCS digest тела, aggregate_id, expected/result_revision, APPLIED; полный ответ не в SQL |
| command.expected_revision | обязательна для изменения существующего агрегата, readiness/rehearsal; для создания отсутствует | SQL CAS + advisory command/draft lock; If-Match при наличии совпадает; чужая устаревшая команда 412 |
| DraftWrite.config | ровно принятый DraftConfig | канонический JSON в собственном содержимом DRAFT_DATA; не второй продуктовый формат |
| DraftConfig.draft_id/revision | UUID черновика, начальная revision=1; обновление требует текущую revision и увеличивает её на 1 | drafts.draft_id/revision; поля config сверяются с SQL и private root |
| name | пользовательское имя | только полное собственное содержимое; не SQL/логи/owner query |
| source_connection_id / destination_connection_id | заявленные UUID соединений; сами по себе не полномочия | SQL technical_config + полное содержимое; при проверке текущий owner порт проверяет scope и resolves bindings |
| mode | LIVE/TEST из OAS, без нормализации регистра | безопасный SQL subset + собственное содержимое |
| price_alias / list_alias / assignee_alias | пользовательские псевдонимы; не идентификаторы провайдера | только содержимое; owner Check получает ровно эти псевдонимы и connection IDs, не name/контекст |
| service_label | выбранная подпись каталога; не ответ внешнего API | только содержимое; readiness проверяет NFC/управляющие символы; не расширяется safe_config_valid |
| title_pattern | OAS enum | SQL безопасный subset + содержимое |
| required_context_keys | массив OAS identifiers, уникальность/границы сохраняются | SQL безопасный subset + содержимое; отсутствующий массив отличается от пустого |
| ReadinessCheck.check_id | новый UUID только при новой успешно сохранённой команде | полный response file, связан с command node; replay возвращает тот же UUID |
| draft_id/draft_revision/config_digest | точная сохранённая редакция и JCS SHA256 её значимых полей | полный ответ; digest исключает name/draft_id/revision, включает остальные фактически присутствующие поля, порядок массивов сохраняется |
| checked_at/expires_at | фактическое время; минимум CheckTTL, draft/grant/binding expiry | полный ответ; новая проверка не продлевает старую; успешность перепроверяется при commit |
| passed/issues | полнота + текущие owner connections/account/mode/Price/List/Assignee и свежая привязка workspace/principal/epoch | честный отрицательный ответ при неизвестности/нет порта; issues — OAS Error с безопасной причиной, без provider diagnostics/секретов |
| RehearsalRequest.command/draft_id/draft_revision | команда конкретной сохранённой версии | metadata + refs как выше, version mismatch 412 |
| examples[].synthetic_session_alias | только example_* по OAS | только полный request digest; не настоящая Session/order identity |
| amount_minor / paid / duplicate / missing_context_keys | точные синтетические значения; ноль/false известны, не отсутствуют | строгие обязательные поля/числа/массивы; никакой записи SourceFacts/реального контекста |
| RehearsalResult.rehearsal_id/draft_id/draft_revision/config_digest | новый rehearsal UUID и сохранённая конфигурация | полный response file + command ref, повтор восстанавливает исходные байты |
| results | 1–10 результатов в исходном порядке примеров | pure evaluator adapter; paid positive CONFIRM, unpaid/missing context WAIT, paid zero EXCLUDE; duplicate без доказанного прежнего результата HOLD_UNKNOWN |
| live_requests_made | всегда известный 0 | OAS const=0 и тесты; rehearsal не получает ни одного provider/network порта |
| Error.code/message/request_id | безопасная категория и UUID корреляции | HTTP 400/401/403/404/409/412/413/503, без private request/ошибок БД; optional current_revision/retry_after_seconds не выдумываются |

## Ссылки, атомарность, срок и replay

`drafts.prose_ref/prose_sha256/prose_expires_at` указывают на private root DRAFT_DATA; `expires_at` совпадает со сроком корня/config. Root — частный конверт ссылок, содержащий ref config, его digest и tail command ledger. Config bytes остаются точным DraftConfig. `Origin.CaseID` — историческое имя поля собственного файлового хранилища; для DRAFT_DATA там draft UUID, никакого case_identity не создаётся.

Command node в собственном содержимом связывает workspace/draft/actor/request_id, digest тела, operation kind, expected/result revision, config digest, response ref и предыдущий node. Там же сохраняется authority epoch; для положительной готовности сохраняется свежий BindingProof (scope, owner IDs, псевдонимы, режим, проверенное время и срок). Это частная provenance, не новые поля OAS/SQL. Будущая явная активация должна сравнить текущую редакцию/digest/epoch/привязки и срок, а не доверять одному passed; activateOperation здесь не реализована.

Files: write → file fsync → rename → directory fsync; SQL metadata/новый root публикуются только после этого в одной scoped транзакции. Сбой до SQL commit оставляет не опубликованный orphan; его не удаляют при неизвестном исходе, только после срока. Потеря ACK/аварийный выход после commit восстанавливает полный response по исходной команде. Нет второго результата, новых таблиц или payload в user_commands/technical_config. Readiness/rehearsal append не меняют config revision/updated_at и не сдвигают исходный срок. Старые ответы после изменения черновика — исторические, не новое положительное разрешение.

Отсутствующий/испорченный/истёкший/очищенный private root или response, исчерпанный явный бюджет обхода ReplayNodes дают 503. Нельзя восстановить ответ из одних metadata или сделать новый check под прежним ID. Старый ledger может быть недоступен после истечения его содержимого; бессрочная персистентность не обещается. SQL используется hp_processing_app с SET LOCAL workspace и принудительным RLS, без owner/superuser/BYPASSRLS. Никакой миграции заново.
