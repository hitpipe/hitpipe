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
crosswalk = []
for criterion, tasks in nb_crosswalk.items():
    lab = criterion.split("/")[0]
    crosswalk.append({"criterion": criterion, "tasks": tasks, "status": "PARTIAL_HUMAN_PENDING" if criterion in human_pending else "PASS_CONTENT", "artifacts": artifact_by_lab[lab], "human_gate": criterion in human_pending})
(NB / "criteria-crosswalk.json").write_text(json.dumps({"student": "nb", "count": len(crosswalk), "entries": crosswalk}, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

entries = [
    ("0302-pz01-01", "Требование, задача и запрос на слияние", "Open https://hitpipe.atlassian.net/browse/HIT-50 and the eventual HIT-50 PR; until Lead creates it, show the branch and record PR UNKNOWN.", "git rev-parse HEAD; git diff --check", "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "task key, author, branch and meaningful requirement diff", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-01.png"),
    ("0302-pz01-02", "Автоматические проверки изменения требований", "Open the eventual Actions run for work/HIT-50-nb-refinement; currently use docs/labs/pr12/nb/evidence/hit-50-r2-validation.log.", "git diff --check; python3 -m json.tool docs/labs/pr12/nb/requirements-trace.json", "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "SHA, commands, validation output and exit 0; no nonexistent CI", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-02.png"),
    ("0302-pz01-03", "Оппонирование другого проекта и принятое решение", "Open docs/labs/pr12/nb/human-gates/external-opposition.md after a real teacher-assigned discussion; currently NOT_CONDUCTED.", "Human records discussion URL and date", "Human-provided full SHA; currently UNKNOWN", "external project, participant, date, real objection/decision", "docs/labs/pr12/nb/evidence/screenshots/0302-pz01-03.png"),
    ("0302-pz02-01", "Локальный прогон тестов собственного модуля", "Open a clean checkout at candidate SHA 2186a00ca9c441645c99f82fd45b34241864ac6a.", "cd services/intake && go test -count=1 ./... && go test -race -count=1 ./...", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "git rev-parse HEAD, test names, PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-01.png"),
    ("0302-pz02-02", "Тесты сдаваемой версии в CI", "Open https://github.com/hitpipe/hitpipe/actions/runs/39262304491 and expand the check job.", "PR12_RACE=1 python3 tools/pr12/workspace.py check intake", "2186a00ca9c441645c99f82fd45b34241864ac6a; run 39262304491", "SHA, job, command, named tests and success", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-02.png"),
    ("0302-pz02-03", "Покрытие собственного модуля и разбор пробела", "Open docs/labs/pr12/nb/evidence/hit-53-baseline-v2.cover.", "cd services/intake && go test -coverprofile=../../docs/labs/pr12/nb/evidence/hit-53-baseline-v2.cover ./...; go tool cover -func=../../docs/labs/pr12/nb/evidence/hit-53-baseline-v2.cover", "835bbb1bc565e9b2786d5ed1a2cac166f816f7a8; Go go1.27.2", "domain 87.7%, application 83.3%, CLI/adapters 0% unit profile and gap analysis", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-03.png"),
    ("0302-pz02-04", "Обнаружение одного из засеянных дефектов", "Open evidence/mutations/M01.patch and M01.log; source is a temporary copy.", "go test -run '^$' ./...; go test -count=1 ./...", "Baseline 835bbb1bc565e9b2786d5ed1a2cac166f816f7a8; M01 digest in log", "compiling mutation, violated raw-byte rule, named FAIL and exit 1", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-04.png"),
    ("0302-pz02-05", "Успешный прогон после полного восстановления кода", "Return to clean candidate SHA 2186a00ca9c441645c99f82fd45b34241864ac6a.", "git status --short; PR12_RACE=1 python3 tools/pr12/workspace.py check intake; go test -race -count=1 ./...", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "no mutations, tests PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0302-pz02-05.png"),
    ("0104-pz01-01", "Собственная часть исполняемого каркаса", "Open https://github.com/hitpipe/hitpipe/blob/2186a00ca9c441645c99f82fd45b34241864ac6a/services/intake/cmd/intake-contractcheck/main.go.", "git show --stat --oneline 2186a00ca9c441645c99f82fd45b34241864ac6a", "2186a00ca9c441645c99f82fd45b34241864ac6a", "path, own CLI composition/diagnostics, branch and candidate status", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-01.png"),
    ("0104-pz01-02", "Подготовка компонента в двух режимах", "Use the owned wrapper route in the report workspace.", "python3 tools/pr12/workspace.py smoke intake --require; python3 tools/pr12/workspace.py inspect intake --mode release", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "commands, dev/release outputs, build mode and safe fixture scope", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-02.png"),
    ("0104-pz01-03", "Запуск поставки вне исходного кода и IDE", "Open the outside-checkout temporary directory shown in hit-53-baseline-v2.log.", "intake-contractcheck --fixture positive_fixture.json --format json", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "pwd outside checkout, release binary, version, PASS and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-03.png"),
    ("0104-pz01-04", "Штатный запуск компонента", "Open the clean candidate wrapper run and synthetic fixture output.", "python3 tools/pr12/workspace.py smoke intake --require", "2186a00ca9c441645c99f82fd45b34241864ac6a", "safe status/reason/version, fixture scope and exit 0", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-04.png"),
    ("0104-pz01-05", "Неверная конфигурация", "This bounded component has no runtime configuration input. Do not invent an invalid-config run; record NOT_APPLICABLE if Lead requires this slot.", "No valid command; human records NOT_APPLICABLE", "Candidate SHA 2186a00ca9c441645c99f82fd45b34241864ac6a", "explicit scope gap, not fabricated PASS", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-05.png"),
    ("0104-pz01-06", "Независимое воспроизведение части другого студента", "Open docs/labs/pr12/nb/human-gates/peer-scaffold-out.md after another student runs it; currently NOT_CONDUCTED.", "Human clean-clone command and test command", "Human-provided full SHA; currently UNKNOWN", "reviewer, clean environment, commands, result, remarks and retest", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-06.png"),
    ("0104-pz01-07", "Отрицательная локальная проверка автоматического gate", "Open docs/labs/pr12/nb/evidence/hit-53-ci-sensitivity.log.", "PR12_RACE=1 python3 tools/pr12/workspace.py check intake", "2186a00ca9c441645c99f82fd45b34241864ac6a; Go go1.27.2", "controlled failure exit 1 then restored exit 0", "docs/labs/pr12/nb/evidence/screenshots/0104-pz01-07.png"),
    ("0104-pz02-01", "Публичный контракт и существенная реализация", "Open https://github.com/hitpipe/hitpipe/blob/2a20fb6a21fe762f2cf37ded433c99bf5390be81/services/intake/internal/domain/stripesignature/verifier.go and module-contract.md.", "git show 2a20fb6a21fe762f2cf37ded433c99bf5390be81 --stat", "2a20fb6a21fe762f2cf37ded433c99bf5390be81; Go go1.27.2", "Verify, exact bytes, bounded Outcome and no business side effect", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-01.png"),
    ("0104-pz02-02", "Документация модуля по стандарту стека", "Open services/intake/docs/module-contract.md.", "sed -n '1,220p' services/intake/docs/module-contract.md", "2a20fb6a21fe762f2cf37ded433c99bf5390be81", "purpose, inputs, result, errors, dependencies, example and limits", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-02.png"),
    ("0104-pz02-03", "Один потребитель и две подключаемые реализации", "Open authenticatewebhook Consumer and composition root; inspect tests.", "cd services/intake && go test -count=1 ./...", "2a20fb6a21fe762f2cf37ded433c99bf5390be81; Go go1.27.2", "same consumer, file-backed and in-memory/fixed implementations, normal/error/boundary", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-03.png"),
    ("0104-pz02-04", "Модульная карта с легендой и статусами", "Open docs/labs/pr12/nb/module-map.md at the candidate revision.", "git show 2a20fb6a21fe762f2cf37ded433c99bf5390be81:docs/labs/pr12/nb/module-map.md", "2a20fb6a21fe762f2cf37ded433c99bf5390be81", "responsibilities, paths, imports, statuses, author and A→B legend", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-04.png"),
    ("0104-pz02-05", "Проверки с обеими реализациями", "Open hit-52-c2-local-clean.log and the named application test source.", "cd services/intake && go test -count=1 ./...", "2a20fb6a21fe762f2cf37ded433c99bf5390be81; Go go1.27.2", "both implementations and normal/boundary/error PASS", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-05.png"),
    ("0104-pz02-06", "Реальная взаимная проверка модуля другого студента", "Open docs/labs/pr12/nb/human-gates/peer-module-out.md after an independent student completes it; currently NOT_CONDUCTED.", "Human reviewer command and retest", "Human-provided full SHA; currently UNKNOWN", "reviewer, date, other module, commit, build/normal/error/boundary/substitution", "docs/labs/pr12/nb/evidence/screenshots/0104-pz02-06.png"),
]

lines = ["# Human screenshot instructions", "", "Status: CONTENT_READY_IMAGES_PENDING. Exactly 21 placeholders are retained; no PNG files were captured or generated by the agent.", "", "Common redaction: hide tokens, passwords, cookies, personal email, customer/provider data and unrelated windows. Preserve relevant command, cwd, version, SHA, test names, outcome and exit code. Insert each real PNG manually at the named report slot.", ""]
for name, caption, where, command, sha_value, visible, destination in entries:
    lines += [f"## {name} — {caption}", "", "Status: PLACEHOLDER_ONLY.", f"1. Open/execute: {where}", f"2. CWD: `{ROOT}`; command: `{command}`", f"3. Verify SHA/version: `{sha_value}`", f"4. Must be visible: {visible}", f"5. Save PNG as `{destination}` and insert it at the matching figure slot with caption `Рисунок — {caption}`.", "6. Hide secrets, cookies, personal data and unrelated windows; never fabricate output.", ""]
(SCREEN / "INSTRUCTIONS.md").write_text("\n".join(lines), encoding="utf-8")

report_files = sorted((NB / "reports").glob("*.docx"))
evidence_files = sorted(p for p in EVIDENCE.rglob("*") if p.is_file() and p.name != "index.json")
index = {"status": "CONTENT_READY_IMAGES_PENDING", "student": "Никита Будыкин", "repository": "hitpipe/hitpipe", "baseline": "94b78199c202a31bcb63326ae7f2a4748aa29360", "reports": [{"path": rel(p), "sha256": sha(p), "bytes": p.stat().st_size} for p in report_files], "evidence": [{"path": rel(p), "sha256": sha(p), "bytes": p.stat().st_size} for p in evidence_files], "ci": [{"task": "HIT-50", "sha": "35b4c247319cf3bf0c8d8288d109c996fc721d0b", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262488242", "status": "success"}, {"task": "HIT-53", "sha": "2186a00ca9c441645c99f82fd45b34241864ac6a", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262304491", "status": "success"}, {"task": "HIT-54", "sha": "a74a5e1a8ada1147b5755503fd01fd458b8a4706", "url": "https://github.com/hitpipe/hitpipe/actions/runs/39262425665", "status": "success"}], "human_gates": "docs/labs/pr12/nb/human-gates/index.md; all external gates pending", "screenshots": {"count": 21, "directory": rel(SCREEN), "policy": "placeholders only; no PNG generated"}}
(EVIDENCE / "index.json").write_text(json.dumps(index, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

submission = {"status": "CONTENT_READY_IMAGES_PENDING", "student": "Никита Будыкин", "feature": "HIT-15", "repository": "hitpipe/hitpipe", "candidate_code_sha": "2186a00ca9c441645c99f82fd45b34241864ac6a", "report_branch": "work/HIT-56-nb-reports", "report_commit": "[record after final report commit]", "tags": {"0104-pz01-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0104-pz02-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0302-pz01-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE", "0302-pz02-nb-v1": "NOT_CREATED_HUMAN_AFTER_MERGE"}, "pull_requests": {"HIT-51": "NOT_CREATED", "HIT-52": "NOT_CREATED", "HIT-53": "NOT_CREATED"}, "ci": {"HIT-50": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262488242", "conclusion": "success"}, "HIT-53": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262304491", "conclusion": "success"}, "HIT-54": {"run": "https://github.com/hitpipe/hitpipe/actions/runs/39262425665", "conclusion": "success"}}, "human_gates": "PENDING: interview, blind clarity, reciprocal scaffold/module checks, opposition, review, merge, screenshots, defense", "note": "Not formal acceptance or submission-ready until humans complete gates and create immutable tags."}
(NB / "submission-record.json").write_text(json.dumps(submission, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
print(f"metadata: {len(crosswalk)} criteria, {len(entries)} screenshot instructions, {len(evidence_files)} evidence files")
