# Решения по неоднозначности

Локальный кандидат HIT-26 от требований HIT-25 (035516461b5f45a37aa1129d6f11cf8bdb89f277; PR https://github.com/hitpipe/hitpipe/pull/3 открыт). Автор: Тимофей Патрушев при помощи Codex.

| Вопрос | Решение | Проверка |
|---|---|---|
| Ноль или unknown | Pointers для nullable mandatory facts; bool false/integer0 известны, nil unknown | TestUnknownAndZero |
| Несколько нарушений | Shape/version → source identity → completeness → freshness → business predicates | TestPrecedence |
| Равенство expiry | now>=valid_until требует refresh | TestProofBoundary |
| Последнее чтение или oldest | max freshness считается от oldest mandatory fact | TestProofBoundary |
| Ошибка dependency | INCOMPLETE, без повторного чтения и side effects | TestReaderSubstitution |
| Финансовый успех | Только financial eligibility; не dispatch permission | TestFinancialScope |

Blind package размещается в human-gates/clarity.md; версия требований выдаётся реальному студенту с тремя входами, без expected answers. Actual responses NOT_RECEIVED. Эта таблица — self-analysis автора, не независимая проверка. Human review/merge остаётся обязательным. Противоречий с доступным pinned source не обнаружено при self-review; это не acceptance upstream.
