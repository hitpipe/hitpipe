> ИСТОРИЯ ДО CCR. Описанный ниже пробел устранён утверждённой HP-CCR-ACTIVATION-20261116, редакцией2 задачи и реализацией текущего PR. Прежняя диагностика сохранена как основание решения, а не актуальное препятствие. См. [текущий отчёт](README.md) и [карту полей](FIELD_MAP.md).

# HIT-102 — проверка предпочтительного варианта B

Поручение пользователя в чате: проверить возможность самостоятельного исправления без ожидания Lead. Предпочтение — check/rehearsal сохраняют draft digest, а ActivationCommand/SQL/Operation используют один SHA-256 RFC8785 JCS окончательного canonical_config; нельзя имитировать подтверждение, если существующий договор не даёт получить этот хеш. Анализ выполнен Codex по делегации Тимофея Патрушева. **Вариант B не объявлен реализованным или согласованным: установлен конкретный отсутствующий предактивационный выход.**

## Что подтверждается источниками

SQL safe_config_valid и потребитель HIT-95 совместимы с точным хешем разрешённого canonical_config. Хранить там hash другого объекта неправильно: реальный интеграционный прогон уже дал READ_UNAVAILABLE. Раздельные draft и canonical hash можно связать в собственном durable журнале, не меняя форму SQL; это технически не требует ослаблять финансовые правила.

Сохранение check/rehearsal draft digest соответствует фактическому HIT-100. Source `draftjson.Codec.ConfigDigest` исключает name/draft_id/revision, но включает имеющиеся draft aliases и другие значимые поля. Private BindingProof отдельно удерживает фактические owner IDs. Проверка текущих Price/List/Assignee IDs уже отказывает при изменениях; после дополнительного поручения пользователя отдельные реальные негативные List и Assignee сценарии добавлены и пройдены в verification-06.

## Конкретный разрыв: подтверждающий клиент не получает окончательный digest

Прочитан единственный принятый OAS, SHA256 `a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e`, версия `fedbe40bb597e25f66aeb1172af3ff7459c81b40`. Ни один существующий предактивационный ответ не отдаёт окончательные canonical bytes/digest или необходимые для его вычисления разрешённые provider IDs:

| Доступный шаг/схема | Что возвращается | Чего нет |
|---|---|---|
|saveDraft/getDraft → DraftConfig|draft_id/revision/name, connections, aliases, mode/keys/title/service_label|resolved Price/List/Assignee/destination workspace IDs; canonical hash|
|checkDraftReadiness → ReadinessCheck|check_id, draft/version, **draft** config_digest, passed/issues, checked/expires|окончательный canonical_config или его digest, BindingProof|
|rehearseDraft → RehearsalResult|rehearsal_id, draft/version, **draft** config_digest, results/live_requests_made|окончательный canonical digest или resolved IDs|
|listConnections → Connections/Connection|connection_id/provider/mode/account_label/health/security_epoch/time/revision|provider account/destination workspace IDs и разрешённые параметры конфигурации|
|listConnectionOptions → Options/DestinationOption|alias/label/kind/available, complete/time/cursor|Price/List/Assignee ID или canonical digest|
|listOperations → Operation|digest уже выбранной принятой конфигурации|конфигурация нового ещё не принятого черновика; HIT-100 не создаёт operations/accepted_configs|

Все эти object schemas используют additionalProperties=false. Прочитаны все44 операции OAS; отдельного pre-activation canonical preview/resolve endpoint нет. DeliveryIntent/SourceFacts — внутренние сообщения/чтение уже допущенного дела, не пользовательский preview и не источник согласия до первой активации. Private BindingProof доступен Processing через owner port и свои файлы, но принятого BFF API для его чтения нет. Считать прямой доступ BFF/браузера к provider/чужой БД разрешённым нельзя.

Контрпример [JSON](history/digest-contract-counterexample.json) содержит два различных допустимых private canonical_config с разными Price IDs и одинаковыми OAS-видимыми DraftConfig/Options/ReadinessCheck/RehearsalResult. Четыре публичных представления проверены на соответствие точному OAS. Их canonical digest различается:

- `sha256:0ab14c402828f7fa61707770818f96c007dbfb118fc8fb4e3974fab82dd8f803`;
- `sha256:c1c1a833f7190825e9360126e71425b2036bed852863ad9e537d570cfde72dfe`.

Хеши вычислены реальной Go RFC8785 JCS библиотекой. [Исходник](history/digest_contract_counterexample.go.txt), [команда/cwd/env/source SHA/exits/digests](contract-check/commands.json), [проверка полей и схем](history/schema-gap-verification.json). Это **офлайн-синтетический контрпример договору**, не HTTP-запуск, не настоящий пользователь и не принятие. Он показывает отсутствие однозначного способа получить требуемый canonical digest из нынешних клиентских ответов.

Возвращённый после activation201 хеш приходит слишком поздно для предварительного явного подтверждения. Подстановка тестом данных из приватного BindingProof, кодирование хеша в check_id/label/error message или молчаливый переход на draft hash не создают согласованный пользовательский путь. Создание PREPARED operation/accepted config в readiness для обхода проблемы изменило бы принятый HIT-100 и его проверенное отсутствие активационных записей.

## Минимальный вопрос Lead

**Из какого принятого предактивационного ответа BFF/человек должен получать SHA-256 RFC8785 JCS окончательного canonical_config для ActivationCommand.accepted_config_digest?** Если такой источник уже существует, нужна точная ссылка и версия. Если его нет, требуется согласовать минимальный способ выдачи canonical digest/preview, связанный с теми же draft revision, readiness, rehearsal, BindingProof и scope/epoch; либо явно выбрать иной договор соответствия входного digest и окончательной SQL-конфигурации. Предпочтение пользователя — вариант B.

До этого не меняются formal SQL/OAS, HIT-95 или чужие сервисы. Не выдаются успешная интеграция, пользовательское согласие, все8 DoD или READY. Текущий код PR63 остаётся незавершённым и Draft; документация и результаты прежних запусков сохранены. После решения необходимы минимальное исправление, новые фактические проверки положительного BFF→activation→Intake→HIT-95 пути и прежних отрицательных сценариев, новый CI, регистрация результата/evidence и handoff-preflight.
