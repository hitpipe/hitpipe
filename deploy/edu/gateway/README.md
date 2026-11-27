# Рабочая область в запросе помощника

`assistant-workspace.patch` дополняет публичный шлюз HitPipe из выпуска от
25 ноября. Исходные Python-файлы находятся в
`/home/deploy/hitpipe-provider-self-service-20261125/gateway`. Их точные
SHA-256 сохранены в `nov25-gateway.sha256`. Этот пакет содержит исходники двух
переходов запроса, Cloudflare origin и аутентифицированного шлюза к локальному
BFF рабочей области.

Только точный `POST /api/v1/ai/proposals` принимает
`X-HitPipe-Workspace`. Публичный шлюз отклоняет повторённое или
неканоническое значение. После проверки сессии и CSRF шлюз приложения сравнивает
запрошенную область с областью аутентифицированного участника. Несовпадение даёт
HTTP 403. В локальный BFF отправляется идентификатор из подтверждённой области.
Запрос без заголовка сохраняет существующее поведение. Остальные маршруты
сохраняют свои списки пропускаемых заголовков.

## Воспроизведение во временной копии

```bash
set -euo pipefail
receipt=$(realpath deploy/edu/gateway/nov25-gateway.sha256)
change=$(realpath deploy/edu/gateway/assistant-workspace.patch)
source=/home/deploy/hitpipe-provider-self-service-20261125/gateway
candidate=$(mktemp -d /dev/shm/hitpipe-gateway.XXXXXX)
(cd "$source" && sha256sum --strict --check "$receipt")
while read -r digest file; do
  cp -- "$source/$file" "$candidate/$file"
done < "$receipt"
(
  cd "$candidate"
  patch --batch --forward -p1 < "$change"
  TMPDIR=/dev/shm python3 -B -m unittest discover
)
```

Перед переключением launcher запускается с проверенной отдельной копией
шлюза. Копия рабочей конфигурации, ключи, база сессий и файлы доступа для этого
патча не требуются.
