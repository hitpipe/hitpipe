# TP ADR 001 Граница финансовой проверки

Автор: Тимофей Патрушев при помощи Codex. 18.10.2026, HIT-28. Статус local implementation candidate, review pending. Upstream: BP003/T002/006/012, BP002/T004, RULE006/012, BP007/T005/010 и LAB-STACK-001. Это не новый canonical HP-ADR-001.

Контекст: финансовая conjunction должна воспроизводиться без Stripe/ClickUp/DB; unknown нельзя coercion-ить в ноль. При этом будущий owner обязан проверять config/case scope до использования результата.

Варианты: (A) rule в CLI/HTTP handler — меньше типов, но consumer и parser смешиваются, отдельный тест затруднён; (B) общий межсервисный business helper — уменьшает дублирование DTO, но переносит authority и связывает релизы; (C) service-local pure domain плюс application-owned reader — дополнительная граница, но один evaluator для offline и будущего owner-path. Выбран C.

Решение: Policy/Facts — read-only value projection с nullable pointers; explicit now; Result — owned values. Consumer не импортирует adapter; File и test double проходят один набор. CLI только composition. В принятой конфигурации поддерживаемая schema/evaluator version проверяется без silent migration/latest. Domain не имеет внешних side effects.

Цена: типы Query/Snapshot/Request, runtime проверка pin, один косвенный вызов reader, pointer lifetime contract и маппинг будущего generated wire. Переданная mutable память не должна изменяться конкурентно; результаты не ссылаются на вход. Future adapter должен проверить canonical OAS/authenticity/digest отдельно. Наличие mock не позволяет деплоить фиктивный production источник.

Проверка: TestReaderSubstitution: обе реализации, normal/error/boundary; TestContextPins; TestCanceledAndInvalidContext; TestFinancialScope; domain tests и imports guard. Coverage/race и мутации HIT-29. Вопрос будущего исследования — насколько optional representation и проверка provenance влияют на стоимость обработки bounded snapshots; bottleneck без измерений не заявляется.
