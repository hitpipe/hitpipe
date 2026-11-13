# Анализ фактического покрытия

Актуализация HIT-32, 19.10.2026. Принятый main `ec0dcba7ca42ea59909489cef9650c6edff12f35`, Go 1.27.2 darwin/arm64. Новые exact commands/cwd/env/time/exit/digests: evidence/report-refresh-20261019/accepted-main/ и accepted-main-extra/. Profile processing.cover получен реальным wrapper check; go tool cover -func завершился exit 0. Linux CI этого же main: https://github.com/hitpipe/hitpipe/actions/runs/39229206910 — отдельный автоматический прогон, не локальный Linux запуск автора.

Statement coverage, не branch coverage: domain/application — 100.0%; весь сервис — **91.2%**. CLI run — 93.5%; input.Read — 90.9%; StrictDecode — 100.0%; walk — 81.0%; sourcefacts.File.Read — 87.5%. Исторические 92.3% и walk 79.2% относились к версии до TP-REV-001; исходные evidence/c3/processing.cover и coverage.log не переписаны. Снижение итогового процента не означает провал regression tests: добавились ветви точного сопоставления имён; новый профиль хранится отдельно.

Непроверенные участки и решения:

- main/os.Exit — 0% unit instrumentation; оба реальных бинарника успешно запускались вне checkout, это отдельное smoke evidence.
- sourcefacts.File.Read: post-read cancellation не вызван детерминированно. Не вводить flaky sleep ради процента; при расширении adapter нужен injected IO control. Pre-cancel/read failure/normal paths проверены.
- input.walk: malformed EOF, неподдерживаемая object/array shape и ветви reflection для типов, отсутствующих в текущем Scenario, покрыты не полностью. Scope — bounded offline typed fixtures, а не общий production parser. TP-REV-001 закрыт содержательными регрессиями всех имён, top-level/nested, обоих порядков aliases и known zero/false. Высокое покрытие не заменяет проверку семантики.

Два естественных дефекта учитываются отдельно: старый CLI writer defect (HIT-29, evidence/natural-defect/) и TP-REV-001 parser aliases (HIT-27, PR №20, evidence/tp-rev-001/). PR №20 APPROVED после делегированного повторного review и фактически merged; fix присутствует в main. Canonical refund=1 даёт INELIGIBLE / REFUND, exit 0; оба порядка case aliases дают INVALID_JSON, exit 2, stdout пуст. Canonical refund=0/false допустимы.

На принятом main новый локальный mutation experiment: 6 seeded, 6 compiled, 6 KILLED, 0 survivors/invalid, restored exit 0, clean=true; negative gate exit 1. Summary: evidence/report-refresh-20261019/accepted-mutations/summary.json. Natural-defects=2 — ввод из отдельно зарегистрированных ошибок, не число, найденное mutation runner-ом. Исторический CI параметр natural-defects=1 сохранён без изменения общего workflow; актуальная отчётная трактовка — две реальные ошибки, шесть намеренных мутаций. Setup/helper errors дефектами продукта не считались.

100% domain/application statements не доказывают authenticity/provenance, remote truth, TLS/revoke, atomic cross-service facts или полный MVP. Hidden suite NOT_PROVIDED_NOT_RUN. Performance и частота дефектов не измерялись. Результаты Codex по поручению Lead не подменяют личную слепую проверку другого студента, интервью, opposition и защиту.
