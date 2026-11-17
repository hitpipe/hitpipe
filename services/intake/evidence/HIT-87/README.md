# HIT-87 — измерение и оптимизация проверки webhook

Реализовано одно улучшение: HMAC получает timestamp, точку и исходные байты последовательными Write вместо выделения и копирования body-sized сообщения. Для успешного completed-входа256KiB в повторном сравнении выделяется1 178 343,56 вместо1 448 686,44B/событие (**−18,66%**); throughput978,67→1041,67событий/с (+6,44%), p50 1025,625→1008,667мкс, p95 1133,459→1099,958мкс. Это локальные медианы пяти раундов, не эксплуатационный SLO. Число выделений слегка выросло (444,42→445,06), что не скрыто.

## Что переиспользовано

Main `ce771d37505e7770003d0796ae932556d06b7bab` не содержит готовый HIT-73. Jira HIT-73 прочитана как Done; PR65 остаётся открытым и не выдан за merged. Точный исходный SHA `551399a8efeeb266f6f417a097c5cf2b6db08864` сохранён; его AuthenticateWebhook, Normalize и Verify побайтно неизменны в baseline `8acc83de7daa316719004212fe0d76092aecb8f2`. До baseline-замеров добавлены только общий измеритель и его проверки, baseline после старта не переписан. Новый production-код отличается только Verify; нормализатор не дублирован. Новый `internal/measurement/webhook` — общий локальный ограниченный reader+синтетический набор для обеих версий, не HTTP-маршрут.

Улучшенный код: `7f688aa26b4e96c19e39f906a5ffaa51a7ce3a12`. Инструмент воспроизведения/CI: `f7fbe681056e61d9d650633cdf7afe1342d04360`. В artifact-коммите добавляются только доказательства и представление результата; окончательный SHA фиксируется неизменяемым URL регистрации. Пути и SHA256 — [source-audit.json](source-audit.json).

## Профиль → гипотеза → изменение

До изменения, отдельный настоящий профиль `initial/baseline.heap.pprof` показал409,61MiB flat /16,33% alloc_space в stripesignature.Verify; тело конкатенировалось только ради одного digest.Write. Гипотеза: потоковая подача точно тех же байтов устраняет body-sized allocation без изменения подписи/парсера. Выбрано это ограниченное улучшение, а не удаление строгих проверок или общей snapshot-копии Normalize. Последняя сохраняет владение данными на границе auth/parse. Все HMAC/buffers остаются локальными; pools, кэш успеха/event ID, общий изменяемый буфер и дополнительный параллелизм не введены.

CPU/alloc профили обеих версий, их текстовые tops, команды и исходные измерения сохранены. MemProfileRate=1 и overhead явно ограничивают интерпретацию CPU; абсолютные totals профилей с разным числом операций не выдаются за снижение на событие. Эффект установлен отдельными non-profile allocation-циклами, где одинаковый исход измеряется до/после. Весь набор и разброс — [measurement-summary.md](measurement-summary.md), полные цифры — [comparison.json](reproduced/comparison.json).

## Как измерено

Один Apple M1,8GiB RAM,8 logical CPU, macOS26.5 arm64, Go1.27.2, GOMAXPROCS=1, зависимости HIT-73 без изменения pins. Нет выделенной машины, фиксации ядра/частоты/температуры; фоновые процессы возможны. Все существенные параметры в initial/host.json, reproduced/host.json и go-env.stdout. Увеличение размера сделано JSON whitespace: это воспроизводимый стресс чтения/HMAC/границы, не модель распределения реальных Stripe webhook.

35сценариев:17успешных и18отказов; completed, async_payment_succeeded, async_payment_failed ×6размеров (512…262145B), повтор, подпись/повреждение/конфликт timestamp/unsupported scheme, tolerance300/301sec, malformed/duplicate/unknown/null/missing и неизвестный контекст. Подпись/генерация/consumer/corpus check исключены из таймера. Один и тот же путь: ограниченное чтение≤256KiB+1 → snapshot HIT-73 → HMAC → строгий разбор/JCS → Signal. Результат не сохраняется на диск/в SQL.

Каждый раунд имеет раздельные циклы по200операций для latency, throughput и TotalAlloc/Mallocs;10warmup/сценарий. Задержки содержат time.Now/Since overhead, p50/p95 — nearest upper rank. Пять раундов с чередованием B/O;3первичных baseline-прогона до оптимизации и первые5пар сохранены в initial/. Полная проверка инструмента повторила5пар в reproduced/. Набор/normalized outcomes/errors совпали побайтно (SHA `4e9abe37832c2e4abb9ea8f6e37ef257019b8aff0c452355e481f159d9b5826a`). Не смешиваются успех, ранний отказ и oversize. Числа для каждого исхода, ranges и все raw samples сохранены.

## Корректность

Обе версии действительно собраны и запущены. `go test -race -count=1 -json ./...`, `go vet ./...`, build и mod verify успешны на обеих; baseline120 и improved122 PASS test/subtest events,0failed/0skipped tests. Четыре packages без тестов отражены отдельно. Существующие отрицательные HIT-73 тесты required/null/absent/версий/подписи и точный RFC8785 сохранены. Новые tests сравнивают HMAC с независимой конкатенирующей подписью на бинарных данных и разных размерах, мутацию байтов, каждый повтор, конкурентные разные ключи/тела и результаты35сценариев. Ограниченный reader проверяет overflow и читает не более262145B, mapping ошибки чтения не раскрывает dependency text, проверяет истёкший context и truncated input.

Неизменны constant-time hmac.Equal,300сек, точный raw body и auth-before-parse. Каждый повтор проходит Resolve/Verify, parser не становится кэшем. Caller не должен мутировать вход одновременно с исходным snapshot-copy, как в HIT-73. Reader проверяет context между Read; блокирующий транспорт сам обязан обеспечивать deadline — реальный transport/HTTP этой задачей не квалифицирован. Синтетический корпус намеренно живёт в памяти измерителя, не является хранением production payload; вывод включает только projection, bounded error и digest, а не raw/secret.

Компактный фактический [протокол](test-transcript.txt) содержит команды/exit codes, ссылается на настоящие логи и [SHA256 index](artifacts.json). Не выдаётся исходник теста за его запуск.

## Как повторить

Из checkout этой ветки/PR с доступными закреплёнными commits и Go1.27.2:

```sh
python3 services/intake/tools/compare_webhook.py --go /absolute/path/to/go1.27.2/bin/go --output /tmp/hit87-new-comparison
```

Выходная папка должна отсутствовать. Инструмент извлекает exact baseline/candidate через git archive во временные каталоги, собирает, проверяет race/vet/modules, чередует5пар, снимает профили, сохраняет команды/exit codes/samples и comparison.json. Бинарники локального воспроизведения не коммитятся; pprof читается `go tool pprof -top /tmp/hit87-new-comparison/baseline /tmp/hit87-new-comparison/baseline.cpu.pprof` (для памяти добавить `-alloc_space` и `.heap.pprof`). Stripe/секреты/БД/сеть исполнения не нужны; Go module download возможен при пустом cache для точных pins. Старые учебные tasks/проверки не запускаются.

## Предел результата и следующий шаг

Подтверждено локальное снижение выделенных байтов при тех же результатах/отказах. Для малых входов latency шумит; меньше allocations не заявлено. Это не надёжный durable receipt HIT-86, HTTP ACK, sender, Processing, IAM/runtime, production disk, нагрузка deployment или Stripe→ClickUp. Эксплуатационные SLO и security qualification cohort этим исследованием не подтверждены. Независимое ревью и человеческое принятие ещё не выполнены.

Следующий шаг после серверной регистрации/READY: ответственному участнику лично передать HIT-87 в In Review для независимой проверки этого кода, замеров и границ.
