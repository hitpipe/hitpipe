# Реестр рисков Processing

HIT-29. Использована матрица методички 03.02 ПР2 п2.4: низкая×небольшой низкий; низкая×значимый средний; низкая×тяжёлый высокий; средняя×небольшой средний, остальные высокий; высокая×небольшой средний, остальные высокий. Это качественные предположения для selection, не измеренная вероятность.

| ID | Событие, причина и ущерб | Вероятность и обоснование | Ущерб | Приоритет | Проверка и частота |
|---|---|---|---|---|---|
| R01 | Unpaid/refunded/partial capture ошибочно разрешён из paid shortcut; лишняя работа/финансовые потери | Средняя: несколько независимых provider facts | Тяжёлый | Высокий | FinancialDecisionTable/UnknownAndZero + M01/M02/M03/M06 при каждом изменении/CI |
| R02 | Чужой account/connection/test-live proof принят; нарушение изоляции и ошибочное создание | Низкая: scoped input, но ошибки mapping возможны | Тяжёлый | Высокий | source scope, ContextPins + M05 при каждом изменении; реальные auth/tenant tests будущего adapter отдельно |
| R03 | Unknown/read error превращён в false/zero/success | Средняя: nullable wire и outages обычны | Тяжёлый | Высокий | EveryNullableFact/ReaderSubstitution/Precedence при каждом изменении |
| R04 | Старый proof омоложен completed_at; действие после недостоверной финансовой проверки | Средняя: многозапросный snapshot | Тяжёлый | Высокий | ProofBoundary/Timeline каждый change; live atomicity не обещается |
| R05 | Недоверенный JSON допускает duplicate/extra/oversize или отражает secret body в logs | Средняя: boundary input недоверен | Тяжёлый | Высокий | StrictFixture/ReadFailure/CLI redaction каждый change; 1MiB только offline envelope, full security integration отложена до authenticated adapter |
| R06 | Вывод CLI потерян, exit0 скрывает ошибку | Низкая: pipe/disk failure редки | Значимый | Средний | TestOutputFailure каждый change после естественного defect HIT-29 |
| R07 | Файловая подмена ошибочно воспринимается как live payment authority | Низкая: явный offline contract | Тяжёлый | Высокий | Human review module-contract и будущей composition перед integration; live path в этой задаче отсутствует |
| R08 | Долгое объяснение/документация неудобны | Высокая: много критериев и команд | Небольшой | Средний | Реальный student clarity/clean-room и индивидуальная защита HIT-31; пока HUMAN_PENDING |

Для всех высоких локально проверяемых рисков automation runs every change. Real TLS/current authority/revoke/lineage/DB/live endpoints/high load требуют будущих integration/environment tests и human admission; их отсутствие не записано как PASS лабораторного consumer.

## Уточнение риска офлайн ввода после TP-REV-001

19.10.2026, принятый main ec0dcba7ca42ea59909489cef9650c6edff12f35. Case aliases в synthetic fixture могли перекрыть refund и исказить диагностический результат. Вероятность — средняя (качественная инженерная оценка, не измеренная частота), ущерб — значимый, приоритет — высокий. Это не доказанная production vulnerability. Меры: exact JSON names до decode; aliases top-level/nested и оба порядка конфликтов отвергаются; canonical zero/false и прежние строгие guards сохраняются. Набор regression tests и actual CLI exits проверяются при каждом связанном изменении. PR №20 принят и merged; evidence/tp-rev-001/ и report-refresh-20261019/ сохраняют red/fix/green. Два реальных дефекта и шесть seeded mutants считаются раздельно. Human review source authenticity/real integration по-прежнему отдельно.
