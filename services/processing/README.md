# Processing financial eligibility

Reusable domain/application module for HP-RULE-006; MANUAL_LAB_PR12 Candidate design input. No production server, live provider access, config acceptance, case state, actor/epoch/lineage guards or dispatch. Public contracts and ownership: docs/module-contract.md. Test selection: docs/test-design.md. No third-party Go modules, therefore no go.sum.

Prerequisites: Git, Python3.10+ for the shared wrapper, exact Go1.27.2 on PATH. The recorded local machine is macOS arm64; GitHub Actions uses Ubuntu24.04 and setup-go1.27.2. Obtain Go from https://go.dev/dl/ and verify official platform checksum. The actually used darwin-arm64 archive SHA256 is 76812b213b1b2302c978d28fa52fa92d541704b9e7d9d5db8002c50e4018c4c5. Do not silently substitute another Go. No DB, Docker, Stripe key, ClickUp account or personal fixture is needed.

From a fresh clone: checkout the exact reviewed PR head or immutable submitted tag, record git rev-parse HEAD and go version. From repository root:

```sh
python3 tools/pr12/workspace.py validate
python3 tools/pr12/workspace.py check processing --require
python3 tools/pr12/workspace.py smoke processing --require
python3 tools/pr12/workspace.py inspect processing --mode release --require
```

From services/processing: go test -race -count=1 ./... . The fixed fixture testdata/valid.json gives VALID/FINANCIAL_ELIGIBILITY_ONLY at its explicit synthetic time, not wall clock. testdata/refunded.json gives INELIGIBLE/REFUND. testdata/invalid.json and invalid --format give exit2. Reset is rereading versioned input; tests allocate fresh values/temp files. Same fixed time/data ensures reproducibility independent of today's clock. Only synthetic snapshots allowed; real payment data must not be pasted into this diagnostic.

Build release then copy .pr12-build/processing/release/processing-contractcheck and desired testdata/*.json to a new empty directory outside checkout. Change to that directory, run ./processing-contractcheck --version, then ./processing-contractcheck --fixture valid.json --format json and print exit code. Runtime package is binary+declared fixture; on macOS it uses system libSystem/libresolv (actual otool record). No Go compiler/source tree is needed at runtime. Other OS binaries must be built for that OS. Do not infer a static Linux binary from the macOS inspection.

Mutation reproduction from repo root, clean exact version, disposable worktree automatically removed:

```sh
python3 services/processing/tools/mutations/run.py --output .pr12-build/processing-mutations --natural-defects 1 --natural-defect-task https://hitpipe.atlassian.net/browse/HIT-29
```

Expect six compiled mutants detected and a genuine failing negative-gate log, followed by clean green restoration. Inspect summary.json; any INVALID/SURVIVED requires investigation and preserved retest. Never merge mutants. This command does not take screenshots. Human instructions live in docs/labs/pr12/tp/evidence/screenshots/INSTRUCTIONS.md (assembled with reports).
