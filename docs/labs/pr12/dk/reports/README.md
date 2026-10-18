# Четыре индивидуальных отчёта Дмитрия Карачебана

Текст подготовлен по актуальным четырём методичкам, фактическому IAM-коду и журналам. Статус **CONTENT_READY_IMAGES_PENDING / PREPARED_FOR_REVIEW**, окончательная сдача `SUBMISSION_READY=false` до реальных человеческих gates. Это заполненные отчёты, а не рекомендации их написать.

| Назначение | DOCX | Редактируемый исходник |
| --- | --- | --- |
| Основной 03.02 ПР1 — требования/регламент | [0302-pz01-dk.docx](0302-pz01-dk.docx) | [Markdown](0302-pz01-dk.md) |
| Основной 03.02 ПР2 — система проверок | [0302-pz02-dk.docx](0302-pz02-dk.docx) | [Markdown](0302-pz02-dk.md) |
| Отдельный 01.04 ПР1 — инструментальный контур | [0104-pz01-dk.docx](0104-pz01-dk.docx) | [Markdown](0104-pz01-dk.md) |
| Отдельный 01.04 ПР2 — модульная архитектура | [0104-pz02-dk.docx](0104-pz02-dk.docx) | [Markdown](0104-pz02-dk.md) |

В основных0302 есть явные support-выжимки0104; полные отдельные экспорты сохраняют все29результатов. Не утверждается согласие преподавателя сдавать только два файла.

[Результат машинной проверки](validation.json), [29критериев](../criteria-crosswalk.json), [21инструкция снимков](../evidence/screenshots/INSTRUCTIONS.md), [human gates](../human-gates.md), [handoff](../handoff.md). Изображений агент не создавал. Реквизиты/подпись/оценка/настоящие human responses не подставлены.

Для пересборки: отдельный Python venv с зависимостями [requirements.txt](requirements.txt), затем из root `python docs/labs/pr12/dk/reports/build_reports.py`. Менять Markdown/figures.json, а не общий template. Структурная проверка без dependencies: `python3 docs/labs/pr12/dk/reports/verify_package.py`. Сборка не является визуальным rendering: открыть DOCX в Word/LibreOffice, обновить TOC, проверить страницы после реальных рисунков.

Код-кандидат с полным CI evidence — `2cd7c032e8ede98c75181599b440e97657bff197`; локальный detailed test SHA `5b14ab92d1ca2a68f46fd3bda783b188549fbf5a` имеет идентичный Go source. Последний report package head после commit проверяется отдельно; фактический SHA/CI фиксируется в комментарии HIT-24 и внешнем record, чтобы не делать бесконечные self-SHA commits. Submission tags создаются после настоящего human review/merge, сейчас их нет.
