# HIT-15 NB repeat-review correction record (round 2)

Participant: Никита Будыкин  
Repository: `hitpipe/hitpipe`  
Mode: `MANUAL_LAB_PR12`  
Component: `Intake - Source Integrations`  
Status: `CONTENT_READY_IMAGES_PENDING; NB-REV-007 mutation series PASS; corrected-baseline gate PENDING`

## Review input

The second delegated review package was fetched without checking out the
reviewer branch:

```text
git fetch origin codex/HIT-55-nb-repeat-review
git show FETCH_HEAD:docs/labs/pr12/nb/review-round-2/REVIEW_RESULTS_RU.md
```

The review record identifies the reviewed report source as
`3fdd78828a69bf788b7a2da9aa3acf58a5e1531a` and the corrected policy already
present in C3 as `df1a7d58179148abfda8e71862654f5ef7fcdbc7`. The verdict was
`CHANGES_REQUIRED`; no approval or formal Project OS review is claimed.

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
| NB-REV-001 | Moved the numerical fix and regression tests into C1, then ordinary parent merges into C2 and C3. | C1 `53687c138e266fda9104ef620edbc7d4252d8f6`; C2 `e64f4ff4310c597414f768ea705b6ceb1221e1b2`; C3 `d164aebd5b746151da41717f0ffb483eb9b39a63`. Current-head native checks are recorded with successful jobs in `evidence/index.json`. | No live Stripe endpoint or trusted production clock is claimed. |
| NB-REV-002 | Created separate PRs for all remaining R1/R2/D1/H1 work and preserved the three stacked code PRs. | [PR #31](https://github.com/hitpipe/hitpipe/pull/31), [PR #32](https://github.com/hitpipe/hitpipe/pull/32), [PR #33](https://github.com/hitpipe/hitpipe/pull/33), [PR #37](https://github.com/hitpipe/hitpipe/pull/37), [PR #38](https://github.com/hitpipe/hitpipe/pull/38), [PR #39](https://github.com/hitpipe/hitpipe/pull/39), [PR #40](https://github.com/hitpipe/hitpipe/pull/40) are open and unmerged. | Human review, merge and branch-protection checks remain pending. |
| NB-REV-003 | Reused existing CLI `--format`: text and json are valid; invalid value returns safe `format_invalid`, exit 2. | Executed by the owned dev/release smoke route in correction CI; details in `evidence/hit-53-correction-cli.md`. | This is a bounded diagnostic configuration experiment, not production configuration. |
| NB-REV-004 | Replaced generic NOT RUN/PASS claims; corrected DI pair to `FileResolver+FixedClock` and `MapSecrets+FixedClock`; separated coverage from planned pprof; refreshed four DOCX from the requirements trace. | Four regenerated DOCX rendered and visually inspected; ZIP/XML integrity PASS; technical tables contain concrete observations and source/command boundaries; 21 text-only image instructions retained. | CPU/memory pprof and human gates are not executed. |
| NB-REV-005 | Corrected normative references, exact module path, 29-entry HIT-key crosswalk fields, actual adapter claims and `low×serious → medium` CI risk. Р4 is `human_gate=none`; its fixtures/security checks are agent-owned. | `criteria-crosswalk.json`, `requirements-trace.json`, `module-map.md`, `risk-register.md` and report outputs contain the correction. | Interview, peer, opposition, review, merge and defense gates remain human-pending; Candidate/In Review source status remains unchanged. |
| NB-REV-006 | Rewrote all 21 screenshot instructions with cwd, full argv, source/SHA, expected exit outside argv, visible evidence, destination, caption and redaction. Added release distribution, `.pr12-build`, path-existence and current r2 mutation routes. | Instruction count check: 21; no PNG generated or captured by the agent. | Human must perform captures and insert actual PNGs. |

## NB-REV-007 mutation correction series

The historical M01–M05 files remain unchanged. A separate r2 series was seeded
from corrected C3 baseline `d164aebd5b746151da41717f0ffb483eb9b39a63`; each
mutant is on a temporary branch, has a distinct full commit SHA and patch file,
and has a native CI run with an explicit compile step followed by the behavioral
detection step. All five native jobs completed successfully: each compiled with
exit 0, then the named behavioral suite exited 1 and the harness recorded
`RESULT=KILLED`. The full stdout, exits, CWD and tool versions are preserved in
the versioned r2 logs.

| ID | Full mutant SHA | Patch SHA-256 | Branch | Native run | Current result |
| --- | --- | --- | --- | --- | --- |
| M01 | `4606e78f350b85717ef56d9b3766c5fd8bd4af4f` | `8695baa773b424149417e7d453e2341ef512ddfaa8ef98511e721c1d34db6dd1` | `work/nb-mutation-r2-M01` | [39282875566](https://github.com/hitpipe/hitpipe/actions/runs/39282875566), job 118160603037 | compile 0; behavioral 1; KILLED |
| M02 | `b835a7eb3fbd4512396ef44ec8b693d6a5bf0c18` | `d075109572c75d2cb512a09f2df226bfe11c6b04d61c2af4f81b07bc57be4f59` | `work/nb-mutation-r2-M02` | [39282905596](https://github.com/hitpipe/hitpipe/actions/runs/39282905596), job 118160720826 | compile 0; behavioral 1; KILLED |
| M03 | `5a8929b141f2b75774418114cfac1bbb7d8a01dd` | `ae02bb56e32c1b8f933aec90dde742da67d0dd5d73a9bada5393608008eaa050` | `work/nb-mutation-r2-M03` | [39282909976](https://github.com/hitpipe/hitpipe/actions/runs/39282909976), job 118160734812 | compile 0; behavioral 1; KILLED |
| M04 | `3cc2a0d5ef75a55283802152b7295b2bb244cc83` | `462b7897cf1f55a84bca439a9ce86c0246b1127267647566780f11079b728ace` | `work/nb-mutation-r2-M04` | [39282914689](https://github.com/hitpipe/hitpipe/actions/runs/39282914689), job 118160751628 | compile 0; behavioral 1; KILLED |
| M05 | `90cc9ab91ce0d24a157fa451c9f4efe71a725286` | `b38e284c736844cb43d16acb5fcbb53a226bce2c989cd4cbf26fa1fb5bc028c2` | `work/nb-mutation-r2-M05` | [39282919252](https://github.com/hitpipe/hitpipe/actions/runs/39282919252), job 118160771432 | compile 0; behavioral 1; KILLED |

The temporary workflow is not part of the deliverable branches. Its intended
success condition is compile exit `0`, behavioral-suite exit `1`, and a final
`RESULT=KILLED`; the five completed job logs are copied into versioned r2
evidence files. No survivor or retest occurred. The corrected-baseline gate
after the series is still queued. Hidden suite remains `NOT_PROVIDED` and
reviewer probes remain separate from author mutations.

## Current package

Report branch: `work/HIT-56-nb-reports`
Report source snapshot before this correction: `a2eee527180ba5f8456046cd40cdd41a2fd5ff5b`; final correction commit is recorded after commit, not invented here.
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

The independent reviewer should inspect PRs #31–#34 and #37–#40 and this correction record,
re-run the named correction CI or equivalent clean-checkout commands, verify the
four DOCX and 29 crosswalk entries, and record a new verdict. The reviewer must
not treat this author record as its own approval.
