# Воспроизведение HIT-148

Из чистой копии Processing `107ed4e3ae0d43300a09dea10b2d43ad4423efc1` со второй чистой копией IAM `c715a079ab57d4c9e7b56862f7f2584c792b74b0`, Go 1.27.2, Python 3 и локальным Docker выполните

```sh
python3 -B services/processing/scripts/recovery/verify_deletion_inventory.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --iam /absolute/path/to/clean-iam-checkout \
  --output /absolute/path/to/new-hit148-evidence
```

Нужен образ `docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f`. Сборщик использует только отдельный контейнер c сетью `none`. Он проверяет хешы миграций и точный чистый IAM SHA. Go overlay включает тестовую композицию и локальную привязку TLS/Unix-сокета. Модуль Processing и сохранённый Platform pin берутся из соответствующих зафиксированных рабочих копий. Сборщик также выполняет обычный Go race, vet, build и регрессии HIT-126/HIT-128.

## Применение

Сначала принятая процедура IAM подтверждает владельца, фиксирует точные версии и сохраняет APPLYING. Контур Processing использует `processing-purge --local-stand --apply-deletion-inventory /absolute/path/to/issuer.json --content-dir /absolute/path/to/private-volume`. Роль в локальной `HITPIPE_PURGE_DSN` — существующая ограниченная `hp_processing_purge`. Для `issuer.json` используются `origin`, `server_pin`, `publisher_id`, `workspace_id`, `generation`, `ca`, `client_certificate`, `client_key`, `publisher_public_key`. Временные файлы доверия принадлежат тестовому процессу, режим 0600. Открытый ключ издателя — PEM PKIX. Оператор не вводит case ID или версии в CLI. Перед unlink реальный авторитетный источник и SQL подтверждают точный список. Ошибка при передаче квитанции разрешается запуском на том же источнике и поколении. Уже применённые решения проверяются без повторной отправки POST.

## Restore

Супервизор закрепляет одно восстановленное пространство, фактический каталог, PostgreSQL и целевое поколение. `restore_case_deletion: true` заставляет штатный Processing запускаться закрытым, пока доверенная локальная композиция не подключит `maintenance.CaseAuthority.ServeRestored`. Супервизор читает живой полный снимок и входит в реальный `Registry.WithCurrent`. Его синхронный callback сверяет каждую действующую квитанцию с локальным устойчивым доказательством и применяет обычный свежий IAM-снимок к фактическому gate. Публикация изменений заблокирована до завершения проверки и открытия. При недоступности и несоответствии поколений или ревизий Protected API недоступен.

Сырой результат содержит stdout, stderr, фактические команды и `binaries.json`. Изолированные описание/композиция воспроизводят TEST, развёртывание или политика конфигурации рабочих окружений в прогоне не проверяются.
