# Одно поручение Codex на стороне Тимофея

Скопировать блок полностью. Выполнение начинается в собственной сессии Тимофея.
Этот файл задаёт процесс, а не заранее положительный вердикт.

~~~text
Я Тимофей Патрушев, GitHub Tim1hik, назначенный проверяющий IAM-работы
Дмитрия Карачебана: hitpipe/hitpipe, Feature HIT-11, задачи HIT-17–24.
Режим MANUAL_LAB_PR12. Дмитрий поручил перевести командное ревью на ту же
схему делегирования Codex, которая используется для других участников.

Делегирую тебе фактическое концептуальное, code и document review,
выполнение проверок, подготовку evidence, публикацию замечаний/итога
и настоящих GitHub reviews из моей уже авторизованной сессии.
Разрешено отправлять APPROVE/REQUEST_CHANGES/COMMENT после собственной
проверки соответствующего PR. В тексте явно пиши: «Codex по делегации
Тимофея Патрушева». Не утверждай, что человек лично запускал команды.
Мне нужны итоговый вердикт и ссылки. Не требуй от меня сначала вручную
ответить на blind-сценарии, собрать окружение, пройти каждый PR или
нажать Submit review. Не переноси на меня разрешённую техническую работу.

1. ДОСТУП И СОХРАННОСТЬ
Определи ОС/shell/инструменты/текущий repo и незакоммиченные изменения.
Не reset/clean/force-push, не переключай чужую работу на review target.
Работай в новом уникальном clone/worktree, журналы храни снаружи.
Используй существующий GitHub connector или gh/действующую git auth.
Перед публикацией проверь реальную личность API/инструмента: должна
быть Tim1hik, не dkar-dev/другой студент/общий служебный аккаунт.
Проверь доступ к hitpipe/hitpipe. Право Tim1hik уже проверено: admin.
Не запрашивай и не показывай пароли, токены, приватные ключи.
Если нет login/2FA либо действие запрещено политикой, запроси только
это конкретное вмешательство и продолжай независимую доступную работу.
Не останавливай весь review из-за отсутствия Jira/Confluence connector.

2. ПОЛУЧЕНИЕ КОНТЕКСТА
Прочитай AGENTS.md, docs/labs/pr12/AGENT_LAB_WORKFLOW.md и
docs/labs/pr12/MASTER_PROMPT_TEMPLATE.md. Проверь SHA frozen
docs/labs/pr12/upstream/project-os/AGENT_MASTER_PROMPT.md по context-lock,
прочитай его. Это ограниченный manual lab route, не Project OS runtime.
Не вызывай /start,/review,bootstrap,FORCE,service-token bridge.
Читай docs/labs/pr12/context-lock.json, source-files.json, frozen task
exports docs/labs/pr12/tasks/HIT-11.json и HIT-17…24.json, четыре
актуальные методички/матрицу METHODICS_MATRIX.md и применимые
upstream Discovery/Product/Architecture/BP/formal sources.
Не требуй историю чужого Codex-чата/VPS. Exact copies в repo достаточны.
Нормы Candidate/In Review, grant=none; не присваивай им Accepted.

Прочитай актуальные TIMOFEY_REVIEW_GUIDE_RU.md и reviewer-assignment.json
из docs/labs/pr12/dk/review/. Они заменяют прежний manual-only review flow
только для командного технического ревью. Личные учебные требования
не удаляются, но они не являются условием начала твоей работы.

3. ТОЧНЫЕ REVIEW TARGETS
Запроси реальные full head/base SHA, diff, reviews и checks каждого PR:
12→HIT-17 R1, 13→HIT-18 R2, 14→HIT-19 C1, 15→HIT-20 C2,
16→HIT-21 C3, 17→HIT-22 D1, 18→HIT-23 H1, 19→HIT-24 REPORT.
Автор PR dkar-dev. PR2 bootstrap уже merged.
Полный проверяемый комплект — текущий head PR19
(ref codex/HIT-24-dk-lab-reports), не main и не исторический SHA из отчёта.
Через API/gh получи head, fetch нужную ref и сверяй git rev-parse HEAD.
Оригинальные PR stacked; отдельный diff каждого обязателен.
Запиши SHA один раз в review manifest; frozen task exports — historical,
live PR/GitHub state проверяй отдельно. Смена head в ходе review требует
оценки новой delta/повтора затронутых проверок, не переноса старого PASS.

4. СРЕДА И ЖУРНАЛЫ
Для полного локального прогона Go ровно 1.27.2, Python3.10+, git,
make, поддерживаемый C compiler для race; file/readelf для ELF inspection.
Сначала используй имеющиеся инструменты. При необходимости подготовь
user-local Go из официального источника с проверкой checksum, не меняя
системный Go. Не требуй root, если доступна user-local/CI альтернатива.
GOTOOLCHAIN=local, GOMAXPROCS=2, GOFLAGS=-p=1; для race CGO_ENABLED=1.
Адаптируй синтаксис к ОС: env передавай программно, не копируй Bash в PowerShell.

Для каждой команды фиксируй полный argv, cwd, start/end UTC, full SHA,
tool versions, stdout/stderr, настоящий exit и digest журнала.
Можно использовать Python subprocess, чтобы не ловить exit вручную.
PASS/FAIL/BLOCKED/NOT_RUN различай. Ошибка среды не доказанный дефект кода.
Сохраняй результаты вне source checkout до завершения mutation runner:
ему нужен clean git input. Исторические авторские логи не перезаписывай.

5. ПРОВЕРКИ
Из root checkout, python — найденный подходящий интерпретатор:
  python3 docs/labs/pr12/dk/reports/verify_package.py
  python3 tools/pr12/workspace.py validate
  python3 tools/pr12/test_workspace.py
  PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require
  python3 tools/pr12/workspace.py smoke iam --require
  python3 tools/pr12/workspace.py inspect iam --mode release --require
Из services/iam:
  go test ./internal/adapters/fixtures/sessionlookup -run TestSameConsumerContractWithBothAdapters -v -count=1
  go list -json ./...
  go doc ./internal/domain/sessionpolicy
  go doc ./internal/application/authenticatesession
Снова из root:
  python3 services/iam/tools/mutation_check.py --output <новый-внешний-каталог>

Для перечисленных обычных команд exit0. Mutation runner тоже exit0,
но внутри все7 behavioral mutants должны build0/test assertionFAIL,
source restored и final clean gate0. Первоначальный M07 survivor и
его retest — сохранённая история; не выдавай её за свой свежий опыт.
Hidden suite NOT_PROVIDED; coverage не является human review или profiling.

Если локальная машина не позволяет полный прогон, используй уже
существующий workflow pr12-dk.yml с workflow_dispatch и branch ref
PR19. Его default-branch версия существует; ref PR19 содержит текущую
IAM проверку package/tests/vet/race/smoke/coverage/семи mutations.
Например: gh workflow run pr12-dk.yml --repo hitpipe/hitpipe --ref codex/HIT-24-dk-lab-reports
Сохрани время запроса, найди именно новый workflow_dispatch run своего
аккаунта и сверь headSha с зафиксированным review target; считывай
состояние этого run, не последний случайный run ветки. Дождись completion,
скачай logs/artifacts, проверь protocol.json и фактически выполненные steps.
Не называй старый author CI новым независимым прогоном.
Недостающие go list/doc/inspection/свои probes выполни где доступны;
непроверенные свойства явно перечисли, критичный пробел даёт LIMITED.
При запрете Actions/исчерпании квоты сохрани checkpoint, не выдумывай PASS.
Самостоятельно проверяй доступные альтернативы до запроса ручной настройки.

6. НЕЗАВИСИМЫЙ АНАЛИЗ
Сопоставь requirements.md, requirement-change.md, module-contract,
FR/AC/NFR и frozen BP/formal с кодом, не только с авторскими тестами.
Проверь idle/absolute equality и precedence, revoked/unknown, неверные
snapshots, cancel/deadline, Lookup failures, clock sampling после Lookup,
fail-closed positive decision, raw JSON exact keys/числовые границы,
tenant/session mismatch, ownership/mutation, безопасные diagnostics.
VALID session не business grant; issuance/renewal/SQL/OIDC/полный IAM
не входят в эту порцию и не считаются реализованными.
Один consumer, consumer-owned ports, две реальные реализации,
normal/error/boundary для обеих, composition root и source dependency
должны соответствовать module map/imports/Godoc.
Проверь meaningful regression assertions, техники, risk matrix и CI.
При конкретной неопределённости добавь собственный воспроизводимый probe
в изолированном временном окружении/Go overlay; production source и
оригинальные тесты не меняй. Независимый oracle обоснуй нормой.
Не придумывай замечаний ради количества; не фиксируй авторский код в review.

7. ДОКУМЕНТЫ
Проверь четыре DOCX/Markdown, 29 критериев, 21 инструкцию плейсхолдеров,
template adjustments, SHA/CI/evidence digests и фактические conclusions.
verify_package PASS не заменяет чтение содержания/трассировки.
Команды для снимков должны работать на указанном SHA и правильном cwd,
не затирать исторические evidence; payloads/секреты не раскрывать.
PNG не делать и не генерировать. DOCX ZIP/XML/text/tables/links проверь
программно; renderer используй если доступен. Visual NOT_RUN явно укажи,
не требуй Word от человека как предварительное условие code review.
Личные student blind/peer/clean-room/module actions, интервью, внешнее
оппонирование, снимки, подписи и защита остаются отдельными human records.
Не сочиняй ответы от имени Тимофея и не отмечай их пройденными по агентному
анализу или публикации через его аккаунт. Возможный AI clarity analysis
называется агентным; если oracle уже был доступен, не называй его blind.

8. ПУБЛИКАЦИЯ
Подготовь review manifest, checks/logs, findings и итог по
TIMOFEY_REVIEW_RESULT_TEMPLATE.md. Каждый finding:
DK-REV-NNN, priority, affected PR/full SHA, файл/строки или критерий,
наблюдаемая проблема, ожидаемый исход/основание, repro, evidence,
требуемое исправление и способ retest. Отделяй введённый дефект от
унаследованного; не копируй один finding восемь раз.
Опубликуй только reviewer artifacts в своей отдельной branch/record;
авторские branches/services/нормативные sources/frozen evidence не правь.
Запиши immutable commit/URLs. Если создаёшь PR для review artifacts,
обозначь его review-only и не делай его merge условием исправления автора.

Сводное заключение опубликуй в PR19. Отправь настоящее review каждого
действительно проверенного PR12–19 через connector/gh/API:
APPROVE только при достаточной проверке этого scope и актуальном head;
REQUEST_CHANGES при конкретных требуемых исправлениях;
COMMENT при неполной проверке/внешнем ограничении. Не делай массовый
approval только по зелёному CI. При API укажи точный commit_id head.
Везде укажи «Codex по делегации Тимофея Патрушева; это агентное ревью».
Проверь readback submitted state/body/commit/account/URL каждого review.
При timeout сначала читай existing reviews/comments, не создавай дубликаты.
Если авторизация неправильная, не имитируй Tim1hik и не отправляй от автора.

В HIT-23 добавь реальные ссылки/вердикт/scope и отдельные human pending.
Если Jira connector нет, сохрани jira-update.md в том же опубликованном
review record и включи ссылку в GitHub summary; техническое ревью заверши.
Не меняй assignee автора, не обманывай Jira автоматику, не вызывай
официальный Project OS и не закрывай задачи/Feature.
Не делай merge, retarget/rebase/force-push авторских PR, submission tags,
не меняй rulesets/permissions. Это поручение на review, не интеграцию.

9. ИТОГ
Вердикт отдельно для technical/code и document preparation:
APPROVED_FOR_SCOPE, CHANGES_REQUIRED либо LIMITED_ENVIRONMENT.
Если замечания есть — создай единый AUTHOR_CORRECTION_PROMPT_RU.md
для агента Дмитрия, с перечнем DK-REV, actual источниками, готовыми
решениями внутри frozen scope, проверками и критериями повторного review.
Не отправляй разрешённые продуктовые трактовки на повторное согласование.
Не представляй будущий retest выполненным. Никакого заранее заданного PASS.

Закончи кратким ответом: что проверено, итог, основные проблемы/границы,
ссылки на submitted reviews/evidence/поручение исправлений и что остаётся
личным учебным действием. Мне останется передать Дмитрию этот результат.
Работай до публикации реального итога, не останавливайся на плане/черновике.
~~~
