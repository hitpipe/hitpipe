# HIT-148. Стыки реестра и Processing

Договор Jira редакции 2, отпечаток `4f6de4914566687537272ebc38e95ab78c3297c0678319725a86d2585c92c7c5`. Исходники издателя HIT-154 проверены на `c715a079ab57d4c9e7b56862f7f2584c792b74b0`. HIT-154 принят, независимая проверка 4/4. [CCR](https://github.com/hitpipe/project-os/blob/0dc25affc7cb19b1a74a16d80b93eac0d9961c05/contracts/hitpipe/case-deletion-2026-11-22/CASE_DELETION_AUTHORITY_RU.md) · [Ревью HIT-154](https://hitpipe.atlassian.net/browse/HIT-154?focusedCommentId=16317).

| Стык | Что проверено в закреплённом источнике | Ограничение для HIT-148 |
| --- | --- | --- |
| Получение решения | Выделенный TLS 1.3, CA, SAN, привязка DER и список workspace. Подписанный полный снимок связывает TEST, издателя, workspace, поколение и ревизию. Processing проверяет эти поля и минимум записи. | Чтение снимка доступно. Оно сообщает текущую ревизию на момент ответа. [Маршруты издателя](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/internal/adapters/http/casedeletion/server.go#L230). |
| Выбор точных версий и устойчивая квитанция | `TargetAuthority.WithExactTargets` удерживает проверку принадлежности в процессе издателя. `ApplicationAuthority.WithApplied` вызывается перед записью `APPLIED` и обязана подтвердить эффект в подчинённом хранилище. Единственные реализации этих портов в проверенной версии находятся в тестовых файлах. Интеграционный тест HIT-154 проверяет свою синтетическую копию. | Для настоящих файлов/SQL Processing нужен доверенный адаптер к инвентарю и устойчивому результату Processing. mTLS и SHA квитанции сами этот результат не подтверждают. [Проверка квитанции](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/internal/application/casedeletion/registry.go#L423) · [Синтетическая реализация портов](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/internal/adapters/composition/casedeletion/fixture_integration_test.go#L218). |
| Открытие после восстановления | `WithCurrent` сравнивает снимок с живым издателем под его блокировкой, требует `APPLIED` или `CANCELLED` для всех решений и синхронно вызывает открытие. Этот метод локальный для издателя. Удалённый контур предоставляет только GET снимка и POST квитанции. | Без доверенной композиции координатора с реальным открытием Processing нельзя показать атомарное открытие между независимыми процессами. Повторный GET оставляет гонку между чтением и открытием. Общий каталог издателя нарушает его границу доступа. [Барьер](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/internal/application/casedeletion/snapshot.go#L101) · [Инструкция HIT-154](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/docs/HIT-154.md#L85). |

Предложение для владельцев интеграции. Согласовать композицию доверенного локального координатора HIT-154 вокруг фактического допуска восстановленного Processing. Провести read-only проверку версий по RLS и ограниченному файловому барьеру Processing. Адаптер квитанции получает только минимальное подтверждение эффекта, связывающее ID, хеш неизменяемой области, версию, поколение и исходные метаданные. Координатор удерживает `WithCurrent` до подтверждённого локального открытия. Необходимы доказательства отказа при потерянном ответе, смене поколения и публикации между сверкой и открытием. Адаптация относится к закрытому обслуживающему контуру и не требует изменения публичного API, SQL или ролей.

Дальнейшая проверка Processing должна зафиксировать точный идентификатор версии в инвентаре, совпадение с принадлежащим делу файловым ref, проверку остальных живых ссылок перед unlink, fsync и устойчивое свидетельство повторного применения. Для истории нужны исходный `expires_at` и известное по точному ref исключение для тела. Произвольная потеря или повреждение живой детали остаются `READ_UNAVAILABLE`. Удаление или подмена строк `safety_history`, термина хранения и финансового fence не требуются.

## Подтверждение версий и исходников

Processing: `0eba954c24652655cfc661fc63628b2097ebd33e`. База PR №86: `8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3`. IAM собран отдельно.

| Задача | Проверенный SHA | Включение |
| --- | --- | --- |
| HIT-126, принята | `4c2f1df34f7cf72ddb2de1f6888e033d035913ac` | Точный предок Processing |
| HIT-128, принята | `c426dc400a4bec03fafba72e2a5780c9d85af1e2` | Точный предок Processing, пустой diff исполняемого кода до базы PR №86 |
| HIT-154, принята | `c715a079ab57d4c9e7b56862f7f2584c792b74b0` | Чистая отдельная сборка, точный предок head PR №89 `fde4ad66cf736bd4ab67af926ae98f4de8eb5f69`. Изменения после него касаются материала результата, diff пакетов и команд IAM пуст |

[Полный diff от базы](https://github.com/hitpipe/hitpipe/compare/8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3...0eba954c24652655cfc661fc63628b2097ebd33e) · [Исходник клиента](https://github.com/hitpipe/hitpipe/blob/0eba954c24652655cfc661fc63628b2097ebd33e/services/processing/internal/adapters/deletionregistry/read.go) · [Определение теста](https://github.com/hitpipe/hitpipe/blob/0eba954c24652655cfc661fc63628b2097ebd33e/tools/hit148/inventory-probe.go). SHA-256 приложенного diff `8112f7eb96c0c2da1b833693e42f9d8881f8a8a163718858cd7c09149159edda`.

Команды Git, UTC 2026-11-22T19:53:24.078637+00:00. В последнем grep exit 1 означает отсутствие реализаций вне тестов.

```json
[
  {
    "argv": [
      "git",
      "rev-parse",
      "HEAD"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "0eba954c24652655cfc661fc63628b2097ebd33e\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "4c2f1df34f7cf72ddb2de1f6888e033d035913ac",
      "0eba954c24652655cfc661fc63628b2097ebd33e"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "c426dc400a4bec03fafba72e2a5780c9d85af1e2",
      "0eba954c24652655cfc661fc63628b2097ebd33e"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3",
      "0eba954c24652655cfc661fc63628b2097ebd33e"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--exit-code",
      "c426dc400a4bec03fafba72e2a5780c9d85af1e2",
      "8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3",
      "--",
      "services/processing/internal",
      "services/processing/cmd",
      "services/processing/scripts"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--exit-code",
      "8e679371504de0f1aa4a2cbb3ef6e5be2783c9e3",
      "0eba954c24652655cfc661fc63628b2097ebd33e",
      "--",
      "services/processing/scripts/migrations",
      "services/processing/tools/drafts",
      "services/processing/internal/runtime",
      "services/processing/internal/application"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--name-status",
      "87dafd1a171374fafe35802223288292b36dd429",
      "0eba954c24652655cfc661fc63628b2097ebd33e"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "A\tservices/processing/cmd/processing-purge/deletion_inventory.go\nM\tservices/processing/cmd/processing-purge/main.go\nA\tservices/processing/internal/adapters/deletionregistry/read.go\nA\tservices/processing/scripts/recovery/verify_deletion_inventory.py\nA\ttools/hit148/inventory-probe.go\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--check",
      "87dafd1a171374fafe35802223288292b36dd429",
      "0eba954c24652655cfc661fc63628b2097ebd33e"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-case-content-deletion",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "rev-parse",
      "HEAD"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 0,
    "stdout": "c715a079ab57d4c9e7b56862f7f2584c792b74b0\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "status",
      "--porcelain"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
      "fde4ad66cf736bd4ab67af926ae98f4de8eb5f69"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--exit-code",
      "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
      "fde4ad66cf736bd4ab67af926ae98f4de8eb5f69",
      "--",
      "services/iam/internal",
      "services/iam/cmd"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "grep",
      "-n",
      "-E",
      "WithApplied|WithExactTargets|\\.WithCurrent\\(",
      "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
      "--",
      "services/iam/internal",
      ":!*test.go"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 0,
    "stdout": "c715a079ab57d4c9e7b56862f7f2584c792b74b0:services/iam/internal/application/casedeletion/model.go:131:\tWithExactTargets(context.Context, Scope, func(context.Context) error) error\nc715a079ab57d4c9e7b56862f7f2584c792b74b0:services/iam/internal/application/casedeletion/registry.go:20:\tWithApplied(context.Context, Receipt, func(context.Context) error) error\nc715a079ab57d4c9e7b56862f7f2584c792b74b0:services/iam/internal/application/casedeletion/registry.go:214:\t\t\treturn r.targets.WithExactTargets(ownerCtx, scope, func(targetCtx context.Context) error {\nc715a079ab57d4c9e7b56862f7f2584c792b74b0:services/iam/internal/application/casedeletion/registry.go:283:\t\t\treturn r.targets.WithExactTargets(ownerCtx, scope, func(targetCtx context.Context) error {\nc715a079ab57d4c9e7b56862f7f2584c792b74b0:services/iam/internal/application/casedeletion/registry.go:457:\treturn r.applications.WithApplied(ctx, receipt, func(proofCtx context.Context) error {\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "grep",
      "-n",
      "-E",
      "func .*WithApplied|func .*WithExactTargets",
      "c715a079ab57d4c9e7b56862f7f2584c792b74b0",
      "--",
      "services/iam/internal",
      ":!*test.go"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit148-iam-pr89",
    "exit_code": 1,
    "stdout": "",
    "stderr": ""
  }
]
```
