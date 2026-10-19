# HIT-45 risk matrix

| Risk | Impact | Mitigation | Evidence |
|---|---|---|---|
| partial readback becomes verification | high | medium | false confirmation | incomplete outcome precedes match | `HIT-45-mutations-actual.log`, C1/C3 tests |
| provider error is treated as no-effect | high | low | unsafe retry or duplicate creation | unknown outcome, no retry in domain/application | VerifyTarget error test |
| Unicode or newline drift | medium | medium | false mismatch or lossy match | NFC + CRLF/CR normalization only | review regression tests |
| malformed handoff evidence | high | medium | untrusted confirmation | exact canonical v1 block validation | canonical/malformed tests and M2 |
| target evidence relabelled | high | low | confirmation attached to wrong task | FileReader target ID + provenance validation | DI contract test |
| external permissions not represented | high | low | overclaim of live state | saved-observation limitation documented | module contract and R1 security baseline |

Risk score is qualitative probability × impact for this offline module; checks run per
code correction and CI. Live provider permissions and hidden suites remain NOT PROVIDED.

This is a local candidate risk matrix, not a production security acceptance.
