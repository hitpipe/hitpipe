# Инструкции для 21 ручного снимка

Агент не создавал и не снимал изображения. Full steps и версии ниже. Ни одна незавершённая человеческая проверка не объявлена выполненной.

## 0302-pz01-tp-01

CAPTION: Рисунок 1 — Требование, задача и запрос на слияние

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 1; ссылка из соответствующего раздела основного текста

SOURCE: https://hitpipe.atlassian.net/browse/HIT-26; https://github.com/hitpipe/hitpipe/pull/4

STATE: НЕЗАВИСИМЫЙ REVIEW/MERGE PENDING

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется

COMMAND: не применяется

STEPS: 1. Открыть HIT-26 в браузере. 2. Открыть PR4, выбрать Files changed. 3. Раскрыть docs/labs/pr12/tp/requirement-change.md и абзац precedence/TP-ERR-01. 4. Показать task key, автора, head/base ветки и фактическое состояние PR. 5. Снять реальное окно вручную; два подрисунка допустимы.

MUST_BE_VISIBLE: Task, branches, author, meaningful before/after, actual open/merged status

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz01-tp-02

CAPTION: Рисунок 2 — Автоматические проверки изменения требований

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 2; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/actions/runs/39120467832

STATE: PASS bundle-only 3aba8f611044ed9d62eb22b11120622af39f78ff

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется

COMMAND: не применяется

STEPS: 1. В evidence/ci/runs.json открыть запись HIT-26 и её фактический URL. 2. Выбрать pr12-processing. 3. Раскрыть Verify bootstrap documentation и actual named-tests step. 4. Показать SHA и что на requirements-only версии module ещё NOT_IMPLEMENTED: зелёный bootstrap не student-code PASS. 5. Для реализованного контракта рядом показать code CI run39121031226.

MUST_BE_VISIBLE: Exact requirement PR SHA/job/check scope; не выдавать bootstrap за tests module

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz01-tp-03

CAPTION: Рисунок 3 — Оппонирование другого проекта и принятое решение

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 3; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/external-opposition.md

STATE: BLOCKED_HUMAN_ASSIGNMENT

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется

COMMAND: не применяется

STEPS: 1. Lead получает настоящее назначение другого проекта у преподавателя. 2. Тимофей разбирает exact чужую спецификацию и участвует в обсуждении своих требований. 3. Записать реальные author/project/date/remark/decision/version и ссылку на дискуссию в human-gates/external-opposition.md. 4. Открыть именно эту записанную дискуссию, раскрыть настоящее замечание и ответ. 5. После этого сделать ручной снимок. До разговора изображение и PASS отсутствуют.

MUST_BE_VISIBLE: Другой реальный проект, author/date, requirement, remark либо настоящий протокол без замечаний, decision/retest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz02-tp-01

CAPTION: Рисунок 1 — Локальный прогон тестов собственного модуля

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 1; ссылка из соответствующего раздела основного текста

SOURCE: Local terminal; evidence/c3/check.log

STATE: PASS source 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go test -count=1 -v ./...

STEPS: 1. В отдельном clean checkout версии из STATE открыть terminal. 2. Показать git rev-parse HEAD и go version. 3. Перейти в services/processing. 4. Выполнить указанную команду, затем echo $?. 5. Оставить имена своих tests и итог в кадре.

MUST_BE_VISIBLE: Full SHA, Go1.27.2, own named tests, PASS and exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz02-tp-02

CAPTION: Рисунок 2 — Тесты сдаваемой версии в CI

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 2; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/actions/runs/39121031226

STATE: PASS CI 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется

COMMAND: не применяется

STEPS: 1. Открыть точный Actions URL. 2. Выбрать pr12-processing. 3. Раскрыть named tests, race/mutation и diagnostics smoke. 4. Показать полный head SHA и success; открыть свой именованный TestFinancialDecisionTable. 5. До final submission проверить новый merge/tag CI через external record.

MUST_BE_VISIBLE: SHA, job, own test, command, actual conclusion

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz02-tp-03

CAPTION: Рисунок 3 — Покрытие собственного модуля и разбор пробела

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 3; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/c3/coverage.log

STATE: PASS statement coverage 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go tool cover -func=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/c3/processing.cover

STEPS: 1. Открыть сохранённый exact profile и coverage-analysis.md. 2. Выполнить команду. 3. Показать domain/application100%, total92.3% и конкретный File.Read/input.walk gap. 4. Показать решение о post-read cancellation/future adapter, а не только процент.

MUST_BE_VISIBLE: Module scope, statement metric, source SHA, uncovered property and decision

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz02-tp-04

CAPTION: Рисунок 4 — Обнаружение одного из засеянных дефектов

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 4; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/mutations/M01.patch; /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/mutations/M01-test.log

STATE: COMPILED/KILLED, source 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 services/processing/tools/mutations/run.py --output .pr12-build/figure-mutations --natural-defects 1 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29

STEPS: 1. Открыть M01.patch, M01-build.log и summary.json: build exit0, patch digest. 2. Открыть M01-test.log и failing refund assertion. 3. Для повторения использовать указанную команду в clean exact checkout: она сама создаёт отдельный временный worktree, меняет по одному и полностью восстанавливает. 4. Снять diff+реальный FAIL, не compile error.

MUST_BE_VISIBLE: Rule006 refund, one patch, build0, failing named test, SHA/patch digest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0302-pz02-tp-05

CAPTION: Рисунок 5 — Успешный прогон после полного восстановления кода

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 5; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/mutations/restored.log

STATE: PASS clean restored 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; git status --short; git rev-parse HEAD; go test -count=1 -v ./...; echo $?

STEPS: 1. Использовать исправный exact checkout, не mutation worktree. 2. Показать git status и full SHA. 3. Выполнить tests и echo exit. 4. Сопоставить с summary clean=true/restored exit0.

MUST_BE_VISIBLE: Clean source, no mutation, own tests PASS exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-01

CAPTION: Рисунок 1 — Собственная часть исполняемого каркаса

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 1; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/blob/4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0/services/processing/cmd/processing-contractcheck/main.go

STATE: SOURCE 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source

COMMAND: git show 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0:services/processing/cmd/processing-contractcheck/main.go

STEPS: 1. Открыть exact GitHub file либо редактор clean checkout. 2. Найти var version/commit/dirty/buildMode, flag fixture/format и run. 3. Показать собственный parser/diagnostic/composition фрагмент и Task HIT-27/PR5.

MUST_BE_VISIBLE: File path, SHA, metadata, config selection, safe output/error handling

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-02

CAPTION: Рисунок 2 — Подготовка компонента в двух режимах

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 2; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/d1/record.json

STATE: PASS build 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-delivery

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 tools/pr12/workspace.py build processing --mode dev --require; python3 tools/pr12/workspace.py build processing --mode release --require

STEPS: 1. Открыть terminal exact source8ae5 и показать fullSHA/Go1.27.2. 2. Выполнить обе команды. 3. Раскрыть actual flags -N -l и -trimpath, linker metadata. 4. Показать exit0 каждого; размеры из delivery-observations.json.

MUST_BE_VISIBLE: Both commands and flags, modes, same source SHA, exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-03

CAPTION: Рисунок 3 — Запуск поставки вне исходного кода и IDE

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 3; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/d1/delivery-observations.json

STATE: PASS outside checkout 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: repo root затем отдельный каталог вне checkout

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; python3 tools/pr12/workspace.py build processing --mode release --require; TP_DELIVERY_DIR=$(mktemp -d /tmp/tp-delivery.XXXXXX); cp .pr12-build/processing/release/processing-contractcheck services/processing/testdata/valid.json "$TP_DELIVERY_DIR/"; cd "$TP_DELIVERY_DIR"; pwd; ./processing-contractcheck --version; ./processing-contractcheck --fixture valid.json --format json; echo $?

STEPS: 1. Выполнить smoke: он собирает оба режима и копирует binary+fixtures во временный каталог. 2. В выводе показать пути pr12-outside-checkout, invocation --version и --fixture. 3. Для ручной демонстрации скопировать release binary и valid.json в новый пустой каталог, перейти туда. 4. Запустить ./processing-contractcheck --version, затем --fixture valid.json --format json и echo $?.

MUST_BE_VISIBLE: Cwd вне source, binary/version/sourceSHA, declared files, VALID exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-04

CAPTION: Рисунок 4 — Штатный запуск компонента

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 4; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/d1/release-normal.log

STATE: PASS 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-delivery

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; .pr12-build/processing/release/processing-contractcheck --version; .pr12-build/processing/release/processing-contractcheck --fixture services/processing/testdata/valid.json --format json; echo $?

STEPS: 1. Собрать release exact source как на рисунке2. 2. Выполнить указанные version и fixture команды. 3. Оставить start/result, VALID/FINANCIAL_ELIGIBILITY_ONLY и exit0 в кадре.

MUST_BE_VISIBLE: Command, fullSHA/version/mode, safe diagnostics, result and exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-05

CAPTION: Рисунок 5 — Неверная конфигурация

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 5; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/d1/release-invalid-config.log

STATE: PASS expected error exit2 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-delivery

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; .pr12-build/processing/release/processing-contractcheck --fixture services/processing/testdata/invalid.json --format json; echo $?

STEPS: 1. Использовать тот же binary/source что normal. 2. Выполнить invalid command без set -e, чтобы показать exit. 3. Повторить с --format invalid при valid fixture. 4. В обоих случаях должно быть понятное INVALID_JSON/CONFIGURATION и nonzero2, без payload.

MUST_BE_VISIBLE: Same version, command, safe error, exit2

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-06

CAPTION: Рисунок 6 — Независимое воспроизведение части другого студента

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 6; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/peer-scaffold-out.md

STATE: HUMAN_PENDING

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется до получения foreign exact artifact

COMMAND: Команды именно из actual README исполнителя HIT-13; Processing commands ему не приписываются

STEPS: 1. Lead получает чужой UX scaffold SHA/README. 2. Тимофей лично готовит clean environment только по нему, фиксирует env/fullSHA. 3. Исполняет объявленные build/version/config/diagnostic commands, сохраняет own results/remarks. 4. После actual retest открыть протокол и terminal; сделать ручной снимок. 5. Отдельно получить incoming check от HIT-11.

MUST_BE_VISIBLE: Реальный проверяющий и другой автор, дата/env/foreignSHA/cwd/commands/results/remarks/retest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-06.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz01-tp-07

CAPTION: Рисунок 7 — Отрицательная локальная проверка автоматического gate

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx, приложение инструкций, после заголовка Рисунок 7; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/mutations/negative-gate.log

STATE: PASS sensitivity: expected nonzero on M01; restored PASS

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 services/processing/tools/mutations/run.py --output .pr12-build/figure-mutations --natural-defects 1 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29

STEPS: 1. Запустить mutation runner в exact clean checkout или открыть сохранённые logs. 2. Показать M01.patch (refund condition changed) и build exit0. 3. Открыть negative-gate.log: python3 tools/pr12/workspace.py check processing --require действительно nonzero из refund assertion. 4. Показать summary negative_gate.exit и restored.exit0/clean=true.

MUST_BE_VISIBLE: Exactly same CI wrapper, violated condition, real assertion FAIL/nonzero, restoration

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-07.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-01

CAPTION: Рисунок 1 — Публичный контракт и существенная реализация

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 1; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/blob/4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0/services/processing/internal/domain/eligibility/eligibility.go

STATE: SOURCE 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source

COMMAND: git show 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0:services/processing/internal/domain/eligibility/eligibility.go

STEPS: 1. Открыть exact file. 2. Развернуть GoDoc Evaluate и финансовую checks table. 3. Показать Result/Policy/Facts и nullable fields. 4. Показать строку financial-only result.

MUST_BE_VISIBLE: Public contract, source path/SHA, actual predicates, financial-only boundary

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-02

CAPTION: Рисунок 2 — Документация модуля по стандарту стека

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 2; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/blob/4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0/services/processing/docs/module-contract.md

STATE: SOURCE 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go doc ./internal/domain/eligibility Evaluate; go doc ./internal/application/evaluatepaidorder EvaluatePaidOrder

STEPS: 1. Показать Go1.27.2 и exact SHA. 2. Выполнить обе go doc команды. 3. В редакторе рядом открыть module-contract.md: purpose/inputs/results/errors/precedence/ownership/effects/example.

MUST_BE_VISIBLE: GoDoc plus source contract, dependencies and owned result semantics

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-03

CAPTION: Рисунок 3 — Один потребитель и две подключаемые реализации

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 3; ссылка из соответствующего раздела основного текста

SOURCE: https://github.com/hitpipe/hitpipe/blob/4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0/services/processing/internal/application/evaluatepaidorder/evaluate_test.go

STATE: PASS same consumer source 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go test -count=1 -v ./internal/application/evaluatepaidorder -run TestReaderSubstitution

STEPS: 1. Открыть test и consumer. 2. Показать File и double selection и одну EvaluatePaidOrder callsite. 3. Выполнить команду. 4. Показать обе реализации и normal/error/boundary, объяснить finite mock limits.

MUST_BE_VISIBLE: One unchanged consumer, two implementations, composition and actual assertions

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-04

CAPTION: Рисунок 4 — Модульная карта с легендой и статусами

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 4; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/module-map.md

STATE: MAP reconciled via c3/imports.log

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go list -f '{{.ImportPath}} => {{join .Imports " "}}' ./...

STEPS: 1. Открыть module-map.md и показать автора. 2. Показать легенду A→B source dependency и actual/planned statuses. 3. Выполнить go list и сопоставить imports; показать отсутствие production cycles и неготовность future adapter.

MUST_BE_VISIBLE: Paths, responsibilities/contracts, import arrows/legend, statuses/author, actual imports

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-05

CAPTION: Рисунок 5 — Проверки с обеими реализациями

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 5; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/c3/check.log

STATE: PASS DI cases source 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/figure-source/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go test -count=1 -v ./internal/application/evaluatepaidorder -run TestReaderSubstitution

STEPS: 1. Выполнить same consumer contract suite. 2. Развернуть file/normal,boundary,error_unknown и double/normal,boundary,error_unknown. 3. Показать expected actual assertions и PASS, завершить echo $?.

MUST_BE_VISIBLE: Minimum six real cases, two implementations, command/SHA/exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

## 0104-pz02-tp-06

CAPTION: Рисунок 6 — Реальная взаимная проверка модуля другого студента

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx, приложение инструкций, после заголовка Рисунок 6; ссылка из соответствующего раздела основного текста

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/peer-module-out.md

STATE: HUMAN_PENDING

SOURCE_SHA: 4a2bfcdd9bf7656a8d5c55963750b8aecf186fb0

CI_SHA: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb

CWD: не применяется до получения foreign exact artifact

COMMAND: Команды actual module README HIT-13, не выдуманный Processing аналог

STEPS: 1. Lead получает exact чужой module/contract/map. 2. Тимофей лично выполняет build, normal/error/boundary/substitution, сверяет imports/map. 3. Сохраняет actual author/date/fullSHA/commands/results/remarks/retest. 4. После реального действия сделать снимок протокола. 5. Отдельно исполнитель HIT-11 проверяет Processing и оставляет настоящий отзыв.

MUST_BE_VISIBLE: Other student/module, real reviewer/date/version, contract/substitution/map results and remarks

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-06.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Проверить полный фактический SHA и actual status. Исторические источники выше не выданы за merged/tagged submission. После human merge/tag использовать внешнюю submission record, повторить проверки на сдаваемом SHA и актуализировать версии; pending human gate сначала реально выполнить. Placeholder не evidence PASS.

