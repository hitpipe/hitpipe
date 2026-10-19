# HIT-14 — повторное ревью исправлений Дмитрия Стацюка

19.10.2026. Ответственный проверяющий — Дмитрий Карачебан; фактическое source/code/document review выполнено Codex по его делегации. Проверен [PR №29](https://github.com/hitpipe/hitpipe/pull/29), exact head `9ebe6373467ded83f32939a849bac31905eed99f`, base `0a6f2ef3a5cd6db30ff4cef5efbefd185fc47b00`. История первоначального review сохраняется в PR №28.

**Вердикт: RE-REVIEW COMPLETED / CHANGES REQUESTED. Два замечания закрыты в пределах offline slice, семь исправлены частично либо остаются открытыми. Пакет пока не принят.**

## Подтверждённый прогресс

- Все девять первоначальных примеров дефектов перепроверены на актуальных типах и полноценном каноническом положительном fixture: **9 PASS**. Это не означает автоматического закрытия девяти более широких замечаний.
- Frozen bundle, 17 wrapper tests, named Delivery tests/vet, dev/release smoke вне checkout и отдельный race — **PASS** на Linux amd64 / Go 1.27.2. Обязательные четыре CI-контекста PR29 успешны на проверенном head; текущий [Delivery CI](https://github.com/hitpipe/hitpipe/actions/runs/39271344496).
- Пять авторских мутаций теперь существуют. Независимо повторены все пять изменений через Go overlay: вычисленные source/mutant hashes совпадают с авторским журналом, все пять компилируются и дают содержательные assertion FAIL; немутированный source после опыта даёт PASS. Не требуем заново доказывать отсутствие этих экспериментов.
- Штатная `workspace.py check delivery --require` в физической изолированной копии с M1 действительно даёт exit1/assertion FAIL. Предварительная попытка передать overlay через GOFLAGS совместно с coverage вернула PASS и **не засчитана** отрицательной демонстрацией; продуктовая ошибка wrapper из этого не выводится.
- Target ID, непустые source/read-time и противоречивый `RetrievalError` теперь проверяются; CR и CRLF нормализуются в LF. CLI действительно вызывает application consumer. R1 desk-research документы опубликованы с честным отсутствием интервью.
- Измеренное unit statement coverage всей Delivery **56.9%**; application VerifyTarget75.0%, FileReader63.2%, CLI0%. Это наблюдение, не новый процентный gate. Cross-package исполнение FileReader не опровергается нулём отдельной инструментированной функции.

Точные argv, SHA, exits и source-restoration hashes находятся в [evidence/review-checks.json](evidence/review-checks.json). Авторские production-файлы и четыре DOCX reviewer-ом не изменены.

## Статус исходных замечаний

| ID | Итог повторной проверки | Остаток |
| --- | --- | --- |
| DS-REV-001 | PARTIALLY CORRECTED, P1 | Канонические delimiters приняты, но структура блока, case UUID, control input и JCS budget не обеспечены полностью |
| DS-REV-002 | CLOSED FOR OFFLINE SLICE | Привязка к запрошенному target и обязательность provenance подтверждены; live authority не заявляется |
| DS-REV-003 | PARTIALLY CORRECTED, P1 | Исходный alias/битый UTF-8/lone surrogate отклонены; duplicates, mixed/escaped aliases и лишняя закрывающая скобка всё ещё дают match; валидные Unicode escapes отвергаются |
| DS-REV-004 | PARTIALLY CORRECTED, P2 | Невалидный UTF-8 title и retrieval contradiction исправлены; NUL/control input может дать match |
| DS-REV-005 | CLOSED | CR/CRLF→LF и сохранение значимых пробелов подтверждены |
| DS-REV-006 | PARTIALLY CORRECTED, P2 | Мутационные опыты подтверждены; две техники тест-дизайна, разбор покрытия и точный risk registry не завершены |
| DS-REV-007 | PARTIALLY CORRECTED, P2 | R1 теперь доступен; 29 строк есть, но повторяют общие evidence и устаревшие ограничения; R2 продолжает говорить об отсутствующем R1 и несуществующем exact-set mode |
| DS-REV-008 | OPEN, P2 | Четыре DOCX содержат TEMPLATE_UNFILLED, пустые обязательные таблицы, неверные/неполные инструкции и преждевременный CONTENT_READY_IMAGES_PENDING |
| DS-REV-009 | PARTIALLY CORRECTED, P2 | CLI/composition и import map исправлены; normal/boundary/error не исполнены для каждой из двух реализаций; GoDoc/проекция нуждаются в завершении |

Это продолжение прежних условий, не новый стратегический scope. [finding-status.json](finding-status.json) содержит проверяемые условия закрытия.

## Воспроизводимый остаток в коде

Все дополнительные тесты компилируются. Зафиксировано **13 FAIL на уровне отдельных случаев**, плюс три агрегирующих parent FAIL. Их нельзя считать шестнадцатью независимыми дефектами. Исходные девять случаев — PASS. Полный журнал и имена: [evidence/independent-repeat-probes.log](evidence/independent-repeat-probes.log), [probe-results.json](probe-results.json).

1. **DS-REV-003: неоднозначный JSON становится положительным evidence.** Вместо `"retrieved":true` переданы `"retrieved":false,"retrieved":true`, либо `"retrieved":false,"retRieved":true`, либо escaped `"\u0052etrieved":true`. Получается match, в том числе через FileReader. `DisallowUnknownFields` не проверяет уникальность и exact spelling; regex только с заглавной первой буквой не определяет структуру JSON. После корректного документа лишний `}` также принят: `decoder.More()` не является проверкой EOF документа. Нужны структурная проверка декодированных ключей/формы, duplicates/aliases и обязательный EOF, общая политика для обеих границ.
2. **DS-REV-003: ложный отказ валидному Unicode.** `\ud000` — допустимый BMP scalar, `\ud83d\ude00` — допустимая supplementary pair. Оба отвергаются правилом «любое `\ud...` запрещено». Проверять нужно корректность surrogate pairing, не запрещать все такие текстовые последовательности. Эквивалентные literal/escaped Unicode должны сохранять значение; настоящие unpaired surrogates/invalid bytes должны оставаться ошибкой.
3. **DS-REV-001: неполная проверка handoff.** Одинаковый expected/observed блок с 36 буквами `x` вместо UUID, заменой LF после opening marker на `X`, отсутствующей строкой `Context:` или вложенным полным handoff даёт match. Проверка только prefix/длины/suffix этого не исключает. Часть собственных positive fixtures тоже не содержит `Context:`. Нужен точный T/013, валидный expected input, один блок/пара delimiter lines, case/source linkage и явная проверяемая проекция канонических DTO. Если `HandoffBlock` уже извлечённый блок, документировать и проверить владельца extraction, включая разрешённые заметки снаружи; не выдавать строковый projection за готовый provider adapter.
4. **DS-REV-004: control input.** Одинаковый Title с NUL проходит UTF-8 check и даёт match, хотя T/013 требует отклонения недопустимых control input. Допустимые нормализуемые переводы строк не следует смешивать с запрещёнными controls.
5. **DS-REV-001: размер контекста.** Проверяется длина свободного raw `Context`, а требование задаёт UTF-8 размер JCS object из configured keys и normalized values, включая кавычки/escaping. Для одного ключа `address` и 20,000 кавычек value object занимает **40,014 bytes**; raw value и rendered block укладываются в нынешние bounds, Compare даёт match. Нужна явная проекция/граница валидации такого контекста и точные 32KiB/36KiB boundary tests. Это отсутствие требуемого ограничения локального module contract; live send/production exploit не заявляется.

## Проверки, документы и provenance

**DS-REV-006.** Пять mutations засчитываются как выполненный опыт. Остаток конкретен: нет законченного вывода тестов минимум двумя техниками из правил; coverage section и таблица пробелов пусты. `HIT-45-risk-matrix.md` имеет header на4 столбца, строки на7; не содержит корректно оформленного probability/impact→priority по таблице преподавателя с ID, rationale, частотой и остаточным риском. В DOCX соответствующие строки пусты. Нужен законченный registry, не новая произвольная шкала.

В `correction-index.json` **5 из6 SHA-256 не совпадают с текущими repo bytes**. Для4 из этих5 исходный digest совпадает при CRLF representation; это объясняет часть расхождений и не является обвинением в выдуманных запусках. Пять mutant hashes отдельно совпали и независимо воспроизведены. Индекс должен различать raw-run и versioned-blob bytes и проверяться после Git normalization; фактические digests сохранены в [evidence/author-evidence-hashes.json](evidence/author-evidence-hashes.json).

**DS-REV-007.** R1 уже не отсутствует: вопросы/FR/security подготовлены. Нельзя сохранять old limitation «interview/questions absent from published stack» как текущую истину. Каждой из29 строк нужны свой actual Task, критерий, конкретный документ/test/log/CI, применимость и honest human status. `requirements-trace.json` должен вести к действительным проверкам; упоминание имени теста не доказывает весь FR. R2 и security baseline согласовать с реально поддерживаемыми гарантиями.

В первоначальном reviewer criteria audit были переставлены описания **0104 ПР1 Р1/Р2 и Р3–Р5**. Это ошибка нашего предыдущего аудита; исправлена в этой редакции по frozen `METHODICS_MATRIX.json`: Р1=toolchain, Р2=свой scaffold, Р3=config/поведение, Р4=source→runtime/inspection, Р5=dev/release. Копирование прежнего audit не создаёт новой нормы. Новый [29-result audit](criteria-audit.json) строится непосредственно из методичек.

**DS-REV-008.** ZIP/XML четырёх DOCX исправны, unresolved `{{...}}` нет, embedded media0 и21 image slot сохранены. Но во всех остаётся TEMPLATE_UNFILLED; у0302 ПР1 пустые FR/NFR/technical-results таблицы, у0302 ПР2 пустые requirement/test-design/risk/coverage/conclusion таблицы. Mutation rows одновременно говорят `NOT RUN` и5executed. У0104 ПР1 все12 dev/release cells одинаковые `NOT RUN / diagnostic_smoke PASS`, семь toolchain rows повторяют одно средство/назначение; profiling необоснованно находится среди общих PASS. У0104 ПР2 обе implementation rows и6case rows всё ещё NOT RUN. Добавление сводного абзаца в конец не заполняет основной отчёт. Исправить исходные ячейки и полный текст, затем вновь извлечь и проверить таблицы. Не считать девять review findings девятью подтверждёнными исправленными code defects.

В отчёте всё ещё есть ссылка `0104-pz01-ds-v1 → SHA` при отсутствии такого тега. `submission-record` отстаёт от актуального PR head/CI; финальный SHA можно фиксировать сопровождающей записью после коммита, без бесконечного самообновления. `CONTENT_READY_IMAGES_PENDING` пока недостоверен. Визуальная пагинация reviewer-ом не проверялась: renderer на VPS отсутствует; отсутствие renderer не оправдывает пустые таблицы.

21 human capture instruction требуется перепроверить. Примеры: Go-команды из root без go.mod; `-gcflags=all=-N -l` с отдельным `-l`; binary/output path относительно неверного cwd; PENDING CI вместо существующего URL; module map=`NOT CONDUCTED`; HIT-46 сопоставлен PR23/HIT42. Человеческие интервью/проверки действительно остаются pending, их не заменять вымышленными URL. Для каждого такого шага дать подготовленный протокол и конкретное действие, для доступных агентных шагов — исполненные команды/точные результаты.

**DS-REV-009.** `boundary` в `di_contract_test.go` повторяет тот же FileReader/path/input/Match, что и normal. `error` заменяет FileReader отдельным errorReader. Это не три категории для FileReader; application fake сам по себе не восполняет пробел. Выполнить normal/boundary/error каждым adapter через один consumer; отклонения input/evidence проверить с реальной FileReader реализацией. CLI использует synthetic fixtureReader, поэтому не приписывать ему исполнение FileReader. Завершить public GoDoc и местный контракт отображения/ошибок.

## Следующие действия

1. Авторский агент выполняет [готовое поручение](AUTHOR_CORRECTION_PROMPT_RU.md) в существующем PR29, сохраняя историю. Предварительных действий Lead не требуется.
2. После кода — актуальные regressions, пять мутаций на изменённом source/test наборе, tests/vet/race/smoke/CI; исторические журналы сохраняются.
3. Заполнить четыре DOCX и29-result crosswalk, проверить все21 инструкции и artifact digests на versioned bytes; передать exact head + CI + correction mapping.
4. Codex по делегации Lead проводит повторную проверку; после положительного verdict возможны реальное approval/интеграция. Старые PR23–27 пока содержат прежние непроверенные исправлениями heads: их не принимать только из-за зелёного PR29. При интеграции сохранить три осмысленных code PR и получить CI на принимаемых ревизиях.
5. После технической приёмки — необходимые реальные личные учебные действия, снимки, визуальная проверка/обновление полей DOCX, согласованный merge и immutable tags. H1/Feature не закрываются до фактического DOD.

Discovery/Product/Architecture acceptance, canonical sources, shared runtime и конфигурация Project OS не менялись. Личные действия студентов, подписи, интервью, cross-project opposition и защита не засвидетельствованы данным агентным review.
