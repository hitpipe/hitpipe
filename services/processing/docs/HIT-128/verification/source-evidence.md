# HIT-128. Исходники и зависимости

База `19831026d635dd74d4b54dd4acd93e53319c182e` соответствует head Draft PR №82, в котором находится принятый в Jira HIT-127. Его независимый итог — PASS WITH NOTES, 6/6 DoD. Исходный проверенный код HIT-127 — `4da48202e77131427730b1827df9ee81940e559a`; между этим SHA и базой изменена документация, исходники Processing совпадают. Наблюдавшийся main `ce771d37505e7770003d0796ae932556d06b7bab` этот стек не содержит. [PR №82](https://github.com/hitpipe/hitpipe/pull/82) · [Принятое независимое ревью](https://hitpipe.atlassian.net/browse/HIT-127?focusedCommentId=15638)

| Задача | Проверенный SHA из исходного результата | `git merge-base --is-ancestor <SHA> c426dc400a4bec03fafba72e2a5780c9d85af1e2` |
| --- | --- | ---: |
| HIT-111 | `b2e823012854575146595b01a6f56d5ac677fc44` | 0 |
| HIT-112 | `0f710a6682d15b437f847376b348947cb4757c7c` | 0 |
| HIT-113 | `9165e814cb7b46e9cfae484c62ccc70f63fc1ee3` | 0 |
| HIT-114 | `dd05559bc911437d33c0c2582b604d2f2728d362` | 0 |
| HIT-126 | `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` | 0 |
| HIT-127 | `4da48202e77131427730b1827df9ee81940e559a` | 0 |

Ранее проверенная связь через merge e381972, изменения конфликтных файлов и blob-хеши приведены в [доказательстве HIT-127](https://github.com/hitpipe/hitpipe/blob/19831026d635dd74d4b54dd4acd93e53319c182e/services/processing/docs/HIT-127/review/dependency-chain.md). [Полный diff исполняемого сценария](https://github.com/hitpipe/hitpipe/compare/19831026d635dd74d4b54dd4acd93e53319c182e...c426dc400a4bec03fafba72e2a5780c9d85af1e2)

```text
$ git diff --name-status 19831026d635dd74d4b54dd4acd93e53319c182e c426dc400a4bec03fafba72e2a5780c9d85af1e2
A	services/processing/internal/adapters/postgres/recovery_cluster_test.go
A	services/processing/internal/adapters/postgres/recovery_process_test.go
A	services/processing/internal/adapters/postgres/recovery_sql_test.go
M	services/processing/internal/adapters/postgres/runtime_process_test.go
A	services/processing/scripts/recovery/verify_recovery.py
$ git diff --exit-code 19831026d635dd74d4b54dd4acd93e53319c182e c426dc400a4bec03fafba72e2a5780c9d85af1e2 -- services/processing ':(exclude)**/*_test.go' ':(exclude)services/processing/scripts/recovery/**' services/platform
exit 0, пустой stdout
```

Принятые исполняемые компоненты, модули, SQL, Platform и контрактные обработчики сохранены. Результат ограничен исполняемым каталогом сценария и файлами тестов. [Runner](https://github.com/hitpipe/hitpipe/blob/c426dc400a4bec03fafba72e2a5780c9d85af1e2/services/processing/scripts/recovery/verify_recovery.py) · [SQL-проекции и вставка через RLS](https://github.com/hitpipe/hitpipe/blob/c426dc400a4bec03fafba72e2a5780c9d85af1e2/services/processing/internal/adapters/postgres/recovery_sql_test.go) · [Сценарий отказов](https://github.com/hitpipe/hitpipe/blob/c426dc400a4bec03fafba72e2a5780c9d85af1e2/services/processing/internal/adapters/postgres/recovery_process_test.go) · [Раздельные PostgreSQL](https://github.com/hitpipe/hitpipe/blob/c426dc400a4bec03fafba72e2a5780c9d85af1e2/services/processing/internal/adapters/postgres/recovery_cluster_test.go)

Политика переноса следует HP-MVP-A-025, HP-MVP-A-026 и HP-MVP-A-027. Для PostgreSQL применена версия 18.7 по актуальной карточке и поправке HIT-63. [Принятая архитектура](https://github.com/hitpipe/project-os/blob/b2461564e4355ea5a72a55a30a635655397c463b/contracts/hitpipe/mvp-2026-10-31/SYSTEM_DESIGN_MVP_RU.md) · [Поправка PostgreSQL](https://hitpipe.atlassian.net/browse/HIT-63?focusedCommentId=12229)
