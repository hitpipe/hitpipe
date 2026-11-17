# Intake

`internal/application/normalizesignal` authenticates each bounded Stripe delivery
with the retained `authenticatewebhook.Consumer`, validates explicit installation,
account/mode/API-version and receipt metadata, then projects only the pinned
`NormalizedSignal` fields. It computes the message digest with RFC8785 JCS.

[HIT-73: результат, границы и воспроизведение](docs/HIT-73/README.md).

The signature policy, authenticator, synthetic fixture adapters and diagnostic CLI
are retained from exact PR #32 head e64f4ff4310c597414f768ea705b6ceb1221e1b2.
Their original module/test-design documents remain historical context. No closed
laboratory task or academic gate is reopened by this reuse.

This module has no webhook HTTP route, receipt storage, outbox sender, financial
admission or external business action. A normalized signal is neither a durable
receipt nor an HTTP acknowledgement. Production installation/credential/clock
and current authority integration remain separate work.

Run current checks with Go 1.27.2:

```sh
python3 services/intake/tools/check.py --go /path/to/go
```

The script records exact source commit, dirty state, source hashes, environment,
commands, stdout/stderr and exit codes under `services/intake/evidence/HIT-73`.
Only synthetic data is used. Existing CLI fixtures are diagnostic, not live keys.
