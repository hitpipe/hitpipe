# HIT-22 — инструментальный контур IAM

Автор-owner Дмитрий Карачебан; агентное исполнение Codex, MANUAL_LAB_PR12. Основа — [shared/stack-decision.md](../shared/stack-decision.md), [HP-ADR-001](../upstream/architecture/HP-ADR-001.md), [implementation convention](../IMPLEMENTATION_CONVENTION.md). Shared decision остаётся общим источником; личный [DK-ADR-001](module-adr.md) объясняет ограниченную реализацию. Это не принятие Candidate фазовых контрактов.

Go **1.27.2** закреплён в service go.mod и wrapper; per-service module не меняет legacy root. Прямые внешние Go зависимости: **0**; стандартная библиотека версии toolchain. `go.sum` отсутствует обоснованно, implicit latest dependencies нет. Python3.14.4 — фактический локальный wrapper, контракт wrapper Python3.10+; Git2.53.0, GitHub CLI2.46.0, binutils readelf2.46; эти host tools не входят в runtime binary. Go архив получен с официального download index, проверен SHA256 `ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5` для linux-amd64 Go1.27.2. Проверенные команды и точная среда — evidence.

| Все 7 позиций | Средство и назначение | Маршрут на участке | Фактический статус |
| --- | --- | --- | --- |
| 1 Build/transformation | Go1.27.2 compiler/linker; Python wrapper | root → workspace.py build iam --mode dev/release → module cmd → ELF | WORKS: C1/C2/C3/D1 actual smoke |
| 2 Dependency management | Go modules1.27.2 | собственный services/iam/go.mod; go list -json ./...; no third-party modules | WORKS: d1-imports; никаких private credentials |
| 3 Tests | go test1.27.2, testing/fuzz, coverage | named tests, fixtures, DI contracts, mutation runner | WORKS: c3-final-check, fuzz, cross coverage, mutation protocols |
| 4 Static analysis | go vet1.27.2 + existing source import guard | wrapper check: domain/application boundaries, vet ./... | WORKS: local/CI; не SAST proof/security certification |
| 5 Dynamic analysis | Go race detector1.27.2 + external diagnostic smoke | PR12_RACE=1 wrapper; реальный binary dev/release вне checkout | WORKS на linux/amd64 с C compiler; ограничен выполненными путями |
| 6 Profiling | Go1.27.2 pprof/benchmark/trace | будущий measured production-authority workload, отдельная задача после реального adapter | PLANNED / NOT RUN. Профиль CPU/alloc не снимался; bottleneck и latency цифры не заявляются |
| 7 CI | GitHub Actions ubuntu24.04, checkout@v4, setup-go@v5, upload-artifact@v4 | .github/workflows/pr12-dk.yml: exact head→sources→vet/tests/race→smoke→coverage→mutations→artifacts | WORKS подтверждается конкретными CI runs в финальном index; hosted action major refs унаследованы, это не immutable toolchain digests |

В production module нет dependency on Python/gh/Atlassian/LLM/browser. Для clean reproduction нужны Git, Python3.10+, exact Go, Linux binutils для inspection; `PR12_RACE=1` дополнительно требует поддерживаемый host+C compiler. Установка tools и отдельное human воспроизведение — разные факты. Последнее остаётся HIT-23, не заменяется собственным запуском.

Порядок первого запуска и команды — [services/iam/README.md](../../../../services/iam/README.md). Ненулевой gate подтверждён M01 при build0/tests1, затем restored0; false green от отсутствующего/skip теста запрещён wrapper. Доступны все исходники и синтетические fixture без внешнего аккаунта.
