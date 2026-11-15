# HIT-95 — защищённое чтение: независимые части проверены, полный DoD не готов

Статус: **PARTIAL / SOURCE_DECISION_PENDING**, не READY. Выполнено Codex по
делегации Тимофея Патрушева / Tim1hik. Независимое ревью/личная приёмка не проведены.

## Результат и остаток

Добавлены read-only application ports и приватный Processing HTTPS adapter для
listCases/getCase/getCaseHistory, owner PostgreSQL reader, scoped content reader
и подписанные курсоры. У каждой операции fresh IAM authority port и общий
actual-start/epoch boundary до SQL/content и повторно перед началом ответа.
Разрешающих заглушек/ключей по умолчанию нет. Listener принимает только явно
заданный private IP:8443 (loopback:0 для тестов), TLS1.3, доверенный client CA,
проверенный BFF SAN; browser authority headers не являются полномочиями.

Согласованного источника order_label и правила next_action нет. Участник подтвердил
это 15.11.2026 и поручил продолжить независимые части без SQL/OAS/бизнес-изменений.
Непустые listCases/getCase возвращают503 READ_UNAVAILABLE. Полный CaseDetail,
его nullable/readback/actions проекции и положительный непустой Cases200 ещё
требуют завершения после решения; сам факт наличия DTO не является проверкой API.

- [Точечный запрос решения Lead](LEAD_REQUEST_RU.md).
- [Карта каждого обязательного поля](FIELD_MAP.md).
- [Exact source audit и ограничения](source-audit.json).

История с полными явно сохранёнными данными читается. Реальная начальная
SIGNAL_ADMITTED от HIT-78 не имеет history_content.expires_at и возвращает503:
срок не выдуман из at+90days. Positive fixtures отдельно сохраняют synthetic
metadata/owner files в новой тестовой БД; они не маскируют этот production-writer
пробел и не изменяют старые writers.

## Версии и reuse

База: HIT-79 `3bae6417421465868c432fe18b379d46be6144aa`, включая HIT-78
`c218eb3d87c1df51d999d955d263b9d410f77bd7`. Обе задачи Done, но PR50/52 наблюдались
OPEN; это зависимая ветка, а не принятый main. Наблюдавшийся main —
`ce771d37505e7770003d0796ae932556d06b7bab`. Отдельный PR53/HIT-80 сохранён,
его оптимизация здесь не повторялась и ветки не переписывались.

Проверенная текущая кодовая версия:
`444aa011080cc7b321e3f37b0a0d0d5bb1f0e9de`. Последующие изменения этого пакета —
документация и представление частичного результата; нет попытки коммитить отчёт
ради собственного SHA. Финальный head/CI фиксируются внешним наблюдением в Jira.

Все production изменения — новые read-only файлы application/readcases,
adapters/casereadhttp, postgres/read_cases.go и contentfiles/read_owned.go.
Admission, подготовка/финансовые правила/writers, store.go, migration, go.mod/sum,
другие сервисы, клиент и общий CI не менялись. Доказательство —
[supplemental/commands.json](supplemental/commands.json).

## Фактические проверки

Полный воспроизводимый прогон05: [96 exact commands](verification/commands.json),
[итог с full_dod_ready=false](verification/summary.json). Обязательные положительные
команды exit0; ожидаемые отрицательные SQLSTATE/начальные readiness probes и
cleanup-probe сохранены с настоящими exit codes. cleanup-probe exit1 означает,
что принадлежащий прогону контейнер уже отсутствует. Рабочие данные/DSN не принимаются.

- Go1.27.2 darwin/arm64: полный `go test -race -count=1 -v ./...`, `go vet ./...`,
  оба exit0. [Полный race stdout](verification/go-race.stdout),
  [vet stdout](verification/go-vet.stdout), [Go env](verification/go-env.json).
  DB tests здесь явно SKIP: этот host запуск не выдаётся за Linux/PG race.
- Go1.27.2 linux/arm64 CGO_ENABLED=0: настоящий PostgreSQL18.7 Bookworm,
  pinned image digest из согласования HIT-95/12254; приватный Unix socket,
  network-none, fsync/full_page_writes/synchronous_commit on. Полный HIT-76/78/79
  regression и новые read tests exit0: [полный stdout](verification/postgres-tests.stdout).
  Linux-race и физическая power-loss qualification не заявлены.
- Настоящий TLS1.3 handshake: missing/foreign/untrusted/expired client cert,
  TLS1.2, отсутствие сессии, forged authority header, stale authority/epoch,
  закрытый SQL start, deny после SQL перед response start, отсутствие зависимости.
  До разрешения нет repository read; после revoke нет protected200.
- RLS на реальной non-owner/NOBYPASSRLS роли приложения; scoped READ ONLY
  REPEATABLE READ, SET LOCAL, две параллельные workspace, повторное использование
  соединения после commit/error/rollback, чужой case/cursor и отсутствие утечки scope.
  Реальный SQL INSERT внутри READ ONLY отвергнут; GET не создаёт бизнес-записи.
- History keyset:100+2 страницы по настоящему HTTPS→application→PostgreSQL→files,
  строгие nullable/UTC поля, без повторов; реальный case SQL keyset/filter и bounds
  проверены отдельно без выдуманных display полей. API пагинация непустого Cases
  пока не считается подтверждённой.
- Missing/corrupt/hash mismatch/symlink/wrong owner/expired content и traversal;
  expiry-on-read удаляет только проверенный собственный файл, safety остаётся.
  Настоящая отмена SQL pg_sleep по deadline, заранее отменённый context, закрытое
  DB соединение, принудительный rollback чтения до emit — без успешного view.

Семь [реальных API captures](verification/api-capture.json) проверены по точным
accepted OAS bytes: History200100+2, getCase503, listCases503,
listCases200 **пустой фильтр CONFIRMED**, Error400/403.
[Вывод schema проверки](verification/schema-check.stdout). Валидатор реализует
закрытое используемое подмножество OAS/schema, отвергает неизвестные keywords и
проверяет sensitivity удалением required поля; это не заявка на реализацию общего
JSON Schema движка. Cases/CaseDetail с непустыми проекциями не подделывались.

Команды, cwd/env, время/source SHA и digests находятся в протоколах.
Файлы stdout/stderr/stdin, названные в commands.json, целиком сохранены как members
[raw-run.tar.gz](verification/raw-run.tar.gz); важные stdout также опубликованы
отдельно выше. Архив не содержит тестовых executable, сертификатов или private keys.
Ключи тестовой PKI создаются в памяти, synthetic auth/gate явно обозначены.

Ранние прогоны не переписаны: [development-runs.json](development-runs.json) и
четыре immutable архива рядом. В01 тест wrong-hash ошибочно пытался запрещённый
UPDATE истории; роль правильно отказала. Исправлена только подготовка нового
synthetic ref через разрешённый INSERT. В02 указан собственный untracked Python
bytecode.03–05 стартовали на чистых версиях; актуальный05 включает расширенные
actual API pagination и SQL cancellation/rollback. Они не выдаются за одну версию.

## Воспроизведение локального стенда

Из корня checkout проверенной версии, Docker с локальным Unix context и точный
Go1.27.2 (образ PG18.7 уже доступен локально):

```sh
GOTOOLCHAIN=local PYTHONDONTWRITEBYTECODE=1 python3 \
  services/processing/scripts/postgres/verify_case_reads.py \
  --output /absolute/new-empty-run \
  --go /absolute/pinned-go1.27.2/bin/go \
  --oas /absolute/exact/mvp-openapi.json
```

Runner не принимает existing DSN/сервер. Он создаёт собственный временный PG18.7,
применяет byte-identical migration и включает Go DB tests только в нём, снимает
actual HTTPS responses, проверяет exact OAS SHA256 и удаляет свой контейнер.
NewServer — минимальная явно внедряемая композиция, а не разрешающий production
entry point: без обязательных authority/boundary/clock/repository/cert/CA/SAN
нет подходящего сервера. Живые IAM/BFF и общий runtime gate пока не подключены.

## Статус восьми frozen критериев

| Критерий | Статус текущей работы |
|---|---|
|1 source SHA/reuse/полная карта полей | PARTIAL: карта/исходники сохранены; order_label/next_action BLOCKED |
|2 три полные реальные OAS проекции | PARTIAL: controlled history/empty list подтверждены; непустые Cases/CaseDetail не готовы |
|3 все required/null/context/observation различия | PARTIAL: History и Error проверены; полный CaseDetail ещё не квалифицирован |
|4 mTLS/session/fresh authority/epoch/start boundary | Проверены независимые локальные части, с явно controlled IAM/gate; production не заявлен |
|5 scoped SQL/RLS/pool reuse/cross-workspace | Проверены на настоящей роли/БД, без bypass |
|6 pagination/filter/readonly | PARTIAL: History HTTPS/SQL и case SQL/codec проверены; непустой Cases API200 BLOCKED |
|7 content faults/expiry/rollback/deadline/DB outage | Проверены независимые read paths; safety не стирается |
|8 все API/PG/content/race/vet и честные границы | PARTIAL: настоящие журналы есть; три полных положительных операции пока не подтверждены |

Полный DoD не объявляется выполненным. Handoff-preflight не запрашивается до
устранения известных обязательных пробелов; Task не переносится агентом в Review,
Blocked или Done. PR остаётся draft. Следующий шаг — решение Lead по
[точечному запросу](LEAD_REQUEST_RU.md), затем реализация подтверждённой проекции,
проверки всех обязательных/nullable полей на успешных трёх операциях, обновлённые
evidence/CI и штатный handoff. Новое правило/producer не считается уже принятым.
