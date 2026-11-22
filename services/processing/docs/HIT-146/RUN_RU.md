# Воспроизведение HIT-146

Из чистой копии Processing `717c954fca4f06f165074a8688f0e94a31b398e7` запустите квалификацию с отдельной чистой копией IAM `e1be0c1a0be577a3626a3e120707b3aeeb579c6f`, Go 1.27.2, Python 3 и локальным Docker с Unix-сокетом.

```sh
docker pull docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f
python3 -B services/processing/scripts/postgres/verify_real_iam.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --iam /absolute/path/to/clean-iam-checkout \
  --output /absolute/path/to/new-hit146-evidence
```

Сборщик проверяет точный IAM SHA и хеш миграции. Go overlay заменяет только тестовый входной main. Все пакеты IAM поступают из указанной копии. Стенд одноразовый, сеть контейнера отключена, процессы используют loopback и TLS 1.3. Рабочие учётные данные не нужны.

## Доверенная привязка запуска Processing

В доверенной локальной конфигурации параметр `boot_identity_file` задаёт абсолютный путь. Используйте принадлежащий системному пользователю процесса каталог с режимом 0700 (без симлинков) и обычный файл 0600. Четыре строки без пробелов по краям, с LF после каждой: runtime ID, десятичное поколение, заранее согласованный свежий boot ID, отпечаток исходящего TLS leaf в формате `sha256:<64 hex>`. Те же значения закрепляются в IAM. Обработчик отзыва использует отдельный закреплённый серверный сертификат. Не удаляйте отметку потребления boot и не выдавайте новый каталог для повторного использования старого boot. Для уже зарегистрированного старого экземпляра нужен штатный fencing и новое поколение по [инструкции IAM](https://github.com/hitpipe/hitpipe/blob/e1be0c1a0be577a3626a3e120707b3aeeb579c6f/services/iam/docs/HIT-146_RUNTIME_STARTUP_RU.md).

В `summary.json` ожидается PASS. `commands.json` содержит команды, каталог, UTC, exit codes и SHA потоков. `binaries.json` содержит SHA двух исходных версий и файлов сборки. Отсутствие тестового контейнера после очистки подтверждается ожидаемым exit 1 команды inspect.
