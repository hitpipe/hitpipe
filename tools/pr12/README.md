# Воспроизводимый инструментальный контур ПР1–2

Это bootstrap tooling. Студенческий модуль и его результаты ещё не реализованы.
`NOT_IMPLEMENTED`/`NOT_RUN` — честный исход отсутствующего module, не PASS лабораторной.
Legacy root Go/Vite dependencies остаются неизменными. Полный Next app/deployment не создаётся.

## Чистый клон

1. Клонировать репозиторий и checkout точный bootstrap commit, указанный в Feature export.
   До merge bootstrap PR разрешённая рабочая база — его точный head SHA, а не mutable branch.
2. Python 3.10+ для wrapper. Backend: Go **1.27.2** (C1 service go.mod ровно этой версии).
   Frontend: Node **24.22.0**, npm **11.19.1**, exact cohort в `tooling/pr12/frontend`.
3. `python3 tools/pr12/workspace.py validate` — проверяет loaders и hashes frozen stack copies.
4. `python3 tools/pr12/workspace.py status` — показывает, какие реальные модули существуют.
5. Frontend **после появления модуля**: `cd tooling/pr12/frontend && npm ci --strict-peer-deps`.
   Не запускать legacy `npm install` ради нового cohort и не обновлять lock по случайному latest.

Точный стек уже выбран upstream; копии `docs/labs/pr12/upstream/stack` неизменяемые.
Неиспользуемые runtime библиотеки не надо искусственно добавлять к pure module. Новая внешняя
зависимость требует pinned version, совместимости/лицензии и rationale в своей карточке.

## Контракт C1 — диагностический executable

Backend: `services/<service>/cmd/<service>-contractcheck/main.go`; entrypoint содержит
строковые `var version, commit, dirty, buildMode`, которые build wrapper задаёт через Go linker.
Обычный service server не заменяется этой диагностической командой. Domain policy используется
реальным application consumer; никакого fake production webhook/auth endpoint.

Frontend: `tools/pr12/case-contractcheck.ts`; C1 CLI использует тот же pure TS domain module,
что будущий presenter. Для immutable build metadata импортировать
`buildInfo` из `./component/build-info.js`: wrapper генерирует TS module с фактическим SHA/dirty
до компиляции, включает его в distribution. Это generated файл в .gitignore, не hand-authored
business model. Не читать исходный Git при запуске готового JS снаружи checkout.

Обе команды поддерживают `--fixture PATH`, `--format text|json`, `--version`. Positive fixture
даёт exit0 и объявленный безопасный marker; malformed negative fixture и invalid format — nonzero.
Никаких secrets/PII/body dumps. Путь fixture выбирается конфигурацией, не hardcoded local file.
Фикстуры исключительно синтетические и входят в repo. В README своего модуля описать reset/re-run.

C1 создаёт `lab-component.json` ВНУТРИ своего component root (Go: `services/<service>/`,
TS: `src/features/case-status/`), используя example из tools/pr12. `positive_output_contains`
выводится из требования; `expected_test_names` содержит минимум три реально названных tests
normal/error/boundary, не фиктивные имена example. `negative_fixture` — malformed input/config,
а не просто допустимое business-denied решение, если оно законно возвращает exit0.

## Проверки и два режима

```bash
python3 tools/pr12/workspace.py check iam --require
python3 tools/pr12/workspace.py build iam --mode dev --require
python3 tools/pr12/workspace.py build iam --mode release --require
python3 tools/pr12/workspace.py smoke iam --require
python3 tools/pr12/workspace.py inspect iam --mode release --require
PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require
```

Заменить `iam` на `intake`, `processing`, `delivery`; в frontend — `case-status`.
Backend check выполняет import-boundary guard, `go vet`, actual named `go test` и coverage;
`-race` опционален по среде, NOT_RUN без включения. Mode dev использует `-N -l`, release
`-trimpath`; wrapper не заявляет ускорение без измерений. Binary inspection сообщает actual info,
не предполагает static/dynamic linkage заранее. TS check использует pinned tsc + native Node tests
с assertions, nonzero named tests и coverage. Native Node coverage не заменяет ручной разбор
непроверенных свойств. React/browser/accessibility проверки C2/C3 требуют собственных meaningful
tests и pinned Playwright; никакой browser screenshot агентом не делается.

`smoke` собирает два режима и запускает диагностический результат из временного каталога
вне checkout с скопированными synthetic fixtures. Go binary копируется отдельно. TS core diagnostic
должен быть dependency-free runtime: compiled modules + declared Node runtime. Если ему нужны
внешние runtime packages, нельзя выдать текущую limited packaging за PASS: добавить явный
packaging Task/полный declared dependency set. Этот TS smoke не доказывает custom Next server,
Next SSR/route/BFF, external permissions, whole service или production readiness.

TS compiler output — `.pr12-build/frontend`; это ECMAScript modules. Для NodeNext использовать
`.js` import specifiers в TS source; не смешивать bare server imports с browser bundles. Изолированный
cohort включает React/Next только для type/build context; wrapper не запускает несуществующее app.
TS dev/release на одном diagnostic core используют declared Node+tsc route: сравнение конкретных
оптимизаций/доставляемого состава студент обязан подтвердить фактическими командами/наблюдениями.

## CI и доказательства

Workflow каждого владельца существует заранее, но до C1 показывает `NOT_IMPLEMENTED` и
`student_checks: NOT_RUN`. Если component files появились без manifest, проверка падает;
если manifest есть, missing tests/entrypoint/fixtures, zero tests и failing assertions падают.
Green bootstrap check не означает student PASS, review, merge, Accepted или готовность отчёта.
C3 дополняет свой workflow необходимыми meaningful tests/coverage/артефактами. Root CI редактирует
только Lead owner; у участников отдельные `pr12-dk/tp/mg/ds/nb.yml`.

Generated `.pr12-build` не является сданным evidence: после actual run перенести безопасные logs/
coverage в `docs/labs/pr12/<code>/evidence`, записать full SHA, команды, версии, timestamps,
exit/status, scope и CI URL. Скриншоты — **только текстовые места и полные инструкции человеку**.
Для МДК01.04 ПР1 отдельно воспроизвести той же CI-командой нарушенное условие и сохранить
реальный nonzero лог; не портить main/полезный финальный код. Мутации C3 выполняются по одной
во временной ветке/worktree, после опыта final clean run обязателен.


## React-компонент без тупика legacy/Next

Pure Node runner не ограничивает работу только pure TS: C2/C3 добавляют `.test.tsx`
рядом с CaseStatusCard; isolated tsc компилирует TSX через react-jsx, а готовый Node
test импортирует `react-dom/server` и проверяет `renderToStaticMarkup` (semantic
headings, role/status, labels, escaped text, unknown/stale/time markers). React19
находится из `.pr12-build/frontend/node_modules` — ссылка на isolated cohort, не legacy React18.
Это настоящее исполняемое React rendering evidence, но SSR string не доказывает browser
keyboard/focus/hydration. Для таких свойств C2/C3 создают явный local fixture host с
pinned React/Next/Playwright, actual assertions и declared capabilities; не ждут полного
production приложения и не выдают fixture за live. Агент не снимает browser screenshots.

TS dev mode оставляет source maps/comments и Node `--enable-source-maps`; release
убирает их, metadata содержит actual mode. Это подтверждаемое отличие поставки/диагностики,
не заявление об оптимизации скорости. Wrapper очищает output до tsc: удалённый исходник
или test не может продолжать исполняться из stale JS. Каждый command имеет bounded timeout.

Required CI contexts backend: `pr12-iam`, `pr12-intake`, `pr12-processing`, `pr12-delivery`.
Frontend: `pr12-case-status`. Все PR запускают все owner workflows, чтобы required checks
не зависали из-за path filter. Human branch protection/review остаётся отдельной настройкой.
Для private frontend, если GitHub plan возвращает403, статус `BLOCKED_HUMAN_GITHUB_PLAN`:
использовать обязательный review/merge Lead и прямой запрет main push по регламенту до
доступного защищающего тарифа; не писать «ветка защищена» без реального readback.


## Граница автоматической проверки imports и имен тестов

Go import guard читает только canonical double-quoted import paths, допускает обычные
именованные/blank aliases и группы, игнорирует комментарии/обычные строки. Dot imports,
raw literals и escaped import paths отклоняются явно, чтобы допустимый язык Go не давал
обойти ограниченную проверку слоев. Это узкий guard, не полный Go AST validator и не sandbox:
полную грамматику проверяют go vet/build, содержательные зависимости — review.
Node TAP проверка связывает объявленное имя с точным именем реально PASS-теста; prefix,
SKIP и TODO не считаются подтверждением. Эти проверки подтверждают только собственный
bounded scope; не source acceptance, security proof всего продукта или student lab PASS.
