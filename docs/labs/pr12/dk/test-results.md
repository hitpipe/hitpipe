# HIT-21 — actual results, 18 октября 2026 UTC

Проверяемый код: `5b14ab92d1ca2a68f46fd3bda783b188549fbf5a`; clean Git input, Go1.27.2/linux/amd64, Python3.14.4, GOMAXPROCS=2, GOFLAGS=-p=1. Полные argv/cwd/env/UTC/exit/SHA256 и input digests — соседние `evidence/*.json`, исходные журналы — `.log`. PASS относится только к указанному запуску, не к чужому review или готовности IAM целиком.

## Локальные результаты

- `c3-final-check`: wrapper vet + реальные named tests/coverage + `go test -race -count=1 ./...` — exit0, PASS.
- `c3-cross-coverage`: `go test -count=1 -coverpkg=./... -coverprofile=../../.pr12-build/iam-all.cover ./...` — exit0, PASS.
- `c3-final-smoke`: dev/release, source metadata, positive fixture, malformed fixture и invalid format вне checkout — exit0, PASS.
- `c3-fuzz`: bounded fuzz policy invariants на предшествующем test SHA `61a54285e979fd373eb101116ecb6fa1531913f5` — PASS, 215557 executions за 3s fuzz phase, 2 workers. Production policy не менялась; это ограниченный случайный поиск, не exhaustive proof. Точные числа относятся только к этому запуску.

CI exact head проверяется отдельно после push; сохраняемый URL будет в `evidence/ci-runs.json` финального пакета. Зеленый bootstrap соседних отсутствующих компонентов не считается их тестированием.

## Mutation protocol и отрицательный gate

Исходный исправный baseline `462d553e8373af6047f0950e0a9adfe15e85c83f`: PASS. Все семь дефектов отдельно компилируются, build exit0. Первый набор: M01–06 KILLED, M07 SURVIVED. M07 убирает проверку Reason из Decision.IsValid: policy сама выдаёт согласованные значения, поэтому happy-path assertions не обнаруживали возможность вручную собрать противоречивый Decision. Добавлен `TestDecisionRejectsMixedStateAndReason` в `61a54285e979fd373eb101116ecb6fa1531913f5`; тот же M07 KILLED, failing test указан в retest protocol.

Итого **7 засеяно; 6 обнаружено первым набором; 1 пропущен; после усиления обнаружены все 7** (шесть прежних обнаружений + повтор M07). Не считается восемь разных дефектов. Compile-invalid=0. Baseline и восстановленный final-clean exit0, source digest восстановлен для каждого; временные ветки удалены. В репозитории исправный production code и отдельные `.patch` как данные эксперимента.

Для проверки CI sensitivity M01 выполнял ту же команду, что ordinary CI: `python3 tools/pr12/workspace.py check iam --require`; build0, tests1 с TestEvaluateIdleBoundary/equal и другими assertions. После восстановления та же команда0. Это фактический локальный отрицательный gate, не вымышленный красный GitHub run. CI дополнительно воспроизводит серию в собственной временной ветке.

Естественных дефектов **1**, задача HIT-19: decoder принимал `Version` вместо exact `version`. `natural-json-case-before` → настоящий failing assertion «expected config exit2, got0», exit1; после exact object key validation → exit0. Input tree digests и тест зафиксированы. M07 — намеренно засеянный дефект/пробел assertions, не второй естественный production defect. Скрытые преподавательские дефекты не предоставлены, их обнаружение NOT RUN.

## Coverage analysis

В обычном Go coverage каждый test binary инструментирует собственный package, поэтому CLI tests не учитывают вызванный ими parser package. Добавлен **отдельный cross-package профиль**, включающий все пять IAM packages; объединённое statement coverage **94.5%**, domain Evaluate/IsValid и application Execute **100% statements**. Это не branch coverage и не доказательство всех комбинаций.

Конкретные оставшиеся участки: `main()` (os.Exit entrypoint) не вызывается внутри unit-test процесса; он реально выполняется external binary smoke, но не включён в этот coverage profile. В generic JSON token reader не все malformed closing-token paths достигнуты; typed decoder имеет дополнительные invalid chronology/type returns. Небольшая ветвь reflective nil classification для экзотических interface wrappers не покрыта всеми Go kinds. Решение: существующий строгий negative suite, size/depth limits, live binary smoke и constructor typed-nil tests обязательны; exhaustive grammar fuzzing и OS write/read fault matrix отложены, не считаются PASS. Для read-error/output-error добавлены конкретные regression tests в C3.

Даже 100% core statements не проверяют свежесть настоящей SQL authority, time synchronization, tenant authorization, revocation barrier, durability, distributed races или OIDC. Они требуют будущих adapter/integration задач. Влияние этих ограничений на выбор проверок записано в `risk-registry.md`; independent human review oracle и clean-room reproduction остаются HIT-23.
