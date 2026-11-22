# Воспроизведение HIT-146

Нужны чистые рабочие копии одного репозитория. Для Processing используется `ce317193244623c5a464a3014bb39f32875c1e98`. Для IAM используется `ae5fd0f35ec7d5097cace75b3b15ecb14042c654`. Потребуются Go 1.27.2, Python 3 и локальный Docker с Unix-сокетом. Из корня копии Processing выполните

```sh
docker pull docker.io/library/postgres:18.7-bookworm@sha256:ece55f7868da684e400f82f1030a050718adbdd65c23aa500c486f6e2950dc0f
env -u DOCKER_HOST -u DOCKER_CONTEXT PYTHONDONTWRITEBYTECODE=1 \
  python3 services/processing/scripts/postgres/verify_real_iam.py \
  --go /absolute/path/to/go1.27.2/bin/go \
  --iam /absolute/path/to/clean-iam-checkout \
  --output /absolute/path/to/new-hit146-evidence
```

Runner сверяет IAM SHA и хеш принятой миграции. Для сборки тестового процесса используется Go overlay только на входном файле `main.go`. Пакеты IAM остаются из проверенной копии. Тестовый вход находится в `tools/hit146` и не входит в обычную сборку. Мигратор, PostgreSQL, ключи, рабочие области и сессия одноразовые. Сеть контейнера `none`, сервисы общаются на loopback по TLS 1.3. Рабочий DSN или сертификаты не требуются.

В сводке `summary.json` ожидается `BLOCKED_REPRODUCED`. В полном stdout ищите две разные загрузки Processing с `CLOSED`, настоящий `SYNCING` от IAM для точной привязки, `PENDING` и `INCIDENT` без EFFECTIVE. PASS теста означает воспроизведение отрицательных наблюдений. Он не заменяет положительные критерии HIT-146.

`commands.json` содержит команды, каталог, даты UTC, exit codes и SHA потоков. `binaries.json` содержит SHA обеих версий и бинарников. Предыдущий HIT-111 тест с управляемым IAM идёт отдельно. Тестовый контейнер удаляется, отсутствие проверяется командой inspect с ожидаемым exit 1.
