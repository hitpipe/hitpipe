# Следующий проход — Дмитрий Стацюк, HIT-14

```text
Продолжи HIT-14 / HIT-41–48, участник Дмитрий Стацюк (lasarc),
Delivery - Destination Integrations, MANUAL_LAB_PR12.
Исправления выполняй в существующей работе/PR29 без потери local changes/history.
Прочитай AGENTS.md, frozen master/workflow/lock и текущие Jira descriptions/comments.

Повторное ревью Codex по делегации Дмитрия Карачебана выполнено на
9ebe6373467ded83f32939a849bac31905eed99f. CHANGES REQUESTED.
Закрыты DS-REV-002 (offline target/provenance binding) и DS-REV-005 (CR normalization).
Все9 исходных воспроизведений проходят; пять твоих мутаций реально подтверждены
и повторены reviewer-ом с совпадением hashes. Это сохраняем как достигнутый результат.
Открыты/частичны DS-REV-001,003,004,006,007,008,009.

Полный контекст следующего review доступен без смены авторской ветки:
git fetch origin codex/HIT-47-ds-repeat-review
git show FETCH_HEAD:docs/labs/pr12/ds/review/round-2/REVIEW_RESULTS_RU.md
Сохрани fetched full SHA; прочитай finding-status.json, criteria-audit.json,
probes, evidence, reproduce.py и этот полный prompt в том же каталоге.
Не reset/checkout авторскую работу на reviewer branch. Предварительный merge
review-only PR и дополнительная ручная продуктовая/концептуальная проверка Lead не нужны.

1. Заверши DS-REV-003 на обеих JSON границах. Нужны структурная проверка exact
   decoded keys и duplicates, запрет mixed/escaped casing aliases, правильные types/
   shape и обязательный EOF. Не считать regex и DisallowUnknownFields достаточными.
   retrieved:false,retrieved:true; retRieved; \u0052etrieved и лишний } не дают match.
   Валидные \ud000 и \ud83d\ude00 сохраняют то же значение, что literal UTF-8;
   malformed UTF-8/unpaired surrogate отвергаются до lossy decoding.
   Политика одна для Fixture CLI и saved-observation FileReader.
2. Заверши DS-REV-001/004 по точному BP004/T013: valid lowercase UUID, ровно одна
   пара delimiter lines, обязательные labels/newlines, отсутствие nested/multiple
   marker input, одинаковый case/source linkage, валидный expected, разрешённые
   переводы строк и запрет недопустимых controls (NUL сейчас даёт match).
   Объяви и проверь service-local mapping DeliveryIntent/ObservedTaskAttributes,
   место extraction блока и поведение допустимых заметок снаружи. Не меняй upstream
   Blueprint/wire ради существующего string DTO. Existing positive fixtures без
   Context: не являются каноническими; исправь fixtures и independent oracles.
   32KiB измеряй как JCS object configured-key→normalized-value с JSON escaping,
   36KiB — весь rendered block; raw Context len недостаточно. Boundary и quote-heavy
   object 40014bytes должны проверяться на настоящей объявленной границе.
3. Заверши DS-REV-009: один consumer × FileReader/тестовая реализация × normal/
   настоящий boundary/error, минимум6 фактически различающихся сценариев.
   FileReader error не заменять отдельным errorReader; boundary не должен повторять
   normal с тем же input. Дай ожидаемый/фактический outcome и provenance каждого.
   CLI synthetic fixtureReader не выдавать за FileReader. Допиши public GoDoc,
   контракт допустимых входов/ошибок/проекции, синхронизируй module map.
4. Заверши DS-REV-006: два метода тест-дизайна с выводом входных классов/границ/
   решений из конкретных правил, actual tests; coverage область/команда/пробел/
   решение; точная qualitative risk matrix преподавателя с ID/P/impact/rationale/
   priority/проверкой/частотой/остаточным риском, включая security risk.
   Исправь повреждённую Markdown risk table (header4, rows7). Не вводи процентный gate.
   Сохрани исходные mutation logs. После изменения кода/тестов выполни актуальные ≥5
   compiling behavioral mutations с assertions/survivor/retest/restoration/finalPASS.
5. Заверши DS-REV-007. R1 уже существует: убери старую ложную формулировку об
   отсутствии документов/вопросов из current crosswalk/R2, сохрани исторический факт
   отдельной записью. Удали unsupported exact-set mode либо реализуй только то,
   что следует из реального frozen contract; extras assignees сами не являются ошибкой.
   Согласуй security baseline с действительными гарантиями. Все29 результатов сверяй
   с frozen METHODICS_MATRIX.json, а не копированием старого reviewer audit:
   0104PR1 R1=toolchain,R2=scaffold,R3=config,R4=execution/inspect,R5=dev-release.
   В старом audit reviewer-а эти описания были переставлены, исправление уже признано.
   Каждая строка: свой criterion→actual HIT→конкретный документ/test/log/CI/SHA→status.
   Human rows честно NOT_CONDUCTED с конкретным следующим действием.
6. Главное DS-REV-008: ЗАПОЛНИ ОСНОВНЫЕ ТАБЛИЦЫ ЧЕТЫРЁХ DOCX, а не добавляй ещё один
   абзац в конец. Сейчас все содержат TEMPLATE_UNFILLED,0302PR1 FR/NFR/technical
   tables пусты,0302PR2 test-design/risks/coverage/conclusions пусты,mutation rows
   одновременно NOT RUN/5executed,0104PR1 dev-release12cells одинаковые NOT RUN/PASS,
   0104PR2 implementations/6cases NOT RUN. Семь toolchain positions должны иметь
   собственные средства/версии/назначение/маршрут/status. Profiling честно PLANNED
   с объектом/этапом, если CPU/memory опыт не выполнялся. Исправь редакционные
   инструкции, ложную ссылку на несуществующий tag и запись «9 code defects fixed».
   Не заполняй вымышленным личные student actions/подписи/оценку/группу.
   Извлеки текст И таблицы каждой готовой DOCX и проверь каждую обязательную ячейку.
   CONTENT_READY_IMAGES_PENDING допустим только после доступного агентного содержания.
7. Проверь21 screenshot instruction из clean checkout: shell/cwd/cd, правильные
   quoted build flags, существующий binary/output path, exact source/CI URL,
   реальные Task↔PR, команда создания вынесенной distribution, ожидаемые exit/кадр,
   redaction, подпись и место вставки. Module map уже существует — не NOT_CONDUCTED.
   Human-only protocols подготовь с понятными шагами вместо вымышленного результата.
   PNG/снимки не создавать. Visual DOCX QA выполнять только реальным renderer;
   при отсутствии указать границу, но это не повод оставлять пустые таблицы.
8. Обнови evidence index по versioned Git blob bytes (git show SHA:path), определив
   byte policy. Сейчас5из6correction digests расходятся;4объясняются CRLF. Не выдавай
   normalized blob digest за raw Windows log hash. Проверь JSON index на чистом clone.
   Запусти named tests/vet/race/dev-release smoke/inspection и fresh remote CI.
   Не пиши SHA текущего коммита бесконечно в него же: финальный head/CI — в настоящем
   post-commit Jira/PR handoff или следующей evidence record с честным scope.
9. Передай correction mapping по семи открытым IDs: commit/PR→конкретный regression/
   evidence→готовый документ; exact final head/CI; четыре проверенных DOCX и29строк.
   Сохрани историю PR23–29 и три отдельные осмысленные code Tasks/PR.
   Не принимай старые heads23–27 как исправленные: подготовь непротиворечивый стек
   собственных веток обычными merge/backport без force-push и дубликатов результатов.
   Если Jira automation откатывает manual status, не обходи guard, запиши факт и
   продолжай разрешённую работу. Не изображай official /start,/review,freeze/acceptance.

До положительного независимого повторного verdict не self-approve, не merge в main,
не Done/Feature close и не submission tags. После готового пакета повторное review
выполняет Codex по делегации Lead. Личные интервью/blind/clean-room/module checks,
outside-project opposition, screenshots и защита требуют реальных действий людей;
они не блокируют технические исправления и не подделываются.
```
