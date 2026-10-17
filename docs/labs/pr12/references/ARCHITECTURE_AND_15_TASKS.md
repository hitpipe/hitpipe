# Внутренняя структура и 15 начальных задач HitPipe

План; студенческий код и его проверки ещё не выполнены. Canonical machine-readable companion: ARCHITECTURE_AND_15_TASKS.json. Фактические Jira keys и порядок — manifest.json/tasks/.

Architecture Candidate: sha256:18f94e1cfb6ba4f98cdd0c09133e798ad619d8fa83a74af13c017ca1f0ee418b; Product Candidate: sha256:e2a5ab63b35ce807c4de7bd79b30858fa0e54161eae252d022882fc8ceb7cd71. Выбран HP-VER-001; formal inputs из hitpipe/project-os@84249ff3ba611358232294c46f6022f030ea223d. Candidate не означает Accepted/admitted.

Внутренняя конвенция: domain ← application (порты потребителя) ← adapters; composition root cmd/bootstrap. Это конкретизация структуры кода в HP-DELEG-001 AUTO, не новая архитектура или изменение wire/data/authority. Создавать только используемые слои. Полное правило — IMPLEMENTATION_CONVENTION.md.

Диагностические CLI сохраняются рядом с реальными policy modules и вызывают тот же production code. Synthetic fixtures и unit checks не доказывают полный runtime, provider access, durability или готовность продукта.

- issue_execution_kind: Native Epic10000 → Feature10047 → Sub-task10080. Project OS jira.py maps physical Subtask to logical Task. IMP/VAL execution_kind is carried as task metadata; no Jira global hierarchy change.
- authority: Manual educational/user-authorized slice using exact Candidate sources; not Project OS formal acceptance or Development admission.
- screenshots: HUMAN_CAPTURE_REQUIRED only. Agent prepares exact command/version/caption and empty placeholder; never generates screenshots, captures browser/terminal images or fabricates human actions.
- mutations: At least5 compiling defects, one at a time in temporary branch/worktree, baselinePASS, actual detected/missed counts plus natural defects links, improve missed tests and rerun, restore clean code; no deliberately false initial results.
- evidence: PASS only for actually executed checks at exact commit; FAIL/BLOCKED/NOT RUN explicit. PR review/merge and required student/customer interviews remain real humans.
- future_reuse: Keep code in owner component paths and immutable Jira key; later move feature/task parent or link to future owner epic, preserve commits, test IDs, evidence and requirement refs. No recoding required.

## DK-C1 · IAM: реализовать session policy и воспроизводимый диагностический компонент

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Минимальный сохраняемый компонент и собственная содержательная rule-логика; не реализовать весь сервис.

Владелец: Дмитрий Карачебан. PRIMARY: `services/iam/internal/domain/sessionpolicy`, `services/iam/docs/module-contract.md`, `services/iam/go.mod`, `services/iam/go.sum`, `services/iam/cmd/iam-contractcheck`, `services/iam/testdata`, `services/iam/Makefile`.

Зависимости: общее подготовленное окружение.

Нормы: HP-BP-001/T/002, HP-BP-001/T/003, HP-BP-001/T/013, HP-BP-007/T/005.

Критерии:

- Функция принимает snapshot session и явное trusted now, возвращает typed valid/denied с безопасным reason; не читает сеть/БД/окружение и не создает business grant.
- Unknown, revoked, equality/overflow/malformed timestamps и clock uncertainty явно отклоняются; пароль/OIDC не реализуются как локальный shortcut.
- CLI iam-contractcheck использует тот же production policy module, не печатает secret/session digest/PII и работает с synthetic fixtures; обычный и ошибочный запуск, version/SHA, format/path config воспроизводимы.
- Минимальный компонент: конфигурация влияет на вывод/выбор fixture; неверная config → явная ошибка/nonzero; --version показывает version+commit+dirty+buildmode; старт/исход диагностируются без входных данных.
- Dev/release builds реально сравниваются на одном scenario; release artifact запускается из каталога вне checkout с явно перечисленными dependencies; binary/module inspection фиксирует факты.
- Семь позиций контура описаны с actual работает/запланировано и маршрутом: build/deps/test/static/dynamic/profiling/CI. Общий stackADR переиспользуется, студент объясняет свой участок.

Проверяемые сценарии:

- valid just before both deadlines
- idle deadline -1ns / exact / +1ns
- absolute deadline -1ns / exact / +1ns
- revoked despite future expiry
- unknown session
- same policy on store failure and clock failure
- malformed chronology; immutable input

Пределы доказательств:

- Не проверяется Keycloak OIDC/code replay
- Не проверяются фактическая tenant authorization и whole-roster revoke
- Fixture store не доказывает SQL/RLS/durability

Будущий эпик компонента: Identity & Access / Platform.

## DK-C2 · IAM: подключить AuthenticateSession через SessionLookup и Clock ports

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Подключенный production application consumer, narrow dependency ports, реальные файловые diagnostic adapters и изолированные synthetic doubles.

Владелец: Дмитрий Карачебан. PRIMARY: `services/iam/internal/domain/sessionpolicy`, `services/iam/docs/module-contract.md`, `services/iam/internal/application/authenticatesession`, `services/iam/internal/adapters/fixtures/sessionlookup`, `services/iam/internal/adapters/clock/system`.

Зависимости: DK-C1.

Нормы: HP-BP-001/T/002, HP-BP-001/T/003, HP-BP-001/T/013, HP-BP-007/T/005.

Критерии:

- Один application consumer получает SessionLookup и Clock через constructor; реальные OIDC/session mutation/runtime barrier остаются явно вне этой порции.
- File-backed synthetic snapshot adapter и test double подключаются в одном composition root без изменения consumer; ограничения обоих зафиксированы.
- Неверная/неустановленная dependency, неизвестная session и store/clock error дают denied/unavailable, никогда default allow; публичный production auth endpoint не открывается.
- Документация module contract описывает purpose/input/output/errors/ownership/side-effects/error precedence и пример; Godoc либо TSDoc соответствует выбранному стеку.
- DI contract test запускает штатный, ошибочный и граничный сценарий с обеими реализациями; одна точка сборки, consumer unchanged; ограничения подмены указаны явно.
- Запись собственного модуля в карте с фактическими import arrows, статусом и автором; две системные сложности, существующий module-candidate и собственный local implementation ADR с options/cost/check; не создавать второй HP-ADR-001.

Проверяемые сценарии:

- valid just before both deadlines
- idle deadline -1ns / exact / +1ns
- absolute deadline -1ns / exact / +1ns
- revoked despite future expiry
- unknown session
- same policy on store failure and clock failure
- malformed chronology; immutable input

Пределы доказательств:

- Не проверяется Keycloak OIDC/code replay
- Не проверяются фактическая tenant authorization и whole-roster revoke
- Fixture store не доказывает SQL/RLS/durability

Будущий эпик компонента: Identity & Access / Platform.

## DK-C3 · IAM: проверить session boundaries, отказ и чувствительность security regression suite

Репозиторий: `hitpipe/hitpipe`; задача: VAL. Система проверок собственного настоящего module, coverage, CI и reproducible mutation experiment; без внесения mutant code в main.

Владелец: Дмитрий Карачебан. PRIMARY: `services/iam/internal/domain/sessionpolicy`, `services/iam/docs/module-contract.md`, `services/iam/**/*_test.go`, `services/iam/testdata`, `services/iam/tools/mutations`, `services/iam/docs/test-design.md`, `.github/workflows/pr12-dk.yml`.

Зависимости: DK-C2.

Нормы: HP-BP-001/T/002, HP-BP-001/T/003, HP-BP-001/T/013, HP-BP-007/T/005.

Критерии:

- Test oracle записан до запуска из указанных правил, а не скопирован из actual output. Не менее двух техник: equivalence classes+boundary; decision/state table по объекту. Включены normal/error/boundary cases.
- Реальные local+CI прогоны exact submission commit; CI запускает собственные названные тесты и падает при нарушении assertion. Dataset synthetic/fixed seed, clean-clone command без secret/private fixture.
- Coverage получен для собственного module, выявлен конкретный missed branch либо непроверяемое свойство и принято решение; одного процента недостаточно.
- Не менее5 compiling seeded defects поодиночке с actual protocol; missed → explanation+new test+rerun; исходный и финальный clean runsPASS, естественные defects учтены честно.
- Risk registry qualitative probability×impact с rationale и security risk меняет selection/frequency/manual/deferred checks; fake fullE2E/provider/acceptance claims отсутствуют.
- Human screenshot placeholders содержат exact command/revision/expected visible evidence; агент исполняет проверки и сохраняет журналы, человек делает только реальные снимки. Human review и чистое independent reproduction остаются явным gate.
- Локальный отрицательный прогон ТОЙ ЖЕ содержательной CI-проверки: намеренно нарушить проверяемое условие → фактический ненулевой exit/FAIL и журнал → восстановить исправный код/условие → повторный PASS. Сохранены exact argv, SHA, patch/digest, exit/log paths и human instruction для рисунка7 МДК01.04 ПР1; мутация/ошибочное условие не попадают в main.

Проверяемые сценарии:

- valid just before both deadlines
- idle deadline -1ns / exact / +1ns
- absolute deadline -1ns / exact / +1ns
- revoked despite future expiry
- unknown session
- same policy on store failure and clock failure
- malformed chronology; immutable input

Предложенные мутации (это ещё не результаты эксперимента):

- idle >= заменить >
- absolute expiry check удалить
- revoked check удалить
- unknown session разрешить
- clock error заменить valid default
- использовать только idle и проигнорировать absolute

Пределы доказательств:

- Не проверяется Keycloak OIDC/code replay
- Не проверяются фактическая tenant authorization и whole-roster revoke
- Fixture store не доказывает SQL/RLS/durability

Будущий эпик компонента: Identity & Access / Platform.

## TP-C1 · Processing: реализовать deterministic paid-order eligibility evaluator

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Минимальный сохраняемый компонент и собственная содержательная rule-логика; не реализовать весь сервис.

Владелец: Тимофей Патрушев. PRIMARY: `services/processing/internal/domain/eligibility`, `services/processing/docs/module-contract.md`, `services/processing/go.mod`, `services/processing/go.sum`, `services/processing/cmd/processing-contractcheck`, `services/processing/testdata`, `services/processing/Makefile`.

Зависимости: общее подготовленное окружение.

Нормы: HP-RULE-006, HP-BP-002/T/004, HP-BP-003/T/002, HP-BP-003/T/006, HP-BP-007/T/005.

Критерии:

- Публичный контракт фиксирует VALID/INELIGIBLE/INCOMPLETE/REFRESH_REQUIRED либо эквивалентные local typed results без изменения wire enum; reason codes и порядок ошибок однозначны.
- Все нормативные paid/capture/receive/refund/dispute/currency/qty/price/account/mode predicates присутствуют, null/unknown остаются отдельным случаем.
- CLI processing-contractcheck исполняет тот же evaluator по synthetic files; accepted config read-only, нет admission, новой config, записи case или внешнего effect.
- Минимальный компонент: конфигурация влияет на вывод/выбор fixture; неверная config → явная ошибка/nonzero; --version показывает version+commit+dirty+buildmode; старт/исход диагностируются без входных данных.
- Dev/release builds реально сравниваются на одном scenario; release artifact запускается из каталога вне checkout с явно перечисленными dependencies; binary/module inspection фиксирует факты.
- Семь позиций контура описаны с actual работает/запланировано и маршрутом: build/deps/test/static/dynamic/profiling/CI. Общий stackADR переиспользуется, студент объясняет свой участок.

Проверяемые сценарии:

- complete valid fixed Price qty1
- zero/partial/captured-false/refund-positive/disputed
- null financial fields vs explicit false/zero
- wrong account/mode/Price/qty/currency
- activation boundary -1ns / exact / +1ns
- proof oldest observation 10s boundary and expired valid_until
- complete=false or partial pagination
- reordered facts/input immutability

Пределы доказательств:

- Нет фактических Stripe API facts
- Нет Processing SQL admission/config pin transaction
- ALLOWED не подтверждает side-effect permission/readback

Будущий эпик компонента: Operations Processing.

## TP-C2 · Processing: подключить evaluator к EvaluatePaidOrder через SourceFactsReader

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Подключенный production application consumer, narrow dependency ports, реальные файловые diagnostic adapters и изолированные synthetic doubles.

Владелец: Тимофей Патрушев. PRIMARY: `services/processing/internal/domain/eligibility`, `services/processing/docs/module-contract.md`, `services/processing/internal/application/evaluatepaidorder`, `services/processing/internal/adapters/fixtures/sourcefacts`.

Зависимости: TP-C1.

Нормы: HP-RULE-006, HP-BP-002/T/004, HP-BP-003/T/002, HP-BP-003/T/006, HP-BP-007/T/005.

Критерии:

- Application consumer получает immutable context+SourceFactsReader; file fixture adapter и test double воспроизводят один consumer scenario без изменения consumer.
- Принятый snapshot/config identity сверяется; несовместимая или неполная версия удерживает действие; exception reader не считается unpaid/zero.
- Result прямо обозначает только financial eligibility: case lifecycle/actor/epoch/lineage/fresh dispatch проверяются будущим owner-path, в этой задаче не симулируются PASS.
- Документация module contract описывает purpose/input/output/errors/ownership/side-effects/error precedence и пример; Godoc либо TSDoc соответствует выбранному стеку.
- DI contract test запускает штатный, ошибочный и граничный сценарий с обеими реализациями; одна точка сборки, consumer unchanged; ограничения подмены указаны явно.
- Запись собственного модуля в карте с фактическими import arrows, статусом и автором; две системные сложности, существующий module-candidate и собственный local implementation ADR с options/cost/check; не создавать второй HP-ADR-001.

Проверяемые сценарии:

- complete valid fixed Price qty1
- zero/partial/captured-false/refund-positive/disputed
- null financial fields vs explicit false/zero
- wrong account/mode/Price/qty/currency
- activation boundary -1ns / exact / +1ns
- proof oldest observation 10s boundary and expired valid_until
- complete=false or partial pagination
- reordered facts/input immutability

Пределы доказательств:

- Нет фактических Stripe API facts
- Нет Processing SQL admission/config pin transaction
- ALLOWED не подтверждает side-effect permission/readback

Будущий эпик компонента: Operations Processing.

## TP-C3 · Processing: проверить financial decision table, границы proof и seeded defects

Репозиторий: `hitpipe/hitpipe`; задача: VAL. Система проверок собственного настоящего module, coverage, CI и reproducible mutation experiment; без внесения mutant code в main.

Владелец: Тимофей Патрушев. PRIMARY: `services/processing/internal/domain/eligibility`, `services/processing/docs/module-contract.md`, `services/processing/**/*_test.go`, `services/processing/testdata`, `services/processing/tools/mutations`, `services/processing/docs/test-design.md`, `.github/workflows/pr12-tp.yml`.

Зависимости: TP-C2.

Нормы: HP-RULE-006, HP-BP-002/T/004, HP-BP-003/T/002, HP-BP-003/T/006, HP-BP-007/T/005.

Критерии:

- Test oracle записан до запуска из указанных правил, а не скопирован из actual output. Не менее двух техник: equivalence classes+boundary; decision/state table по объекту. Включены normal/error/boundary cases.
- Реальные local+CI прогоны exact submission commit; CI запускает собственные названные тесты и падает при нарушении assertion. Dataset synthetic/fixed seed, clean-clone command без secret/private fixture.
- Coverage получен для собственного module, выявлен конкретный missed branch либо непроверяемое свойство и принято решение; одного процента недостаточно.
- Не менее5 compiling seeded defects поодиночке с actual protocol; missed → explanation+new test+rerun; исходный и финальный clean runsPASS, естественные defects учтены честно.
- Risk registry qualitative probability×impact с rationale и security risk меняет selection/frequency/manual/deferred checks; fake fullE2E/provider/acceptance claims отсутствуют.
- Human screenshot placeholders содержат exact command/revision/expected visible evidence; агент исполняет проверки и сохраняет журналы, человек делает только реальные снимки. Human review и чистое independent reproduction остаются явным gate.
- Локальный отрицательный прогон ТОЙ ЖЕ содержательной CI-проверки: намеренно нарушить проверяемое условие → фактический ненулевой exit/FAIL и журнал → восстановить исправный код/условие → повторный PASS. Сохранены exact argv, SHA, patch/digest, exit/log paths и human instruction для рисунка7 МДК01.04 ПР1; мутация/ошибочное условие не попадают в main.

Проверяемые сценарии:

- complete valid fixed Price qty1
- zero/partial/captured-false/refund-positive/disputed
- null financial fields vs explicit false/zero
- wrong account/mode/Price/qty/currency
- activation boundary -1ns / exact / +1ns
- proof oldest observation 10s boundary and expired valid_until
- complete=false or partial pagination
- reordered facts/input immutability

Предложенные мутации (это ещё не результаты эксперимента):

- amount_refunded==0 check удалить
- charge_captured check удалить
- quantity==1 заменить >=1
- null disputed привести к false
- currency equality проигнорировать
- 10s freshness > заменить <

Пределы доказательств:

- Нет фактических Stripe API facts
- Нет Processing SQL admission/config pin transaction
- ALLOWED не подтверждает side-effect permission/readback

Будущий эпик компонента: Operations Processing.

## MG-C1 · UX: реализовать CasePresentation и диагностический TS-компонент

Репозиторий: `hitpipe/hitpipe_client`; задача: IMP. Минимальный сохраняемый компонент и собственная содержательная rule-логика; не реализовать весь сервис.

Владелец: Матвей Головачев. PRIMARY: `src/features/case-status/domain`, `src/features/case-status/docs/module-contract.md`, `src/features/case-status/testdata`, `tools/pr12/case-contractcheck.ts`.

PRIMARY includes tools/pr12/case-contractcheck.ts and owner src/features/case-status paths; tooling/pr12/frontend/{package.json,package-lock.json,tsconfig.json} RELATED_BY_LEAD_ONLY; root legacy manifests DO NOT TOUCH. Own .github/workflows/pr12-mg.yml may be extended only by its C3 owner.

Зависимости: общее подготовленное окружение.

Нормы: HP-RULE-002, HP-RULE-007, HP-RULE-008, HP-BP-005/T/010, HP-BP-005/T/013, HP-BP-005/T/014, HP-BP-005/T/015, HP-BP-005/T/019.

Критерии:

- Pure TS projection не импортирует React/Next/fetch/process и не создает девятое case state; отображает authoritative state+knowledge+timestamps без hidden inference.
- Неполный/невалидный boundary JSON отклоняется runtime validator по frozen OAS; generated types не объявляются validator и не создается вторая ручная wire schema.
- CLI case-contractcheck собирается в release JS под pinned Node/TS, принимает --fixture/--format, выводит version/SHA и безопасную summary; тот же core используется React consumer.
- Минимальный компонент: конфигурация влияет на вывод/выбор fixture; неверная config → явная ошибка/nonzero; --version показывает version+commit+dirty+buildmode; старт/исход диагностируются без входных данных.
- Dev/release builds реально сравниваются на одном scenario; release artifact запускается из каталога вне checkout с явно перечисленными dependencies; binary/module inspection фиксирует факты.
- Семь позиций контура описаны с actual работает/запланировано и маршрутом: build/deps/test/static/dynamic/profiling/CI. Общий stackADR переиспользуется, студент объясняет свой участок.

Проверяемые сценарии:

- all8canonical states table
- UNKNOWN/POSSIBLE_EFFECT cannot show success or retry-create
- 202/pending transport not business confirmation
- late lower revision ignored
- equal/newer revision behavior
- failed refresh keeps last knowledge+stale marker
- workspace switch drops late old result
- null last_observation_at differs current UI read time
- malicious markup renders escaped text

Пределы доказательств:

- Fixture source не доказывает IAM/BFF authorization
- Unit projection не доказывает полные accessibility/user study gates
- Python/model preparation и prod custom server revoke не входят в эту порцию

Будущий эпик компонента: Experience & AI Preparation.

## MG-C2 · UX: подключить CasePresenter и доступную CaseStatusCard через CaseSnapshotSource

Репозиторий: `hitpipe/hitpipe_client`; задача: IMP. Подключенный production application consumer, narrow dependency ports, реальные файловые diagnostic adapters и изолированные synthetic doubles.

Владелец: Матвей Головачев. PRIMARY: `src/features/case-status/domain`, `src/features/case-status/docs/module-contract.md`, `src/features/case-status/application`, `src/features/case-status/adapters/fixtures`, `src/features/case-status/components/CaseStatusCard.tsx`.

PRIMARY includes tools/pr12/case-contractcheck.ts and owner src/features/case-status paths; tooling/pr12/frontend/{package.json,package-lock.json,tsconfig.json} RELATED_BY_LEAD_ONLY; root legacy manifests DO NOT TOUCH. Own .github/workflows/pr12-mg.yml may be extended only by its C3 owner.

Зависимости: MG-C1.

Нормы: HP-RULE-002, HP-RULE-007, HP-RULE-008, HP-BP-005/T/010, HP-BP-005/T/013, HP-BP-005/T/014, HP-BP-005/T/015, HP-BP-005/T/019.

Критерии:

- CasePresenter принимает CaseSnapshotSource; file fixture source и controlled test double переключаются в одной точке сборки, core/consumer не переписываются.
- React CaseStatusCard отображает viewmodel с семантическими headings/labels/status, escaped text, видимыми unknown/stale/last-observation markers; protected fetch отсутствует в domain/component.
- Known revision не понижается поздним ответом; workspace change/logout инвалидируют pending result binding; synthetic view явно обозначена fixture и не выдается за live app.
- Документация module contract описывает purpose/input/output/errors/ownership/side-effects/error precedence и пример; Godoc либо TSDoc соответствует выбранному стеку.
- DI contract test запускает штатный, ошибочный и граничный сценарий с обеими реализациями; одна точка сборки, consumer unchanged; ограничения подмены указаны явно.
- Запись собственного модуля в карте с фактическими import arrows, статусом и автором; две системные сложности, существующий module-candidate и собственный local implementation ADR с options/cost/check; не создавать второй HP-ADR-001.

Проверяемые сценарии:

- all8canonical states table
- UNKNOWN/POSSIBLE_EFFECT cannot show success or retry-create
- 202/pending transport not business confirmation
- late lower revision ignored
- equal/newer revision behavior
- failed refresh keeps last knowledge+stale marker
- workspace switch drops late old result
- null last_observation_at differs current UI read time
- malicious markup renders escaped text

Пределы доказательств:

- Fixture source не доказывает IAM/BFF authorization
- Unit projection не доказывает полные accessibility/user study gates
- Python/model preparation и prod custom server revoke не входят в эту порцию

Будущий эпик компонента: Experience & AI Preparation.

## MG-C3 · UX: проверить revision/freshness/scope boundaries и seeded UI defects

Репозиторий: `hitpipe/hitpipe_client`; задача: VAL. Система проверок собственного настоящего module, coverage, CI и reproducible mutation experiment; без внесения mutant code в main.

Владелец: Матвей Головачев. PRIMARY: `src/features/case-status/domain`, `src/features/case-status/docs/module-contract.md`, `src/features/case-status/**/*.test.ts`, `tools/mutations/case-status`, `tests/browser/case-status.spec.ts`, `.github/workflows/pr12-mg.yml`.

PRIMARY includes tools/pr12/case-contractcheck.ts and owner src/features/case-status paths; tooling/pr12/frontend/{package.json,package-lock.json,tsconfig.json} RELATED_BY_LEAD_ONLY; root legacy manifests DO NOT TOUCH. Own .github/workflows/pr12-mg.yml may be extended only by its C3 owner.

Зависимости: MG-C2.

Нормы: HP-RULE-002, HP-RULE-007, HP-RULE-008, HP-BP-005/T/010, HP-BP-005/T/013, HP-BP-005/T/014, HP-BP-005/T/015, HP-BP-005/T/019.

Критерии:

- Test oracle записан до запуска из указанных правил, а не скопирован из actual output. Не менее двух техник: equivalence classes+boundary; decision/state table по объекту. Включены normal/error/boundary cases.
- Реальные local+CI прогоны exact submission commit; CI запускает собственные названные тесты и падает при нарушении assertion. Dataset synthetic/fixed seed, clean-clone command без secret/private fixture.
- Coverage получен для собственного module, выявлен конкретный missed branch либо непроверяемое свойство и принято решение; одного процента недостаточно.
- Не менее5 compiling seeded defects поодиночке с actual protocol; missed → explanation+new test+rerun; исходный и финальный clean runsPASS, естественные defects учтены честно.
- Risk registry qualitative probability×impact с rationale и security risk меняет selection/frequency/manual/deferred checks; fake fullE2E/provider/acceptance claims отсутствуют.
- Human screenshot placeholders содержат exact command/revision/expected visible evidence; агент исполняет проверки и сохраняет журналы, человек делает только реальные снимки. Human review и чистое independent reproduction остаются явным gate.
- Локальный отрицательный прогон ТОЙ ЖЕ содержательной CI-проверки: намеренно нарушить проверяемое условие → фактический ненулевой exit/FAIL и журнал → восстановить исправный код/условие → повторный PASS. Сохранены exact argv, SHA, patch/digest, exit/log paths и human instruction для рисунка7 МДК01.04 ПР1; мутация/ошибочное условие не попадают в main.

Проверяемые сценарии:

- all8canonical states table
- UNKNOWN/POSSIBLE_EFFECT cannot show success or retry-create
- 202/pending transport not business confirmation
- late lower revision ignored
- equal/newer revision behavior
- failed refresh keeps last knowledge+stale marker
- workspace switch drops late old result
- null last_observation_at differs current UI read time
- malicious markup renders escaped text

Предложенные мутации (это ещё не результаты эксперимента):

- UNKNOWN label заменить Confirmed
- 202 принять как business success
- revision guard удалить
- workspace binding guard удалить
- failed refresh заменить empty success
- available_actions дополнить generic retry

Пределы доказательств:

- Fixture source не доказывает IAM/BFF authorization
- Unit projection не доказывает полные accessibility/user study gates
- Python/model preparation и prod custom server revoke не входят в эту порцию

Будущий эпик компонента: Experience & AI Preparation.

## DS-C1 · Delivery: реализовать exact target-readback comparator

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Минимальный сохраняемый компонент и собственная содержательная rule-логика; не реализовать весь сервис.

Владелец: Дмитрий Стацюк. PRIMARY: `services/delivery/internal/domain/readback`, `services/delivery/docs/module-contract.md`, `services/delivery/go.mod`, `services/delivery/go.sum`, `services/delivery/cmd/delivery-contractcheck`, `services/delivery/testdata`, `services/delivery/Makefile`.

Зависимости: общее подготовленное окружение.

Нормы: HP-RULE-007, HP-RULE-008, HP-BP-004/T/007, HP-BP-004/T/008, HP-BP-004/T/013, HP-BP-007/T/005.

Критерии:

- Comparator использует normalized NFC/newline semantics BP004/T013, exact expected title/block/Workspace/List и наличие заданного assignee.
- Missing/null/несколько delimiter-pairs/incomplete observation возвращают typed incomplete/mismatch; нет fuzzy matching, taskID-only success или known-no-effect из отсутствия объекта.
- CLI delivery-contractcheck вызывает тот же comparator по synthetic expectation/observation; нет token, произвольного fetch URL или provider POST.
- Минимальный компонент: конфигурация влияет на вывод/выбор fixture; неверная config → явная ошибка/nonzero; --version показывает version+commit+dirty+buildmode; старт/исход диагностируются без входных данных.
- Dev/release builds реально сравниваются на одном scenario; release artifact запускается из каталога вне checkout с явно перечисленными dependencies; binary/module inspection фиксирует факты.
- Семь позиций контура описаны с actual работает/запланировано и маршрутом: build/deps/test/static/dynamic/profiling/CI. Общий stackADR переиспользуется, студент объясняет свой участок.

Проверяемые сценарии:

- exact allmatches
- each workspace/list/title/marker/context mismatch independently
- expected assignee among extra external assignees
- null inaccessible workspace or block
- missing/duplicate/malformed handoff delimiters
- NFC/decomposed Unicode and CRLF equivalence
- meaningful whitespace mismatch
- taskID-only and missing retrieval evidence
- size boundaries32KiB context/36KiB block from normative serialization

Пределы доказательств:

- Saved observation не подтверждает настоящий ClickUp GET/permissions
- Нет effect ledger/fence/one-shot provider transport verification
- Нет окончательного business state transition/confirmation

Будущий эпик компонента: Delivery & Destination Connectors.

## DS-C2 · Delivery: подключить VerifyTarget через TargetReader без provider writes

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Подключенный production application consumer, narrow dependency ports, реальные файловые diagnostic adapters и изолированные synthetic doubles.

Владелец: Дмитрий Стацюк. PRIMARY: `services/delivery/internal/domain/readback`, `services/delivery/docs/module-contract.md`, `services/delivery/internal/application/verifytarget`, `services/delivery/internal/adapters/fixtures/targetreader`.

Зависимости: DS-C1.

Нормы: HP-RULE-007, HP-RULE-008, HP-BP-004/T/007, HP-BP-004/T/008, HP-BP-004/T/013, HP-BP-007/T/005.

Критерии:

- Один VerifyTarget application consumer вызывает TargetReader; file-backed saved-observation adapter и synthetic test double взаимозаменяемы без изменения consumer.
- Сохраняются observed values/time/provenance в returned result; consumer не объявляет Processing Confirmed и не эмитит durable message без отдельного canonical protocol.
- Read error/404/expired observation не доказывают no-effect, не разрешают новое создание; любые дальнейшие provider adapters требуют отдельно common gate/one-shot policy.
- Документация module contract описывает purpose/input/output/errors/ownership/side-effects/error precedence и пример; Godoc либо TSDoc соответствует выбранному стеку.
- DI contract test запускает штатный, ошибочный и граничный сценарий с обеими реализациями; одна точка сборки, consumer unchanged; ограничения подмены указаны явно.
- Запись собственного модуля в карте с фактическими import arrows, статусом и автором; две системные сложности, существующий module-candidate и собственный local implementation ADR с options/cost/check; не создавать второй HP-ADR-001.

Проверяемые сценарии:

- exact allmatches
- each workspace/list/title/marker/context mismatch independently
- expected assignee among extra external assignees
- null inaccessible workspace or block
- missing/duplicate/malformed handoff delimiters
- NFC/decomposed Unicode and CRLF equivalence
- meaningful whitespace mismatch
- taskID-only and missing retrieval evidence
- size boundaries32KiB context/36KiB block from normative serialization

Пределы доказательств:

- Saved observation не подтверждает настоящий ClickUp GET/permissions
- Нет effect ledger/fence/one-shot provider transport verification
- Нет окончательного business state transition/confirmation

Будущий эпик компонента: Delivery & Destination Connectors.

## DS-C3 · Delivery: проверить readback mismatch/incomplete и seeded false-confirmation defects

Репозиторий: `hitpipe/hitpipe`; задача: VAL. Система проверок собственного настоящего module, coverage, CI и reproducible mutation experiment; без внесения mutant code в main.

Владелец: Дмитрий Стацюк. PRIMARY: `services/delivery/internal/domain/readback`, `services/delivery/docs/module-contract.md`, `services/delivery/**/*_test.go`, `services/delivery/testdata`, `services/delivery/tools/mutations`, `services/delivery/docs/test-design.md`, `.github/workflows/pr12-ds.yml`.

Зависимости: DS-C2.

Нормы: HP-RULE-007, HP-RULE-008, HP-BP-004/T/007, HP-BP-004/T/008, HP-BP-004/T/013, HP-BP-007/T/005.

Критерии:

- Test oracle записан до запуска из указанных правил, а не скопирован из actual output. Не менее двух техник: equivalence classes+boundary; decision/state table по объекту. Включены normal/error/boundary cases.
- Реальные local+CI прогоны exact submission commit; CI запускает собственные названные тесты и падает при нарушении assertion. Dataset synthetic/fixed seed, clean-clone command без secret/private fixture.
- Coverage получен для собственного module, выявлен конкретный missed branch либо непроверяемое свойство и принято решение; одного процента недостаточно.
- Не менее5 compiling seeded defects поодиночке с actual protocol; missed → explanation+new test+rerun; исходный и финальный clean runsPASS, естественные defects учтены честно.
- Risk registry qualitative probability×impact с rationale и security risk меняет selection/frequency/manual/deferred checks; fake fullE2E/provider/acceptance claims отсутствуют.
- Human screenshot placeholders содержат exact command/revision/expected visible evidence; агент исполняет проверки и сохраняет журналы, человек делает только реальные снимки. Human review и чистое independent reproduction остаются явным gate.
- Локальный отрицательный прогон ТОЙ ЖЕ содержательной CI-проверки: намеренно нарушить проверяемое условие → фактический ненулевой exit/FAIL и журнал → восстановить исправный код/условие → повторный PASS. Сохранены exact argv, SHA, patch/digest, exit/log paths и human instruction для рисунка7 МДК01.04 ПР1; мутация/ошибочное условие не попадают в main.

Проверяемые сценарии:

- exact allmatches
- each workspace/list/title/marker/context mismatch independently
- expected assignee among extra external assignees
- null inaccessible workspace or block
- missing/duplicate/malformed handoff delimiters
- NFC/decomposed Unicode and CRLF equivalence
- meaningful whitespace mismatch
- taskID-only and missing retrieval evidence
- size boundaries32KiB context/36KiB block from normative serialization

Предложенные мутации (это ещё не результаты эксперимента):

- workspace comparison удалить
- assignee membership заменить any-assignee
- whole-block equality заменить prefix match
- duplicate block принять первый
- incomplete observation назвать verified
- ошибку reader трактовать known-no-effect

Пределы доказательств:

- Saved observation не подтверждает настоящий ClickUp GET/permissions
- Нет effect ledger/fence/one-shot provider transport verification
- Нет окончательного business state transition/confirmation

Будущий эпик компонента: Delivery & Destination Connectors.

## NB-C1 · Intake: реализовать строгую проверку Stripe raw-body signatures

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Минимальный сохраняемый компонент и собственная содержательная rule-логика; не реализовать весь сервис.

Владелец: Никита Будыкин. PRIMARY: `services/intake/internal/domain/stripesignature`, `services/intake/docs/module-contract.md`, `services/intake/go.mod`, `services/intake/go.sum`, `services/intake/cmd/intake-contractcheck`, `services/intake/testdata`, `services/intake/Makefile`.

Зависимости: общее подготовленное окружение.

Нормы: HP-BP-002/T/001, HP-BP-002/T/002, HP-BP-002/T/010, HP-BP-007/T/005.

Критерии:

- Строгий parser signature header однозначно обрабатывает v1, timestamps и malformed/duplicate contradictory timestamp cases, затем constant-time HMAC compare по неизмененным bytes.
- Точность300s соблюдается для прошлых и будущих timestamps; tolerance не отключается параметром0, secret/body/header не пишутся в diagnostics.
- CLI intake-contractcheck читает synthetic raw-body/header/secret fixture, выводит только safe verdict/reason/version; status VERIFIED не называется ACCEPTED/paid.
- Минимальный компонент: конфигурация влияет на вывод/выбор fixture; неверная config → явная ошибка/nonzero; --version показывает version+commit+dirty+buildmode; старт/исход диагностируются без входных данных.
- Dev/release builds реально сравниваются на одном scenario; release artifact запускается из каталога вне checkout с явно перечисленными dependencies; binary/module inspection фиксирует факты.
- Семь позиций контура описаны с actual работает/запланировано и маршрутом: build/deps/test/static/dynamic/profiling/CI. Общий stackADR переиспользуется, студент объясняет свой участок.

Проверяемые сценарии:

- official-style known expected HMAC vector fixed offline
- same bytes success/onebytechange fail
- past/future299/300/301s
- missing timestamp/v1/empty signature
- conflicting timestamp headers
- multiple v1 rotation candidates
- v0 downgrade only
- nonUTF8 raw bytes versus reserialized JSON
- unavailable installation secret/clock

Пределы доказательств:

- Нет Stripe live deliveries/credential setup
- Нет durable normalized receipt/outbox/admission/2xx gate
- Нет source read proof и финансовой допустимости оплаты

Будущий эпик компонента: Intake & Source Connectors.

## NB-C2 · Intake: подключить AuthenticateWebhook через scoped secret и Clock ports

Репозиторий: `hitpipe/hitpipe`; задача: IMP. Подключенный production application consumer, narrow dependency ports, реальные файловые diagnostic adapters и изолированные synthetic doubles.

Владелец: Никита Будыкин. PRIMARY: `services/intake/internal/domain/stripesignature`, `services/intake/docs/module-contract.md`, `services/intake/internal/application/authenticatewebhook`, `services/intake/internal/adapters/fixtures/signingsecrets`, `services/intake/internal/adapters/clock/system`.

Зависимости: NB-C1.

Нормы: HP-BP-002/T/001, HP-BP-002/T/002, HP-BP-002/T/010, HP-BP-007/T/005.

Критерии:

- Consumer сначала разрешает installation-scoped secret+trusted clock, проверяет bytes, только после успеха допускает strict parse/whitelist normalization следующему typed consumer.
- Один consumer работает с saved fixture source и test double; повтор event ID не обходит подпись, неизвестный installation/secret/clock error дают отказ.
- Production webhook route/2xx в этой порции не включаются: пока нет durable commit у реального receipt port, запрещено отвечать fake accepted; факт этого ограничения отражен в README.
- Документация module contract описывает purpose/input/output/errors/ownership/side-effects/error precedence и пример; Godoc либо TSDoc соответствует выбранному стеку.
- DI contract test запускает штатный, ошибочный и граничный сценарий с обеими реализациями; одна точка сборки, consumer unchanged; ограничения подмены указаны явно.
- Запись собственного модуля в карте с фактическими import arrows, статусом и автором; две системные сложности, существующий module-candidate и собственный local implementation ADR с options/cost/check; не создавать второй HP-ADR-001.

Проверяемые сценарии:

- official-style known expected HMAC vector fixed offline
- same bytes success/onebytechange fail
- past/future299/300/301s
- missing timestamp/v1/empty signature
- conflicting timestamp headers
- multiple v1 rotation candidates
- v0 downgrade only
- nonUTF8 raw bytes versus reserialized JSON
- unavailable installation secret/clock

Пределы доказательств:

- Нет Stripe live deliveries/credential setup
- Нет durable normalized receipt/outbox/admission/2xx gate
- Нет source read proof и финансовой допустимости оплаты

Будущий эпик компонента: Intake & Source Connectors.

## NB-C3 · Intake: проверить HMAC/timestamp parser и seeded ingress bypass defects

Репозиторий: `hitpipe/hitpipe`; задача: VAL. Система проверок собственного настоящего module, coverage, CI и reproducible mutation experiment; без внесения mutant code в main.

Владелец: Никита Будыкин. PRIMARY: `services/intake/internal/domain/stripesignature`, `services/intake/docs/module-contract.md`, `services/intake/**/*_test.go`, `services/intake/testdata`, `services/intake/tools/mutations`, `services/intake/docs/test-design.md`, `.github/workflows/pr12-nb.yml`.

Зависимости: NB-C2.

Нормы: HP-BP-002/T/001, HP-BP-002/T/002, HP-BP-002/T/010, HP-BP-007/T/005.

Критерии:

- Test oracle записан до запуска из указанных правил, а не скопирован из actual output. Не менее двух техник: equivalence classes+boundary; decision/state table по объекту. Включены normal/error/boundary cases.
- Реальные local+CI прогоны exact submission commit; CI запускает собственные названные тесты и падает при нарушении assertion. Dataset synthetic/fixed seed, clean-clone command без secret/private fixture.
- Coverage получен для собственного module, выявлен конкретный missed branch либо непроверяемое свойство и принято решение; одного процента недостаточно.
- Не менее5 compiling seeded defects поодиночке с actual protocol; missed → explanation+new test+rerun; исходный и финальный clean runsPASS, естественные defects учтены честно.
- Risk registry qualitative probability×impact с rationale и security risk меняет selection/frequency/manual/deferred checks; fake fullE2E/provider/acceptance claims отсутствуют.
- Human screenshot placeholders содержат exact command/revision/expected visible evidence; агент исполняет проверки и сохраняет журналы, человек делает только реальные снимки. Human review и чистое independent reproduction остаются явным gate.
- Локальный отрицательный прогон ТОЙ ЖЕ содержательной CI-проверки: намеренно нарушить проверяемое условие → фактический ненулевой exit/FAIL и журнал → восстановить исправный код/условие → повторный PASS. Сохранены exact argv, SHA, patch/digest, exit/log paths и human instruction для рисунка7 МДК01.04 ПР1; мутация/ошибочное условие не попадают в main.

Проверяемые сценарии:

- official-style known expected HMAC vector fixed offline
- same bytes success/onebytechange fail
- past/future299/300/301s
- missing timestamp/v1/empty signature
- conflicting timestamp headers
- multiple v1 rotation candidates
- v0 downgrade only
- nonUTF8 raw bytes versus reserialized JSON
- unavailable installation secret/clock

Предложенные мутации (это ещё не результаты эксперимента):

- verify transformed/reserialized body вместо raw
- MAC comparison пропустить
- acceptv0 withoutv1
- abs timestamp difference заменить signed
- tolerance <=300 заменить <=301
- conflicting timestamps взять первый

Пределы доказательств:

- Нет Stripe live deliveries/credential setup
- Нет durable normalized receipt/outbox/admission/2xx gate
- Нет source read proof и финансовой допустимости оплаты

Будущий эпик компонента: Intake & Source Connectors.
