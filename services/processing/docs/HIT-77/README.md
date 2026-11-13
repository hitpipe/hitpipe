# HIT-77 — чтение фактов из Intake

Codex по поручению Тимофея Патрушева / Tim1hik.

Main при старте: ce771d37505e7770003d0796ae932556d06b7bab.
PR44 проверен повторно: MERGED, head047642704cb6fbe8c3e990ced41110ebb3b6db10,
merge20adc01a037148d20ea8da6c281f52d25cb2d9e3. HIT-72 — Done. Исходные
Decode, schema-generated metadata, тесты и synthetic fixture переиспользованы без
изменений; финансовая функция EvaluatePaidOrder и eligibility.Evaluate сохранены.
Работа HIT-76 и оригинальная рабочая папка не менялись.

Проверенная реализация: 204fabc518dbb403a84ef945da215ed26a0b1e89.

Добавлен Reader для POST /internal/v1/source-facts/read. Wire-request сгенерирован
из принятой OAS, обязательный X-Request-ID и шесть полей проверены реальным
HTTPS-handler. Local Policy отделена от установленных caller workspace/epoch/
deadline и не отправляется Intake. Успешный ответ проходит прежний Decode.
Для совместимого сохранения provenance в локальном порте добавлен необязательный
Snapshot.Proof; это не wire-контракт и не изменение финансовых правил.

Обязательны явно переданные TLS1.3/mTLS transport и StartBoundary. Продуктового
gate/coordinator или разрешающего fallback здесь нет. Проверки зависимости
закрывают отсутствие контекста/транспорта/gate. Boundary охватывает connect и
каждую реальную запись socket, а не только предварительный check. Стенд проверяет
отказ до connect и после TLS verification, до HTTP; учитываются epoch/workspace.
Redirect/replay/keepalive/HTTP2 не создают скрытого повторного POST.

Фактически проверены ошибки TLS/имени/CA/client principal, 401/403/429/503,
malformed/mismatch/field-size/buffer-size, dropped connection, timeout, cancellation,
завершение reader/server в cancellation-тесте, отсутствие старого результата и
независимое владение параллельными ответами. В существующей композиции подтверждены
VALID, INELIGIBLE/REFUND, INCOMPLETE и REFRESH_REQUIRED. Ошибки не дают разрешения
действия. Метаданные source proof сохраняют фактические строки времён и digest;
проверка digest или подлинности финансовых данных этим не заявляется.

Среда: Go1.27.2 darwin/arm64, GOTOOLCHAIN=local. Реальные команды, cwd/env/UTC,
exit codes и SHA256/stdout/stderr находятся в verification/commands.json.
go test -race -count=1 -json ./... — 6пакетов PASS, 332 события PASS тестов/подтестов,
0fail. Это не 332 независимых финансовых сценария. go vet ./..., генерация и diff
сгенерированного request — exit0. Source audit фиксирует точные изменённые пути,
reuse и неизменность финансовой функции. Полная инструкция: ../../internal/adapters/
intake/sourcefacts/HTTP_READER_RU.md.

Лимиты не выдаются за новые требования продукта: OAS задаёт пределы полей, но не
общий байтовый максимум SourceFacts. Частная граница буфера проверенного транспорта
обязательна и не имеет default. Стенд задаёт только тестовое значение; оно не
объявляется принятым production/OAS-порогом. Предел production-транспорта, его
origin/credentials и общий runtime gate должна предоставить отдельная настоящая
композиция. Connect/write используют существующую границу2s A012, весь read —
явный caller deadline, без нового продуктового SLA.

Граница результата: управляемый loopback HTTPS/mTLS стенд с ephemeral test CA/keys
в памяти, тестовым gate и временным портом. Это не работающий Intake listener8443,
не IAM authority/epoch coordinator, не Stripe и не сквозная интеграция. Никаких
рабочих данных, service tokens, case/outbox writes, activation или внешних действий.
Проверка synthetic финансовых фактов использует их фиксированное время; deadline
сетевого теста идёт по реальному времени. VALID — финансовое знание, не разрешение.
Независимое review и человеческая приёмка ожидаются.
