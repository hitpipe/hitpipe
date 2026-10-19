# Ранние ограничения безопасности

HIT-25; источники RULE006/011/012, BP003/T006, BP006/T008/011, BP007/T005. Учебный ввод синтетический.

1. Защищаем account/mode/Session/config identity, достоверность финансового proof и отсутствие ошибочного разрешения. Реальные контакты/бриф и provider credentials не нужны модулю.
2. Недоверенный ввод — JSON fixture сегодня, authenticated Intake adapter в будущем. Reject duplicate keys, trailing JSON, unknown fields/enums, malformed numbers, oversized input. Null отличается от false/0. Выдавать безопасный machine reason без содержимого входа.
3. Финансовые сведения читает owner-path через scoped SourceFactsReader; оператор не исправляет payment truth. Реальную session/epoch/actor authorization выполняет будущий owner, этот offline consumer не предоставляет пользовательских полномочий.
4. Реалистичный ущерб — ClickUp work на неоплаченный/чужой/refunded заказ, раскрытие payment data, ошибочное продолжение на старом proof. Поэтому любой unknown удерживает действие, identity проверяется до финансового результата.
5. Secrets в будущих owner secret files согласно BP006/T011, отдельно от JSON/SQL/logs. Здесь секретов нет. Запрещены raw body, контакты, brief, cookies, tokens и credential paths в диагностике. Логи CLI содержат только start/result/reason/version; ошибки JSON sanitised.

В C3 риски приводятся к матрице методички: низкая×тяжёлый=высокий; высокая×небольшой=средний. Полная threat model, TLS/actual-start/revoke и live accounts требуют отдельной разработки и проверки.
