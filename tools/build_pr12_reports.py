from pathlib import Path
import re
from docx import Document

ROOT = Path(__file__).resolve().parents[1]
TEMPLATES = ROOT / "docs/labs/pr12/templates"
OUT = ROOT / "docs/labs/pr12/nb/reports"
OUT.mkdir(parents=True, exist_ok=True)

COMMON = {
    "student_code": "nb",
    "student_name": "Никита Будыкин",
    "group": "[группа — заполняет человек]",
    "component_name": "Intake - Source Integrations",
    "module_path": "services/intake/internal/domain/stripesignature",
    "repository_url": "https://github.com/hitpipe/hitpipe",
    "submission_commit": "d164aebd5b746151da41717f0ffb483eb9b39a63",
    "submission_tag": "[human after merge: immutable tag not created]",
    "branch": "work/HIT-56-nb-reports",
    "date": "2026-10-19 Europe/Moscow",
    "environment": "macOS darwin/arm64; bundled Go go1.27.2; Python 3.12 runtime",
    "tool_version": "Go go1.27.2; Python 3.12; Git 2.x",
    "lockfiles": "services/intake/go.mod; services/intake/go.sum (empty, stdlib-only)",
    "student,date,commit": "[human reviewer, date, full SHA — not conducted]",
    "other_student_name": "[human reviewer not assigned]",
    "peer_reviewer_name": "[human reviewer not assigned]",
    "peer_commit": "[human commit not available]",
    "peer_runbook": "docs/labs/pr12/nb/human-gates/peer-scaffold-out.md (NOT_CONDUCTED)",
    "protocol_log": "docs/labs/pr12/nb/human-gates/peer-scaffold-out.md (NOT_CONDUCTED)",
    "protocol_url_log": "docs/labs/pr12/nb/human-gates/peer-module-out.md (NOT_CONDUCTED)",
    "module_peer_review_protocol_url_or_path": "docs/labs/pr12/nb/human-gates/peer-module-out.md (NOT_CONDUCTED)",
    "module_peer_review_log_path": "docs/labs/pr12/nb/human-gates/peer-module-out.md (NOT_CONDUCTED)",
    "review_url": "[human review URL not created]",
    "merge_commit": "[human merge SHA not available]",
    "scaffold_pr_url": "https://github.com/hitpipe/hitpipe/pull/39",
    "requirements_pr_url": "https://github.com/hitpipe/hitpipe/pull/38",
    "scaffold_pipeline_url": "https://github.com/hitpipe/hitpipe/actions/runs/39262425665 (success; historical D1 head)",
    "requirements_pipeline_url": "https://github.com/hitpipe/hitpipe/actions/runs/39262488242 (success; historical R2 head)",
    "test_pipeline_url": "https://github.com/hitpipe/hitpipe/actions/runs/39281597831 (C3 head d164aeb; native check queued at build time)",
    "external_project_name": "[teacher-assigned external project not provided]",
    "opposition_assignment": "[teacher/Lead assignment not provided]",
    "opposition_discussion_url": "docs/labs/pr12/nb/human-gates/external-opposition.md (NOT_CONDUCTED)",
    "opposition_protocol_url": "docs/labs/pr12/nb/human-gates/external-opposition.md (NOT_CONDUCTED)",
    "interview_protocol_url": "docs/labs/pr12/nb/interview.md",
    "interview_date_participants": "[human interview not conducted]",
    "representative_status": "UNKNOWN",
    "jira_task_url": "https://hitpipe.atlassian.net/browse/HIT-53",
    "requirement_id": "FR-SIG-01 / FR-SIG-02",
    "requirements_commit": "35b4c247319cf3bf0c8d8288d109c996fc721d0b",
    "requirements_job_name": "local document validation; PR #38; native CI run 39262488242 success",
    "meaningful_condition_and_command": "PR12_RACE=1 python3 tools/pr12/workspace.py check intake; a controlled failing test produced exit 1",
    "source_files": "verifier.go, authenticate.go, adapters, fixtures and cmd/intake-contractcheck",
    "dependency_preparation": "Go module resolution from services/intake; stdlib only; no live credentials",
    "actual_code_transformations": "gofmt, compile, HMAC-SHA256 over decimal timestamp + '.' + exact raw bytes, safe typed result",
    "runnable_result": "intake-contractcheck dev/release binary",
    "runtime_process": "offline synthetic fixture diagnostic outside checkout; no public route",
    "artifact_inspection_command": "go version -m .pr12-build/intake/release/intake-contractcheck",
    "same_scenario": "verified positive fixture and tampered fixture",
    "release_outside_source_directory": "temporary pr12-outside-checkout directory created by workspace.py",
    "release_run_command": "intake-contractcheck --fixture positive_fixture.json --format json",
    "valid_config_run_command": "python3 tools/pr12/workspace.py smoke intake --require",
    "invalid_config_run_command": "./intake-contractcheck --fixture verified.json --format invalid",
    "invalid_config_expected_exit": "2 (format_invalid)",
    "dev_build_command": "python3 tools/pr12/workspace.py smoke intake --require",
    "release_build_command": "python3 tools/pr12/workspace.py inspect intake --mode release",
    "dev_evidence": "PASS; docs/labs/pr12/nb/evidence/hit-54-d1-clean.log",
    "release_evidence": "PASS; docs/labs/pr12/nb/evidence/hit-54-d1-clean.log",
    "example_config": "synthetic fixture paths only; no secrets or environment credentials",
    "route": "workspace wrapper around services/intake; local and GitHub workflow route",
    "actual_status_stage": "content checks PASS; refreshed current-head and r2 native CI queued / human review pending",
    "cmd_commit_log": "see evidence/hit-53-baseline-v2.log and hit-54-d1-clean.log",
    "oracle": "safe bounded outcome and exact exit; no secret/body/header in diagnostics",
    "scaffold_contribution": "offline contractcheck component, build metadata, safe fixtures and CI-sensitive wrapper path",
    "result_boundary_environment": "candidate Intake slice; local macOS darwin/arm64, no provider credentials or production deployment",
    "documentation_standard": "Go doc comments plus services/intake/docs/module-contract.md",
    "implemented_rule": "raw-body Stripe HMAC verification with fixed absolute 300-second tolerance",
    "consumer": "authenticatewebhook.Consumer",
    "consumer_path": "services/intake/internal/application/authenticatewebhook",
    "consumer_source_path": "services/intake/internal/application/authenticatewebhook/authenticate.go",
    "consumer_hash_or_shared_callsite": "consumer calls domain Verify through the same application boundary; no consumer rewrite between adapters",
    "module_contract_path": "services/intake/docs/module-contract.md",
    "module_source_path": "services/intake/internal/domain/stripesignature/verifier.go",
    "module_start_line": "18",
    "module_end_line": "86",
    "module_documentation_url_or_path": "services/intake/docs/module-contract.md",
    "module_documentation_command": "sed -n '1,220p' services/intake/docs/module-contract.md",
    "composition_root": "cmd/intake-contractcheck composition root",
    "composition_root_path": "services/intake/cmd/intake-contractcheck/main.go",
    "replacement_demo_command": "go test -count=1 ./... (file-backed resolver and in-memory double use same Consumer)",
    "implementation_a": "FileResolver + FixedClock (executed application test)",
    "implementation_b": "MapSecrets + FixedClock (executed application test)",
    "replacement_demo_command": "go test -count=1 ./...",
    "both_implementations_test_command": "cd services/intake && go test -count=1 ./...",
    "module_map_url_or_path": "docs/labs/pr12/nb/module-map.md",
    "module_map_url_revision": "docs/labs/pr12/nb/module-map.md; verify at final correction SHA",
    "candidate_path": "services/intake/internal/domain/stripesignature",
    "candidate_input": "testdata/verified.json and tampered.json plus in-memory raw-byte fixtures",
    "candidate_observation": "same Consumer behavior with file-backed and in-memory dependency implementations",
    "personal_adr_path": "docs/labs/pr12/nb/module-adr.md",
    "adr_author": "Никита Будыкин",
    "stack_adr_url_revision": "personal ADR only; no canonical HP-ADR ID",
    "direction_1": "data handling",
    "direction_2": "trust and inter-component failure",
    "own_object_1": "raw bytes to future strict parser",
    "own_object_2": "untrusted ingress, secret and clock ports",
    "question_1": "how to preserve exact bytes without excess retention/copies",
    "question_2": "how to remain fail-closed without false receipt/2xx",
    "readiness_1": "candidate contract exercised; retention/parser not implemented",
    "readiness_2": "ports and bounded reasons exercised; transport/receipt not implemented",
    "decision_source": "module-contract.md and verifier.go; Candidate/In Review",
    "limits_a": "fixture adapter, no live secret store",
    "limits_b": "in-memory double, no live clock/provider",
    "limits": "no production ingress, secure store, durable receipt or financial decision",
    "import_check_command_log": "go vet ./... and workspace checks in evidence/hit-52-c2-local-clean.log",
    "fixtures_path_or_seed": "services/intake/testdata/verified.json; tampered.json; synthetic Go fixtures",
    "technique_1": "equivalence classes and boundary values",
    "technique_2": "decision/state table plus mutation testing",
    "coverage_command": "go test -coverprofile=evidence/hit-53-baseline-v2.cover ./...; go tool cover -func=...",
    "coverage_report_path_or_url": "docs/labs/pr12/nb/evidence/hit-53-baseline-v2.cover",
    "coverage_gap_location": "CLI/adapters have 0% unit statement profile; wrapper smoke covers behavior",
    "local_test_command": "cd services/intake && go test -count=1 ./... && go test -race -count=1 ./...",
    "restored_test_command": "PR12_RACE=1 python3 tools/pr12/workspace.py check intake; go test -race -count=1 ./...",
    "mutation_runbook_path": "docs/labs/pr12/nb/MUTATION_PROTOCOL.md",
    "mutation_branch": "temporary work/nb-mutation-r2-M0X branch from corrected C3 baseline d164aeb; never merged",
    "mutation_id": "M01 (r2; all M01-M05 are recorded)",
    "mutation_log_path": "docs/labs/pr12/nb/evidence/mutations/r2/M01.log",
    "mutation_test_command": "go test -count=1 ./... in the temporary copy",
    "natural_defect_count": "0 real defects found",
    "natural_defect_issues": "none; no issue/PR link",
    "seeded_count": "5",
    "detected_initial": "pending native r2 jobs",
    "missed_initial": "pending native r2 jobs",
    "detected_final": "pending native r2 jobs",
    "test_job_name": "temporary PR12 NB mutation r2 harness; native jobs queued",
    "local_rationale": "error precedence and boundary determinism remove implementation ambiguity",
    "rule_and_change": "compiled behavioral mutation; see individual patch and log",
    "analysis_or_not_applicable": "pending until native compile/test logs; hidden suite NOT_PROVIDED",
    "log": "docs/labs/pr12/nb/evidence/mutations/r2/M0X.log (created only after native job completion)",
    "resolution": "NOT_CONDUCTED; human gate required",
    "negative_gate_condition": "add controlled failing test that violates the expected wrapper condition",
    "negative_gate_log": "docs/labs/pr12/nb/evidence/hit-53-ci-sensitivity.log; fail exit 1, restored exit 0",
    "same_ci_check_command": "PR12_RACE=1 python3 tools/pr12/workspace.py check intake",
    "scaffold_source_path": "services/intake/cmd/intake-contractcheck/main.go",
    "scaffold_start_line": "1",
    "scaffold_end_line": "120",
    "peer_runbook": "docs/labs/pr12/nb/human-gates/peer-scaffold-out.md (NOT_CONDUCTED)",
    "clean_reproduction_protocol_url_or_path": "docs/labs/pr12/nb/human-gates/peer-scaffold-out.md",
    "clean_reproduction_log_path": "[human log not available]",
}

MUTATIONS = {
    "mutation_1_patch": "evidence/mutations/r2/M01.patch",
    "mutation_2_patch": "evidence/mutations/r2/M02.patch",
    "mutation_3_patch": "evidence/mutations/r2/M03.patch",
    "mutation_4_patch": "evidence/mutations/r2/M04.patch",
    "mutation_5_patch": "evidence/mutations/r2/M05.patch",
}

def all_paragraphs(doc):
    for p in doc.paragraphs:
        yield p
    for table in doc.tables:
        for row in table.rows:
            for cell in row.cells:
                for p in cell.paragraphs:
                    yield p

def set_para(p, text):
    if p.runs:
        p.runs[0].text = text
        for r in p.runs[1:]:
            r.text = ""
    else:
        p.add_run(text)

def set_cell(cell, text):
    cell.text = text

def set_row(table, row_index, values):
    while row_index >= len(table.rows):
        table.add_row()
    for col, value in enumerate(values):
        set_cell(table.cell(row_index, col), value)

def replace_tokens(doc, mapping):
    for p in all_paragraphs(doc):
        if "{{" not in p.text:
            continue
        text = p.text
        def repl(m):
            key = m.group(1)
            return mapping.get(key, f"[HUMAN INPUT REQUIRED: {key}]")
        set_para(p, re.sub(r"\{\{([^}]+)\}\}", repl, text))

def fill_common(doc, mapping):
    replace_tokens(doc, mapping)
    for p in doc.paragraphs:
        if p.text == "ОТЧЕТ \nЗАЩИЩЕН С ОЦЕНКОЙ":
            set_para(p, "ОТЧЕТ\nCONTENT_READY_IMAGES_PENDING")
        if p.text == "ПРЕПОДАВАТЕЛЬ":
            set_para(p, "ПРЕПОДАВАТЕЛЬ: [заполняет человек]")
        if p.text == "РАБОТУ ВЫПОЛНИЛ":
            set_para(p, "РАБОТУ ВЫПОЛНИЛ\nНикита Будыкин")
        if p.text.startswith("Шаблон адаптирован"):
            set_para(p, "Содержание отражает фактические локальные результаты кандидата и явные ограничения. Статус отчёта CONTENT_READY_IMAGES_PENDING: 21 реальных изображений ещё не вставлены человеком.")
        if p.text.startswith("Экспортная форма подготовлена"):
            set_para(p, "Форма сохранена с минимальными корректировками актуального пакета; отсутствие человеческих проверок, подписей, оценки и merge не скрывается.")
        if p.text.startswith("Каждый слот заполнить реальным снимком"):
            set_para(p, "Каждый слот содержит текстовую инструкцию для ручной вставки реального изображения. Агент изображения не создавал и не захватывал.")

def make_0302_pz01():
    d = Document(TEMPLATES / "0302-pz01-report-template.docx")
    m = dict(COMMON)
    m.update({
        "submission_tag": "[human after merge: 0302-pz01-nb-v1 not created]",
        "jira_task_url": "https://hitpipe.atlassian.net/browse/HIT-50",
        "requirement_id": "FR-SIG-01, FR-SIG-02, FR-SIG-03, FR-SIG-04",
        "requirements_commit": "35b4c247319cf3bf0c8d8288d109c996fc721d0b",
        "requirements_job_name": "PR #38; native CI run 39262488242 success",
        "requirements_pipeline_url": "https://github.com/hitpipe/hitpipe/pull/38",
        "opposition_discussion_url": "docs/labs/pr12/nb/human-gates/external-opposition.md (NOT_CONDUCTED)",
    })
    fill_common(d, m)
    overrides = {
        27: "Цель работы: уточнить проверяемый контракт Intake StripeSignature так, чтобы error precedence, границы свежести, unknown/version tokens и отсутствие бизнес-побочных эффектов имели один воспроизводимый смысл.",
        28: "Задача: HIT-49 baseline → HIT-50 refinement → кандидатные C1/C2/C3; исходы остаются Candidate/In Review до human review/merge.",
        35: "Пользователь и полезный сценарий: внешний Stripe webhook должен получить криптографическую проверку exact raw bytes до parsing; VERIFIED не означает ACCEPTED или paid.",
        36: "Постановка задачи: закрепить local refinement в requirement-change.md и ambiguity-decisions.md, не меняя canonical upstream.",
        37: "Источник сведений: docs/labs/pr12/nb/interview.md; реальная дата и участники [human interview not conducted]; статус представителя заказчика UNKNOWN.",
        40: "Подтверждённые условия: Candidate source pins, raw-body proof, fixed absolute 300 seconds, safe diagnostics. Предположения: provider event scope and retention remain unresolved. Открытые вопросы: interview, operator wording, future receipt/2xx policy; ответственные Lead/product.",
        41: "Связь с MVP: это только ранняя ingress proof boundary; strict parser, durable receipt, outbox, financial proof и transport остаются резервом.",
        42: "Общие материалы: context-lock/source-files, четыре методички, AGENTS.md, frozen Universal Master Prompt; source audit missing=0 mismatch=0.",
        47: "Исходные данные и подготовка состояния: synthetic bytes, headers, fixed clock and synthetic secret only; no live Stripe credentials or customer data.",
        50: "Причина и задача: precedence and duplicate-token ambiguity in R1. Ссылка на изменение: docs/labs/pr12/nb/requirement-change.md; branch work/HIT-50-nb-refinement; commit 35b4c247319cf3bf0c8d8288d109c996fc721d0b; PR https://github.com/hitpipe/hitpipe/pull/38.",
        51: "Применение DoD: git diff --check, python3 -m json.tool, marker audit; actual evidence in docs/labs/pr12/nb/evidence/hit-50-r2-validation.log; native CI run 39262488242 succeeded. Review/merge remain pending.",
        52: "Значимость и review: изменение влияет на domain and consumer behavior and is intended for separate human review; author does not self-approve.",
        53: "Результаты оппонирования ДРУГОГО проекта: [не проведено], назначение по кольцу преподавателя [не предоставлено], протокол docs/labs/pr12/nb/human-gates/external-opposition.md. Искусственные замечания не записывались.",
        62: "Строки замечаний остаются пустыми: внешний opposition и blind clarity check NOT_CONDUCTED. Это не PASS и не отсутствие реального замечания.",
        63: "Обязательный рисунок 3 остаётся текстовым placeholder с инструкцией; реальное обсуждение другого проекта должен провести человек.",
        68: "Blind clarity check передаётся другому студенту без answer key; фактические ответы, расхождения и retest пока NOT_CONDUCTED.",
        70: "Baseline 1.0 зафиксирована в requirements.md; цепочка HIT-50 → work/HIT-50-nb-refinement → 35b4c24 → local validation. Candidate implementation verification recorded separately in C1/C2/C3 evidence.",
        71: "Будущие проверки: human clarity, opposition, PR review/merge, CI for refinement branch and immutable tag.",
        76: "Общие материалы и результаты других участников: C1/C2/C3 candidate artifacts are linked as exact branches/SHA in handoff; no independent student or reviewer result is claimed.",
        78: "Вывод: refinement material is content-ready and locally validated, but human interview, clarity, opposition, PR review/merge and screenshot insertion remain open.",
    }
    for i,t in overrides.items(): set_para(d.paragraphs[i], t)
    t=d.tables[3]
    for r,c,v in [(1,1,"Никита Будыкин; группа [человек]"),(2,1,"HitPipe; Intake - Source Integrations"),(3,1,"StripeSignature; exact raw-body proof"),(4,1,"https://github.com/hitpipe/hitpipe"),(5,1,"[human after merge: 0302-pz01-nb-v1 not created]"),(7,1,"35b4c247319cf3bf0c8d8288d109c996fc721d0b"),(8,1,"CONTENT_READY_IMAGES_PENDING")]: set_cell(t.cell(r,c),v)
    rows=[
        ["FR-SIG-01","Exact raw bytes + HMAC-SHA256 v1","HP-BP-002 T/001","MVP proof; parser later","known vector/raw-byte tests","candidate implemented"],
        ["FR-SIG-02","strict headers and inclusive abs 300 s","HP-BP-002 T/001; HP-BP-007 T/010","MVP freshness","boundary/malformed tests","candidate implemented"],
        ["FR-SIG-03","safe fail-closed errors","HP-BP-007 T/005","MVP security","dependency and output tests","candidate implemented"],
        ["FR-SIG-04","no receipt/paid/2xx","HP-BP-002 T/001,T/002","future boundary","consumer/import checks","candidate boundary"],
        ["NFR-SIG-01","no secret/body/header logs","HP-BP-002 T/006,T/008; HP-DOC-NFR-001","security","safe output scan","candidate implemented"],
        ["NFR-SIG-02","deterministic bounded verification","HP-BP-007 T/010","clock/deadline","fixed clock and overflow regressions","candidate implemented"],
    ]
    t=d.tables[5]
    for i,row in enumerate(rows,1):
        set_row(t, i, row)
    interview_rows = [
        ["Interview with user/representative", "NOT_CONDUCTED by agent; docs/labs/pr12/nb/interview.md records unknown participants/date and does not invent answers", "Human gate real_interview remains pending"],
    ]
    for i,row in enumerate(interview_rows,1):
        set_row(d.tables[4], i, row)
    review_rows = [
        ["External opposition", "NOT_CONDUCTED; no artificial objection recorded", "Wait for teacher-assigned project and human discussion"],
        ["Clarity review", "NOT_CONDUCTED; answer key is not disclosed", "Human gate remains pending"],
    ]
    for i,row in enumerate(review_rows,1):
        set_row(d.tables[6], i, row)
    set_cell(d.tables[8].cell(1,0),"FR-SIG-02 boundary and conflict scenarios")
    set_cell(d.tables[8].cell(1,1),"299/300 pass; 301 reject; conflict rejects")
    set_cell(d.tables[8].cell(1,2),"NOT_CONDUCTED by other student")
    set_cell(d.tables[8].cell(1,3),"[human reviewer/date/SHA]")
    set_cell(d.tables[8].cell(1,4),"Human clarity check remains pending; technical oracle is recorded in results table")
    result_rows = [
        ["Requirements and traceability", "clean checkout; git diff --check; python3 -m json.tool docs/labs/pr12/nb/requirements-trace.json", "JSON and marker validation exit 0; source remains Candidate/In Review", "HIT-50 35b4c247319cf3bf0c8d8288d109c996fc721d0b; hit-50-r2-validation.log; PR #38; CI 39262488242", "No human interview or acceptance is claimed"],
        ["C1 numerical policy", "PR31 head 53687c138e266fda9104ef620edbc7d4252d8f6f; native intake workflow", "checked parse, positive clock and safe distance are present; three reviewer regressions pass on corrected chain", "PR #31; C1 head 53687c1; native CI after push", "C1/C2/C3 remain unmerged"],
        ["C3 verification and CLI", "PR33 head d164aebd5b746151da41717f0ffb483eb9b39a63; race wrapper, smoke and inspection", "tests/vet/race/smoke/inspection PASS; text/json/invalid exits 0/0/2", "PR #33; hit-53-correction-ci.json and hit-53-correction-cli.md", "No production deployment"],
        ["Freshness and HMAC evidence", "49 freshness cases plus four independent HMAC vectors on corrected policy", "all signed cases PASS; 27 report/evidence digests match the reviewed package", "review-checks.json; artifact-verification.json; evidence/index.json", "Reviewer probes are not author mutations"],
    ]
    for i,row in enumerate(result_rows, 1):
        set_row(d.tables[7], i, row)
    individual = [
        ["Технический результат", "Подготовлены requirements, traceability, refinement, exact source pins, C1/C2/C3 code chain, CI and corrected report inputs.", "HIT-49/50 PRs #37/#38; code PRs #31/#32/#33; report PR #34; evidence/index paths and actual CI links."],
        ["Человеческие результаты", "Interview, clarity, opposition, independent review/merge, signatures, screenshots and defense are not conducted by the agent.", "Human-gates files explicitly remain NOT_CONDUCTED/PENDING; status CONTENT_READY_IMAGES_PENDING."],
    ]
    for i,row in enumerate(individual, 1):
        set_row(d.tables[9], i, row)
    materials = {
        1: "МДК.03.02, ПЗ №1, Никита Будыкин",
        2: "[immutable tag after authorized human merge] → d164aebd5b746151da41717f0ffb483eb9b39a63 code snapshot; report head is recorded after correction commit",
        3: "requirements.md; requirement-change.md; requirements-trace.json; criteria-crosswalk.json; evidence/index.json; four DOCX; screenshot instructions",
        4: "HIT-49 PR #37; HIT-50 PR #38; HIT-51 PR #31; HIT-52 PR #32; HIT-53 PR #33; HIT-54 PR #39; HIT-55 PR #40; HIT-56 PR #34",
        5: "HIT-50 CI 39262488242; C3 correction/current chain links in evidence/index.json and handoff.md; no main merge",
        6: "External opposition NOT_CONDUCTED; human gate file retained",
        7: "Candidate/In Review; no formal Project OS admission, acceptance, tags, signatures or production effects",
    }
    for r,v in materials.items(): set_cell(d.tables[10].cell(r,1),v)
    out=OUT/"0302-pz01-nb.docx"; d.save(out); return out

def make_0302_pz02():
    d=Document(TEMPLATES/"0302-pz02-report-template.docx"); m=dict(COMMON)
    m.update({"submission_tag":"[human after merge: 0302-pz02-nb-v1 not created]","submission_commit":"d164aebd5b746151da41717f0ffb483eb9b39a63"})
    m.update(MUTATIONS)
    fill_common(d,m)
    overrides={
        30:"Цель работы: проверить собственный Intake module contract with equivalence/boundary classes, decision table, fixtures, local+CI checks, coverage analysis and five compiling behavioral mutations.",
        31:"Задача: HIT-53 NB-C3 validates StripeSignature and AuthenticateWebhook candidate without claiming production acceptance.",
        38:"Контракт модуля: exact raw-body HMAC-SHA256 with strict t/v1/v0 header policy, fixed abs 300-second tolerance and bounded Outcome.",
        39:"Доступная граница: domain + application + diagnostic CLI; no public webhook route, durable receipt, 2xx or payment decision.",
        40:"Зависимости: stdlib domain; application-owned SecretResolver and Clock; file/system adapters and in-memory/fixed doubles.",
        45:"Техника 1: equivalence classes and boundary values — normal, malformed, conflicting, v0-only, altered bytes, 299/300/301 past/future.",
        46:"Техника 2: decision/state table plus mutation testing — one fresh temporary copy per compiled mutant, same test set, full restoration.",
        49:"Полный набор тест-кейсов: verifier known vector/raw bytes/header/freshness/fail-closed; consumer normal/boundary/error/repeat/dependency failures; CLI positive/tampered/format checks.",
        50:"Чек-лист: compile, go test, vet, race, wrapper check, dev/release smoke, artifact metadata, negative wrapper sensitivity.",
        52:"Данные: services/intake/testdata/verified.json and tampered.json plus synthetic Go fixtures; all reproducible from clean clone without personal files or working keys.",
        53:"Подготовка и сброс состояния: fixed timestamp 1700000000, synthetic secret, temporary copies for mutations; baseline source restored after each run.",
        54:"Разработанные автоматические тесты: verifier/application named assertions plus CLI fixture checks; historical M01-M05 retained and a new versioned r2 M01-M05 series is queued on corrected C3 baseline.",
        55:"Изменение и включение в CI: .github/workflows/pr12-nb.yml runs PR12_RACE=1 and release inspection. C1/C2/C3 ordinary merges now carry the checked parsing, fail-closed clock and safe distance policy; correction CI links are in evidence/index.json.",
        62:"Среда: macOS darwin/arm64 author workspace; native CI Go go1.27.2. Historical baseline is 835bbb1bc565e9b2786d5ed1a2cac166f816f7a8; corrected C3 source/test baseline is d164aebd5b746151da41717f0ffb483eb9b39a63.",
        63:"Commands: cd services/intake && go test -coverprofile=.pr12-build/intake.cover ./...; go tool cover -func=.pr12-build/intake.cover; go vet ./...; go test -race -count=1 ./...; PR12_RACE=1 python3 tools/pr12/workspace.py check intake; smoke/inspect. Native correction checks are linked by full run IDs.",
        66:"Скрытый набор: не получен; hidden suite NOT_PROVIDED.",
        67:"Причины FAIL, BLOCKED и пропусков: original boundary probes failed before correction; the correction adds rejection and fail-closed tests. CLI/adapters have 0% unit profile but actual smoke; CPU/memory profiling is PLANNED, not run; human review/tag gates remain pending.",
        69:"Инструмент, команда, вид и область покрытия: Go statement coverage profile for services/intake; domain 87.7%, application 83.3%, CLI/adapters 0% unit profile. This is not CPU/memory profiling.",
        70:"Отчёт инструмента: docs/labs/pr12/nb/evidence/hit-53-baseline-v2.cover; new profiles go to ignored .pr12-build; analysis in docs/labs/pr12/nb/coverage-analysis.md.",
        73:"Полное выполнение тестов не доказывает provider behavior, production secret storage, transport, persistence, performance or human independence. CPU/memory pprof is planned and the unit coverage gap is explicit.",
        77:"Воспроизведение: docs/labs/pr12/nb/MUTATION_PROTOCOL.md; historical five patch/log pairs remain under evidence/mutations, and new versioned r2 pairs are under evidence/mutations/r2. Each r2 mutant is a separate temporary branch from corrected baseline d164aeb; none is merged.",
        81:"Итог r2 series на момент сборки отчёта: seeded 5; native compile/detection jobs queued; survivors/retests/final restoration evidence pending. Hidden suite NOT_PROVIDED.",
        83:"Состояние сдачи: corrected candidate source/test baseline d164aebd5b746151da41717f0ffb483eb9b39a63; all temporary mutants are outside the deliverable branches; native CI links are recorded as queued. Immutable tag not created; human review/merge required.",
        87:"Общие средства и результаты других участников: none claimed; human module peer check is NOT_CONDUCTED.",
        88:"Полученные замечания и решения: agent self-review fixed wrapper JSON marker mismatch and CLI composition boundary; no human remarks yet.",
        90:"Что обнаруживает разработанный набор: exact-body, MAC, timestamp, parser and consumer dependency regressions; evidence is local logs, mutation patches and GitHub Actions run.",
        91:"Что показали покрытие и эксперимент: full module statement coverage 55.8%; domain/application historical profiles 87.7%/83.3%; CLI/adapters unit profile 0%; r2 mutation outcome awaits native jobs.",
        92:"Границы доказательства и остаточные риски: no live provider/production route, no hidden suite, no human review/merge/opposition/screenshots/defense.",
    }
    for i,t in overrides.items(): set_para(d.paragraphs[i],t)
    t=d.tables[3]
    vals={1:"Никита Будыкин; группа [человек]",2:"HitPipe; Intake - Source Integrations",3:"https://github.com/hitpipe/hitpipe",4:"StripeSignature + AuthenticateWebhook",5:"[human after merge: 0302-pz02-nb-v1 not created]",7:"d164aebd5b746151da41717f0ffb483eb9b39a63",8:"CONTENT_READY_IMAGES_PENDING"}
    for r,v in vals.items(): set_cell(t.cell(r,1),v)
    req=[
        ["FR-SIG-01","exact raw bytes and MAC","HP-BP-002 T/001","unit + mutation"],
        ["FR-SIG-02","strict headers and inclusive 300-second boundary","HP-BP-002 T/001; HP-BP-007 T/010","unit + mutation"],
        ["FR-SIG-03","safe dependency failures","HP-BP-007 T/005","application + CLI"],
        ["FR-SIG-04","no business admission","HP-BP-002 T/001,T/002","module boundary"],
        ["NFR-SIG-01","no secret/body/header logs","HP-BP-002 T/006,T/008; HP-DOC-NFR-001","output scan + CI"],
        ["NFR-SIG-02","deterministic bounded verification","HP-BP-007 T/010","clock/deadline checks"],
    ]
    for i,row in enumerate(req,1):
        set_row(d.tables[4], i, row)
    cases=[
        ["T01","FR-SIG-01","known vector","verify exact bytes","VERIFIED","high-risk contract"],
        ["T02","FR-SIG-01","non-UTF8/reserialized","verify altered bytes","reject","raw-byte boundary"],
        ["T03","FR-SIG-02","299/300/301 past/future","freshness matrix","pass/pass/reject","inclusive boundary"],
        ["T04","FR-SIG-02","conflict/v0/unknown","strict parse","reject","fail closed"],
        ["T05","FR-SIG-03","missing secret/clock","consumer dependency errors","safe reject","security"],
        ["T06","FR-SIG-04","repeat event altered body","authenticate each call","reject","no shortcut"],
    ]
    for i,row in enumerate(cases,1):
        set_row(d.tables[5], i, row)
    risks=[
        ["R1","MAC bypass or normalized body causes false verification","low after tests; high consequence","heavy: false receipt","high","unit/mutation per commit; residual provider integration"],
        ["R2","timestamp direction/edge bug enables replay window","medium rule risk","heavy: replay","high","boundary tests + M03/M04 + overflow regressions; residual clock trust"],
        ["R3","secret/clock failure leaks or fails open","low with ports; heavy","heavy: false action/data leak","high","dependency tests + safe output scan"],
        ["R4","CLI/adapters unit profile gap","medium","significant diagnostic regression","medium","wrapper smoke each run; add unit coverage later"],
    ]
    for i,row in enumerate(risks,1):
        set_row(d.tables[6], i, row)
    runrows=[
        ["historical corrected-policy baseline","835bbb1; Go 1.27.2","tests/vet/race/wrapper PASS","PASS","hit-53-baseline-v2.log; historical series only"],
        ["current corrected C3 chain","d164aeb; PR12_RACE=1","current-head native checks queued; prior PASS is historical","QUEUED","PR #33; current native CI links in evidence/index.json"],
        ["current report package","a2eee52 source snapshot; refreshed files in working tree","content/evidence checks and report build PASS locally; current CI queued","QUEUED","PR #34; current report CI link in evidence/index.json"],
    ]
    for i,row in enumerate(runrows,1):
        set_row(d.tables[7], i, row)
    gaps=[["CLI/adapters unit coverage 0%","unit report omits those files","retain black-box smoke and list follow-up","wrapper smoke PASS"],["hidden suite unavailable","unknown external cases","mark NOT_PROVIDED","no retest possible"]]
    for i,row in enumerate(gaps,1):
        set_row(d.tables[8], i, row)
    states=[
        ["Исходный корректный код","d164aebd5b746151da41717f0ffb483eb9b39a63; Go 1.27.2","baseline is the corrected C3 source; current final gate queued","current C3 native CI link; restoration log pending"],
        ["Код с дефектом","work/nb-mutation-r2-M01..M05; one branch per mutant","compile/detection results queued","r2 patch files and five queued native runs"],
        ["Восстановленный correction code","d164aebd5b746151da41717f0ffb483eb9b39a63","restoration/final gate pending native mutation completion","final restoration evidence pending"],
    ]
    for i,row in enumerate(states,1):
        set_row(d.tables[9], i, row)
    mutrows=[
        ["M01 / mutations/r2/M01.patch","current exact raw bytes replaced by constant bytes","queued","raw-byte/known-vector assertions / expected exit 1","pending","survivor/retest pending","r2/M01.log after native run; queued run in evidence/index.json"],
        ["M02 / mutations/r2/M02.patch","current constant-time compare bypassed with expected-vs-expected","queued","altered-body/repeat assertions / expected exit 1","pending","survivor/retest pending","r2/M02.log after native run; queued run in evidence/index.json"],
        ["M03 / mutations/r2/M03.patch","current outsideTolerance ignores timestamps in the past","queued","past301 freshness assertion / expected exit 1","pending","survivor/retest pending","r2/M03.log after native run; queued run in evidence/index.json"],
        ["M04 / mutations/r2/M04.patch","current inclusive 300-second boundary becomes stale","queued","past300/future300 boundary / expected exit 1","pending","survivor/retest pending","r2/M04.log after native run; queued run in evidence/index.json"],
        ["M05 / mutations/r2/M05.patch","current conflicting timestamp comparison is inverted","queued","timestamp-conflict assertion / expected exit 1","pending","survivor/retest pending","r2/M05.log after native run; queued run in evidence/index.json"],
    ]
    for i,row in enumerate(mutrows,1):
        set_row(d.tables[10], i, row)
    set_row(d.tables[11], 1, ["Технический результат", "Named tests, coverage analysis and five versioned mutation patches are recorded; native mutation compile/detection and restoration jobs are queued.", "See tables above and evidence/index.json; hidden suite NOT_PROVIDED."])
    set_row(d.tables[11], 2, ["Человеческие gates", "Peer review, interview, opposition, screenshots, signatures and defense are not agent-run.", "Explicit NOT_CONDUCTED/PENDING human-gate files; status CONTENT_READY_IMAGES_PENDING."])
    summary={
        1:"МДК.03.02, ПЗ №2, Никита Будыкин",
        2:"[immutable tag after authorized human merge] → d164aebd5b746151da41717f0ffb483eb9b39a63 code snapshot",
        3:"HIT-51/52/53 code chain; FR-SIG-01..04; NFR-SIG-01/02; exact raw bytes, HMAC, freshness and safe failures",
        4:"verifier/application tests; verified.json/tampered.json; native CI wrapper; r2 mutation branches; hidden suite NOT_PROVIDED",
        5:"PR #33 current chain; current native checks queued; prior verified checks remain historical; PR #34 report package open",
        6:"coverage 55.8% full module statement; domain/application historical 87.7%/83.3%; CLI/adapters 0%; pprof PLANNED",
        7:"historical M01-M05 retained; current r2 M01-M05 patches and queued runs recorded; no mutation PASS or restoration result claimed yet; no mutant merged",
        8:"synthetic offline candidate only; no production route/provider credentials; no hidden suite or human acceptance",
    }
    for r,v in summary.items(): set_cell(d.tables[12].cell(r,1),v)
    out=OUT/"0302-pz02-nb.docx"; d.save(out); return out

def make_0104_pz01():
    d=Document(TEMPLATES/"0104-pz01-report-template.docx"); m=dict(COMMON)
    m.update({"submission_tag":"[human after merge: 0104-pz01-nb-v1 not created]","jira_task_url":"https://hitpipe.atlassian.net/browse/HIT-54","scaffold_pipeline_url":"https://github.com/hitpipe/hitpipe/pull/39; historical native run 39262425665 success","scaffold_source_path":"services/intake/cmd/intake-contractcheck/main.go","scaffold_start_line":"1","scaffold_end_line":"120"})
    fill_common(d,m)
    overrides={20:"Цель: воспроизводимо исполняемый Intake diagnostic component. Собственная часть: wrapper route, safe fixture CLI and release metadata inspection. Граница: offline candidate, no public endpoint.",24:"Задача HIT-54 → ветка work/HIT-54-nb-docs → PR https://github.com/hitpipe/hitpipe/pull/39 → native/local checks recorded → review/merge pending. Main code candidate is linked by exact SHA, not declared merged.",26:"Схема: Go source → module preparation → gofmt/compile/HMAC transformation → intake-contractcheck binary → fresh outside-checkout distribution.",28:"One verified positive and one tampered fixture were observed in dev/release. Release inspection records vcs.modified=false. No CPU/memory performance claim.",31:"Versions: native CI Go go1.27.2; Python 3.12 wrapper; Git. Synthetic fixture only. Evidence: docs/labs/pr12/nb/evidence/hit-54-d1-clean.log; source snapshot a74a5e1a8ada1147b5755503fd01fd458b8a4706.",33:"Independent clean reproduction by another student: NOT_CONDUCTED; protocol docs/labs/pr12/nb/human-gates/peer-scaffold-out.md.",35:"PR #39 is published; historical native run 39262425665 is success and current-head checks are linked from the package index. Human screenshot/review/merge pending.",37:"Individual result: toolchain, execution path and seven-position declaration are content-ready. Immutable tag 0104-pz01-nb-v1 is not created; personal defense not conducted."}
    for i,t in overrides.items(): set_para(d.paragraphs[i],t)
    t=d.tables[3]
    for r,v in {1:"Никита Будыкин; группа [человек]",3:"Intake - Source Integrations / services/intake",4:"[human after merge: 0104-pz01-nb-v1 not created]",5:"https://github.com/hitpipe/hitpipe",7:"CONTENT_READY_IMAGES_PENDING"}.items(): set_cell(t.cell(r,1),v)
    rows=[
        ["Сборка/подготовка к исполнению","Go go1.27.2","workspace validate; services/intake","PASS local; candidate"],
        ["Управление зависимостями","go.mod/go.sum","stdlib-only module","PASS; no live deps"],
        ["Тестирование","go test -count=1 ./...","domain/application tests","PASS local"],
        ["Статический анализ","gofmt; go vet; git diff --check","format/static hygiene","PASS local"],
        ["Динамическая диагностика","go test -race; PR12_RACE=1 wrapper","race and CI sensitivity","PASS restored; negative gate exit 1"],
        ["Покрытие","go test -coverprofile + go tool cover","statement coverage and gap analysis","PASS: domain 87.7%, application 83.3%; CLI/adapters 0% unit profile"],
        ["CPU/memory profiling","Go pprof route","bounded performance measurement","PLANNED; not executed, no performance claim"],
        ["Автоматические проверки CI","GitHub Actions pr12-nb.yml","same wrapper route","C3 success; D1 human/review pending"],
    ]
    for i,row in enumerate(rows,1):
        set_row(d.tables[4], i, row)
    dev_release = [
        ["Подготовка и запуск", "workspace.py smoke intake --require; dev binary outside-checkout; diagnostic_smoke PASS (hit-54-d1-clean.log)", "workspace.py inspect intake --mode release; release binary outside-checkout; diagnostic_smoke PASS (hit-54-d1-clean.log)"],
        ["Преобразования кода и окружения", "go build with -gcflags=all=-N -l; Go 1.27.2; vcs.modified=false", "go build -trimpath; Go 1.27.2; vcs.modified=false and release metadata inspected"],
        ["Оптимизации", "No optimization measurement; debug build is intentionally diagnostic", "-trimpath only; no performance measurement or claim; pprof PLANNED"],
        ["Отладочная информация и диагностика", "--version and JSON fixture output show component/mode/PASS/synthetic scope", "--version and JSON fixture output show release mode/PASS/synthetic scope; no secret/body/header"],
        ["Проверки времени выполнения", "verified fixture accepted; tampered fixture rejected by contractcheck", "verified fixture accepted; tampered fixture rejected by release binary; valid route exit 0"],
        ["Состав результата", ".pr12-build/intake/dev/intake-contractcheck plus outside distribution and verified fixture", ".pr12-build/intake/release/intake-contractcheck plus freshly copied outside distribution and verified fixture"],
    ]
    for i,row in enumerate(dev_release,1):
        for j,v in enumerate(row): set_cell(d.tables[5].cell(i,j),v)
    checks = [
        ["PASS", "verified fixture produces safe diagnostic", "PASS; dev/release smoke exit 0", "python3 tools/pr12/workspace.py smoke intake --require; Go 1.27.2; hit-54-d1-clean.log"],
        ["format_invalid", "invalid format is rejected without secret data", "PASS; invalid format route is exit 2", "./intake-contractcheck --fixture verified.json --format invalid; expected exit recorded separately"],
        ["version", "binary identifies source/build mode", "PASS; go version -m shows go1.27.2, vcs.revision, vcs.modified=false", "go version -m .pr12-build/intake/release/intake-contractcheck; hit-54-d1-clean.log"],
        ["safe output", "no secret/body/header logging", "PASS; output is bounded JSON/text diagnostic", "hit-54-d1-clean.log; synthetic fixtures only"],
        ["restoration", "clean source and repeatable build", "PASS in native recorded run; current branch remains reviewable", "PR #39; run 39262425665; human merge remains pending"],
    ]
    for i,row in enumerate(checks,1):
        for j,v in enumerate(row): set_cell(d.tables[6].cell(i,j),v)
    out=OUT/"0104-pz01-nb.docx"; d.save(out); return out

def make_0104_pz02():
    d=Document(TEMPLATES/"0104-pz02-report-template.docx"); m=dict(COMMON)
    m.update({"submission_tag":"[human after merge: 0104-pz02-nb-v1 not created]","submission_commit":"d164aebd5b746151da41717f0ffb483eb9b39a63","jira_task_url":"https://hitpipe.atlassian.net/browse/HIT-52"})
    fill_common(d,m)
    overrides={20:"Цель: StripeSignature module with typed contract and replaceable SecretResolver/Clock. Requirement FR-SIG-01 → raw-body proof → Verify/Consumer → authenticatewebhook.Consumer. Outside: persistence, route and financial decision.",25:"Own logic is in services/intake/internal/domain/stripesignature and is called by authenticatewebhook.Consumer. C2 is work/HIT-52-nb-authenticate-webhook at e64f4ff; corrected C3 is work/HIT-53-nb-verification at d164aebd5b746151da41717f0ffb483eb9b39a63; PRs #32/#33 are open and unmerged.",27:"Consumer callsite is unchanged while the executed pair FileResolver+FixedClock and MapSecrets+FixedClock are swapped at composition/tests. SystemClock is compiled/documented but was not executed in this experiment. This proves a replaceable boundary, not production adapter readiness.",29:"Module map is docs/labs/pr12/nb/module-map.md; actual imports and build are recorded in hit-52-c2-local-clean.log. The canonical module contract is services/intake/docs/module-contract.md. No cycles claimed.",31:"Candidate exists in services/intake/internal/domain/stripesignature with verified/tampered fixtures. Observation: same application consumer works with two executed dependency implementations. ADR: docs/labs/pr12/nb/module-adr.md; author Никита Будыкин; no canonical HP-ADR ID.",34:"Native C2/C3 checks and actual two-implementation tests are in authenticate_test.go and evidence logs; CI/review/merge status is not promoted to acceptance.",36:"Reciprocal module check by another student: NOT_CONDUCTED; protocol docs/labs/pr12/nb/human-gates/peer-module-out.md.",38:"Individual result: module contract, consumer, map, complexity profile and ADR are content-ready. Immutable tag 0104-pz02-nb-v1 not created; defense not conducted."}
    for i,t in overrides.items(): set_para(d.paragraphs[i],t)
    t=d.tables[3]
    for r,v in {1:"Никита Будыкин; группа [человек]",3:"Intake - Source Integrations / services/intake/internal/domain/stripesignature",4:"[human after merge: 0104-pz02-nb-v1 not created]",5:"https://github.com/hitpipe/hitpipe",7:"CONTENT_READY_IMAGES_PENDING"}.items(): set_cell(t.cell(r,1),v)
    contract=[
        ["Назначение и публичная операция","Verify(rawBody, header, secret, now) → Outcome; module-contract.md"],
        ["Допустимые входы и предусловия","non-empty exact bytes, t/v1 header, secret, trusted now"],
        ["Результат и постусловия","VERIFIED only for matching MAC and freshness; otherwise REJECTED"],
        ["Категории ошибок и порядок при нескольких нарушениях","secret/clock, empty body, header, freshness/MAC"],
        ["Владение входом/выходом, срок действия ссылок","caller-owned bytes/result; deterministic fixed-input policy"],
        ["Состояние и побочные эффекты, явно если отсутствуют","no I/O, log, persistence, receipt, 2xx or payment action"],
    ]
    for i,row in enumerate(contract,1):
        for j,v in enumerate(row): set_cell(d.tables[4].cell(i,j),v)
    impl=[
        ["FileResolver + FixedClock","application tests","normal/boundary/error PASS","executed fixture resolver; no live provider"],
        ["MapSecrets + FixedClock","application tests","normal/boundary/error PASS","executed in-memory double; SystemClock not part of this experiment"],
    ]
    for i,row in enumerate(impl,1):
        set_row(d.tables[5], i, row)
    comp=[
        ["data handling","raw bytes to strict parser","preserve exact proof bytes without excess retention","candidate exercised; future parser"],
        ["trust/inter-component","untrusted ingress and dependency failure","remain fail-closed without false receipt/2xx","ports exercised; transport future"],
    ]
    for i,row in enumerate(comp,1):
        set_row(d.tables[6], i, row)
    six=[
        ["file-backed","normal","VERIFIED","PASS; hit-52-c2-local-clean.log"],
        ["file-backed","boundary","300 passes; 301 rejects","PASS; same log"],
        ["file-backed","error","safe REJECTED","PASS; same log"],
        ["in-memory/fixed","normal","VERIFIED","PASS; same log"],
        ["in-memory/fixed","boundary","300 passes; 301 rejects","PASS; same log"],
        ["in-memory/fixed","error","safe REJECTED","PASS; same log"],
    ]
    for i,row in enumerate(six,1):
        set_row(d.tables[7], i, row)
    out=OUT/"0104-pz02-nb.docx"; d.save(out); return out

for f in (make_0302_pz01, make_0302_pz02, make_0104_pz01, make_0104_pz02):
    f()
print("created", sorted(str(p) for p in OUT.glob("*.docx")))
