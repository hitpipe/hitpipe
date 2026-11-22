# Повторить сборку и восстановление HIT-155

В отдельной чистой копии репозитория используйте исходник `1ed91653a747b47a98f2738ca0d831a38cebc98a`. Потребуются Git, Python 3.11+, Go 1.27.2, локальный Docker Engine с Unix-сокетом и Buildx. Для первого запуска нужен доступ к закреплённым образам и Go-модулям. Сохраните результат во вновь создаваемый каталог вне checkout.

```sh
git switch --detach 1ed91653a747b47a98f2738ca0d831a38cebc98a
docker pull docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f
env -u DOCKER_HOST -u DOCKER_CONTEXT PYTHONDONTWRITEBYTECODE=1 \
  python3 services/processing/scripts/recovery/verify_oci_recovery.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --output /absolute/path/to/new-hit155-evidence
```

Сценарий собирает и экспортирует образ, проверяет архив, затем запускает загруженный образ по проверенному идентификатору для обоих тестов. Команды в `commands.json` содержат аргументы, каталог, UTC, exit code и контрольные суммы потоков. В `image-identity.json` сравнивайте SHA исходника, архитектуру, манифест, конфигурацию, слой и SHA бинарника. Экспортный tar имеет свою контрольную сумму. При разных временных тегах его index и manifest содержат разные имена, хотя байты образа совпадают.

Стенд создаёт исходный PostgreSQL без внешней сети и использует временный второй кластер с отдельным system identifier. Оба на закреплённом PostgreSQL 18.7. Процессы Processing работают с UID/GID 10001, read-only root и отдельным томом. Фикстура создаёт временные входы и управляемых mTLS-соседей. Инструмент на хосте запускает и останавливает контейнеры. Сокет Docker внутрь контейнеров не передаётся. По завершении удаляются собственные контейнеры, временные тома и тестовый тег.

Сверьте в stdout тестов результаты `RESTART PASS` и `RECOVERY PASS`, ответы OPS Health с `CLOSED` и `READY`, одинаковые `source safety` и `restored safety`, один create и число query до и после. Отдельно сверяются удержанный intent, снятая аренда, исходный срок, совпадение квитанции Intake и отсутствие второго эффекта. Проверенный прогон и границы приведены в [основном отчёте](README.md) и [исходных журналах](verification/raw-execution.tar.gz).
