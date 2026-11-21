# HIT-128. Шесть зависимостей из обязательного CONTEXT

Проверены рабочий исходник `c426dc4` и рассмотренный head PR №85 `c394de8`. `git merge-base --is-ancestor` для каждого проверенного SHA и каждого SHA зарегистрированного результата вернул 0 на обеих целях. Шесть исходных JSON совпали с результатами в официальных пакетах Jira, их Git blob совпал на обеих целях. База PR `1983102` находится вне main.

| Задача | Проверенный SHA → SHA результата | Независимое ревью | Реализация и проверка на c426dc4 |
| --- | --- | --- | --- |
| HIT-111 | `b2e823012854575146595b01a6f56d5ac677fc44` → `49229e1` | №14258, PASS WITH NOTES, 8/8 | `cmd/processing/main.go`, `runtime/config.go`, `runtime/iam.go`. Три одинаковых Git blob. `TestProcessingExecutableOwnedStand` PASS |
| HIT-112 | `0f710a6682d15b437f847376b348947cb4757c7c` → `3b542e8` | №14400, PASS WITH NOTES, 8/8 | `postgres/caseworker.go` и `runtime/caseworker.go`. Фильтр таймеров из HIT-114, диагностика из HIT-127. `TestProcessingCaseWorkerOwnedStand` PASS |
| HIT-113 | `9165e814cb7b46e9cfae484c62ccc70f63fc1ee3` → `dd1a36f` | №14534, PASS, 8/8 | `postgres/recheck.go` побайтно совпал. В `runtime/recheck.go` добавлена диагностика. `TestProcessingRecheckOwnedStand` PASS |
| HIT-114 | `dd05559bc911437d33c0c2582b604d2f2728d362` → `33d2c8d` | №14726, PASS, 8/8 | `postgres/timeouts.go` побайтно совпал. В `runtime/timeouts.go` добавлена диагностика. `TestProcessingCaseTimeoutsOwnedStand` PASS |
| HIT-126 | `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` → `d450efc` | №15121, PASS, 6/6 | Отдельный `cmd/processing-purge`. `postgres/purge.go` и `contentfiles/maintenance.go` побайтно совпали. CLI получил структурированный stderr. `TestProcessingPurgeOwnedStand` PASS |
| HIT-127 | `4da48202e77131427730b1827df9ee81940e559a` → `1983102` | №15637, PASS WITH NOTES, 6/6 | `runtime/diagnostics.go`, `postgres/diagnostics.go` и `runtime/runtime.go`. Три одинаковых Git blob. `TestProcessingDiagnosticsOwnedStand` PASS |

Git-цепочка от HIT-111 и HIT-112 раздваивается на HIT-114 и HIT-113. Обе ветки входят через merge `e381972` с родителями `33d2c8d` и `dd1a36f`. Повторный `git merge-tree` вернул 1 и три ожидаемых конфликта. Реальный `git show --cc` сохраняет фильтр очереди по `kind` и состоянию сообщений, причину RECHECK и причины таймеров, а также оба вызова `advanceRecheck` и `advanceTimeout`. Затем идут HIT-126, HIT-127, HIT-128 и коммиты отчётов PR №85. Исходные SHA не переписаны. `bdf3187` изменил четыре обвязки для диагностики, как показывает точный diff. `git diff --exit-code` между `c426dc4` и `c394de8` вне доказательств и результата вернул 0.

`go list -deps` на обеих закреплённых копиях в конфигурации Linux arm64 подтвердил включение файлов в исходные пакеты `processing` и `processing-purge`. Команды реальной сборки, SHA-256 бинарников и все шесть успешных процессных регрессий взяты из исходной квалификации `c426dc4`.

В зарегистрированном JSON-результате `dependency_proof_after_15797.dependencies` содержит полные исходные результаты, официальные вердикты и Git blob. `commands` содержит все 71 выполненную команду, коды выхода и полные stdout/stderr, включая граф, конфликтный merge и diff. `original_process_qualification` связывает результаты с проверенной сборкой. Эти данные входят в сам результат Project OS.
