# Человеческий review и merge

Состояние проверено 19.10.2026 в GitHub и по [HIT-31, комментарий 10455](https://hitpipe.atlassian.net/browse/HIT-31?focusedCommentId=10455). **HUMAN_PENDING** относится к настоящим human gates/merge, а не к отсутствию технических reviews.

Техническое review выполнил **Codex по поручению Lead Дмитрия Карачебана**. Это не личная слепая проверка другого студента. По трём проверенным финансовым сценариям расхождений трактовок не обнаружено; точные результаты и исходный SHA зафиксированы в [clarity.md](clarity.md) по [HIT-26](https://hitpipe.atlassian.net/browse/HIT-26?focusedCommentId=10453).

| PR | Фактическое состояние до интеграции исправления |
|---|---|
| [№2 bootstrap](https://github.com/hitpipe/hitpipe/pull/2) | OPEN, REVIEW_REQUIRED; native approval другого участника отсутствует |
| [№3 / HIT-25](https://github.com/hitpipe/hitpipe/pull/3) | OPEN, техническое APPROVED; интервью этим не проведено |
| [№4 / HIT-26](https://github.com/hitpipe/hitpipe/pull/4) | OPEN, техническое APPROVED; student blind gate этим не пройден |
| [№5 / HIT-27](https://github.com/hitpipe/hitpipe/pull/5) | OPEN, CHANGES_REQUESTED, TP-REV-001 |
| [№6 / HIT-28](https://github.com/hitpipe/hitpipe/pull/6), [№7 / HIT-29](https://github.com/hitpipe/hitpipe/pull/7), [№8 / HIT-30](https://github.com/hitpipe/hitpipe/pull/8), [№9 / HIT-31](https://github.com/hitpipe/hitpipe/pull/9) | OPEN, техническое APPROVED; зависимая цепочка, не merged |
| [№10 / HIT-32](https://github.com/hitpipe/hitpipe/pull/10) | OPEN, approval не предоставлен; отчёты не пересобраны |
| [№11 reviewer-а](https://github.com/hitpipe/hitpipe/pull/11) | CLOSED WITHOUT MERGE; не засчитывается как авторское исправление |
| [№20 авторский fix](https://github.com/hitpipe/hitpipe/pull/20) | OPEN к work/HIT-27-paid-order-eligibility; AWAITING_REVIEW; local/CI PASS не означают принятие замечания |

## TP-REV-001: собственные фактические статусы

Источник — [Request changes на исходном SHA PR №5](https://github.com/hitpipe/hitpipe/pull/5#pullrequestreview-5564750022), [HIT-27, комментарий 10454](https://hitpipe.atlassian.net/browse/HIT-27?focusedCommentId=10454). Область: офлайн-парсер синтетических fixtures. Production vulnerability не доказана и не заявляется.

* **REPRODUCED:** чистый исходный C1 `69b26f496c0e3bf4c837d49252203b70157101f1`: полный вход с refund=1 и следующим uppercase refund=0 даёт VALID, exit 0, вместо отказа. Новый regression patch на исходном parser даёт exit 1.
* **FIX_IMPLEMENTED:** собственный code commit `55fba39a6e50b05f27f37960e655f5bb9bae94b3`: точные canonical names проверяются до decode; tests top-level/nested/both orders/known zero и прежних строгих отказов. Исходники/patch PR №11 не использованы.
* **LOCAL_RETEST_PASS:** Go 1.27.2, четыре обязательные root-команды и отдельный race exit 0 на code SHA; исходный отрицательный вход и обратный порядок теперь exit 2 / INVALID_JSON без eligibility output. PR №20 head `2ee224fb3ff7828f370e30c4a47dfd21123f82c8` добавляет только новые журналы к проверенному коду.
* **CI_PASS:** [push run 39220471355](https://github.com/hitpipe/hitpipe/actions/runs/39220471355), [PR run 39220532283](https://github.com/hitpipe/hitpipe/actions/runs/39220532283), head `2ee224fb3ff7828f370e30c4a47dfd21123f82c8`, оба success. C1 workflow исполняет check/smoke; race C1 выполнен отдельно локально.
* **STACK_EXPERIMENT_PASS:** отдельная ветка от полного PR №10 `adfbfc33bf20cb0b0641e49e48c0502d2f5d108d` + собственный code commit = `8e3be4b638958072b1e8d57ab0317df170f3d017`. Все пять local checks exit 0; повторный mutation experiment 6/6 compiled/KILLED, negative gate exit 1, restoration exit 0, clean=true. [CI 39220298854](https://github.com/hitpipe/hitpipe/actions/runs/39220298854) success, включает race и mutations. Это экспериментальная интеграция, не merge PR №5–10.
* **AWAITING_REVIEW / NOT_MERGED:** reviewer ещё должен независимо проверить fix и дать verdict. Самопроверка автора не закрывает TP-REV-001.

[Exact commands, stdout/stderr, exit codes, время, toolchain, SHA и digests](https://github.com/hitpipe/hitpipe/tree/2ee224fb3ff7828f370e30c4a47dfd21123f82c8/docs/labs/pr12/tp/evidence/tp-rev-001). Старые журналы не переписаны. Новые readbacks CI опубликованы вместе с этим протоколом.

Lead организует разрешённого reviewer для повторной проверки PR №20 и TP-REV-001. Проверить исходное воспроизведение, canonical keys, both orders, typed numeric guards и новые journals. После действительного approval/current CI интегрировать PR №20 в ветку PR №5 **до принятия C1**. Далее последовательно обновить parent→child ветки PR №6→7→8→9→10 обычными merge без переписывания опубликованной истории, разрешить реальные конфликты, проверить каждый новый SHA и CI. Поправки протоколов из отдельного PR к ветке HIT-31 также передать в PR №10. Нынешние approvals/CI относятся к прежним SHA и не заменяют проверки после реальной интеграции. Агент merge не выполнял.

Source contract acceptance, formal Development admission и official /review backend не выполнялись. Для immutable lab tags требуются actual merge и проверенный финальный SHA; запись tag→SHA→CI→report digests в внешней submission record после tag, без собственного SHA в report commit. Теги не создавать/не переносить до этих условий. Защита/оценка/подписи преподавателя человеком, здесь не заполнены.

**STOP BEFORE 8**: текущая задача заканчивается подготовкой повторного review. DOCX, снимки, теги, закрытие задач/Feature/Epic не выполняются. Интервью NOT CONDUCTED / BLOCKED_NO_USER; личные взаимные проверки и opposition HUMAN_PENDING/BLOCKED сохраняются.
