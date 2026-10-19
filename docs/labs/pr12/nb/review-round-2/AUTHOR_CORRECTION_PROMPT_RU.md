# Следующий запуск агента Никиты — повторное ревью

~~~text
Продолжи HIT-15, HIT-49–56, Intake - Source Integrations, MANUAL_LAB_PR12.
Командный reviewer — Дмитрий Карачебан, анализ — Codex по его делегации.
Повторный verdict CHANGES_REQUIRED. Сначала сохрани текущую работу,
получи реальные PR31–34/heads/Jira и прочитай AGENTS/manual workflow/frozen
Universal Master Prompt/context lock и этот review record.
Review package можно получить через:
  git fetch origin codex/HIT-55-nb-repeat-review
  git show FETCH_HEAD:docs/labs/pr12/nb/review-round-2/REVIEW_RESULTS_RU.md
Не checkout/reset свою работу на reviewer branch.

Проверенный report source 3fdd78828a69bf788b7a2da9aa3acf58a5e1531a.
Правильная policy уже есть в C3 df1a7d58179148abfda8e71862654f5ef7fcdbc7.
На итоговом пакете reviewer подтвердил tests/vet/race/smoke/inspection,
три исходных regression cases,49 дополнительные freshness cases и4 HMAC vectors.
text/json/invalid дают0/0/2,27 digests совпали. Не начинай реализацию заново.

1. Согласуй реальные ветки. Минимальный числовой fix+регрессии должны быть
   уже в C1/PR31 (сейчас он старый), затем обычные merges родителей в C2/PR32,
   C3/PR33 и report/PR34. Сохрани три самостоятельных code PR, историю и
   отдельный смысл каждого. Не force-push. Не сливай в main.
   Исправление остаётся прежним: checked parse, invalid clock fail-closed,
   fixed inclusive300s, safe distance. Новых SDK/архитектуры не нужно.
   Попубликуй настоящие отдельные PR оставшихся R1/R2/D1/H1 задач
   согласно их DOD, с корректными bases/diffs и собственными source pins.
   В особенности HIT50 significant requirement change не может навсегда
   оставаться «eventual PR, until Lead creates it». Сам имеешь write.
   Во всех8 Task зафиксируй actual branch/PR/SHA/check/evidence.
   Если review авторских refs уже изменился, используй actual состояние без дублей.

2. NB-REV-007: сохрани старые mutations как историческую серию. Создай
   новую versioned correction series на зафиксированном исправленном baseline.
   Выполни >=5 настоящих независимых behavioral mutants: build0,
   test assertionFAIL либо survivor→усиление теста→actual retest.
   Patch apply failure и compile failure не считаются detection.
   Перед каждым/после каждого проверь baseline/restoration, final gate PASS.
   M03/M04 должны менять текущий outsideTolerance, а не удалённый delta block.
   Source/patch hashes, full SHA, argv/cwd/date/tool versions и stdout/exits
   обязаны быть настоящими. Обнови protocol/report/index/negative gate route.
   Не выдавай reviewer probes за свои mutations; hidden suite NOT_PROVIDED.
   Не переписывай старые evidence файлы результатами нового опыта.

3. NB-REV-004/005: исправь tools/build_pr12_reports.py и пересобери4 DOCX.
   - 0104PR1:12 одинаковых NOT RUN/PASS cells заменить конкретными dev/release
     наблюдениями по6 вопросам, с командами/источником/границами.
   - 0302PR1: заполнить техническую results table, свой технический вклад,
     итоговые ссылки на материалы/Task/PR/CI. Пустые human-only поля допустимы
     только с явной причиной, а не вместо доступных технических данных.
   - FR-SIG-03 source BP007/T005; NFR-SIG-01 privacy BP002/T006,T008
     и применимые канонические записи. Источник requirements-trace,
     не устаревшая hardcoded таблица. Clock/deadline BP007/T010 отдельно.
   - source/report/test snapshots различать. Точные actual SHA/CI,
     значения coverage и counts брать из журналов; будущий self-SHA не сочинять.
   - FileResolver+FixedClock / MapSecrets+FixedClock, SystemClock неисполнен
     в этом опыте; coverage не CPU/memory profiling, pprof PLANNED если не запускался.
   - Конкретно проверить content после генерации, не только ZIP/hash.
     Содержательный validator должен ловить пустые технические таблицы,
     generic NOT RUN/PASS и рассогласованные FR/source до объявления ready.
   - Перечитать все4 DOCX; renderer если доступен, иначе честный visual NOT_RUN.
     Ни подписи, ни личные ответы студента не заполнять.

4. NB-REV-005: все29 результатов сверить с frozen METHODICS_MATRIX.
   Добавить реальные HIT keys и Task→PR→artifact→source/check/log/status связи.
   0302-pz01/Р4 имеет canonical gate=none: fixtures/security проверяет агент,
   не ждать для него человека. Настоящие interview/peer/opposition gates
   остаются pending. Risk low×serious=medium уже исправлен; сохранить.
   Не превращать просто наличие файла/папки/CI в PASS всех критериев.

5. NB-REV-006: все21 инструкции выполнить/сверить из чистого checkout.
   Исключить '(expected exit 2)' из shell argv; ожидаемый выход писать отдельно.
   Указать реальный distribution cwd для ./intake-contractcheck/verified.json
   или полные пути; включить сборку и новое копирование до запуска.
   Убрать буквальные <SHA> из исполняемых команд: полный доступный pin
   или вычисленный full SHA с проверкой file exists. Никакого self-SHA loop.
   Перепривязать mutation route к применимому новому patch/baseline.
   Снимки не делать: PNG остаются человеческими плейсхолдерами с полной
   навигацией, результатом, местом вставки и redaction.
   Старые профили/журналы не затирать; новый output в ignored .pr12-build
   либо новый versioned evidence каталог.

6. Выполни затронутые checks после source/test changes; сохрани regression
   oracles и неизменность правил. Actual CI каждого изменённого PR head.
   Content/evidence-only поправки без изменения входов не требуют бесконечного
   повторения уже выполненной правильной мутационной серии.
   Обнови correction-record NB-REV-001–007→commit/PR→evidence/retest,
   source manifest, evidence index, submission record и handoff.
   В Jira только реальные комментарии/labels/status; автоматику не обходить,
   Project OS /start,/review/FORCE не имитировать.

Передай один полный пакет на независимое повторное review.
Не сам себе APPROVE, не main merge/Done/tags/Feature close.
Не жди новых продуктовых решений Lead и не начинай исследований заново.
Личные interview/peer/clarity/clean-room/opposition/defense остаются
фактическими человеческими действиями и не останавливают эти исправления.
~~~
