# HIT-128. GitHub Actions

На SHA `c426dc400a4bec03fafba72e2a5780c9d85af1e2` все шесть проверок GitHub Actions завершились успешно.
Два job Processing относятся к push и pull_request. Реальное восстановление двух кластеров проверялось отдельно на локальном стенде.

| Job | Статус | Завершение UTC |
| --- | --- | --- |
| [pr12-iam](https://github.com/hitpipe/hitpipe/actions/runs/44881435140/job/136777382723) | SUCCESS | 2026-11-21T07:44:25Z |
| [pr12-delivery](https://github.com/hitpipe/hitpipe/actions/runs/44881435104/job/136777382707) | SUCCESS | 2026-11-21T07:42:09Z |
| [pr12-intake](https://github.com/hitpipe/hitpipe/actions/runs/44881435105/job/136777382272) | SUCCESS | 2026-11-21T07:42:06Z |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/44881435097/job/136777382454) | SUCCESS | 2026-11-21T07:43:38Z |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/44881430920/job/136777370075) | SUCCESS | 2026-11-21T07:43:45Z |
| [platform-start-gate](https://github.com/hitpipe/hitpipe/actions/runs/44881435121/job/136777382413) | SUCCESS | 2026-11-21T07:42:41Z |

[Снимок CI](ci-pr.json) · [Команды и SHA, исходные результаты обоих job Processing](ci-commands.json)

Проверки после добавления отчётного коммита доступны на странице Draft PR №85.

```sh
gh pr view 85 --json headRefOid,baseRefOid,author,isDraft,state,mergeable,statusCheckRollup,url
gh run view 44881435097 --json headSha,event,status,conclusion,jobs,url,workflowName
gh run view 44881430920 --json headSha,event,status,conclusion,jobs,url,workflowName
```
