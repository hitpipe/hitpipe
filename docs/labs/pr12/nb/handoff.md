# HitPipe HIT-15 Nikita Budykin handoff

Current package status: `CONTENT_READY_IMAGES_PENDING`. This is the text and
evidence package for Lead review, not formal Project OS admission, acceptance,
merge or final submission.

Correction record for the delegated source review: `docs/labs/pr12/nb/correction-record.md`.

## Candidate branches and commits

| task | branch | current candidate SHA | local result | external state |
| --- | --- | --- | --- | --- |
| HIT-49 R1 | `work/HIT-49-nb-requirements` | `9659c39ad0010e1329bfe3e3bc92c9edd84982bb` | requirements/security/trace checks PASS | [PR #37](https://github.com/hitpipe/hitpipe/pull/37), base `main`; interview/review pending |
| HIT-50 R2 | `work/HIT-50-nb-refinement` | `35b4c247319cf3bf0c8d8288d109c996fc721d0b` | document validation PASS | [PR #38](https://github.com/hitpipe/hitpipe/pull/38), base HIT-49; review/clarity pending |
| HIT-51 C1 | `work/HIT-51-nb-stripe-signature` | `53687c138e266fda9104ef620edbc7d4252d8f6` | corrected parser/clock/distance plus regressions | [PR #31](https://github.com/hitpipe/hitpipe/pull/31); current-head native job 118156713785 success |
| HIT-52 C2 | `work/HIT-52-nb-authenticate-webhook` | `e64f4ff4310c597414f768ea705b6ceb1221e1b2` | ordinary no-ff parent merge of C1 | [PR #32](https://github.com/hitpipe/hitpipe/pull/32); current-head native job 118156750440 success |
| HIT-53 C3 | `work/HIT-53-nb-verification` | `d164aebd5b746151da41717f0ffb483eb9b39a63` | ordinary no-ff parent merge of C2; corrected policy retained | [PR #33](https://github.com/hitpipe/hitpipe/pull/33); current-head native job 118156796184 success |
| HIT-54 D1 | `work/HIT-54-nb-docs` | `a74a5e1a8ada1147b5755503fd01fd458b8a4706` | toolchain/document checks PASS | [PR #39](https://github.com/hitpipe/hitpipe/pull/39), base C2; review pending |
| HIT-55 H1 | `work/HIT-55-nb-human-gates` | `78a4ecb87a50ec79f8979b08a4449fcfd3d77c6b` | protocols prepared | [PR #40](https://github.com/hitpipe/hitpipe/pull/40); all human gates pending |
| HIT-56 REPORT | `work/HIT-56-nb-reports` | report package commit `c42820c5063822443102e0c085b7d27e433b6780`; code snapshot `d164aebd5b746151da41717f0ffb483eb9b39a63` | four regenerated DOCX, 29-key crosswalk, 21 instructions; r2 series and final baseline gate PASS | [PR #34](https://github.com/hitpipe/hitpipe/pull/34); independent repeat review pending |

Three code PRs remain separate and stacked (#31–#33); R1/R2/D1/H1 each also
have their own published PR (#37–#40). None is merged into `main`, and no PR
review or acceptance is claimed by this package.

## Verified commands and CI

- C1: `go test ./...`, `go vet ./...`, `go test -race ./...`, wrapper
  validate/check/smoke/release and safe CLI checks; see
  `evidence/hit-51-c1-clean.log` and `hit-51-c1-cli-clean.log`.
- C2: same local route with application tests and both dependency
  implementations; see `evidence/hit-52-c2-local-clean.log`.
- C3: corrected source/test baseline
  `d164aebd5b746151da41717f0ffb483eb9b39a63`, verifier source digest
  `0cc138639531dae97ccff253208e494fe39ac7029bb16bc3bcc046752c964640`; current
  native checks completed successfully in jobs 118156713785, 118156750440 and
  118156796184. Prior verified observations remain historical and are not
  silently relabeled as the current-head result.
- Verified GitHub Actions: run
  `https://github.com/hitpipe/hitpipe/actions/runs/39262304491`, SHA
  `2186a00ca9c441645c99f82fd45b34241864ac6a`, conclusion `success`.
- Verified refinement Actions: run
  `https://github.com/hitpipe/hitpipe/actions/runs/39262488242`, SHA
  `35b4c247319cf3bf0c8d8288d109c996fc721d0b`, conclusion `success`.
- Verified documentation Actions: run
  `https://github.com/hitpipe/hitpipe/actions/runs/39262425665`, SHA
  `a74a5e1a8ada1147b5755503fd01fd458b8a4706`, conclusion `success`.
- Verified C3 correction Actions: run
  `https://github.com/hitpipe/hitpipe/actions/runs/39274386817`, job
  `118134753907`, SHA `df1a7d58179148abfda8e71862654f5ef7fcdbc7`, conclusion
  `success`.
- Current refreshed report-package Actions runs for head `c42820c` are tracked
  in `evidence/index.json`; iam job 118177294111 completed successfully,
  intake job 118177292029 is in progress, and processing/delivery jobs remain
  queued.
  The package is not calling the incomplete workflow set PASS.

## NB-REV-007 temporary mutation series

The historical mutation files remain untouched. The new r2 branches are seeded
from corrected C3 and each has a compile-preserving patch plus a temporary native
workflow with separate compile and behavioral exits. All five native jobs
completed successfully; each compile exited 0 and its named behavioral suite
exited 1, so each mutant was killed. Full logs are versioned under
`evidence/mutations/r2/`; the post-series corrected-baseline gate PASSed in run
39285847846/job 118171472621:

| ID | branch / full SHA | patch SHA-256 | native run |
| --- | --- | --- | --- |
| M01 | `work/nb-mutation-r2-M01` / `4606e78f350b85717ef56d9b3766c5fd8bd4af4f` | `8695baa773b424149417e7d453e2341ef512ddfaa8ef98511e721c1d34db6dd1` | [39282875566](https://github.com/hitpipe/hitpipe/actions/runs/39282875566), job 118160603037, KILLED |
| M02 | `work/nb-mutation-r2-M02` / `b835a7eb3fbd4512396ef44ec8b693d6a5bf0c18` | `d075109572c75d2cb512a09f2df226bfe11c6b04d61c2af4f81b07bc57be4f59` | [39282905596](https://github.com/hitpipe/hitpipe/actions/runs/39282905596), job 118160720826, KILLED |
| M03 | `work/nb-mutation-r2-M03` / `5a8929b141f2b75774418114cfac1bbb7d8a01dd` | `ae02bb56e32c1b8f933aec90dde742da67d0dd5d73a9bada5393608008eaa050` | [39282909976](https://github.com/hitpipe/hitpipe/actions/runs/39282909976), job 118160734812, KILLED |
| M04 | `work/nb-mutation-r2-M04` / `3cc2a0d5ef75a55283802152b7295b2bb244cc83` | `462b7897cf1f55a84bca439a9ce86c0246b1127267647566780f11079b728ace` | [39282914689](https://github.com/hitpipe/hitpipe/actions/runs/39282914689), job 118160751628, KILLED |
| M05 | `work/nb-mutation-r2-M05` / `90cc9ab91ce0d24a157fa451c9f4efe71a725286` | `b38e284c736844cb43d16acb5fcbb53a226bce2c989cd4cbf26fa1fb5bc028c2` | [39282919252](https://github.com/hitpipe/hitpipe/actions/runs/39282919252), job 118160771432, KILLED |

## Human actions required

1. Create and independently review/merge PRs for HIT-51, HIT-52 and HIT-53;
   preserve exact URLs, review comments, fix commits, final merge SHAs and CI.
2. Conduct the real interview, blind clarity check, reciprocal scaffold/module
   checks and teacher-assigned cross-project opposition. Fill only actual names,
   dates, answers, remarks and retest evidence.
3. The bounded CLI's existing `--format` parameter now supplies the invalid
   configuration experiment: `text` and `json` are valid representations;
   `--format invalid` returns safe `format_invalid` and exit 2. The report slot
   records this actual run; no fictitious config flag is introduced.
4. Insert exactly 21 real PNGs using
   `evidence/screenshots/INSTRUCTIONS.md`; the agent did not capture them.
5. After actual merge and CI, create immutable tags
   `0104-pz01-nb-v1`, `0104-pz02-nb-v1`, `0302-pz01-nb-v1`,
   `0302-pz02-nb-v1`, then record tag→SHA→CI in the external submission record.
6. Human checks title/group/signature/teacher fields and performs personal
   defense. Until then the package cannot be `SUBMISSION_READY`.
