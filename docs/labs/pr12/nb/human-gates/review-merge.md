# PR review, correction and merge protocol

Status: `PENDING_PR_CREATION_AND_REVIEW`. No PR URL or merge is claimed here.

For each code task HIT-51, HIT-52 and HIT-53, Lead must create/use a separate
PR from the matching branch, assign an independent reviewer, retain all review
comments, require the current CI run, record fixes and rerun evidence, then
merge only after human approval and branch protection requirements are met.
The author must not approve or merge his own PR.

| task | branch | PR URL | review/remarks | final merged SHA | CI URL/conclusion |
| --- | --- | --- | --- | --- | --- |
| HIT-51 | `work/HIT-51-nb-stripe-signature` | `[human fills]` | `[human fills]` | `[human fills]` | `[human fills]` |
| HIT-52 | `work/HIT-52-nb-authenticate-webhook` | `[human fills]` | `[human fills]` | `[human fills]` | `[human fills]` |
| HIT-53 | `work/HIT-53-nb-verification` | `[human fills]` | `[human fills]` | `[human fills]` | `https://github.com/hitpipe/hitpipe/actions/runs/39262304491` — success for SHA `2186a00ca9c441645c99f82fd45b34241864ac6a`; review/merge still pending |

Required human sequence: inspect diff and ownership, run/review CI, write
actionable remarks, author fixes and updates evidence, reviewer rechecks, Lead
merges, then record the immutable merge SHA. A successful CI run alone is not a
review or acceptance.
