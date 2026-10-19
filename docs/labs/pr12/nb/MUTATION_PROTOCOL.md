# HIT-53 / NB-C3 — mutation protocol

The clean baseline is built and tested first. Each mutant is applied to a fresh
temporary copy of `services/intake`, never to the branch. The same named test
command is used for the mutant; a compile failure is `INVALID`, not a killed
behavior. After a survivor, the missing assertion is added to the real test suite,
the survivor is rerun, and both runs are retained. The source is restored before
the next case and after the final case.

The final table is filled from actual logs:

| ID | Behavioral change | Compile | First test result | Detected? | Survivor/retest | Patch/log/digest |
| --- | --- | --- | --- | --- | --- | --- |
| M01 | transformed body | pending | pending | pending | pending | pending |
| M02 | MAC compare bypass | pending | pending | pending | pending | pending |
| M03 | signed timestamp delta | pending | pending | pending | pending | pending |
| M04 | 300-second edge rejected | pending | pending | pending | pending | pending |
| M05 | conflicting timestamp accepted | pending | pending | pending | pending | pending |

Natural defect count is `0` unless a real issue/PR link exists. Hidden suite is
`NOT_PROVIDED`. Mutation evidence does not represent a failed main branch or CI
acceptance; it is a temporary controlled experiment.
