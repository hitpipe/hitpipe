# HIT-45 defect and mutation matrix

Status: candidate evidence. This matrix records the intended behavioral guards and the
actual baseline test status. It does not claim mutation survivors or retests until the
mutated source is compiled and executed.

| ID | Behavioral defect | Guard | Actual status |
|---|---|---|---|
| M1 | missing retrieval treated as success | retrieval missing -> unknown | baseline test added; mutation run pending |
| M2 | duplicate handoff marker accepted | duplicate marker -> incomplete | baseline test added; mutation run pending |
| M3 | changed title accepted | title mismatch -> mismatch | baseline test added; mutation run pending |
| M4 | malformed UTF-8 accepted | invalid bytes -> unknown | baseline test added; mutation run pending |
| M5 | missing assignee accepted | no configured assignee -> mismatch | baseline test added; mutation run pending |

No provider writes, retry, state transition, or customer data are involved. A complete
mutation record must include exact mutated diff, command, exit, survivor/retest result,
and restoration hash. Human review and CI remain separate gates.
