# HIT-154 · протокол квалификации реестра адресного удаления

Проверенный код: `c715a079ab57d4c9e7b56862f7f2584c792b74b0`. Изолированная Ubuntu VM, `go version go1.27.2 linux/amd64`,
PostgreSQL 18.7, временные сертификаты и два синтетических
пространства. Реальные пользователи и рабочая инфраструктура не участвовали.

Проверки зафиксировали раздельное хранение рабочего набора восстановления и
доверенного реестра издателя; проверку текущего владельца и точных версий;
подписанную полную выдачу и доступ Processing к чтению и узкому приёму квитанций.
В стенде подтверждение применения, повтор после сбоя и возврат старой копии
проверяются на синтетических подчинённых данных. Реальное удаление и обслуживание
содержимого Processing остаются за HIT-148.

Зафиксированные успешные группы PostgreSQL:

- TestPostgresCaseDeletionQualification
- TestPostgresCaseDeletionOnlyCurrentOwner
- TestPostgresCaseDeletionOwnerRemainsCurrentThroughRegistryCommit
- TestPostgresCaseDeletionRejectsWrongDatabaseAuthorityAndUntrustedTime

Материалы:
- [Полный вывод пяти пакетов HIT-154](case-deletion-full.log) и [исходный модульный лог](iam/postgres-race.log).
- [Команды, версии и результаты PostgreSQL](iam/verification.json), [Go race, vet, сборка](go.log), [регрессии startgate](platform/verification.json).
- [Окружение](runtime.log) и [очистка стенда](cleanup.log).
- [Инструкция, формат и барьер для HIT-148](https://github.com/hitpipe/hitpipe/blob/c715a079ab57d4c9e7b56862f7f2584c792b74b0/services/iam/docs/HIT-154.md).
- [Закреплённая поправка HP-CCR-CASE-DELETION-AUTHORITY-20261122](https://github.com/hitpipe/project-os/blob/0dc25affc7cb19b1a74a16d80b93eac0d9961c05/contracts/hitpipe/case-deletion-2026-11-22/CASE_DELETION_AUTHORITY_RU.md).
- [Принятый результат HIT-115](https://github.com/hitpipe/hitpipe/blob/c7a74f55ea8111fa290f803976d256cc8a4a202e/.project-os/results/HIT-115.json) в [PR #80](https://github.com/hitpipe/hitpipe/pull/80), база `c7a74f55ea8111fa290f803976d256cc8a4a202e`.
