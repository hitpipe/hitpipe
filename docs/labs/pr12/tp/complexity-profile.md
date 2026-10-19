# Направления системной сложности

Тимофей Патрушев, HIT-28 / HIT-30. Кандидат уже существует: services/processing/internal/domain/eligibility, Evaluate; фиксированный input testdata/valid.json и explicit now; отдельно наблюдаем через processing-contractcheck, без всего продукта.

| Направление | Собственный объект и вопрос | Готовность |
|---|---|---|
| Хранение и обработка данных | Nullable финансовые values и exact integer comparison; как representation сохраняет null/false/zero и exact boundary 2^53−1? Какова будущая стоимость projection/copy? | Семантика исполняется; production data/persistence и performance measurements ещё не выполнены |
| Межкомпонентное взаимодействие | SourceFactsReader и bounded snapshot; как отличить недоступность от отрицательного факта и сохранить oldest freshness при частичном чтении? | DI/error/boundary tests; authenticated Intake/provenance/digest/live latency остаются будущим scope |

Профиль не называет Go библиотеку отдельным направлением и не объявляет измеренное узкое место. Текущая проверка correctness не доказывает throughput, cost или Product NFR всего сценария. Личный ADR module-adr.md фиксирует варианты и цену; общий stack-decision переиспользуется по ссылке.
