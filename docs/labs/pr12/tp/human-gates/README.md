# Пакет настоящих человеческих проверок

Актуальное техническое состояние 19.10.2026: инженерные PR №3–9/20/22 и bootstrap №2 merged, TP-REV-001 VERIFIED_FIXED в main ec0dcba7ca42ea59909489cef9650c6edff12f35. Принятие отчётного PR №10 ожидается. Источник HIT-12/10507; старый handoff TP-REV-001 ниже — исторический. Интервью NOT CONDUCTED / BLOCKED_NO_USER; реальные student peer checks, opposition, снимки, submission tags и защита остаются ожидающими.

HIT-31, Feature HIT-12, Тимофей Патрушев. PREPARED_FOR_HUMAN_ACTION, ни один gate не пройден агентом. Lead Дмитрий Карачебан организует контакты; автор и Codex не выступают независимым студентом. Никаких сообщений другим участникам этим пакетом не отправлено.

| Gate | Реальный исполнитель | Материал и следующее действие | Фактический статус |
|---|---|---|---|
| Интервью | Пользователь/представитель, согласованный преподавателем | interview.md: пользователь пока не найден; Lead сначала находит пользователя, затем организует разговор | NOT CONDUCTED / BLOCKED_NO_USER |
| Clarity входящий | Исполнитель HIT-11 | clarity.md и pinned requirements; дать свои ответы до сравнения с oracle автора | HUMAN_PENDING |
| Clean-room входящий | Исполнитель HIT-11, не готовивший scaffold | peer-scaffold-in.md; fresh environment и README | HUMAN_PENDING |
| Module review входящий | Исполнитель HIT-11 | peer-module-in.md, same consumer two readers | HUMAN_PENDING |
| Clean-room и module исходящие | Тимофей лично проверяет исполнителя HIT-13 | peer-scaffold-out.md / peer-module-out.md; Lead получает exact чужой артефакт | HUMAN_PENDING, чужой артефакт здесь не получен |
| External opposition | Студенты другого проекта по кольцу преподавателя | external-opposition.md; Lead получает назначение, участник разбирает чужой пакет и обсуждает свои требования | BLOCKED_HUMAN_ASSIGNMENT |
| Review/merge | Независимый разрешённый human reviewer/Lead | review-merge.md; рассмотреть конкретные PR, замечания и checks | HUMAN_PENDING |
| Снимки, личные данные, защита, теги | Участник/преподаватель/уполномоченный человек | Полные screenshot instructions + handoff; защита самим студентом без generative AI | HUMAN_PENDING |

Авторские local runs, CI и controlled mutation experiments подтверждают только собственный technical scope. Они не заменяют эти действия. Реальные замечания возвращаются в соответствующий PR, relevant checks повторяются, исходный протокол сохраняется. Feature/Epic закрывает Lead после полного DOD.

Уточнение 19.10.2026 по [HIT-26](https://hitpipe.atlassian.net/browse/HIT-26?focusedCommentId=10453) и [HIT-31](https://hitpipe.atlassian.net/browse/HIT-31?focusedCommentId=10455): Codex по поручению Lead проверил три финансовых сценария, расхождений трактовок не обнаружено. Это отдельная техническая запись в clarity.md, не личная слепая проверка другого студента. TP-REV-001 отдельно воспроизведён и исправлен в собственном [PR №20](https://github.com/hitpipe/hitpipe/pull/20); собственные retests не заменяют повторный verdict reviewer-а. Состояние и порядок интеграции — [новый handoff](../tp-rev-001-handoff.md). Документы/DOCX прежнего пакета не пересобирались; остановка перед пунктом 8 сохраняется.
