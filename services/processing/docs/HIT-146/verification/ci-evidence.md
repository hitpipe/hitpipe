# CI проверенного исходника HIT-146

Наблюдение UTC 2026-11-22T15:20:41.507963+00:00. Head `ce317193244623c5a464a3014bb39f32875c1e98`. 6/6 заданий GitHub Actions завершились SUCCESS. Настоящий IAM был задействован в отдельном локальном прогоне.

Команда чтения CI, exit 0.

```sh
gh pr view 90 -R hitpipe/hitpipe --json baseRefOid,headRefOid,isDraft,state,statusCheckRollup,url
```

| Задание | Завершение UTC | Результат |
| --- | --- | --- |
| [pr12-iam](https://github.com/hitpipe/hitpipe/actions/runs/45037482726/job/137254348840) | 2026-11-22T15:20:11Z | SUCCESS |
| [pr12-delivery](https://github.com/hitpipe/hitpipe/actions/runs/45037482401/job/137254347567) | 2026-11-22T15:17:48Z | SUCCESS |
| [pr12-intake](https://github.com/hitpipe/hitpipe/actions/runs/45037482443/job/137254348035) | 2026-11-22T15:17:45Z | SUCCESS |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/45037482882/job/137254349326) | 2026-11-22T15:20:02Z | SUCCESS |
| [pr12-processing](https://github.com/hitpipe/hitpipe/actions/runs/45037478978/job/137254337576) | 2026-11-22T15:19:22Z | SUCCESS |
| [platform-start-gate](https://github.com/hitpipe/hitpipe/actions/runs/45037482850/job/137254349186) | 2026-11-22T15:18:25Z | SUCCESS |

## История исправления запуска

На e19c0a4 оба Processing job отклонили размещение исходника тестового IAM внутри Processing. Исполняемый тест на новой версии собран с Go overlay в контексте защищённых IAM packages. Тестовый launcher располагается в tools/hit146 с явным build tag и не является частью модуля Processing. Общий CI сохранён. [Первый отклонённый job](https://github.com/hitpipe/hitpipe/actions/runs/45037247488/job/137253628083) · [второй](https://github.com/hitpipe/hitpipe/actions/runs/45037214399/job/137253520720).

Исходный лог первого job SHA-256 8865638ce3a4b32e5208c6ccc7176cef12ec8d39c70b13e69f5e67b1bbb9f838. Значимые строки.

```text
ERROR: scripts/postgres/testdata/hit146/iam-process.go: foreign service internal import github.com/hitpipe/hitpipe/services/iam/internal/adapters/composition/privateiam
scripts/postgres/testdata/hit146/iam-process.go: foreign service internal import github.com/hitpipe/hitpipe/services/iam/internal/adapters/http/revocation
```
