# HIT-45 defect and mutation matrix

Status: actual executed evidence. Source baseline `7e815aa790e0ebba58e453e786bda45b9266161d`;
full log: `HIT-45-mutations-actual.log`. Each mutation compiled, its named assertion failed,
and the source was restored to SHA-256 `ab3d84f890fc099643be134a8ff51e0a7dffcb4a323abfe84f78701d6a615688`.

| ID | Behavioral defect | Guard | Actual status |
|---|---|---|---|
| M1 | missing retrieval treated as success | `TestCompareIncompleteAndUnknownPrecedence` | compiled, exit 1 assertion; restored |
| M2 | malformed handoff classified as mismatch | `TestCompareIncompleteAndUnknownPrecedence` | compiled, exit 1 assertion; restored |
| M3 | changed title accepted | `TestCompareMismatchEachRequiredField/title` | compiled, exit 1 assertion; restored |
| M4 | malformed UTF-8 title accepted | `TestReviewComparatorRejectsInvalidTitleAndRetrievalError` | compiled, exit 1 assertion; restored |
| M5 | missing assignee accepted | `TestC3FalseConfirmationGuards/missing_assignee` | compiled, exit 1 assertion; restored |

No provider writes, retry, state transition, or customer data are involved. A complete
mutation record must include exact mutated diff, command, exit, survivor/retest result,
and restoration hash. Human review and CI remain separate gates.
