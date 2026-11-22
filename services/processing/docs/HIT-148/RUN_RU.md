# Воспроизведение HIT-148

Прогон чтения реестра и регрессий Processing. Нужны чистые рабочие копии Processing и IAM `c715a079ab57d4c9e7b56862f7f2584c792b74b0`, локальный Docker и Go 1.27.2. Из корня копии Processing выполните

```sh
python3 -B services/processing/scripts/recovery/verify_deletion_inventory.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --iam /absolute/path/to/clean-iam-checkout   --output /absolute/path/to/new-hit148-evidence
```

Требуются закреплённый образ PostgreSQL 18.7 из `scripts/postgres/verify.py`, отдельные одноразовые базы и сеть Docker `none`. Сборщик сопоставляет SHA IAM, хеш SQL и сохраняет хеши бинарников. Go overlay добавляет только интеграционный тест и адаптирует исходный TEST-стенд IAM для Unix-сокета и числовой проверки версии PostgreSQL.

Команда потребителя запускается отдельно через `processing-purge --local-stand --verify-deletion-inventory /absolute/path/to/config.json`. Доверенная настройка указывает `origin`, `server_pin`, `publisher_id`, `workspace_id`, `generation`, `ca`, `client_certificate`, `client_key` и `publisher_public_key`. Файлы должны лежать по абсолютным путям без симлинков, принадлежать пользователю процесса и иметь режим 0600. Открытый ключ издателя задаётся PEM PKIX `PUBLIC KEY`. В стенде PKI и файлы генерируются автоматически. Исходные тела, ключи и сессии не попадают в stdout.

Успешный код 0 подтверждает только проверку снимка и сообщает рабочую область, поколение, ревизию и количество решений. Отсутствие источника, закрытая область, чужой TLS клиент, неправильный пин или подпись дают код 1. Неверная локальная конфигурация даёт код 2. Команда не использует PostgreSQL Processing или каталог содержимого и не принимает параметр case ID. Дальнейшее открытие Serving требует доверенного координатора из [опубликованного анализа](verification/INTEGRATION_BOUNDARY.md).
