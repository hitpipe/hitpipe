# Инструментальный контур Processing

HIT-30. Общий выбор LAB-STACK-001 ../shared/stack-decision.md и HP-ADR-001 upstream переиспользован, не приписан студенту. Мой участок — service-local Go1.27.2, pure policy, явная reader DI, offline diagnostic. Модель последовательная в одном вызове; IO в adapter, hidden retries отсутствуют.

| Позиция | Средство и маршрут подключения | Фактический статус |
|---|---|---|
| Сборка | Go1.27.2; wrapper build processing --mode dev/release; Makefile | Работает: evidence/c1 и d1 build logs, реальные бинарники |
| Зависимости | service-local go.mod exact Go1.27.2; stdlib only; go version -m | Работает, внешних modules0, go.sum не требуется; legacy root не менялся |
| Тесты | Go testing; workspace.py check processing --require; fixed JSON/t.TempDir | Работает: c3/check.log, именованные assertions и coverage |
| Статический анализ | go vet ./... + shared import guard | Работает: wrapper check, domain/application concrete boundary guard |
| Динамическая диагностика | go test -race -count=1 ./...; safe CLI start/result/version; Delve для будущего interactive debugging | Race/diagnostics реально PASS; Delve не установлен/не запускался, PLANNED перед profiling research |
| Профилирование | go test -bench/-cpuprofile + go tool pprof, будущий benchmark существующего evaluator | PLANNED следующая инженерная работа, сейчас performance не измерялся, bottleneck не заявлен |
| CI | GitHub Actions Ubuntu24.04, setup-go1.27.2, owner pr12-tp.yml; pull_request и work/** push | C1/C2 actual PASS; C3 run readback/index содержит действительный итог. Upload named test log, coverage, mutation protocols; review/merge отдельно |

Локальная установка Go arm64 в task work/go после сверки официальной JSON release checksum: 76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5. Личные access tokens/service credentials не читались и не помещались в код. Существующая авторизация gh использовалась для repo/PR/CI, не как личность независимого проверяющего. Cwd, tool versions, даты, exits и digests каждого прогона в evidence/*/record.json.
