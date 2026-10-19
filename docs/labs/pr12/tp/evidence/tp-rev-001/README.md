# TP-REV-001 — авторское исправление, 19.10.2026

MANUAL_LAB_PR12 / HIT-12 / HIT-27 / Тимофей Патрушев. Исполнитель — Codex по поручению участника. Статус: **REPRODUCED → FIX_IMPLEMENTED → LOCAL_RETEST_PASS → AWAITING_REVIEW**. Это не принятие замечания reviewer-ом и не human gate PASS.

Источник: [Request changes PR №5](https://github.com/hitpipe/hitpipe/pull/5#pullrequestreview-5564750022), [HIT-27](https://hitpipe.atlassian.net/browse/HIT-27?focusedCommentId=10454). PR №11 закрыт без merge; его исправление не читалось и не переносилось. Решение и тесты написаны в собственной ветке от актуального PR №5.

## Причина и граница

`encoding/json` сопоставляет имена полей без учёта регистра даже при `DisallowUnknownFields`. Старый token walk запрещал только точные дубликаты. Поэтому `amount_refunded: 1` и затем `AMOUNT_REFUNDED: 0` в полной fixture давали VALID. Это дефект **офлайн-парсера синтетических fixtures**, не доказанная уязвимость production-интеграции.

Предварительный обход теперь получает тип целевой fixture и сверяет каждое декодированное имя с точным JSON-тегом соответствующего struct до присваивания значений. Типизированный decode продолжает проверять числа и форму; проверка дубликатов и EOF сохраняется. Финансовые predicates, контракты продукта, source locks, CI и соседние сервисы не менялись.

## Проверенные версии

| Версия | Full SHA | Результат |
|---|---|---|
| Исходный C1 / PR №5 | `69b26f496c0e3bf4c837d49252203b70157101f1` | Дефект воспроизведён на чистом detached checkout |
| Собственное исправление C1 | `55fba39a6e50b05f27f37960e655f5bb9bae94b3` | Все пять обязательных проверок exit 0, CLI aliases exit 2 |
| Исходный полный стек / PR №10 | `adfbfc33bf20cb0b0641e49e48c0502d2f5d108d` | База экспериментальной интеграции |
| Полный стек + собственный code commit | `8e3be4b638958072b1e8d57ab0317df170f3d017` | Все пять проверок exit 0; mutation runner exit 0 |

Последняя строка — отдельная экспериментальная ветка `work/HIT-27-tp-rev-001-stack-check`, а не фактический merge PR №5–10. [Её CI](https://github.com/hitpipe/hitpipe/actions/runs/39220298854) имеет собственный статус; итоговый readback публикуется отдельно после завершения.

## Воспроизведение до исправления

Полные входы находятся в `baseline-verified/fixtures/`. `reproduction.json` содержит ожидаемый отказ, фактический вывод, exit codes, исходный SHA и SHA256 входов. Sidecar каждого запуска содержит argv, cwd, версии, UTC/МСК, SHA, dirty и digest журнала.

На чистом исходном SHA с Go 1.27.2 собрать `services/processing/cmd/processing-contractcheck`, затем выполнить из `services/processing`:

```sh
go build -o /tmp/tp-rev001-original ./cmd/processing-contractcheck
/tmp/tp-rev001-original --fixture testdata/valid.json --format json
# Для отрицательного входа взять полный baseline-verified/fixtures/shadow-refund.json
/tmp/tp-rev001-original --fixture /absolute/path/shadow-refund.json --format json
```

Ожидается exit 2 без результата eligibility. Фактически исходная CLI: exit 0, `VALID / FINANCIAL_ELIGIBILITY_ONLY`. Контроль с единственным каноническим refund=1: exit 0, `INELIGIBLE / REFUND`; с refund=0: VALID. После исправления тот же отрицательный вход и обратный порядок полей дают exit 2 / INVALID_JSON, stdout пуст; оба контрольных результата сохранены (`c1-cli-fixed/`).

`regression-red-verified/` — новые тесты на старом parser: exit 1. Исходный SHA + test-only patch и его digest записаны явно; dirty=true не выдаётся за чистый baseline. Patch: `regression-before-fix-verified.patch`. Входы проверены как полноценные JSON fixtures. Коррекция первоначального helper-а описана в `evidence-correction.md`; ранние невалидные артефакты не используются как доказательство регрессии и сохранены отдельно в локальном пакете.

## Проверки после исправления

`c1-fixed/` и `stack-fixed/` — отдельные новые запуски на чистых соответствующих SHA, Go **1.27.2 darwin/arm64**, Python 3.14.2, Git 2.52.0. Из корня:

```sh
python3 tools/pr12/workspace.py validate
python3 tools/pr12/test_workspace.py
python3 tools/pr12/workspace.py check processing --require
python3 tools/pr12/workspace.py smoke processing --require
```

Из `services/processing`: `go test -race -count=1 ./...`. **Каждая команда exit 0 на обеих версиях.** Wrapper check включает go vet и именованные tests/coverage; отдельный race журнал подтверждает реально выполненный race, несмотря на строку NOT_RUN внутри обычного wrapper check.

Новые tests проверяют uppercase/title aliases каждого поля полной fixture на верхнем и вложенных уровнях; конфликтующие facts/policy/envelope в обоих порядках; Unicode case-fold alias; канонические escaped names, known zero/false; точные и escaped дубликаты, неизвестные поля, trailing JSON, дробные/экспоненциальные/переполненные/строковые и NaN числа. CLI tests требуют именно exit 2 и отсутствие результата.

## Повтор mutation experiment

Новый запуск на `8e3be4b638958072b1e8d57ab0317df170f3d017`, после изменения parser/tests:

```sh
python3 services/processing/tools/mutations/run.py --output /absolute/path/new-output --natural-defects 2 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-27
```

`stack-mutation-run/mutation.json` — фактическая команда и exit 0. `stack-mutations/summary.json`: 6 seeded, 6 compiled, 6 KILLED, 0 SURVIVED, 0 INVALID; negative wrapper gate exit 1; восстановленные tests exit 0; clean=true. Отдельные patch/build/test журналы сохранены. Natural defects=2 — отдельно зарегистрированные старый CLI writer defect (HIT-29) и новый TP-REV-001 (HIT-27), не результат обнаружения mutation runner и не переписывание старой статистики. Общий workflow не менялся и сохраняет прежний параметр natural-defects=1; сравнивать следует фактические mutation outcomes. Hidden suite NOT_PROVIDED_NOT_RUN.

## Передача

Reviewer повторяет исходное воспроизведение, проверяет решение и tests и оставляет действительный verdict. Lead после approval/current CI интегрирует собственный fix PR в `work/HIT-27-paid-order-eligibility`, прежде чем принимать PR №5. Затем последовательно переносит обновлённый parent в ветки PR №6, №7, №8, №9, №10 (обычными merge parent→child без переписывания опубликованной истории), разрешает реальные конфликты и проверяет новый SHA/CI каждой ветки. Эксперимент не заменяет эти действия.

Протоколы интервью/clarity обновляются отдельным малым PR к ветке HIT-31. Интервью NOT CONDUCTED / BLOCKED_NO_USER; независимые студенческие проверки и opposition остаются ожидающими. STOP BEFORE 8: DOCX, снимки, теги сдачи, merge/закрытие задач, Feature и Epic не выполняются.

`sha256.json` индексирует сохранённые файлы evidence; CI и PR snapshots добавляются отдельными записями с фактическим временем. Исторические журналы не переписаны под новый SHA.
