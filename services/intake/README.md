# Intake

HIT-117 реализует приватный readSourceFacts для Processing: строгий mTLS HTTP,
ограниченный read-only Stripe transport и собственные PostgreSQL/content adapters.

[Русский результат, границы и инструкция](evidence/HIT-117/README.md).

```sh
python3 services/intake/tools/check_sourcefacts.py --go /path/to/go1.27.2/bin/go --output /tmp/source-facts-verification
```

Нужен работающий Docker с точным образом PostgreSQL18.7 из runner. Только собственные
управляемые HTTPS/SQL/files fixtures; никаких рабочих keys или env DSN.
Authority, credential и shared actual-start gate обязательны и не имеют разрешающего default.
HIT-71 DDL и HIT-74 contentstore переиспользованы без изменения. Приём webhook,
outbox sender, реальный installation/IAM/runtime и deploy не входят в этот slice.
