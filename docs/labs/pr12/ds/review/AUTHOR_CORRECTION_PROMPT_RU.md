# Поручение агенту Дмитрия Стацюка после ревью

```text
Продолжи мою Feature HIT-14 и Task HIT-41–48 в hitpipe/hitpipe, MANUAL_LAB_PR12.
Это исправление уже проведённого командного ревью, а не новое исследование продукта.
Прочитай AGENTS.md, frozen Universal Master Prompt и AGENT_LAB_WORKFLOW.md;
затем текущие Jira/PR, docs/labs/pr12/ds/review/reviewer-assignment.json,
REVIEW_RESULTS_RU.md и criteria-audit.json. Проверяющий со стороны команды —
Дмитрий Карачебан; технический/концептуальный/code review он делегировал Codex.
Матвей не является назначенным проверяющим HIT-14. Не пиши, что теперь Lead
должен самостоятельно анализировать концепцию или код: это уже сделано,
вердикт CHANGES REQUESTED, DS-REV-001–009 открыты.

Если review/ ещё нет в твоей рабочей ветке, пакет уже опубликован в
https://github.com/hitpipe/hitpipe/pull/28, ветка codex/HIT-47-ds-delegated-review.
Получить его можно без изменения своей ветки: git fetch origin codex/HIT-47-ds-delegated-review,
затем читать файлы командой git show origin/codex/HIT-47-ds-delegated-review:docs/labs/pr12/ds/review/REVIEW_RESULTS_RU.md
и аналогично reviewer-assignment.json, criteria-audit.json и остальные review artifacts.
Первоначальный неизменяемый review package: 119cfa83dafc5666cae502e56c85fc32d05eff86.
Не сбрасывай свою ветку к reviewer branch; её назначение — контекст и воспроизведение.

1. Сначала восстанови текущие remote heads и свои локальные незапушенные изменения.
   Не сбрасывай их. Найди свой R1 commit 0fb84594b18d1f8855f38761331b2ce79d16a97e
   и опубликуй реальный R1 отдельной веткой/PR. Во время ревью у lasarc уже был
   write в hitpipe/hitpipe; старый HTTP403 не является текущим доказанным блокером.
   Если локальный результат утрачен, честно подготовь новую desk-research baseline
   по exact sources, с новой provenance; не выдумывай старый commit или интервью.
2. Устрани DS-REV-001–005 по HP-BP-004/T/007,T/008,T/013 и formal schemas.
   Не меняй canonical Blueprint/SQL/wire/config pins ради текущего кода.
   Canonical rendering, нормализация NFC и CRLF/CR→LF, marker/case/source linkage,
   контекст/JCS escaping/byte budgets, unknown/incomplete/mismatch и side-effect
   границы должны быть однозначны. Не принимай неполные/повреждённые данные как match.
   Saved evidence связывается с requested target; source/read time и contradictory
   retrieval flags проверяются. JSON не должен терять malformed Unicode или позволять
   duplicate/alias keys скрывать исход. Provider write, retry и business state transition
   в этот offline модуль не добавлять. Новый DTO не является новым formal wire contract.
3. Добавь регрессионные проверки по каждому воспроизведённому reviewer case.
   В review/probes лежат тесты для исходного API; при необходимом изменении модели
   сохрани исходный oracle, адаптировав fixtures/API, а не ослабляя условия.
   Один consumer, две реализации за тем же port, normal/boundary/error для обеих;
   composition и фактическая карта imports; GoDoc публичных границ, ошибки/владение.
4. Заверши DS-REV-006: минимум пять компилируемых поведенческих мутаций по одной,
   baseline PASS, mutated build, обнаруживший assertion/честный survivor, усиление
   при необходимости, restoration hashes, final PASS. Сохрани actual logs/patches.
   Самописный небольшой runner допустим; отсутствие установленного mutation tool
   не препятствует выполнению опыта. Reviewer probes не выдавать за свои мутации.
   Выполни реальные named tests/vet/race/dev-release smoke/coverage; запиши применимость
   и непроверенные свойства. Минимум две техники тест-дизайна и точная risk matrix.
5. Заверши R1 trace/security/AC и 29-строчную criteria/evidence матрицу. Не заменяй
   исходное интервью desk research и не подписывай личные действия другого студента.
6. После стабилизации кода поправь module map/ADR/toolchain и четыре полных DOCX
   по четырём формам и актуальным методичкам. Не заполняй семь разных toolchain rows
   одинаковыми общими данными. Profiling работает только при фактическом запуске;
   иначе честный PLANNED с целью/этапом. Выводы, риски, test design и coverage должны
   быть заполнены, не оставляй редакционные команды и пустые "...: .".
   Не называй пакет CONTENT_READY_IMAGES_PENDING до готовности агентной части.
   Теги ещё не созданы: NOT CREATED, никаких фиктивных tag→SHA. Exact final source/CI,
   evidence digests и 21 полная human screenshot instruction; PNG не создавать.
   DOCX ZIP/XML/links/text/slot checks, renderer только если действительно доступен.
7. Сохрани три осмысленных code Tasks/PR, исходную историю и текущие ownership paths.
   Исправления делай своими commits/PR; не force-push чужие ветки, не меняй protected
   rules/общие immutable sources/общий CI. Если обновляешь стек, последовательно
   интегрируй родителей обычным согласованным merge и проверяй exact heads.
8. В correction-record перечисли DS-REV-001–009 → commit/PR → regression/evidence;
   сохрани авторские первоначальные результаты и reviewer findings как историю.
   Код без независимого повторного verdict не называть принятой работой.
   Получи реальные CI results нового head. Передай один законченный пакет на
   повторное ревью Codex по делегации Дмитрия Карачебана. Не сам себе APPROVE,
   не делай merge/Done/Feature close, не имитируй официальный Project OS /review.

Не спрашивай Lead о продуктовых трактовках, которые уже закреплены exact Blueprint
и условиями review. Закрой доступные технические/документальные результаты полностью.
Интервью, личные student checks, внешнее opposition, подписи/защита и реальные снимки
сохраняют честный human status; они не причина откладывать доступные исправления.
```
