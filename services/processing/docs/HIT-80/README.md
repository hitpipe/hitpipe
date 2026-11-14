# HIT-80 — измерение и сокращение затрат Decode

Выполнено Codex по делегации Тимофея Патрушева / Tim1hik. Независимое ревью и личная приёмка не заменяются этим отчётом.

## Вывод и изменение

Профиль выделений корректного исходного Decode на синтетическом valid SourceFacts показал: regexp.MustCompile/regexp.compile занимают 64,54% alloc_space (2,39 GB из 3,70 GB за профилируемый прогон; это совокупные выделения, не память одной операции). См. [исходный профиль](baseline/alloc.pprof) и [расшифровку](baseline/alloc-cumulative.stdout).

Изменение одно: два неизменных pattern из точной принятой OAS компилируются генератором в regexp.MustCompile при инициализации пакета. fieldRule хранит *regexp.Regexp и вызывает MatchString без повторной компиляции. Текст regex, nullable, форматы, проверки чисел, scope, JSON и финансовая семантика сохранены. Общий объект regex не модифицируется; параллельная регрессия проверяет чтение и независимость возвращаемых указателей под race.

Пересериализация map→JSON→Facts остаётся прежней. Её изменение не испытывалось и не объявляется отвергнутым экспериментом. Первый подтверждённый профилем кандидат дал выигрыш; неудачных вариантов оптимизации нет.

## Версии и условия

- Принятый HIT-72: PR44 действительно MERGED, merge 20adc01a037148d20ea8da6c281f52d25cb2d9e3 находится в main `ce771d37505e7770003d0796ae932556d06b7bab`.
- Исходное измерение `30a28b9a904da27c1c1ebd52b0df09846fe38efb`: добавлены только общий benchmark/corpus и runner; production Decode/schema/tests побайтно совпадают с main (проверка в supplemental/commands.json).
- Изменённая проверенная версия `f54907c0cb5b95f1f7e389c4bbf5d806cf8b0370`; после неё добавлены только сравнение и доказательства. Открытые PR HIT-76–79 не объявляются частью main.
- Go1.27.2 darwin/arm64, Apple M4, macOS27.0.1, GOMAXPROCS=1, CGO_ENABLED=1, один поток benchmark (-cpu=1), по10 измерений каждого из13 сценариев, benchtime100ms. См. [условия old](baseline/environment.json), [new](optimized/environment.json).
- Benchmark вызывает полный настоящий Decode. Создание fixtures, чтение файлов и проверка ожидаемого отказа исключены из таймера. Профили получены отдельно на valid с benchtime3s; их ns/op не смешаны с обычными samples.

## Результаты всех сценариев

ns/op — медиана и полный диапазон [min–max] десяти samples; память и allocations — медианы. В [comparison.json](comparison.json) сохранены все samples, средние, стандартные отклонения и разброс каждой метрики; ничего не отброшено. B/op и allocs/op постоянны внутри каждой группы из10 samples.

| Сценарий | old ns/op [min–max] | new ns/op [min–max] | B/op old → new | allocs/op old → new |
|---|---:|---:|---:|---:|
| alias_order_0 | 10447 [10319–10691] | 9263.5 [9152–9351] | 8456 → 5776 | 251 → 215 |
| alias_order_1 | 9833.5 [9734–9914] | 8666.5 [8577–12300] | 8208 → 5528 | 232 → 196 |
| boundary_max | 30604 [30485–30971] | 22334.5 [22015–22566] | 33079 → 11618 | 541 → 315 |
| boundary_over | 2991.5 [2977–3025] | 2955.5 [2944–2988] | 2920 → 2920 | 80 → 80 |
| duplicate | 10636 [10602–10716] | 9204 [9076–9238] | 8456 → 5776 | 251 → 215 |
| fraction | 3223.5 [3217–3311] | 3183 [3168–3202] | 3056 → 3056 | 87 → 87 |
| integer_decimal | 30692 [30451–31310] | 22295.5 [22078–22407] | 33127 → 11666 | 547 → 321 |
| integer_exponent | 30682 [30428–30785] | 22209.5 [22111–22285] | 33047 → 11586 | 547 → 321 |
| known_false | 30526 [30360–30847] | 21989 [21775–22162] | 33015 → 11554 | 541 → 315 |
| known_zero | 30651 [30297–30838] | 22138.5 [22022–22226] | 33015 → 11554 | 541 → 315 |
| nullable | 28829.5 [28618–29017] | 21761.5 [21685–21837] | 30302 → 11522 | 503 → 313 |
| unknown | 20813 [20714–20949] | 12322 [12193–12378] | 27915 → 6456 | 470 → 244 |
| valid | 30739 [30633–30840] | 22109 [21865–22294] | 32983 → 11506 | 541 → 315 |

На valid медиана времени ниже на28,08%, B/op ниже на65,12%, allocs/op ниже на41,77% в этих локальных прогонах. Это описательное сравнение, не статистический доверительный интервал и не целевой процент. В new allocation profile повторная компиляция не является горячим путём; [new профиль](optimized/alloc.pprof), [расшифровка](optimized/alloc-cumulative.stdout).

CPU-профили обоих прогонов сохранены. Исходный CPU profile заметно занят runtime.kevent/GC (76,75% flat kevent); его доли не интерпретируются как чистая стоимость Decode или доказательство процента ускорения CPU. Решение об изменении основано на allocation profile; временной результат взят только из непрофилируемых benchmarks.

## Корректность и проверки

[309 полных old записей](baseline/corpus.json) и [new записей](optimized/corpus.json) совпадают побайтно, SHA256 `61e9a1602ddbe3565346af916eb4566631bcf9e52bf3884a03b77f13dc1ea4b7`. Сохранены входные digests, полный Projection (включая исходные строки provenance), диагностический отказ и финансовые результаты до/ровно на истечении срока. При каждом отказе проверяется точное равенство Projection{}. Набор включает каждое обязательное поле: отсутствие/null/неверный тип, точный дубликат в обоих порядках, uppercase alias; refund shadowing в обоих порядках; unknown, 0/false/null,1.0/1e0, точные границы/дроби/переполнение, неверный scope/policy, даты и форматы. Существующий corpus HIT-72 не переписан.

На обеих версиях выполнены все9 команд runner, exit0: corpus,10x benchmarks, CPU/alloc profile, четыре pprof расшифровки, go test -race -count=1 -v ./..., go vet ./.... Полные журналы [old](baseline/commands.json), [new](optimized/commands.json), [new race stdout](optimized/race.stdout), [new vet stdout](optimized/vet.stdout). Дополнительно7 команд exit0 проверили Go, форматирование, генерацию точной OAS и отсутствие diff, чистый production baseline, неизменность domain/application/dependencies/common CI и сравнение результатов: [протокол](supplemental/commands.json).

## Воспроизведение

Из services/processing, с закреплённым Go1.27.2 на PATH и чистыми коммитами:

```sh
# В отдельных worktree для old/new SHA выше, одинаковый компьютер и окружение.
python3 tools/sourcefacts/measure.py --out /absolute/new-empty/old --label baseline
python3 tools/sourcefacts/measure.py --out /absolute/new-empty/new --label optimized
# compare.py из текущего PR; берёт полный набор, ничего не фильтрует.
python3 tools/sourcefacts/compare.py /absolute/new-empty/old /absolute/new-empty/new comparison.json
# Детерминированная генерация из exact OAS с обязательным SHA256 внутри generator:
python3 tools/sourcefacts/generate.py /path/to/exact/mvp-openapi.json
```

Команды исполняются каждая в своей версии, не обе в одном checkout. Параметры/точный cwd/env/время/exit/source SHA сохранены в опубликованных протоколах. Исходный CPU/allocation profile можно открыть `go tool pprof -top cpu.pprof` или `go tool pprof -top -alloc_space alloc.pprof`. При необходимости test binary пересобирается на соответствующем SHA через `go test -c -o sourcefacts.test ./internal/adapters/intake/sourcefacts`. Локальные бинарники не публикуются; их digests указаны в [source-audit.json](source-audit.json). Все опубликованные файлы проверяются по [evidence-digests.json](evidence-digests.json), не включающему себя.

## Матрица8 критериев

| Frozen DoD ID | Фактическое доказательство |
|---|---|
| 3a2e630c-d0e3-4cec-9491-d9afa75d83f0 | source-audit.json; supplemental/commands.json step4 |
| 1777c541-bcfd-4774-83cf-1c64614bfc64 | measurement_test.go; baseline/corpus.stdout и optimized/corpus.stdout |
| 1b8d2c1d-6893-4bdc-8582-f17880606374 | baseline/bench.stdout, cpu.pprof, alloc.pprof, alloc-cumulative.stdout |
| 384f31e0-56a8-4c07-acd4-48bbe4db5eef | comparison.json; обе environment.json, commands.json и bench.stdout |
| db84b12d-56ab-4a4b-9786-ab1c4d4e4826 | comparison.json; optimized/alloc-cumulative.stdout |
| 422c0b5d-ec67-4626-a056-5eef1605bdf3 | baseline/corpus.json = optimized/corpus.json; оба race.stdout |
| 04ef62c9-7b3f-44a2-b16e-48ac532e210e | optimized/commands.json; race.stdout; vet.stdout; supplemental/commands.json |
| 890385b8-b41f-4f36-a7a9-21d0213524ea | README.md: причина, результат, ограничения и воспроизведение |

## Ограничения

Это локальная оптимизация строгого parser/projection, а не скорость end-to-end, throughput сервера или коммерческая/production готовность HitPipe. Измерения выполнены последовательно old затем new на одном компьютере; фоновые нагрузки, CPU affinity, governor и тепловой режим не контролировались. Холодный старт и дополнительная работа инициализации двух regex не измерялись. Обе реализации выделяют независимые результаты; race safety не обменивалась на метрику.

Реальных IAM/Intake/Stripe, внешних действий, SQL/outbox или серверного запуска здесь нет; действующая DB qualification относится к другим задачам. В этой ветке actual main не содержит PostgreSQL Processing adapter; согласованная18.7 учитывается, но не выдаётся за выполненную здесь DB проверку. Финансовый VALID остаётся только финансовым знанием, не полномочием внешнего действия. PR не сливается агентом; независимая проверка и личное решение остаются за ответственными участниками.
