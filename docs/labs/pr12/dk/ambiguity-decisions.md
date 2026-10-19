# Реестр локальных решений DK

Статус: приняты автором участка как кандидат для review, не решение внешнего рецензента. Источники и основание — requirements.md и requirement-change.md.

| ID | Неоднозначность | Решение | Причина / цена / проверка |
| --- | --- | --- | --- |
| DK-DEC-01 | Equality на сроке | Равенство означает expired | Без скрытого grace period; boundary tests ±1ns |
| DK-DEC-02 | Оба срока истекли | absolute_expired прежде idle_expired | Стабильная диагностика; decision-table cases |
| DK-DEC-03 | Проверять idle относительно created или активности? | Использовать authority-owned idle deadline, без новой lastActivity колонки | Не подменять renewal/SQL; тест renewed idle >created+8h при сроке <absolute |
| DK-DEC-04 | Время до/после Lookup? | После успешного Lookup | Исключить устаревшее now из-за ожидания; order spy + advancing clock test |
| DK-DEC-05 | Ошибка адаптера | unavailable с фиксированным reason, без исходной error string | Защита от canary/PII leak, цена — подробности остаются у future secure owner diagnostics |
| DK-DEC-06 | Неустановленный port | Constructor reject nil/typed-nil; zero service fail closed | Локальная reflect nil-проверка в application; constructor/zero-service tests |
| DK-DEC-07 | CLI denied считать error процесса? | Корректный denied=exit0; broken config=exit2 | Не путать отрицательное domain решение с malformed fixture; отдельные smoke fixtures |
| DK-DEC-08 | Можно ли назвать valid полномочием? | Только session_valid; никаких бизнес-грантов | Сохранение T/003; contract/reviewer checklist |
| DK-DEC-09 | Строгий JSON | Отклонять duplicate/extra/null required/trailing/oversize/depth/version | Малый явно ограниченный parser; negative fixtures/test matrix |

Ни один пункт не является эмпирическим подтверждением удобства сессий или результатом проведённого интервью. Непроверенные пользовательские вопросы остаются в interview.md.
