# HIT-128. Контролируемое восстановление Processing

Codex по делегации Тимофея Патрушева (`Tim1hik`). [HIT-128](https://hitpipe.atlassian.net/browse/HIT-128) · [Draft PR №85](https://github.com/hitpipe/hitpipe/pull/85)

Исполняемый локальный сценарий переносит разрешённые SQL-метаданные между двумя отдельными кластерами PostgreSQL 18.7. Содержимое остаётся в исходном каталоге. Проверены холодный перезапуск, отказ без свежих авторитетных входов, повтор Intake и команды, удержание незавершённого outbox, сохранение fence и ровно один вызов create в управляемом Delivery. SIGKILL до SQL COMMIT оставил сироту. Очистка убрала её и истёкший файл, сохранив исходные сроки и минимальную историю.

Проверенный исходник [c426dc4](https://github.com/hitpipe/hitpipe/commit/c426dc400a4bec03fafba72e2a5780c9d85af1e2). Шесть процессных регрессий плюс Go 1.27.2 race/vet/build прошли. Протокол описывает реальный локальный стенд, а не рабочее восстановление, внешний end-to-end или эксплуатационный RTO.

[Инструкция](RUN_RU.md) · [Протокол по всем шести критериям](verification/test-evidence.md) · [Происхождение зависимостей](verification/source-evidence.md) · [Исходные журналы](verification/original-run.tar.gz) · [CI](verification/ci-evidence.md)
