# HIT-22 — source → binary → runtime, фактическое сравнение

Один сценарий: synthetic `services/iam/testdata/valid.json`, text/JSON result session_valid, source `2cd7c032e8ede98c75181599b440e97657bff197`, dirty=false. Полные build commands и external-directory smoke — `evidence/d1-build-smoke.*`; binary ELF/sections/version/digests — `evidence/binary-inspection.json`. Это actual observation, не план.

```text
Go source (domain/application/adapters/cmd) + service go.mod (stdlib only)
  → Go1.27.2 import/type checking + compile + link (Go runtime/stdlib included)
  → linux/amd64 ELF iam-contractcheck + separate synthetic JSON input
  → OS loader/runtime → main/run → strict adapter decode → composition root
  → Execute: Lookup → Clock → Evaluate → safe state/reason → exit
```

JSON fixture не компилируется в binary и остаётся отдельным input. Python wrapper/Git/gh нужны для подготовки/проверок, не для запуска готового ELF. Interpreter Node/Python/LLM и shared third-party runtime libraries этому binary не требуются на проверенном Linux amd64. Это не cross-platform portability claim.

| Наблюдение | Dev | Release |
| --- | --- | --- |
| Общий source, input, ожидаемый результат | один SHA / valid.json / session_valid | тот же SHA / тот же input / тот же result |
| Флаги | -gcflags=all=-N -l; linker version/commit/dirty/mode | -trimpath; linker version/commit/dirty/mode |
| Actual bytes | 4622983 | 4409836 |
| ELF | x86-64, statically linked, no dynamic section | x86-64, statically linked, no dynamic section |
| DWARF .debug_info | Есть, не stripped | Есть, не stripped; release не означает удаления symbols |
| Build metadata | Go1.27.2, mode=dev, commit=2cd7c03..., dirty=false | Go1.27.2, mode=release, тот же full SHA, dirty=false |
| Запуск вне исходников | binary+fixture скопированы в отдельный temporary directory; --version, positive, malformed и invalid format проверены | тот же протокол, PASS |

Это измерение размера и состава **одного** build в одной среде. Скорость, allocation, production capacity не измерялись; меньший файл не доказывает performance benefit. Debug/release могут одинаково содержать symbols; -trimpath убирает build source paths, не runtime data. `CGO_ENABLED=1` в toolchain metadata не делает этот конкретный stdlib-only binary dynamically linked — фактический ELF проверен.

Из root: `python3 tools/pr12/workspace.py build iam --mode dev --require` и с `--mode release`; `python3 tools/pr12/workspace.py smoke iam --require`; `file .pr12-build/iam/release/iam-contractcheck`; `go version -m ...`; `readelf -S ...`; `readelf -d ...`. Для запуска достаточно binary+fixture на совместимом host; `--clock system` без attestation даёт unavailable3. Отдельный student clean reproduction остаётся human gate, собственный outside-checkout запуск за него не выдаётся.
