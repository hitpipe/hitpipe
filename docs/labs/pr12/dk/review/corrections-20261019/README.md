# HIT-24 — исправления DK-REV-001/002 после ревью Тимофея

19.10.2026. Режим MANUAL_LAB_PR12. Исполнитель исправлений и проверки —
Codex в авторской сессии Дмитрия Карачебана. Это авторская проверка,
не повторное независимое заключение Тимофея и не личная учебная приёмка.

Исходный [вердикт Tim1hik](https://github.com/hitpipe/hitpipe/pull/19#issuecomment-6271215905):
код APPROVED_FOR_SCOPE, PR12–18 scoped APPROVE; отчёты CHANGES_REQUIRED,
PR19 REQUEST_CHANGES на `f67dd3faefb6a9e61dc1b3dbcbdfe158c5744fcd`.
[Точные ссылки и область ревью](original-review.json).

## Исправления и проверяемый результат

| Замечание | Исправление | Проверка |
| --- | --- | --- |
| DK-REV-001 | В 0104-pz02-02 и 0104-pz02-05 команды используют `go -C services/iam`; каталог явно остаётся корнем чистого clone. JSON, человеческая инструкция и DOCX генерируются из одного источника. | Старое `go doc ./internal/domain/sessionpolicy` из root воспроизведено с exit1; обе исправленные команды Go doc и TestSameConsumerContractWithBothAdapters из root — exit0. [argv/cwd/env/time/exits](commands.json), [проверка всех 21 инструкций](figure-audit.json). |
| DK-REV-002 | В генерируемых таблицах запрещены разрывы строк, заголовок повторяется, ширина колонок ограничена печатной областью. Убраны унаследованные от шаблона отступы внутри ячеек; шрифт таблиц сохранён 10pt. Титулы и исходные формы не менялись. | Реальный LibreOffice Writer 26.2.6.3 → PDF, Poppler 26.01.0 → PNG 100 DPI. Просмотрены все 69 страниц; после последнего исправления повторно просмотрены 25 изменившихся страниц, 44 остальные PNG совпали с уже просмотренной редакцией. Полные строки «Время работы» находятся на стр.3/3/4/4; таблица рисков на стр.7–8 повторяет заголовок. [Точные DOCX/PDF hashes и verdict](../../reports/visual-validation.json). |

Дополнительно реальные оглавления двух 0302 DOCX обновлены через Writer UNO
и сохранены в документах. Уточнено, что естественный JSON case-key дефект
относился к offline fixture-декодеру, а не к production-интеграции.
Исторические числа и журнал первого survivor M07 не переписаны.

| Отчёт | PDF для проверки вёрстки | Страницы |
| --- | --- | --- |
| 0104 ПР1 | [0104-pz01-dk.pdf](pdf/0104-pz01-dk.pdf) | 1–17 |
| 0104 ПР2 | [0104-pz02-dk.pdf](pdf/0104-pz02-dk.pdf) | 1–16 |
| 0302 ПР1 | [0302-pz01-dk.pdf](pdf/0302-pz01-dk.pdf) | 1–17 |
| 0302 ПР2 | [0302-pz02-dk.pdf](pdf/0302-pz02-dk.pdf) | 1–19 |

[Журнал настоящего рендера](render-run.json), [пакеты локального renderer](renderer-packages.json),
[команды PNG и хеши страниц](page-renders.json), [сохранность ячеек](table-preservation.json),
[сохранность исходников/истории](preservation.json),
[пересчитанный digest производной инструкции](derived-guide-revision.json).
PDF и PNG являются превью страниц для QA, не учебными снимками работающего приложения.
21 место для настоящих снимков сохранено, изображения в DOCX не вставлялись.

## Воспроизведение

Из корня clone с python-docx 1.2.0:

```sh
python3 docs/labs/pr12/dk/reports/build_reports.py
python3 docs/labs/pr12/dk/reports/render_reports.py --output /tmp/dk-layout
python3 docs/labs/pr12/dk/reports/verify_package.py
python3 tools/pr12/workspace.py validate
python3 tools/pr12/test_workspace.py
```

Renderer требует LibreOffice Writer, соответствующий Python UNO и Poppler.
На VPS использована изолированная распаковка официальных Ubuntu-пакетов
без установки системных пакетов и без root; фактические абсолютные argv,
окружение и времена сохранены в render-run.json. Стандартная установка
использует `soffice`, `pdfinfo`, `pdftotext` из PATH. После пересборки документы
получают новые hashes: нужно заново экспортировать PDF, просмотреть страницы
и заменить visual-validation.json реальным результатом. Старый визуальный
verdict нельзя переносить на новые файлы; verify_package проверяет hashes.

Семантика свойств строк и таблиц сверена с первичными источниками:
[OOXML row properties в python-docx](https://python-docx.readthedocs.io/en/latest/dev/analysis/features/table/table-row.html),
[table widths](https://python-docx.readthedocs.io/en/latest/dev/analysis/features/table/table-props.html),
[Writer document indexes](https://api.libreoffice.org/docs/idl/ref/interfacecom_1_1sun_1_1star_1_1text_1_1XDocumentIndexesSupplier.html).
Они не заменяют фактическую проверку опубликованных PDF.

## Интеграция и границы

Дмитрий прямо разрешил после исправлений слияние с bypass повторного approval,
без возвращения пакета Тимофею. Это решение владельца, а не выданный от имени
Tim1hik APPROVE исправленной редакции. Его исходное REQUEST_CHANGES сохраняется
в истории. Обязательные четыре CI-проверки не обходятся, защиты не отключаются.
PR19 интегрирует весь уже проверенный стек с сохранением исходных коммитов;
фактические head/merge SHA и результаты CI фиксируются в GitHub и HIT-24
после выполнения, без выдуманного будущего SHA внутри этого коммита.

Исходники IAM, тесты, fixtures, contract, frozen sources и шаблоны неизменны.
Личный опрос, blind/peer/clean-room процедуры студентов, внешнее оппонирование,
реквизиты, настоящие снимки и защита не объявляются выполненными агентом.
Сдача остаётся CONTENT_READY_IMAGES_PENDING / SUBMISSION_READY=false.
