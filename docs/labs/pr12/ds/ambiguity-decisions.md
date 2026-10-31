# HIT-42 ambiguity decisions

Status: CANDIDATE / In Review. These are local decisions for the TargetReadback module,
not changes to canonical upstream sources.

| Question | Decision | Boundary / follow-up |
|---|---|---|
| Does a missing expected key count as mismatch? | No. It is `incomplete`. | Prevents partial evidence from becoming a false confirmation. |
| Can retrieval failure be retried by the comparator? | No. It is `unknown`; retry policy belongs to the application/adapter boundary. | A future consumer must make retry intent explicit and preserve the original evidence. |
| Can malformed bytes be normalized lossily? | No. | Return `unknown`; do not log raw payloads. |
| Are unexpected keys ignored? | No in exact-set mode; they produce `mismatch`. | The application must choose exact-set mode explicitly; no silent widening. |
| Who performs provider reads and writes? | Adapters perform reads; this module performs neither reads nor writes. | Keeps ownership and side effects outside the pure comparator. |
| Is this a canonical schema change? | No. | Escalate any conflict with pinned Architecture/Product sources to Lead. |

The independent blind scenario check and any opposition/review response are human gates;
no observations are claimed here.
