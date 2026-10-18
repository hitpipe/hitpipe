# Независимая проверка ясности требований

Статус HUMAN_PENDING. Назначенный проверяющий: реальный исполнитель HIT-11. Выдать requirements.md и requirement-change.md на exact reviewed SHA из handoff, без устных пояснений автора. Не выдавать test source/output как готовый ответ. Ответов пока нет; author self-review не считается этой проверкой.

Самостоятельно назвать ожидаемые status/reason и основание нормы для трёх входов:
1. Все обязательные финансовые признаки выполнены; proof freshest +5s, refund0, disputed=false; account/mode совпадают.
2. Все признаки как в первом, но amount_refunded=null и proof истёк; account верный.
3. Session created ровно activation, quantity1, complete proof, now равен valid_until.

Записать настоящий full SHA, дату, ФИО/роль, самостоятельный ответ и clause requirement для каждого. Затем автор сопоставляет с oracle requirements/ambiguity-decisions, сохраняет реальные расхождения и принятую правку с commit/PR/retest. Пока этих действий нет, совпадение ответов и PASS не заявляются.
