# HIT-45 risk matrix

| Risk | Impact | Mitigation | Evidence |
|---|---|---|---|
| partial readback becomes verification | false confirmation | incomplete outcome precedes match | C1/C3 tests |
| provider error is treated as no-effect | unsafe retry or duplicate creation | unknown outcome, no retry in domain/application | C1/C2 tests |
| Unicode or newline drift | false mismatch | NFC + CRLF normalization only | C1 test |
| malformed handoff evidence | untrusted confirmation | exactly-one delimiter validation | C1/C3 tests |
| external permissions not represented | overclaim of live state | saved-observation limitation documented | C2 module docs |

This is a local candidate risk matrix, not a production security acceptance.
