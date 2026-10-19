# HIT-45 risk register

This is a qualitative offline-module registry, not production acceptance. Probability and impact use the methodic matrix categories Low/Medium/High; priority follows the teacher matrix, including the rule that Low probability × High impact remains High while High probability × Low impact is Medium.

| ID | Event / cause / consequence | Probability and rationale | Impact and rationale | Matrix priority | Verification and frequency | Residual risk |
|---|---|---|---|---|---|---|
| R-001 | Malformed or duplicated JSON/handoff is accepted as a match because a parser stops at a prefix. | Medium: adversarial or corrupted saved evidence is plausible. | High: false delivery confirmation. | High | strict boundary regression suite on every code change and CI | Low for covered local syntax; hidden provider formats unknown |
| R-002 | Retrieval failure is converted to match or retried as if no effect existed. | Low in current code; guarded by consumer error path. | High: duplicate external effect could follow a false absence. | High | error/provenance tests every change; live provider behavior not exercised | Medium because provider authority is outside this offline slice |
| R-003 | Target evidence is relabelled to another target. | Medium: saved files are user-controlled inputs. | High: evidence attaches to wrong task. | High | FileReader target/provenance tests every change | Low for target/source/time fields present |
| R-004 | NUL or another forbidden control passes text validation. | Medium before correction; boundary test now runs every change. | Medium: parser ambiguity and false mismatch/match. | High | control-input regression test every change | Low for accepted text fields; upstream policy remains candidate |
| R-005 | Quote-heavy context bypasses the 32 KiB serialized budget. | Medium: escaping expands attacker-controlled text. | High: oversized or divergent evidence can be persisted/processed. | High | JCS/escaped-byte boundary test every change | Low for local projection; persistence enforcement is outside scope |
| R-006 | Normalization changes meaningful spaces or case. | Low: normalization is narrowly implemented. | Medium: false match or false mismatch. | Medium | NFC/CRLF and whitespace regressions every change | Low |
| R-007 | Human screenshot/interview/review is mistaken for automated PASS. | High in a manual lab package unless explicitly separated. | Medium: assessment provenance becomes false. | High | handoff/report audit before each review; human gate frequency per submission | Medium until the named person performs the gate |

Security risk is explicitly represented by R-001–R-005. No production security acceptance or live provider permission is claimed.
