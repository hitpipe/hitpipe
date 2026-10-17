# ADR — Same-origin Next.js UX с owner-backed командами и честным состоянием

**ID:** HP-ADR-007. **Рабочее состояние:** In Review. **Ответственный:** делегированный архитектор, UX/BFF owner; source integration — Codex root.

## 1. Контекст и область решения

Человек управляет судьбой бизнес-заказа и незавершённого handoff, а не техническими запусками. HP-RULE-002/007/009, HP-NFR-005/008/011 требуют достоверного состояния, безопасности и доступных действий. Заказчик предпочитает Next.js/TypeScript/Tailwind. Нужно выбрать browser/backend interaction, не создающий второй authority и не усложняющий маленькую первую версию несколькими API gateways, global state stores или обязательным realtime transport.

Область — HP-PART-006, связь с Go Identity и Processing, bounded refresh UI, server/client caching и доступность основных путей. Этот ADR не выбирает новую capability, экран как нормативный модуль или backend-owned case lifecycle. Auth protocol и physical-start revocation остаются HP-BP-001/007.

## 2. Выбранный вариант и rationale

Next.js App Router предоставляет английский UI и same-origin BFF. Выбран официальный custom Node server, обычный next build output без standalone, чтобы private IAM-control/OPS listeners и защищённые response gates жили в одном процессе. Shared globalThis symbol registry устраняет копии lock в bundled route chunks; exact behavior BP005/T021. Это оправданный отказ от части automatic static optimizations, не неявное смешение standalone/custom-server. Go services сохраняют data/command authority; BFF authenticates/forwards действительного инициатора и не принимает бизнес-решения через React state/Server Action. Next SSR помогает initial navigation, но защищённые данные всегда request-scoped и uncached across tenants. Для наблюдаемого незавершённого дела используется ограниченный polling плюс fresh-read после действия/возврата фокуса; отдельный WebSocket runtime MVP не требуется.

UI явно различает proposed/saved/accepted configuration, pending/unknown command outcome, восемь case states и время последнего подтверждения. Backend определяет допустимые действия и ещё раз проверяет их при применении. Семантические элементы, клавиатура и screen-reader/zoom проверяются на целых путях, не как необязательное оформление. Выбран browser intersection реально используемых Next/Tailwind: Chrome/Edge111+, Safari16.4+, Firefox128+; дополнительные bleeding-edge CSS utilities не становятся обязательной зависимостью старых поддержанных браузеров.

**Статус выбора:** Accepted как архитектурное решение по явному делегированию пользователя; source/baseline остаётся In Review / Candidate, формальная приёмка не выполнена. Смысл обязательств — HP-BP-005/T/001–T/003/T/009/T/010/T/013–T/016, не этот rationale.

## 3. Рассмотренные альтернативы

| Вариант | Соответствие и trade-offs | Вывод |
| --- | --- | --- |
| Next same-origin BFF, authoritative Go owners, bounded polling | Один browser trust boundary и меньше long-lived transports; требует аккуратного auth/cache/CSRF и stale-state представления | Выбран; не привязан к Vercel hosting |
| Next Server Actions напрямую пишут business DB/вызывают provider | Меньше API forwarding, но создаёт обход data authority и отдельный implementation path guard/retry | Не выбран для HitPipe state/effect mutations |
| SPA с прямыми bearer calls из browser в каждый microservice | Возможна независимая frontend delivery, но добавляет browser token exposure/CORS и несколько публичных trust boundaries | Не выбран для данного cohort; не универсальный запрет SPA |
| WebSocket/SSE как обязательный transport любого состояния | Удобная push модель, но нужны дополнительные reconnect/order/revocation semantics без продукта, требующего субсекундного realtime | Не выбран для MVP; может пересматриваться по измеренным потребностям |
| Optimistic local business success после transport ACK | Выглядит быстрее, но создаёт ложное confirmed/activation state при unknown/lost response | Не соответствует product outcome semantics |

## 4. Последствия и канонические технические источники

| Следствие | Канонический источник | Контекст |
| --- | --- | --- |
| Same-origin защищённый BFF, opaque session и текущая authority | HP-BP-001; HP-BP-005/T/001/002 | SSR и API проходят одинаковую проверку доступа |
| Изоляция protected caches и nonce CSP | HP-BP-005/T/003/014/016 | Next runtime production configuration и edge policy согласованы |
| Case/action state берётся из owner query; lost response не local success | HP-FORMAL-001; HP-BP-005/T/009/010/013 | Конкуренция вкладок, late responses и unknown external effect |
| Bounded refresh и полный keyboard/reader/zoom опыт | HP-BP-005/T/014/015 и PARAM/REFRESH | Product NFR остаются источником критериев, не новый «достаточный» UI score |
| Browser/framework compatibility и type bridge | HP-ADR-005; HP-BP-005/T/019 | Пинованные libs, clean build и browser evidence |

Риски: polling требует общих NFR capacity tests; скрытая вкладка/плохая сеть не доказывают свежесть и не дают основания изменять бизнес-состояние. Next имеет несколько уровней caching, поэтому «отключили один fetch cache» не равно tenant-isolation proof. CSP/nonce должны реально проверяться с hydration и deployed edge; compile success этого не доказывает.

Actual evidence — bounded Next production Turbopack build и strict TS compilation на pinned cohort. Маленькая synthetic страница и generated API types не являются полным UX, доказанным WCAG соответствием или whole-platform E2E. Итоговые browser/runtime результаты указываются в evidence manifest с точными source hashes. Эмпирическая понятность продукта проверяется HP-NFR-012; decision не выдаёт её за достигнутую.

## 5. Пересмотр, исторические связи и изменения

Основания пересмотра: измеренно неприемлемая polling capacity/latency, отдельное product требование live push/offline operation, поддержка браузеров вне выбранного пересечения, несовместимый Next/security change либо изменение authority boundary. Добавление realtime/offline не выполняется исключительно выбором новой React library. Необоснованное ослабление accessibility/tenant protection для ускорения UI не является допустимым trade-off.

Первое решение этой области; прежний mock UI в старом repository baseline не объявляется принятым source. Ни один исторический ADR не переписан и supersession без основания не заявляется. Frozen implementation context обновляется только согласованным source-set change; новая page revision не меняет применяемую архитектуру самостоятельно.

## 6. Проверка перед source review и faithful-проекцией

Сохранены один связный browser/backend interaction выбор, обязательная microservice authority, реальные альтернативы и причины. Norms/parameters принадлежат BP/AD/formal; ADR не копирует wire fields и количественные NFR. Actual evidence ограничено выполненными проверками, независимый review и formal source acceptance остаются отдельными записями итогового пакета.
