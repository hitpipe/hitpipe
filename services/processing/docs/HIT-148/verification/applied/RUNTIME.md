# HIT-148. Полный вывод запуска с настоящим издателем

Проверенный Processing 107ed4e3ae0d43300a09dea10b2d43ad4423efc1, IAM c715a079ab57d4c9e7b56862f7f2584c792b74b0. Первый сценарий ниже проверяет фактический Processing. Последующие сценарии — регрессии prior inventory и тестовой композиции HIT-154. [Команды остальных проверок и полный архив](RUNS.md).

## real-iam-inventory

```json
{"label": "real-iam-inventory", "argv": ["docker", "exec", "-e", "HITPIPE_IAM_TEST_ADMIN_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=postgres", "-e", "HITPIPE_IAM_TEST_APP_DSN=host=/var/run/postgresql dbname=hit68_test_hit148 sslmode=disable user=hp_hit81_app", "-e", "HIT148_PROCESSING_SOURCE=107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "9a0bd5be54e84339f69faf285722e2d5db2bb332e406085a4d965448386b9f39", "/test/iam-deletion.test", "-test.v", "-test.count=1", "-test.timeout=50s", "-test.run=^Test(ProcessingConsumesRealHIT154Inventory|ProcessingAppliesAndRestoresRealHIT154|PostgresCaseDeletionQualification)$"], "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion", "environment": "Inherited CLI environment; no database DSN/credentials accepted or used. Docker context is recorded by docker version.", "source_sha": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1", "tree_status_at_start": "", "runner_sha256": "6e00944af22aa1d1174ed2d7a21bd32a9bd38c9e4f1bf304d05fe3d2b8568aa2", "started_at": "2026-11-22T20:27:47.909974+00:00", "finished_at": "2026-11-22T20:27:50.378713+00:00", "exit_code": 0, "stdout": {"file": "040-real-iam-inventory.stdout", "bytes": 15888, "sha256": "375a9013b6a94362982b46540035359804040b757d6558506f865f3ced75b831"}, "stderr": {"file": "040-real-iam-inventory.stderr", "bytes": 211, "sha256": "3bf1af1eb3097352cfef057be04569b829779dcdfb736f8d6ee1d9abed2b6ff8"}}
```

```text
=== RUN   TestProcessingAppliesAndRestoresRealHIT154
    processing_deletion_flow_test.go:129: postgres=180007 role=hp_hit81_app isolated_workspaces=2
    processing_deletion_flow_test.go:152: HIT-128 PostgreSQL backup:
        === RUN   TestHIT148CopySQL
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.case_identity: rows=2
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.case_content: rows=3
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.safety_history: rows=2
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.history_content: rows=2
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.case_identity: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.case_content: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.safety_history: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.history_content: rows=1
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:167: scoped SELECT hp_processing.outbox: rows=0
        --- PASS: TestHIT148CopySQL (0.01s)
        PASS
    processing_deletion_flow_test.go:247: no authorized decision exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:248: unknown generation exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:290: purge denied for app exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:291: purge denied for worker exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:322: unknown missing body exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:330: transport interrupted after durable deletion exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:342: retry same application and actual proof exit=0 {"verified_decisions":1}
    processing_deletion_flow_test.go:350: repeat already applied exit=0 {"verified_decisions":1}
    processing_deletion_flow_test.go:152: HIT-128 PostgreSQL restore:
        === RUN   TestHIT148CopySQL
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.case_identity: rows=2
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.case_content: rows=3
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.safety_history: rows=2
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.history_content: rows=2
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.case_identity: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.case_content: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.safety_history: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.history_content: rows=1
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:171: scoped SELECT hp_processing.outbox: rows=0
        --- PASS: TestHIT148CopySQL (0.01s)
        PASS
    processing_deletion_flow_test.go:367: old generation cannot read after advance exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:387: actual Processing mTLS Health 200 CLOSED build_revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1
    processing_deletion_flow_test.go:388: restore reapplies before opening exit=0 {"verified_decisions":1}
    processing_deletion_flow_test.go:400: actual Processing mTLS Health 200 CLOSED build_revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1
    processing_deletion_flow_test.go:400: actual Processing mTLS Health 200 CLOSED build_revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1
    processing_deletion_flow_test.go:400: actual Processing stayed CLOSED on stale revision and when the live publisher refused its snapshot
    processing_deletion_flow_test.go:400: actual Processing mTLS Health 200 READY build_revision=107ed4e3ae0d43300a09dea10b2d43ad4423efc1
    processing_deletion_flow_test.go:400: actual mTLS readback of removed history detail returned 200, entry=f9c379b1-63f5-4838-83b0-60aeeeae06f1 original expires_at=2026-11-23T20:26:47.978408Z
    processing_deletion_flow_test.go:400: actual mTLS case detail returned 200 without either selected context version
    processing_deletion_flow_test.go:152: HIT-128 PostgreSQL verify:
        === RUN   TestHIT148CopySQL
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.case_identity: rows=2
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.case_content: rows=3
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.safety_history: rows=2
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.history_content: rows=2
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_identity: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_content: rows=3
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.safety_history: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.history_content: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_identity: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_content: rows=3
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.safety_history: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.history_content: rows=2
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:194: source, pre-deletion backup and restored SQL projections match e0886dd3178149725d8d799d896f1753b5e5323450a39529596b56ffc17b3df8
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.case_identity: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.case_content: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.safety_history: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.history_content: rows=1
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:190: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_identity: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_content: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.safety_history: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.history_content: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.operations: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.accepted_configs: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_identity: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.episodes: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.case_content: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.safety_history: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.history_content: rows=1
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.user_commands: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.inbox: rows=0
            case_deletion_fixture_test.go:191: scoped SELECT hp_processing.outbox: rows=0
            case_deletion_fixture_test.go:194: source, pre-deletion backup and restored SQL projections match 172f09a7d6ebed418fffb52ba49bd6f55339e132b6cd6c6a502be02a0612e5ad
        --- PASS: TestHIT148CopySQL (0.02s)
        PASS
    processing_deletion_flow_test.go:403: issuer unavailable exit=1 confirmed case deletion incomplete or refused
    processing_deletion_flow_test.go:404: complete: two checked durable receipts, exact old history expires_at preserved, no duplicate external effect
--- PASS: TestProcessingAppliesAndRestoresRealHIT154 (2.30s)
=== RUN   TestProcessingConsumesRealHIT154Inventory
    processing_inventory_probe_test.go:30: postgres=180007 role=hp_hit81_app isolated_workspaces=2
    processing_inventory_probe_test.go:128: signed-empty: exit=0 {"workspace_id":"a1ae8e37-caa8-4443-af38-41b727c74273","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_probe_test.go:129: neighbor-empty: exit=0 {"workspace_id":"a4d74605-4f62-42dc-a2ac-cc851627de15","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_probe_test.go:130: foreign-workspace: exit=1 trusted case deletion inventory unavailable
    processing_inventory_probe_test.go:131: unknown-generation: exit=1 trusted case deletion inventory unavailable
    processing_inventory_probe_test.go:132: wrong-mTLS-client: exit=1 trusted case deletion inventory unavailable
    processing_inventory_probe_test.go:133: wrong-server-pin: exit=1 trusted case deletion inventory unavailable
    processing_inventory_probe_test.go:134: wrong-signer: exit=1 trusted case deletion inventory unavailable
    processing_inventory_probe_test.go:144: owner-confirmed-applying: exit=0 {"workspace_id":"a1ae8e37-caa8-4443-af38-41b727c74273","generation":1,"revision":3,"decision_count":1,"serving":"CLOSED"}
    processing_inventory_probe_test.go:145: neighbor-stays-empty: exit=0 {"workspace_id":"a4d74605-4f62-42dc-a2ac-cc851627de15","generation":1,"revision":1,"decision_count":0,"serving":"CLOSED"}
    processing_inventory_probe_test.go:147: no-current-source: exit=1 trusted case deletion inventory unavailable
--- PASS: TestProcessingConsumesRealHIT154Inventory (0.07s)
=== RUN   TestPostgresCaseDeletionQualification
    qualification_integration_test.go:82: postgres=180007 role=hp_hit81_app isolated_workspaces=2
    qualification_integration_test.go:144: real TLS 1.3 publisher read: valid full empty rev=1; other cert and absent source scope denied
    qualification_integration_test.go:170: owner checks: foreign workspace, foreign target, downgraded operator and tampered offer denied without new record
    qualification_integration_test.go:209: committed APPLYING before synthetic mutation id=2867d880-50ec-48e6-a0d3-eb29d188b037 generation=1 apply_revision=5; pre-proof receipt rejected
    qualification_integration_test.go:237: reopened issuer, reloaded same id and apply revision; durable receipt stored once, published rev=6
    qualification_integration_test.go:307: restored subordinate backup independently; generation=2 old receipt/stale/partial/foreign states denied; applied exact old id and opened under rev=10
    qualification_integration_test.go:336: independent source missing over real TLS: no snapshot; restore stayed closed; registry/publication excluded synthetic content and credentials
--- PASS: TestPostgresCaseDeletionQualification (0.04s)
PASS
```
