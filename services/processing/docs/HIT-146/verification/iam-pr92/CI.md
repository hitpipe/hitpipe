# HIT-146 — GitHub Actions

Снято 2026-11-22T19:46:59.458776+00:00. SHA 2b20016b39188138583da860e8c0bd2fc0c49e8e. Все шесть проверок завершились SUCCESS. Коммит содержит тот же код 717c954fca4f06f165074a8688f0e94a31b398e7 и пакет локальных доказательств.

Команды получения: `gh pr view 90 -R hitpipe/hitpipe --json headRefOid,baseRefOid,isDraft,state,statusCheckRollup` (exit 0), `gh run view 45062211666 -R hitpipe/hitpipe --log` (exit 0). SHA-256 полного GitHub журнала: 0658f486d4760b73bf1ad9a2acc3dbb67369d6a264ef04ec123f40a76ad3f965.

```json
{
  "baseRefOid": "49229e1db3f371a051a022ea9e31951dd558eecb",
  "headRefOid": "2b20016b39188138583da860e8c0bd2fc0c49e8e",
  "isDraft": true,
  "state": "OPEN",
  "statusCheckRollup": [
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:43:59Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062211543/job/137330432067",
      "name": "pr12-iam",
      "startedAt": "2026-11-22T19:41:52Z",
      "status": "COMPLETED",
      "workflowName": "IAM \u2014 \u043f\u0440\u043e\u0432\u0435\u0440\u043a\u0438 \u0442\u0435\u043a\u0443\u0449\u0435\u0439 \u0440\u0435\u0430\u043b\u0438\u0437\u0430\u0446\u0438\u0438"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:42:00Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062211527/job/137330431876",
      "name": "pr12-delivery",
      "startedAt": "2026-11-22T19:41:52Z",
      "status": "COMPLETED",
      "workflowName": "PR12 delivery \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:42:01Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062211534/job/137330431853",
      "name": "pr12-intake",
      "startedAt": "2026-11-22T19:41:52Z",
      "status": "COMPLETED",
      "workflowName": "PR12 intake \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:43:40Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062211666/job/137330431988",
      "name": "pr12-processing",
      "startedAt": "2026-11-22T19:41:52Z",
      "status": "COMPLETED",
      "workflowName": "PR12 processing \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:44:19Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062206154/job/137330415420",
      "name": "pr12-processing",
      "startedAt": "2026-11-22T19:41:48Z",
      "status": "COMPLETED",
      "workflowName": "PR12 processing \u2014 bootstrap or submitted component checks"
    },
    {
      "__typename": "CheckRun",
      "completedAt": "2026-11-22T19:42:42Z",
      "conclusion": "SUCCESS",
      "detailsUrl": "https://github.com/hitpipe/hitpipe/actions/runs/45062211565/job/137330432013",
      "name": "platform-start-gate",
      "startedAt": "2026-11-22T19:41:52Z",
      "status": "COMPLETED",
      "workflowName": "Platform \u2014 \u043b\u043e\u043a\u0430\u043b\u044c\u043d\u0430\u044f \u0437\u0430\u0449\u0438\u0442\u0430 \u043d\u0430\u0447\u0430\u043b\u0430 \u0440\u0430\u0431\u043e\u0442\u044b"
    }
  ]
}
```

Выборка команд и результатов из журнала Processing. Сквозной реальный IAM тест имеет отдельный локальный журнал.

```text
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:06.4084114Z + go vet ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:22.0941083Z + go test -count=1 -json -coverprofile=/home/runner/work/hitpipe/hitpipe/.pr12-build/processing.cover ./...
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3194738Z {"Time":"2026-11-22T19:42:26.075039748Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3201618Z {"Time":"2026-11-22T19:42:26.07508203Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json","Output":"--- PASS: TestCLIRejectsRefundShadowing/refund-shadowed-reverse.json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3205595Z {"Time":"2026-11-22T19:42:26.075090603Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIRejectsRefundShadowing","Output":"--- PASS: TestCLIRejectsRefundShadowing (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3211106Z {"Time":"2026-11-22T19:42:26.075650392Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestCLIConfiguration","Output":"--- PASS: TestCLIConfiguration (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3216471Z {"Time":"2026-11-22T19:42:26.076040158Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Test":"TestOutputFailure","Output":"--- PASS: TestOutputFailure (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3218880Z {"Time":"2026-11-22T19:42:26.076048365Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/cmd/processing-contractcheck","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3235526Z {"Time":"2026-11-22T19:42:26.273414682Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/ok","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/ok (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3240359Z {"Time":"2026-11-22T19:42:26.277538595Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/no_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/no_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3245690Z {"Time":"2026-11-22T19:42:26.280359551Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/foreign_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/foreign_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3250665Z {"Time":"2026-11-22T19:42:26.283946599Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/untrusted_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/untrusted_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3255819Z {"Time":"2026-11-22T19:42:26.286663211Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/expired_cert","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/expired_cert (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3260662Z {"Time":"2026-11-22T19:42:26.288065654Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/TLS1.2","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/TLS1.2 (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3266182Z {"Time":"2026-11-22T19:42:26.290749958Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/missing_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/missing_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3271353Z {"Time":"2026-11-22T19:42:26.295069734Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/forged_authority","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/forged_authority (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3277565Z {"Time":"2026-11-22T19:42:26.297108146Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_grant","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_grant (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3283421Z {"Time":"2026-11-22T19:42:26.29996321Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/stale_epoch","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/stale_epoch (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3289629Z {"Time":"2026-11-22T19:42:26.302665907Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/wrong_workspace","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/wrong_workspace (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3297076Z {"Time":"2026-11-22T19:42:26.305161669Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/closed_SQL_gate (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3302324Z {"Time":"2026-11-22T19:42:26.307984718Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/revoked_before_response","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/revoked_before_response (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3307712Z {"Time":"2026-11-22T19:42:26.310872242Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/denied_session","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/denied_session (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3312806Z {"Time":"2026-11-22T19:42:26.314303292Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/database_error","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/database_error (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3317686Z {"Time":"2026-11-22T19:42:26.316001667Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries/invalid_query","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries/invalid_query (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3320217Z {"Time":"2026-11-22T19:42:26.316109087Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestRealMTLSAndFreshReadBoundaries","Output":"--- PASS: TestRealMTLSAndFreshReadBoundaries (0.05s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3325056Z {"Time":"2026-11-22T19:42:26.321194047Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Test":"TestParsingAndUnavailableComposition","Output":"--- PASS: TestParsingAndUnavailableComposition (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3327144Z {"Time":"2026-11-22T19:42:26.321211882Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/casereadhttp","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3337725Z {"Time":"2026-11-22T19:42:26.34256618Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/workspace (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3343486Z {"Time":"2026-11-22T19:42:26.342596816Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/case","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/case (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3349015Z {"Time":"2026-11-22T19:42:26.342605532Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/revision","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/revision (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3354543Z {"Time":"2026-11-22T19:42:26.342616094Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/hash","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/hash (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3360080Z {"Time":"2026-11-22T19:42:26.34262495Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/class","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/class (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3365752Z {"Time":"2026-11-22T19:42:26.342636882Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/bytes","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/bytes (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3371155Z {"Time":"2026-11-22T19:42:26.342644946Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/expiry","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/expiry (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3376780Z {"Time":"2026-11-22T19:42:26.342652552Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/traversal","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal/traversal (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3379540Z {"Time":"2026-11-22T19:42:26.34285924Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestReadOwnedCausalScopeIntegrityExpiryAndTraversal","Output":"--- PASS: TestReadOwnedCausalScopeIntegrityExpiryAndTraversal (0.02s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3384389Z {"Time":"2026-11-22T19:42:26.346636388Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestDurableContentAndRestart","Output":"--- PASS: TestDurableContentAndRestart (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3391427Z {"Time":"2026-11-22T19:42:26.347045416Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestFaultStepsNeverReturnAcceptedRef/write","Output":"--- PASS: TestFaultStepsNeverReturnAcceptedRef/write (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3396540Z {"Time":"2026-11-22T19:42:26.347774405Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestFaultStepsNeverReturnAcceptedRef/file-fsync","Output":"--- PASS: TestFaultStepsNeverReturnAcceptedRef/file-fsync (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3402014Z {"Time":"2026-11-22T19:42:26.348506987Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestFaultStepsNeverReturnAcceptedRef/rename","Output":"--- PASS: TestFaultStepsNeverReturnAcceptedRef/rename (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3407576Z {"Time":"2026-11-22T19:42:26.349207751Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestFaultStepsNeverReturnAcceptedRef/directory-fsync","Output":"--- PASS: TestFaultStepsNeverReturnAcceptedRef/directory-fsync (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3410169Z {"Time":"2026-11-22T19:42:26.349277211Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestFaultStepsNeverReturnAcceptedRef","Output":"--- PASS: TestFaultStepsNeverReturnAcceptedRef (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3415265Z {"Time":"2026-11-22T19:42:26.359034018Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestExpiryCorruptionTraversalAndSymlink","Output":"--- PASS: TestExpiryCorruptionTraversalAndSymlink (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3420502Z {"Time":"2026-11-22T19:42:26.364754792Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestUncertainCommitOrphanRetainedUntilExpiry","Output":"--- PASS: TestUncertainCommitOrphanRetainedUntilExpiry (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3425460Z {"Time":"2026-11-22T19:42:26.372060223Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestContentProcessRestart","Output":"--- PASS: TestContentProcessRestart (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3432211Z {"Time":"2026-11-22T19:42:26.37872973Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestCrashBetweenFileSteps/write","Output":"--- PASS: TestCrashBetweenFileSteps/write (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3437242Z {"Time":"2026-11-22T19:42:26.386022386Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestCrashBetweenFileSteps/file-fsync","Output":"--- PASS: TestCrashBetweenFileSteps/file-fsync (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3442133Z {"Time":"2026-11-22T19:42:26.392216641Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestCrashBetweenFileSteps/rename","Output":"--- PASS: TestCrashBetweenFileSteps/rename (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3446914Z {"Time":"2026-11-22T19:42:26.396405352Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestCrashBetweenFileSteps/directory-fsync","Output":"--- PASS: TestCrashBetweenFileSteps/directory-fsync (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3449225Z {"Time":"2026-11-22T19:42:26.396427804Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Test":"TestCrashBetweenFileSteps","Output":"--- PASS: TestCrashBetweenFileSteps (0.02s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3451192Z {"Time":"2026-11-22T19:42:26.396435529Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/contentfiles","Output":"PASS\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3459159Z {"Time":"2026-11-22T19:42:26.474797789Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestActualTLSExactBytesAndNoRedirect","Output":"--- PASS: TestActualTLSExactBytesAndNoRedirect (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3466594Z {"Time":"2026-11-22T19:42:26.486293368Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/foreign_ca","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/foreign_ca (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3471692Z {"Time":"2026-11-22T19:42:26.486323538Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/wrong_service","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/wrong_service (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3477495Z {"Time":"2026-11-22T19:42:26.490046112Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/expired_client","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/expired_client (0.01s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3484594Z {"Time":"2026-11-22T19:42:26.490929168Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/missing_client","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/missing_client (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3490825Z {"Time":"2026-11-22T19:42:26.492991798Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/wrong_server","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/wrong_server (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3496500Z {"Time":"2026-11-22T19:42:26.494083662Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/closed_gate","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/closed_gate (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3501975Z {"Time":"2026-11-22T19:42:26.495109456Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler/no_gate_call","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler/no_gate_call (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3504617Z {"Time":"2026-11-22T19:42:26.495148901Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestTLSAndGateRejectionBeforeHandler","Output":"--- PASS: TestTLSAndGateRejectionBeforeHandler (0.02s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3511756Z {"Time":"2026-11-22T19:42:26.497896793Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestReceiptStatusContentAndRetryAfter/empty202","Output":"--- PASS: TestReceiptStatusContentAndRetryAfter/empty202 (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3516932Z {"Time":"2026-11-22T19:42:26.500659146Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestReceiptStatusContentAndRetryAfter/bad_id","Output":"--- PASS: TestReceiptStatusContentAndRetryAfter/bad_id (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3522357Z {"Time":"2026-11-22T19:42:26.503054955Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestReceiptStatusContentAndRetryAfter/bad_json","Output":"--- PASS: TestReceiptStatusContentAndRetryAfter/bad_json (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3527455Z {"Time":"2026-11-22T19:42:26.505927754Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestReceiptStatusContentAndRetryAfter/oversized","Output":"--- PASS: TestReceiptStatusContentAndRetryAfter/oversized (0.00s)\n","OutputType":"frame"}
pr12-processing	Run actual named tests when module exists	2026-11-22T19:42:29.3532611Z {"Time":"2026-11-22T19:42:26.511932707Z","Action":"output","Package":"github.com/hitpipe/hitpipe/services/processing/internal/adapters/deliveryhttp","Test":"TestReceiptStatusContentAndRetryAfter/wrong_type","Output":"--- PASS: TestReceiptStatusContentAndRetryAfter/wrong_type (0.01s)\n","OutputType":"frame"}
```
