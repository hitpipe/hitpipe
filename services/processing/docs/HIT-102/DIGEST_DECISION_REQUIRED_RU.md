# HIT-102 — требуется согласованное соответствие digest

## Установленный факт

На коде `ec0eeb8c35c253dcccbab83b78ae329b8256dab3` локальные компоненты проходят96 команд и79 настоящих HTTPcaptures, CI5/5SUCCESS. Однако отдельная интеграционная ветка с HIT-95/98/99/104 воспроизводит activation201 → Intake202 → listCases503 READ_UNAVAILABLE. Самостоятельно пройденные компоненты не заменяют этот результат.

HIT-100 (`draftjson.Codec.ConfigDigest`) хеширует JCS значимых полей DraftConfig, исключив name/draft_id/revision. В этом представлении есть Price/List/Assignee aliases. HIT-102 сейчас проверяет этот hash по ActivationCommand/ReadinessCheck/RehearsalResult и записывает его в accepted_configs.digest.

Фактический SQL canonical_config содержит другой объект: PAID_ORDER_TASK_V1, разрешённые Price/List/Assignee/destination workspace IDs, source/destination connection, mode, required_context_keys, title_pattern, service_label. HIT-95 (`CaseReadStore.readCases`) требует `SHA256(JCS(canonical_config)) == accepted_configs.digest`. Сквозной тест с настоящими SQL/файлами/TLS подтверждает различие, а не только предположение по исходникам: [точные значения и запрос](DIGEST_DIAGNOSTIC.txt), [полный исходный прогон](history/integration-diagnostic-03.tar.gz).

## Уточнение после поручения пользователя

Проверен предпочтительный **вариант B**: входной accepted_config_digest должен совпадать с hash окончательного canonical_config, а не черновика. Конкретный недостаток принятого обмена — клиент до активации не получает ни этот hash, ни необходимые resolved IDs. Полное обоснование и проверяемый контрпример: [CONTRACT_COMPATIBILITY_RU.md](CONTRACT_COMPATIBILITY_RU.md). Это установленный разрыв, не общее предположение.

## Что нужно определить

Какой объект хеширует каждое из значений:

1. ReadinessCheck.config_digest и RehearsalResult.config_digest;
2. ActivationCommand.accepted_config_digest;
3. Operation.accepted_config_digest и accepted_configs.digest.

Исторически первоначально предложенный вариант A — сохранить существующий digest черновика в check/rehearsal/входной команде, вычислять accepted SQL/Operation digest по точному разрешённому canonical_config и явно связать оба hash, check/rehearsal/owner bindings и принятие в существующем durable журнале. Этот вариант **не утверждён и не применён**; после уточнения пользователя предпочтителен вариант B, см. проверку выше. Его достаточность для явного пользовательского принятия и клиентского договора должен подтвердить Lead. Другой вариант потребует пересмотра образования digest на этапе подготовки и затронет HIT-100/потребителей; делать это молча нельзя.

Ослаблять проверку digest в HIT-95, хранить произвольный alias/prose payload в SQL, менять formal SQL/OAS или выдавать новую конфигурацию за прежнюю запрещено. Схема API задаёт форму digest, но не снимает обнаруженное различие двух представлений. Связанная клиентская задача HIT-97 требует принадлежности команды/готовности/репетиции одной конфигурации и строгого чтения Operation, поэтому соответствие нельзя скрыть от её владельца.

До решения PR63 — Draft; Jira «В работе»; полный DoD/READY/приёмка отсутствуют. Запрос Lead не подменяется новым серверным запуском. После решения нужно сохранить ссылку на него, внести минимальное исправление, повторить затронутые компоненты и полный локальный интеграционный сценарий, обновить CI/результат/evidence и выполнить handoff-preflight.
