# Путь от исходников к исполнению

HIT-30, автор owner-записи Тимофей Патрушев при помощи Codex. Exact build observation source: 8ae5d67b9eb0f4ffc729a270c2ee14e1c052b0cb. База bootstrap Candidate; ни parent PR, ни source contract не объявлены merged/Accepted.

Git source + service-local go.mod → проверка exact Go1.27.2/imports → Go compiler со stdlib и собственными packages → linker с version/full commit/dirty/buildMode → Mach-O arm64 executable → macOS loader + system dylibs → flags → synthetic fixture adapter → EvaluatePaidOrder → eligibility.Evaluate → owned typed result → safe diagnostics/exit. Domain/application не загружают providers или чужие DB. Схема отражает именно существующий source dependency map (module-map.md).

| Наблюдение | dev | release |
|---|---|---|
| Команда из repo root | python3 tools/pr12/workspace.py build processing --mode dev --require | Та же с --mode release |
| Подтверждённые flags | -gcflags=all=-N -l; metadata linker flags | -trimpath; metadata linker flags, нет -N -l |
| Объём binary на данном SHA | 3948210 bytes | 3909730 bytes |
| Отладка | Оптимизации и inlining отключены; compiler default debug info, section audit c1/sections.log | Пути trimmed; debug info не удаляется флагами -s/-w (они не задавались); section audit d1/inspect-release.log |
| Версия | pr12, fullSHA8ae5…, dirty=false, buildmode=dev | Тот же full SHA, dirty=false, buildmode=release |
| Normal valid.json | VALID FINANCIAL_ELIGIBILITY_ONLY, exit0 | То же, exit0 |
| Изменение fixture на refunded.json | INELIGIBLE REFUND, exit0 | То же, exit0 |
| Неверная config invalid.json | INVALID_JSON, exit2 | То же, exit2 |
| Состав запуска | Бинарник и синтетический fixture | Бинарник и синтетический fixture |

Все восемь реальных запусков выполнены вне checkout из разных t.TemporaryDirectory; precise cwd/argv/log/digest/binary hash сохранены evidence/d1/delivery-observations.json. Go compiler/source не требовались во время этих запусков. Это самостоятельное авторское воспроизведение, НЕ независимая проверка другого студента.

Инспекция go version -m показывает module и flags; otool -L на macOS показал /usr/lib/libSystem.B.dylib и /usr/lib/libresolv.9.dylib. Следовательно, нельзя заявлять полностью статический бинарник или переносимость на Linux из этих наблюдений. Linux CI собирает собственный ELF. Проверки ввода явные, не отключаемые assert; оба режима реально дали те же business outcomes. Различие размера зафиксировано, измерения скорости не проводились.

Clean-room инструкция: services/processing/README.md. Сторонний студент получает exact reviewed SHA, ставит указанные prerequisites только по инструкции, запускает check/smoke/version/invalid, сохраняет свои env/commands/results/remarks и после правок повторяет из чистого состояния. Этот human gate ещё pending HIT-31.
