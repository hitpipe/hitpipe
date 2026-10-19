# Замечания DK-REV

Codex по делегации Тимофея Патрушева; это агентное ревью. Проверенный PR19: `f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd`.

## DK-REV-001 [P2] Согласовать cwd и команды двух инструкций МДК01.04 ПР2

PR #19; SHA `f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd`; [docs/labs/pr12/dk/reports/figures.json:330](https://github.com/hitpipe/hitpipe/blob/f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd/docs/labs/pr12/dk/reports/figures.json#L330). Введено в PR19, не унаследованный дефект IAM.

**Наблюдение:** Для 0104-pz02-02 и 0104-pz02-05 cwd явно указывает корень clone, тогда как go doc ./internal/... и go test ./internal/... разрешаются относительно services/iam. Navigation указывает services/iam, то есть поля противоречат друг другу. При буквальном выполнении из указанного cwd обе команды завершаются exit 1: directory not found. Та же ошибка встроена в DOCX и INSTRUCTIONS.md.

**Ожидание/основание:** Один однозначный cwd и непосредственно исполняемая команда на source SHA. MANUAL_LAB_PR12 workflow §6/§10 и HIT-24 AC4 требуют точные исполнимые cwd+command, а не только непустые поля.

**Воспроизведение:** Из корня чистого review checkout: go doc ./internal/domain/sessionpolicy Из корня: go test ./internal/adapters/fixtures/sessionlookup -run TestSameConsumerContractWithBothAdapters -v -count=1

**Доказательства:** [logs/repro-godoc-root.json](logs/repro-godoc-root.json), [logs/repro-godoc-root.stderr](logs/repro-godoc-root.stderr), [logs/repro-contract-root.json](logs/repro-contract-root.json), [logs/repro-contract-root.stderr](logs/repro-contract-root.stderr), [logs/godoc-domain.json](logs/godoc-domain.json), [logs/adapter-contract.json](logs/adapter-contract.json)

**Исправление:** Для обеих записей явно задать services/iam и согласовать путь открытия module-contract.md; либо оставить root и использовать go -C services/iam doc/test. Синхронно обновить figures.json, INSTRUCTIONS.md и пересобранный 0104-pz02-dk.docx. Проверить все21 инструкцию на подобные противоречия.

**Повторная проверка:** В отдельном чистом clone на новом package head буквально выполнить исправленные команды в записанном cwd: exit0, package docs и 10 file/fixed subtests; verify_package.py PASS, ровно21 placeholder. Авторские исторические логи не переписывать.

**Статус:** OPEN; future author fix/retest NOT_PERFORMED

## DK-REV-002 [P2] Устранить разрыв строк и потерю контекста у продолжений таблиц

PR #19; SHA `f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd`; [docs/labs/pr12/dk/reports/build_reports.py:75](https://github.com/hitpipe/hitpipe/blob/f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd/docs/labs/pr12/dk/reports/build_reports.py#L75). Введено в PR19, не унаследованный дефект IAM.

**Наблюдение:** Во всех четырёх PDF строка «Время работы» первой содержательной таблицы разрывается: на следующую страницу уходит только слово JSON в правой ячейке с пустой левой ячейкой. 0104-pz01/02: страницы3→4; 0302-pz01/02:4→5. В 0302-pz02 таблица рисков продолжается на страницах9/10 без повторения шапки, с разрывами записей. Сборщик создаёт строки без cantSplit и tblHeader. ZIP/XML PASS этого не обнаруживает.

**Ожидание/основание:** Короткие строки не разрываются между страницами; многостраничные таблицы имеют повторяемую шапку, идентифицируемые продолжения и достаточную ширину колонок без потери содержания. Проверка реального отображения DOCX/таблиц/пагинации входит в HIT-24 AC5 и общий manual workflow. Замечание относится к полученным формам, не к неизменяемым upstream шаблонам.

**Воспроизведение:** Рендер каждого из четырёх DOCX штатным render_docx.py; открыть указанные страницы PDF. Exact argv/cwd/time/exit сохранены в logs/render-*.json.

**Доказательства:** [document-review.json](document-review.json), [docx-structure.json](docx-structure.json), [pdf/0104-pz01-dk.pdf](pdf/0104-pz01-dk.pdf), [pdf/0104-pz02-dk.pdf](pdf/0104-pz02-dk.pdf), [pdf/0302-pz01-dk.pdf](pdf/0302-pz01-dk.pdf), [pdf/0302-pz02-dk.pdf](pdf/0302-pz02-dk.pdf)

**Исправление:** Минимально исправить создание таблиц: запрет разрыва коротких строк, повторение шапки, ширины колонок/альбомная секция для широкого реестра при необходимости. Не удалять записи и не уменьшать их до нечитаемого кегля. Пересобрать четыре DOCX и проверить весь рендер.

**Повторная проверка:** verify_package.py PASS, таблицы/ячейки/полный текст/21 placeholder сохранены, повторный DOCX→PDF с новым SHA и фактическим просмотром всех страниц. Указанных оторванных фрагментов больше нет; продолжения имеют контекст.

**Статус:** OPEN; future author fix/retest NOT_PERFORMED
