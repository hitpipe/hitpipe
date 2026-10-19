# HIT-15 — ревью Никиты Будыкина и дальнейшая работа

19.10.2026. Проверяющий со стороны команды — **Дмитрий Карачебан**; фактический
анализ кода, нормативных оснований, тестов и документов — **Codex по его делегации**.
Прежнее назначение Стацюка на командное ревью HIT-15 отменено:
[текущее назначение](reviewer-assignment.json). Отдельного анализа концепции и кода
со стороны Lead не требуется.

**Результат: SOURCE REVIEW COMPLETED / CHANGES REQUIRED. Работа пока не принята.**
Это состоявшаяся проверка опубликованных исходников. Авторских PR пока нет, поэтому
GitHub APPROVE/CHANGES_REQUESTED к ним не выставлялись и не выдумываются.

## Проверенный источник и фактическое состояние

Полный пакет проверен в отдельном worktree на
`b23001983b511999d815fbc7f77063e763516bc2`, ветка `work/HIT-56-nb-reports`.
Все восемь веток доступны. Их точные heads и native CI сохранены в
[branch-inventory.json](branch-inventory.json). Родительские линии веток неодинаковы:
нельзя подставлять предполагаемые base branches или заменять отсутствующие PR
ссылкой «Create PR», полученной при push.

| Task | Ветка | Опубликованный head |
| --- | --- | --- |
| HIT-49 / R1 | work/HIT-49-nb-requirements | `9659c39ad0010e1329bfe3e3bc92c9edd84982bb` |
| HIT-50 / R2 | work/HIT-50-nb-refinement | `35b4c247319cf3bf0c8d8288d109c996fc721d0b` |
| HIT-51 / C1 | work/HIT-51-nb-stripe-signature | `8634767abf8fa235407db3581a8c38705bbdebb8` |
| HIT-52 / C2 | work/HIT-52-nb-authenticate-webhook | `2a20fb6a21fe762f2cf37ded433c99bf5390be81` |
| HIT-53 / C3 | work/HIT-53-nb-verification | `2186a00ca9c441645c99f82fd45b34241864ac6a` |
| HIT-54 / D1 | work/HIT-54-nb-docs | `a74a5e1a8ada1147b5755503fd01fd458b8a4706` |
| HIT-55 / H1 | work/HIT-55-nb-human-gates | `78a4ecb87a50ec79f8979b08a4449fcfd3d77c6b` |
| HIT-56 / REPORT | work/HIT-56-nb-reports | `b23001983b511999d815fbc7f77063e763516bc2` |

У `Budykin` есть **write** в `hitpipe/hitpipe`. Для начала исправлений не требуется
изменение прав или предварительный merge пакета ревью. PR создаёт автор/его агент
под собственной учётной записью; Lead отвечает за последующее review/merge.
Текущий режим — `MANUAL_LAB_PR12`, без официальных `/start`/`/review` Project OS.
Discovery/Product/Architecture не повышены из Candidate в Accepted.

## Что действительно подтверждено

- Frozen bundle validation и 17 тестов wrapper — PASS.
- Именованные тесты Intake, vet, **race**, dev/release smoke вне checkout — PASS
  на Linux / Go 1.27.2. Exact argv, source hashes, время и exits:
  [evidence/review-checks.json](evidence/review-checks.json).
- Независимые HMAC-векторы, рассчитанные Python hashlib/hmac, проходят, включая
  настоящие невалидные UTF-8 bytes и изменённый raw body. Новые границы timestamp
  дают **три компилируемых FAIL**; это не ошибки сборки и не авторские мутации.
- Все **25** авторских report/evidence hashes совпали.
- Пять авторских mutation patches/logs действительно содержат compile exit0,
  test exit1 и содержательные assertion failures. Baseline source hash совпадает
  с текущим verifier; не объявляется, что reviewer повторно исполнил весь опыт.
- Есть 29 записей crosswalk, содержательные R1/security/ADR/test-design/risk
  документы и четыре DOCX с 3+5+7+6 слотами. Обнаружены описанные ниже пробелы.
- Native CI полного авторского head `b230019…` содержит четыре успешных контекста;
  Intake run [39263670615](https://github.com/hitpipe/hitpipe/actions/runs/39263670615).
- Unit statement coverage всего Intake **53.8%**. По областям: domain87.7%,
  application83.3%; это наблюдение, не введённый reviewer-ом процентный порог.
  Отсутствие instrumented coverage у adapter не означает, что он не запускался
  через application test; SystemClock отдельно не показан в выполненном DI опыте.

## Замечания и условия исправления

### NB-REV-001 · P2 · Переполнение parsing и вычисления freshness

`verifier.go:parseUnix` проверяет `next < parsed` после умножения, что не ловит
все переполнения int64. Строка `23000000000000000000` принимается как
`4553255926290448384`. Публичный verifier выдаёт stale вместо malformed.
Кроме того, `now=-9223372036854775808`, `t=9223372036854775807` превращаются при
`now-t` в delta1: с соответствующим synthetic MAC получается `VERIFIED`, хотя
математическая абсолютная дистанция значительно больше300s.

Это проверка крайних входов чистого модуля/ошибочного Clock, **не доказанный bypass
живого Stripe endpoint**. Обычные позитивные clocks и тесты ±299/300/301 проходят.
Основание: FR-SIG-02, HP-BP-002/T/001, fail-closed clock uncertainty BP007/T/010.

Нужны checked decimal parsing, явный допустимый clock/timestamp domain и безопасное
сравнение дистанции без signed overflow. Overflow/out-of-range timestamp отклонять;
невалидный Clock не должен давать положительное доказательство. Сохранить inclusive
300s и неизменяемый tolerance. Новая dependency или изменение Blueprint не требуются.

Repro: `TestReviewTimestampParserRejectsOverflow`, `TestReviewOverflowHeaderReportsMalformed`,
`TestReviewAbsoluteDistanceCannotWrap`; исходники и журнал в `probes/`, `evidence/`.

### NB-REV-002 · P2 · Не опубликован PR workflow и не связана работа с Jira

Восемь remote branches есть, авторских PR — **ноль**. Карточки HIT-49–56 были
Backlog без execution/evidence комментариев. H1 предлагает, чтобы Lead создавал PR
и заполнял их URL вручную, хотя автор имеет write и может выполнить публикацию сам.

Автору восстановить актуальные heads, сохранить всю локальную работу, согласовать
зависимые ветки обычными merge и опубликовать настоящие PR. Сохранить три отдельных
осмысленных code PR HIT-51/52/53; остальные результаты тоже связать со своими Task.
Рекомендуемая конечная цепочка base: R1→main, R2→R1, C1→R2, C2→C1, C3→C2,
D1→C3, H1→D1, REPORT→H1. Это **целевой порядок после интеграции родителей**,
а не выдуманное описание сегодняшней истории. Не force-push и не терять ветки.
Проверить содержимое diff каждого PR, чтобы он не удалял или дублировал чужие результаты.

В Jira записать реальные task→branch→PR→SHA→checks. Если штатная автоматика мешает
ручному lab-status, не подделывать start/freeze: фиксировать actual result и проблему
в комментарии, продолжая разрешённую независимую работу. После новых PR и исправления
нужен независимый повторный verdict; self-approval/merge не выполняется.

### NB-REV-003 · P2 · Invalid configuration ошибочно объявлена NOT_APPLICABLE

Handoff и 0104-pz01-05 предлагают спросить Lead/преподавателя об исключении обязательного
опыта. При этом CLI уже поддерживает runtime параметр `--format text|json` и
отклоняет другое значение с exit2. Tampered signature — ошибочные данные, а не
тот же опыт неправильной конфигурации.

**Решение принято:** использовать существующий `--format` как конфигурацию вывода.
Показать один valid fixture в text и json и `--format invalid` → bounded reason
`format_invalid`, exit2. Исполнить и сохранить отдельный журнал, обновить отчёт и
слот5. Не вводить фиктивный config flag и не запрашивать отказ от требования.

### NB-REV-004 · P2 · Отчёты и toolchain содержат неверные/незавершённые выводы

В 0104 ПР1 вся dev/release таблица заполнена одинаковым `NOT RUN / PASS` вместо
наблюдений. В 0302 ПР1 техническая таблица результатов остаётся пустой при наличии
авторских проверок. Текст местами всё ещё instructs «указать источники» вместо
реестра реально использованных материалов; часть CI/SHA сведений устарела.

0104 ПР2 приписывает DI-опыту `file-backed + SystemClock`, но в обеих реализациях
используется **fixedClock**. SystemClock скомпилирован, но в этом опыте не исполнен.
FileResolver исполняется application-тестами, а CLI использует другой fixture resolver;
формулировка «все adapters проверены smoke» неверна. Покрытие не доказывает иной запуск.

`toolchain.md` называет coverage профилированием. **Решение:** coverage оставить
тестовой метрикой; CPU/memory profiling объявить PLANNED с реальным Go pprof route,
объектом и условием будущего запуска, если такой опыт ещё не выполнен. Не изображать
результат измерения производительности и не расширять лабораторную ради нового стека.

Заполнить наблюдения dev/release по фактическим flags/inspection; все технические
таблицы/выводы/источники, точные refs и статусы. Честные human pending строки могут
остаться. Повторить DOCX integrity/link/text checks, сохранить 21 слот без PNG.
Исходные DOCX были только структурно проверены reviewer-ом; renderer QA здесь
не повторялся. Авторское утверждение render/inspection не выдается за наш запуск.

### NB-REV-005 · P2 · Прослеживаемость и нормативные вопросы требуют выверки

В requirements-trace safe diagnostics привязаны к BP007/T/010 (timeout/cancel),
а privacy к BP002/T/003 (outbox). Исправить смысловые источники: BP007/T/005 для
безопасных boundary errors, BP002/T/006/T/008 и соответствующие Product NFR для
секретов/логирования. BP007/T/010 применим именно к clock/deadline uncertainty.

29-entry crosswalk перечисляет одинаковые группы файлов для разных критериев;
`module-contract.md` как относительный NB path не существует: документ находится
в `services/intake/docs/module-contract.md`. Нужны exact repo paths, реальные HIT
keys, релевантный результат, check/log/CI/source и actual status для каждой строки.
Не отмечать техническое условие human-only просто из-за того, что снимок вставит человек.

В risk register `low × serious` у CI-риска ошибочно даёт high. При declared mapping
serious=значимый матрица методички даёт medium. Либо исправить результат, либо честно
переоценить P/impact с основанием, не менять саму матрицу ради желаемого приоритета.

**Закрытые решения:** v1-only и фиксированные300s уже определены T/001; принятые
safe reason codes и равные duplicate timestamps остаются локальным R2 решением,
согласованным с запретом contradictory timestamps. Повторного продуктового утверждения
не требуется. Provider API/event/version и retention limits брать из frozen BP/NFR;
реальные permissions остаются live validation, будущие receipt/transport — deferred
implementation. Не выдавать эти границы за требуемые новые решения Lead.

### NB-REV-006 · P2 · Инструкции снимков пока не полностью воспроизводимы

Конкретные дефекты:

- Slot0104-pz01-05 содержит «No valid command» вместо уже работающего invalid format.
- Slot0104-pz01-03 ссылается на старую temporary distribution, которую wrapper удаляет.
  Нужны команды создать новую дистрибуцию, скопировать binary/fixtures, выйти из checkout
  и выполнить `--version`/fixture, с явным cwd и сохранённым результатом.
- Slot0302-pz02-05 заканчивается `go test -race ./...` из repo root без Go module;
  нужен `cd services/intake` либо тот же корректный wrapper.
- Coverage инструкция перезаписывает versioned evidence файл. Генерировать новый
  профиль в ignored `.pr12-build`, исторический журнал не менять.
- Снимки карты/GoDoc задают C2 SHA, где некоторые D1 документы ещё отсутствовали.
  Проверить наличие каждого файла именно в указанном commit и согласовать финальный source.
- В описании подмены снова указано неисполненное SystemClock; в mutation slot нет
  полной команды получения/применения patch к изолированной копии.

Перепроверить все21 инструкции из чистого checkout. В них должны быть шаги выбора
source/version, cwd+полный argv, ожидаемый реальный exit, visible evidence, куда вставить
и redaction. Human-only события честно описывают последующее действие, но технические
опыты исполняет агент. Картинки не создавать и не генерировать.

## Следующие шаги

[Готовое поручение агенту Никиты](AUTHOR_CORRECTION_PROMPT_RU.md) включает получение
review context без смены рабочей ветки, закрытие NB-REV-001–006, публикацию PR и
единый correction-record. До этого не требуется действий Lead с правами или продуктом.
Reviewer branch — контекст и доказательства, не замена авторского source tree.

Нельзя приравнивать source review к проведённому GitHub PR review, человеку-студенту,
интервью, личной blind/clean-room/module проверке, opposition, подписи, защите или
formal phase acceptance. Эти статусы не выдуманы; Feature/Epic не закрыты.

## Внешняя сверка

Порядок проверки неизменённого raw body и endpoint signing secret сопоставлен с
[официальной документацией Stripe](https://docs.stripe.com/events/manage-webhook-endpoints#signature-errors).
Это справочная проверка, не live Stripe experiment. Установка SDK не требуется;
исправление относится к существующему stdlib модулю. Выбранные дополнительные
ограничения HitPipe определяются pinned BP, а не автоматически копируются из SDK defaults.
