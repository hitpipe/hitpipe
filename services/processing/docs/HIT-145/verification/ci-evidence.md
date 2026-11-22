# HIT-145. GitHub Actions

На проверенном исходнике `30385d09962c3fa4aad4808253a5493235559d01` завершились все шесть наблюдавшихся проверок PR №84. Два прогона Processing относятся к событиям push и pull_request.

| Job | Вывод | Завершение UTC |
| --- | --- | --- |
| [pr12-iam](https://github.com/hitpipe/hitpipe/actions/runs/44878599741/job/136768701649) | SUCCESS | 2026-11-21T07:07:31Z |
| [pr12-delivery](https://github.com/hitpipe/hitpipe/actions/runs/44878599815/job/136768701634) | SUCCESS | 2026-11-21T07:05:46Z |
| [pr12-intake](https://github.com/hitpipe/hitpipe/actions/runs/44878599792/job/136768701677) | SUCCESS | 2026-11-21T07:05:43Z |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/44878599772/job/136768701599) | SUCCESS | 2026-11-21T07:07:12Z |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/44878595568/job/136768688600) | SUCCESS | 2026-11-21T07:07:11Z |
| [platform-start-gate](https://github.com/hitpipe/hitpipe/actions/runs/44878599853/job/136768702115) | SUCCESS | 2026-11-21T07:06:23Z |

Processing успешно выполнил проверки workspace, именованные тесты, диагностический smoke, race и лабораторный mutation runner. Изолированный контейнерный TEST и PostgreSQL 18.7 проверены локально отдельным стендом.

[Снимок PR](ci-pr.json) · [Подробности pull_request](ci-processing.json) · [Подробности push](ci-processing-push.json) · [Команды и SHA-256](ci-commands.json)

Вывод фиксирует коммит кода. После добавления протоколов текущий head и его CI проверяются отдельным снимком PR.

```sh
gh pr view 84 --json headRefOid,baseRefOid,isDraft,state,mergeable,author,statusCheckRollup,url
gh run view 44878599772 --json headSha,status,conclusion,jobs,url,workflowName,event
gh run view 44878595568 --json headSha,status,conclusion,jobs,url,workflowName,event
```
