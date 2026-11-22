# HIT-145. Контейнер Processing

Codex по делегации Тимофея Патрушева (`Tim1hik`). [Draft PR №84](https://github.com/hitpipe/hitpipe/pull/84) · [HIT-145](https://hitpipe.atlassian.net/browse/HIT-145)

На исходнике [30385d0](https://github.com/hitpipe/hitpipe/commit/30385d09962c3fa4aad4808253a5493235559d01) два контейнерных прогона завершились с кодом 0. Платформа `linux/arm64`, PostgreSQL 18.7, digest манифеста `sha256:972262cf9f8c50dd5a1d9009999b8d6b0177d1df23b5be555e2ccdd4df2f0a33`.

| Критерий | Результат |
| --- | --- |
| Закреплённые основания и идентичность | Go 1.27.2, PostgreSQL 18.7 (server 180007), проверены манифест, config, слой и бинарник |
| Изоляция и внешние входы | UID 10001, read-only root, отдельный RW content volume, конфигурация и Unix-сокет ro. В слое только бинарник |
| Допуск и рестарт | OPS Health CLOSED → READY в каждом процессе. Бизнес-вызов в CLOSED 503, чужому техническому сертификату OPS Health 403. После рестарта сохранены квитанция и история. Delivery получил ровно один запрос |
| Новая копия | Отдельный чистый worktree, отдельная сборка и загрузка из сохранённого архива. Digest манифеста и хеш бинарника совпали |

[Воспроизведение и внешние входы](RUN_RU.md) · [Протокол двух запусков](verification/test-evidence.md) · [Границы diff и зависимость](verification/source-evidence.md) · [Оригинальные журналы](verification/original-runs.tar.gz) · [Манифест оригиналов](verification/original-runs-manifest.json)
