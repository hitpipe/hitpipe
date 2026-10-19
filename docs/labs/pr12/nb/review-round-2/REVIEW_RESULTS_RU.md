# HIT-15 — повторное ревью исправлений Никиты Будыкина

19.10.2026, Москва. Командный проверяющий — Дмитрий Карачебан; фактический
code/document/source review — Codex по прямой делегации.
**Вердикт: CHANGES_REQUIRED. Итоговый код исправлен; весь комплект пока не принят.**

Проверенный пакет: PR34, **3fdd78828a69bf788b7a2da9aa3acf58a5e1531a**.
Три code PR: [31](https://github.com/hitpipe/hitpipe/pull/31)
8634767abf8fa235407db3581a8c38705bbdebb8,
[32](https://github.com/hitpipe/hitpipe/pull/32)
2a20fb6a21fe762f2cf37ded433c99bf5390be81,
[33](https://github.com/hitpipe/hitpipe/pull/33)
df1a7d58179148abfda8e71862654f5ef7fcdbc7.
Полный [PR34](https://github.com/hitpipe/hitpipe/pull/34) — документация и пакет.
Это exact-source review, не formal Project OS acceptance, student evidence или merge.

## Что подтверждено повторно

- Frozen bundle:205 sources и17 wrapper tests PASS.
- **Intake named tests, vet, race, dev/release outside-checkout smoke,
  release inspection** — PASS. Отдельно видны успешные native CI контексты
  pr12-iam/intake/processing/delivery соответствующих PR.
- Три первоначальных reviewer regression oracles теперь PASS на PR34.
- 49 freshness cases с точной Python integer арифметикой и независимо
  подписанными HMAC, плюс четыре исходных независимых вектора, включая
  invalid UTF-8 и MaxInt64, PASS. Это не авторские мутации.
- Тот же набор трёх regression oracles с policy bytes **из PR31** всё ещё
  даёт три assertion FAIL. Checkout при этом PR34: применён readonly Go
  overlay policy PR31, а не объявлен новый нативный CI PR31.
- CLI: text/json/invalid дают соответственно **0/0/2**; safe format_invalid.
- Все27 текущих report/evidence digests совпадают:4 DOCX и23 evidence files.
- Четыре DOCX проходят ZIP/XML разбор, содержат7+6+3+5=21 мест для снимков
  без media. Reviewer renderer недоступен; визуальный просмотр не заявляется.
- Coverage полного модуля55.8% statements. Новый процентный gate не вводится.
- Production source и авторские тесты reviewer-ом не менялись; restoration
  hashes совпали после каждого исполняемого check.

Подробности: [журналы](evidence/review-checks.json),
[проверка артефактов](evidence/artifact-verification.json),
[содержимое таблиц DOCX](evidence/docx-review.json),
[аудит crosswalk](evidence/crosswalk-audit.json).
Исходники проверок — [Go overlay tests](probes/domain.go.txt).

## Статус прежних замечаний

| ID | Результат повторной проверки |
| --- | --- |
| NB-REV-001 | Исправлено в PR33/34: checked int64 parse, positive clock, безопасная дистанция. Но текущие головы PR31/32 ещё уязвимы; перед последовательным merge исправление должно появиться уже в C1 и быть подтянуто детьми. |
| NB-REV-002 | Частично: три code PR и пакет PR34 созданы, ссылки в Jira есть. В submission-record нет собственных PR для HIT49/50/54/55, а инструкция0302-pz01-01 всё ещё ждёт создания R2 PR от Lead. Полный task→branch→PR учёт и реальный significant R2 workflow не завершены. |
| NB-REV-003 | Поведение подтверждено и закрыто повторным выполнением text/json/invalid. Представление этого опыта в отчёте/инструкции исправляется в004/006. |
| NB-REV-004 | Открыто: DI и planned pprof поправлены, но технические таблицы отчётов остались незавершёнными. |
| NB-REV-005 | Частично: исходный requirements-trace и CI risk исправлены. DOCX всё ещё содержит старые нормативные ссылки, crosswalk использует role IDs без actual HIT keys и неправильно назначает human gate техническому0302-pz01/Р4. |
| NB-REV-006 | Открыто:21 секция есть, но часть команд неисполнима/неполна; конкретные repro ниже. |

## NB-REV-001 / PR31 — поправить самый ранний исполняемый head

PR33/34 содержат правильную policy; PR31 и унаследовавший её PR32 не обновлены.
В PR31 verifier.go:164–165 всё ещё multiplication с проверкой next < parsed,
а строка75 вычитает signed delta. Повторяются исходные три дефекта.
Не считать code approval позднего пакета разрешением самостоятельно слить
неисправленный C1. Backport минимального fix и его регрессий в C1, обычные
merge родителей в дочерние ветки без force-push; повторить затронутые checks.
Это не требование нового продукта или иной архитектуры.

## NB-REV-004 — незавершённые технические отчёты

**0104-pz01-nb.docx, таблица «Вопрос / Режим разработки / Режим поставки»:**
все12 содержательных cells по-прежнему содержат одинаковое
«NOT RUN / PASS; ...hit-54-d1-clean.log» вместо реальных различий и наблюдений.
Ни факт новых bytes DOCX, ни замена DI-слов не заполняют эту таблицу.

**0302-pz01-nb.docx, таблица «Предмет проверки / Способ и условия /
Фактический результат / Версия и доказательство / Ограничения»:**
единственная data row имеет все5 пустых cells. Пустыми остаются таблица
индивидуального результата и большая часть итоговой карточки материалов/PR/CI.
Реальные code/build/tests уже доступны: заполнить их, не сочиняя интервью,
peer answers, подписи и внешний opposition.

Генератор tools/build_pr12_reports.py необходимо исправить вместе с DOCX,
иначе следующая пересборка вернёт дефекты. Он также записывает FR-SIG-03
и NFR-SIG-01 через BP007/T010 (строки251/253), хотя requirements-trace уже
исправлен на safe errors BP007/T005 и privacy BP002/T006,T008.
Провести содержательную сверку всех четырёх экспортов с каноническими данными;
ZIP/XML/hash проверка не ловит эти ошибки.

## NB-REV-005 — нормативные связи и статус критериев

В source JSON ссылки исправлены, но неверная копия в DOCX создаёт два разных
основания одного требования. Канонический владелец — requirements-trace;
в отчёт переносить тот же source, не собственную устаревшую константу.

Все29 rows crosswalk существуют, но ни один tasks field не содержит actual
HIT key. Добавить однозначные real task/PR mappings и конкретные результаты.
У0302-pz01/Р4 human_gate=true и PARTIAL_HUMAN_PENDING, хотя frozen
METHODICS_MATRIX задаёт gate=none: это технические fixtures/security, а не
личная внешняя процедура. Исправить semantically, не делать весь crosswalk
зелёным автоматически. Настоящие student/human gates сохраняются pending.

## NB-REV-006 — инструкции не проходят воспроизведение

- INSTRUCTIONS.md:131,0104-pz01-05: приписка «(expected exit 2)» включена
  в executable shell command. **bash -n возвращает2**, syntax error near '('.
  Отделить ожидаемый exit от команды и записывать реальные0/0/2.
- Там же CWD задан как repo root, а ./intake-contractcheck и verified.json
  лежат в созданном вне checkout distribution. Указать именно distribution
  cwd или абсолютные пути, с полными шагами сборки/копирования.
- Строки171/191 оставляют буквальный <SHA> в командах. Установить точный
  доступный pin или получить full SHA программно; путь проверять на нём.
- Строка10 всё ещё ждёт создания HIT50 PR от Lead. Публикация доступна автору.
- Для actual mutation screenshot нужен применимый patch к указанному baseline,
  а не прежний M04, который на исправленный source уже не накладывается.

Проверить все21 route с реальными cwd, командами, доступным исходником и
ожидаемым выходом. Placeholder-only для самого PNG остаётся правильным решением.

## NB-REV-007 · P2 · Current mutation experiment не подтверждён

Это оставшийся verification scope после изменения кода, явно указанный в
первом author correction prompt, пункт7; не новая количественная норма.
Все пять retained logs и protocol относятся к baseline
**835bbb1bc565e9b2786d5ed1a2cac166f816f7a8**, policy digest
**c48c08bece0cc13e11e8409dd0ad5a58eff47551d84eb7959957eb5c24052510**.
Текущий policy digest:
**0cc138639531dae97ccff253208e494fe39ac7029bb16bc3bcc046752c964640**.

Dry-run M04 против копии исправленного verifier даёт **exit1,
Hunk #1 FAILED at73**. В patch остался старый delta block, которого больше нет.
Это ошибка применения patch, **не killed compiling behavioural mutant**.
Исторические пять опытов остаются действительными для старого baseline;
они не проверяют текущую версию после числовой правки. Текущий workflow
вообще не запускает mutation experiment; зелёный CI не доказывает его повтор.

Сохранить старую серию, выполнить актуальные >=5 meaningful compiling mutations
на зафиксированном исправленном source/test baseline с build0, assertion FAIL
либо честным survivor/retest, real logs/digests и restoration/final PASS.
Новые M03/M04 должны изменять действующую outsideTolerance policy.
Обновить protocol, report, crosswalk, screenshot route и evidence index.

## Дальнейшие действия

[Готовое единое поручение авторскому агенту](AUTHOR_CORRECTION_PROMPT_RU.md).
Права менять не требуется; дополнительных продуктовых решений Lead нет.
Повторное review после исполнения оценивает новые exact heads и затронутые
проверки, а не запускает заново исследования/архитектуру/весь продукт.

Личные student checks, интервью, оппонирование другого проекта, PNG, подписи,
защита и formal acceptance этим review не подтверждаются. Merge, tags, Done
и закрытие Feature не выполнены.
