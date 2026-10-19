# Тимофею Патрушеву: проверка работы Дмитрия Карачебана

Назначение от Дмитрия: **19 октября 2026**. Проверяющий — Тимофей Патрушев, GitHub `Tim1hik`. Участок — IAM / Feature [HIT-11](https://hitpipe.atlassian.net/browse/HIT-11), человеческие проверки — [HIT-23](https://hitpipe.atlassian.net/browse/HIT-23). Работа идёт в ручном учебном режиме MANUAL_LAB_PR12.

Эта инструкция заменяет прежнее назначение NB → DK **только для проверки Дмитрия**. Общий frozen manifest не переписывается; действующее назначение — [reviewer-assignment.json](reviewer-assignment.json). Обратная проверка Дмитрием работы Тимофея остаётся отдельным действием. Назначенное преподавателем оппонирование другого проекта не заменяется проверкой товарища из HitPipe.

## 1. Что уже готово и чего от тебя ждут

Для начала ревью готов весь технический пакет: исходники, требования, три отдельных code PR, тесты, CI, доказательства и четыре заполненных отчёта с инструкциями вместо снимков. Тебе не нужно заново разрабатывать IAM или составлять задачи.

Нужно лично проверить понятность требований, воспроизвести участок в чистом каталоге, рассмотреть код и отчёты, затем оставить собственное решение. Успешный CI помогает проверке, но не является твоим ревью.

Не объявлять лабораторные полностью сданными: интервью, оппонирование другого проекта, настоящие снимки, личные реквизиты и защита ещё pending. Открытый human gate сам по себе не мешает начать code/document review; он должен оставаться видимым в итоговом заключении.

На момент подготовки инструкции PR #12–19 открыты, прежний проверенный head пакета — `9a06ca2b45aaf874144d1084e33d8a381fbd2941`, все четыре его CI checks успешны. PR #2 с общим bootstrap уже merged **19.10.2026 09:04 UTC**. Текущая поправка в PR #19 меняет назначение/инструкции/отчёты; IAM source остаётся прежним. Перед проверкой зафиксируй текущий head #19, а не принимай этот исторический SHA за новый.

## 2. Сначала проверка однозначности — до тестов и авторского oracle

1. Войди в GitHub своим аккаунтом `Tim1hik`. Он уже имеет доступ к backend-репозиторию; аккаунт автора PR — `dkar-dev`, под ним проверять нельзя.
2. Открой [PR #13](https://github.com/hitpipe/hitpipe/pull/13), **Files changed**, документы `requirement-change.md` и `module-contract.md`. Это контракт, написанный до кода; исходный R2 SHA `9d7c37151606656e9dc2f2f65876b8bd93b397cd`. Дополнительно прочитай [требования участка](../requirements.md).
3. Открой [шесть ситуаций](blind-scenarios.md). **Не открывай пока `oracle.md`, тесты с expected values, протоколы мутаций и вывод программы. Не поручай агенту отвечать на эти шесть вопросов.**
4. Для каждой ситуации сам запиши outcome, reason, можно ли считать бизнес-действие разрешённым и какой пункт текста даёт ответ. Зафиксируй ответы отдельным комментарием в PR #13, указав дату, своё имя и SHA текста. Не редактируй первоначальный ответ после сверки: уточнения добавь отдельным дополнением.
5. Только после публикации ответов открой `oracle.md`, сравни и зафиксируй совпадения/расхождения. Не придумывай замечания ради их наличия. Если текст двусмысленен — укажи конкретное место и допустимые прочтения; Дмитрий исправляет, затем ты проверяешь повторно.

Форма для ответов и итогового заключения — [TIMOFEY_REVIEW_RESULT_TEMPLATE.md](TIMOFEY_REVIEW_RESULT_TEMPLATE.md). Она пока не заполнена фактическими ответами и не является evidence.

## 3. Зафиксируй один проверяемый checkout

Используй **свой компьютер или отдельное чистое окружение**. Не запускай проверки в авторской рабочей копии на VPS и не сбрасывай чужие изменения. Для приведённых команд удобны Linux или WSL с Bash; нужны Git, Python **3.10+**, Go **1.27.2**. Для race detector нужен поддерживаемый C compiler; для ELF inspection — `file` и `readelf` из binutils. Go бери из [официальных загрузок](https://go.dev/dl/).

В новом каталоге, которого ещё нет:

```bash
git clone https://github.com/hitpipe/hitpipe.git hitpipe-dk-review
cd hitpipe-dk-review
git fetch origin codex/HIT-24-dk-lab-reports
git switch --detach FETCH_HEAD
git rev-parse HEAD
git status --short
go version
python3 --version
```

Запиши полный SHA. `git status --short` должен быть пустым, Go — ровно `go1.27.2`. Если папка уже есть, выбери другое имя; не делай reset/clean чужой копии. Открой PR #19 → **Commits / Checks**, сравни head с полученным SHA. При новых commits во время проверки сохраняй старый результат как относящийся к старому SHA; новую версию проверяй отдельно.

Не заполняй tracked JSON/Markdown протоколы в этом checkout до окончания mutation experiment: ему нужен clean source. Логи и свои записи сохраняй **вне репозитория**, а заключение публикуй через GitHub/Jira.

## 4. Выполни проверки и сохрани действительные результаты

После шага 2 можно использовать своего агента для выполнения команд и разбора кода. Ты должен просмотреть его действия, результат и самостоятельно сформулировать заключение. Агент не публикует approval/подпись от твоего имени и не изображает другого студента.

Команды из корня checkout:

```bash
export GOTOOLCHAIN=local
export GOMAXPROCS=2
export GOFLAGS='-p=1'

python3 docs/labs/pr12/dk/reports/verify_package.py
python3 tools/pr12/workspace.py validate
PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require
python3 tools/pr12/workspace.py smoke iam --require
python3 tools/pr12/workspace.py inspect iam --mode release --require

cd services/iam
go test ./internal/adapters/fixtures/sessionlookup -run TestSameConsumerContractWithBothAdapters -v -count=1
go list -json ./...
go doc ./internal/domain/sessionpolicy
go doc ./internal/application/authenticatesession
cd ../..

python3 services/iam/tools/mutation_check.py
```

После **каждой** команды зафиксируй её реальный exit code до следующей команды (`printf 'exit=%s\n' "$?"`), cwd, SHA, stdout/stderr и дату. Сохраняй terminal transcript или вывод средствами своего агента во внешний каталог. Не продолжай молча после ошибки. Для обычных проверок ожидается exit **0**. `BLOCKED` из-за отсутствия Go/C compiler — не PASS, сначала исправь среду или явно запиши ограничение.

Что подтвердить в результатах:

- `verify_package.py`: 4 отчёта, 29 результатов, 21 уникальный placeholder, hashes/evidence совпадают; human acceptance=false.
- `validate`: точные frozen sources совпадают. Это проверка контекста, не всего продукта.
- `check iam --require`: действительно запущены именованные IAM tests и race; нет skipped/TODO, выданных за успех.
- `smoke`: оба режима dev/release, копирование executable и fixture во временный каталог **вне checkout**, проверка версии и неверного ввода.
- `TestSameConsumerContractWithBothAdapters`: под одним consumer выполнены file/fixed × normal/error/boundary и остальные заявленные случаи.
- Mutation runner: все семь дефектов компилируются, обнаружены assertions, каждый восстановлен, конечный clean gate успешен. Общая команда runner возвращает **0**, хотя внутри протокола mutant tests должны падать. Не путай эти два уровня.

Ничего не засеивай вручную в сдаваемую ветку. Runner сам использует disposable Git worktree/temporary branch и сохраняет результаты в `.pr12-build/iam/mutations/`. Скопируй этот каталог во внешние логи после завершения. Не смешивай новый свой прогон с авторскими журналами: они имеют разные SHA/среду/даты.

## 5. Посмотри изменения по отдельным PR

Полный текущий код находится в checkout #19, но существенные изменения разделены на PR. Просматривай **Files changed** каждого, чтобы оценить собственно его diff:

| PR | Что проверяется |
| --- | --- |
| [#12 — HIT-17](https://github.com/hitpipe/hitpipe/pull/12) | Источники, FR/NFR/AC, MVP-границы, безопасность, исходные fixture и честный статус интервью |
| [#13 — HIT-18](https://github.com/hitpipe/hitpipe/pull/13) | Существенное уточнение контракта до кода; твой clarity-протокол |
| [#14 — HIT-19](https://github.com/hitpipe/hitpipe/pull/14) | Pure SessionPolicy, реальные проверки правил и безопасный диагностический CLI |
| [#15 — HIT-20](https://github.com/hitpipe/hitpipe/pull/15) | Один AuthenticateSession, consumer-owned ports, две реализации и одна composition root |
| [#16 — HIT-21](https://github.com/hitpipe/hitpipe/pull/16) | Assertions, техники тестирования, coverage gaps, mutation survivor/retest, risks и CI |
| [#17 — HIT-22](https://github.com/hitpipe/hitpipe/pull/17) | Фактические imports/module map, Godoc, личный ADR, toolchain и dev/release observations |
| [#18 — HIT-23](https://github.com/hitpipe/hitpipe/pull/18) | Подготовка человеческих процедур; не выданы ли планы за уже проведённые проверки |
| [#19 — HIT-24](https://github.com/hitpipe/hitpipe/pull/19) | Весь актуальный report/review package, включая новое назначение Тимофея |

Старые #12/#18 и их исторические версии содержат прежнее NB → DK: это не выполненное ревью Никиты. Новое назначение находится в текущем #19 и явно supersedes прежний личный routing. Общий bootstrap [#2](https://github.com/hitpipe/hitpipe/pull/2) уже слит: повторный merge ему не нужен.

Чеклист кода: source dependency direction соблюдён; правила не продублированы в CLI; production-oriented consumer действительно вызывается; две реализации меняются без правки consumer; nil/cancellation/ошибки обрабатываются по контракту; значения не изменяют caller data; внешние effects и business grants не появляются из диагностики; raw inputs/секреты не печатаются. Сравни каждую спорную ветвь с контрактом, а не с тем, что тест случайно ожидает.

Чеклист verification: ожидаемый результат задан до кода; есть normal/error/boundary; coverage не подменяет проверку требований; первоначальный M07 survivor и его retest сохранены; compiler error не засчитан за найденный behavioral defect; source восстановлен; естественный дефект не смешан с seeded/hidden; риски влияют на набор/частоту проверок.

## 6. Проверь отчёты и фактические границы сдачи

Открой [reports](../reports/README.md), четыре DOCX и их Markdown. В Word/LibreOffice проверь открытие, таблицы, поля, оглавление и читаемость. До вставки реальных снимков не утверждай окончательную пагинацию. Для code review не требуется самому вставлять Дмитрию все рисунки.

Проверь [29-критериевую матрицу](../criteria-crosswalk.json): 21 результат agent-checked, 8 результатов зависят от человека. Placeholder — инструкция, не снимок и не доказательство интервью/review. Неизвестные группа/кафедра, подпись/оценка и человеческие ответы должны оставаться незаполненными до настоящего действия.

Решение «код и честно обозначенная подготовка документов корректны» отличается от «все лабораторные выполнены и сданы». Можно принять первое и сохранить открытыми соответствующие учебные human gates. Нельзя отметить интервью/оппонирование состоявшимися только потому, что ты одобрил код.

## 7. Зафиксируй результат своим аккаунтом

1. Скопируй [форму результата](TIMOFEY_REVIEW_RESULT_TEMPLATE.md) в комментарий **PR #19**. Впиши свои реальные имя/account/date, полный проверенный SHA, команды/exit/logs, scope и наблюдения. Назови, что выполнил сам, а где использовал агента как помощника.
2. Замечания к коду/тексту оставляй в соответствующем PR: **Files changed → строка → “+” → Start a review**. Для каждого укажи файл/правило, наблюдаемую проблему, ожидаемое поведение и проверку исправления.
3. Заверши фактическое review каждого просмотренного PR через **Review changes → Comment / Request changes / Approve → Submit review**. `Comment` сам по себе не approval. При существенном дефекте — Request changes; после исправления — настоящий retest и новое решение.
4. Если проверены все восемь PR, каждому нужно собственное review-решение: approval #19 не одобряет автоматически #12–18. Не одобряй непросмотренный PR только по общей зелёной сводке.
5. В [HIT-23](https://hitpipe.atlassian.net/browse/HIT-23) добавь ссылку на своё реальное GitHub-заключение и какие именно clarity/scaffold/module действия выполнены. Достаточно комментария; не нужно вызывать Project OS `/start`, `/review`, FORCE или перемещать всю Feature в Done.

Порядок кнопок сверён с [официальной инструкцией GitHub](https://docs.github.com/en/pull-requests/how-tos/review-pull-requests/reviewing-proposed-changes-in-a-pull-request). Pending review-комментарии нужно именно отправить через Submit review, иначе их не увидит автор.

## 8. Merge и дальнейшие действия

Твоя текущая роль — **независимое человеческое ревью**. Merge, изменение base, rebase/squash, теги и закрытие Feature оставь Дмитрию либо отдельному явно уполномоченному исполнителю. Наличие admin прав не заменяет это разделение поручений.

PR stacked: после принятия сначала интегрируется #12, затем #13 и далее. Bootstrap уже в main; Lead проверяет правильный base/ancestry и актуальный CI. После изменения исходников/SHA требуется повтор затронутых проверок/review. Не запускай массовый merge всех PR одной командой.

На 19.10.2026 main защищён **rulesets**, а не classic branch protection: действуют независимое approval, last-push approval, resolution review threads и четыре strict CI checks. HTTP404 от classic `/branches/main/protection` в данном репозитории не означает отсутствие защиты; проверены реальные branch rules. Настройки в рамках этой инструкции не менялись.

Jira runtime пока конфликтует с manual лабораторными переходами; реальные результаты отражаются в PR/comments/labels, а не фиктивным formal verdict. Это не мешает тебе читать код и публиковать настоящее GitHub review.

## Промпт твоему агенту после собственных blind-ответов

```text
Я Тимофей Патрушев, назначен человеком проверяющим IAM-работу Дмитрия Карачебана (HIT-11/HIT-23). Помоги провести техническую проверку PR12–19 в hitpipe/hitpipe. Собственные blind-ответы я уже отдельно зафиксировал; не сочиняй их и не называй свою работу независимым человеческим review.

Прочитай AGENTS.md, MANUAL_LAB_PR12 workflow и docs/labs/pr12/dk/review/TIMOFEY_REVIEW_GUIDE_RU.md на текущем head PR19. Зафиксируй exact SHA, работай в чистом отдельном clone, логи храни вне repo. Выполни проверку пакета, frozen sources, реальные IAM tests/race, dev/release external smoke, contract substitution и compiling mutation experiment по инструкции. Сопоставь исходники с контрактом, imports и evidence. Различай PASS/FAIL/BLOCKED/NOT RUN; результаты других запусков не выдавай за новые.

Подготовь мне перечень замечаний с файлами/правилами/воспроизведением и черновик заключения с реальными командами/выходами. Не меняй авторский code/нормативные sources и не присваивай выполненность отсутствующим human gates. Не отправляй review/approval, не делай merge/rebase/tags, не закрывай Jira от моего имени. Я сам изучу результаты, приму решение и опубликую своё review.
```
