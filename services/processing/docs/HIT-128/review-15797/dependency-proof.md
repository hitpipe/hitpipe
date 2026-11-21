# HIT-128. Проверяемая цепочка всех шести зависимостей

Ответ на №15797: 6/6 DoD уже подтверждены. Проверенные цели: `c426dc400a4bec03fafba72e2a5780c9d85af1e2` и `c394de8abb8d6ed4f5324f45a4b023c3749f1a5f`, база PR №85 `19831026d635dd74d4b54dd4acd93e53319c182e`. Изменены только материалы ревью.

| Задача | Включение в Git | Ревью Jira |
| --- | --- | --- |
| HIT-111 | `b2e8230 → 49229e1 → HIT-112`, main/config/IAM | №14258, PASS WITH NOTES, 8/8 |
| HIT-112 | `0f710a6 → 3b542e8 → оба родителя merge`, caseworker | №14400, PASS WITH NOTES, 8/8 |
| HIT-113 | `9165e81 → dd1a36f → второй родитель merge`, RECHECK | №14534, PASS, 8/8 |
| HIT-114 | `dd05559 → 33d2c8d → первый родитель merge`, timeouts | №14726, PASS, 8/8 |
| HIT-126 | `4c2f1df → d450efc → HIT-127`, processing-purge | №15121, PASS, 6/6 |
| HIT-127 | `4da4820 → 4250bb4 → 1983102 → HIT-128`, diagnostics | №15637, PASS WITH NOTES, 6/6 |

## Зарегистрированные исходные результаты

Полные JSON из Git совпали с материализованными результатами и официальными Jira review packets. Для HIT-111 проверен блок №14255. `git rev-parse <ref>:.project-os/results/<key>.json` дал тот же blob на SHA регистрации и обеих целях.


### HIT-111

Результат `49229e1db3f371a051a022ea9e31951dd558eecb`, blob `0419a5be287bddebfc5a78fbc073169aaa7c8719`, отпечаток `018b9da4634ef3c3d3603add4044f58389a515259d272fa4b7545cc3ab065f5b`.

~~~~json
{"authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","changed_artifacts":["services/processing/cmd/processing/main.go","services/processing/docs/HIT-111/RUN_RU.md","services/processing/docs/HIT-111/processing.example.json","services/processing/go.mod","services/processing/internal/adapters/draftjson/schemas.json","services/processing/internal/adapters/postgres/lifecycle_test.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/application/drafts/wire_generated.go","services/processing/internal/runtime/access.go","services/processing/internal/runtime/config.go","services/processing/internal/runtime/iam.go","services/processing/internal/runtime/queue.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/source.go","services/processing/internal/testfixtures/draft.go","services/processing/scripts/postgres/verify_runtime.py","services/processing/tools/drafts/generate.py","services/processing/docs/HIT-111/",".project-os/results/HIT-111.json"],"deviations":[],"known_limitations":["Внешние участники в проверенном запуске управляемые. Обычный запуск остаётся CLOSED без авторитетных источников CSRF/привязок/очереди/roster.","SourceFacts подключён как готовая фабрика по принятому маршруту; автоматическое финансовое продвижение нового дела и RECHECK относятся к последующим задачам.","Реальные IAM/BFF/Stripe/ClickUp и распределённый fencing не квалифицированы; работающие среды и секреты не использовались."],"materialized_result":{"base_sha":"1bbb3567c042a1ac79f21d69f78d71e8a4fef41e","contract_fingerprint":"ba730271dfb71bf71adde963e5300305132eb47a868e671bafdcb68a065c7c84","contract_revision":1,"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/72","summary":"Единый исполняемый сервис Processing с собственными PostgreSQL 18.7 и файлами, строгим TLS 1.3, единственным HIT-69 startgate, прежними HTTP slice, отправщиком и остановкой.","tested_code_sha":"b2e823012854575146595b01a6f56d5ac677fc44"},"subject":"HIT-111","verification":{"actual_stand":"PASS; 98 command records including expected negative SQL checks. Separate clean Go race/vet/build and real Linux arm64 process/HTTP/mTLS/PG18.7/files.","ci_protocol":"services/processing/docs/HIT-111/verification/ci-evidence.json","code_ci":"https://github.com/hitpipe/hitpipe/actions/runs/44392292963","configuration_template":"services/processing/docs/HIT-111/processing.example.json","original_process_log":"services/processing/docs/HIT-111/verification/postgres-storage-tests.stdout.gz","report":"services/processing/docs/HIT-111/README.md","runbook":"services/processing/docs/HIT-111/RUN_RU.md","source_audit":"services/processing/docs/HIT-111/source-audit.json","test_protocol":"services/processing/docs/HIT-111/verification/test-evidence.json"}}
~~~~

### HIT-112

Результат `3b542e88447f41f8287cc0507814de3f3a330cc6`, blob `837e45140b7670bc9da0804ce1aac82051007e10`, отпечаток `f5597c9da8be42b727592849717b50ed2c29793e1917cb2534fb4f0d04ceff6b`.

~~~~json
{"authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","changed_artifacts":["services/processing/internal/adapters/postgres/caseworker.go","services/processing/internal/adapters/postgres/caseworker_process_test.go","services/processing/internal/adapters/postgres/caseworker_test.go","services/processing/internal/adapters/postgres/prepare.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/application/evaluatepaidorder/evaluate.go","services/processing/internal/application/preparecase/prepare.go","services/processing/internal/runtime/caseworker.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/source.go","services/processing/docs/HIT-112/",".project-os/results/HIT-112.json"],"deviations":[],"known_limitations":["Управляемые IAM, Intake и Delivery на локальном стенде. Live IAM/BFF/Stripe/ClickUp и распределённый fencing не квалифицированы.","В обычном запуске HIT-111 без авторитетных входов сохраняется CLOSED.","Бизнес-таймеры возраста и ручной RECHECK оставлены HIT-114 и HIT-113."],"materialized_result":{"base_sha":"49229e1db3f371a051a022ea9e31951dd558eecb","contract_fingerprint":"9449a85361fc49867f1b04b3c155dc56d776acec1b2ed7476789800a302a65b3","contract_revision":1,"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/73","summary":"Processing восстанавливает допущенные дела, получает свежие факты и атомарно готовит единственную доставку. Настоящий mTLS и PostgreSQL 18.7 проверены на собственном стенде.","tested_code_sha":"0f710a6682d15b437f847376b348947cb4757c7c"},"subject":"HIT-112","verification":{"actual_stand":"PASS. TestProcessingCaseWorkerOwnedStand and storage regressions verified on owned PostgreSQL18.7, process and HTTPS/mTLS.","ci_protocol":"services/processing/docs/HIT-112/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44414140750","https://github.com/hitpipe/hitpipe/actions/runs/44414129541"],"original_postgres_output":"services/processing/docs/HIT-112/verification/postgres-storage-tests.stdout.gz","report":"services/processing/docs/HIT-112/README.md","source_audit":"services/processing/docs/HIT-112/source-audit.json","test_protocol":"services/processing/docs/HIT-112/verification/test-evidence.json"}}
~~~~

### HIT-113

Результат `dd1a36f6529f57d7452a27b615b6cad18b1288bd`, blob `ae3939ee23c564c451589310bd4f40a1cce0779f`, отпечаток `6998afbcaabc192c6947e86f15b57175c9723e6093d607e982d7c43a1b27e881`.

~~~~json
{"authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","changed_artifacts":["services/processing/internal/adapters/contentfiles/store.go","services/processing/internal/adapters/deliveryhttp/query.go","services/processing/internal/adapters/deliveryhttp/send.go","services/processing/internal/adapters/draftjson/schemas.json","services/processing/internal/adapters/postgres/dispatch.go","services/processing/internal/adapters/postgres/observations.go","services/processing/internal/adapters/postgres/read_cases.go","services/processing/internal/adapters/postgres/recheck.go","services/processing/internal/adapters/postgres/recheck_dispatch.go","services/processing/internal/adapters/postgres/recheck_process_test.go","services/processing/internal/adapters/postgres/recheck_test.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/adapters/recheckhttp/http.go","services/processing/internal/application/dispatchintent/dispatch.go","services/processing/internal/application/drafts/wire_generated.go","services/processing/internal/application/readcases/presentation.go","services/processing/internal/application/readcases/read.go","services/processing/internal/application/recheck/recheck.go","services/processing/internal/runtime/access.go","services/processing/internal/runtime/recheck.go","services/processing/internal/runtime/runtime.go","services/processing/tools/drafts/generate.py","services/processing/docs/HIT-113/",".project-os/results/HIT-113.json"],"deviations":[],"known_limitations":["Управляемые IAM, Intake, Delivery на стенде. Живой ClickUp, credentials и распределённый барьер не квалифицированы.","Обычный режим HIT-111 требует авторитетных источников доступа и привязок."],"materialized_result":{"base_sha":"3b542e88447f41f8287cc0507814de3f3a330cc6","contract_fingerprint":"a86588178d202552f2f9f9bb2c7f9d75fa6c28c2990d0d4e6ccb496e989dcfd1","contract_revision":1,"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/76","summary":"Processing принимает RECHECK сохранённой task identity и доставляет типизированный read-only запрос. Валидный TaskObservation применяет существующий HIT-99. Квалифицированы настоящий PostgreSQL 18.7, процесс и mTLS.","tested_code_sha":"9165e814cb7b46e9cfae484c62ccc70f63fc1ee3"},"subject":"HIT-113","verification":{"actual_stand":"PASS. TestProcessingRecheckOwnedStand plus rollback/replay/concurrency and predecessor regressions on owned PG18.7, mTLS and Linux executable.","ci_protocol":"services/processing/docs/HIT-113/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44501255421","https://github.com/hitpipe/hitpipe/actions/runs/44501245262"],"field_map":"services/processing/docs/HIT-113/field-map.md","original_postgres_output":"services/processing/docs/HIT-113/verification/postgres-storage-tests.stdout.gz","report":"services/processing/docs/HIT-113/README.md","source_audit":"services/processing/docs/HIT-113/source-audit.json","test_protocol":"services/processing/docs/HIT-113/verification/test-evidence.json"}}
~~~~

### HIT-114

Результат `33d2c8da52585bcdc2216df9d2033f9089209edb`, blob `1508011ebfccadd5f6dbc0857178bc7ec3c9d187`, отпечаток `a01ad3417f64161f797620a2857cc160b741a41bb43d365d3dc39c93a25d5e05`.

~~~~json
{"authorship":"Codex по делегации Тимофея Патрушева / Tim1hik","changed_artifacts":["services/processing/internal/adapters/casereadhttp/http.go","services/processing/internal/adapters/postgres/caseworker.go","services/processing/internal/adapters/postgres/dispatch.go","services/processing/internal/adapters/postgres/prepare_test.go","services/processing/internal/adapters/postgres/read_cases.go","services/processing/internal/adapters/postgres/timeouts.go","services/processing/internal/adapters/postgres/timeouts_process_test.go","services/processing/internal/adapters/postgres/timeouts_test.go","services/processing/internal/application/casetimeouts/timeout.go","services/processing/internal/application/readcases/presentation.go","services/processing/internal/application/readcases/read.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/timeouts.go","services/processing/docs/HIT-114/",".project-os/results/HIT-114.json"],"deviations":[],"known_limitations":["Управляемые IAM, Intake, Delivery на стенде. Проверка локальных правил по делу, эксплуатационные SLO и живые провайдеры вне стенда.","Обычный режим HIT-111 требует авторитетных источников доступа и привязок."],"materialized_result":{"base_sha":"3b542e88447f41f8287cc0507814de3f3a330cc6","contract_fingerprint":"0f7bdb4560cb1fde9b26d8bd6b7a2a385658a772b07b448ad5b7162a610ade52","contract_revision":1,"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/78","summary":"Таймеры Processing применяют сохранённый срок WORKING и длительность непрерывного WAITING_DATA, защищают установленное знание и показывают причину через существующие чтения.","tested_code_sha":"dd05559bc911437d33c0c2582b604d2f2728d362"},"subject":"HIT-114","verification":{"actual_stand":"PASS. TestProcessingCaseTimeoutsOwnedStand plus exact deadline, concurrency, lease and predecessor regressions on owned PG18.7, mTLS and Linux executable.","ci_protocol":"services/processing/docs/HIT-114/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44597605354","https://github.com/hitpipe/hitpipe/actions/runs/44597582040"],"original_postgres_output":"services/processing/docs/HIT-114/verification/postgres-storage-tests.stdout.gz","report":"services/processing/docs/HIT-114/README.md","source_audit":"services/processing/docs/HIT-114/source-audit.json","test_protocol":"services/processing/docs/HIT-114/verification/test-evidence.json","timing_map":"services/processing/docs/HIT-114/timing-map.md"}}
~~~~

### HIT-126

Результат `d450efc7cfc11f8155a476304aced96dad0ebaf6`, blob `a816950a26e4dbb007adb994e35997c38d786be2`, отпечаток `47e3caad04fe96e000881af06628111a7a242d68d95619731b129a890e785cf4`.

~~~~json
{"authorship":{"executor":"Codex по делегации владельца","human_owner":"Тимофей Патрушев / Tim1hik"},"changed_artifacts":["services/processing/cmd/processing-purge/main.go","services/processing/internal/adapters/contentfiles/draft.go","services/processing/internal/adapters/contentfiles/maintenance.go","services/processing/internal/adapters/contentfiles/maintenance_test.go","services/processing/internal/adapters/contentfiles/read_owned.go","services/processing/internal/adapters/contentfiles/read_owned_test.go","services/processing/internal/adapters/contentfiles/store.go","services/processing/internal/adapters/contentfiles/store_test.go","services/processing/internal/adapters/postgres/activation_test.go","services/processing/internal/adapters/postgres/drafts_security_test.go","services/processing/internal/adapters/postgres/expiry_fixture_test.go","services/processing/internal/adapters/postgres/history_api_revision3_test.go","services/processing/internal/adapters/postgres/history_revision3_test.go","services/processing/internal/adapters/postgres/intake_faults_test.go","services/processing/internal/adapters/postgres/observation_faults_test.go","services/processing/internal/adapters/postgres/projection_revision3_test.go","services/processing/internal/adapters/postgres/purge.go","services/processing/internal/adapters/postgres/purge_test.go","services/processing/internal/adapters/postgres/read_cases_test.go","services/processing/scripts/postgres/verify_purge.py","services/processing/docs/HIT-126",".project-os/results/HIT-126.json"],"deviations":[],"known_limitations":["Очистка проверена на локальном одноразовом стенде с одной выделенной базой и каталогом. Рабочие данные не использовались.","Точные версии HIT-111, HIT-113 и HIT-114 входят в изолированную проверенную базу; их ветки не слиты в main.","Неверифицируемые файлы остаются для разбирательства. Для прежних процессов без файловой блокировки требуется остановка перед очисткой."],"materialized_result":{"base_sha":"e381972ab7ceb8913e345852c8e073998ec59afe","contract_fingerprint":"7bed9e9a200f0705cf7b9f2e8400de10e4fdc0fa44bb39ced6b8f49ac958564a","contract_revision":1,"implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_POSTGRES_AND_CONTENT_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/81","summary":"Пакетная физическая очистка истёкшего содержимого Processing через отдельно разрешённую роль, с повторной проверкой refs и сохранением safety. Истёкшая доставка удерживается.","tested_code_sha":"4c2f1df34f7cf72ddb2de1f6888e033d035913ac"},"subject":"HIT-126","verification":{"actual_stand":"docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f","ci_protocol":"services/processing/docs/HIT-126/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44744088636","https://github.com/hitpipe/hitpipe/actions/runs/44744081639"],"report":"services/processing/docs/HIT-126/README.md","retention_map":"services/processing/docs/HIT-126/retention-map.md","runbook":"services/processing/docs/HIT-126/RUN_RU.md","source_audit":"services/processing/docs/HIT-126/source-audit.json","test_protocol":"services/processing/docs/HIT-126/verification/test-evidence.json"}}
~~~~

### HIT-127

Результат `19831026d635dd74d4b54dd4acd93e53319c182e`, blob `6fa27bf90469407ab130eed6fc6cf6b4d8eff8bd`, отпечаток `923aa2e441792f9597bdca667f5099fc139f528be9119f1be473cf1713f39bbe`.

~~~~json
{"authorship":{"executor":"Codex по делегации владельца","human_owner":"Тимофей Патрушев / Tim1hik"},"changed_artifacts":["services/processing/cmd/processing-purge/main.go","services/processing/internal/adapters/contentfiles/store.go","services/processing/internal/adapters/postgres/diagnostics.go","services/processing/internal/adapters/postgres/diagnostics_test.go","services/processing/internal/adapters/postgres/dispatch.go","services/processing/internal/adapters/postgres/observation_faults_test.go","services/processing/internal/adapters/postgres/observations.go","services/processing/internal/adapters/postgres/observations_test.go","services/processing/internal/adapters/postgres/prepare.go","services/processing/internal/adapters/postgres/runtime_process_test.go","services/processing/internal/runtime/caseworker.go","services/processing/internal/runtime/diagnostics.go","services/processing/internal/runtime/diagnostics_test.go","services/processing/internal/runtime/recheck.go","services/processing/internal/runtime/runtime.go","services/processing/internal/runtime/timeouts.go","services/processing/scripts/postgres/verify_diagnostics.py","services/processing/docs/HIT-127",".project-os/results/HIT-127.json"],"deviations":[],"known_limitations":["Локальные ограниченные выборки, измерения на изолированном стенде и управляемых mTLS-соседях.","Возраст нового Observation измеряется при коммите и сохраняется в журнале. Ретроспективная обработка архива и HIT-128 вне задачи.","Все пять точных зависимостей входят в проверенный head открытой ветки. GitHub main отделён."],"materialized_result":{"base_sha":"d450efc7cfc11f8155a476304aced96dad0ebaf6","contract_fingerprint":"4bc9b0983e0250104c9557f34237aafb0a36dc25f65af5e381ead8e3b85ecb46","contract_revision":1,"dependency_provenance_review_comment":"15416","dependency_provenance_scope":"Пять точных предков 4da4820 и 4250bb4, разрешение merge e381972, go list и побайтовая повторная сборка двух CLI. Код 4da4820 и доказательства шести DoD сохранены.","implementation_status":"IMPLEMENTED_AND_VERIFIED_ON_OWNED_PROCESS_AND_POSTGRES_STAND","observed_main":"ce771d37505e7770003d0796ae932556d06b7bab","pull_request":"https://github.com/hitpipe/hitpipe/pull/82","review_findings_comment":"15172","rework_of":"fdda9d1da3f5d0697b0a8811585aa2f8988964e2","summary":"Рабочая диагностика Processing с фактическими очередями и отказами. В пакет повторного ревью вложены точный diff, полные целевые журналы, реальные HTTP-ответы Health и полный протокол.","tested_code_sha":"4da48202e77131427730b1827df9ee81940e559a"},"subject":"HIT-127","verification":{"actual_runtime_log":"services/processing/docs/HIT-127/verification/diagnostic-runtime.jsonl","actual_stand":"docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f","ci_protocol":"services/processing/docs/HIT-127/verification/ci-evidence.json","code_ci":["https://github.com/hitpipe/hitpipe/actions/runs/44797634267","https://github.com/hitpipe/hitpipe/actions/runs/44797621844"],"dependency_chain":{"file":"services/processing/docs/HIT-127/review/dependency-chain.md","reviewed_pr_head":"4250bb434a9a48ed436904e0a4609717135f7143","sha256":"0c7b8eed24fdb96a7a8d3bdb65219324bb1c11fc27e275ec0aa94fed3de2bef0"},"diagnostics_map":"services/processing/docs/HIT-127/diagnostics-map.md","report":"services/processing/docs/HIT-127/README.md","reviewable_commands":"services/processing/docs/HIT-127/review/commands-and-output.md","reviewable_health_and_runbook":"services/processing/docs/HIT-127/review/operations.md","reviewable_process_and_http":"services/processing/docs/HIT-127/review/process-and-http.md","runbook":"services/processing/docs/HIT-127/RUN_RU.md","source_audit":"services/processing/docs/HIT-127/source-audit.json","source_diff":"services/processing/docs/HIT-127/review/exact-source.diff","test_protocol":"services/processing/docs/HIT-127/verification/test-evidence.json"}}
~~~~


## Предки и содержимое сборки

Команда `git merge-base --is-ancestor <проверенный SHA или SHA результата> <цель>` выполнена 24 раза. Ниже полные exit codes, stdout и stderr пусты во всех случаях. Столбцы идут в порядке tested/result → c426dc4, tested/result → c394de8.

| Задача | c426dc4 | c394de8 |
| --- | --- | --- |
| HIT-111 | 0, 0 | 0, 0 |
| HIT-112 | 0, 0 | 0, 0 |
| HIT-113 | 0, 0 | 0, 0 |
| HIT-114 | 0, 0 | 0, 0 |
| HIT-126 | 0, 0 | 0, 0 |
| HIT-127 | 0, 0 | 0, 0 |

Вывод `git rev-parse <ref>:services/processing/<path>`, все exit 0. Третий столбец одинаков на двух проверяемых целях. Различия blob у HIT-112/113/114/126 расшифрованы точными патчами ниже.

| Задача и путь | Проверенный исходный blob | blob c426dc4 = c394de8 |
| --- | --- | --- |
| HIT-111 `cmd/processing/main.go` | 48335af3cd2c24cb372fffbc0392552ba84b56f6 | 48335af3cd2c24cb372fffbc0392552ba84b56f6 |
| HIT-111 `internal/runtime/config.go` | 13852b29fe1d2b32374907d67f63236b518fabec | 13852b29fe1d2b32374907d67f63236b518fabec |
| HIT-111 `internal/runtime/iam.go` | 0336b808d1e50340568892f29e27c9fde181d8c8 | 0336b808d1e50340568892f29e27c9fde181d8c8 |
| HIT-112 `internal/adapters/postgres/caseworker.go` | b3047429b1d804fa5cb1ef9594cd363cabf25a47 | 541f8d4cdd0e2ff829436ebc8b977a901d3d22d3 |
| HIT-112 `internal/runtime/caseworker.go` | 84d855654ae46f80bd466522052f9827f766e617 | 3f4a2277f8e92f95ab88fe1d19e739d378ba0352 |
| HIT-113 `internal/adapters/postgres/recheck.go` | 80ecf221f184607e82683550634c37f15710df78 | 80ecf221f184607e82683550634c37f15710df78 |
| HIT-113 `internal/runtime/recheck.go` | ead75b4b85fafe67a439df9789081f4fb36a973a | 86ee1800f0e7cecbc94289e18c4b502907c87faf |
| HIT-114 `internal/adapters/postgres/timeouts.go` | c319df7375f0bb62e36d63c6ea5033dff03e571e | c319df7375f0bb62e36d63c6ea5033dff03e571e |
| HIT-114 `internal/runtime/timeouts.go` | 54baece7ffdb1a3798f4b1d9adc0b52db4817425 | 2086539582cd5919cc2b920d27fbf9e3afcf6052 |
| HIT-126 `cmd/processing-purge/main.go` | 817c64f54eebf143d81d9c01cb475f3a0b081602 | 981ba7fbdc06da1e22d70d8827ec14bed703fc45 |
| HIT-126 `internal/adapters/postgres/purge.go` | 3316baccde796a2a9f93190b8761a7e864c4751a | 3316baccde796a2a9f93190b8761a7e864c4751a |
| HIT-126 `internal/adapters/contentfiles/maintenance.go` | ca3473f6d8d01702f150675f115cab94666dcd7f | ca3473f6d8d01702f150675f115cab94666dcd7f |
| HIT-127 `internal/runtime/diagnostics.go` | aaa214dbd7deb48c3ad2884462c05535a02b3c14 | aaa214dbd7deb48c3ad2884462c05535a02b3c14 |
| HIT-127 `internal/adapters/postgres/diagnostics.go` | abafb0400f25e5d27e7ffc9de5975c03a2b7e77c | abafb0400f25e5d27e7ffc9de5975c03a2b7e77c |
| HIT-127 `internal/runtime/runtime.go` | ca9add65cd97742d7baa5fc3aaba46d5d00dc5c6 | ca9add65cd97742d7baa5fc3aaba46d5d00dc5c6 |

## Фактический Git-граф, конфликт и разрешение

Полный stdout, stderr и exit выбранных команд.

~~~~text
$ git log --ancestry-path '--format=%H %P %s' 'b2e823012854575146595b01a6f56d5ac677fc44^..c394de8abb8d6ed4f5324f45a4b023c3749f1a5f'
exit=0
c394de8abb8d6ed4f5324f45a4b023c3749f1a5f e2c47485f548d66920d151320df5e87a3763f3b6 Deliver full HIT-128 source and execution evidence for re-review
e2c47485f548d66920d151320df5e87a3763f3b6 c426dc400a4bec03fafba72e2a5780c9d85af1e2 Publish HIT-128 recovery runbook, evidence and result
c426dc400a4bec03fafba72e2a5780c9d85af1e2 40ab678566624f20c914b62d4c0b9b1939d8f36d Verify exact Intake replay and rejection of the original worker claim
40ab678566624f20c914b62d4c0b9b1939d8f36d e7c28ecf8f925d7ae13fc0d118dc5f2d0451c0b6 Verify sticky denial separately from the recovered authorized scope
e7c28ecf8f925d7ae13fc0d118dc5f2d0451c0b6 546245a9db20466ee207e9f230a298b9cbe36798 Use a runtime-compatible database name on the recovery stand
546245a9db20466ee207e9f230a298b9cbe36798 19831026d635dd74d4b54dd4acd93e53319c182e Exercise scoped Processing recovery against disposable PostgreSQL clusters
19831026d635dd74d4b54dd4acd93e53319c182e 4250bb434a9a48ed436904e0a4609717135f7143 docs(processing): prove provenance of HIT-127 dependencies
4250bb434a9a48ed436904e0a4609717135f7143 4da48202e77131427730b1827df9ee81940e559a docs(processing): deliver readable evidence for HIT-127 re-review
4da48202e77131427730b1827df9ee81940e559a fdda9d1da3f5d0697b0a8811585aa2f8988964e2 test(processing): capture exact Health responses and focused fault logs
fdda9d1da3f5d0697b0a8811585aa2f8988964e2 f1ddceeda0a61b7a7fd58a79d9b3616d4d614702 docs(processing): record HIT-127 process diagnostics qualification
f1ddceeda0a61b7a7fd58a79d9b3616d4d614702 f30fdf8b1e08a90e1106fce5b482fcfbcce3877a Distinguish a held dispatch from gate denial in diagnostics
f30fdf8b1e08a90e1106fce5b482fcfbcce3877a bdf3187b03558c56f30caa422d6ebeecc2986d8f Wait for committed hold before retiring seeded diagnostics job
bdf3187b03558c56f30caa422d6ebeecc2986d8f d450efc7cfc11f8155a476304aced96dad0ebaf6 processing: report scoped operational diagnostics from runtime state
d450efc7cfc11f8155a476304aced96dad0ebaf6 4c2f1df34f7cf72ddb2de1f6888e033d035913ac docs(processing): record HIT-126 retention qualification
4c2f1df34f7cf72ddb2de1f6888e033d035913ac 633f7c35c4db4707c9dae6576829f54f187887ce Test purge recovery after abrupt exit following outbox hold
633f7c35c4db4707c9dae6576829f54f187887ce e381972ab7ceb8913e345852c8e073998ec59afe processing: add role-scoped offline retention purge
e381972ab7ceb8913e345852c8e073998ec59afe 33d2c8da52585bcdc2216df9d2033f9089209edb dd1a36f6529f57d7452a27b615b6cad18b1288bd Integrate HIT-113 recheck and HIT-114 case timeouts for HIT-126
33d2c8da52585bcdc2216df9d2033f9089209edb dd05559bc911437d33c0c2582b604d2f2728d362 docs(processing): record HIT-114 timer verification
dd05559bc911437d33c0c2582b604d2f2728d362 3b542e88447f41f8287cc0507814de3f3a330cc6 feat(processing): enforce durable case age limits in HIT-114
dd1a36f6529f57d7452a27b615b6cad18b1288bd 9165e814cb7b46e9cfae484c62ccc70f63fc1ee3 docs(processing): record HIT-113 RECHECK proof and source map
9165e814cb7b46e9cfae484c62ccc70f63fc1ee3 3b542e88447f41f8287cc0507814de3f3a330cc6 feat(processing): qualify and dispatch known-task RECHECK in HIT-113
3b542e88447f41f8287cc0507814de3f3a330cc6 0f710a6682d15b437f847376b348947cb4757c7c docs(processing): record HIT-112 worker verification
0f710a6682d15b437f847376b348947cb4757c7c 49229e1db3f371a051a022ea9e31951dd558eecb feat(processing): recover and advance admitted cases in HIT-112
49229e1db3f371a051a022ea9e31951dd558eecb b2e823012854575146595b01a6f56d5ac677fc44 docs(processing): record HIT-111 executable and authorization checks
b2e823012854575146595b01a6f56d5ac677fc44 1bbb3567c042a1ac79f21d69f78d71e8a4fef41e feat(processing): compose private HTTP, IAM start gate and durable sender

$ git merge-base --is-ancestor 19831026d635dd74d4b54dd4acd93e53319c182e origin/main
exit=1

$ git diff --exit-code c426dc400a4bec03fafba72e2a5780c9d85af1e2 c394de8abb8d6ed4f5324f45a4b023c3749f1a5f -- . ':(exclude)services/processing/docs/HIT-128/**' ':(exclude).project-os/results/HIT-128.json'
exit=0

$ git merge-tree --write-tree --name-only 33d2c8da52585bcdc2216df9d2033f9089209edb dd1a36f6529f57d7452a27b615b6cad18b1288bd
exit=1
9941fcc5d6157048c313db38021fdbc42c1e980d
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

$ git show --cc --format= --unified=0 e381972ab7ceb8913e345852c8e073998ec59afe -- services/processing/internal/adapters/postgres/dispatch.go services/processing/internal/application/readcases/presentation.go services/processing/internal/runtime/runtime.go
exit=0
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

$ git diff --unified=0 0f710a6682d15b437f847376b348947cb4757c7c c426dc400a4bec03fafba72e2a5780c9d85af1e2 -- services/processing/internal/adapters/postgres/caseworker.go
exit=0
diff --git a/services/processing/internal/adapters/postgres/caseworker.go b/services/processing/internal/adapters/postgres/caseworker.go
index b304742..541f8d4 100644
--- a/services/processing/internal/adapters/postgres/caseworker.go
+++ b/services/processing/internal/adapters/postgres/caseworker.go
@@ -42,0 +43,8 @@ const eligibleCase = `
+ AND NOT EXISTS (
+   SELECT 1 FROM hp_processing.safety_history d
+   WHERE d.workspace_id=k.workspace_id AND d.case_id=k.case_id AND d.old_state=k.state AND d.new_state=k.state
+     AND d.reason_code LIKE 'CASE\_TIMER\_%' AND d.at >= COALESCE((
+       SELECT max(h.at) FROM hp_processing.safety_history h
+       WHERE h.workspace_id=k.workspace_id AND h.case_id=k.case_id AND h.old_state IS DISTINCT FROM h.new_state
+     ),'-infinity')
+ )

$ git show --format= --unified=0 bdf3187b03558c56f30caa422d6ebeecc2986d8f -- services/processing/internal/runtime/caseworker.go services/processing/internal/runtime/recheck.go services/processing/internal/runtime/timeouts.go services/processing/cmd/processing-purge/main.go
exit=0
diff --git a/services/processing/cmd/processing-purge/main.go b/services/processing/cmd/processing-purge/main.go
index 817c64f..981ba7f 100644
--- a/services/processing/cmd/processing-purge/main.go
+++ b/services/processing/cmd/processing-purge/main.go
@@ -10,0 +11 @@ import (
+	"log/slog"
@@ -60,0 +62 @@ func run(ctx context.Context, workspace, path string, limit int, local bool) int
+	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
@@ -63 +65 @@ func run(ctx context.Context, workspace, path string, limit int, local bool) int
-		fmt.Fprintln(os.Stderr, "private Processing content directory unavailable or busy")
+		log.Warn("private Processing content directory unavailable or busy", "reason", "purge_volume_unavailable", "workspace_id", workspace)
@@ -69 +71 @@ func run(ctx context.Context, workspace, path string, limit int, local bool) int
-		fmt.Fprintln(os.Stderr, "local Processing database unavailable")
+		log.Warn("local Processing database unavailable", "reason", "purge_postgres_unavailable", "workspace_id", workspace)
@@ -76 +78 @@ func run(ctx context.Context, workspace, path string, limit int, local bool) int
-			fmt.Fprintln(os.Stderr, postgres.ErrPurgeScope)
+			log.Warn(postgres.ErrPurgeScope.Error(), "reason", "purge_scope_denied", "workspace_id", workspace)
@@ -78 +80 @@ func run(ctx context.Context, workspace, path string, limit int, local bool) int
-			fmt.Fprintln(os.Stderr, "processing purge incomplete; review local state before repeating")
+			log.Warn("processing purge incomplete; review local state before repeating", "reason", "purge_incomplete", "workspace_id", workspace)
diff --git a/services/processing/internal/runtime/caseworker.go b/services/processing/internal/runtime/caseworker.go
index 84d8556..3f4a227 100644
--- a/services/processing/internal/runtime/caseworker.go
+++ b/services/processing/internal/runtime/caseworker.go
@@ -5,0 +6 @@ import (
+	"time"
@@ -54,0 +56 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
+	fault := barrierDenied
@@ -55,0 +58 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
+		fault = jobRecoveryFailed
@@ -58,0 +62 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
+		fault = jobClaimFailed
@@ -64 +68 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
-		rt.logger.Warn("processing case worker could not claim work")
+		rt.diag.fault(fault, workspace, "", "")
@@ -84,0 +89,2 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
+	case err != nil && !time.Now().Before(claim.Until()):
+		rt.diag.fault(leaseExpired, workspace, claim.CaseID, "")
@@ -86 +92 @@ func (rt *Runtime) advanceCase(ctx context.Context, workspace string) {
-		rt.logger.Info("processing case worker lost current claim", "case_id", claim.CaseID)
+		rt.diag.fault(leaseStale, workspace, claim.CaseID, "")
diff --git a/services/processing/internal/runtime/recheck.go b/services/processing/internal/runtime/recheck.go
index ead75b4..86ee180 100644
--- a/services/processing/internal/runtime/recheck.go
+++ b/services/processing/internal/runtime/recheck.go
@@ -89 +89,2 @@ func (rt *Runtime) advanceRecheck(ctx context.Context, workspace string) {
-	out, _ := rt.queries.RunScope(c, dispatchintent.Request{WorkspaceID: workspace, Epoch: p.epoch})
+	out, err := rt.queries.RunScope(c, dispatchintent.Request{WorkspaceID: workspace, Epoch: p.epoch})
+	rt.diag.dispatchFault(recheckInterrupted, workspace, err)
diff --git a/services/processing/internal/runtime/timeouts.go b/services/processing/internal/runtime/timeouts.go
index 54baece..2086539 100644
--- a/services/processing/internal/runtime/timeouts.go
+++ b/services/processing/internal/runtime/timeouts.go
@@ -43 +43 @@ func (rt *Runtime) advanceTimeout(ctx context.Context, workspace string) {
-		rt.logger.Warn("processing case timer interrupted")
+		rt.diag.fault(timerInterrupted, workspace, "", "")
~~~~


Merge-tree вернул 1 при трёх конфликтах. Сохранены kind, состояния, RECHECK и таймеры. Фильтр CASE_TIMER совпадает с диффом на HIT-114. Диагностика добавлена в bdf3187.


## Проверенная компоновка

`go list -deps` на Linux arm64 с Go 1.27.2, CGO_ENABLED=0, `./cmd/processing`, exit 0 для обеих копий. Одинаковый полный stdout выбранных пакетов:

~~~~text
github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres | activation.go admit.go caseworker.go diagnostics.go dispatch.go draft_check.go drafts.go lifecycle.go lifecycle_drain.go lifecycle_resume.go observation_read.go observations.go prepare.go purge.go read_cases.go recheck.go recheck_dispatch.go record.go timeouts.go
github.com/hitpipe/hitpipe/services/processing/internal/runtime | access.go caseworker.go config.go diagnostics.go iam.go queue.go recheck.go runtime.go source.go timeouts.go
github.com/hitpipe/hitpipe/services/processing/cmd/processing | main.go
~~~~

`go list -deps` на Linux arm64 с Go 1.27.2, CGO_ENABLED=0, `./cmd/processing-purge`, exit 0 для обеих копий. Одинаковый полный stdout выбранных пакетов:

~~~~text
github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles | draft.go maintenance.go read_owned.go store.go
github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres | activation.go admit.go caseworker.go diagnostics.go dispatch.go draft_check.go drafts.go lifecycle.go lifecycle_drain.go lifecycle_resume.go observation_read.go observations.go prepare.go purge.go read_cases.go recheck.go recheck_dispatch.go record.go timeouts.go
github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge | main.go
~~~~

Процессные регрессии c426dc4, exit 0. Команда полностью сохранена в EVID-007, оригинальный вывод в EVID-001.

~~~~text
--- PASS: TestProcessingCaseWorkerOwnedStand (16.40s)
--- PASS: TestProcessingDiagnosticsOwnedStand (14.17s)
--- PASS: TestProcessingPurgeOwnedStand (0.13s)
--- PASS: TestProcessingRecheckOwnedStand (23.68s)
--- PASS: TestProcessingExecutableOwnedStand (3.19s)
--- PASS: TestProcessingCaseTimeoutsOwnedStand (1.41s)
{"platform":"linux/arm64","sha256":{"processing":"9fadd08dec36f4d8a32d1ca6c75d82868d2b3ff4862bf6cf878f843b4de96b3e","processing-purge":"3b0011350119f3f164aecfc7975b624d2108623265bd4eb2ea27d3d59383917a","storage.test":"6d0053c7d89f1d1ed76a9de465f1cb040cc74d76554a611928acd9d791a892f9"},"source_sha":"c426dc400a4bec03fafba72e2a5780c9d85af1e2"}
~~~~

Полные четыре добавленных файла HIT-128 сохранены в активном EVID-006 exact-source.diff: hunks содержат все строки с первого байта, плюс отдельный патч runtime_process_test.go. SHA-256 патча e09ecf6812fb3bce67a6c4ba9ddf05cabd923f84c82c1572b33e95110c21fbd1. Применение к 1983102 даёт tree ac69c7a9cce91c70fdbfe31a38ab725927defdff.
