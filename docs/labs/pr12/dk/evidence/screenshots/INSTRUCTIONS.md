# DK — 21 человеческий снимок, агент изображения не создаёт

Ниже точные версии существующих артефактов. Предварительный код-кандидат не является итоговым submission tag. После human merge/source rewrite использовать новый фактический submission-record и заново снять затронутые результаты; не переносить старый SHA молча. Pending human gate не превращается в PASS наличием placeholder.

## 0302-pz01-01 — Рисунок 1 — Требование, задача и значимый запрос на слияние

Место: 0302 ПР1 §3.2 после описания изменения; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Браузер → https://hitpipe.atlassian.net/browse/HIT-18 → описание; рядом https://github.com/hitpipe/hitpipe/pull/13 → Files changed → requirement-change.md и local contract.

Версия: 9d7c37151606656e9dc2f2f65876b8bd93b397cd

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Открыть PR #13, сверить head 9d7c37151606656e9dc2f2f65876b8bd93b397cd и base #12. Показать before/after правила equality/freshness, Task key и реальный статус review. Approval/merge сейчас pending.

В кадре: HIT-18; PR #13; полный head SHA; существенное уточнение contract; реальный статус без выдуманного approval

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz01-01.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz01-02 — Рисунок 2 — Автоматические проверки изменения требований

Место: 0302 ПР1 §4 после таблицы проверок; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/pull/13 → Checks → pr12-iam; прямой URL https://github.com/hitpipe/hitpipe/actions/runs/39175925989/job/117818455616

Версия: 9d7c37151606656e9dc2f2f65876b8bd93b397cd

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Раскрыть Verify bootstrap documentation and frozen copies и Report actual implementation state. Это R2 до кода: green source validation, а student module NOT_IMPLEMENTED/NOT_RUN; не подписывать как тест готового IAM.

В кадре: R2 head 9d7c3715...; название job; source checks SUCCESS; NOT_IMPLEMENTED на этой исторической версии

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz01-02.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz01-03 — Рисунок 3 — Оппонирование другого проекта и принятое решение

Место: 0302 ПР1 §3.3 после честного статуса оппонирования; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: HUMAN_ACTION_PENDING: до реального действия снимок не доказывает результат; запрещено фотографировать незаполненный протокол как PASS.

Навигация: https://github.com/hitpipe/hitpipe/blob/efa7d1860f09bf8b28ec239896780257fbfe559a/docs/labs/pr12/dk/human-gates.md → §5; после реального обсуждения открыть URL обсуждения, который человек внесёт в review/peer-protocol.json.

Версия: efa7d1860f09bf8b28ec239896780257fbfe559a

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Сейчас НЕ СНИМАТЬ вымышленное обсуждение. Lead получает назначение преподавателя и внешний проект; DK проводит обсуждение, фиксирует actual URL/дату/участников/версии/замечания/решение/изменение. Затем открыть этот реальный URL и показать связанные ID требований. Отсутствующий будущий URL честно не подставлен.

В кадре: другой проект и реальные участники; дата; рассмотренная версия; конкретное замечание; решение/изменение или обоснованный отказ

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz01-03.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz02-01 — Рисунок 1 — Локальный прогон собственных IAM-тестов

Место: 0302 ПР2 §4.1 после локальных результатов; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал → отдельный clone, checkout 2cd7c032e8ede98c75181599b440e97657bff197; исходный evidence https://github.com/hitpipe/hitpipe/blob/2cd7c032e8ede98c75181599b440e97657bff197/docs/labs/pr12/dk/evidence/c3-final-check.log (локальный лог на идентичном Go source 5b14ab92d1ca2a68f46fd3bda783b188549fbf5a).

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git fetch origin 2cd7c032e8ede98c75181599b440e97657bff197; git switch --detach 2cd7c032e8ede98c75181599b440e97657bff197; git rev-parse HEAD; go version; PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require; printf "exit=%s\n" "$?". Capture реальный новый run; не выдавать прошлый log за новый.

В кадре: полный SHA; Go1.27.2; TestEvaluateIdleBoundary; TestSameConsumerContractWithBothAdapters; реальный итог и exit0

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz02-01.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz02-02 — Рисунок 2 — Собственные тесты и mutations в CI

Место: 0302 ПР2 §4.1 после CI результата; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/actions/runs/39177171258/job/117822118583 → job pr12-iam → Run actual named tests / race → Compile isolated IAM mutations.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Открыть run 39177171258 для 2cd7c032e8ede98c75181599b440e97657bff197. Показать checkout SHA, собственные test names, итог race и M01–M07 KILLED, final gate. Во вкладке summary доступны artifacts pr12-iam-2cd7c032e8ede98c75181599b440e97657bff197.

В кадре: exact head SHA; pr12-iam; собственные tests; все семь compiling mutants; SUCCESS

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz02-02.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz02-03 — Рисунок 3 — Покрытие IAM и конкретный непроверенный участок

Место: 0302 ПР2 §4.2 после анализа покрытия; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал на 2cd7c032e8ede98c75181599b440e97657bff197; файл docs/labs/pr12/dk/evidence/iam-all.cover и test-results.md.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: cd services/iam; go tool cover -func=../../docs/labs/pr12/dk/evidence/iam-all.cover; go tool cover -html=../../docs/labs/pr12/dk/evidence/iam-all.cover -o ../../.pr12-build/iam-coverage.html. Открыть локальный HTML в браузере → cmd/iam-contractcheck/main.go → main entrypoint; рядом test-results.md с решением. Не утверждать branch coverage.

В кадре: имя IAM module; 94.5% statements; core Evaluate/Execute 100% statements; main entrypoint/конкретный gap; обоснованное решение

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz02-03.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz02-04 — Рисунок 4 — Компилируемый дефект M01 обнаружен тестами

Место: 0302 ПР2 §4.3 после mutation таблицы; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: На 2cd7c032e8ede98c75181599b440e97657bff197 открыть docs/labs/pr12/dk/evidence/mutations-initial/M01.patch, protocol.json и M01-check.log.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git rev-parse HEAD; cat docs/labs/pr12/dk/evidence/mutations-initial/M01.patch; python3 -m json.tool docs/labs/pr12/dk/evidence/mutations-initial/protocol.json; rg -n -C 2 "TestEvaluateIdleBoundary/equal|FAIL" docs/labs/pr12/dk/evidence/mutations-initial/M01-check.log. Оригинальный experiment source462d553e8373af6047f0950e0a9adfe15e85c83f, temporary branch записана в protocol и уже удалена после восстановления.

В кадре: M01 diff >= заменён на >; build exit0; та же wrapper command; failing named assertion; test exit1; source/patch digests

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz02-04.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0302-pz02-05 — Рисунок 5 — Успешный прогон после восстановления

Место: 0302 ПР2 §4.3 после итогов серии; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал → clean clone 2cd7c032e8ede98c75181599b440e97657bff197; исходный final-clean.log в mutations-M07-retest.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git status --short; git rev-parse HEAD; python3 tools/pr12/workspace.py check iam --require; printf "exit=%s\n" "$?"; python3 -m json.tool docs/labs/pr12/dk/evidence/mutations-M07-retest/protocol.json. Показать restored=true и final_clean.exit_code0. Не применять patch к сдаваемой ветке.

В кадре: clean source; полный SHA; реальные named tests PASS; exit0; restored hash и clean flag

Файл: docs/labs/pr12/dk/evidence/screenshots/0302-pz02-05.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-01 — Рисунок 1 — Собственная часть исполняемого каркаса

Место: 0104 ПР1 §3.1 после маршрута PR; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/pull/14 → Files changed → services/iam/cmd/iam-contractcheck/main.go; current module https://github.com/hitpipe/hitpipe/tree/2cd7c032e8ede98c75181599b440e97657bff197/services/iam

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Показать C1 PR #14, scope собственного module и actual current entrypoint. Через вкладку Commits сверить C1 0ead65c2a7bd767841659c0f26f66edd7d763cfb; позже C2 consumer wiring существует на 2cd7c032e8ede98c75181599b440e97657bff197.

В кадре: Task HIT-19; отдельный PR; свой entrypoint; реальный code; состояние review pending

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-01.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-02 — Рисунок 2 — Подготовка одного компонента dev/release

Место: 0104 ПР1 §3.3 после сравнения режимов; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал → отдельный clone 2cd7c032e8ede98c75181599b440e97657bff197, root.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git rev-parse HEAD; go version; python3 tools/pr12/workspace.py build iam --mode dev --require; python3 tools/pr12/workspace.py build iam --mode release --require; .pr12-build/iam/dev/iam-contractcheck --version; .pr12-build/iam/release/iam-contractcheck --version; file .pr12-build/iam/dev/iam-contractcheck .pr12-build/iam/release/iam-contractcheck. Зафиксировать actual размеры своего нового build отдельно от сохранённого observation.

В кадре: тот же SHA/input; -N -l и -trimpath; оба binary paths; mode/dirty metadata; ELF тип

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-02.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-03 — Рисунок 3 — Поставка запущена вне исходников

Место: 0104 ПР1 §3.3 после outside-checkout результата; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал root clean clone 2cd7c032e8ede98c75181599b440e97657bff197.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git rev-parse HEAD; python3 tools/pr12/workspace.py smoke iam --require; printf "exit=%s\n" "$?". В журнале показать pr12-outside-checkout temporary path, --version, fixture и PASS обоих режимов. Wrapper реально копирует binary+fixtures, не запускает go run из checkout.

В кадре: outside-checkout path; copied executable; версия; positive input; оба режима PASS; exit0

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-03.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-04 — Рисунок 4 — Штатный диагностический запуск

Место: 0104 ПР1 §4.1 после таблицы component checks; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал root clean clone 2cd7c032e8ede98c75181599b440e97657bff197 после release build.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: .pr12-build/iam/release/iam-contractcheck --version; .pr12-build/iam/release/iam-contractcheck --fixture services/iam/testdata/valid.json --format json; printf "exit=%s\n" "$?".

В кадре: source SHA; synthetic fixture path; event=start/end; state valid/reason session_valid; exit0

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-04.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-05 — Рисунок 5 — Неверная конфигурация и безопасная диагностика

Место: 0104 ПР1 §4.1 после ошибочного случая; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал того же release build 2cd7c032e8ede98c75181599b440e97657bff197.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: .pr12-build/iam/release/iam-contractcheck --fixture services/iam/testdata/malformed.json --format json; printf "exit=%s\n" "$?"; .pr12-build/iam/release/iam-contractcheck --fixture services/iam/testdata/valid.json --format invalid; printf "exit=%s\n" "$?". Без set -e, иначе shell остановится до вывода exit.

В кадре: оба неверных ввода; invalid_config; exit2; нет содержимого fixture/секретов

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-05.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-06 — Рисунок 6 — Реальное независимое воспроизведение чужой части

Место: 0104 ПР1 §4.2 после статуса peer gate; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: HUMAN_ACTION_PENDING: до реального действия снимок не доказывает результат; запрещено фотографировать незаполненный протокол как PASS.

Навигация: https://github.com/hitpipe/hitpipe/pull/10 → TP candidate adfbfc33bf20cb0b0641e49e48c0502d2f5d108d; H1 instructions https://github.com/hitpipe/hitpipe/blob/efa7d1860f09bf8b28ec239896780257fbfe559a/docs/labs/pr12/dk/human-gates.md → §4.

Версия: adfbfc33bf20cb0b0641e49e48c0502d2f5d108d

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Дмитрий лично клонирует TP exact candidate в чистую среду и выполняет validate/check processing/smoke processing по H1. После реального выполнения заполнить reviewer/date/env/fullSHA/argv/exit/remarks/retest в review/peer-protocol.json и показать свой terminal+protocol. Агент этого student action не выполнял. Для обратной стороны Тимофей Патрушев проверяет DK в отдельном чистом checkout текущего head PR #19 и фиксирует actual SHA по review/TIMOFEY_REVIEW_GUIDE_RU.md.

В кадре: кто реально проверял; чужой Processing component; полный TP SHA; чистая среда; actual commands/result/remarks/retest

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-06.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz01-07 — Рисунок 7 — Та же команда CI отвергает нарушенное условие

Место: 0104 ПР1 §4.3 после отрицательного gate; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Сохранённые M01-build.log, M01-check.log, protocol.json и final-clean.log на 2cd7c032e8ede98c75181599b440e97657bff197.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Открыть .github/workflows/pr12-dk.yml и protocol.json рядом. Показать одинаковую команду python3 tools/pr12/workspace.py check iam --require, build0, нарушенный idle equality→test1, затем восстановленный0. Это настоящий локальный negative gate с CI command, не поддельный красный CI run.

В кадре: одна и та же command; компилируемый M01; незелёный assertion; восстановленный PASS; exact source

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz01-07.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-01 — Рисунок 1 — Публичный контракт и существенная реализация

Место: 0104 ПР2 §3.2 после реализации; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/blob/2cd7c032e8ede98c75181599b440e97657bff197/services/iam/internal/domain/sessionpolicy/policy.go и /services/iam/internal/application/authenticatesession/service.go.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Открыть Evaluate и Service.Execute, затем CLI call-site. Показать pure values, idle/absolute equality, ports и реальное подключение. Рядом PR14/15 с отдельными Task keys.

В кадре: Evaluate; Execute; портовая граница; реальная ветвящаяся логика; вызов consumer

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-01.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-02 — Рисунок 2 — Документация модуля по стандарту Go

Место: 0104 ПР2 §3.1 после контракта; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал → корень отдельного чистого clone hitpipe/hitpipe на 2cd7c032e8ede98c75181599b440e97657bff197. Go-команды явно выбирают services/iam через -C; файл контракта открывать от корня.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: go -C services/iam doc ./internal/domain/sessionpolicy; go -C services/iam doc ./internal/application/authenticatesession; открыть services/iam/docs/module-contract.md. Показать purpose/input/output/errors/ownership/side effects и пример, не только список сигнатур.

В кадре: source SHA; Go package docs; публичные типы/операции; контракт и example

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-02.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-03 — Рисунок 3 — Один consumer и две подключаемые реализации

Место: 0104 ПР2 §3.3 после substitution; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/blob/2cd7c032e8ede98c75181599b440e97657bff197/services/iam/cmd/iam-contractcheck/main.go и internal/adapters/fixtures/sessionlookup/adapters.go.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Показать var lookup authenticatesession.SessionLookup, выбор NewFile/NewFixed и один service.Execute без case внутри consumer. Затем release CLI с --lookup file и --lookup fixed на valid fixture; оба результата session_valid. Ограничения synthetic reference и frozen clock оставить видимыми в README.

В кадре: один Execute; две реализации; одна composition root; одинаковые результаты; ограничения подмены

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-03.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-04 — Рисунок 4 — Модульная карта с импортами, автором и статусами

Место: 0104 ПР2 §3.4 после карты; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: https://github.com/hitpipe/hitpipe/blob/9a8a59cbc762c10b15abf5d119435e08ecc2c752/docs/labs/pr12/dk/module-map.md → render Mermaid и таблица.

Версия: 9a8a59cbc762c10b15abf5d119435e08ecc2c752

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Открыть module-map.md в GitHub (не Raw), показать схему, legend A imports B, owner и HUMAN_REVIEW_PENDING. Сверить с cd services/iam && go list -json ./... на CODE2cd7c03. Не путать import arrows с runtime call flow.

В кадре: 5 packages; верные стрелки; легенда; автор/статус; source pin

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-04.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-05 — Рисунок 5 — Штатный, ошибочный и граничный случай обеих реализаций

Место: 0104 ПР2 §4.1 после результатов; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: IMAGE_PENDING: фактическое текстовое evidence уже существует; снимок делает человек.

Навигация: Терминал → корень отдельного чистого clone hitpipe/hitpipe на 2cd7c032e8ede98c75181599b440e97657bff197. Go-команды явно выбирают services/iam через -C; файл контракта открывать от корня.

Версия: 2cd7c032e8ede98c75181599b440e97657bff197

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: git rev-parse HEAD; go -C services/iam test ./internal/adapters/fixtures/sessionlookup -run TestSameConsumerContractWithBothAdapters -v -count=1; printf "exit=%s\n" "$?". Показать file/fixed normal,error,idle_equal,unknown,revoked — это реальные subtests одного consumer.

В кадре: полный SHA; file и fixed; normal/error/boundary names; PASS; exit0

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-05.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.

## 0104-pz02-06 — Рисунок 6 — Реальная взаимная проверка чужого модуля

Место: 0104 ПР2 §4.2 после human boundary; заменить только выделенное место этой фигуры реальным снимком, подпись сохранить.

Статус: HUMAN_ACTION_PENDING: до реального действия снимок не доказывает результат; запрещено фотографировать незаполненный протокол как PASS.

Навигация: https://github.com/hitpipe/hitpipe/pull/10 → pinned TP source; H1 §4; review/peer-protocol.json.

Версия: adfbfc33bf20cb0b0641e49e48c0502d2f5d108d

Каталог: Отдельный чистый clone hitpipe/hitpipe, корень репозитория. Не переключать и не сбрасывать рабочую копию с незакоммиченными изменениями.

Действия: Дмитрий лично проверяет чужой Processing contract/consumer/substitution/map и выполняет его реальные normal/error/boundary commands из TP README на adfbfc33...; сохраняет actual protocol. Тимофей Патрушев отдельно проверяет IAM на текущем head PR #19, фиксируя actual full SHA по review/TIMOFEY_REVIEW_GUIDE_RU.md. После действий показать module protocol с reviewer/date/commit/commands/remarks/retest. Сейчас NOT PERFORMED; не создавать изображение с выдуманным результатом.

В кадре: реальный проверяющий; чужой module; полный SHA; contract/substitution/map; команды/результаты/замечания/повтор

Файл: docs/labs/pr12/dk/evidence/screenshots/0104-pz02-06.png

Редакция: Скрыть cookies, tokens, SSH/private keys, лишние email/персональные вкладки. Синтетические fixture и безопасные reason codes оставить. Не обрезать команду, SHA, имя теста/участника, итог и exit. Если не помещается — последовательные кадры с тем же номером и суффиксами a,b; лог не редактировать.
