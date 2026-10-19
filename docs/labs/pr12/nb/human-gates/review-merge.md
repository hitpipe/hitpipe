# Team source review, author correction and actual PR review

Status: `SOURCE_REVIEW_COMPLETED_CHANGES_REQUIRED` on source
`b23001983b511999d815fbc7f77063e763516bc2`. Accountable reviewer:
Дмитрий Карачебан; actual technical/code/document review: Codex by explicit
delegation. Previous Statsyuk incoming team-review route is superseded.
See [review results](../review/REVIEW_RESULTS_RU.md).

Author PRs are not created: no GitHub APPROVE/CHANGES_REQUESTED or personal
student academic activity is manufactured. No merge is claimed.

For each code task HIT-51, HIT-52 and HIT-53, the author/agent publishes a separate
real PR from the matching branch, fixes NB-REV-001–006, retains all actual review
comments and current CI, and submits corrected exact heads for independent
re-review. Lead need not create the author's PRs or repeat the conceptual analysis
manually. Permitted merge follows a real independent verdict and required checks.
The author must not approve or merge his own PR.

| task | branch | PR URL | review/remarks | final merged SHA | CI URL/conclusion |
| --- | --- | --- | --- | --- | --- |
| HIT-51 | `work/HIT-51-nb-stripe-signature` | NOT_CREATED | source review: numeric boundary fix required | NOT_MERGED | record actual new CI after correction |
| HIT-52 | `work/HIT-52-nb-authenticate-webhook` | NOT_CREATED | composition inspected; inherit corrected policy | NOT_MERGED | record actual new CI after integration |
| HIT-53 | `work/HIT-53-nb-verification` | NOT_CREATED | original tests/mutations inspected; new regressions/evidence required | NOT_MERGED | `https://github.com/hitpipe/hitpipe/actions/runs/39262304491` — success for original SHA `2186a00ca9c441645c99f82fd45b34241864ac6a` |

Required workflow: inspect diff and ownership, run/review CI, write
actionable remarks, author fixes and updates evidence, reviewer rechecks, Lead
merges, then record the immutable merge SHA. A successful CI run alone is not a
review or acceptance.
