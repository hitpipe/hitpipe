# GitHub Actions для проверенного исходника HIT-155

Наблюдение 2026-11-22T13:44:29.648551+00:00 UTC. Head `1ed91653a747b47a98f2738ca0d831a38cebc98a`. Все 6 заданий завершились SUCCESS. OCI-образ и два кластера проверялись отдельным локальным запуском.

Из корня рабочей копии выполнены команды чтения.

```sh
gh pr view 88 -R hitpipe/hitpipe --json baseRefOid,headRefOid,isDraft,state,statusCheckRollup,url
gh run view 45028666404 -R hitpipe/hitpipe --json headSha,status,conclusion,jobs,workflowName,url
gh run view 45028594444 -R hitpipe/hitpipe --json headSha,status,conclusion,jobs,workflowName,url
```

| Задание | Завершение UTC | Результат |
| --- | --- | --- |
| [pr12-iam](https://github.com/hitpipe/hitpipe/actions/runs/45028666472/job/137227187695) | 2026-11-22T13:42:33Z | SUCCESS |
| [pr12-delivery](https://github.com/hitpipe/hitpipe/actions/runs/45028666521/job/137227187577) | 2026-11-22T13:40:08Z | SUCCESS |
| [pr12-intake](https://github.com/hitpipe/hitpipe/actions/runs/45028666447/job/137227187216) | 2026-11-22T13:40:11Z | SUCCESS |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/45028666404/job/137227187085) | 2026-11-22T13:41:57Z | SUCCESS |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/45028594444/job/137226962419) | 2026-11-22T13:41:43Z | SUCCESS |
| [platform-start-gate](https://github.com/hitpipe/hitpipe/actions/runs/45028666456/job/137227187251) | 2026-11-22T13:40:54Z | SUCCESS |

### Processing 45028666404

Фактические шаги задания.

- Set up job · success
- Run actions/checkout@v4 · success
- Verify bootstrap documentation and frozen copies · success
- Verify bootstrap wrapper boundaries (not student tests) · success
- Report actual implementation state · success
- Run actions/setup-go@v5 · success
- Run actual named tests when module exists · success
- Run declared diagnostic smoke in both modes · success
- Race and mutation sensitivity for Processing · success
- Run actions/upload-artifact@v4 · success
- Scope of this job · success
- Post Run actions/setup-go@v5 · success
- Post Run actions/checkout@v4 · success
- Complete job · success

### Processing 45028594444

Фактические шаги задания.

- Set up job · success
- Run actions/checkout@v4 · success
- Verify bootstrap documentation and frozen copies · success
- Verify bootstrap wrapper boundaries (not student tests) · success
- Report actual implementation state · success
- Run actions/setup-go@v5 · success
- Run actual named tests when module exists · success
- Run declared diagnostic smoke in both modes · success
- Race and mutation sensitivity for Processing · success
- Run actions/upload-artifact@v4 · success
- Scope of this job · success
- Post Run actions/setup-go@v5 · success
- Post Run actions/checkout@v4 · success
- Complete job · success
