# HIT-148 — GitHub Actions

Получено 2026-11-22T19:55:32.466841+00:00. Проверенный код `0eba954c24652655cfc661fc63628b2097ebd33e`. Шесть успешных проверок существующих workflows. Тест настоящего инвентаря выполнен отдельно локально.

Команды получения (exit 0): `gh run list -R hitpipe/hitpipe --commit 0eba954c24652655cfc661fc63628b2097ebd33e --json databaseId,headSha,name,status,conclusion,url --limit 10` и `gh run view 45063165986 -R hitpipe/hitpipe --log`. SHA-256 полного журнала `9784c23eb1a9650c43ec8fafdd3874ec9bbbdac1d54fd7465c41265ad5579d90`.

```json
[
  {
    "conclusion": "success",
    "databaseId": 45063166035,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "PR12 intake — bootstrap or submitted component checks",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063166035"
  },
  {
    "conclusion": "success",
    "databaseId": 45063166275,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "PR12 delivery — bootstrap or submitted component checks",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063166275"
  },
  {
    "conclusion": "success",
    "databaseId": 45063165996,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "Platform — локальная защита начала работы",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063165996"
  },
  {
    "conclusion": "success",
    "databaseId": 45063165992,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "IAM — проверки текущей реализации",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063165992"
  },
  {
    "conclusion": "success",
    "databaseId": 45063165986,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "PR12 processing — bootstrap or submitted component checks",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063165986"
  },
  {
    "conclusion": "success",
    "databaseId": 45063159982,
    "headSha": "0eba954c24652655cfc661fc63628b2097ebd33e",
    "name": "PR12 processing — bootstrap or submitted component checks",
    "status": "completed",
    "url": "https://github.com/hitpipe/hitpipe/actions/runs/45063159982"
  }
]
```

Выборка команд и результатов проверки Processing.

```text
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:16.5131564Z + go vet ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:34.2001567Z + go test -count=1 -json -coverprofile=/home/runner/work/hitpipe/hitpipe/.pr12-build/processing.cover ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9620855Z {"Time":"2026-11-22T19:52:39.28400841Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9630605Z {"Time":"2026-11-22T19:52:39.284087038Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9635467Z {"Time":"2026-11-22T19:52:39.284096933Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing","Output":"--- PASS: TestCLIRejectsRefundShadowing (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9643848Z {"Time":"2026-11-22T19:52:39.284689951Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIConfiguration","Output":"--- PASS: TestCLIConfiguration (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9651583Z {"Time":"2026-11-22T19:52:39.285086349Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestOutputFailure","Output":"--- PASS: TestOutputFailure (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9655247Z {"Time":"2026-11-22T19:52:39.285164336Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9685052Z {"Time":"2026-11-22T19:52:39.520482904Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/ok","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/ok (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9693090Z {"Time":"2026-11-22T19:52:39.523862596Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/no_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/no_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9699237Z {"Time":"2026-11-22T19:52:39.528021311Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/foreign_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/foreign_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9704673Z {"Time":"2026-11-22T19:52:39.531845974Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/untrusted_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/untrusted_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9710121Z {"Time":"2026-11-22T19:52:39.535585853Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/expired_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/expired_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9715198Z {"Time":"2026-11-22T19:52:39.537767089Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/TLS1.2","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/TLS1.2 (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9720679Z {"Time":"2026-11-22T19:52:39.540967396Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/missing_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/missing_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9727923Z {"Time":"2026-11-22T19:52:39.545829942Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/forged_authority","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/forged_authority (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9735935Z {"Time":"2026-11-22T19:52:39.549374623Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_grant","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_grant (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9741625Z {"Time":"2026-11-22T19:52:39.558716608Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_epoch","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_epoch (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9747090Z {"Time":"2026-11-22T19:52:39.562176681Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/wrong_workspace","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/wrong_workspace (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9752963Z {"Time":"2026-11-22T19:52:39.565768522Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9758807Z {"Time":"2026-11-22T19:52:39.569141995Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/revoked_before_response","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/revoked_before_response (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9764662Z {"Time":"2026-11-22T19:52:39.572512353Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/denied_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/denied_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9770235Z {"Time":"2026-11-22T19:52:39.575913707Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/database_error","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/database_error (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9775501Z {"Time":"2026-11-22T19:52:39.580351523Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/invalid_query","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/invalid_query (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9778556Z {"Time":"2026-11-22T19:52:39.580415219Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries (0.07s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9783628Z {"Time":"2026-11-22T19:52:39.585171455Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestParsingAndUnavailableComposition","Output":"--- PASS: TestParsingAndUnavailableComposition (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9785834Z {"Time":"2026-11-22T19:52:39.585190955Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9788400Z {"Time":"2026-11-22T19:52:39.60625479Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestMaintenanceExcludesRuntimeAcrossRenameAndCommit","Output":"--- PASS: TestMaintenanceExcludesRuntimeAcrossRenameAndCommit (0.06s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9796415Z {"Time":"2026-11-22T19:52:39.608429547Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestMaintenanceRejectsUnsafeNamesAndRechecksIdentity","Output":"--- PASS: TestMaintenanceRejectsUnsafeNamesAndRechecksIdentity (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:52:43.9805064Z {"Time":"2026-11-22T19:52:39.609437315Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace (0.00s)\n","OutputType":"frame"}
```
