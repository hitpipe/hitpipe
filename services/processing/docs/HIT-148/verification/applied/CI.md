# HIT-148. GitHub Actions

Снято 2026-11-22T20:30:06.872278+00:00 UTC. Все шесть проверок завершились SUCCESS на инженерном SHA `107ed4e3ae0d43300a09dea10b2d43ad4423efc1`. Сквозной прогон реального удаления и восстановления выполнен отдельно локально.

Команды получения (exit 0): `gh pr view 86 -R hitpipe/hitpipe --json headRefOid,baseRefOid,isDraft,state,statusCheckRollup`, `gh run view 45066472502 -R hitpipe/hitpipe --log`. SHA-256 полного CI-журнала `a4f9bca65feaa9978e7a7b17a3e17d25e4890654e89d2e26ca7941372ded2ec8`.

```json
{
  "baseRefOid": "8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3",
  "headRefOid": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
  "isDraft": true,
  "state": "OPEN",
  "statusCheckRollup": [
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:29:10Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066472489/job/137343639342",
      "name": "pr12-iam",
      "startedAt": "2026-11-22T20:26:53Z",
      "status": "COMPLETED",
      "workflowName": "IAM \u2014 \u043f\u0440\u043e\u0432\u0435\u0440\u043a\u0438 \u0442\u0435\u043a\u0443\u0449\u0435\u0439 \u0440\u0435\u0430\u043b\u0438\u0437\u0430\u0446\u0438\u0438"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:26:59Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066472512/job/137343639323",
      "name": "pr12-delivery",
      "startedAt": "2026-11-22T20:26:53Z",
      "status": "COMPLETED",
      "workflowName": "PR12 delivery \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:27:01Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066472564/job/137343639360",
      "name": "pr12-intake",
      "startedAt": "2026-11-22T20:26:53Z",
      "status": "COMPLETED",
      "workflowName": "PR12 intake \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:28:48Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066472502/job/137343639421",
      "name": "pr12-processing",
      "startedAt": "2026-11-22T20:26:53Z",
      "status": "COMPLETED",
      "workflowName": "PR12 processing \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:29:33Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066468207/job/137343626985",
      "name": "pr12-processing",
      "startedAt": "2026-11-22T20:26:51Z",
      "status": "COMPLETED",
      "workflowName": "PR12 processing \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T20:27:42Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45066472539/job/137343639440",
      "name": "platform-start-gate",
      "startedAt": "2026-11-22T20:26:53Z",
      "status": "COMPLETED",
      "workflowName": "Platform \u2014 \u043b\u043e\u043a\u0430\u043b\u044c\u043d\u0430\u044f \u0437\u0430\u0449\u0438\u0442\u0430 \u043d\u0430\u0447\u0430\u043b\u0430 \u0440\u0430\u0431\u043e\u0442\u044b"
    }
  ]
}
```

Выборка фактических строк Processing workflow.

```text
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:06.6857339Z + go vet ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:22.7910426Z + go test -count=1 -json -coverprofile=/home/runner/work/hitpipe/hitpipe/.pr12-build/processing.cover ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3093128Z {"Time":"2026-11-22T20:27:26.802278552Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3098229Z {"Time":"2026-11-22T20:27:26.802343707Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3100895Z {"Time":"2026-11-22T20:27:26.802378347Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing","Output":"--- PASS: TestCLIRejectsRefundShadowing (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3105693Z {"Time":"2026-11-22T20:27:26.802862344Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIConfiguration","Output":"--- PASS: TestCLIConfiguration (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3110613Z {"Time":"2026-11-22T20:27:26.803070174Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestOutputFailure","Output":"--- PASS: TestOutputFailure (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3112884Z {"Time":"2026-11-22T20:27:26.803092595Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3124976Z {"Time":"2026-11-22T20:27:27.048916326Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/ok","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/ok (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3130834Z {"Time":"2026-11-22T20:27:27.053452941Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/no_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/no_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3135799Z {"Time":"2026-11-22T20:27:27.054502585Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/foreign_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/foreign_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3142088Z {"Time":"2026-11-22T20:27:27.057466697Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/untrusted_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/untrusted_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3149943Z {"Time":"2026-11-22T20:27:27.060322044Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/expired_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/expired_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3159022Z {"Time":"2026-11-22T20:27:27.061828295Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/TLS1.2","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/TLS1.2 (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3165878Z {"Time":"2026-11-22T20:27:27.064718005Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/missing_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/missing_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3173030Z {"Time":"2026-11-22T20:27:27.069528917Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/forged_authority","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/forged_authority (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3182151Z {"Time":"2026-11-22T20:27:27.078707929Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_grant","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_grant (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3191098Z {"Time":"2026-11-22T20:27:27.082133762Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_epoch","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_epoch (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3199229Z {"Time":"2026-11-22T20:27:27.084927638Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/wrong_workspace","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/wrong_workspace (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3207999Z {"Time":"2026-11-22T20:27:27.087768778Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3216261Z {"Time":"2026-11-22T20:27:27.094965759Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/revoked_before_response","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/revoked_before_response (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3222927Z {"Time":"2026-11-22T20:27:27.098733426Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/denied_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/denied_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3229712Z {"Time":"2026-11-22T20:27:27.102149684Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/database_error","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/database_error (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3236105Z {"Time":"2026-11-22T20:27:27.103708936Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/invalid_query","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/invalid_query (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3238539Z {"Time":"2026-11-22T20:27:27.103777009Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries (0.06s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3242770Z {"Time":"2026-11-22T20:27:27.113255504Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestParsingAndUnavailableComposition","Output":"--- PASS: TestParsingAndUnavailableComposition (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3244981Z {"Time":"2026-11-22T20:27:27.113331256Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3258768Z {"Time":"2026-11-22T20:27:27.344441041Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestMaintenanceExcludesRuntimeAcrossRenameAndCommit","Output":"--- PASS: TestMaintenanceExcludesRuntimeAcrossRenameAndCommit (0.02s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3263750Z {"Time":"2026-11-22T20:27:27.345283797Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestMaintenanceRejectsUnsafeNamesAndRechecksIdentity","Output":"--- PASS: TestMaintenanceRejectsUnsafeNamesAndRechecksIdentity (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T20:27:31.3271634Z {"Time":"2026-11-22T20:27:27.349809072Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace (0.00s)\n","OutputType":"frame"}
```
