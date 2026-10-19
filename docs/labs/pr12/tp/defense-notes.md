# Индивидуальная памятка к защите

Тимофей объясняет и выполняет действия сам, без генеративного ИИ. Это подготовка, защита ещё не состоялась.

1. Показать HP-RULE-006 и почему paid недостаточно: received/captured/total, partial refund, dispute, currency, quantity/Price/account/mode. В Evaluate указать проверку unknown и объяснить null/false/zero.
2. Объяснить, почему VALID — финансовая допустимость, а не current rights/dispatch/confirmed. Назвать будущие actor/epoch/lifecycle/lineage и authenticated Intake обязанности.
3. В CLI изменить --fixture valid.json на refunded.json, затем invalid.json; показать разные outcomes, safe diagnostics и exit. --version связать с fullSHA/dirty/buildMode.
4. Объяснить compiler/linker/runtime и реальные dev -N -l/release -trimpath. Показать binary/module inspection, system dylibs и outside-checkout запуск; измерения скорости не выдавать за выполненные.
5. Открыть EvaluatePaidOrder, SourceFactsReader, File и finite double, показать одну consumer callsite. Выполнить TestReaderSubstitution для normal/error/boundary обеих реализаций; назвать непроверенные remote properties.
6. Показать map arrows как source imports, свой ADR с тремя вариантами/ценой и два направления сложности. Объяснить, почему profiling ещё planned и bottleneck не установлен.
7. Вывести тестовые oracles из требований: exact expiry/activation, money boundary, unknown precedence. Показать шесть mutation patches, compile0 и один failing assertion; restored green/clean. Survivors0 честно; естественных дефекта два: output writer и TP-REV-001 parser aliases, отдельно от шести seeded mutants.
8. Разобрать coverage91.2%/domain-app100.0%, конкретный File post-read cancel gap и риск. Объяснить qualitative risk matrix (low×severe HIGH), как selection/frequency изменились.
9. Честно назвать pending interview/peer/opposition/report review/screenshots/tags; engineering merge уже состоялся. Не утверждать, что защита или source acceptance состоялась. Преподавателю не заполнять оценку и подпись.

10. Объяснить TP-REV-001: DisallowUnknownFields допускает case-fold aliases; точные JSON-теги проверяются до decode. Показать оба порядка refund shadowing (exit 2, stdout пуст), canonical zero/false (VALID), единственный canonical refund=1 (INELIGIBLE/REFUND, exit 0). Принятый main ec0dcba7ca42…; реальные red/fix/green logs и merged PR №20.
