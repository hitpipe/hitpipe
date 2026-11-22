# Протокол раздельных процессов Processing и IAM

Codex по делегации Тимофея Патрушева. Linux arm64, PostgreSQL 18.7, выделенный Docker без внешней сети, разные базы Processing и IAM и отдельные прикладные роли. Go race/vet с хоста записаны отдельно.

Квалификация BLOCKED. PASS теста означает воспроизведение блокировки и безопасного поведения при отсутствии ACK.

Processing ce317193244623c5a464a3014bb39f32875c1e98. IAM ae5fd0f35ec7d5097cace75b3b15ecb14042c654. Ветка IAM включает принятые HIT-116 и HIT-115.

## Прогон 6

Фактический запуск и полный вывод.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT146_IAM_BIN=/test/iam-probe", "9c957143591334f14b7c05439afa0f68defb02cfc65194fa5e12834066e1304e", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestRealIAMRegistrationBoundary$"]
```

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation, UTC 2026-11-22T15:17:37.485312+00:00–2026-11-22T15:17:42.871559+00:00, exit 0.

stdout SHA-256 59e6d8b20c59e78e7e96f9d0b52a5f82cc1da58cc2150f91028fe7d2b9304bd5.

```text
=== RUN   TestRealIAMRegistrationBoundary
    real_iam_process_test.go:196: exact provisioning, actual IAM HTTP 200 before and after local IAM OPEN: {"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","generation":2,"boot_id":"c411a0c5-6970-470a-a0c0-1973b6367446","snapshot_revision":1795360657589172,"denials":[],"gate":"SYNCING"} / {"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","generation":2,"boot_id":"c411a0c5-6970-470a-a0c0-1973b6367446","snapshot_revision":1795360657633533,"denials":[],"gate":"SYNCING"}
    real_iam_process_test.go:206: actual generation=2 snapshot rejected by generation=3 startgate
    real_iam_process_test.go:217: old generation registration HTTP 403, wrong mTLS principal rejected at TLS
    real_iam_process_test.go:223: actual IAM authorize for valid TEST session and registered remote SYNCING runtime HTTP 403
    real_iam_process_test.go:235: first-start boot=1ef37a6a-5ca6-412d-a758-a70266a3a436, actual IAM registration HTTP 403, protected call HTTP 503, OPS HTTP 200 {"build_revision":"ce317193244623c5a464a3014bb39f32875c1e98","checked_at":"2026-11-22T15:17:37.683054511Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    real_iam_process_test.go:235: restart boot=e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d, actual IAM registration HTTP 403, protected call HTTP 503, OPS HTTP 200 {"build_revision":"ce317193244623c5a464a3014bb39f32875c1e98","checked_at":"2026-11-22T15:17:37.728921386Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    real_iam_process_test.go:256: control TLS leaf=sha256:be93e7a9e09c23a8b91764ed3d36cc45a5cf0a62461c8e433878ee882da69042; registered outbound leaf=sha256:ab6828e5300f264b57854a977e180463ffa17ad1afe0b36be16725c893765e3f
    real_iam_process_test.go:274: BLOCKED: no protected start, no create, no accepted remote OPEN and no EFFECTIVE revocation
    real_iam_process_test.go:178: actual IAM process trace:
        {"time":"2026-11-22T15:17:37.55126897Z","level":"INFO","msg":"iam-listening"}
        {"time":"2026-11-22T15:17:37.58934797Z","level":"INFO","msg":"iam-register","status":200,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"c411a0c5-6970-470a-a0c0-1973b6367446","generation":2}
        {"time":"2026-11-22T15:17:37.59652522Z","level":"INFO","msg":"iam-gate-open"}
        {"time":"2026-11-22T15:17:37.633704511Z","level":"INFO","msg":"iam-register","status":200,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"c411a0c5-6970-470a-a0c0-1973b6367446","generation":2}
        {"time":"2026-11-22T15:17:37.63552172Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"c411a0c5-6970-470a-a0c0-1973b6367446","generation":1}
        2026/11/22 15:17:37 http: TLS handshake error from 127.0.0.1:52674: privateiam: client certificate rejected
        {"time":"2026-11-22T15:17:37.638296011Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T15:17:37.65263897Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"1ef37a6a-5ca6-412d-a758-a70266a3a436","generation":2}
        {"time":"2026-11-22T15:17:37.699666803Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:37.767058261Z","level":"INFO","msg":"coordinator-send","runtime_id":"32d529c8-78ab-472e-9d12-37dc62bffbc7","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:37.76772622Z","level":"INFO","msg":"coordinator-send","runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:37.768212845Z","level":"INFO","msg":"coordinator-pending","status":"PENDING","accepted_at":"2026-11-22T15:17:37.763594Z","deadline_at":"2026-11-22T15:17:42.763594Z","effective":false,"missing":2}
        {"time":"2026-11-22T15:17:38.701374679Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:39.706443221Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:40.706847055Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:41.702871597Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:42.701457625Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","boot_id":"e56fbb28-f3a4-4b64-8f9e-816fdaf9e07d","generation":2}
        {"time":"2026-11-22T15:17:42.851931125Z","level":"INFO","msg":"coordinator-send","runtime_id":"32d529c8-78ab-472e-9d12-37dc62bffbc7","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:42.853827916Z","level":"INFO","msg":"coordinator-send","runtime_id":"0e388490-3c0b-47f8-9c0c-4bb25d3befe9","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:42.855101958Z","level":"INFO","msg":"coordinator-deadline","status":"INCIDENT","effective":false,"deadline_breached":true,"missing":2,"observed_at":"2026-11-22T15:17:42.855096Z","elapsed_ms":5091}
--- PASS: TestRealIAMRegistrationBoundary (5.33s)
PASS
```

Идентификаторы бинарников этого прогона.

```json
{
  "processing_sha": "ce317193244623c5a464a3014bb39f32875c1e98",
  "iam_sha": "ae5fd0f35ec7d5097cace75b3b15ecb14042c654",
  "platform": "linux/arm64",
  "fixture_main_sha256": "4ca31ed8937c8b0938a701413136085ff828d57748801b7b97f607979777e2d0",
  "binaries": {
    "processing": "c60c4eba5141d85d482dc49e29c657e876d1a72f25f581693ad431d1caf0fc56",
    "storage.test": "0082138826029056032318267ef5816b366a5874a2030411e0b214a6a0232511",
    "iam-probe": "9d1fb17e3176c42a327493da86a3cc77800b2d0e6e4359efbc1b8d57f408aaeb",
    "iam-migrate": "d517fe10be42469ded8e085be25a11f7701484c95714fb4f17ff75d01aaf69eb"
  }
}
```

## Прогон 7

Фактический запуск и полный вывод.

```json
["docker", "exec", "-e", "HIT78_ISOLATED_PG=1", "-e", "HIT111_PROCESSING_BIN=/test/processing", "-e", "HIT146_IAM_BIN=/test/iam-probe", "ca500e7d5ef31252d0a2094eb0a769a1b49f5f4f59dc66209e7d6323daacf524", "/test/storage.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^TestRealIAMRegistrationBoundary$"]
```

Каталог /Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation, UTC 2026-11-22T15:17:54.456255+00:00–2026-11-22T15:17:59.849052+00:00, exit 0.

stdout SHA-256 800a981f075d3eec8703d6a244f0ad0ca88ace496e043c1969b5728e78753c76.

```text
=== RUN   TestRealIAMRegistrationBoundary
    real_iam_process_test.go:196: exact provisioning, actual IAM HTTP 200 before and after local IAM OPEN: {"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","generation":2,"boot_id":"68e451fd-685e-4542-8110-f184a6bb2f61","snapshot_revision":1795360674562299,"denials":[],"gate":"SYNCING"} / {"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","generation":2,"boot_id":"68e451fd-685e-4542-8110-f184a6bb2f61","snapshot_revision":1795360674608581,"denials":[],"gate":"SYNCING"}
    real_iam_process_test.go:206: actual generation=2 snapshot rejected by generation=3 startgate
    real_iam_process_test.go:217: old generation registration HTTP 403, wrong mTLS principal rejected at TLS
    real_iam_process_test.go:223: actual IAM authorize for valid TEST session and registered remote SYNCING runtime HTTP 403
    real_iam_process_test.go:235: first-start boot=a70f588d-976b-4b89-a483-0790f39002df, actual IAM registration HTTP 403, protected call HTTP 503, OPS HTTP 200 {"build_revision":"ce317193244623c5a464a3014bb39f32875c1e98","checked_at":"2026-11-22T15:17:54.655953047Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    real_iam_process_test.go:235: restart boot=722a02b8-b57e-4017-9bbf-223da279ba21, actual IAM registration HTTP 403, protected call HTTP 503, OPS HTTP 200 {"build_revision":"ce317193244623c5a464a3014bb39f32875c1e98","checked_at":"2026-11-22T15:17:54.702280089Z","protocol_version":"1.0","service":"PROCESSING","status":"CLOSED"}
    real_iam_process_test.go:256: control TLS leaf=sha256:8f2afe7a237325ef878292636a139ba8fb502a208c3d069257dfed169ec1ae3f; registered outbound leaf=sha256:7c4940716b4f683f586a4690fb26fa8d7aa8c932daedb87834fe86d069f6f3b5
    real_iam_process_test.go:274: BLOCKED: no protected start, no create, no accepted remote OPEN and no EFFECTIVE revocation
    real_iam_process_test.go:178: actual IAM process trace:
        {"time":"2026-11-22T15:17:54.523219297Z","level":"INFO","msg":"iam-listening"}
        {"time":"2026-11-22T15:17:54.56247238Z","level":"INFO","msg":"iam-register","status":200,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"68e451fd-685e-4542-8110-f184a6bb2f61","generation":2}
        {"time":"2026-11-22T15:17:54.568145172Z","level":"INFO","msg":"iam-gate-open"}
        {"time":"2026-11-22T15:17:54.608693714Z","level":"INFO","msg":"iam-register","status":200,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"68e451fd-685e-4542-8110-f184a6bb2f61","generation":2}
        {"time":"2026-11-22T15:17:54.609790214Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"68e451fd-685e-4542-8110-f184a6bb2f61","generation":1}
        2026/11/22 15:17:54 http: TLS handshake error from 127.0.0.1:51986: privateiam: client certificate rejected
        {"time":"2026-11-22T15:17:54.61268313Z","level":"INFO","msg":"iam-http","status":403,"path":"/internal/v1/authorize"}
        {"time":"2026-11-22T15:17:54.626793464Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"a70f588d-976b-4b89-a483-0790f39002df","generation":2}
        {"time":"2026-11-22T15:17:54.671476589Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:54.73755138Z","level":"INFO","msg":"coordinator-send","runtime_id":"cbf686e5-08d9-4d4e-b78f-36064edd03ce","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:54.738114672Z","level":"INFO","msg":"coordinator-send","runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:54.73863638Z","level":"INFO","msg":"coordinator-pending","status":"PENDING","accepted_at":"2026-11-22T15:17:54.733499Z","deadline_at":"2026-11-22T15:17:59.733499Z","effective":false,"missing":2}
        {"time":"2026-11-22T15:17:55.673294964Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:56.679365131Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:57.675826715Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:58.675517007Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:59.679420049Z","level":"INFO","msg":"iam-register","status":403,"runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","boot_id":"722a02b8-b57e-4017-9bbf-223da279ba21","generation":2}
        {"time":"2026-11-22T15:17:59.823007549Z","level":"INFO","msg":"coordinator-send","runtime_id":"cbf686e5-08d9-4d4e-b78f-36064edd03ce","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:59.824692174Z","level":"INFO","msg":"coordinator-send","runtime_id":"ec23f163-f4aa-456c-a092-039448ce085f","generation":2,"ack_received":false}
        {"time":"2026-11-22T15:17:59.825909424Z","level":"INFO","msg":"coordinator-deadline","status":"INCIDENT","effective":false,"deadline_breached":true,"missing":2,"observed_at":"2026-11-22T15:17:59.825903049Z","elapsed_ms":5092}
--- PASS: TestRealIAMRegistrationBoundary (5.33s)
PASS
```

Идентификаторы бинарников этого прогона.

```json
{
  "processing_sha": "ce317193244623c5a464a3014bb39f32875c1e98",
  "iam_sha": "ae5fd0f35ec7d5097cace75b3b15ecb14042c654",
  "platform": "linux/arm64",
  "fixture_main_sha256": "4ca31ed8937c8b0938a701413136085ff828d57748801b7b97f607979777e2d0",
  "binaries": {
    "processing": "c60c4eba5141d85d482dc49e29c657e876d1a72f25f581693ad431d1caf0fc56",
    "storage.test": "0082138826029056032318267ef5816b366a5874a2030411e0b214a6a0232511",
    "iam-probe": "9d1fb17e3176c42a327493da86a3cc77800b2d0e6e4359efbc1b8d57f408aaeb",
    "iam-migrate": "d517fe10be42469ded8e085be25a11f7701484c95714fb4f17ff75d01aaf69eb"
  }
}
```

Полные команды, выбранные исходные потоки и журнал каждой попытки упакованы в [raw-execution.tar.gz](raw-execution.tar.gz).

SHA-256 архива baeaf73c8c0f40044c40260cdcd1c2b51983fd8b0f7a4490fabd6404713ffbec. Подготовительные логи Docker/PostgreSQL за рамками опубликованного архива.
