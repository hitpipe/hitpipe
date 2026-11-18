# HIT-117 — защищённое чтение актуальных фактов Stripe

Реализован приватный `POST /internal/v1/source-facts/read` для Processing по mTLS. Он читает актуальные факты через ограниченный Stripe HTTPS-адаптер, сохраняет минимальную SourceFacts-проекцию в собственных файлах и её метаданные в принятой PostgreSQL-схеме Intake. Webhook, receive HIT-86/HIT-107, новый case, signals/outbox и внешние записи не вызываются.

**Проверенный чистый код:** `17cc3c46a102251c07262d1c2bbb4d5f6ca01e6f`. Настоящий итоговый прогон:7команд exit0, unit-race25PASS test/subtest events, integration-race86PASS (включая unit),0failed/0skipped tests,377SQL checks. [Фактический протокол](test-transcript.txt), [команды и SHA256 исходников](commands.json), [сырые integration events](integration.stdout). Серверный READY, независимое ревью и человеческая приёмка этим отчётом не выставляются.

## Переиспользование и границы

Исходный main `ce771d37505e7770003d0796ae932556d06b7bab` прочитан непосредственно. HIT-71 и HIT-74 в Jira Done; PR47/66 на чтении открыты и не слиты. В сборку включены exact SHA `9af6ba6d1b1619d4f3df04059adc186cfaa5c01a` и `22a99989f1f4fb3a73747647eef98c20b508cd0c`: SQL и HIT-74 store/test bytes сохранены. Из schema checker изменён только readiness-transport на localhost TCP, чтобы не принять временный startup Unix server за готовую БД. Вызывающий HIT-77 прочитан по `b31a3adae4fac625c97fd903e13f9aad8d6ca771`; его код и финансовый kernel не изменены.

Принятый OAS/DDL и внешний контракт не менялись. `schema.json` — точное извлечение SourceReadRequest/SourceFacts из pinned OAS; Go types генерируются и проходят drift check, runtime дополнительно проверяет строгую форму. Реестр источников, SHA reuse и изменённые пути — [source-audit.json](source-audit.json).

## Проверка доступа и сетевой старт

`http/sourcefacts.New` создаёт только приватный listener (адрес конфигурации, обычный порт8443; тесты используют ephemeral loopback). TLS≥1.3, явные CA и серверный сертификат, RequireAndVerifyClientCert, ровно разрешённый Processing DNS SAN, обычная проверка цепочки/expiry. Сертификаты другого issuer, другого service SAN и просроченный отвергаются. Нет browser identity/role headers, public webhook/control/health роутов. X-Request-ID, path/method/content-type/body, точные поля, UUID, safe integer epoch и deadline проверяются до успешного ответа. Отдельный RFC3339 wire regex отвергает допускаемые Go parser расширения: timezone24:00/00:60, comma-fraction и час из одной цифры. Четыре регрессии и реальный HTTP отказ до сети проверены.

mTLS определяет service caller, но не выдаёт business authority. Обязательный `Authority.Current` получает из отдельной проверенной композиции workspace/connection/account/test-live/epoch/credential ref/API version/revision/ready. Он обязан проверить также принадлежность Session/case и текущий grant. `Revision` связывает изменения grant/installation/credential. Отсутствующий интерфейс, неизвестный контекст и любое изменение снимка закрывают операцию. Конфигурация не выводится из полей запроса или ответа Stripe.

`Authority.Start` — обязательный общий текущий runtime/epoch guard, исполняющий callback синхронно, один раз, под тем же барьером. Он применяется к protected SQL и началу защищённого ответа. Stripe transport применяет guard при socket connect и каждом raw socket Write, включая TLS records: одно предварительное разрешение не считается допуском будущего запроса. SQL-проверка READY/unrevoked/verified account/mode/epoch выполняется перед каждым provider read и снова при сохранении/загрузке. SQL-транзакции не удерживаются при сети или fsync. Callback/gate операции ограничены2сек; runtime coordinator за пределами задачи обязан выполнить этот контракт и CLOSED/re-registration после restart.

Ответ с фактическими SourceFacts записывается и принудительно flush под тем же guard с write deadline не позже valid_until/request deadline. Retrying, общих credentials/facts cache, разрешающего default или обхода отсутствующего authority нет.

## Какие данные действительно читаются

Разрешены только GET к одному явно заданному HTTPS origin: `/v1/account`, известная Session, её line_items, известный Price, связанный PaymentIntent, его latest Charge и списки refunds/disputes с фильтром по тому же Charge. Origin, trust roots, credential port и ресурсные границы обязательны. Не доверяется URL пагинации от провайдера: строится тот же path/filter с starting_after. Redirects не выполняются; новый отдельный HTTP/1 transport/connection для запроса и отсутствие keepalive исключают скрытый replay/reuse. TLS hostname/CA проверяются; proxy и insecure/custom TLS dial запрещены. Запрос несёт только явно полученный read credential, проверенный account и API version. Перенаправление и 429 не повторяются.

Проверяются account ID, Session/PI/Charge/Price identity, livemode, PI↔Charge/refund/dispute joins. У line item/refund нет выдуманного поля livemode: их связь устанавливается Session path/Price и точным Charge в проверенном account; Dispute livemode проверяется отдельно. Неиспользуемые provider поля допускаются только как входные данные провайдера и никогда не проецируются в доказательство. Дубли ключей/невалидный JSON/UTF-8 отвергаются, важные значения не преобразуются из строк в числа.

Наблюдаемая проекция включает Session.created, mode/payment_status, сумму и валюту, полноту/count line items, quantity и наблюдённый fixed Price. PriceID устанавливается только после one_time/per_unit, наличия unit_amount и явного отсутствия recurring/custom_unit_amount/transform_quantity. Это не сравнение с финансовым правилом Processing: тот остаётся владельцем решения. Положительные и отрицательные известные факты сохраняются одинаково; complete не означает «заказ допущен».

Для связанного платежа прочитаны PI status/amount_received; Charge status+paid/captured/amount_captured/amount_refunded/disputed; полные связанные refund/dispute lists. Сумма состоявшихся refunds сверяется с Charge, pending/неизвестный/неполный ответ не становится нулём. Найденный dispute не превращается в отсутствие; противоречие снимков сохраняет incomplete. Currency сравнивается по всем обязательным прочитанным объектам. Неизвестное, null/отсутствие и неподдержанная форма не заменяются ожидаемым положительным значением. При нескольких line items сохраняются известные полнота/count, но не выдумывается единственный Price/quantity. Это не финансовое решение и не атомарный snapshot нескольких Stripe объектов.

## Время, полнота и checksum

Снимок не строится из webhook. `oldest_observed_at` консервативно берётся перед первым обязательным provider read; это не позднее фактического наблюдения. completed_at берётся после чтений. Timestamp precision ограничена вниз до PostgreSQL microsecond, а `valid_until=min(oldest+10сек,исходный deadline)` дополнительно округлён вниз. Если к публикации срок прошёл, успех не возвращается. Все mandatory факты/контексты/списки должны быть известны для complete; ограниченная пагинация/unknown не скрываются.

Wire `digest` — `sha256:` от RFC8785 JCS всей SourceFacts без самоссылочного поля digest. При Encode/Decode он проверяется. Отдельно HIT-74 file digest покрывает все сохранённые bytes, включая wire digest; именно content checksum находится в SQL `source_facts.digest` и Ref. Это различие не прячет payload в SQL. В OAS SourceFacts нет workspace_id: привязка workspace сохраняется в проверенном Request, SQL key/RLS и workspace-only content root, без добавления wire-поля.

## PostgreSQL и файлы

После текущего scope check минимальная проекция сохраняется неизменённым HIT-74: file fsync→atomic rename→directory fsync. Считываемые bytes проверяются до SQL. Затем короткая транзакция исходной source_facts с повторной блокировкой/проверкой source_connection; исходные роли и forced RLS, SET LOCAL workspace, параметризованные запросы, statement/lock/transaction timeout2сек. App pool bounded max10; superuser/BYPASSRLS/мигратор отвергаются. В SQL только opaque keys/ref/hash/size/completeness/time/expiry; raw provider payload, customer fields и read key не записываются.

HIT-74 используется через отдельный root/workspace с0700 и файлами0600; пути/ссылки защищает исходный os.Root store. Retention задаётся явно владельцем и ограничен90сут от completed, независимо от пригодности фактов≤10сек. Стенд использует1час как тестовую настройку, не новый продуктовый срок. После commit выполнены независимая SQL metadata load, чтение файла, content hash/JCS digest и сверка exact case/session/connection/account/mode. Переоткрытый pool/метод находит сохранённый результат; чужой workspace не читает его. Истёкшее, отсутствующее или повреждённое содержимое не выдаётся как пригодное. При ошибке до SQL возможен узнаваемый orphan; существующий Purge требует полного authoritative reference snapshot и исключения конкурентного publisher. Автоматическое расписание purge/retention и квалификация рабочего диска здесь не реализованы.

## Что проверено настоящим запуском

Управляемые TLS listeners, client certificates и Stripe HTTPS сервер, а не fake RoundTripper; отдельная собственная PostgreSQL18.7 Alpine exact image, роли HIT-71, temporary content directories, fsync/full_page_writes/synchronous_commit=on. Нет рабочих аккаунтов/keys. Проверены запрос/сертификаты/границы контекста, safe integer semantics, подпорченные ответы/duplicates/UTF-8/content-type/encoding/size, неизвестные обязательные поля, known negative payment/capture/refund/dispute, fixed/recurring Price, валюта/identity, полный и обрезанный cursor flow, 429/302/307/500 без retry/redirect, cancellation/timeout, revoke после credential lookup до socket connect, смена epoch и реальное DB revoke между чтениями, protected response gate, concurrent workspace isolation, file put/read/corruption/removal, закрытый реальный DB pool после file durability, expired facts/deadline и отсутствие signals/outbox writes. Сохранённые SourceFacts содержат ровно28 OAS полей; raw marker/credential отсутствуют в ответе, SQL и файле. Ошибки и серверные logs не раскрывают provider/dependency bodies.

Go1.27.2 darwin/arm64, macOS; PostgreSQL18.7 Alpine index `sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73`. Точный host/Python/команды/выходы/hashes — commands.json. Временный контейнер удалён. Schema377 проверок повторены на текущем DDL. Прежние тесты HIT-74 сохранены. Все7команд завершены0 на чистом `17cc3c46a102251c07262d1c2bbb4d5f6ca01e6f`; source hashes совпали после выполнения. CI отдельно проверяет фактический PR head и не заменяет эти локальные протоколы независимым ревью.

## Воспроизведение и включение

```sh
python3 services/intake/tools/check_sourcefacts.py --go /absolute/path/to/go1.27.2/bin/go --output /tmp/hit117-check
```

Нужен работающий Docker и точный заранее загруженный image. Runner создаёт только собственный loopback PostgreSQL sandbox, игнорирует внешние DSN, применяет исходную миграцию мигратором, предоставляет app fixture отдельную роль, выполняет generation/race/integration/vet/build/modules/schema и удаляет стенд. Генерация: `python3 services/intake/tools/generate_sourcefacts.py --check --gofmt /path/to/gofmt`. Схема SQL не редактируется. Старые учебные задачи не запускаются.

Для приватного сервера композиция явно создаёт Repository/Content/Stripe Client/Service и `http/sourcefacts.New(privateAddr,processingDNS,tlsConfig,service,bodyLimit)`, затем Serve на приватном listener. Без реальных Authority/Credentials/Start wiring production serve не предоставляется «разрешающим» примером. Локальные operational bounds стенда: HTTP request8192B/headers16KiB, Stripe response65536B,4pages/300items; transport operations2сек. Это ограниченные настройки потребления ресурсов, не новые финансовые критерии или обещание обслуживания любого провайдерского ответа. При превышении — ошибка/неполнота, не усечённый complete.

## Ограничения и один следующий шаг

Установлено поведение управляемого источника и правильность локального fail-closed соединения. Настоящие installation/grant/credential, общий runtime barrier/registration, actual provider/API compatibility в рабочем аккаунте, SQL production TLS, disk/power-loss/backup, deployment и Stripe→ClickUp не квалифицированы. SourceFacts не даёт финансового допуска и не является durable webhook ACK. Персональные данные и рабочие секреты для проверки не требовались.

До применения pinned Go проверена [GO-2026-6859](https://vuln.go.dev/ID/GO-2026-6859.json), оригинал сохранён в GO-2026-6859.json: закреплённая1.27.2 затронута гонкой HTTP/1 MaxBytesReader/MaxBytesHandler при одновременном чтении body и записи headers. Этот handler использует последовательный LimitReader и flush, без обоих MaxBytes API и без такого параллелизма. Pin не менялся; специальный CVE exploit/security qualification не выполнялся, безопасность cohort в целом не заявляется.

Следующий шаг после server registration и свежего READY — ответственному участнику лично перенести HIT-117 в In Review для независимого ревью результата с этими ограничениями.

## Первичные источники Stripe

Поведение GET и связанных объектов прочитано в официальных источниках: [Session](https://docs.stripe.com/api/checkout/sessions/retrieve), [line items](https://docs.stripe.com/api/checkout/sessions/line_items), [PaymentIntent](https://docs.stripe.com/api/payment_intents/object), [Charge](https://docs.stripe.com/api/charges/object), [Price](https://docs.stripe.com/api/prices/object), [refund filters/pagination](https://docs.stripe.com/api/refunds/list), [dispute filters/pagination](https://docs.stripe.com/api/disputes/list), [account](https://docs.stripe.com/api/accounts/retrieve). Это спецификации API, не доказательство реального подключения; фактические подтверждения относятся к управляемому HTTPS fixture.
