# DK-ADR-001 — чистая SessionPolicy и consumer-owned ports

Дата: 18.10.2026 UTC. Автор-owner: Дмитрий Карачебан; подготовил агент Codex. Статус: implemented candidate, **ожидает отдельного человеческого review**. Локальный implementation ADR HIT-22; не новый HP-ADR-001 и не изменение Architecture Contract.

## Контекст

HP-BP-001/T/002–003 требуют fail-closed validity и отделяют её от business grant. Лабораторная должна дать реальную переиспользуемую логику, consumer и подстановку двух реализаций, не захватывая SQL/OIDC/чужие service boundaries. В snapshot нет lastActivity; нельзя изобрести column и нельзя ограничить обновлённый idle первыми восемью часами от created.

## Варианты

1. Проверять всё прямо в CLI с time.Now и чтением JSON: меньше файлов, но тесты зависят от host time, нет reusable consumer и явной authority boundary.
2. Сразу встроить SQL/OIDC/current authorization: ближе к полному IAM, но требует реальных trust/storage протоколов, меняет общий scope и блокирует isolated verification; fake live endpoint недопустим.
3. Чистая policy значений + один application consumer с двумя узкими ports; File/Fixed и Clock adapters на внешней границе: проверяет заявленные правила и substitution без новой canonical wire/SQL.

## Решение и основание

Выбран **3**. Ports принадлежат application consumer. Время читается после Lookup; отсутствие/ошибка/отмена dependencies возвращает unavailable. Нельзя конвертировать adapter error в прежний success. New отвергает typed-nil; zero service безопасен. File/fixed diagnostic adapter выбирается только в cmd. Snapshot/Decision — values, внешнее владение не утекает через изменяемые указатели. Strict JSON локален adapter, его ошибка не раскрывает исходный payload. System time требует explicit health probe; CLI не изображает её наличие.

## Цена и последствия

Добавлены две интерфейсные границы, composition root и локальная reflect-проверка nil в application; domain остаётся только time. Синтетические File/Fixed не доказывают production authority и не заменяют будущую persistence/health integration. Ещё нужны production adapters, OIDC, выдача/продление/CSRF и business authorization по контракту. Возможное развитие не требует переписать Evaluate/Execute, но каждый реальный adapter обязан подтвердить freshness, cancellation и error semantics — механическая замена недостаточна.

## Проверка решения

Фактические imports сверены `go list -json`; wrapper guard/vet/tests PASS. Один Service.Execute проходит normal/error/boundary с File/Fixed; test latency crosses idle и clock читается после lookup; typed-nil/zero/cancel/error tests fail closed. Domain/input immutable; shared fixed snapshot проходит race exercise. Семь compiling mutants проверили чувствительность; M07 survivor устранён усилением assertions. Build modes/outside-checkout smoke фактически выполнены. Отдельный человек должен проверить oracle/границы и PR; approval/merge не подделаны.
