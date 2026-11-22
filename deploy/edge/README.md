# HIT-141 · Приёмочные пробы внешнего Caddy

Тест читает [принятый шаблон](https://github.com/hitpipe/project-os/blob/84249ff3ba611358232294c46f6022f030ea223d/contracts/hitpipe/architecture-2026-10-16/formal/platform/Caddyfile.template)
через его побайтовое зеркало в `docs/labs/pr12/upstream/architecture/formal/platform/Caddyfile.template`.
Перед каждым запуском проверяется SHA-256. Второй набор правил для теста не
создаётся. Отдельно проверяется закреплённый официальный архив
[Caddy 2.11.7](https://github.com/caddyserver/caddy/releases/tag/v2.11.7);
до запуска тест извлекает только бинарник, фиксирует его SHA и отсутствие
дополнительных модулей, выполняет `caddy adapt` и `caddy validate` на том же шаблоне.

Запускайте только на отдельном тестовом Linux amd64 с локальным Docker, Python
3.12+, OpenSSL, C-компилятором для детектора гонок и Go 1.27.2. При наличии
проверенного архива сам стенд не требует соединений с Интернетом, не скачивает
образов и не публикует порты на хосте.

```bash
set -euo pipefail
test -z "$(git status --porcelain)"
export PYTHONDONTWRITEBYTECODE=1
work=$(mktemp -d)
asset=caddy_2.11.7_linux_amd64.tar.gz
curl --fail --location --proto '=https' \
  "https://github.com/caddyserver/caddy/releases/download/v2.11.7/$asset" \
  --output "$work/$asset"
printf '%s  %s\n' 727b91701a392de6ebc5027509f548bf39979e5216340d0faed8fa5e69c84f8b "$work/$asset" | sha256sum -c -
python3 deploy/edge/qualify.py --isolated-test --archive "$work/$asset" --evidence-dir "$work/evidence"
test -z "$(git status --porcelain)"
```

В этом прогоне внешний клиент подключён только к первой внутренней сети
стенда. Пробные IAM, BFF, Intake, Keycloak и активные TCP-слушатели
5432/8443/9443/9444 работают во второй сети. Только Caddy подключён к обеим.
Приватный клиент сначала подтверждает, что слушатели действительно работают;
внешний клиент проверяет недоступность тех же адресов и портов. Заглушка 5432
не запускает PostgreSQL. Имена `app.edge.test` и `idp.edge.test` используются
для HTTPS внутри стенда, рабочий DNS не меняется.

- `GET /auth/login`, `GET /auth/callback` идут в IAM, UI и API — в BFF,
  `POST /webhooks/stripe/{endpoint_id}` — в Intake. IdP пропускает только
  браузерные пути шаблона. Попадание неизвестного пути основного сайта в общий
  маршрут BFF фиксируется отдельно от прямого доступа к внутренним сервисам.
- На каждом допустимом маршруте проверяется удаление `X-HitPipe-*`. На
  Stripe-маршруте сравниваются SHA-256 тела и `Stripe-Signature` до и после
  проксирования; `Authorization` и `Cookie` не должны попасть в Intake.
  Тест не проверяет валидность подписи Stripe и не отправляет реальное событие.
- Приватные слушатели требуют TLS 1.3, IAM/BFF/Intake также требуют клиентский
  сертификат edge. Подтверждается реально согласованный TLS; входной минимум
  частных слушателей не приписывается реализации исходящего транспорта Caddy.
  В двух отрицательных прогонах меняется только сертификат пробного IAM:
  сначала доверенный издатель с другим DNS-именем, затем правильное имя с
  недоверенным издателем. Caddy запускается заново с тем же файлом и бинарником.

Результат — `verification.json` с SHA коммита, версиями, командами, итогами проб,
отметками о причинах отказа и проверкой очистки. Сырые HTTP-запросы и TLS-ключи
в отчёт не входят, Caddy-логи целиком не сохраняются. Одноразовые сети,
контейнеры и образ проверяются по метке запуска и удаляются.
