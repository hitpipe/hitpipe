# HIT-41 R1 — security baseline

Status: technical desk-research baseline; no live security acceptance is claimed.

1. Can evidence be relabelled to another target? No: `target_id` is checked at the file boundary and again by VerifyTarget.
2. Can malformed or lossy Unicode become a positive result? No: raw UTF-8 and surrogate escapes are rejected before decoding.
3. Can retrieval failure or contradictory retrieval metadata become a match? No: `unknown` takes precedence.
4. Can a malformed or duplicated handoff block become a match? No: exactly one canonical v1 block with matching case ID is required.
5. Can this module cause an external side effect? No: no network/provider/write/retry/state-transition dependency exists in the bounded implementation.
