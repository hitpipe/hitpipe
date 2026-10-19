# Mutation experiment inputs

This directory is the reproducibility note for HIT-53. Mutation execution occurs
in a temporary copy/worktree only; no mutant is committed or merged. The baseline
must be green before each independent mutation, and the final baseline must be
green after all runs.

Seeded candidates, one at a time:

1. Hash a transformed body instead of exact raw bytes.
2. Bypass the constant-time MAC comparison.
3. Compare signed timestamp only in one direction instead of absolute distance.
4. Reject the inclusive 300-second boundary (`>=` instead of `>`).
5. Accept a conflicting duplicate timestamp by changing the conflict predicate.
6. (Optional survivor probe) change an untested parser edge only if it is a
   meaningful behavioral rule; if a survivor appears, add a test, rerun it, and
   record both results.

For each mutation record source diff/patch digest, mutated SHA or temporary source
digest, compile result, exact test command/log/exit, violated rule and
`killed`/`survived`. Compile failure is `INVALID`, not detected. Restore the clean
baseline before the next mutation.
