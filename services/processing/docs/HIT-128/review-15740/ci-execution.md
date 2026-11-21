# Фактические результаты GitHub Actions для проверенного HIT-128

PR №85, исходник `c426dc400a4bec03fafba72e2a5780c9d85af1e2`. Оригинальные JSON ниже включены полностью. Github REST сообщает conclusion каждого job и step. Для shell-шага с зарегистрированным `/usr/bin/bash -e` и conclusion `success` итоговый код шага равен 0. Данные о произвольных внутренних командах, которые шаг не публикует, не приписываются GitHub. Для локальных команд настоящий exit code есть в local-execution.md.

Запуск push действительно checkout `c426dc400a4bec03fafba72e2a5780c9d85af1e2`. Pull request checkout использовал GitHub merge ref `0042dc7b3df73256db6d38a92a5fffaf75691f76`. Отдельная проверка GitHub tree ниже позволяет сопоставить его содержимое с точным исходником. Встроенная строка `Race check NOT_RUN` относится к раннему шагу workspace, а отдельный последующий шаг действительно выполнял `go test -race -count=1 ./...`. CI собирает диагностическую команду. Полная сборка Processing и PostgreSQL квалификация зафиксированы в локальных журналах.

## ci-commands.json

SHA-256 `511106bc50cb5858c77e90562002cdd407ded4e08c6d33caeefce365334e0dd7`.

~~~~json
[
  {
    "argv": [
      "gh",
      "pr",
      "view",
      "85",
      "--json",
      "headRefOid,baseRefOid,author,isDraft,state,mergeable,statusCheckRollup,url"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit128-processing-recovery",
    "source_sha": "c426dc400a4bec03fafba72e2a5780c9d85af1e2",
    "started_at": "2026-11-21T07:44:27.750474+00:00",
    "finished_at": "2026-11-21T07:44:28.898144+00:00",
    "exit_code": 0,
    "stdout_file": "ci-pr.json",
    "stdout_sha256": "d5f1730d50255734d6a53fb1081c8a37fffb759561ec73f2c8d12dd169258747",
    "stderr": ""
  },
  {
    "argv": [
      "gh",
      "run",
      "view",
      "44881435097",
      "--json",
      "headSha,event,status,conclusion,jobs,url,workflowName"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit128-processing-recovery",
    "source_sha": "c426dc400a4bec03fafba72e2a5780c9d85af1e2",
    "started_at": "2026-11-21T07:44:28.898631+00:00",
    "finished_at": "2026-11-21T07:44:30.684543+00:00",
    "exit_code": 0,
    "stdout_file": "ci-processing-44881435097.json",
    "stdout_sha256": "f6ee8654acbd884b7cc0e6f0f93fc33f95e1ecea3d1083f4094661756f0816ea",
    "stderr": ""
  },
  {
    "argv": [
      "gh",
      "run",
      "view",
      "44881430920",
      "--json",
      "headSha,event,status,conclusion,jobs,url,workflowName"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit128-processing-recovery",
    "source_sha": "c426dc400a4bec03fafba72e2a5780c9d85af1e2",
    "started_at": "2026-11-21T07:44:30.684815+00:00",
    "finished_at": "2026-11-21T07:44:32.256767+00:00",
    "exit_code": 0,
    "stdout_file": "ci-processing-44881430920.json",
    "stdout_sha256": "3ef70e2f5c07f2294ba6630244fb2d580c5b73c127449b3e67cac41b5b8655b6",
    "stderr": ""
  }
]
~~~~

## ci-pr.json

SHA-256 `d5f1730d50255734d6a53fb1081c8a37fffb759561ec73f2c8d12dd169258747`.

~~~~json
{"author":{"id":"U_kgDOCNgUFQ","is_bot":false,"login":"Tim1hik","name":"Tima"},"baseRefOid":"19831026d635dd74d4b54dd4acd93e53319c182e","headRefOid":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","isDraft":true,"mergeable":"MERGEABLE","state":"OPEN","statusCheckRollup":[{"__typename":"CheckRun","completedAt":"2026-11-21T07:44:25Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881435140/job/136777382723","name":"pr12-iam","startedAt":"2026-11-21T07:41:54Z","status":"COMPLETED","workflowName":"IAM — проверки текущей реализации"},{"__typename":"CheckRun","completedAt":"2026-11-21T07:42:09Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881435104/job/136777382707","name":"pr12-delivery","startedAt":"2026-11-21T07:41:55Z","status":"COMPLETED","workflowName":"PR12 delivery — bootstrap or submitted component checks"},{"__typename":"CheckRun","completedAt":"2026-11-21T07:42:06Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881435105/job/136777382272","name":"pr12-intake","startedAt":"2026-11-21T07:41:55Z","status":"COMPLETED","workflowName":"PR12 intake — bootstrap or submitted component checks"},{"__typename":"CheckRun","completedAt":"2026-11-21T07:43:38Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881435097/job/136777382454","name":"pr12-processing","startedAt":"2026-11-21T07:41:54Z","status":"COMPLETED","workflowName":"PR12 processing — bootstrap or submitted component checks"},{"__typename":"CheckRun","completedAt":"2026-11-21T07:43:45Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881430920/job/136777370075","name":"pr12-processing","startedAt":"2026-11-21T07:41:51Z","status":"COMPLETED","workflowName":"PR12 processing — bootstrap or submitted component checks"},{"__typename":"CheckRun","completedAt":"2026-11-21T07:42:41Z","conclusion":"SUCCESS","detailsUrl":"https://github.com/hitpipe/hitpipe/actions/runs/44881435121/job/136777382413","name":"platform-start-gate","startedAt":"2026-11-21T07:41:55Z","status":"COMPLETED","workflowName":"Platform — локальная защита начала работы"}],"url":"https://github.com/hitpipe/hitpipe/pull/85"}
~~~~

## ci-processing-44881430920.json

SHA-256 `3ef70e2f5c07f2294ba6630244fb2d580c5b73c127449b3e67cac41b5b8655b6`.

~~~~json
{"conclusion":"success","event":"push","headSha":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","jobs":[{"completedAt":"2026-11-21T07:43:45Z","conclusion":"success","databaseId":136777370075,"name":"pr12-processing","startedAt":"2026-11-21T07:41:51Z","status":"completed","steps":[{"completedAt":"2026-11-21T07:41:54Z","conclusion":"success","name":"Set up job","number":1,"startedAt":"2026-11-21T07:41:52Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Run actions/checkout@v4","number":2,"startedAt":"2026-11-21T07:41:54Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Verify bootstrap documentation and frozen copies","number":3,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Verify bootstrap wrapper boundaries (not student tests)","number":4,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Report actual implementation state","number":5,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:42:06Z","conclusion":"success","name":"Run actions/setup-go@v5","number":6,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:42:30Z","conclusion":"success","name":"Run actual named tests when module exists","number":7,"startedAt":"2026-11-21T07:42:06Z","status":"completed"},{"completedAt":"2026-11-21T07:42:38Z","conclusion":"success","name":"Run declared diagnostic smoke in both modes","number":8,"startedAt":"2026-11-21T07:42:30Z","status":"completed"},{"completedAt":"2026-11-21T07:43:43Z","conclusion":"success","name":"Race and mutation sensitivity for Processing","number":9,"startedAt":"2026-11-21T07:42:38Z","status":"completed"},{"completedAt":"2026-11-21T07:43:43Z","conclusion":"success","name":"Run actions/upload-artifact@v4","number":10,"startedAt":"2026-11-21T07:43:43Z","status":"completed"},{"completedAt":"2026-11-21T07:43:43Z","conclusion":"success","name":"Scope of this job","number":11,"startedAt":"2026-11-21T07:43:43Z","status":"completed"},{"completedAt":"2026-11-21T07:43:44Z","conclusion":"success","name":"Post Run actions/setup-go@v5","number":21,"startedAt":"2026-11-21T07:43:43Z","status":"completed"},{"completedAt":"2026-11-21T07:43:44Z","conclusion":"success","name":"Post Run actions/checkout@v4","number":22,"startedAt":"2026-11-21T07:43:44Z","status":"completed"},{"completedAt":"2026-11-21T07:43:44Z","conclusion":"success","name":"Complete job","number":23,"startedAt":"2026-11-21T07:43:44Z","status":"completed"}],"url":"https://github.com/hitpipe/hitpipe/actions/runs/44881430920/job/136777370075"}],"status":"completed","url":"https://github.com/hitpipe/hitpipe/actions/runs/44881430920","workflowName":"PR12 processing — bootstrap or submitted component checks"}
~~~~

## ci-processing-44881435097.json

SHA-256 `f6ee8654acbd884b7cc0e6f0f93fc33f95e1ecea3d1083f4094661756f0816ea`.

~~~~json
{"conclusion":"success","event":"pull_request","headSha":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","jobs":[{"completedAt":"2026-11-21T07:43:38Z","conclusion":"success","databaseId":136777382454,"name":"pr12-processing","startedAt":"2026-11-21T07:41:54Z","status":"completed","steps":[{"completedAt":"2026-11-21T07:41:56Z","conclusion":"success","name":"Set up job","number":1,"startedAt":"2026-11-21T07:41:55Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Run actions/checkout@v4","number":2,"startedAt":"2026-11-21T07:41:56Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Verify bootstrap documentation and frozen copies","number":3,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Verify bootstrap wrapper boundaries (not student tests)","number":4,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:41:59Z","conclusion":"success","name":"Report actual implementation state","number":5,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:42:05Z","conclusion":"success","name":"Run actions/setup-go@v5","number":6,"startedAt":"2026-11-21T07:41:59Z","status":"completed"},{"completedAt":"2026-11-21T07:42:28Z","conclusion":"success","name":"Run actual named tests when module exists","number":7,"startedAt":"2026-11-21T07:42:05Z","status":"completed"},{"completedAt":"2026-11-21T07:42:34Z","conclusion":"success","name":"Run declared diagnostic smoke in both modes","number":8,"startedAt":"2026-11-21T07:42:28Z","status":"completed"},{"completedAt":"2026-11-21T07:43:35Z","conclusion":"success","name":"Race and mutation sensitivity for Processing","number":9,"startedAt":"2026-11-21T07:42:34Z","status":"completed"},{"completedAt":"2026-11-21T07:43:36Z","conclusion":"success","name":"Run actions/upload-artifact@v4","number":10,"startedAt":"2026-11-21T07:43:35Z","status":"completed"},{"completedAt":"2026-11-21T07:43:36Z","conclusion":"success","name":"Scope of this job","number":11,"startedAt":"2026-11-21T07:43:36Z","status":"completed"},{"completedAt":"2026-11-21T07:43:36Z","conclusion":"success","name":"Post Run actions/setup-go@v5","number":21,"startedAt":"2026-11-21T07:43:36Z","status":"completed"},{"completedAt":"2026-11-21T07:43:36Z","conclusion":"success","name":"Post Run actions/checkout@v4","number":22,"startedAt":"2026-11-21T07:43:36Z","status":"completed"},{"completedAt":"2026-11-21T07:43:36Z","conclusion":"success","name":"Complete job","number":23,"startedAt":"2026-11-21T07:43:36Z","status":"completed"}],"url":"https://github.com/hitpipe/hitpipe/actions/runs/44881435097/job/136777382454"}],"status":"completed","url":"https://github.com/hitpipe/hitpipe/actions/runs/44881435097","workflowName":"PR12 processing — bootstrap or submitted component checks"}
~~~~

## Processing push, run 44881430920, job 136777370075

Из оригинального вывода `gh run view --repo hitpipe/hitpipe 44881430920 --job 136777370075 --log`. Отбор содержит checkout, команды проверки, результаты smoke, а также весь вывод шага race и mutation. Номер строки относится к оригинальному потоку. Управляющие ANSI-последовательности сняты. SHA-256 оригинала `489562ee1752ba5e5a4069bc1bc42559b57dc02a9eae18e974d1fc23cf0575d2`, 1415932 байт.

~~~~text
102: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:58.8709274Z [command]/usr/bin/git checkout --progress --force -B work/HIT-128-processing-recovery refs/remotes/origin/work/HIT-128-processing-recovery
106: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.0800889Z [command]/usr/bin/git log -1 --format=%H
107: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.0822020Z c426dc400a4bec03fafba72e2a5780c9d85af1e2
109: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.1014719Z python3 tools/pr12/workspace.py validate
114: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.3694195Z {"status": "BOOTSTRAP_VALIDATION_PASS", "verified_frozen_files": 6, "student_implementation": "NOT_ASSESSED", "human_gates": "NOT_ASSESSED"}
116: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.3786929Z python3 tools/pr12/test_workspace.py
122: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.5708342Z Ran 18 tests in 0.007s
124: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.5708624Z OK
150: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:04.7136733Z go version go1.27.2 linux/amd64
205: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:06.0194689Z python3 tools/pr12/workspace.py check processing | tee .pr12-build/processing-check.log
208: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:06.0578004Z + go version
209: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:06.1020229Z + go vet ./...
217: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:21.9777135Z + go test -count=1 -json -coverprofile=/home/runner/work/hitpipe/hitpipe/.pr12-build/processing.cover ./...
4553: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.5035151Z Race check NOT_RUN; use PR12_RACE=1 where supported and report scope honestly.
4555: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.5501642Z python3 tools/pr12/workspace.py smoke processing --require
4558: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.5872162Z + go version
4559: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.5909165Z + git rev-parse HEAD
4560: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.5922190Z + git status --porcelain
4561: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:30.6450503Z + go build -ldflags -X main.version=pr12 -X main.commit=c426dc400a4bec03fafba72e2a5780c9d85af1e2 -X main.dirty=false -X main.buildMode=dev -gcflags=all=-N -l -o /home/runner/work/hitpipe/hitpipe/.pr12-build/processing/dev/processing-contractcheck ./cmd/processing-contractcheck
4563: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:33.8989012Z + git rev-parse --short HEAD
4565: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:33.9052240Z {"component": "processing", "mode": "dev", "diagnostic_smoke": "PASS", "scope": "declared synthetic fixtures only"}
4566: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:33.9052663Z + git rev-parse HEAD
4567: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:33.9063325Z + git status --porcelain
4568: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:33.9134052Z + go build -ldflags -X main.version=pr12 -X main.commit=c426dc400a4bec03fafba72e2a5780c9d85af1e2 -X main.dirty=false -X main.buildMode=release -trimpath -o /home/runner/work/hitpipe/hitpipe/.pr12-build/processing/release/processing-contractcheck ./cmd/processing-contractcheck
4570: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.2653774Z + git rev-parse --short HEAD
4572: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.2722320Z {"component": "processing", "mode": "release", "diagnostic_smoke": "PASS", "scope": "declared synthetic fixtures only"}
4573: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3244639Z ##[group]Run cd services/processing
4574: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3244890Z cd services/processing
4575: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3245092Z go test -race -count=1 ./...
4576: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3245287Z cd ../..
4577: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3245821Z python3 services/processing/tools/mutations/run.py --output .pr12-build/processing-mutations --natural-defects 1 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29
4578: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3273692Z shell: /usr/bin/bash -e {0}
4579: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:38.3273908Z ##[endgroup]
4580: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.5766223Z ?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
4581: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.5984871Z ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.021s
4582: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.5985898Z ?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
4583: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.5987077Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
4584: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.7725091Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.166s
4585: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.8726134Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.142s
4586: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.8727264Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.222s
4587: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.8729412Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
4588: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.9479033Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.175s
4589: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:02.8675894Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.046s
4590: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:02.8843013Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.011s
4591: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:03.5188354Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	1.595s
4592: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:03.5189454Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
4593: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:03.5190406Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
4594: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:03.5191509Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
4595: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:03.5192399Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.030s
4596: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5188915Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.044s
4597: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5189584Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.032s
4598: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5490828Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.030s
4599: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5491543Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
4600: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5492204Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
4601: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5492855Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
4602: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.5493601Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
4603: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:04.6015809Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.009s
4604: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.5202531Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.011s
4605: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.5413877Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.021s
4606: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.5415236Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
4607: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.5593058Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.009s
4608: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.5594564Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
4609: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:05.6198724Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.017s
4610: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:06.8593491Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.012s
4611: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:06.8594019Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
4612: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:06.9210831Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.012s
4613: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:07.4485794Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.022s
4614: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:07.4486487Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
4615: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:43.0265861Z {"baseline_sha": "c426dc400a4bec03fafba72e2a5780c9d85af1e2", "seeded": 6, "compiled": 6, "detected": 6, "missed": 0, "invalid": 0, "clean": true}
~~~~

## Processing pr, run 44881435097, job 136777382454

Из оригинального вывода `gh run view --repo hitpipe/hitpipe 44881435097 --job 136777382454 --log`. Отбор содержит checkout, команды проверки, результаты smoke, а также весь вывод шага race и mutation. Номер строки относится к оригинальному потоку. Управляющие ANSI-последовательности сняты. SHA-256 оригинала `12ca35030d203eaed97aa2a4f258e7247bf2a0515b00613d80e47c491338101d`, 1417269 байт.

~~~~text
102: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.2543160Z [command]/usr/bin/git checkout --progress --force refs/remotes/pull/85/merge
122: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.4538867Z [command]/usr/bin/git log -1 --format=%H
123: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.4558601Z 0042dc7b3df73256db6d38a92a5fffaf75691f76
125: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.4724863Z python3 tools/pr12/workspace.py validate
130: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.6556359Z {"status": "BOOTSTRAP_VALIDATION_PASS", "verified_frozen_files": 6, "student_implementation": "NOT_ASSESSED", "human_gates": "NOT_ASSESSED"}
132: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.6631426Z python3 tools/pr12/test_workspace.py
138: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.8096326Z Ran 18 tests in 0.007s
140: pr12-processing	UNKNOWN STEP	2026-11-21T07:41:59.8096748Z OK
166: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:04.6314034Z go version go1.27.2 linux/amd64
221: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:05.2453850Z python3 tools/pr12/workspace.py check processing | tee .pr12-build/processing-check.log
224: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:05.2819953Z + go version
225: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:05.3264459Z + go vet ./...
233: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:20.1192291Z + go test -count=1 -json -coverprofile=/home/runner/work/hitpipe/hitpipe/.pr12-build/processing.cover ./...
4569: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.0137338Z Race check NOT_RUN; use PR12_RACE=1 where supported and report scope honestly.
4571: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.0603034Z python3 tools/pr12/workspace.py smoke processing --require
4574: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.0967592Z + go version
4575: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.0993394Z + git rev-parse HEAD
4576: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.1004650Z + git status --porcelain
4577: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:28.2185149Z + go build -ldflags -X main.version=pr12 -X main.commit=0042dc7b3df73256db6d38a92a5fffaf75691f76 -X main.dirty=false -X main.buildMode=dev -gcflags=all=-N -l -o /home/runner/work/hitpipe/hitpipe/.pr12-build/processing/dev/processing-contractcheck ./cmd/processing-contractcheck
4579: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:31.0653069Z + git rev-parse --short HEAD
4581: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:31.0718795Z {"component": "processing", "mode": "dev", "diagnostic_smoke": "PASS", "scope": "declared synthetic fixtures only"}
4582: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:31.0719226Z + git rev-parse HEAD
4583: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:31.0728653Z + git status --porcelain
4584: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:31.0794438Z + go build -ldflags -X main.version=pr12 -X main.commit=0042dc7b3df73256db6d38a92a5fffaf75691f76 -X main.dirty=false -X main.buildMode=release -trimpath -o /home/runner/work/hitpipe/hitpipe/.pr12-build/processing/release/processing-contractcheck ./cmd/processing-contractcheck
4586: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:34.9754237Z + git rev-parse --short HEAD
4588: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:34.9832775Z {"component": "processing", "mode": "release", "diagnostic_smoke": "PASS", "scope": "declared synthetic fixtures only"}
4589: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0384261Z ##[group]Run cd services/processing
4590: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0384540Z cd services/processing
4591: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0384743Z go test -race -count=1 ./...
4592: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0384928Z cd ../..
4593: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0385485Z python3 services/processing/tools/mutations/run.py --output .pr12-build/processing-mutations --natural-defects 1 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29
4594: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0415398Z shell: /usr/bin/bash -e {0}
4595: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:35.0415600Z ##[endgroup]
4596: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:55.1849383Z ?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing	[no test files]
4597: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:56.1993645Z ok  	github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck	1.015s
4598: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:56.1994640Z ?   	github.com/hitpipe/hitpipe/services/processing/cmd/processing-purge	[no test files]
4599: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:56.1995595Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/activationhttp	[no test files]
4600: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:56.4535009Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp	1.156s
4601: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:57.4044183Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles	2.100s
4602: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:57.4241240Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp	1.224s
4603: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:57.4241847Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/drafthttp	[no test files]
4604: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:57.5776250Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/draftjson	1.124s
4605: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:58.3273401Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/input	1.044s
4606: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:58.4159417Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/fixtures/sourcefacts	1.011s
4607: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.0084106Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intake/sourcefacts	1.583s
4608: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.0084785Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/intakehttp	[no test files]
4609: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.0085327Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/lifecyclehttp	[no test files]
4610: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.0085884Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationhttp	[no test files]
4611: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.0086361Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/observationjson	1.028s
4612: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.8687786Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres	1.038s
4613: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9288441Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/intent	1.032s
4614: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9453897Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres/normalizedsignal	1.029s
4615: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9454841Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/adapters/recheckhttp	[no test files]
4616: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9455560Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/activation	[no test files]
4617: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9456237Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/admission	[no test files]
4618: pr12-processing	UNKNOWN STEP	2026-11-21T07:42:59.9456886Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/casetimeouts	[no test files]
4619: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.0167510Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/dispatchintent	1.008s
4620: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.8800942Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/drafts	1.011s
4621: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.9497832Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/evaluatepaidorder	1.020s
4622: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.9498395Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/lifecycle	[no test files]
4623: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.9551625Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/observations	1.009s
4624: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:00.9552704Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/preparecase	[no test files]
4625: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.0345375Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/readcases	1.017s
4626: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.8903218Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/application/receivesignals	1.010s
4627: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:01.8903781Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/application/recheck	[no test files]
4628: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:02.1476200Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/domain/eligibility	1.011s
4629: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:02.7331061Z ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.022s
4630: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:02.7331717Z ?   	github.com/hitpipe/hitpipe/services/processing/internal/testfixtures	[no test files]
4631: pr12-processing	UNKNOWN STEP	2026-11-21T07:43:35.4438612Z {"baseline_sha": "0042dc7b3df73256db6d38a92a5fffaf75691f76", "seeded": 6, "compiled": 6, "detected": 6, "missed": 0, "invalid": 0, "clean": true}
~~~~

## Дерево исходника в обоих запусках

~~~~text
$ gh api repos/hitpipe/hitpipe/git/commits/c426dc400a4bec03fafba72e2a5780c9d85af1e2 --jq '{sha,tree:.tree.sha,parents:[.parents[].sha]}'
{"parents":["40ab678566624f20c914b62d4c0b9b1939d8f36d"],"sha":"c426dc400a4bec03fafba72e2a5780c9d85af1e2","tree":"ac69c7a9cce91c70fdbfe31a38ab725927defdff"}
exit=0
$ gh api repos/hitpipe/hitpipe/git/commits/0042dc7b3df73256db6d38a92a5fffaf75691f76 --jq '{sha,tree:.tree.sha,parents:[.parents[].sha]}'
{"parents":["19831026d635dd74d4b54dd4acd93e53319c182e","c426dc400a4bec03fafba72e2a5780c9d85af1e2"],"sha":"0042dc7b3df73256db6d38a92a5fffaf75691f76","tree":"ac69c7a9cce91c70fdbfe31a38ab725927defdff"}
exit=0
~~~~
