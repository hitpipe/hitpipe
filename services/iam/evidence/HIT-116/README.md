# HIT-116 · квалификация координатора отзыва

Код проверен на коммите `d13f723511b61cb3068bfdf63d40bd7100fa1e89` в отдельной Ubuntu VM с PostgreSQL 18.7,
Go 1.27.2 и временными сертификатами. Развёртывание рабочих сервисов не выполнялось.

Координатор принимает уже авторизованное намерение для SESSION или WORKSPACE.
Одна транзакция существующей схемы записывает запрет/новую эпоху, PENDING и точные
runtime/generation/boot обязательных участников. Только подтверждённое состояние
используется для доставки. Повтор после неопределённого COMMIT восстанавливает ту же
операцию и эпоху. Регистрирующийся процесс начинает с закрытым gate.

Полномочия проверяются в вызывающем слое IAM до передачи намерения: координатор
проверяет согласованность типа и области субъекта, не выдаёт прав и не принимает
намерения по сети. Интеграция с пользовательскими командами остаётся за их владельцами.

Отдельный IAM-only mTLS listener 9443 использует тот же startgate, что начало
защищённой операции процесса. Подтверждение проверяется по точному получателю,
субъекту, эпохе и факту завершения секций старта. Уже начавшаяся работа может завершиться.
При неполном составе нет effective_at. Это время, когда координатор увидел полный
набор уже записанных ACK. Если подтвердить отзыв за пять секунд не удалось, записывается
инцидент. Позднее чтение исходного своевременного ACK может завершить отзыв с сохранением инцидента.
Недоступность, lease expiry, смена boot или удаление из балансировщика не считаются доказательством fencing.

Подтверждённые группы новых интеграционных тестов:

- TestPostgresRevocationMultiProcess
- TestPostgresRevocationAtomicallyFreezesRegistryAndUpdatesSubject
- TestPostgresRevocationExactAckRecoveryAndIncident
- TestPostgresRevocationRejectedCommitAndCancellationLeaveNoPartialState
- TestPostgresRevocationUnknownCommitReplaysOnceWithoutAnotherEpoch
- TestPostgresRevocationNeverSilentlyAcceptsAnIncompleteRoster

Полные выводы: [четыре пакета HIT-116](revocation-full.log), [весь модуль и гонки](iam/postgres-race.log),
[протокол PostgreSQL](iam/verification.json),
[команды проверки Go](go.log), [прежние проверки startgate](platform/verification.json),
[окружение](runtime.log), [очистка](cleanup.log).

База: [PR HIT-115 №80](https://github.com/hitpipe/hitpipe/pull/80), точный коммит `c7a74f55ea8111fa290f803976d256cc8a4a202e`.
Проверены существующие таблицы HIT-81, текущее чтение HIT-84 и gate HIT-69.
Изменение не добавляет DDL, пользовательские маршруты, logout/BFF, поддержку
MEMBERSHIP/CONNECTION, автоматическое fencing или подключение рабочих сервисов.
