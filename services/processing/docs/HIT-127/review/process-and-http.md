# HIT-127. Полный текст процессных доказательств

Код `4da48202e77131427730b1827df9ee81940e559a`, база `d450efc7cfc11f8155a476304aced96dad0ebaf6`, исполнение 2026-11-20T18:58:42.136279+00:00. PostgreSQL, версия и образ в протоколе команд. Все строки ниже — фактические потоки именованной команды `diagnostic-postgres-tests`, её CLI и двух дочерних процессов. Отдельные поля HTTP capture содержат ответы сервера. Запросы, заголовки авторизации и тела запросов в записи отсутствуют. Точные определения сценариев и проверок задержек, границ полей и утечек находятся в зарегистрированном `exact-source.diff`.

Файлы переданы полностью. SHA-256 логов `04ea7cecb9a4268a8cfd17364f1e95e51cc7b47129508fa1ff0fb2d5b4c5c42e`, SHA-256 HTTP `569a4aa452ad73d604f8a08cd1a37a05f5bfdb49fd5f734dc6d8543b34f10a48`. Повтор снабжён одним наблюдением; состояние READY подтверждено при удержанных сообщениях и ATTENTION.

## diagnostic-runtime.jsonl (весь поток)

~~~jsonl
{"time":"2026-11-20T18:58:25.020042425Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
{"time":"2026-11-20T18:58:25.023214717Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"CLOSED"}
{"time":"2026-11-20T18:58:26.024700592Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
{"time":"2026-11-20T18:58:26.255014592Z","level":"WARN","msg":"processing diagnostic fault","reason":"dispatch_held","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:27.025336676Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"OPEN"}
{"time":"2026-11-20T18:58:27.032524676Z","level":"WARN","msg":"processing diagnostic fault","reason":"lease_expired","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:27.032570092Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":2,"cap_reached":false,"held":0,"max_age_ms":2046},"outbox":{"rows":1,"cap_reached":false,"pending":1,"claimed":0,"held":0,"acked":0,"expired_leases":0,"max_age_ms":2042,"max_due_lag_ms":2042},"jobs":{"rows":1,"cap_reached":false,"due":1,"expired_leases":1,"max_due_lag_ms":2042},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":0,"unknown":1},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238","expired_job_id":"e648472a-eaad-4d7b-bcbb-da2bf9755453"}
{"time":"2026-11-20T18:58:27.264249468Z","level":"INFO","msg":"processing recheck outbox transition","request_id":"c96e6f5e-cafb-4305-975b-1cfd68b27e34","state":"HELD","disposition":"HELD"}
{"time":"2026-11-20T18:58:28.448689385Z","level":"WARN","msg":"processing diagnostic fault","reason":"dispatch_held","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:29.03736576Z","level":"WARN","msg":"processing diagnostic fault","reason":"lease_expired","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:29.037420968Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":2,"cap_reached":false,"held":0,"max_age_ms":4051},"outbox":{"rows":2,"cap_reached":false,"pending":1,"claimed":0,"held":1,"acked":0,"expired_leases":0,"max_age_ms":4047,"max_due_lag_ms":4047},"jobs":{"rows":1,"cap_reached":false,"due":1,"expired_leases":1,"max_due_lag_ms":4047},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":0,"unknown":1},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238","expired_job_id":"e648472a-eaad-4d7b-bcbb-da2bf9755453"}
{"time":"2026-11-20T18:58:31.042792136Z","level":"WARN","msg":"processing diagnostic fault","reason":"lease_expired","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:31.042824094Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":2,"cap_reached":false,"held":0,"max_age_ms":6057},"outbox":{"rows":2,"cap_reached":false,"pending":0,"claimed":0,"held":2,"acked":0,"expired_leases":0,"max_age_ms":6053,"max_due_lag_ms":null},"jobs":{"rows":1,"cap_reached":false,"due":1,"expired_leases":1,"max_due_lag_ms":6053},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":0,"unknown":1},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238","expired_job_id":"e648472a-eaad-4d7b-bcbb-da2bf9755453"}
{"time":"2026-11-20T18:58:33.046145804Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":2,"cap_reached":false,"held":0,"max_age_ms":8060},"outbox":{"rows":2,"cap_reached":false,"pending":0,"claimed":0,"held":2,"acked":0,"expired_leases":0,"max_age_ms":8057,"max_due_lag_ms":null},"jobs":{"rows":0,"cap_reached":false,"due":0,"expired_leases":0,"max_due_lag_ms":null},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":0,"unknown":1},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:33.074678429Z","level":"INFO","msg":"processing observation committed","observation_age_ms":708,"disposition":"APPLIED","case_state":"ATTENTION","skipped_previous_window":0,"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238","case_id":"46ac1dfa-0c51-40b1-a211-bb1c1369561c","message_id":"3b3c20ca-142e-4bd0-9c89-24d31c0f619d","effect_id":"e648472a-eaad-4d7b-bcbb-da2bf9755453"}
{"time":"2026-11-20T18:58:35.05313043Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":3,"cap_reached":false,"held":0,"max_age_ms":10067},"outbox":{"rows":2,"cap_reached":false,"pending":0,"claimed":0,"held":2,"acked":0,"expired_leases":0,"max_age_ms":10063,"max_due_lag_ms":null},"jobs":{"rows":0,"cap_reached":false,"due":0,"expired_leases":0,"max_due_lag_ms":null},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":1,"unknown":0},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:35.081075805Z","level":"WARN","msg":"processing diagnostic fault","reason":"content_unavailable"}
{"time":"2026-11-20T18:58:35.098533388Z","level":"INFO","msg":"processing listeners started","gate":"CLOSED"}
{"time":"2026-11-20T18:58:35.098580138Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"CLOSED"}
{"time":"2026-11-20T18:58:35.100955721Z","level":"INFO","msg":"processing runtime registered","gate":"OPEN"}
{"time":"2026-11-20T18:58:35.31440818Z","level":"WARN","msg":"processing diagnostic fault","reason":"job_recovery_failed","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:35.319839513Z","level":"WARN","msg":"processing diagnostic fault","reason":"recheck_interrupted","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:35.324886846Z","level":"WARN","msg":"processing diagnostic fault","reason":"timer_interrupted","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:35.327816763Z","level":"WARN","msg":"processing diagnostic fault","reason":"outbox_interrupted","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:37.09874Z","level":"INFO","msg":"processing barrier diagnostic","barrier":"OPEN"}
{"time":"2026-11-20T18:58:37.105119916Z","level":"WARN","msg":"processing diagnostic fault","reason":"postgres_unavailable","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:39.113059292Z","level":"INFO","msg":"processing queue sample","sample_limit":100,"interval_ms":2000,"inbox":{"rows":3,"cap_reached":false,"held":0,"max_age_ms":14127},"outbox":{"rows":2,"cap_reached":false,"pending":0,"claimed":0,"held":2,"acked":0,"expired_leases":0,"max_age_ms":14123,"max_due_lag_ms":null},"jobs":{"rows":0,"cap_reached":false,"due":0,"expired_leases":0,"max_due_lag_ms":null},"cases":{"rows":1,"cap_reached":false,"waiting_data":0,"attention":1,"unknown":0},"workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:39.122792751Z","level":"WARN","msg":"processing diagnostic fault","reason":"timer_interrupted","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:39.126573959Z","level":"WARN","msg":"processing diagnostic fault","reason":"outbox_interrupted","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
{"time":"2026-11-20T18:58:39.130914251Z","level":"INFO","msg":"processing stopped"}
{"time":"2026-11-20T18:58:35.071436805Z","level":"WARN","msg":"private Processing content directory unavailable or busy","reason":"purge_volume_unavailable","workspace_id":"75eaa7df-dbab-4b58-91aa-bdc353904238"}
~~~

## diagnostic-http.jsonl (все полученные ответы)

~~~jsonl
{"scenario":"closed_before_fresh_iam_snapshot","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:25.05235655Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"CLOSED\"}\n"}
{"scenario":"ready_after_gate_open","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:26.032365009Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"READY\"}\n"}
{"scenario":"ready_with_held_outbox_and_attention_case","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:35.058166971Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"READY\"}\n"}
{"scenario":"non_ops_peer_denied","method":"GET","path":"/internal/v1/health/processing","peer":"DELIVERY","http_status":403,"response_body":"{\"code\":\"HEALTH_ACCESS_DENIED\",\"message\":\"Processing request could not be completed\",\"request_id\":\"06a07ec7-5eed-4603-86c3-10b412f52c87\"}\n"}
{"scenario":"owned_file_unavailable","method":"GET","path":"/api/v1/workspaces/{workspaceId}/cases/{caseId}/history","peer":"BFF","http_status":503,"response_body":"{\"code\":\"READ_UNAVAILABLE\",\"message\":\"Request could not be completed\",\"request_id\":\"4e822523-94ef-4f77-9cc6-bdb1b971d0a3\"}\n"}
{"scenario":"ready_after_process_restart","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:35.127201305Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"READY\"}\n"}
{"scenario":"degraded_on_worker_postgres_connection_failure","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:39.126305917Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"DEGRADED\"}\n"}
{"scenario":"ready_after_worker_postgres_reconnect","method":"GET","path":"/internal/v1/health/processing","peer":"OPS","http_status":200,"response_body":"{\"build_revision\":\"4da48202e77131427730b1827df9ee81940e559a\",\"checked_at\":\"2026-11-20T18:58:39.130374334Z\",\"protocol_version\":\"1.0\",\"service\":\"PROCESSING\",\"status\":\"READY\"}\n"}
~~~

## diagnostic-postgres-tests / stdout

~~~text
=== RUN   TestDiagnosticSampleBoundedAndScoped
--- PASS: TestDiagnosticSampleBoundedAndScoped (0.02s)
=== RUN   TestProcessingDiagnosticsOwnedStand
    diagnostics_test.go:389: HTTP PASS: captured actual Health CLOSED/READY/DEGRADED/recovered and mTLS denial; missing each required field and an extra property were rejected by the pinned validator
    diagnostics_test.go:390: Observed live inbox age 2046 ms, outbox due lag 2042 ms, job due lag 2042 ms and committed observation age 708 ms.
    diagnostics_test.go:392: PROCESS PASS: CLOSED/OPEN and exact Health, actual HELD/UNKNOWN, changed outbox/jobs, observation replay, expired lease, content and PostgreSQL faults, purge reason, restart and log privacy
--- PASS: TestProcessingDiagnosticsOwnedStand (14.15s)
=== RUN   TestProcessingPurgeOwnedStand
    purge_test.go:236: expired pending/claimed messages held before unlink; process exit after commit, restart and idempotent bounded sweep; live SQL ref and foreign file preserved; safety identity and command keys retained
--- PASS: TestProcessingPurgeOwnedStand (0.15s)
PASS
~~~

## diagnostic-postgres-tests / stderr

Пустой поток.

## diagnostic-race-tests / stdout

~~~text
=== RUN   TestDiagnosticsBoundsAndOpaqueIDs
--- PASS: TestDiagnosticsBoundsAndOpaqueIDs (0.00s)
PASS
ok  	github.com/hitpipe/hitpipe/services/processing/internal/runtime	1.897s
~~~

## diagnostic-race-tests / stderr

Пустой поток.
