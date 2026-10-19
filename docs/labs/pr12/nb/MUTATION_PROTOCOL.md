# HIT-53 / NB-C3 mutation protocol

This document retains the original M01–M05 experiment as historical evidence and
defines a separate versioned `r2` series after the numerical policy correction.
The r2 source/test baseline is the corrected C3 chain
`d164aebd5b746151da41717f0ffb483eb9b39a63`; its verifier policy digest and each
patch/source digest are recorded in the versioned r2 logs under
`evidence/mutations/r2/`.

Each r2 mutant is an independent compile-preserving change to the current source:

| ID | Current rule changed | Detection oracle | Branch and native CI |
| --- | --- | --- | --- |
| M01 | exact raw bytes replaced by constant bytes | known-vector/raw-byte assertions exit 1 | `work/nb-mutation-r2-M01` at `4606e78f350b85717ef56d9b3766c5fd8bd4af4f`, job 118160603037 success, run 39282875566 |
| M02 | constant-time candidate comparison bypassed | altered-body/repeat assertions exit 1 | `work/nb-mutation-r2-M02` at `b835a7eb3fbd4512396ef44ec8b693d6a5bf0c18`, job 118160720826 success, run 39282905596 |
| M03 | current `outsideTolerance` ignores timestamps in the past | past301 freshness assertion exit 1 | `work/nb-mutation-r2-M03` at `5a8929b141f2b75774418114cfac1bbb7d8a01dd`, job 118160734812 success, run 39282909976 |
| M04 | current inclusive 300-second boundary becomes stale | past300/future300 boundary assertion exit 1 | `work/nb-mutation-r2-M04` at `3cc2a0d5ef75a55283802152b7295b2bb244cc83`, job 118160751628 success, run 39282914689 |
| M05 | current conflicting timestamp comparison is inverted | timestamp-conflict assertion exit 1 | `work/nb-mutation-r2-M05` at `90cc9ab91ce0d24a157fa451c9f4efe71a725286`, job 118160771432 success, run 39282919252 |

The five branches are temporary experiment branches, not code PRs and not merged
into C1/C2/C3/main. The historical files under `evidence/mutations/M01–M05.*`
are not overwritten. The five r2 jobs compiled successfully and their named
behavioral suites exited 1; the complete stdout/exit/CWD/tool data is retained
in the five versioned r2 logs. A compile failure is `INVALID`, not detection. A
test assertion failure after successful compilation is detection. No survivor
or retest occurred. The corrected-baseline gate after the series is a separate
run and PASS: run 39285847846, job 118171472621, SHA
`d164aebd5b746151da41717f0ffb483eb9b39a63`.

The clean sequence is: corrected baseline gate PASS; fresh copy/branch; apply one
patch; compile with `go test -run '^$' ./...` and record exit 0; run the same
named suite and record exit 1; restore/discard that copy; verify the corrected
baseline gate PASS before the next mutant and after M05. Hidden suite remains
`NOT_PROVIDED`; reviewer probes are not author mutations. The mutation result is
controlled evidence, not production acceptance.
