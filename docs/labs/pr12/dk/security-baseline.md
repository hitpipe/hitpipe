# Ранние ограничения безопасности SessionPolicy

HIT-17; основания HP-BP-001/T/002–003/013 и HP-BP-007/T/005. Пять вопросов МДК03.02 ПР1 §4.6 применены к собственному участку.

| Вопрос | Конкретный ответ | Проверка / граница |
| --- | --- | --- |
| Что защищаем? | Доступ к операциям, session validity, минимальный identity context. Чужая сессия не должна стать разрешением. Секрет cookie/digest и профиль не нужны чистой policy. | Unknown/revoked/expiry/clock tests; tenant authorization не доказана этим module |
| Откуда недоверенный ввод? | CLI arguments, локальный fixture JSON, будущие результаты authority adapter и данные времени. Записанный fixture не является production authority. | Строгий parser: version/type/unknown fields/duplicate keys/trailing data/size; chronology и trust проверяются отдельно |
| Кто имеет доступ к существенным действиям? | В целевом продукте только текущий субъект с подтверждённой областью/ролью/epoch у владельца объекта. SessionPolicy сообщает пригодность сессии, не выдаёт business grant. | Нет HTTP auth shortcut, provider writes, session issuance или внешнего side effect; consumer не трактует Unavailable как Valid |
| Каков реалистичный ущерб? | Повторное использование отозванной/истекшей сессии, несанкционированный просмотр или команда. Неверная диагностика может скрыть отказ и раскрыть токен. | Deadline equality, absolute cap, fail-closed, malformed chronology, error precedence; mutation campaign HIT-21 |
| Где секреты и что не логировать? | Реальные секреты вообще не используются в лабораторной. В целевой системе session digest только IAM DB; provider credentials у connector owner. Нельзя писать cookie, digest, user profile, fixture/body или путь с PII в stdout/stderr. | Canary marker на обычном/ошибочном пути; фиксированные reason/error codes; только синтетические fixture |

Доверие к времени — явный вход, а не утверждение `time.Now()` об исправности часов. Fixture adapter и фиксированное время проверяют правила, но не NTP, SQL/RLS/durability, OIDC, logout race или распределённый revoke. Публичный endpoint и живые внешние действия в этот scope не входят.

Набор покрывает локальные error boundaries. Это не полная threat model и не подтверждение требования продукта про 1000 отрицательных попыток или глобальный отзыв ≤5 секунд. Эти проверки требуют других компонентов и отдельного окружения.
