# HIT-53 / NB-C3 — qualitative risk register

Qualitative matrix: probability `low/medium/high`; impact `small/serious/severe`.
Priority is not a numeric multiplication: low probability + severe impact remains
high priority, while high probability + small impact is medium.

| Event | Cause | Consequence | P | Impact | Priority | Check/frequency | Decision/residual risk |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Raw-body substitution | parser/re-serializer before HMAC | false verification or false rejection | medium | severe | high | raw-byte regression every change; mutation experiment | block merge on failure; residual provider-specific behavior unknown |
| MAC compare bypass | wrong equality or early acceptance | forged ingress may reach future receipt | low | severe | high | named tests + mutation run every submission | fail closed; no production route in slice |
| Timestamp future/past bypass | signed delta instead of absolute | replay/future event acceptance | medium | serious | high | boundary 299/300/301 every change | reject; trusted clock uncertainty remains future work |
| Secret scope confusion | wrong installation lookup | cross-account verification | low | severe | high | DI error tests and review each change | fail closed; secure-store integration not proven |
| Diagnostic leakage | body/header/secret in output | provider/customer data disclosure | medium | serious | high | forbidden-marker scan every diagnostic change | safe reasons only; no live data |
| Repeat-ID shortcut | dedupe before signature | altered repeat bypasses proof | medium | serious | high | repeated-ID negative test every change | authenticate each delivery; durable receipt absent |
| CI assertion removed | workflow only reports status | false green submission | low | serious | high | local negative same-check + required CI | workflow owned by nb; branch protection readback human |
| Unsupported provider shape | schema drift/unknown event | unsafe normalization or hold loss | medium | serious | high | explicit reject tests; provider integration deferred | do not coerce; formal/provider contract pending |

High-priority risks select local assertions and mutation checks on every submission;
provider/live/hidden checks remain deferred or human-owned and are not claimed.
