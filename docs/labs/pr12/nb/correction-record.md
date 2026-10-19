# HIT-15 NB correction record

Participant: Никита Будыкин  
Repository: `hitpipe/hitpipe`  
Mode: `MANUAL_LAB_PR12`  
Component: `Intake - Source Integrations`  
Status: `SOURCE_CORRECTION_READY_FOR_INDEPENDENT_REPEAT_REVIEW`

## Review input

The delegated review context was fetched without checking out the reviewer
branch:

```text
git fetch origin codex/HIT-55-nb-delegated-review
git show FETCH_HEAD:docs/labs/pr12/nb/review/AUTHOR_CORRECTION_PROMPT_RU.md
```

Reviewer context commit: `0e3345b571ad6ad8855d4db4b4265b465a04ed67`. The
reviewed author package was `b23001983b511999d815fbc7f77063e763516bc2`.
The reviewer verdict was `SOURCE REVIEW COMPLETED / CHANGES REQUIRED`; it was
not represented as a GitHub approval.

## NB-REV-001–006 disposition

| Finding | Correction and evidence | Actual result | Remaining boundary |
| --- | --- | --- | --- |
| NB-REV-001 | Checked decimal `int64` parsing; positive Unix-second domain; subtraction-free absolute-distance comparison; three reviewer regressions and invalid-UTF-8 bytes in `verifier_test.go`. | Correction SHA `df1a7d58179148abfda8e71862654f5ef7fcdbc7`; native CI run [39274386817](https://github.com/hitpipe/hitpipe/actions/runs/39274386817), job `118134753907`, success. | No live Stripe endpoint or trusted production clock is claimed. |
| NB-REV-002 | Created separate stacked code PRs and recorded task→branch→SHA→CI in Jira and `submission-record.json`. | [PR #31](https://github.com/hitpipe/hitpipe/pull/31), [PR #32](https://github.com/hitpipe/hitpipe/pull/32), [PR #33](https://github.com/hitpipe/hitpipe/pull/33) are open and unmerged. | Human review, merge and branch-protection checks remain pending. |
| NB-REV-003 | Reused existing CLI `--format`: text and json are valid; invalid value returns safe `format_invalid`, exit 2. | Executed by the owned dev/release smoke route in correction CI; details in `evidence/hit-53-correction-cli.md`. | This is a bounded diagnostic configuration experiment, not production configuration. |
| NB-REV-004 | Replaced generic NOT RUN/PASS claims; corrected DI pair to `FileResolver+FixedClock` and `MapSecrets+FixedClock`; separated coverage from planned pprof; refreshed four DOCX. | Four DOCX rendered and inspected; ZIP/XML integrity PASS; 21 text-only image instructions retained. | CPU/memory pprof and human gates are not executed. |
| NB-REV-005 | Corrected normative references, exact module path, 29-entry crosswalk fields, actual adapter claims and `low×serious → medium` CI risk. | `criteria-crosswalk.json`, `requirements-trace.json`, `module-map.md`, `risk-register.md` and report outputs contain the correction. | Candidate/In Review source status remains unchanged; no formal acceptance is inferred. |
| NB-REV-006 | Rewrote all 21 screenshot instructions with cwd, full argv, source/SHA, expected exit, visible evidence, destination, caption and redaction. Added new distribution, `.pr12-build`, path-existence and isolated mutation routes. | Instruction count check: 21; no PNG generated or captured by the agent; correction report CI success [39274814284](https://github.com/hitpipe/hitpipe/actions/runs/39274814284), job `118136237778`. | Human must perform captures and insert actual PNGs. |

## Current package

Report branch: `work/HIT-56-nb-reports`  
Report head: `3c0c8f077d67f7ec2fa1c0243192a2ac7eaa9e4f`  
Package status: `CONTENT_READY_IMAGES_PENDING`  
Reports: `docs/labs/pr12/nb/reports/`  
Evidence index: `docs/labs/pr12/nb/evidence/index.json`  
Human instructions: `docs/labs/pr12/nb/evidence/screenshots/INSTRUCTIONS.md`  
Crosswalk: `docs/labs/pr12/nb/criteria-crosswalk.json`  
Handoff: `docs/labs/pr12/nb/handoff.md`

The four report exports are deliberately retained: two 0302 primary reports
and two separate 0104 exports, as required by the report profile. No screenshot,
signature, score, interview, peer review, opposition, defense, merge, tag or
formal Project OS operation is claimed here.

## Independent repeat review request

The independent reviewer should inspect PRs #31–#33 and this correction record,
re-run the named correction CI or equivalent clean-checkout commands, verify the
four DOCX and 29 crosswalk entries, and record a new verdict. The reviewer must
not treat this author record as its own approval.
