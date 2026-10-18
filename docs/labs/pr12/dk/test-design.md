# HIT-21 — план проверок и oracle

Автор-owner: Дмитрий Карачебан; агент реализации Codex. MANUAL_LAB_PR12; upstream Candidate, не acceptance. Oracle сформулирован до реализации в R1 `fixture-spec.md`, затем уточнён в HIT-18 `services/iam/docs/module-contract.md`. План C3 добавляет проверку чувствительности, не получает ожидания из actual output.

| Техника / класс | Вход и независимое ожидание | Реальные тесты |
| --- | --- | --- |
| Эквивалентные классы | Known/not revoked/correct chronology/trusted before deadlines → session_valid; unknown/revoked отдельно → denied | TestEvaluateValid/Unknown/Revoked |
| Граничные значения | Idle и absolute: −1ns допустимо, equality/+1ns истёк; 24h cap/+1ns; zero/future/unsupported year | TestEvaluateIdleBoundary/AbsoluteBoundary/InvalidChronology/ClockAndFutureCreation |
| Таблица решений | Clock > unknown > revoked > malformed > future > absolute > idle > valid, включая одновременно истинные причины | TestDecisionTablePrecedence |
| Contract substitution | Один Execute с File и Fixed: normal/error/idle equality/unknown/revoked → один oracle | TestSameConsumerContractWithBothAdapters |
| Fail closed и свежесть | Lookup error → unavailable без Clock; Clock error/untrusted → unavailable; now считывается после Lookup; cancellation до/во время ports → unavailable | TestPortErrorsDoNotAllowOrLeak, TestClockReadAfterLookupAndCancellation |
| Недоверенный ввод | Duplicate/extra/case mismatch/null/type/version/trailing/deep/oversize/invalid timestamp → config error 2; canary не печатается | TestDiagnosticMalformedAndCanary, TestAdapterFailureAndBoundaryInputs |
| Инварианты | Valid подразумевает found, not revoked, trusted, valid time interval; одинаковый input даёт одинаковое decision | FuzzValidityInvariants, TestEvaluateDeterministicImmutable |

Данные synthetic/fixed UTC, не production cookies/IDs/secrets. Time-dependent policy тестируется с injected time, без sleep. SystemClock тестирует только wiring explicit probe, не доказывает NTP/host health. Concurrency exercise — 32 чтения неизменяемой копии; race detector ограничен выполненными путями.

## Эксперимент чувствительности

`python3 services/iam/tools/mutation_check.py` требует committed clean source. Для M01–M07 создаётся временная Git-ветка/worktree, выполняется исправный baseline; каждый дефект вносится отдельно, `go build ./...` должен дать 0, затем **та же команда CI** `python3 tools/pr12/workspace.py check iam --require` должна упасть с именованным failing test. Compile failure не считается обнаружением. Patch, input digests, argv/cwd/time/exit/log hash сохраняются; исходник восстанавливается в finally, конечный clean gate обязан пройти. Main/PR не получают mutant production code.

M01: idle equality; M02: absolute equality; M03: пропуск revoked; M04: unknown default allow; M05: удвоение lifetime cap; M06: lookup error default allow; M07: неполный положительный Decision.IsValid. При SURVIVED фиксируется первый результат, добавляется осмысленный тест и отдельно повторяется тот же mutant с новым test SHA. До запуска результаты не объявляются PASS.

## Reproduction

Из root на exact PR head с Go1.27.2: `python3 tools/pr12/workspace.py validate`; `PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require`; `python3 tools/pr12/workspace.py smoke iam --require`; `python3 services/iam/tools/mutation_check.py`. Coverage: `cd services/iam && go tool cover -func=../../.pr12-build/iam.cover`. Bounded fuzz: `go test ./internal/domain/sessionpolicy -run '^$' -fuzz FuzzValidityInvariants -fuzztime=3s -parallel=2`.

CI checks exact pull request head, выполняет собственные named tests и race, затем smoke и mutation gate; upload-artifact сохраняет coverage/protocol/patches/logs. Настоящий CI URL и head SHA включаются в evidence после выполнения. Независимая проверка человеком, полноценный IAM E2E/SQL/OIDC и эксплуатационная готовность этим набором не подтверждаются.
