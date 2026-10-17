# HP-ADR-008 — Retention-safe persistence и ограниченное восстановление MVP

**Choice status:** Accepted — архитектурный выбор принят делегированным архитектором в настоящем поручении. **Source/baseline status:** In Review / Candidate. Это не personal Jira acceptance, не implementation evidence и не Development admission.

## 1. Контекст и вопрос

MVP должен сохранять принятые дела/конфигурации и possible-effect fence после controlled restart, одновременно удалять raw/held/context/history по различным срокам NFR009. Полные физические PostgreSQL backups/WAL archive вместе с обычными payload колонками сохраняли бы уже истёкший текст дольше его срока. Применение tombstone только при restore не удаляет исторические копии. Новая схема восстановления не должна создавать custom KMS/service только ради удобного PITR.

Драйверы: NFR001/009/014, RULE001/004/012/013; current MVP scope PILOT и Product controlled-fault allowance. Не обещается recover после уничтожения всех durable copies.

## 2. Принятый выбор и rationale

PostgreSQL содержит минимальную identity/config/safety/receipt/state и opaque content references/digests/expiry. Expiring payload хранится только в service-owned durable content volume, не PG/WAL. File fsync+atomic rename+directory fsync предшествуют SQL reference commit; expiry/read-time guard и safe orphan cleanup определены BP006. Ordinary raw/AI transcript persistence отсутствует. Provider credentials — отдельные encrypted connector files, не business backup.

MVP backup — selective logical safety/config/deletion projection с конечным TTL21d. Expiring content/secrets/drafts не копируются, full physical PITR/WAL archive/host-volume snapshots отсутствуют. Контролируемый RPO0 включает process kill/restart/temporary unavailable DB с сохранёнными исправными PG+content volumes. При потере/непроверенной актуальности volume восстановление минимального состояния сопровождается hold/Unknown и разрешённым reacquisition/reconnect, не ложной полнотой и не blind resend. RTO/availability Product измеряются на реальном выбранном fault contour, не объявлены достигнутыми.

Necessary accepted catalog/service label — ограниченная часть неизменяемого правила, сохраняется с accepted config; arbitrary name/intent/customer brief/contact/history — expiring content, не perpetual config escape hatch. Plaintext location/backup inventory точно разделены.

## 3. Альтернативы и компромиссы

| Альтернатива | Причина невыбора |
| --- | --- |
| Всё в PostgreSQL + обычный physical PITR21/30d | Старые backup/WAL содержат уже истёкшие payload; restore-only фильтр не исправляет retention |
| Per-record crypto-erasure и отдельный текущий key registry без historical key backups | Может обеспечить иной backup/recovery профиль, но требует отдельной целостной проверки key lifecycle/replication/deletion и увеличивает стартовый safety surface |
| Вообще без backups | Не даёт полезного ограниченного recovery минимальных identity/safety facts при иных инцидентах |
| Полный HA/multi-region + managed KMS сразу | Увеличивает число систем и стоимость до подтверждения первой операции; продукт не требует безграничного disaster RPO0 |

Выбранный вариант сознательно ограничивает disaster восстановление содержимого. Это не универсальная схема для будущего публичного SLA. Переход к иному профилю требует совместного нового ADR, storage/formal definitions и реальной проверки retention/recovery, без тихого включения snapshot backup.

## 4. Последствия и нормативные владельцы

Активные последствия: HP-ARC-009/010, HP-DATA-002–007, HP-BP-006, HP-BP-007/T/001/003/009/010 и owner SQL definitions. File/SQL commit gap требует explicit orphan/missing-content behavior; типизированный API обязан показывать expired/incomplete evidence, а не fake empty success. Queue delivery и case safety identity не зависят от восстановления пользовательского текста.

Это ссылки на canonical нормы, не их вторая редакция. Фактическое backup/restore/expiry evidence ещё отсутствует и должно появиться на реализации. Архитектурная достаточность нормы не доказывает уничтожение каждой копии.

## 5. Пересмотр и история

Первое издание, supersession отсутствует. Пересмотреть при выборе TEAM/PUBLIC topology, требовании wider storage-loss RPO, невозможности исключить host snapshots/retained payload copies, измеренном RTOFAIL или необходимости content backup. Условия внешнего storage provider и operational settings проверяются до live; incompatible context не допускается, а не исключается из политики молча.

## 6. Self-review и acceptance boundary

Проверены отсутствие competing data owner, explicit fault contour, отдельные сроки/classes, durable reference protocol и safe restore. Независимый review должен проверить точный BP/SQL/OAS и source projections; эта авторская запись не является independent PASS. Choice Accepted по делегации не делает phase Accepted.
