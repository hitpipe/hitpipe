# Личная подготовка Дмитрия к защите

Это памятка для самостоятельного изучения; защита студентом ещё не проводилась и должна проходить без генеративного ИИ. Код/проверки подготовлены агентом по поручению, не следует приписывать себе фактическое независимое выполнение чужого review.

1. Объяснить место IAM в Operations: session_valid — предварительная проверка, отдельные current role/workspace/epoch/start gates обязательны. Почему неизвестность/ошибка не разрешает действие.
2. Показать Evaluate и пример idle−1ns/equality/+1ns, absolute24h и precedence; объяснить, почему renewed idle нельзя сравнивать только с created+8h при отсутствии lastActivity.
3. Показать Service.Execute, два consumer-owned ports и чтение clock после lookup; что случится при cancelled context, typed-nil и ошибке store/time. Не объявлять time.Now attestation.
4. Переключить --lookup file/fixed в одном root, не менять consumer. Объяснить synthetic reference/frozen fixture clock и отличие от настоящего authority.
5. По actual imports нарисовать cmd→adapters/application→domain; различить source arrow и runtime callback. Назвать владельца/cost/проверку DK-ADR-001 и две complexity directions.
6. Построить dev/release, --version, positive/invalid fixture и exit0/2/3; показать binary вне checkout, ELF/symbols; не выводить throughput из размера.
7. Запустить названный boundary/DI test, объяснить oracle до actual output, coverage94.5% statements и реальные gaps; 100%core не production proof.
8. Прочитать M01 patch/build0/failing assertion/final-clean0; объяснить7seeded/6initially killed/1survived→7, новый mixed Decision test, natural bug1 vs seeded/hidden distinction.
9. Обосновать qualitative risk matrix и every-change security tests, перечислить deferred integration и human gates. Отличать prepared report/image placeholder от факта evidence/approval.
10. Перед сдачей лично выполнить интервью, peer/opposition задания, вставить настоящие снимки/реквизиты, пройти отдельный review/merge; только после создать immutable tags и записать exact final SHA/CI. Не представлять этот список как уже выполненные действия.
