# HIT-146 — проверяемые входы

Processing 717c954fca4f06f165074a8688f0e94a31b398e7. База PR №90 49229e1db3f371a051a022ea9e31951dd558eecb. Интеграция через отдельную сборку IAM e1be0c1a0be577a3626a3e120707b3aeeb579c6f из Draft PR №92. Дерево IAM не переносилось в ветку Processing. Бинарники собрались из двух чистых рабочих копий на linux/arm64. Go overlay накладывает только тестовый main, привязанный к исходнику Processing. Закреплённый пакет startgate собирается из Git-дерева соответствующей версии своего сервиса.

| Зависимость | Проверенный SHA | Включение |
| --- | --- | --- |
| HIT-111, принятое ревью №14274 | b2e823012854575146595b01a6f56d5ac677fc44 | Предок исполняемого Processing |
| HIT-115, принятое ревью №15478 | 0ff7f039ccaf450d60a59cd9cc0504c13e032b08 | Предок проверенной сборки IAM |
| HIT-116, принятое ревью №16116 | d13f723511b61cb3068bfdf63d40bd7100fa1e89 | Предок проверенной сборки IAM |
| Исправление IAM, Draft PR №92 | e1be0c1a0be577a3626a3e120707b3aeeb579c6f | Точный HEAD чистой отдельной IAM-копии, проверенный сборщиком |

Полный diff нового инженерного коммита: source.patch (SHA-256 1ec5e53615b433d6621b0f59fbac0f913eabd604b052d31814ae3553b54e8379). [Полный diff от базы PR](https://github.com/hitpipe/hitpipe/compare/49229e1db3f371a051a022ea9e31951dd558eecb...717c954fca4f06f165074a8688f0e94a31b398e7).

Источники: [проверка файлов запуска](https://github.com/hitpipe/hitpipe/tree/717c954fca4f06f165074a8688f0e94a31b398e7/services/processing/internal/runtime/boot.go), [тесты](https://github.com/hitpipe/hitpipe/tree/717c954fca4f06f165074a8688f0e94a31b398e7/services/processing/internal/adapters/postgres/real_iam_process_test.go), [сборка реального IAM TEST-стенда](https://github.com/hitpipe/hitpipe/tree/717c954fca4f06f165074a8688f0e94a31b398e7/tools/hit146/iam-process.go), [инструкция IAM](https://github.com/hitpipe/hitpipe/blob/e1be0c1a0be577a3626a3e120707b3aeeb579c6f/services/iam/docs/HIT-146_RUNTIME_STARTUP_RU.md).

Git-проверки (UTC 2026-11-22T19:41:25.412949+00:00).

```json
[
  {
    "argv": [
      "git",
      "rev-parse",
      "HEAD"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "717c954fca4f06f165074a8688f0e94a31b398e7\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "status",
      "--porcelain"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "?? services/processing/docs/HIT-146/verification/iam-pr92/\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "b2e823012854575146595b01a6f56d5ac677fc44",
      "717c954fca4f06f165074a8688f0e94a31b398e7"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "49229e1db3f371a051a022ea9e31951dd558eecb",
      "717c954fca4f06f165074a8688f0e94a31b398e7"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "4ce06f30ef8cbd786d7024f0fe2ff81cc6dd8ff7",
      "717c954fca4f06f165074a8688f0e94a31b398e7"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--name-status",
      "4ce06f30ef8cbd786d7024f0fe2ff81cc6dd8ff7",
      "717c954fca4f06f165074a8688f0e94a31b398e7"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
    "exit_code": 0,
    "stdout": "M\tservices/processing/internal/adapters/postgres/real_iam_process_test.go\nA\tservices/processing/internal/runtime/boot.go\nA\tservices/processing/internal/runtime/boot_test.go\nM\tservices/processing/internal/runtime/config.go\nM\tservices/processing/internal/runtime/runtime.go\nM\tservices/processing/scripts/postgres/verify_real_iam.py\nM\ttools/hit146/iam-process.go\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "diff",
      "--check",
      "4ce06f30ef8cbd786d7024f0fe2ff81cc6dd8ff7",
      "717c954fca4f06f165074a8688f0e94a31b398e7"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-real-iam-revocation",
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
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92",
    "exit_code": 0,
    "stdout": "e1be0c1a0be577a3626a3e120707b3aeeb579c6f\n",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "status",
      "--porcelain"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "0ff7f039ccaf450d60a59cd9cc0504c13e032b08",
      "e1be0c1a0be577a3626a3e120707b3aeeb579c6f"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  },
  {
    "argv": [
      "git",
      "merge-base",
      "--is-ancestor",
      "d13f723511b61cb3068bfdf63d40bd7100fa1e89",
      "e1be0c1a0be577a3626a3e120707b3aeeb579c6f"
    ],
    "cwd": "/Users/tima/Documents/Codex/2026-10-18/files-pasted-by-the-user-hitpipe/work/hit146-iam-pr92",
    "exit_code": 0,
    "stdout": "",
    "stderr": ""
  }
]
```
