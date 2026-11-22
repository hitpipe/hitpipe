# HIT-148. Проверяемые исходники и зависимости

| Зависимость | Проверенный SHA | Как использована |
| --- | --- | --- |
| HIT-126 | `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` | Прямой предок Processing через цепочку PR №86. Использованы существующие ограниченная очистка, FORCE RLS и эксклюзивная файловая блокировка. |
| HIT-128 | `c426dc400a4bec03fafba72e2a5780c9d85af1e2` | Прямой предок Processing, пустой исполняемый diff до базы PR №86. Использован существующий перенос SQL-проекции обычной ролью, с сохранением ограничений и защитных записей. |
| HIT-154 | `c715a079ab57d4c9e7b56862f7f2584c792b74b0` | Отдельная чистая копия. Точный предок итоговой ветки PR №89. Go overlay добавляет файлы тестовой композиции, принятые реализации издателя и IAM в чистой копии. |

База PR №86 `8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3`. Исходный HEAD задачи `f0077766a0df0f817b7a07a50e2ac3972a011c58`. Проверенный код `107ed4e3ae0d43300a09dea10b2d43ad4423efc1`. [Полный diff от базы](https://github.com/hitpipe/hitpipe/compare/8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3...107ed4e3ae0d43300a09dea10b2d43ad4423efc1). SHA-256 распакованного `source.patch.gz` равен `7c5ae8967967995525bbe25e5176ce3ad0a1db49a7d243eeaa4d17de4b236309`.

- [Применение только актуального подписанного APPLYING и защищённая отправка квитанции](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/services/processing/cmd/processing-purge/apply_deletion.go)
- [Роль, RLS, SQL-фиксация точных версий, защита живых ссылок и повторная проверка квитанции](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/services/processing/internal/adapters/postgres/case_deletion.go)
- [Минимальная отметка до unlink, подтверждение состояния и fsync](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/services/processing/internal/adapters/contentfiles/deletion.go)
- [Локальная композиция WithApplied и блокировка открывающего вызова](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/services/processing/maintenance/restore.go)
- [Доказательство подлинного WithCurrent, отказы и интеграционный тест](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/tools/hit148/deletion-flow.go), [фактические Health и HTTP readback](https://github.com/hitpipe/hitpipe/blob/107ed4e3ae0d43300a09dea10b2d43ad4423efc1/tools/hit148/deletion-runtime.go)

Команды Git, UTC 2026-11-22T20:29:14.820893+00:00.

```json
[
  {
    "command": "git rev-parse 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "107ed4e3ae0d43300a09dea10b2d43ad4423efc1\n",
    "stderr": ""
  },
  {
    "command": "git merge-base --is-ancestor 4c2f1df34f7cf72ddb2de1f6888e033d035913ac 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git merge-base --is-ancestor c426dc400a4bec03fafba72e2a5780c9d85af1e2 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git merge-base --is-ancestor 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git merge-base --is-ancestor f0077766a0df0f817b7a07a50e2ac3972a011c58 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git diff --name-status f0077766a0df0f817b7a07a50e2ac3972a011c58 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "A\tservices/processing/cmd/processing-purge/apply_deletion.go\nM\tservices/processing/cmd/processing-purge/main.go\nA\tservices/processing/internal/adapters/contentfiles/deletion.go\nM\tservices/processing/internal/adapters/contentfiles/read_owned.go\nM\tservices/processing/internal/adapters/contentfiles/store.go\nM\tservices/processing/internal/adapters/deletionregistry/read.go\nA\tservices/processing/internal/adapters/deletionregistry/receipt.go\nA\tservices/processing/internal/adapters/postgres/case_deletion.go\nA\tservices/processing/internal/adapters/postgres/case_deletion_fixture_test.go\nM\tservices/processing/internal/adapters/postgres/observation_read.go\nM\tservices/processing/internal/adapters/postgres/read_cases.go\nM\tservices/processing/internal/application/readcases/read.go\nM\tservices/processing/internal/runtime/config.go\nA\tservices/processing/internal/runtime/restore.go\nM\tservices/processing/internal/runtime/runtime.go\nA\tservices/processing/maintenance/case_deletion.go\nA\tservices/processing/maintenance/restore.go\nM\tservices/processing/scripts/recovery/verify_deletion_inventory.py\nA\ttools/hit148/deletion-flow.go\nA\ttools/hit148/deletion-runtime.go\n",
    "stderr": ""
  },
  {
    "command": "git diff --check f0077766a0df0f817b7a07a50e2ac3972a011c58 107ed4e3ae0d43300a09dea10b2d43ad4423efc1",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git diff --exit-code f0077766a0df0f817b7a07a50e2ac3972a011c58 107ed4e3ae0d43300a09dea10b2d43ad4423efc1 -- services/processing/scripts/migrations services/processing/tools/drafts services/platform services/iam .github/workflows",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git diff --exit-code c426dc400a4bec03fafba72e2a5780c9d85af1e2 8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3 -- services/processing/internal services/processing/cmd services/processing/scripts",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git rev-parse HEAD",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit": 0,
    "stdout": "c715a079ab57d4c9e7b56862f7f2584c792b74b0\n",
    "stderr": ""
  },
  {
    "command": "git status --porcelain",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git merge-base --is-ancestor c715a079ab57d4c9e7b56862f7f2584c792b74b0 fde4ad66cf736bd4ab67af926ae98f4de8eb5f69",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git diff --exit-code c715a079ab57d4c9e7b56862f7f2584c792b74b0 fde4ad66cf736bd4ab67af926ae98f4de8eb5f69 -- services/iam/internal services/iam/cmd",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "command": "git diff --exit-code c715a079ab57d4c9e7b56862f7f2584c792b74b0 107ed4e3ae0d43300a09dea10b2d43ad4423efc1 -- services/platform/startgate",
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit": 0,
    "stdout": "",
    "stderr": ""
  }
]
```
