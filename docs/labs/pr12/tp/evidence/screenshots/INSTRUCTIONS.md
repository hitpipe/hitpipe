# Инструкции к 21 настоящему снимку

Снимки выполняет человек; изображения в отчётах пока отсутствуют. Принятый main ec0dcba7ca42ea59909489cef9650c6edff12f35; CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Рендеры страниц DOCX для QA не являются снимками этих проверок.

## 0302-pz01-tp-01

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx: блок «Рисунок 1», заменить единственный абзац плейсхолдера для 0302-pz01-01.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 1 — Требование, задача и запрос на слияние

SOURCE: https://hitpipe.atlassian.net/browse/HIT-26; https://github.com/hitpipe/hitpipe/pull/4; https://github.com/hitpipe/hitpipe/pull/20

STATE: MERGED: PR №4 и №20; отчётный PR №10 OPEN, capture pending

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: не применяется

STEPS: 1. Открыть HIT-26 и связанный PR №4. 2. На Conversation показать Merged и Files changed / requirement-change.md. 3. Для TP-REV-001 открыть PR №20, review APPROVED и Merged. 4. Сверить accepted-pr-state.json и main SHA.

MUST_BE_VISIBLE: Task, branches, author, meaningful before/after, actual open/merged status

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz01-tp-02

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx: блок «Рисунок 2», заменить единственный абзац плейсхолдера для 0302-pz01-02.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 2 — Автоматические проверки изменения требований

SOURCE: https://github.com/hitpipe/hitpipe/actions/runs/39229206910

STATE: SUCCESS accepted main ec0dcba7ca42ea59909489cef9650c6edff12f35

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: не применяется

STEPS: 1. Открыть Actions по URL. 2. Проверить head ec0dcba7… и success. 3. Открыть pr12-processing, named tests и race/mutation steps. 4. В снимке должны быть реальные assertions, SHA и итог, не только зелёная иконка.

MUST_BE_VISIBLE: Accepted main SHA, job pr12-processing, named assertions, command и success; merged PR №4 указан как источник требований

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz01-tp-03

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz01-tp.docx: блок «Рисунок 3», заменить единственный абзац плейсхолдера для 0302-pz01-03.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 3 — Оппонирование другого проекта и принятое решение

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/external-opposition.md

STATE: BLOCKED_HUMAN_ASSIGNMENT

SOURCE_SHA: Собственная reference: ec0dcba7ca42ea59909489cef9650c6edff12f35; чужой SHA НЕ ПОЛУЧЕН

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: Не определён до получения реального чужого артефакта; записывает фактический проверяющий

COMMAND: Применить реальные команды полученного чужого артефакта; exact SHA и окружение фиксирует действительный проверяющий. До получения артефакта команда не выполнена.

STEPS: 1. Lead получает настоящее назначение/чужой пакет. 2. Назначенный человек выполняет предусмотренную проверку и записывает дату, роль, exact чужой SHA, cwd/env/commands/results/remarks/retest. 3. Только после этого открыть заполненный протокол и сделать настоящий снимок. 4. Codex по поручению Lead не выдавать за личную проверку студента; текущий placeholder не свидетельствует о выполнении.

MUST_BE_VISIBLE: Другой реальный проект, author/date, requirement, remark либо настоящий протокол без замечаний, decision/retest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz01-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сначала реальный human gate. Фиксировать actual чужой SHA/CI/env/cwd/commands/дату/проверяющего и результат; их нельзя заменить собственным Processing SHA. Собственная reference ec0dcba7ca42ea59909489cef9650c6edff12f35. После вставки снимков обновить пагинацию/оглавление. Placeholder не PASS.

## 0302-pz02-tp-01

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx: блок «Рисунок 1», заменить единственный абзац плейсхолдера для 0302-pz02-01.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 1 — Локальный прогон тестов собственного модуля

SOURCE: Local terminal; evidence/report-refresh-20261019/accepted-main/check.log

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 tools/pr12/workspace.py check processing --require; echo $?

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. В отдельном clean checkout версии из STATE открыть terminal. 2. Показать git rev-parse HEAD и go version. 3. Открыть терминал в точном CWD выше. 4. Выполнить указанную команду, затем echo $?. 5. Оставить имена своих tests и итог в кадре. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Full SHA, Go1.27.2, own named tests, PASS and exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz02-tp-02

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx: блок «Рисунок 2», заменить единственный абзац плейсхолдера для 0302-pz02-02.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 2 — Тесты сдаваемой версии в CI

SOURCE: https://github.com/hitpipe/hitpipe/actions/runs/39229206910

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: не применяется

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть точный Actions URL. 2. Выбрать pr12-processing. 3. Раскрыть named tests, race/mutation и diagnostics smoke. 4. Показать полный head SHA и success; открыть свой именованный TestFinancialDecisionTable. 5. До final submission проверить новый merge/tag CI через external record. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: SHA, job, own test, command, actual conclusion

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz02-tp-03

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx: блок «Рисунок 3», заменить единственный абзац плейсхолдера для 0302-pz02-03.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 3 — Покрытие собственного модуля и разбор пробела

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-main-extra/coverage.log

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 tools/pr12/workspace.py check processing --require; cd services/processing; go tool cover -func=../../.pr12-build/processing.cover

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть сохранённый exact profile и coverage-analysis.md. 2. Выполнить команду. 3. Показать domain/application100.0%, total91.2% и конкретный File.Read/input.walk gap. 4. Показать решение о post-read cancellation/future adapter, а не только процент. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: main SHA, Go 1.27.2, domain/application 100.0%, service 91.2%, StrictDecode 100.0%, walk 81.0%; statement coverage

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz02-tp-04

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx: блок «Рисунок 4», заменить единственный абзац плейсхолдера для 0302-pz02-04.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 4 — Обнаружение одного из засеянных дефектов

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-mutations/M01-test.log; /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-mutations/summary.json

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 services/processing/tools/mutations/run.py --output .pr12-build/figure-mutations --natural-defects 2 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-27 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть M01.patch, M01-build.log и summary.json: build exit0, patch digest. 2. Открыть M01-test.log и failing refund assertion. 3. Для повторения использовать указанную команду в clean exact checkout: она сама создаёт отдельный временный worktree, меняет по одному и полностью восстанавливает. 4. Снять diff+реальный FAIL, не compile error. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Rule006 refund, one patch, build0, failing named test, SHA/patch digest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0302-pz02-tp-05

INSERT_AT: docs/labs/pr12/tp/reports/0302-pz02-tp.docx: блок «Рисунок 5», заменить единственный абзац плейсхолдера для 0302-pz02-05.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 5 — Успешный прогон после полного восстановления кода

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-mutations/restored.log; /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-mutations/summary.json

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; git status --short; git rev-parse HEAD; cd services/processing; go test -count=1 -v ./...; echo $?

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Использовать исправный exact checkout, не mutation worktree. 2. Показать git status и full SHA. 3. Выполнить tests и echo exit. 4. Сопоставить с summary clean=true/restored exit0. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Clean source, no mutation, own tests PASS exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0302-pz02-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-01

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 1», заменить единственный абзац плейсхолдера для 0104-pz01-01.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 1 — Собственная часть исполняемого каркаса

SOURCE: https://github.com/hitpipe/hitpipe/blob/ec0dcba7ca42ea59909489cef9650c6edff12f35/services/processing/cmd/processing-contractcheck/main.go

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: git show ec0dcba7ca42ea59909489cef9650c6edff12f35:services/processing/cmd/processing-contractcheck/main.go

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть exact GitHub file либо редактор clean checkout. 2. Найти var version/commit/dirty/buildMode, flag fixture/format и run. 3. Показать собственный parser/diagnostic/composition фрагмент и Task HIT-27/PR5. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: File path, SHA, metadata, config selection, safe output/error handling

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-02

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 2», заменить единственный абзац плейсхолдера для 0104-pz01-02.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 2 — Подготовка компонента в двух режимах

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-main-extra/inspect-dev.log; /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-main-extra/inspect-release.log

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 tools/pr12/workspace.py build processing --mode dev --require; python3 tools/pr12/workspace.py build processing --mode release --require

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть terminal exact source ec0dcba7 и показать fullSHA/Go1.27.2. 2. Выполнить обе команды. 3. Раскрыть actual flags -N -l и -trimpath, linker metadata. 4. Показать exit0 каждого; размеры из delivery-observations.json. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Both commands and flags, modes, same source SHA, exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-03

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 3», заменить единственный абзац плейсхолдера для 0104-pz01-03.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 3 — Запуск поставки вне исходного кода и IDE

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-delivery-outside

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-report-evidence/delivery-outside

COMMAND: ./processing-release --version; ./processing-release --fixture valid.json --format json; echo $?

STEPS: 1. Открыть новые delivery-observations.json и соответствующий журнал. 2. Для повторения использовать бинарник delivery-outside/processing-release, собранный из main, и полные synthetic fixtures рядом. 3. Выполнить команду из указанного cwd вне checkout; показать SHA/mode/result/exit. 4. Снять реальное окно терминала; не подменять его текстовым плейсхолдером.

MUST_BE_VISIBLE: Cwd вне source, binary/version/sourceSHA, declared files, VALID exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-04

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 4», заменить единственный абзац плейсхолдера для 0104-pz01-04.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 4 — Штатный запуск компонента

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-delivery-outside

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-report-evidence/delivery-outside

COMMAND: ./processing-release --version; ./processing-release --fixture valid.json --format json; echo $?

STEPS: 1. Открыть новые delivery-observations.json и соответствующий журнал. 2. Для повторения использовать бинарник delivery-outside/processing-release, собранный из main, и полные synthetic fixtures рядом. 3. Выполнить команду из указанного cwd вне checkout; показать SHA/mode/result/exit. 4. Снять реальное окно терминала; не подменять его текстовым плейсхолдером.

MUST_BE_VISIBLE: Command, fullSHA/version/mode, safe diagnostics, result and exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-05

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 5», заменить единственный абзац плейсхолдера для 0104-pz01-05.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 5 — Неверная конфигурация

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-delivery-outside

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-report-evidence/delivery-outside

COMMAND: ./processing-release --version; ./processing-release --fixture invalid.json --format json; echo $?

STEPS: 1. Открыть новые delivery-observations.json и соответствующий журнал. 2. Для повторения использовать бинарник delivery-outside/processing-release, собранный из main, и полные synthetic fixtures рядом. 3. Выполнить команду из указанного cwd вне checkout; показать SHA/mode/result/exit. 4. Снять реальное окно терминала; не подменять его текстовым плейсхолдером.

MUST_BE_VISIBLE: Same version, command, safe error, exit2

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz01-tp-06

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 6», заменить единственный абзац плейсхолдера для 0104-pz01-06.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 6 — Независимое воспроизведение части другого студента

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/peer-scaffold-out.md

STATE: HUMAN_PENDING

SOURCE_SHA: Собственная reference: ec0dcba7ca42ea59909489cef9650c6edff12f35; чужой SHA НЕ ПОЛУЧЕН

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: Не определён до получения реального чужого артефакта; записывает фактический проверяющий

COMMAND: Применить реальные команды полученного чужого артефакта; exact SHA и окружение фиксирует действительный проверяющий. До получения артефакта команда не выполнена.

STEPS: 1. Lead получает настоящее назначение/чужой пакет. 2. Назначенный человек выполняет предусмотренную проверку и записывает дату, роль, exact чужой SHA, cwd/env/commands/results/remarks/retest. 3. Только после этого открыть заполненный протокол и сделать настоящий снимок. 4. Codex по поручению Lead не выдавать за личную проверку студента; текущий placeholder не свидетельствует о выполнении.

MUST_BE_VISIBLE: Реальный проверяющий и другой автор, дата/env/foreignSHA/cwd/commands/results/remarks/retest

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-06.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сначала реальный human gate. Фиксировать actual чужой SHA/CI/env/cwd/commands/дату/проверяющего и результат; их нельзя заменить собственным Processing SHA. Собственная reference ec0dcba7ca42ea59909489cef9650c6edff12f35. После вставки снимков обновить пагинацию/оглавление. Placeholder не PASS.

## 0104-pz01-tp-07

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz01-tp.docx: блок «Рисунок 7», заменить единственный абзац плейсхолдера для 0104-pz01-07.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 7 — Отрицательная локальная проверка автоматического gate

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-mutations/negative-gate.log

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; python3 services/processing/tools/mutations/run.py --output .pr12-build/figure-mutations --natural-defects 2 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-27

STEPS: 1. Открыть сохранённые M01.patch, M01-build.log, negative-gate.log и summary.json нового опыта. 2. Показать build exit 0, wrapper exit 1 из-за failing assertion и полный source SHA. 3. Restored exit 0/clean=true показать отдельно. 4. Если нужен новый запуск, команда создаёт disposable worktree; не менять основной checkout.

MUST_BE_VISIBLE: Exactly same CI wrapper, violated condition, real assertion FAIL/nonzero, restoration

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz01-07.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-01

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 1», заменить единственный абзац плейсхолдера для 0104-pz02-01.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 1 — Публичный контракт и существенная реализация

SOURCE: https://github.com/hitpipe/hitpipe/blob/ec0dcba7ca42ea59909489cef9650c6edff12f35/services/processing/internal/domain/eligibility/eligibility.go

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: git show ec0dcba7ca42ea59909489cef9650c6edff12f35:services/processing/internal/domain/eligibility/eligibility.go

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть exact file. 2. Развернуть GoDoc Evaluate и финансовую checks table. 3. Показать Result/Policy/Facts и nullable fields. 4. Показать строку financial-only result. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Public contract, source path/SHA, actual predicates, financial-only boundary

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-01.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-02

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 2», заменить единственный абзац плейсхолдера для 0104-pz02-02.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 2 — Документация модуля по стандарту стека

SOURCE: https://github.com/hitpipe/hitpipe/blob/ec0dcba7ca42ea59909489cef9650c6edff12f35/services/processing/docs/module-contract.md

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go doc ./internal/domain/eligibility Evaluate; go doc ./internal/application/evaluatepaidorder EvaluatePaidOrder

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Показать Go1.27.2 и exact SHA. 2. Выполнить обе go doc команды. 3. В редакторе рядом открыть module-contract.md: purpose/inputs/results/errors/precedence/ownership/effects/example. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: GoDoc plus source contract, dependencies and owned result semantics

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-02.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-03

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 3», заменить единственный абзац плейсхолдера для 0104-pz02-03.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 3 — Один потребитель и две подключаемые реализации

SOURCE: https://github.com/hitpipe/hitpipe/blob/ec0dcba7ca42ea59909489cef9650c6edff12f35/services/processing/internal/application/evaluatepaidorder/evaluate_test.go

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go test -count=1 -v ./internal/application/evaluatepaidorder -run TestReaderSubstitution

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть test и consumer. 2. Показать File и double selection и одну EvaluatePaidOrder callsite. 3. Выполнить команду. 4. Показать обе реализации и normal/error/boundary, объяснить finite mock limits. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: One unchanged consumer, two implementations, composition and actual assertions

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-03.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-04

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 4», заменить единственный абзац плейсхолдера для 0104-pz02-04.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 4 — Модульная карта с легендой и статусами

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/module-map.md

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main/services/processing

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; go list -f '{{.ImportPath}} => {{join .Imports " "}}' ./...

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Открыть module-map.md и показать автора. 2. Показать легенду A→B source dependency и actual/planned statuses. 3. Выполнить go list и сопоставить imports; показать отсутствие production cycles и неготовность future adapter. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Paths, responsibilities/contracts, import arrows/legend, statuses/author, actual imports

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-04.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-05

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 5», заменить единственный абзац плейсхолдера для 0104-pz02-05.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 5 — Проверки с обеими реализациями

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/evidence/report-refresh-20261019/accepted-main/check.log

STATE: PASS_RECORDED_SCOPE; main ec0dcba7ca42ea59909489cef9650c6edff12f35; CAPTURE_PENDING

SOURCE_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit32-accepted-main

COMMAND: export PATH=/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin:$PATH; go version; git rev-parse HEAD; cd services/processing; go test -count=1 -v ./internal/application/evaluatepaidorder; echo $?

STEPS: 1. Открыть указанный источник и сверить SHA ec0dcba7ca42. 1. Выполнить same consumer contract suite. 2. Развернуть file/normal,boundary,error_unknown и double/normal,boundary,error_unknown. 3. Показать expected actual assertions и PASS, завершить echo $?. После настоящей проверки человек делает PNG; агент изображение для отчёта не создавал.

MUST_BE_VISIBLE: Minimum six real cases, two implementations, command/SHA/exit0

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-05.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сверить actual SHA ec0dcba7ca42ea59909489cef9650c6edff12f35 и CI https://github.com/hitpipe/hitpipe/actions/runs/39229206910. Placeholder не PASS. Для human gate сначала выполнить настоящую проверку с участниками/датой/своим SHA. После принятия отчётов и реальных снимков заново проверить пагинацию/оглавление; финальный tag→commit→CI записывает уполномоченный человек.

## 0104-pz02-tp-06

INSERT_AT: docs/labs/pr12/tp/reports/0104-pz02-tp.docx: блок «Рисунок 6», заменить единственный абзац плейсхолдера для 0104-pz02-06.png. Подпись указана в CAPTION; инструкции SOURCE/STATE/COMMAND сохранить до вставки.

CAPTION: Рисунок 6 — Реальная взаимная проверка модуля другого студента

SOURCE: /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hitpipe/docs/labs/pr12/tp/human-gates/peer-module-out.md

STATE: HUMAN_PENDING

SOURCE_SHA: Собственная reference: ec0dcba7ca42ea59909489cef9650c6edff12f35; чужой SHA НЕ ПОЛУЧЕН

CI_SHA: ec0dcba7ca42ea59909489cef9650c6edff12f35

CWD: Не определён до получения реального чужого артефакта; записывает фактический проверяющий

COMMAND: Применить реальные команды полученного чужого артефакта; exact SHA и окружение фиксирует действительный проверяющий. До получения артефакта команда не выполнена.

STEPS: 1. Lead получает настоящее назначение/чужой пакет. 2. Назначенный человек выполняет предусмотренную проверку и записывает дату, роль, exact чужой SHA, cwd/env/commands/results/remarks/retest. 3. Только после этого открыть заполненный протокол и сделать настоящий снимок. 4. Codex по поручению Lead не выдавать за личную проверку студента; текущий placeholder не свидетельствует о выполнении.

MUST_BE_VISIBLE: Other student/module, real reviewer/date/version, contract/substitution/map results and remarks

TARGET_FILE: docs/labs/pr12/tp/evidence/screenshots/0104-pz02-06.png

REDACT: Скрыть tokens/passwords/cookies, личную почту и соседние окна. Не скрывать task/SHA/command/assertion/exit. Не показывать реальные payment/customer payloads; использовать только синтетические fixtures.

VERIFY_BEFORE_INSERT: Сначала реальный human gate. Фиксировать actual чужой SHA/CI/env/cwd/commands/дату/проверяющего и результат; их нельзя заменить собственным Processing SHA. Собственная reference ec0dcba7ca42ea59909489cef9650c6edff12f35. После вставки снимков обновить пагинацию/оглавление. Placeholder не PASS.

