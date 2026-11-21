# HIT-127 · доказательство включения пяти зависимостей

Ответ на единственное замечание [Project OS №15416](https://hitpipe.atlassian.net/browse/HIT-127?focusedCommentId=15416), где независимый проверяющий подтвердил 6/6 DoD. Сборка `4da48202e77131427730b1827df9ee81940e559a`, проверенный предыдущий head Draft PR №82 `4250bb434a9a48ed436904e0a4609717135f7143`, база `d450efc7cfc11f8155a476304aced96dad0ebaf6`. Py/Go проверками зафиксированы точные исходные результаты, Git-объекты и состав двух исполняемых файлов. Ниже приведены читаемые содержимые доказательств без необходимости открывать вложенные ссылки. Проверки выполнены Codex по делегации Tim1hik.

## Задача → проверенная версия → способ включения → проверка в сборке

| Задача | Проверенный SHA из результата | Способ включения | Файл в сборке и фактический тест на 4da4820 |
| --- | --- | --- | --- |
| HIT-111 | `b2e823012854575146595b01a6f56d5ac677fc44` | Прямой предок через HIT-112, оба родителя merge e381972 и базу HIT-126; main.go побайтно совпадает. | `cmd/processing/main.go`, go list для `processing`. `TestProcessingExecutableOwnedStand` PASS. |
| HIT-112 | `0f710a6682d15b437f847376b348947cb4757c7c` | Общий предок обоих родителей merge e381972; worker остаётся в runtime, показана точная добавленная проверка тайм-аута из HIT-114. | `internal/adapters/postgres/caseworker.go`, go list для `processing`. `TestProcessingCaseWorkerOwnedStand` PASS. |
| HIT-113 | `9165e814cb7b46e9cfae484c62ccc70f63fc1ee3` | Второй родитель merge e381972; recheck.go побайтно совпадает, вызов RECHECK сохранён при разрешении конфликтов. | `internal/adapters/postgres/recheck.go`, go list для `processing`. `TestProcessingRecheckOwnedStand` PASS. |
| HIT-114 | `dd05559bc911437d33c0c2582b604d2f2728d362` | Первый родитель merge e381972; timeouts.go побайтно совпадает, вызов тайм-аутов сохранён вместе с RECHECK. | `internal/adapters/postgres/timeouts.go`, go list для `processing`. `TestProcessingCaseTimeoutsOwnedStand` PASS. |
| HIT-126 | `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` | Проверенный код — родитель базового d450efc; purge.go побайтно совпадает, входит в отдельный CLI. | `internal/adapters/postgres/purge.go`, go list для `processing-purge`. `TestProcessingPurgeOwnedStand` PASS. |

## Фактические Git-команды и результаты

~~~text
$ git show -s '--format=%h %P %T %s' 49229e1 3b542e8 dd1a36f 33d2c8d e381972ab7ceb8913e345852c8e073998ec59afe 633f7c3 4c2f1df d450efc7cfc11f8155a476304aced96dad0ebaf6 4da48202e77131427730b1827df9ee81940e559a 4250bb434a9a48ed436904e0a4609717135f7143
exit=0
49229e1 b2e823012854575146595b01a6f56d5ac677fc44 548d356b16e463d41a9716a4563da8afd5a891a3 docs(processing): record HIT-111 executable and authorization checks
3b542e8 0f710a6682d15b437f847376b348947cb4757c7c ed2438898ce44bac99066390d63f5e62e64117d5 docs(processing): record HIT-112 worker verification
dd1a36f 9165e814cb7b46e9cfae484c62ccc70f63fc1ee3 fd16456eb86336f93e5932182cc8eaf18f09ddeb docs(processing): record HIT-113 RECHECK proof and source map
33d2c8d dd05559bc911437d33c0c2582b604d2f2728d362 b67b74f1b19303bbb1d285cb616cfa109a473f2a docs(processing): record HIT-114 timer verification
e381972 33d2c8da52585bcdc2216df9d2033f9089209edb dd1a36f6529f57d7452a27b615b6cad18b1288bd 8e1ca717bb0347807f8201d1e2d98a3b26b8276e Integrate HIT-113 recheck and HIT-114 case timeouts for HIT-126
633f7c3 e381972ab7ceb8913e345852c8e073998ec59afe f6dca2ccd1a828c88a186f7d965c13cde9853ab7 processing: add role-scoped offline retention purge
4c2f1df 633f7c35c4db4707c9dae6576829f54f187887ce 72b6c93583b606c9646f03e04432f9c0c06f6725 Test purge recovery after abrupt exit following outbox hold
d450efc 4c2f1df34f7cf72ddb2de1f6888e033d035913ac 77349d543a99690d56854548d7f0523dab5f02d2 docs(processing): record HIT-126 retention qualification
4da4820 fdda9d1da3f5d0697b0a8811585aa2f8988964e2 6698eb15da95101959f1aacf0ea9e06429cfc699 test(processing): capture exact Health responses and focused fault logs
4250bb4 4da48202e77131427730b1827df9ee81940e559a 43205e77706ee63b982db483f522156a27fcce33 docs(processing): deliver readable evidence for HIT-127 re-review
$ git merge-tree --write-tree --name-only 33d2c8d dd1a36f
exit=1
fda8afd3433cc7effcb8a981e76e82c74d29e1f2
services/processing/internal/adapters/postgres/dispatch.go
services/processing/internal/application/readcases/presentation.go
services/processing/internal/runtime/runtime.go

Auto-merging services/processing/internal/adapters/postgres/dispatch.go
CONFLICT (content): Merge conflict in services/processing/internal/adapters/postgres/dispatch.go
Auto-merging services/processing/internal/adapters/postgres/read_cases.go
Auto-merging services/processing/internal/application/readcases/presentation.go
CONFLICT (content): Merge conflict in services/processing/internal/application/readcases/presentation.go
Auto-merging services/processing/internal/application/readcases/read.go
Auto-merging services/processing/internal/runtime/runtime.go
CONFLICT (content): Merge conflict in services/processing/internal/runtime/runtime.go
~~~

Для каждой цели `d450efc`, `4da4820` и `4250bb4` stdout `git merge-base <исходный или отчётный SHA> <цель>` совпал. На все 30 вызовов получен exit 0. Ниже исходный и отчётный SHA из stdout по каждой задаче.

`HIT-111` — `b2e823012854575146595b01a6f56d5ac677fc44` / `49229e1db3f371a051a022ea9e31951dd558eecb`.
`HIT-112` — `0f710a6682d15b437f847376b348947cb4757c7c` / `3b542e88447f41f8287cc0507814de3f3a330cc6`.
`HIT-113` — `9165e814cb7b46e9cfae484c62ccc70f63fc1ee3` / `dd1a36f6529f57d7452a27b615b6cad18b1288bd`.
`HIT-114` — `dd05559bc911437d33c0c2582b604d2f2728d362` / `33d2c8da52585bcdc2216df9d2033f9089209edb`.
`HIT-126` — `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` / `d450efc7cfc11f8155a476304aced96dad0ebaf6`.

`git diff --exit-code 4da4820 4250bb4 -- . ':(exclude)services/processing/docs/HIT-127/**' ':(exclude).project-os/results/HIT-127.json'` → exit 0, пустой вывод.

| Задача и файл | Git blob на исходном SHA | Git blob на 4da4820 и 4250bb4 |
| --- | --- | --- |
| HIT-111 `cmd/processing/main.go` | `48335af3cd2c24cb372fffbc0392552ba84b56f6` | `48335af3cd2c24cb372fffbc0392552ba84b56f6` |
| HIT-112 `internal/adapters/postgres/caseworker.go` | `b3047429b1d804fa5cb1ef9594cd363cabf25a47` | `541f8d4cdd0e2ff829436ebc8b977a901d3d22d3` |
| HIT-113 `internal/adapters/postgres/recheck.go` | `80ecf221f184607e82683550634c37f15710df78` | `80ecf221f184607e82683550634c37f15710df78` |
| HIT-114 `internal/adapters/postgres/timeouts.go` | `c319df7375f0bb62e36d63c6ea5033dff03e571e` | `c319df7375f0bb62e36d63c6ea5033dff03e571e` |
| HIT-126 `internal/adapters/postgres/purge.go` | `3316baccde796a2a9f93190b8761a7e864c4751a` | `3316baccde796a2a9f93190b8761a7e864c4751a` |

## Что изменилось при объединении

`git show --cc --format= --unified=0 e381972 -- <три конфликтных пути>`, exit 0. Патч показывает сохранённый фильтр состояний отправителя с параметром kind, причины обоих обработчиков и wiring RECHECK с таймерами.

~~~diff
diff --cc services/processing/internal/adapters/postgres/dispatch.go
index 336c582,4bb9ea1..3c2c311
--- a/services/processing/internal/adapters/postgres/dispatch.go
+++ b/services/processing/internal/adapters/postgres/dispatch.go
@@@ -86,6 -92,1 +92,6 @@@ func (d DispatchStore) jobs(ctx context
 -	rows, e := tx.Query(ctx, `SELECT workspace_id::text,job_id::text FROM hp_processing.job_index WHERE ($2='' OR workspace_id::text=$2) AND kind=$3 AND due_at<=clock_timestamp() AND (lease_until IS NULL OR lease_until<=clock_timestamp()) ORDER BY due_at,job_id LIMIT $1`, limit, workspace, kind)
 +	rows, e := tx.Query(ctx, `SELECT j.workspace_id::text,j.job_id::text FROM hp_processing.job_index j
-  WHERE ($2='' OR j.workspace_id::text=$2) AND j.kind='DELIVERY_INTENT' AND j.due_at<=clock_timestamp()
++ WHERE ($2='' OR j.workspace_id::text=$2) AND j.kind=$3 AND j.due_at<=clock_timestamp()
 + AND (j.lease_until IS NULL OR j.lease_until<=clock_timestamp())
 + AND ($2='' OR EXISTS(SELECT 1 FROM hp_processing.outbox o WHERE o.workspace_id=j.workspace_id AND o.message_id=j.job_id
 +   AND o.kind=j.kind AND o.state IN('PENDING','CLAIMED')))
-  ORDER BY j.due_at,j.job_id LIMIT $1`, limit, workspace)
++ ORDER BY j.due_at,j.job_id LIMIT $1`, limit, workspace, kind)
diff --cc services/processing/internal/application/readcases/presentation.go
index 8b2a61c,a7872ba..b1f7445
--- a/services/processing/internal/application/readcases/presentation.go
+++ b/services/processing/internal/application/readcases/presentation.go
@@@ -26,7 -32,1 +34,8 @@@ var reasonCatalog = map[string]reasonIn
+ 	"RECHECK_ACCEPTED":            {"Запрос на повторную проверку принят.", false},
 +	casetimeouts.WorkingElapsed:   {"Истёк срок обработки. Внешний исход требует проверки.", false},
 +	casetimeouts.WaitingElapsed:   {"Проверенные данные не поступили за 24 часа непрерывного ожидания.", false},
 +	casetimeouts.HistoryMissing:   {"Не найдена запись о начале текущего состояния. Действие удержано.", false},
 +	casetimeouts.HistoryInvalid:   {"Данные о начале текущего состояния недоступны или несогласованны. Действие удержано.", false},
 +	casetimeouts.HistoryExpired:   {"Подробности начала текущего состояния просрочены. Действие удержано.", false},
 +	casetimeouts.EpisodeMissing:   {"Не найден открытый эпизод обработки. Действие удержано.", false},
 +	casetimeouts.EpisodeInvalid:   {"Времена или связь эпизода обработки несогласованны. Действие удержано.", false},
diff --cc services/processing/internal/runtime/runtime.go
index fcb1269,60e8b4c..4d7e72f
--- a/services/processing/internal/runtime/runtime.go
+++ b/services/processing/internal/runtime/runtime.go
@@@ -415,1 -430,1 +432,2 @@@ func (rt *Runtime) work(ctx context.Con
+ 			rt.advanceRecheck(ctx, w)
 +			rt.advanceTimeout(ctx, w)
~~~

`git diff 0f710a6 4da4820 -- services/processing/internal/adapters/postgres/caseworker.go`, exit 0. Добавление пришло в `dd05559`. Его Git blob совпадает с `4da4820`.

~~~diff
diff --git a/services/processing/internal/adapters/postgres/caseworker.go b/services/processing/internal/adapters/postgres/caseworker.go
index b304742..541f8d4 100644
--- a/services/processing/internal/adapters/postgres/caseworker.go
+++ b/services/processing/internal/adapters/postgres/caseworker.go
@@ -40,6 +40,14 @@ const eligibleCase = `
    WHERE h.workspace_id=k.workspace_id AND h.case_id=k.case_id
      AND h.old_state IN ('RECEIVED','WAITING_DATA') AND h.new_state='WAITING_DATA'
  ),'-infinity')))
+ AND NOT EXISTS (
+   SELECT 1 FROM hp_processing.safety_history d
+   WHERE d.workspace_id=k.workspace_id AND d.case_id=k.case_id AND d.old_state=k.state AND d.new_state=k.state
+     AND d.reason_code LIKE 'CASE\_TIMER\_%' AND d.at >= COALESCE((
+       SELECT max(h.at) FROM hp_processing.safety_history h
+       WHERE h.workspace_id=k.workspace_id AND h.case_id=k.case_id AND h.old_state IS DISTINCT FROM h.new_state
+     ),'-infinity')
+ )
  AND NOT EXISTS (
    SELECT 1 FROM hp_processing.episodes e WHERE e.workspace_id=k.workspace_id AND e.case_id=k.case_id
    AND (e.completed_at IS NOT NULL OR e.deadline_at<=clock_timestamp())
~~~

## Исполняемые файлы и фактический прогон

В отдельном worktree выполнены `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go list -deps -json ./cmd/processing ./cmd/processing-purge` и сборки `go build -trimpath -buildvcs=false` для обоих CLI на каждом SHA. Инструмент Go 1.27.2. Результаты `GoFiles` для пяти файлов из таблицы приведены ниже. Тест сравнения двух ревизий использует одинаковые флаги, а архивы фактического предыдущего запуска сохраняют собственные SHA бинарников.

~~~text
4da48202e77131427730b1827df9ee81940e559a:services/processing/cmd/processing-purge/main.go:75:	result, err := (postgres.PurgeStore{DB: conn, Files: files}).Run(ctx, workspace, limit)
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:261:	rt.queries.Repository = jobStore{Repository: postgres.RecheckDispatch{DispatchStore: jobs}, a: q}
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:266:	rt.caseJobs = postgres.CaseJobStore{DB: worker}
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:267:	rt.timeouts = postgres.CaseTimeoutStore{DB: worker, Files: rt.files, History: rt.files, Clock: serviceClock}
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:437:			rt.advanceCase(ctx, w)
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:438:			rt.advanceRecheck(ctx, w)
4da48202e77131427730b1827df9ee81940e559a:services/processing/internal/runtime/runtime.go:439:			rt.advanceTimeout(ctx, w)
~~~

~~~json
{
  "witness_membership": {
    "HIT-111": {
      "binary": "processing",
      "package": "github.com/hitpipe/hitpipe/services/processing/cmd/processing",
      "go_file": "main.go"
    },
    "HIT-112": {
      "binary": "processing",
      "package": "github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres",
      "go_file": "caseworker.go"
    },
    "HIT-113": {
      "binary": "processing",
      "package": "github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres",
      "go_file": "recheck.go"
    },
    "HIT-114": {
      "binary": "processing",
      "package": "github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres",
      "go_file": "timeouts.go"
    },
    "HIT-126": {
      "binary": "processing-purge",
      "package": "github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres",
      "go_file": "purge.go"
    }
  },
  "rebuild_sha256_both_revisions": {
    "processing": "b57b182cd14ae83bd29b68055ff2ced88469082f75804b165cb8bf3892cf3a4c",
    "processing-purge": "66ae28d9617a3af431f1d6fc51c1bde4c8c76d387655751dbb9a09ed0abb9d23"
  },
  "original_executables_sha256": {
    "processing": "e56ed981b42e04b9153bef1db44f80eb99fd8ae421a4e333769016a9e77cc884",
    "processing-purge": "d795eca6ccc19aced7b2d17660df2ff1324480c871159d65696685216cf9a4d7"
  }
}
~~~

`compile-purge-linux`, exit 0, source `4da48202e77131427730b1827df9ee81940e559a`, stdout SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

`compile-runtime-linux`, exit 0, source `4da48202e77131427730b1827df9ee81940e559a`, stdout SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

~~~text
env GOOS=linux GOARCH=arm64 CGO_ENABLED=0 /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/go/bin/go -C /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit127-observability/services/processing build -ldflags '-X main.revision=4da48202e77131427730b1827df9ee81940e559a' -o /var/folders/d_/k8xghq856tsg8xzsvgtd_3tm0000gn/T/hit78-test-1_i0vt6w/processing ./cmd/processing
~~~

`compile-linux-test`, exit 0, source `4da48202e77131427730b1827df9ee81940e559a`, stdout SHA-256 `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`.

`postgres-storage-tests`, exit 0, source `4da48202e77131427730b1827df9ee81940e559a`, stdout SHA-256 `30b972801b8d2fa14574766845b46dc6d1ffd274bf9b3b2b2130d7061d2621a6`.

~~~text
docker exec -e HIT100_CAPTURE_FILE=/tmp/hit100-captures.jsonl -e HIT103_CAPTURE_FILE=/tmp/hit103-captures.jsonl -e HIT102_CAPTURE_FILE=/tmp/hit102-captures.jsonl -e HIT111_PROCESSING_BIN=/test/processing -e HIT126_PURGE_BIN=/test/processing-purge -e HIT78_ISOLATED_PG=1 6a262505522eec538c90b0917460c5cd375521360801181954384bccf2b2947a /test/storage.test -test.v -test.count=1 -test.timeout=120s
~~~

~~~text
--- PASS: TestProcessingExecutableOwnedStand (3.18s)
--- PASS: TestProcessingCaseWorkerOwnedStand (16.36s)
--- PASS: TestProcessingRecheckOwnedStand (22.88s)
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.44s)
--- PASS: TestProcessingPurgeOwnedStand (0.12s)
~~~

## Полные исходные результаты пяти задач

Читались через `git show <SHA результата>:.project-os/results/<задача>.json`. GitHub Contents для каждого неизменяемого пути вернул те же байты. На базе, tested SHA и head получены те же байты.

### HIT-111

Коммит результата `49229e1db3f371a051a022ea9e31951dd558eecb`, Git blob `0419a5be287bddebfc5a78fbc073169aaa7c8719`, SHA-256 `bf726f69fe7276dc0adbb73f82bc2ad2dafe8d3ca9d523e723807dadaef9f4ab`. [Исходный JSON](https://github.com/hitpipe/hitpipe/blob/49229e1db3f371a051a022ea9e31951dd558eecb/.project-os/results/HIT-111.json)

~~~json
{"subject":"HIT-111","authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","materialized_result":{"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","summary":"Единый исполняемый сервис Processing с собственными PostgreSQL 18.7 и файлами, строгим TLS 1.3, единственным HIT-69 startgate, прежними HTTP slice, отправщиком и остановкой.","contract_revision":1,"contract_fingerprint":"ba730271dfb71bf71adde963e5300305132eb47a868e671bafdcb68a065c7c84","base_sha":"1bbb3567c042a1ac79f21d69f78d71e8a4fef41e","tested_code_sha":"b2e823012854575146595b01a6f56d5ac677fc44","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/72"},"changed_artifacts":["services/processing/cmd/processing/main.go","services/processing/docs/HIT-111/RUN_RU.md","services/processing/docs/HIT-111/processing.example.json","services/processing/go.mod","services/processing/internal/adapters/draftjson/schemas.json","services/processing/internal/adapters/postgres/lifecycle_test.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/application/drafts/wire_generated.go","services/processing/internal/runtime/access.go","services/processing/internal/runtime/config.go","services/processing/internal/runtime/iam.go","services/processing/internal/runtime/queue.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/source.go","services/processing/internal/testfixtures/draft.go","services/processing/scripts/postgres/verify_runtime.py","services/processing/tools/drafts/generate.py","services/processing/docs/HIT-111/",".project-os/results/HIT-111.json"],"verification":{"report":"services/processing/docs/HIT-111/README.md","runbook":"services/processing/docs/HIT-111/RUN_RU.md","configuration_template":"services/processing/docs/HIT-111/processing.example.json","source_audit":"services/processing/docs/HIT-111/source-audit.json","test_protocol":"services/processing/docs/HIT-111/verification/test-evidence.json","original_process_log":"services/processing/docs/HIT-111/verification/postgres-storage-tests.stdout.gz","ci_protocol":"services/processing/docs/HIT-111/verification/ci-evidence.json","actual_stand":"PASS; 98 command records including expected negative SQL checks. Separate clean Go race/vet/build and real Linux arm64 process/HTTP/mTLS/PG18.7/files.","code_ci":"https://github.com/hitpipe/hitpipe/actions/runs/44392292963"},"deviations":[],"known_limitations":["Внешние участники в проверенном запуске управляемые. Обычный запуск остаётся CLOSED без авторитетных источников CSRF/привязок/очереди/roster.","SourceFacts подключён как готовая фабрика по принятому маршруту; автоматическое финансовое продвижение нового дела и RECHECK относятся к последующим задачам.","Реальные IAM/BFF/Stripe/ClickUp и распределённый fencing не квалифицированы; работающие среды и секреты не использовались."]}
~~~

### HIT-112

Коммит результата `3b542e88447f41f8287cc0507814de3f3a330cc6`, Git blob `837e45140b7670bc9da0804ce1aac82051007e10`, SHA-256 `0f2d2ad85cb28e57ef65220d8a7edcc33978e73da6cc5cded34a8b4c57713985`. [Исходный JSON](https://github.com/hitpipe/hitpipe/blob/3b542e88447f41f8287cc0507814de3f3a330cc6/.project-os/results/HIT-112.json)

~~~json
{"subject":"HIT-112","authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","materialized_result":{"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","summary":"Processing восстанавливает допущенные дела, получает свежие факты и атомарно готовит единственную доставку. Настоящий mTLS и PostgreSQL 18.7 проверены на собственном стенде.","contract_revision":1,"contract_fingerprint":"9449a85361fc49867f1b04b3c155dc56d776acec1b2ed7476789800a302a65b3","base_sha":"49229e1db3f371a051a022ea9e31951dd558eecb","tested_code_sha":"0f710a6682d15b437f847376b348947cb4757c7c","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/73"},"changed_artifacts":["services/processing/internal/adapters/postgres/caseworker.go","services/processing/internal/adapters/postgres/caseworker_process_test.go","services/processing/internal/adapters/postgres/caseworker_test.go","services/processing/internal/adapters/postgres/prepare.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/application/evaluatepaidorder/evaluate.go","services/processing/internal/application/preparecase/prepare.go","services/processing/internal/runtime/caseworker.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/source.go","services/processing/docs/HIT-112/",".project-os/results/HIT-112.json"],"verification":{"report":"services/processing/docs/HIT-112/README.md","source_audit":"services/processing/docs/HIT-112/source-audit.json","test_protocol":"services/processing/docs/HIT-112/verification/test-evidence.json","ci_protocol":"services/processing/docs/HIT-112/verification/ci-evidence.json","original_postgres_output":"services/processing/docs/HIT-112/verification/postgres-storage-tests.stdout.gz","actual_stand":"PASS. TestProcessingCaseWorkerOwnedStand and storage regressions verified on owned PostgreSQL18.7, process and HTTPS/mTLS.","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44414140750","https://github.com/hitpipe/hitpipe/actions/runs/44414129541"]},"deviations":[],"known_limitations":["Управляемые IAM, Intake и Delivery на локальном стенде. Live IAM/BFF/Stripe/ClickUp и распределённый fencing не квалифицированы.","В обычном запуске HIT-111 без авторитетных входов сохраняется CLOSED.","Бизнес-таймеры возраста и ручной RECHECK оставлены HIT-114 и HIT-113."]}
~~~

### HIT-113

Коммит результата `dd1a36f6529f57d7452a27b615b6cad18b1288bd`, Git blob `ae3939ee23c564c451589310bd4f40a1cce0779f`, SHA-256 `d9564c32f46e8568ef2172c7cb668c36d4e5aba4b5a1517e91c75ec889593ff1`. [Исходный JSON](https://github.com/hitpipe/hitpipe/blob/dd1a36f6529f57d7452a27b615b6cad18b1288bd/.project-os/results/HIT-113.json)

~~~json
{"subject":"HIT-113","authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","materialized_result":{"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","summary":"Processing принимает RECHECK сохранённой task identity и доставляет типизированный read-only запрос. Валидный TaskObservation применяет существующий HIT-99. Квалифицированы настоящий PostgreSQL 18.7, процесс и mTLS.","contract_revision":1,"contract_fingerprint":"a86588178d202552f2f9f9bb2c7f9d75fa6c28c2990d0d4e6ccb496e989dcfd1","base_sha":"3b542e88447f41f8287cc0507814de3f3a330cc6","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","tested_code_sha":"9165e814cb7b46e9cfae484c62ccc70f63fc1ee3","pull_request":"https://github.com/hitpipe/hitpipe/pull/76"},"changed_artifacts":["services/processing/internal/adapters/contentfiles/store.go","services/processing/internal/adapters/deliveryhttp/query.go","services/processing/internal/adapters/deliveryhttp/send.go","services/processing/internal/adapters/draftjson/schemas.json","services/processing/internal/adapters/postgres/dispatch.go","services/processing/internal/adapters/postgres/observations.go","services/processing/internal/adapters/postgres/read_cases.go","services/processing/internal/adapters/postgres/recheck.go","services/processing/internal/adapters/postgres/recheck_dispatch.go","services/processing/internal/adapters/postgres/recheck_process_test.go","services/processing/internal/adapters/postgres/recheck_test.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/adapters/recheckhttp/http.go","services/processing/internal/application/dispatchintent/dispatch.go","services/processing/internal/application/drafts/wire_generated.go","services/processing/internal/application/readcases/presentation.go","services/processing/internal/application/readcases/read.go","services/processing/internal/application/recheck/recheck.go","services/processing/internal/runtime/access.go","services/processing/internal/runtime/recheck.go","services/processing/internal/runtime/runtime.go","services/processing/tools/drafts/generate.py","services/processing/docs/HIT-113/",".project-os/results/HIT-113.json"],"verification":{"report":"services/processing/docs/HIT-113/README.md","field_map":"services/processing/docs/HIT-113/field-map.md","source_audit":"services/processing/docs/HIT-113/source-audit.json","test_protocol":"services/processing/docs/HIT-113/verification/test-evidence.json","ci_protocol":"services/processing/docs/HIT-113/verification/ci-evidence.json","original_postgres_output":"services/processing/docs/HIT-113/verification/postgres-storage-tests.stdout.gz","actual_stand":"PASS. TestProcessingRecheckOwnedStand plus rollback/replay/concurrency and predecessor regressions on owned PG18.7, mTLS and Linux executable.","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44501255421","https://github.com/hitpipe/hitpipe/actions/runs/44501245262"]},"deviations":[],"known_limitations":["Управляемые IAM, Intake, Delivery на стенде. Живой ClickUp, credentials и распределённый барьер не квалифицированы.","Обычный режим HIT-111 требует авторитетных источников доступа и привязок."]}
~~~

### HIT-114

Коммит результата `33d2c8da52585bcdc2216df9d2033f9089209edb`, Git blob `1508011ebfccadd5f6dbc0857178bc7ec3c9d187`, SHA-256 `1689257e471f9787deb05f97899dc8a17c453cddc5f7a18b408163ce5095a780`. [Исходный JSON](https://github.com/hitpipe/hitpipe/blob/33d2c8da52585bcdc2216df9d2033f9089209edb/.project-os/results/HIT-114.json)

~~~json
{"subject":"HIT-114","authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","materialized_result":{"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","summary":"Таймеры Processing применяют сохранённый срок WORKING и длительность непрерывного WAITING_DATA, защищают установленное знание и показывают причину через существующие чтения.","contract_revision":1,"contract_fingerprint":"0f7bdb4560cb1fde9b26d8bd6b7a2a385658a772b07b448ad5b7162a610ade52","base_sha":"3b542e88447f41f8287cc0507814de3f3a330cc6","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","tested_code_sha":"dd05559bc911437d33c0c2582b604d2f2728d362","pull_request":"https://github.com/hitpipe/hitpipe/pull/78"},"changed_artifacts":["services/processing/internal/adapters/casereadhttp/http.go","services/processing/internal/adapters/postgres/caseworker.go","services/processing/internal/adapters/postgres/dispatch.go","services/processing/internal/adapters/postgres/prepare_test.go","services/processing/internal/adapters/postgres/read_cases.go","services/processing/internal/adapters/postgres/timeouts.go","services/processing/internal/adapters/postgres/timeouts_process_test.go","services/processing/internal/adapters/postgres/timeouts_test.go","services/processing/internal/application/casetimeouts/timeout.go","services/processing/internal/application/readcases/presentation.go","services/processing/internal/application/readcases/read.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/timeouts.go","services/processing/docs/HIT-114/",".project-os/results/HIT-114.json"],"verification":{"report":"services/processing/docs/HIT-114/README.md","timing_map":"services/processing/docs/HIT-114/timing-map.md","source_audit":"services/processing/docs/HIT-114/source-audit.json","test_protocol":"services/processing/docs/HIT-114/verification/test-evidence.json","ci_protocol":"services/processing/docs/HIT-114/verification/ci-evidence.json","original_postgres_output":"services/processing/docs/HIT-114/verification/postgres-storage-tests.stdout.gz","actual_stand":"PASS. TestProcessingCaseTimeoutsOwnedStand plus exact deadline, concurrency, lease and predecessor regressions on owned PG18.7, mTLS and Linux executable.","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44597605354","https://github.com/hitpipe/hitpipe/actions/runs/44597582040"]},"deviations":[],"known_limitations":["Управляемые IAM, Intake, Delivery на стенде. Проверка локальных правил по делу, эксплуатационные SLO и живые провайдеры вне стенда.","Обычный режим HIT-111 требует авторитетных источников доступа и привязок."]}
~~~

### HIT-126

Коммит результата `d450efc7cfc11f8155a476304aced96dad0ebaf6`, Git blob `a816950a26e4dbb007adb994e35997c38d786be2`, SHA-256 `982b609dbfd21e6e530f0c5a27d507d23e778461e4559f422be48d530139813e`. [Исходный JSON](https://github.com/hitpipe/hitpipe/blob/d450efc7cfc11f8155a476304aced96dad0ebaf6/.project-os/results/HIT-126.json)

~~~json
{"subject":"HIT-126","authorship":{"human_owner":"Тимофей Патрушев / Tim1hik","executor":"Codex по делегации владельца"},"materialized_result":{"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_POSTGRES_AND_CONTENT_STAND","summary":"Пакетная физическая очистка истёкшего содержимого Processing через отдельно разрешённую роль, с повторной проверкой refs и сохранением safety. Истёкшая доставка удерживается.","contract_revision":1,"contract_fingerprint":"7bed9e9a200f0705cf7b9f2e8400de10e4fdc0fa44bb39ced6b8f49ac958564a","base_sha":"e381972ab7ceb8913e345852c8e073998ec59afe","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","tested_code_sha":"4c2f1df34f7cf72ddb2de1f6888e033d035913ac","pull_request":"https://github.com/hitpipe/hitpipe/pull/81"},"changed_artifacts":["services/processing/cmd/processing-purge/main.go","services/processing/internal/adapters/contentfiles/draft.go","services/processing/internal/adapters/contentfiles/maintenance.go","services/processing/internal/adapters/contentfiles/maintenance_test.go","services/processing/internal/adapters/contentfiles/read_owned.go","services/processing/internal/adapters/contentfiles/read_owned_test.go","services/processing/internal/adapters/contentfiles/store.go","services/processing/internal/adapters/contentfiles/store_test.go","services/processing/internal/adapters/postgres/activation_test.go","services/processing/internal/adapters/postgres/drafts_security_test.go","services/processing/internal/adapters/postgres/expiry_fixture_test.go","services/processing/internal/adapters/postgres/history_api_revision3_test.go","services/processing/internal/adapters/postgres/history_revision3_test.go","services/processing/internal/adapters/postgres/intake_faults_test.go","services/processing/internal/adapters/postgres/observation_faults_test.go","services/processing/internal/adapters/postgres/projection_revision3_test.go","services/processing/internal/adapters/postgres/purge.go","services/processing/internal/adapters/postgres/purge_test.go","services/processing/internal/adapters/postgres/read_cases_test.go","services/processing/scripts/postgres/verify_purge.py","services/processing/docs/HIT-126",".project-os/results/HIT-126.json"],"deviations":[],"verification":{"actual_stand":"docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f","source_audit":"services/processing/docs/HIT-126/source-audit.json","report":"services/processing/docs/HIT-126/README.md","runbook":"services/processing/docs/HIT-126/RUN_RU.md","retention_map":"services/processing/docs/HIT-126/retention-map.md","test_protocol":"services/processing/docs/HIT-126/verification/test-evidence.json","ci_protocol":"services/processing/docs/HIT-126/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44744088636","https://github.com/hitpipe/hitpipe/actions/runs/44744081639"]},"known_limitations":["Очистка проверена на локальном одноразовом стенде с одной выделенной базой и каталогом. Рабочие данные не использовались.","Точные версии HIT-111, HIT-113 и HIT-114 входят в изолированную проверенную базу; их ветки не слиты в main.","Неверифицируемые файлы остаются для разбирательства. Для прежних процессов без файловой блокировки требуется остановка перед очисткой."]}
~~~
