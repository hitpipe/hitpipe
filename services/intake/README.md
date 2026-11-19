# Intake

HIT-86 соединяет существующие части в локальный надёжный приём:
`internal/application/receivesignal` → `internal/adapters/postgres/receipt`
и существующие нормализацию/файловое хранилище. Нет публичного HTTP или отправки.

[Полный русский отчёт и ограничения](evidence/HIT-86/README.md).

Проверки текущего модуля на отдельной PostgreSQL18.7 и синтетических данных:

```sh
python3 services/intake/tools/check_receipt.py --go /path/to/go1.27.2/bin/go
```

Миграция и проверки HIT-71 переиспользованы; прежние docs/HIT-71 сохранены как история.
HIT-73 normalizesignal и AuthenticateWebhook сохранены, contentstore HIT-74 получил
только общий алиас типа Ref для соединения портов. Рабочие окружения не затрагиваются.
