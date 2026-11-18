# HIT-104 — маршрут и карта полей

Точный источник: принятый OpenAPI `fedbe40bb597e25f66aeb1172af3ff7459c81b40`, SHA256 `a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e`; операция receiveNormalizedSignal, owner PROCESSING, caller INTAKE, private8443, mTLS. SQL/OAS не менялись.

`existing casereadhttp.NewServer → intakehttp.Attach → verified Intake TLS peer → HTTP method/path/header/256KiB bound → receivesignals.Service → admission.AdmitWithContext (same Admit path) → existing normalizedsignal.Decoder once → ContextSource current verified installation/authority + own operation revision → EstablishedContext.Check → shared actual-start SQL/content boundary → existing postgres.Store.Accept → durable commit → separate response-start gate → Receipt202`.

Attach не запускает сервер. Вызов происходит до ServeTLS на одном закрытом listener; прежний Handler остаётся для всех других путей. В реальных тестах Intake не получает BFF/Delivery доступ; BFF/Delivery не получают Intake доступ. Legacy entrypoints и финансовые модули сохранены.

| Вход/источник | Проверка и использование |
|---|---|
| TLS certificate | CA, SAN, срок, TLS1.3, отдельный Intake peer; сертификат не задаёт workspace/операцию/человеческое право. |
| X-Request-ID | Один обязательный UUID; только correlation ошибок, не authority/message identity. |
| workspace_id/connection_id/stripe_account_id/livemode | Raw hints проходят schema/digest; ContextSource устанавливает совпадение с доверенной установкой. UUID workspace/connection нормализуются после digest для совпадения SQL/content origin. Account/mode не нормализуются. |
| operation_id/ExpectedRevision/PrincipalID/Epoch/grant expiry | Не wire-поля сигнала. Обязательный trusted ContextSource, проверенная собственная операция, действительные полномочия и shared boundary. Тестовая установка явно задана, revision реально читается из PostgreSQL под тем же gate. Неизвестное закрыто. |
| message_id/message_digest/schema_version | Прежний точный decoder и RFC8785. Durable inbox `(workspace,message_id)` сохраняет digest/результат; другой digest409. Корректное original message_id отражается в Receipt. |
| session_id/stripe_account_id/livemode/workspace | Прежняя SQL unique identity; другая Session/account/mode/workspace не склеивается. Новый event/message для той же identity не перепривязывает case/config/owner и не разрешает новый effect. |
| source_created_at | Только Checkout Session.created, `Event.data.object.created`, решение [12171](https://hitpipe.atlassian.net/browse/HIT-78?focusedCommentId=12171). Сравнивается с неизменной first_activation_at; equality допустима. Event.created отсутствует и не подставляется. |
| receipt_id/first_received_at/expires_at | Исходная принадлежность Intake и времена сохраняются в минимальном existing HISTORY detail, receipt_id также proof_ref. Replay не обновляет detail/ref/retention и не превращает source expiry в новый финансовый порог. |
| event_id/event_type | Строгий enum и прежний wire-контракт. В новое SQL/HISTORY-содержимое не копируются raw NormalizedSignal/Stripe event или произвольный текст. |
| Receipt | Ровно разрешённые message_id/disposition/record_id. Revision не выдумывается: исходный inbox его не хранит.202 после commit; original disposition восстанавливается без второго допуска. |

## Отказы и порядок

400 — неверный HTTP/JSON/digest/формат.403 — запрещённый caller/привязка/grant/gate.404 — нет доступной установленной операции.409 — digest conflict или операция/исходная Session.created не допускают новое сообщение.412 — изменилась revision.413 — >256KiB.503 — не установлено надёжное принятие, SQL/content failure или неизвестная обязательная зависимость. Все JSON-ошибки имеют только Error{code,message,request_id}; внутренние exception/SQL/текст сообщения не раскрываются. Неверный cert может отклониться ещё на TLS handshake.

Store сначала читает уже committed inbox под текущей авторизацией и возвращает прежний Receipt даже после pause/revision; это не новая работа. Для нового message Store блокирует собственную операцию, сверяет revision/ACTIVE/принятую config/первую активацию, затем использует старую запись identity и HISTORY. Дубликат/CONFLICT не продлевает срок. Даже если commit состоялся, потерянный DB ACK, разрыв ответа или crash до ответа не заменяются выдуманным успехом: следующая действительная доставка восстанавливает результат из SQL.

Собственный HISTORY file: прежний fsync→rename→directory fsync до SQL commit. При неуспехе SQL не обещается удаление каждого потенциального orphan: прежний owned purge/recovery protocol сохраняется. После expiry/purge остаются SQL minimal identity/safety и исходная inbox квитанция; чужое содержимое не читается.

## Сборка контекста и предел утверждения

`receivesignals.Service` требует Contexts, Boundary, Clock, Decoder и Store. Ни один порт не имеет разрешающего default. Resolve ограничен2сек и не может превратить произвольные JSON/browser поля в verified context. Реализация ContextSource обязана сама охранять защищённые lookup тем же actual-start gate. В стенде это explicitly controlled installation/grant + настоящий read-only own-operation lookup; проверены denied/expired/revocation/epoch и невызывающий gate. Production IAM/bindings resolver этой задачей не заявляется подключённым.

После Admit нет синхронного вызова financial kernel/SourceFacts/provider/Delivery. Дальнейшее продвижение остаётся отдельным фактическим runtime, HTTP202 не маскирует его отсутствие. Источники и версии предшественников: source-audit.json; протоколы и full logs: README.md.
