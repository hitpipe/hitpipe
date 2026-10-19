# HIT-45 risk matrix

Qualitative teacher-matrix categories: probability and impact are Low/Medium/High; priority is not a numeric percentage gate. Low probability × High impact remains High; High probability × Low impact is Medium.

| ID | Event / cause / consequence | Probability and rationale | Impact and rationale | Priority | Verification / frequency | Residual risk |
|---|---|---|---|---|---|---|
| R-001 | Malformed or duplicated JSON/handoff accepted as match; false confirmation. | Medium: saved evidence is an untrusted boundary. | High: wrong delivery decision. | High | strict parser and handoff regressions on every code change and CI | Low locally; hidden formats unknown |
| R-002 | Retrieval error treated as absence or retried; possible duplicate effect. | Low: guarded in consumer. | High: external side effect risk. | High | FileReader/application error test every change; no live provider claim | Medium outside offline slice |
| R-003 | Evidence relabelled to another target. | Medium: file fields are input. | High: confirmation for wrong task. | High | target/provenance DI test every change | Low locally |
| R-004 | NUL/control text accepted. | Medium before correction; now explicit regression. | Medium: ambiguous or unsafe evidence. | High | control-input test every change | Low locally |
| R-005 | Escaping causes quote-heavy context to bypass 32 KiB gate. | Medium: context is untrusted text. | High: oversized evidence. | High | JCS escaped-byte test every change | Low locally; persistence out of scope |
| R-006 | NFC/line-ending normalization changes meaningful spaces. | Low: narrow normalization code. | Medium: false match/mismatch. | Medium | Unicode/CRLF/space regressions every change | Low |
| R-007 | Human gate or screenshot placeholder is presented as automated PASS. | High in manual workflow. | Medium: provenance/assessment error. | High | report/evidence audit per submission | Medium until human gate |

Security risks are R-001–R-005. This candidate matrix is not production security acceptance.
