# HIT-14: результат делегированного ревью Delivery

19.10.2026. Ответственный проверяющий со стороны команды — **Дмитрий Карачебан**.
Фактический анализ требований, архитектурной совместимости, кода, тестов и отчётов
выполнил **Codex по прямому поручению Lead**. Прежнее назначение Матвея Головачева
для командного ревью HIT-14 отменено: [текущее назначение](reviewer-assignment.json).
Отдельно проверять концепцию или код со стороны Lead не требуется.

**Вердикт: REVIEW COMPLETED / CHANGES REQUESTED. Работа не принята.**
Это результат проведённой проверки, а не ожидание, когда Lead начнёт ревью.
Обнаружены воспроизводимые дефекты и незавершённые агентные результаты. Нельзя
сводить остаток к снимкам экрана и человеческому подтверждению.

## Проверенный набор

Репозиторий `hitpipe/hitpipe`. Полный представленный стек проверен в отдельном
worktree на `0a6f2ef3a5cd6db30ff4cef5efbefd185fc47b00`. Авторские production-файлы
и тесты при проверке не изменялись; дополнительные тесты подключены через Go overlay.
Материалы Project OS/методичек проверены штатным валидатором frozen bundle.
Discovery/Product/Architecture остаются Candidate; это ревью не меняет их приёмку.

| Task | Представленный PR | Проверенный head | Итог |
| --- | --- | --- | --- |
| HIT-41 / R1 | опубликованного PR/ветки не найдено | заявленный `0fb84594b18d1f8855f38761331b2ce79d16a97e` не возвращается GitHub API | Нельзя проверить содержимое |
| HIT-42 / R2 | [№23](https://github.com/hitpipe/hitpipe/pull/23) | `2bff32f3841a039e59924c4c9f32e05f9bbdd0d1` | Требуется актуализация контракта и доступный R1 |
| HIT-43 / C1 | [№24](https://github.com/hitpipe/hitpipe/pull/24) | `a1e152c4076caa76e071ecac59ecc8ca69718105` | Дефекты сравнения и декодирования |
| HIT-44 / C2 | [№25](https://github.com/hitpipe/hitpipe/pull/25) | `8e51cbc91b0835a5d7b8a16f4c45706fc49df55b` | Привязка evidence, происхождение и подмена зависимостей |
| HIT-45 / C3 | [№26](https://github.com/hitpipe/hitpipe/pull/26) | `3d3db398b2d061f8e88aa3aff955dcf27338b7a7` | Серия мутаций не выполнена |
| HIT-46 / D1, пакет HIT-48 | [№27](https://github.com/hitpipe/hitpipe/pull/27) | `0a6f2ef3a5cd6db30ff4cef5efbefd185fc47b00` | Карта и отчёты требуют доработки |
| HIT-47 / H1 | отдельного PR/завершённых протоколов не найдено | см. комментарий автора в Jira | Командное ревью проведено; учебные действия не засвидетельствованы |

Недоступный R1 подтверждён не только отсутствием файла в последнем стеке:
GitHub `GET commits/0fb84594b18d1f8855f38761331b2ce79d16a97e` ответил HTTP 422
`No commit found for SHA`. Это не утверждение, что автор не делал локальную работу.
На момент ревью у `lasarc` уже **write** в основном репозитории; прежний HTTP403
из комментария HIT-41 не подтверждает текущий блокер публикации.

## Фактические проверки

Linux amd64, Go 1.27.2; исходный полный SHA указан выше. Точные argv, время,
проверенные hashes, exits и SHA-256 журналов: [evidence/review-checks.json](evidence/review-checks.json).

| Проверка | Результат и граница |
| --- | --- |
| Frozen bundle validation | PASS; не оценка полноты личной работы |
| 17 тестов bootstrap wrapper | PASS; не личный набор студента |
| `workspace.py check delivery --require` | PASS: vet, фактически выполненные именованные тесты, coverage |
| `workspace.py smoke delivery --require` | PASS: dev/release, версия, вынесенный binary, заявленные fixtures и invalid format |
| `go test -race -count=1 ./...` | PASS на Linux |
| Statement coverage всей Delivery | **55.3%**, CLI 0%, FileReader 64.3%; это наблюдение, не новый порог приёмки |
| 9 независимых проверок контракта | **9 воспроизводимых FAIL**, тесты компилируются; сборочных ошибок нет |
| Авторские ≥5 мутаций | **NOT RUN**, прямо указано в авторском пакете; reviewer probes не подменяют этот опыт |

На исходном head №27 все четыре обязательных CI-контекста успешны. Это подтверждает
прохождение имеющегося набора, но не устраняет девять FAIL независимых проверок.
У №23 имеются неуспешные delivery checks; перед интеграцией потребуется успешный
актуальный head каждого принимаемого PR. Не переносить результаты CI между SHA.

## Замечания и условия исправления

### DS-REV-001 · P1 · Неправильный handoff-протокол

`internal/domain/readback/readback.go:21–22,116–123` принимает HTML-комментарии
`<!-- hitpipe:handoff:start -->`, которых нет в `HP-BP-004/T/013`.
Канонический `[HITPIPE-HANDOFF:<case-UUID>:v1] ... [/HITPIPE-HANDOFF:<case-UUID>:v1]`
отвергается как `incomplete` даже при полном совпадении expected/observed.
Фикстуры и тесты подтверждают локальный выдуманный формат вместо upstream.

Исправить по exact T/013: одинаковая версия rendering и case/source linkage,
один корректный блок, полный normalized block equality; заметки вне блока допустимы,
multiple/missing/malformed blocks не дают match. Не менять Blueprint ради текущего кода.
Перепроверить и документировать канонические бюджеты: 32KiB JCS context object
с key/value и escaping, 36KiB полного блока, а не только длину произвольного raw Context.
Нынешний DTO — внутренний срез, не альтернативная wire schema; нужна явная граница
отображения `DeliveryIntent`/`ObservedTaskAttributes`, без выдуманного нового контракта.
Repro: `TestReviewCanonicalHandoffMatches`.

### DS-REV-002 · P1 · Evidence не связано с запрошенной task

`FileReader.Read` использует `targetID` только в тексте ошибки; saved observation
вообще не содержит проверяемой target identity. Один файл возвращается для любого
запрошенного ID, а `VerifyTarget` подписывает результат ID из аргумента. В опыте файл
с `target_id=saved-task` успешно прочитан как `different-task`.
Дополнительно consumer возвращает `match` с нулевым временем и пустым source.

Связать сохранённую observation с действительной target identity, проверить совпадение
с запросом и обязательные метаданные происхождения/времени до положительного результата.
При отсутствии достаточного evidence вернуть non-success. Synthetic adapter должен
сохранять честное synthetic происхождение, не изображать `CLICKUP_GET_TASK_V2`.
Не добавлять provider writes, автоматический retry или переход business case state.
Repro: `TestReviewSavedObservationCannotBeRelabelled`, `TestReviewNoProvenanceCannotMatch`.

### DS-REV-003 · P1 · JSON может скрыть отсутствие retrieval и исправить повреждённый текст

`ParseFixture`/file decoder используют обычный `json.Unmarshal` без строгой проверки
ключей и Unicode до потерь при преобразовании. `"retrieved":false,"Retrieved":true`
даёт **match**. Сырые invalid UTF-8 bytes или одинокий `\ud800` в обоих Context
превращаются в одинаковый replacement character и тоже дают **match**.
Это воспроизведено в offline diagnostic, не заявляется эксплуатационной уязвимостью live-сервиса.

Отклонять malformed Unicode до lossy decoding, aliases/duplicate/unknown keys и
недопустимые типы/форму input согласно явному локальному контракту. Применить одну
наблюдаемую политику и к fixture CLI, и к saved observation boundary. Никакого
исправления повреждённого evidence в подтверждающие данные.
Repro: `TestReviewJSONAliasCannotPromoteRetrieval`,
`TestReviewJSONMalformedBytesCannotMatch`, `TestReviewJSONLoneSurrogateCannotMatch`.

### DS-REV-004 · P1 · Валидация evidence неполна даже без JSON

`Compare` проверяет UTF-8 только Context и HandoffBlock, пропуская Title и остальные
сравниваемые строки. Одинаковый невалидный Title даёт match. Поле `RetrievalError`
полностью игнорируется при `Retrieved=true`: результат с `permission denied` тоже match.

Явно проверить все входы, требуемые валидным контрактом, и противоречивые признаки
retrieval/completeness. Invalid expected не должен допускать ложный положительный
результат; ошибка чтения и отсутствующее происхождение не превращаются в success.
Repro: `TestReviewMalformedTitleCannotMatch`, `TestReviewRetrievalErrorCannotMatch`.

### DS-REV-005 · P2 · Пропущена нормализация одиночного CR

`normalize` заменяет CRLF, но не CR. T/013 требует NFC и **CRLF/CR → LF**, без trim,
case-fold или collapse пробелов. `a\rb` против `a\nb` ошибочно даёт mismatch.
Исправить общее правило и добавить границы для CR, CRLF, NFC, значимых пробелов и controls.
Repro: `TestReviewLoneCRNormalization`.

### DS-REV-006 · P2 · Не завершена обязательная система проверок

HIT-45 mutation matrix содержит пять намерений, но **ноль исполненных мутаций**.
Отчёт прямо пишет `no compilable mutation was created`. Guard cases не заменяют
изменение исходника, компиляцию, обнаружение поведенческого дефекта и восстановление.
Нет законченного вывода двух техник тест-дизайна, анализа фактического покрытия и
заполненной матрицы probability × impact с частотой проверок/риском безопасности.

Выполнить ≥5 независимых компилируемых мутаций по одной: baseline PASS → mutated
build PASS → assertion FAIL либо честный survivor → усиление/retest при необходимости
→ restoration с hashes → final PASS. Сохранить patch/argv/exit/test/source/CI.
Сформировать test design минимум двумя содержательными техниками, coverage gap analysis
и риски по точной матрице методички. Hidden suite остаётся NOT PROVIDED до выдачи.

### DS-REV-007 · P2 · R1 и прослеживаемость 29 результатов не опубликованы

В проверяемом стеке отсутствуют `requirements.md`, `interview.md`, `security-baseline.md`,
`requirements-trace.json`, названные в HIT-41. `criteria-crosswalk.json` не содержит
29 индивидуальных criteria/evidence записей: только общий статус, PR SHA и human-gates.

Опубликовать сохранённый авторский R1 из реальной локальной работы отдельным PR либо
подготовить недостающую desk-research часть с честной новой provenance. Не восстанавливать
старый commit выдуманным SHA. Нужны module FR/error/NFR/AC, место в HP-VER-001, пять
вопросов безопасности, чистые fixtures и source trace. Отсутствие реального интервью
отмечается отдельно и не мешает закончить документы. Сделать [29-строчный аудит](criteria-audit.json)
истинным completion crosswalk после исправлений, не превращать текущий аудит в PASS.

### DS-REV-008 · P2 · DOCX пока не CONTENT_READY_IMAGES_PENDING

Четыре DOCX открываются как ZIP/XML и имеют правильные 3+5+7+6 image slots, media=0.
Однако внутри остаются `TEMPLATE_UNFILLED`, незаполненные выводы (`...: .`), общие
инструкции вместо собственного анализа, `NOT RUN / diagnostic_smoke PASS` в каждой
строке dev/release, одинаковое средство/назначение семи разных позиций toolchain,
включая profiling с необоснованным PASS. Требования, риск-таблицы и coverage не завершены.
В заголовке `tag → commit` указан не созданный tag; это расходится с последующими
ограничениями и submission record. Screenshot инструкции местами содержат `NOT CREATED`
и `NOT RUN` вместо команды, которую агент ещё должен был исполнить сам.

После исправления кода/проверок заполнить четыре формы реальным содержанием и точными
source/CI. Записать не созданные tags как NOT CREATED, не как существующую ссылку;
не ставить рабочие результаты в ожидание ручных снимков. Удалить редакционные
инструкции автору из готового текста; сохранить только известные личные поля и
реальные human boundaries. Нужны 29 records с действительными evidence/status,
21 полная human capture instruction, DOCX integrity/link/text checks, явная граница
визуального render QA. Проверка ZIP/XML не подтверждает пагинацию.

### DS-REV-009 · P2 · Подмена и карта не подтверждены поведением

Существуют FileReader и test double, но один и тот же `VerifyTarget` не прогнан
через normal/boundary/error с каждой реализацией. Файловый adapter имеет только один
happy-path тест; fake reader — normal/error. Module map приписывает CLI зависимость
от application, хотя CLI вызывает domain напрямую. Публичные операции/типы не имеют
требуемой stack-native GoDoc; два Markdown-контракта не заменяют её.

Сохранить consumer-owned port и единственный consumer; продемонстрировать минимум
три категории поведения × две реализации, composition и контракт ошибок/evidence.
Привести карту к реальным imports, документировать использованные публичные границы
и владение данными/side effects/ошибками. Оба adapter могут оставаться offline;
эта лабораторная не требует преждевременной live-интеграции.

## Дальнейшая работа автора

Готовое поручение агенту: [AUTHOR_CORRECTION_PROMPT_RU.md](AUTHOR_CORRECTION_PROMPT_RU.md).
Порядок: R1 → канонический contract/comparator → строгие входы и evidence boundary
→ две реализации/регрессия → действительные мутации и evidence → карта/GoDoc → отчёты.
При совместной правке родительских веток сохранять три осмысленных code PR; не
обходить требуемый CI. Новые commits нуждаются в повторной проверке проверяющего.
Ни один PR этого стека не принят и не merged в ходе текущего ревью.

## Что этим ревью не засвидетельствовано

Реальное интервью, личная blind-проверка другим студентом, личная взаимная проверка
чужих scaffold/module, opposition другого проекта, подписи, скриншоты и защита.
Их требования не отменены, но **они не являются причиной найденных технических
дефектов** и не превращают командное ревью обратно в ожидание пользователя.
Feature/Epic и полная сдача не закрыты. Снимки агентом не создавались.
