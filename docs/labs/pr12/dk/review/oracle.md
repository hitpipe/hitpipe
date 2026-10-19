# Авторский oracle для сверки после blind ответа

Не является ответом проверяющего. Источник — R2 DK-RULE-01/02/03, сформулированные до C1/C2.

| Ситуация | Outcome / reason | Бизнес-доступ |
| --- | --- | --- |
| 1 | valid / session_valid | Не предоставляется этой проверкой; нужна текущая отдельная авторизация |
| 2 | denied / idle_expired | Нет |
| 3 | denied / revoked_session | Нет |
| 4 | unavailable / clock_unavailable | Нет, error не fallback allow |
| 5 | denied / absolute_expired | Нет; absolute выше idle по приоритету |
| 6 | denied / unknown_session после пригодного Clock | Нет; прежний cache не разрешает |

Для acceptance clarity нужны настоящие имя/date/fullSHA/письменные ответы до oracle/сравнение/решения/retest. Этот файл их не заменяет.
