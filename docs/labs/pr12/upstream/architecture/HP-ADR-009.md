# HP-ADR-009 — Один wire owner, проверяемые проекции и пять совместимых workstreams

**Choice status:** Accepted по явной делегации архитектурных решений. **Source/baseline:** In Review / Candidate; human phase acceptance и Development admission не выполнены.

## 1. Контекст и решение

Пять разработчиков должны независимо начать Go/Python/Next части без пяти несовместимых API и догадок о схемах. Нужны полные wire определения, локальные behavioral norms и exact source package. ProjectOS unchanged PCF1.0.1 фактически поддерживает формальную проверку только JSONschema2020-12, хотя список format enums шире. Нельзя ни ухудшать архитектуру ради этого ограничения, ни объявлять неподдерживаемую OAS проверенной PCF.

## 2. Принятый вариант и rationale

Canonical wire source — OpenAPI3.1.0 узкого проверенного interop subset. В нём paths/methods/status/auth/schemas/operation IDs; Go strict server/client и TS types генерируются, Python strict request/response модели обязаны соответствовать той же форме. JSONschema2020-12 projection component payloads — deterministic generated view с exact `$defs` pointers, реальными Git/blob pins и schema validation. HTTP/authorization/behavior semantics не теряются: их полные OAS bytes остаются ordinary native source, проверяются отдельным OAS tool и source review, BP007 содержит общую transport семантику.

SQL owner files — единственный storage schema source; BP определяет транзакционное/временное поведение, не копирует columns. Stack manifest/locks задают одну tested starting selection. Overall manifest включает complete source closure; объект не получает произвольный mutable latest URL вместо реального commit.

Five workstreams используют versioned synthetic contract fixtures и bounded mock adapters до интеграции. Один integration owner меняет shared form, affected consumers получают cross-review. Разработка по сервисам не создаёт ложной независимости shared safety/security/data constraints. Последовательность: common contracts+identity gates→independent service work→first complete synthetic vertical slice→adversarial integration+permitted provider sandbox→product release/live gates. Это архитектурный integration plan, не выдуманный Jira technical backlog или календарное обещание.

## 3. Альтернативы

| Вариант | Компромисс/причина невыбора |
| --- | --- |
| Ручные структуры/Swagger после кода каждого сервиса | Drift null/required/errors/authority проявился бы поздно в пяти потоках |
| gRPC/protobuf для всех + отдельный public REST bridge | Дополнительный translation/runtime/tooling до доказанной необходимости; webhook/Next/AI уже естественно HTTP |
| OAS3.0.3 только из-за старого предположения о генераторе | Actual2.8.0 Go и TS/Python spike поддержали3.1 и null/absent при explicit nullable-type; старое предположение не основание |
| Полный OAS объявлен PCF formal_asset | Реальный валидатор возвращает E_FORMAL_UNSUPPORTED; создавать ложный PASS нельзя |
| JSONschema “HTTPcatalog” как подмена wire source | Payload validator не проверяет paths/status/auth; смысловая подмена создаёт второй несовместимый контракт |

## 4. Последствия

Нормативные последствия принадлежат HP-ARC-012/014/016, HP-BP-007/T/002/005/011/012, HP-BP-005/T/019/020, HP-FORMAL-001 и generated HP-ASSET-PAYLOADS-001. Go nullable-type option обязателен там, где wire различает null/absent. Смена TypeScript/генератора/Next требует clean builds+type bridge reassessment, а не peer suppression.

JSONschema structural PASS сам по себе не faithful review OAS/BP. Package/profile/source completeness/authenticity/accepted status различны. Coverage188rows и per-FR readiness должны соответствовать фактическим техническим источникам, не count-based “all done”.

## 5. Пересмотр/совместимость

Первое издание без supersession. Пересмотр при необходимости иного protocol, неустранимом codegen semantic conflict, новом implemented PCF formal support либо measured transport constraints. Старые frozen pins/queued messages/accepted configuration сохраняют действительность; breaking schema требует major path +migration, не тихого enum расширения старому strict reader. Обновление projector не меняет canonical wire owner.

## 6. Проверка

Actual bounded Go/Python/TS schema/build/browser checks и reproducible locks сохранены отдельно; это не implementation acceptance. Независимый review проверяет обе стороны source→projection, unsupported-format boundaries, actual commit/digests и соответствие selected Product closure. Самопроверка автора не independent PASS. Формальная приёмка остаётся отдельной действительной процедурой ProjectOS.
