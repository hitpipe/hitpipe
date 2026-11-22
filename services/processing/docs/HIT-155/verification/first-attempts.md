# Ход проверки HIT-155

Первая интеграционная попытка закончилась на компиляции тестового драйвера. Имя вспомогательной функции пересеклось с переменной регистрации. Исправлено в тесте.

SHA `c6bd744cd864214862b5299612593b8c8c66ab55`, exit 1, диагностика.

```text
# github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres [github.com/hitpipe/hitpipe/services/processing/internal/adapters/postgres.test]
internal/adapters/postgres/recovery_process_test.go:274:7: no new variables on left side of :=
internal/adapters/postgres/recovery_process_test.go:274:10: cannot use stand.current (variable of struct type drafts.RuntimeRegistration) as func(log string) value in assignment
internal/adapters/postgres/recovery_process_test.go:277:81: boot.BootID undefined (type func(log string) has no field or method BootID)
internal/adapters/postgres/recovery_process_test.go:277:106: boot.Generation undefined (type func(log string) has no field or method Generation)
```

На `f6c1ddd8ff5117ca51f9e9dde787a774bfd8b35e` сборка и Health прошли. Проверка истории при первом перезапуске завершилась ошибкой. Следующий запуск добавил точную диагностику.

SHA `b6bfa5c9715509e806f9f6774a116d0d19f9f6a6`, тест завершился FAIL. Квитанция уже была сравнена и совпала. Снимок до срабатывания case worker содержал только SIGNAL_ADMITTED, снимок после — ту же запись и ожидаемый WAITING_DATA/READER_UNAVAILABLE от управляемого Intake. Оба снимка в исходном выводе.

```text
=== RUN   TestProcessingOCIImageOwnedStand
    image_process_test.go:116: OPS mTLS HTTP 200 {"build_revision":"b6bfa5c9715509e806f9f6774a116d0d19f9f6a6","checked_at":"2026-11-22T13:37:25.899940759Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:125: Negative HTTP checks: CLOSED business 503, non-OPS Health 403, missing mTLS certificate refused
    image_process_test.go:128: OPS mTLS HTTP 200 {"build_revision":"b6bfa5c9715509e806f9f6774a116d0d19f9f6a6","checked_at":"2026-11-22T13:37:26.878922551Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:167: OPS mTLS HTTP 200 {"build_revision":"b6bfa5c9715509e806f9f6774a116d0d19f9f6a6","checked_at":"2026-11-22T13:37:27.493068343Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    image_process_test.go:170: OPS mTLS HTTP 200 {"build_revision":"b6bfa5c9715509e806f9f6774a116d0d19f9f6a6","checked_at":"2026-11-22T13:37:28.40449026Z","protocol_version":"1.0","service":"PROCESSING","status":"READY"}
    image_process_test.go:191: synthetic history changed across restart: before=[{"entry_id":"9393a544-4a98-46c4-9027-20f43e25d4d0","case_id":"503b55cd-2be6-4067-bade-c491dd13e010","at":"2026-11-22T13:37:26.954656Z","actor_kind":"SYSTEM","actor_id":null,"old_state":null,"new_state":"RECEIVED","reason_code":"SIGNAL_ADMITTED","evidence_ref":"03476ded-4903-47a7-b18b-1e90e09f1dc1","episode_id":null,"expires_at":"2027-02-20T13:37:26.954656Z"}] after=[{"entry_id":"9393a544-4a98-46c4-9027-20f43e25d4d0","case_id":"503b55cd-2be6-4067-bade-c491dd13e010","at":"2026-11-22T13:37:26.954656Z","actor_kind":"SYSTEM","actor_id":null,"old_state":null,"new_state":"RECEIVED","reason_code":"SIGNAL_ADMITTED","evidence_ref":"03476ded-4903-47a7-b18b-1e90e09f1dc1","episode_id":null,"expires_at":"2027-02-20T13:37:26.954656Z"},{"entry_id":"0922ac5d-87f9-4c37-be99-cc69cd4b39d4","case_id":"503b55cd-2be6-4067-bade-c491dd13e010","at":"2026-11-22T13:37:27.097753Z","actor_kind":"SYSTEM","actor_id":null,"old_state":"RECEIVED","new_state":"WAITING_DATA","reason_code":"READER_UNAVAILABLE","evidence_ref":null,"episode_id":null,"expires_at":"2027-02-20T13:37:27.097753Z"}]
--- FAIL: TestProcessingOCIImageOwnedStand (2.76s)
FAIL
```

На проверенной версии исходный снимок берётся после ожидаемого удержания. Сравнение полной истории и квитанции сохранено.
