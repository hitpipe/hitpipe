# Implementation decision LAB-STACK-001

Автор: агент Codex по прямому поручению Lead, 17.10.2026. Статус: действующая учебная реализационная конвенция в границах точной кандидатной архитектуры, НЕ новый HP-ADR и НЕ личный ADR каждого студента.

Контекст: legacy backend Go1.25 и frontend Vite/React18 не совпадают с frozen целевым cohort. Простое обновление legacy root lock одновременно с пятью лабораторными смешает несвязанные риски и потребует полной миграции.

Решение: четыре service-local Go modules services/{iam,intake,processing,delivery}, exact Go1.27.2 из upstream; UX pure TS feature с изолированным tooling/pr12/frontend exact Node24.22.0/npm11.19.1/TS5.9.3/React/Next cohort. Frozen lock copies неизменяемы; домен Go/TS без лишних framework dependencies. CLI диагностирует тот же production-rule модуль; порты принадлежат consumer, adapters переключаются в composition root. Имеющиеся Vite/legacy backend root manifests не обновляются этой подготовкой.

Альтернативы: (а) немедленно мигрировать legacy app — значительно больший незаданный scope; (б) одноразовые учебные программы вне продукта — не дают дальнейшего reuse; (в) подчинить новые модули устаревшим зависимостям — противоречит выбранной целевой архитектуре. Выбран (г) service-local bounded development с точным cohort.

Цена: будущая интеграция транспорта/реальной persistence/Next server остаётся отдельной работой; fixture adapters не доказывают production integrations. Нельзя объявлять целый service/продукт готовым. Нужны pinned SDK и реальная установка на машине студента/CI, source+fixture-only clone, safe diagnostics, реальные dev/release observations.

Проверка: wrappers и source hashes проверены отдельно; студент реально строит свой модуль и описывает все 7 позиций build/dependencies/tests/static/dynamic/profiling/CI со статусом работает/запланировано и командой. Dynamic/profile можно честно отнести к следующему этапу там, где методичка допускает; test/CI нельзя объявить выполненными без запуска. Личный module ADR каждого отражает его собственные варианты/стоимость/проверку, а не копию LAB-STACK-001.
