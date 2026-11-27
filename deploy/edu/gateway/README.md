# Рабочая область в запросе помощника

`assistant-workspace.patch` дополняет публичный шлюз HitPipe из выпуска от
25 ноября. Исходные Python-файлы находятся в
`/home/deploy/hitpipe-provider-self-service-20261125/gateway`. Их точные
SHA-256 сохранены в `nov25-gateway.sha256`. Этот пакет содержит исходники двух
переходов запроса, Cloudflare origin и аутентифицированного шлюза к локальному
BFF рабочей области.

Только точный `POST /api/v1/ai/proposals` принимает
`X-HitPipe-Workspace`. Публичный шлюз отклоняет повторённое или
неканоническое значение. Параметры и пустой `?` дают HTTP 400 до передачи
запроса шлюзу приложения. У шлюза приложения уже есть самостоятельная проверка,
отклоняющая такой запрос до обращения к IAM или BFF. После проверки сессии и CSRF
шлюз приложения сравнивает
запрошенную область с областью аутентифицированного участника. Несовпадение даёт
HTTP 403. В локальный BFF отправляется идентификатор из подтверждённой области.
Запрос без заголовка сохраняет существующее поведение. Остальные маршруты
сохраняют свои списки пропускаемых заголовков.

## Воспроизведение во временной копии

```bash
set -euo pipefail
export TMPDIR=/dev/shm
receipt=$(realpath deploy/edu/gateway/nov25-gateway.sha256)
change=$(realpath deploy/edu/gateway/assistant-workspace.patch)
source=/home/deploy/hitpipe-provider-self-service-20261125/gateway
candidate=$(mktemp -d "$TMPDIR/hitpipe-gateway.XXXXXX")
(cd "$source" && sha256sum --strict --check "$receipt")
while read -r digest file; do
  cp -- "$source/$file" "$candidate/$file"
done < "$receipt"
(
  cd "$candidate"
  patch --batch --forward -p1 < "$change"
  python3 -B -m unittest discover
)
```

Перед переключением launcher запускается с проверенной отдельной копией
шлюза. Копия рабочей конфигурации, ключи, база сессий и файлы доступа для этого
патча не требуются.

## Прямой маршрут Cloudflare от 27 ноября

`direct-cloudflare-guest.patch` накладывается после `assistant-workspace.patch`
из `9a56a34`. `school_public_origin.py` рабочего выпуска имеет SHA-256
`7d5c5daef3950ffa9bca6a1147b74594c07b2190af719ff998f1311310fefcf6`.
Скопируйте проверенные модули шлюза в отдельный каталог, примените `patch -p1`
и добавьте `school_guest_snapshot.py` и `test_school_guest_snapshot.py`.
Манифест `gateway-direct.sha256` со сверенными файлами запуска хранится
в каталоге подготовленного кандидата `qa/incident-20261127/direct-cloudflare-candidate`.

Гостевые GET и HEAD получают HTML из проверенного набора артефактов.
Ответы HTML имеют `private, no-store`. Сессия известного формата поступает
владельцу прежним путём. Статические файлы сборки отдаются по точным путям
из манифеста с неизменяемым кэшем, логотип имеет ограниченный срок кэша.
`--require-cloudflare-https` принимает заголовок схемы на отдельном listener
от точного адреса Cloudflare connector. HTTP GET и HEAD перенаправляются на
канонический HTTPS, операции по HTTP получают 403. Отсутствующая или
неоднозначная схема даёт 400.

```bash
cd /home/deploy/hitpipe-mvp-20261127/qa/incident-20261127/direct-cloudflare-candidate
sha256sum --strict --check gateway-direct.sha256
TMPDIR=/dev/shm python3 -B -m unittest test_school_public_origin test_school_guest_snapshot
```

`run-direct-cloudflare-origin.sh` принимает каталог кандидата, каталог
артефактов инцидента, IPv4 действующего Cloudflare connector и путь к тому же
CA, который использует основной listener. Кандидат слушает порт 19485.
В туннеле правило Stripe `/webhooks/stripe/...` должно предшествовать общему
правилу домена и вести в прежний Stripe ingress. DNS и правила туннеля
переключаются отдельно с сохранением пути отката.
