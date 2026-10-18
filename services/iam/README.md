# IAM — bounded SessionPolicy component

HIT-19 / Feature HIT-11, MANUAL_LAB_PR12. Author-owner: Дмитрий Карачебан; agent implementation: Codex. This is reusable IAM logic with a synthetic diagnostic executable, not a deployed authentication service.

## Reproduce

Use Go **1.27.2** (official [download index](https://go.dev/dl/?mode=json)); Python 3.10+ for the existing wrapper, Linux for the documented binary inspection. No external Go libraries or credentials are needed. `go.sum` is absent because this module imports only the standard library. Preserve the legacy root module.

From repository root:

```sh
python3 tools/pr12/workspace.py validate
python3 tools/pr12/workspace.py check iam --require
python3 tools/pr12/workspace.py build iam --mode dev --require
python3 tools/pr12/workspace.py build iam --mode release --require
python3 tools/pr12/workspace.py smoke iam --require
python3 tools/pr12/workspace.py inspect iam --mode release --require
PR12_RACE=1 python3 tools/pr12/workspace.py check iam --require
```

For a source diagnostic without linker metadata: `cd services/iam && go run ./cmd/iam-contractcheck --fixture testdata/valid.json --format json`. A built executable accepts `--version`, `--fixture PATH`, `--format text|json`. `testdata/idle-equal.json` yields denied/idle_expired with exit 0; malformed configuration yields exit 2. Diagnostics contain fixed codes, never fixture contents. It is a short process: exit stops it; reset means rerunning the same read-only fixture from the exact commit. No persistent state or external cleanup is involved.

The wrapper creates `.pr12-build/iam-dev` and `iam-release` artifacts (actual filenames printed by build); smoke copies each binary and synthetic input to a temporary directory outside the checkout, then verifies version, positive output and malformed configuration. Debug/release compiler flags and binary observations must be recorded from real runs; no speed advantage is asserted.

## Source boundaries

- `internal/domain/sessionpolicy`: pure rules and value types; only standard time dependency.
- `cmd/iam-contractcheck`: initial diagnostic composition, strict fixture boundary and safe output.
- `docs/module-contract.md`: sole local normative contract, authored in HIT-18 before code.
- `testdata`: synthetic, fixed dates and no user/production data.

The next task introduces the production application consumer and consumer-owned ports; SQL/Keycloak/OIDC/current workspace authorization/runtime barriers remain outside this slice. A session_valid result grants no business permission. See the exact [candidate input context](../../docs/labs/pr12/context-lock.json).
