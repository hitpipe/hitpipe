# Поручение Codex на стороне Никиты Будыкина

```text
Продолжи мою Feature HIT-15, задачи HIT-49–56 в hitpipe/hitpipe.
Участник — Никита Будыкин (Budykin), component Intake - Source Integrations.
Режим MANUAL_LAB_PR12. Выполни исправления полностью до повторного ревью.
Прочитай AGENTS.md, frozen Universal Master Prompt, AGENT_LAB_WORKFLOW.md,
exact context lock и текущие Jira descriptions/comments.

Разбор кода и документов уже выполнен Codex по делегации Дмитрия Карачебана.
Он проверяющий со стороны команды, прежнее назначение Стацюка отменено.
Вердикт SOURCE REVIEW COMPLETED / CHANGES REQUIRED, NB-REV-001–006.
Авторских PR пока нет; source review не изображать GitHub approval.
Отдельной ручной концептуальной проверки или code review от Lead не ждём.

Если review/ нет в своей ветке, получи контекст без её смены:
git fetch origin codex/HIT-55-nb-delegated-review
git show FETCH_HEAD:docs/labs/pr12/nb/review/REVIEW_RESULTS_RU.md
Сохрани fetched full SHA. Аналогично прочитай reviewer-assignment.json,
branch-inventory.json, criteria-audit.json, probes/evidence и reproduce.py.
Не reset/checkout свою работу на reviewer branch. Исходный проверенный пакет:
b23001983b511999d815fbc7f77063e763516bc2.

1. Восстанови actual remote/local heads, сохрани незакоммиченное. У Budykin write.
   Сам создай настоящие PR под своей учётной записью, не жди Lead для публикации.
   Если PR уже появились, используй их без дублей. Ветки расходятся: часть документов
   была cherry-picked в report branch. Проверь реальные merge bases/diffs.
   Подведи родителей обычными merge, без force-push/потери истории. Целевой порядок:
   R1→main, R2→R1, C1→R2, C2→C1, C3→C2, D1→C3, H1→D1, REPORT→H1.
   Это порядок после интеграции, не описание сегодняшней истории.
   Сохрани три отдельных осмысленных code PR HIT-51/52/53 и реальные Task links.
2. Исправь NB-REV-001: checked decimal int64 parsing и безопасная freshness.
   23000000000000000000 не должно молча стать 4553255926290448384;
   MinInt64 clock/MaxInt64 timestamp не должны дать VERIFIED с delta1.
   Invalid Clock fail-closed, tolerance300 inclusive и v1-only неизменны.
   Достаточны checked parse и явная проверка допустимого времени, без нового SDK.
   Добавь регрессии трёх reviewer cases и настоящий invalid-UTF8 raw-byte vector;
   NUL сам по себе валидный UTF-8. Не ослабляй независимые оракулы.
3. Закрой NB-REV-003 существующим CLI: --format text и json меняют представление,
   --format invalid даёт format_invalid/exit2. Выполни опыт и сохрани журнал.
   Не спрашивай Lead/преподавателя про NOT_APPLICABLE, не вводи фиктивный config.
4. Закрой NB-REV-004/005. Фактическая DI-пара — FileResolver+FixedClock и
   MapSecrets+FixedClock; SystemClock не был исполнен этим опытом.
   FileResolver проверялся application tests, CLI использует другой fixture resolver.
   Поправь claims или добавь соответствующий реальный опыт. Coverage не profiling:
   CPU/memory Go pprof укажи PLANNED с объектом/этапом, если он не запускался.
   Исправь normative refs: безопасные ошибки BP007/T/005, secrets/privacy
   BP002/T/006,T/008, clock/deadline BP007/T/010. Сверь каждую из 29 записей:
   actual HIT key, criterion/source, конкретный repo path, expected/actual/check/log/
   fullSHA/CI и human status. module-contract.md лежит в services/intake/docs/.
   Risk low×serious даёт medium по методичке либо требует честной переоценки P/impact
   с основанием; саму матрицу не менять. Известные продуктовые вопросы закрой по BP/NFR.
5. Заверши четыре DOCX: фактические dev/release наблюдения вместо NOT RUN/PASS,
   техническую таблицу 0302 ПР1, выводы/источники, свежие source/CI/coverage.
   Human-only строки честно NOT_CONDUCTED; личные поля не угадывать.
   CONTENT_READY_IMAGES_PENDING только после завершения доступной агентной части.
6. Закрой NB-REV-006: проверь все21 инструкции из чистого checkout — cd/cwd,
   полный argv/источник/SHA/exit/кадр/место вставки/redaction. Старый temp dir удалён:
   дай команды создать новую дистрибуцию. Go tests из services/intake/wrapper,
   не root без go.mod. Исторические evidence не перезаписывать: новые profiles/logs
   в .pr12-build либо новый versioned evidence. На каждом выбранном SHA файл должен
   существовать (карта/отчёты появились позднее C2). Mutation slot получает полный
   маршрут isolated copy и patch apply. PNG не создавать и не генерировать.
7. Выполни actual named tests/vet/race/dev-release smoke/inspect и CI исправленных
   heads. Пять исходных mutation logs/patches сохранить как историю. После изменения
   кода/тестов проведи актуальные ≥5 compiling behavioral mutations, baseline/final
   PASS, build, assertion FAIL либо survivor/retest, hashes и restoration.
   Reviewer probes не выдавать за свои мутации. Hidden suite NOT_PROVIDED.
8. Фиксируй Jira task/branch/real PR/SHA/evidence/actual status. Не имитируй
   официальный Project OS /start,/review,freeze или service-token bridge.
   Если автоматика откатывает manual-lab статус, не обходи её и не повторяй бесконечно:
   сохрани actual result/проблему в комментарии и продолжай разрешённое.
9. Собери correction-record NB-REV-001–006 → commit/PR → regression/evidence,
   финальный source/CI, четыре проверенных DOCX и 29-result crosswalk.
   Передай один полный пакет на независимое повторное ревью Codex по делегации Lead.
   Не сам себе APPROVE, не сливай PR в main, не делай Done/Feature close/submission tags.
   Merge родителей в свои ветки для исправления стека допустим с проверкой конфликтов.

Не отправляй продуктовые трактовки на повторное согласование. v1-only, 300s,
safe errors, запрет считать VERIFIED бизнес-приёмкой уже заданы.
Provider version/events/retention бери из frozen BP/NFR. Receipt/transport — deferred,
реальные permissions — live validation. Интервью, personal student checks, external
opposition, подписи, снимки и защита не подделываются и не останавливают исправления.
```
