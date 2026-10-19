# HIT-12 — handoff повторного review TP-REV-001

19.10.2026, Europe/Moscow. Тимофей Патрушев, Processing — Operations Core. MANUAL_LAB_PR12. Область текущего продолжения: собственное исправление TP-REV-001 и корректировка протоколов. Это дополнение к историческому handoff пакета HIT-32; старые отчёты и журналы не актуализировались задним числом.

## Изменения для reviewer-а

[Собственный PR №20](https://github.com/hitpipe/hitpipe/pull/20) направлен в **work/HIT-27-paid-order-eligibility**, текущую ветку [PR №5](https://github.com/hitpipe/hitpipe/pull/5). Исходный C1 SHA `69b26f496c0e3bf4c837d49252203b70157101f1`. Авторский code commit `55fba39a6e50b05f27f37960e655f5bb9bae94b3`; PR head `2ee224fb3ff7828f370e30c4a47dfd21123f82c8` добавляет evidence. PR №11 reviewer-а CLOSED WITHOUT MERGE, его решение не переносилось.

Причина: DisallowUnknownFields продолжает принимать case-folded struct field aliases; прежний token walk ловил только точные дубликаты. Новая проверка точных JSON-тегов каждого struct выполняется перед decode, блокируя aliases/shadowing на верхнем и вложенных уровнях. Финансовые predicates не менялись. Дефект относится к офлайн fixtures, production vulnerability не заявляется.

Статусы TP-REV-001: **REPRODUCED / FIX_IMPLEMENTED / LOCAL_RETEST_PASS / CI_PASS / AWAITING_REVIEW / NOT_MERGED**. Ни собственные tests, ни зелёный CI не являются принятием замечания reviewer-ом.

## Evidence

[Индекс воспроизведения и новых журналов](https://github.com/hitpipe/hitpipe/blob/2ee224fb3ff7828f370e30c4a47dfd21123f82c8/docs/labs/pr12/tp/evidence/tp-rev-001/README.md). Metadata sidecars содержат настоящие argv/cwd, exit codes, full SHA, dirty, UTC/МСК, версии и digests. Проверенные полные отрицательные fixtures находятся в baseline-verified/fixtures/; test-only patch и красный regression run сохранены отдельно. Первоначальная ошибка helper-а описана в evidence-correction.md, его неверные fixtures не используются как доказательство.

На исходном чистом SHA полная fixture refund=1 + uppercase refund=0 дала VALID / FINANCIAL_ELIGIBILITY_ONLY, exit 0 вместо ожидаемого отказа. Новые tests на старом parser: exit 1. На code SHA оба порядка конфликта отвергаются, CLI exit 2 / INVALID_JSON, stdout пуст; canonical refund=0 остаётся VALID, refund=1 INELIGIBLE / REFUND.

С Go **1.27.2** на чистом code SHA и отдельно на чистом интеграционном SHA:

| CWD | Реальная команда | Exit на обеих версиях |
|---|---|---|
| Корень | python3 tools/pr12/workspace.py validate | 0 |
| Корень | python3 tools/pr12/test_workspace.py | 0 |
| Корень | python3 tools/pr12/workspace.py check processing --require | 0 |
| Корень | python3 tools/pr12/workspace.py smoke processing --require | 0 |
| services/processing | go test -race -count=1 ./... | 0 |

[CI C1 push 39220471355](https://github.com/hitpipe/hitpipe/actions/runs/39220471355) и [CI PR 39220532283](https://github.com/hitpipe/hitpipe/actions/runs/39220532283): success на `2ee224fb3ff7828f370e30c4a47dfd21123f82c8`. PR snapshot фиксирует восемь SUCCESS checks (четыре component jobs для push/PR events); это не approval. C1 workflow ещё не содержит поздний mutation/race step, локальный race выполнен отдельно.

## Эксперимент с последующим стеком

Ветка `work/HIT-27-tp-rev-001-stack-check` создана от полного PR №10 `adfbfc33bf20cb0b0641e49e48c0502d2f5d108d`, в неё cherry-pick только собственного code commit. Итог: `8e3be4b638958072b1e8d57ab0317df170f3d017`. Конфликтов не было. Это эксперимент, **не фактическое слияние PR №5–10**.

После изменения parser/tests повторён доступный mutation experiment: 6 seeded, 6 compiled, 6 KILLED, 0 survivors, 0 invalid; negative wrapper gate exit 1; restored tests exit 0; clean=true, disposable worktree удалён runner-ом. Полные команды, patch/build/test/restoration logs находятся в stack-mutations/ и stack-mutation-run/ индекса. Natural-defects=2 относится к двум отдельно зарегистрированным дефектам (HIT-29 writer + HIT-27 parser), старые результаты и общий workflow не изменены. Hidden suite NOT_PROVIDED_NOT_RUN.

[CI полного эксперимента 39220298854](https://github.com/hitpipe/hitpipe/actions/runs/39220298854): success на `8e3be4b638958072b1e8d57ab0317df170f3d017`, включает race и mutations; CI artifact независимо подтверждает 6 compiled/KILLED, restoration green/clean. Его исторический параметр natural-defects=1 не является новым подсчётом найденных дефектов.

## Протоколы и границы

Этот отдельный малый documentation PR направлен в **work/HIT-31-human-review-package** (PR №9, исходный SHA `4d142cb5dee60ee6ee9fe8845abcfc5890db4cc4`), без переноса исправления кода и без пересборки отчётов.

* [interview.md](interview.md): «Реального пользователя на данном этапе опросить нельзя: пользователь пока не найден». NOT CONDUCTED / BLOCKED_NO_USER; ответы, участники и дата разговора отсутствуют.
* [clarity.md](human-gates/clarity.md): по трём проверенным финансовым сценариям расхождений трактовок не обнаружено; проверку выполнил Codex по поручению Lead, это не личная слепая проверка другого студента. Источник — HIT-26/10453, исходный проверенный SHA adfbfc3… .
* [review-merge.md](human-gates/review-merge.md): технические approvals PR №3/4/6/7/8/9 записаны отдельно от человеческих gates; PR №5 CHANGES_REQUESTED; TP-REV-001 имеет собственные reproduced/fixed/retest/awaiting-review статусы.

Актуальные Jira статусы прочитаны: HIT-12 «В процессе проверки», HIT-25/26/27/31 «В работе». Источники — комментарии 10452/10453/10454/10455 и Feature 10456; snapshot сохранён в evidence fix PR. Jira не переводилась, задачи/Feature/Epic не закрывались. Обычный Project OS не обходился; /start и /review не вызывались; Candidate source contracts не объявлены Accepted.

## Следующие действия reviewer-а и Lead

1. Reviewer независимо повторяет отрицательный пример и регрессии PR №20, проверяет решение и выставляет фактический verdict по TP-REV-001. До этого AWAITING_REVIEW сохраняется.
2. Lead после approval/current CI интегрирует PR №20 в work/HIT-27-paid-order-eligibility, прежде чем принимать C1 / PR №5. Автономный merge не выполнен.
3. Последовательно перенести обновлённый parent в ветки PR №6→7→8→9→10 обычными merge, сохраняя опубликованную историю. После каждого реального обновления проверить SHA, diff и CI; на обновлённом полном стеке повторить обязательные local checks. Нынешний эксперимент не заменяет такую проверку.
4. Принять отдельную поправку протоколов в ветку PR №9 и передать её в PR №10; проверить актуальность обеих веток/CI. Bootstrap PR №2 всё ещё требует native approval другого участника.
5. Lead находит реального пользователя и организует независимые студенческие проверки и opposition. Непроведённые проверки остаются HUMAN_PENDING/BLOCKED; фиктивные участники/ответы не добавляются.

**ОСТАНОВКА ПЕРЕД ПУНКТОМ 8.** DOCX не финализировать, снимки не делать, теги сдачи не создавать, задачи/Feature/Epic не закрывать. Настоящие merge, человеческая приёмка и защита остаются отдельными последующими действиями.
