import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
NB = ROOT / "docs/labs/pr12/nb"
EVIDENCE = NB / "evidence"
SCREEN = EVIDENCE / "screenshots"
SCREEN.mkdir(parents=True, exist_ok=True)

def sha(path):
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest()

def rel(path):
    return str(path.relative_to(ROOT))

manifest = json.loads((ROOT / "docs/labs/pr12/manifest.json").read_text(encoding="utf-8"))
nb_crosswalk = manifest["criteria_crosswalk"]["nb"]
human_pending = {"0302-pz01/Р1", "0302-pz01/Р4", "0302-pz01/Р5", "0302-pz01/Р6", "0302-pz01/Р7", "0104-pz01/Р2", "0104-pz01/Р6", "0104-pz02/Р3", "0104-pz02/Р7"}
artifact_by_lab = {
    "0302-pz01": ["requirements.md", "requirement-change.md", "ambiguity-decisions.md", "human-gates"],
    "0302-pz02": ["test-design.md", "coverage-analysis.md", "MUTATION_PROTOCOL.md", "evidence/mutations"],
    "0104-pz01": ["toolchain.md", "execution-path.md", "evidence/hit-54-d1-clean.log"],
    "0104-pz02": ["module-contract.md", "module-map.md", "module-adr.md", "complexity-profile.md"],
}
task_sha = {
    "NB-R1": "9659c39ad0010e1329bfe3e3bc92c9edd84982bb",
    "NB-R2": "35b4c247319cf3bf0c8d8288d109c996fc721d0b",
    "NB-C1": "8634767abf8fa235407db3581a8c38705bbdebb8",
    "NB-C2": "2a20fb6a21fe762f2cf37ded433c99bf5390be81",
    "NB-C3": "df1a7d58179148abfda8e71862654f5ef7fcdbc7",
    "NB-D1": "a74a5e1a8ada1147b5755503fd01fd458b8a4706",
    "NB-H1": "78a4ecb87a50ec79f8979b08a4449fcfd3d77c6b",
    "NB-REPORT": "803c9c14727d0649be23994444d5e114179bee41",
}
task_artifacts = {
    "NB-R1": ["docs/labs/pr12/nb/requirements.md", "docs/labs/pr12/nb/requirements-trace.json"],
    "NB-R2": ["docs/labs/pr12/nb/requirement-change.md", "docs/labs/pr12/nb/ambiguity-decisions.md", "docs/labs/pr12/nb/evidence/hit-50-r2-validation.log"],
    "NB-C1": ["services/intake/internal/domain/stripesignature/verifier.go", "services/intake/internal/domain/stripesignature/verifier_test.go", "services/intake/cmd/intake-contractcheck/main.go"],
    "NB-C2": ["services/intake/internal/application/authenticatewebhook/authenticate.go", "services/intake/internal/application/authenticatewebhook/authenticate_test.go", "services/intake/docs/module-contract.md"],
    "NB-C3": ["services/intake/internal/domain/stripesignature/verifier.go", "services/intake/internal/domain/stripesignature/verifier_test.go", "docs/labs/pr12/nb/evidence/mutations", "docs/labs/pr12/nb/evidence/hit-53-baseline-v2.log"],
    "NB-D1": ["docs/labs/pr12/nb/toolchain.md", "docs/labs/pr12/nb/execution-path.md", "docs/labs/pr12/nb/evidence/hit-54-d1-clean.log"],
    "NB-H1": ["docs/labs/pr12/nb/human-gates"],
    "NB-REPORT": ["docs/labs/pr12/nb/reports", "docs/labs/pr12/nb/evidence/screenshots/INSTRUCTIONS.md"],
    "SHARED": ["docs/labs/pr12/context-lock.json", "docs/labs/pr12/source-files.json"],
}
task_log = {
    "NB-R1": "docs/labs/pr12/nb/evidence/hit-49-r1-validation-v4.log",
    "NB-R2": "docs/labs/pr12/nb/evidence/hit-50-r2-validation.log",
    "NB-C1": "docs/labs/pr12/nb/evidence/hit-51-c1-clean.log",
    "NB-C2": "docs/labs/pr12/nb/evidence/hit-52-c2-local-clean.log",
    "NB-C3": "docs/labs/pr12/nb/evidence/hit-53-baseline-v2.log",
    "NB-D1": "docs/labs/pr12/nb/evidence/hit-54-d1-clean.log",
    "NB-H1": "docs/labs/pr12/nb/human-gates/index.md",
    "NB-REPORT": "docs/labs/pr12/nb/evidence/index.json",
}
crosswalk = []
for criterion, tasks in nb_crosswalk.items():
    lab = criterion.split("/")[0]
    artifacts = sorted({path for task in tasks for path in task_artifacts.get(task, [])})
    sha_values = {task: task_sha.get(task, "UNKNOWN") for task in tasks}
    crosswalk.append({
        "criterion": criterion,
        "tasks": tasks,
        "status": "PARTIAL_HUMAN_PENDING" if criterion in human_pending else "PASS_CONTENT",
        "source": "docs/labs/pr12/references/METHODICS_MATRIX.md#" + criterion,
        "artifacts": artifacts,
        "check_or_log": sorted({task_log.get(task, "UNKNOWN") for task in tasks}),
        "full_sha_by_task": sha_values,
        "ci_or_local": "human gate pending" if criterion in human_pending else "actual local evidence; CI links in submission-record.json",
        "human_gate": criterion in human_pending,
    })
(NB / "criteria-crosswalk.json").write_text(json.dumps({"student": "nb", "count": len(crosswalk), "entries": crosswalk}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

entries = [
    ("0302-pz01-01", "Требование, задача и запрос на слияние", "Open https://hitpipe.atlassian.net/browse/HIT-50 and the eventual HIT-50 PR; until Lead creates it, show the branch and record PR UNKNOWN.", "git rev-parse HEAD; git diff --check", "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "task key, author, branch and meaningful requirement diff", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-01.png"),
    ("0302-pz01-02", "Автоматические проверки изменения требований", "Open the eventual Actions run for work/HIT-50-nb-refinement; currently use docs/labs/pr12/nb/evidence/hit-50-r2-validation.log.", "git diff --check; python3 -m json.tool docs/labs/pr12/nb/requirements-trace.json", "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "SHA, commands, validation output and exit 0; no nonexistent CI", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-02.png"),
    ("0302-pz01-03", "Оппонирование другого проекта и принятое решение", "Open docs/labs/pr12/nb/human-gates/external-opposition.md after a real teacher-assigned discussion; currently NOT_CONDUCTED.", "Human records discussion URL and date", "Human-provided full SHA; currently UNKNOWN", "external project, participant, date, real objection/decision", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-03.png"),
    ("0302-pz02-01", "Локальный прогон тестов собственного модуля", "Open a clean checkout at correction SHA df1a7d58179148abfda8e71862654f5ef7fcdbc7.", "cd services/intake && go test -count=1 ./... && go test -race -count=1 ./...", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "git rev-parse HEAD, named regression tests, PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-01.png"),
    ("0302-pz02-02", "Тесты сдаваемой версии в CI", "Open the correction Actions run linked from docs/labs/pr12/nb/handoff.md after push; do not use the original pre-fix run as correction evidence.", "PR12_RACE=1 python3 tools/pr12/workspace.py check intake", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; correction run URL recorded after push", "SHA, job, command, named regressions and success", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-02.png"),
    ("0302-pz02-03", "Покрытие собственного модуля и разбор пробела", "Open a clean checkout at original baseline SHA 2186a00ca9c441645c99f82fd45b34241864ac6a and write a new profile under ignored .pr12-build.", "cd services/intake && mkdir -p ../../.pr12-build && go test -coverprofile=../../.pr12-build/intake.cover ./...; go tool cover -func=../../.pr12-build/intake.cover", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "domain 87.7%, application 83.3%, CLI/adapters 0% unit profile and explicit no-pprof limitation", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-03.png"),
    ("0302-pz02-04", "Обнаружение одного из засеянных дефектов", "Create an isolated copy from correction SHA df1a7d58179148abfda8e71862654f5ef7fcdbc7; apply the selected patch with git apply; capture compile exit 0, assertion exit 1, then discard the isolated copy.", "git clone --no-local . /tmp/hitpipe-pr12-mutant; cd /tmp/hitpipe-pr12-mutant; git checkout --detach df1a7d58179148abfda8e71862654f5ef7fcdbc7; git apply /absolute/path/to/docs/labs/pr12/nb/evidence/mutations/M01.patch; cd services/intake; go test -run '^$' ./...; go test -count=1 ./...", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; M01 patch/log digest recorded in evidence", "isolated copy path, patch apply, compile exit 0, assertion FAIL exit 1, restoration/removal", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-04.png"),
    ("0302-pz02-05", "Успешный прогон после полного восстановления кода", "Return to clean correction SHA df1a7d58179148abfda8e71862654f5ef7fcdbc7.", "git status --short; PR12_RACE=1 python3 tools/pr12/workspace.py check intake; go test -race -count=1 ./...", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "no mutations, new regressions PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-05.png"),
    ("0104-pz01-01", "Собственная часть исполняемого каркаса", "Open the correction source at https://github.com/hitpipe/hitpipe/blob/df1a7d58179148abfda8e71862654f5ef7fcdbc7/services/intake/cmd/intake-contractcheck/main.go.", "git show --stat --oneline df1a7d58179148abfda8e71862654f5ef7fcdbc7", "df1a7d58179148abfda8e71862654f5ef7fcdbc7", "path, own CLI composition/diagnostics, branch and candidate status", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-01.png"),
    ("0104-pz01-02", "Подготовка компонента в двух режимах", "Use the owned wrapper route in the correction checkout.", "python3 tools/pr12/workspace.py smoke intake --require; python3 tools/pr12/workspace.py inspect intake --mode release", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "commands, dev/release outputs, build mode and safe fixture scope", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-02.png"),
    ("0104-pz01-03", "Запуск поставки вне исходного кода и IDE", "Create a new temporary distribution from the clean correction checkout; copy the release binary and synthetic verified.json to it, then leave the checkout.", "dist=$(mktemp -d /tmp/hitpipe-pr12-dist.XXXXXX); cp .pr12-build/intake/release/intake-contractcheck \"$dist/\"; cp services/intake/testdata/verified.json \"$dist/\"; cd \"$dist\"; ./intake-contractcheck --version; ./intake-contractcheck --fixture verified.json --format json", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "pwd outside checkout, release binary, version, PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-03.png"),
    ("0104-pz01-04", "Штатный запуск компонента", "Open the clean correction wrapper run and synthetic fixture output.", "python3 tools/pr12/workspace.py smoke intake --require", "df1a7d58179148abfda8e71862654f5ef7fcdbc7", "safe status/reason/version, fixture scope and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-04.png"),
    ("0104-pz01-05", "Неверная конфигурация", "Use the existing CLI representation parameter against the synthetic verified.json fixture.", "./intake-contractcheck --fixture verified.json --format text; ./intake-contractcheck --fixture verified.json --format json; ./intake-contractcheck --fixture verified.json --format invalid (expected exit 2)", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "text/json representations and safe format_invalid with exit 2", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-05.png"),
    ("0104-pz01-06", "Независимое воспроизведение части другого студента", "Open docs/labs/pr12/nb/human-gates/peer-scaffold-out.md after another student runs it; currently NOT_CONDUCTED.", "Human clean-clone command and test command", "Human-provided full SHA; currently UNKNOWN", "reviewer, clean environment, commands, result, remarks and retest", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-06.png"),
    ("0104-pz01-07", "Отрицательная локальная проверка автоматического gate", "Open docs/labs/pr12/nb/evidence/hit-53-ci-sensitivity.log and the correction CI result.", "PR12_RACE=1 python3 tools/pr12/workspace.py check intake", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; correction CI run 39273968129", "controlled failure exit 1 then restored exit 0; correction CI success", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-07.png"),
    ("0104-pz02-01", "Публичный контракт и существенная реализация", "Open the correction checkout at df1a7d58179148abfda8e71862654f5ef7fcdbc7: verifier.go and services/intake/docs/module-contract.md.", "git show --stat --oneline df1a7d58179148abfda8e71862654f5ef7fcdbc7", "df1a7d58179148abfda8e71862654f5ef7fcdbc7; Go go1.27.2", "Verify, checked timestamp parsing, exact bytes, bounded Outcome and no business side effect", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-01.png"),
    ("0104-pz02-02", "Документация модуля по стандарту стека", "Open services/intake/docs/module-contract.md in the final report checkout; first verify the path exists at the source SHA recorded in handoff.", "git cat-file -e <SHA>:services/intake/docs/module-contract.md; sed -n '1,220p' services/intake/docs/module-contract.md", "final report source SHA recorded in handoff", "purpose, inputs, result, errors, dependencies, example and limits", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-02.png"),
    ("0104-pz02-03", "Один потребитель и две подключаемые реализации", "Open authenticatewebhook Consumer, composition root and tests; verify the executed pair is FileResolver+FixedClock and MapSecrets+FixedClock.", "cd services/intake && go test -count=1 ./internal/application/authenticatewebhook", "correction/report source SHA recorded in handoff; Go go1.27.2", "same consumer, two executed implementations, normal/error/boundary; SystemClock marked not executed here", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-03.png"),
    ("0104-pz02-04", "Модульная карта с легендой и статусами", "Open docs/labs/pr12/nb/module-map.md at the final report source SHA recorded in handoff; first verify the path exists.", "git cat-file -e <SHA>:docs/labs/pr12/nb/module-map.md; git show <SHA>:docs/labs/pr12/nb/module-map.md", "final report source SHA recorded in handoff", "responsibilities, exact paths, imports, statuses, author and A→B legend", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-04.png"),
    ("0104-pz02-05", "Проверки с обеими реализациями", "Open the updated C2 evidence log and named application test source.", "cd services/intake && go test -count=1 ./internal/application/authenticatewebhook", "correction source SHA recorded in handoff; Go go1.27.2", "both executed implementations and normal/boundary/error PASS", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-05.png"),
    ("0104-pz02-06", "Реальная взаимная проверка модуля другого студента", "Open docs/labs/pr12/nb/human-gates/peer-module-out.md after an independent student completes it; currently NOT_CONDUCTED.", "Human reviewer command and retest", "Human-provided full SHA; currently UNKNOWN", "reviewer, date, other module, commit, build/normal/error/boundary/substitution", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-06.png"),
]

lines = ["# Human screenshot instructions", "", "Status: CONTENT_READY_IMAGES_PENDING. Exactly 21 placeholders are retained; no PNG files were captured or generated by the agent.", "", "Common redaction: hide tokens, passwords, cookies, personal email, customer/provider data and unrelated windows. Preserve relevant command, cwd, version, SHA, test names, outcome and exit code. Insert each real PNG manually at the named report slot.", ""]
for name, caption, where, command, sha_value, visible, destination in entries:
    lines += [f"## {name} — {caption}", "", "Status: PLACEHOLDER_ONLY.", f"1. Open/execute: {where}", f"2. CWD: `{ROOT}`; command: `{command}`", f"3. Verify SHA/version: `{sha_value}`", f"4. Must be visible: {visible}", f"5. Save PNG as `{destination}` and insert it at the matching figure slot with caption `Рисунок — {caption}`.", "6. Hide secrets, cookies, personal data and unrelated windows; never fabricate output.", ""]
(SCREEN / "INSTRUCTIONS.md").write_text("\n".join(lines), encoding="utf-8")

report_files = sorted((NB / "reports").glob("*.docx"))
evidence_files = sorted(p for p in EVIDENCE.rglob("*") if p.is_file() and p.name != "index.json")
index = {"status": "CONTENT_READY_IMAGES_PENDING", "student": "Никита Будыкин", "repository": "hitpipe/hitpipe", "baseline": "94b78199c202a31bcb63326ae7f2a4748aa29360", "reports": [{"path": rel(p), "sha256": sha(p), "bytes": p.stat().st_size} for p in report_files], "evidence": [{"path": rel(p), "sha256": sha(p), "bytes": p.stat().st_size} for p in evidence_files], "ci": [{"task": "HIT-50", "sha": "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262488242", "status": "success"}, {"task": "HIT-53-original", "sha": "2186a00ca9c441645c99f82fd45b34241864ac6a", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262304491", "status": "success"}, {"task": "HIT-53-correction", "sha": "df1a7d58179148abfda8e71862654f5ef7fcdbc7", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39274386817", "job": "118134753907", "status": "success"}, {"task": "HIT-54", "sha": "a74a5e1a8ada1147b5755503fd01fd458b8a4706", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262425665", "status": "success"}, {"task": "HIT-56-original", "sha": "47807726acbcbfcbae048c6afc2bcb062df192cb", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39263470537", "job": "118096896666", "status": "success"}, {"task": "HIT-56-correction", "sha": "9f08df6106ef921d65cb665de6a348b00d2138e8", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39275300874", "job": "118137942247", "status": "success"}], "human_gates": "docs/labs/pr12/nb/human-gates/index.md; all external gates pending", "screenshots": {"count": 21, "directory": rel(SCREEN), "policy": "placeholders only; no PNG generated"}}
(EVIDENCE / "index.json").write_text(json.dumps(index, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

submission = {"status": "CONTENT_READY_IMAGES_PENDING", "student": "Никита Будыкин", "feature": "HIT-15", "repository": "hitpipe/hitpipe", "candidate_code_sha": "df1a7d58179148abfda8e71862654f5ef7fcdbc7", "report_branch": "work/HIT-56-nb-reports", "report_commit": "[record after final report commit and human tags]", "tags": {"0104-pz01-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0104-pz02-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0302-pz01-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0302-pz02-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE"}, "pull_requests": {"HIT-51": "https://github.com/hitpipe/hitpipe/pull/31", "HIT-52": "https://github.com/hitpipe/hitpipe/pull/32", "HIT-53": "https://github.com/hitpipe/hitpipe/pull/33", "HIT-56": "https://github.com/hitpipe/hitpipe/pull/34"}, "ci": {"HIT-50": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262488242", "conclusion": "success"}, "HIT-53-original": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262304491", "conclusion": "success"}, "HIT-53-correction": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39274386817", "job": "118134753907", "conclusion": "success"}, "HIT-54": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262425665", "conclusion": "success"}, "HIT-56-original": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39263470537", "conclusion": "success"}, "HIT-56-correction": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39275300874", "job": "118137942247", "conclusion": "success"}}, "human_gates": "PENDING: interview, blind clarity, reciprocal scaffold/module checks, opposition, review, merge, screenshots, defense", "note": "Not formal acceptance or submission-ready until humans complete gates and create immutable tags."}
(NB / "submission-record.json").write_text(json.dumps(submission, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"metadata: {len(crosswalk)} criteria, {len(entries)} screenshot instructions, {len(evidence_files)} evidence files")
