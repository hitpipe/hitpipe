# HIT-77: внутренний клиент чтения SourceFacts

`NewReader(origin, localPolicy, callerContext, transport, boundary, maxResponseBytes)`
создаёт клиент для единственного POST /internal/v1/source-facts/read. Он реализует
прежний SourceFactsReader без изменения Query, оценщика или финансовых правил.
`ReadProjection` возвращает тот же read плюс Provenance из Decode HIT-72; `Read`
возвращает Snapshot существующему финансовому consumer. Общего изменяемого
«последнего результата» нет. Для сохранения provenance вызывающая композиция
использует ReadProjection; EvaluatePaidOrder по-прежнему возвращает финансовое знание.

Источник wire-формы: project-os@fedbe40bb597e25f66aeb1172af3ff7459c81b40,
contracts/hitpipe/mvp-2026-10-31/formal/mvp-openapi.json,
SHA256 a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e.
Из SourceReadRequest сгенерированы request_generated.go и используемые пределы
session_id/expected_epoch. В теле ровно workspace_id, connection_id, session_id,
case_id, expected_epoch, deadline_at; X-Request-ID передаётся обязательным заголовком.
Policy и её локальные config/schema/evaluator pin не отправляются как аттестация.

CallerContext должен быть уже установлен вызывающим кодом. Его заполнение не
означает проверки IAM. Workspace/request/case/connection UUID, непустая session,
положительный безопасный epoch, deadline и известный test/live обязательны. Query
должен совпасть с привязанными case/session/account/connection/config/test-live.
Deadline применяется к запросу, включая чтение body; более ранняя отмена родителя
сохраняется. Никакой фиктивной сессии/epoch и production-origin по умолчанию нет.

Требуется явно переданный проверенный http.Transport с TLS1.3, отдельным набором
корней, клиентским сертификатом и стандартной проверкой имени/цепочки. Insecure TLS,
Proxy и DialTLS обходы отвергаются. Исходный transport клонируется; его DialContext
сохраняется под start-boundary. Предоставленные credentials/callbacks остаются
доверенными неизменяемыми зависимостями на срок использования Reader.

StartBoundary — только интерфейс к общему runtime guard. Реализации координатора
или разрешающей заглушки в production-коде нет. Граница вызывается вокруг connect
и каждой записи в реальный socket, включая TLS records. Только предварительный
check перед RoundTrip оставил бы разрыв. Callback синхронный, одноразовый и не может
сохраниться после возврата. Connect/write ограничены <=2s по A012; весь read —
явным caller deadline. Boundary должна сериализовать start с действительным
revoke/scope/epoch и соблюдать этот контракт; клиент не доказывает её реализацию.

Каждый read использует отдельный HTTP/1 transport/connection без keepalive/HTTP2,
compression, redirects или replayable GetBody. Полный ответ проходит прежний Decode.
HTTP-status доступен отдельно в ReadError, ошибка не содержит body/URL/identity;
cancel/error возвращают пустую Projection, а consumer получает INCOMPLETE.

Пределы не подменяются: OAS ограничивает отдельные поля, но не устанавливает общего
байтового максимума SourceFacts. maxResponseBytes — обязательная частная граница
буфера проверенного транспорта без значения по умолчанию. Она не объявляется новым
продуктовым/OAS-порогом. В стенде значение задаётся только для синтетического body;
проверены Content-Length и chunked превышения, а отдельно session_id>255 по OAS.
Реальная композиция обязана предоставить свою установленную конфигурацию транспорта;
её production-значение этой задачей не выбирается. Отсутствие bound закрывает клиент.

## Воспроизведение

Из services/processing с Go1.27.2 на PATH:

```sh
go test -race -count=1 ./...
go vet ./...
python3 tools/sourcefacts/generate_request.py /path/to/exact/mvp-openapi.json
git diff --exit-code -- internal/adapters/intake/sourcefacts/request_generated.go
```

HTTPS/mTLS стенд генерирует временный тестовый CA и ключи только в памяти; сервер
слушает loopback на временном порту (не qualification рабочего listener8443).
Проверяются wire-форма, полный контекст, сертификаты/неверный peer, actual-start
отказ до connect и после TLS verification, redirect/drop без retry, HTTP401/403/
429/503, schema/malformed/buffer-size, timeout/cancel/join и отсутствие старого
результата, параллельное владение значениями, четыре финансовых исхода. Fixture gate
в _test.go — модель зависимости, не общий coordinator и не production authority.

Результат: проверен клиент на управляемом стенде. Настоящие IAM/Intake/Stripe,
конфигурация live transport/gate, digest correctness и финансовая подлинность не
подтверждены. Нет запуска сервера продукта, case/outbox/activation или внешнего
эффекта. VALID остаётся финансовым знанием. Сохранённый digest не становится
проверенным вследствие Decode. Источник HIT-72 и его тесты переиспользованы без правок.
