# Проверка ясности требований

Независимая личная слепая проверка другого студента: **HUMAN_PENDING**. Назначенный проверяющий: реальный исполнитель HIT-11. Выдать requirements.md и requirement-change.md на exact reviewed SHA из handoff, без устных пояснений автора. Не выдавать test source/output и приведённое ниже сопоставление как готовый ответ. Настоящие самостоятельные ответы другого студента не получены; author self-review и делегированная проверка Codex не считаются этим gate.

Самостоятельно назвать ожидаемые status/reason и основание нормы для трёх входов:
1. Все обязательные финансовые признаки выполнены; proof freshest +5s, refund0, disputed=false; account/mode совпадают.
2. Все признаки как в первом, но amount_refunded=null и proof истёк; account верный.
3. Session created ровно activation, quantity1, complete proof, now равен valid_until.

Записать настоящий full SHA, дату, ФИО/роль, самостоятельный ответ и clause requirement для каждого. Затем автор сопоставляет с oracle requirements/ambiguity-decisions, сохраняет реальные расхождения и принятую правку с commit/PR/retest. Пока этих действий нет, PASS независимого студенческого gate не заявляется.

## Фактическая делегированная техническая проверка

Основание: [HIT-26, комментарий 10453 от 18.10.2026](https://hitpipe.atlassian.net/browse/HIT-26?focusedCommentId=10453), [HIT-31, комментарий 10455](https://hitpipe.atlassian.net/browse/HIT-31?focusedCommentId=10455). Проверку выполнил **Codex по поручению Lead Дмитрия Карачебана; это не личная слепая проверка другого студента**. Результаты внесены 19.10.2026 по действительному комментарию, новый разговор или новый запуск на старом SHA не заявляется.

Проверенный полный submitted stack: `adfbfc33bf20cb0b0641e49e48c0502d2f5d108d`.

| Сценарий | Фактический результат CLI | Сопоставление |
|---|---|---|
| Полные подходящие данные, refund=0 | VALID / FINANCIAL_ELIGIBILITY_ONLY | Совпадает с требованиями |
| amount_refunded=null и истёкший proof | INCOMPLETE / UNKNOWN_FACT | Unknown имеет зафиксированный приоритет |
| session_created_at=activation_at; now=valid_until | REFRESH_REQUIRED / PROOF_EXPIRED | Equality activation допустимо; equality expiry требует refresh |

**По трём проверенным финансовым сценариям расхождений трактовок не обнаружено.** Дополнительное изменение финансовых правил по этим случаям не требуется. [Техническое одобрение PR №4](https://github.com/hitpipe/hitpipe/pull/4#pullrequestreview-5564753576) относится к уточнению требований.

Отдельно обнаружен **TP-REV-001** в офлайн-парсере synthetic fixtures: case aliases могли перекрывать refund. Это не финансовое расхождение и не основание писать «замечаний вообще нет». [Исходное Request changes PR №5](https://github.com/hitpipe/hitpipe/pull/5#pullrequestreview-5564750022); [авторское исправление PR №20](https://github.com/hitpipe/hitpipe/pull/20). Воспроизведение, собственный fix и local retest выполнены; независимое повторное review/принятие замечания **AWAITING_REVIEW**, merge не выполнен. Подробности в [review-merge.md](review-merge.md) и [новом handoff](../tp-rev-001-handoff.md).
