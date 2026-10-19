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
| M01 | transformed body instead of exact raw bytes | 0 | 1 | yes — raw-byte and known-vector assertions failed | none | `mutations/M01.patch`, `mutations/M01.log`; patch SHA-256 `71c24554cdafaea924769264141e30d454180e15d87dd29fa1480586ab7b1deb` |
| M02 | MAC compare bypass (`expected` compared with itself) | 0 | 1 | yes — altered-body and repeat-without-signature assertions failed | none | `mutations/M02.patch`, `mutations/M02.log`; patch SHA-256 `7dd3df025b106ecce78d6e2457aeb3c0694e071c8ff59dbaa1020a2912a91dcc` |
| M03 | signed timestamp delta checked only in positive direction | 0 | 1 | yes — `future301` failed | none | `mutations/M03.patch`, `mutations/M03.log`; patch SHA-256 `3d14dc924d139fdbe9c5ff141796d841160725749136a376f52f43aa19c10667` |
| M04 | reject inclusive 300-second boundary | 0 | 1 | yes — `past300` failed | none | `mutations/M04.patch`, `mutations/M04.log`; patch SHA-256 `05b4a8d56a0ee17291876ae858c9131969e12b40e91dd15e2c5c99af18b912dc` |
| M05 | conflicting timestamp accepted | 0 | 1 | yes — conflicting-timestamp reason assertion failed | none | `mutations/M05.patch`, `mutations/M05.log`; patch SHA-256 `22b8525d30ebc157a8195b1cde5fcdebc9cf4a8cb9827350995021fc97473351` |

Natural defect count is `0` unless a real issue/PR link exists. Hidden suite is
`NOT_PROVIDED`. Mutation evidence does not represent a failed main branch or CI
acceptance; it is a temporary controlled experiment.

All five mutants compiled successfully and were killed by the named test suite.
No survivor or retest exists. The experiment used baseline SHA
`835bbb1bc565e9b2786d5ed1a2cac166f816f7a8`; each log records a separate
temporary-copy path, source digest, command, cwd, Go version and exit codes.
