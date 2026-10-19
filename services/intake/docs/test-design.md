# HIT-53 / NB-C3 — test design and oracle

Status: executable plan for the submitted `StripeSignature` and
`AuthenticateWebhook` modules. Expected outcomes were written before the runs;
actual outputs live in the evidence logs and are not used as the oracle.

## Techniques

### Equivalence classes and boundaries

| Input dimension | Valid class | Invalid classes | Boundary cases |
| --- | --- | --- | --- |
| MAC | exact HMAC-SHA256 over `t.raw_body`; any one valid v1 candidate | altered byte, wrong secret, empty/short/non-hex v1 | one byte changed; non-UTF-8 raw bytes |
| timestamp | integer seconds within absolute tolerance | missing, malformed, contradictory duplicates, v0-only | past/future 299, 300, 301 seconds |
| secret/clock | non-empty secret and non-zero trusted clock | unknown installation, unavailable secret, unavailable clock | clock exactly at signed timestamp; 300-second edge |
| delivery identity | every delivery is independently verified | repeat ID with altered body/signature | same event ID twice, second body changed |

### Decision/state table

| Secret | Clock | Header/body | Expected state | Safe reason |
| --- | --- | --- | --- | --- |
| available | available | valid v1, fresh, exact bytes | `VERIFIED` / typed handoff | `verified` |
| unavailable | available | any | `REJECTED` | `secret_unavailable` |
| available | unavailable | any | `REJECTED` | `clock_unavailable` |
| available | available | stale/conflict/malformed | `REJECTED` | bounded timestamp/header reason |
| available | available | altered bytes/wrong MAC | `REJECTED` | `signature_mismatch` |
| available | available | verified but no receipt port | `VERIFIED` only | never `ACCEPTED`/`paid`/2xx |

The state table is an oracle for the consumer boundary. It deliberately does not
assert persistence, provider behavior, financial eligibility or a public route.

## Fixtures, reset and clean-clone command

- Committed inputs: `services/intake/testdata/verified.json` and `tampered.json`.
- All values are synthetic; no provider/customer secret or payload is used.
- Test helpers construct fixed bytes and fixed Unix time; each subtest creates its
  own resolver/clock state and does not share mutable state.
- Reset: remove only generated `.pr12-build/` and temporary directories; never
  alter committed fixtures or the source baseline.
- Clean-clone command:

```text
git clone https://github.com/hitpipe/hitpipe.git
cd hitpipe
git fetch --tags origin
git checkout <submitted-full-sha>
export PATH=/path/to/go1.27.2/bin:$PATH
PR12_RACE=1 python3 tools/pr12/workspace.py check intake
python3 tools/pr12/workspace.py smoke intake --require
```

The command must use the exact submitted full SHA and pinned Go 1.27.2. The hidden
teacher suite is `NOT_PROVIDED`, not a claimed PASS.
