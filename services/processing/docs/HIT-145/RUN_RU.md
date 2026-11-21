# Контейнер Processing

Образ запускает существующий `cmd/processing` с UID/GID `10001:10001`. В финальном слое находится только исполняемый файл. Конфигурация, TLS-материалы, ключ курсора и DSN подаются при запуске. Схема базы устанавливается отдельно принятым мигратором.

## Воспроизведение в отдельной копии

Нужны Git, Python 3.11+, Go 1.27.2, локальный Docker Engine с Unix-сокетом и Buildx, а также доступ к Docker Hub и исходникам Go-модулей для первой сборки. Поддерживаются нативные Linux-платформы `arm64` и `amd64`. Проверенная платформа и SHA указаны в [итоге](README.md).

Из корня чистого checkout проверенного SHA выполните

```sh
docker pull docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_image.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --output /absolute/path/to/new-evidence-directory
```

Runner создаёт выделенную PostgreSQL 18.7 с сетью `none`, устанавливает принятую миграцию, строит образ, выгружает и читает локальный OCI-архив, удаляет тестовый тег, загружает архив обратно и дважды запускает Processing. TEST использует только закрытый Unix-сокет выделенного PostgreSQL и управляемых соседей на loopback. Для обоих контейнеров совпадают образ, база и собственный том содержимого. Сгенерированные сертификаты и конфигурация подаются отдельным томом только для чтения. По завершении runner удаляет созданные контейнеры, тома и тестовый тег. В выходном каталоге сохраняются `processing-image.tar`, `image-identity.json`, `summary.json`, оригинальные потоки и `commands.json` с аргументами, cwd, временем, exit code и SHA-256.

Для отдельной сборки из того же SHA выберите нативную архитектуру, например `linux/arm64`, и укажите локальный тег

```sh
source_sha=$(git rev-parse HEAD)
docker buildx build --load --platform linux/arm64 --provenance=false --sbom=false \
  --build-arg "SOURCE_SHA=$source_sha" --build-arg SOURCE_DATE_EPOCH=0 \
  --metadata-file /absolute/path/to/build-metadata.json \
  --file services/processing/Dockerfile --tag "hitpipe-processing:$source_sha" .
docker image save --output /absolute/path/to/processing-image.tar "hitpipe-processing:$source_sha"
```

На другом стенде той же платформы сверьте SHA-256 полученного файла с записью отправителя перед загрузкой. `archive_sha256` относится ко всему tar-файлу, `manifest_digest` — к платформенному манифесту, а `config_digest` соответствует локальному `docker image inspect --format '{{.Id}}'`.

```sh
docker image load --input /absolute/path/to/processing-image.tar
docker image inspect --format '{{.Os}}/{{.Architecture}} {{.Id}} {{index .Config.Labels "org.opencontainers.image.revision"}}' "hitpipe-processing:$source_sha"
```

## Запуск с внешними входами

Скопируйте [пример](processing.example.json) во внешний каталог `config` под именем `processing.json`. Подставьте собственные приватные IP listeners и адреса соседей. Порты 8443, 9443 и 9444 закреплены за бизнес-API, control и OPS Health соответственно. Пример привязан к loopback и годится как шаблон. Для приватной сети стенда укажите фактический приватный IP, доступный процессу. Выдайте UID/GID 10001 право чтения на конфигурацию и TLS. DSN, закрытые ключи и 32-байтный ключ курсора должны быть приватными файлами, доступными только владельцу. Подготовьте отдельный том, корень которого принадлежит UID/GID 10001 и имеет права `0700`.

```sh
docker run --pull=never --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --network "$private_network" \
  --mount "type=bind,source=$runtime_files/config,target=/etc/processing,readonly" \
  --mount "type=bind,source=$runtime_files/tls,target=/run/tls,readonly" \
  --mount "type=bind,source=$runtime_files/secrets,target=/run/secrets,readonly" \
  --mount "type=volume,source=$processing_content,target=/var/lib/hitpipe/processing-content" \
  "hitpipe-processing:$source_sha"
```

В полноценном окружении требуются следующие входы.

- Собственная PostgreSQL 18.7 с принятой схемой. Отдельные DSN для `hp_processing_app` и `hp_processing_worker` указывают на одну базу и один узел. В обычном режиме требуется проверка сертификата и имени PostgreSQL.
- Разные TLS 1.3/mTLS сертификаты Processing для сервера и исходящего клиента с соответствующими назначениями и ключами. Входящие BFF, OPS, IAM и Intake используют отдельные технические идентичности. Для исходящих IAM, Intake и Delivery нужны доверенный CA и точные SAN. Для бизнес-запроса BFF дополнительно нужны действующая пользовательская сессия и CSRF там, где это предусмотрено API.
- Для открытия допуска нужен свежий доверенный снимок IAM для активной загрузки процесса, актуальный состав воркеров, подтверждённые сессия/CSRF и привязки, а также поколение удержанной очереди. Проверяйте технический статус по `GET /internal/v1/health/processing` на OPS-порту с разрешённым OPS-сертификатом. `CLOSED` с HTTP 200 означает, что Health отвечает и допуск закрыт. `READY` требует авторитетных входов.

`-stand-config` используется только автоматическим изолированным TEST из этого документа. Ограничения штатного режима и принятые входы описаны в [инструкции HIT-111](../HIT-111/RUN_RU.md).
