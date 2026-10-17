# HP-ADR-003 — Keycloak аутентификация и Go IAM бизнес-полномочия

**ID ADR:** HP-ADR-003. **Рабочая revision:** In Review / Candidate. **Архитектурный выбор:** Accepted by delegated autonomous architect по явному пользовательскому поручению; human phase acceptance не выполнена и не подразумевается.

## 1. Контекст и область решения

Продукт обрабатывает scoped данные разных компаний и выполняет внешние действия. Нужно разделить подтверждение identity, business roles, внешний provider доступ и машинные полномочия. Собственный пароль/MFA/recovery — значительная самостоятельная безопасность, не ценность первого сопровождаемого handoff. Область: HP-PART-001/006/008, HP-IF-001/002/010/012; HP-RULE-005/010/011; HP-NFR-005/006/008/009.

## 2. Выбранный вариант и rationale

Самостоятельно размещённый Keycloak — OIDC IdP, без application realm-admin credentials. Go IAM проверяет code+PKCE/state/nonce и создаёт собственную opaque server session. Browser не хранит role JWT в localStorage. Identity — `(issuer,subject)`, а не email. Текущие memberships, роли, workspace scope и deny epochs принадлежат IAM, а case/config/resource принадлежность проверяет конкретный data owner.

Keycloak не хранит HitPipe business case policy и не является единственным механизмом пятисекундного отзыва. Долгая валидность ID/access token не сохраняет membership после revoke. Provider credentials не передаются в IAM profile: Stripe хранит Intake, ClickUp — Delivery. Внутренние service identities независимы от human initiator; действие не переписывается как «сделала система» ради обхода отзыва.

Выбор поддерживает self-hosting и уменьшает собственный authentication code. Цена — отдельный IdP runtime, защищённая БД, production configuration и расход памяти; существующий2GB VPS не назначен полным dev/stage контуром. Точные совместимые версии и реально выполненные проверки — в `/formal/stack/` и supporting stack report; дата registry не доказывает фактический login.

## 3. Рассмотренные альтернативы

| Вариант | Соответствие и ограничения | Решение |
| --- | --- | --- |
| Собственные password/MFA/recovery в Go | Максимум контроля, но большая дополнительная security область без продуктовой необходимости | Не выбран |
| Keycloak + IAM opaque session | Стандартный OIDC и self-hosting; business grants отделены от IdP | Выбран |
| Внешний managed IdP | Может снизить администрирование; добавляет внешний operational/account/commercial dependency | Реальная категория альтернативы, не выбрана в исходном self-hosted контуре; цены/доступность не выдуманы |
| Долгоживущий подписанный JWT с tenant roles как единственная authority | Меньше online checks, но отзыв зависит от expiry/кэша и оставляет опасные stale grants | Не выбран; подпись токена не доказывает текущую business authority |

## 4. Последствия и канонические технические источники

| Следствие | Источник обязательной семантики | Ограничение |
| --- | --- | --- |
| OIDC flow, opaque session, CSRF | HP-BP-001/T/001–003; HP-BP-005 | Одни endpoint paths/claims из документации не означают выполненную security проверку |
| Роли, минимальный профиль, scope и binding uniqueness | HP-BP-001/T/004–008/013 | Не добавляет TEAM-функции в SINGLE |
| Синхронный отзыв всех runtime | HP-BP-001/T/009–012; HP-BP-007 | См. HP-ADR-004; IdP logout не заменяет gate barrier |
| Production IdP, DB, TLS, secret rotation, backup | HP-BP-006 | Раздельный admin plane и ограниченные credentials |

Источники: [Keycloak OIDC](https://www.keycloak.org/securing-apps/oidc-layers), [Keycloak PostgreSQL support](https://www.keycloak.org/server/db), actual stack compatibility report2026-10-16. Сведения о стандартных OIDC endpoints подтверждены первичным источником; выбор конкретной разделённой authority — решение HitPipe.

## 5. Пересмотр, исторические связи и изменения

Пересмотр при изменении self-hosting/команды/IdP operational cost, необходимости enterprise federation либо установленном несовместимом security ограничении. Миграция IdP требует явного identity linking с доказательством владельца, а не совпадения email; сохраняет workspace/ownership и revoke history. Business authority не мигрирует в JWT по удобству SDK. Это первый целевой ADR; legacy auth code не назван Accepted baseline и не подменён утверждением о уже завершённой миграции.

## 6. Проверка перед source review и faithful-проекцией

Выбор, scope и альтернативы определены. Независимая проверка сверяет JWT/session разделение, CSRF, caller identity, scopes, negative access paths и реальную принадлежность provider secrets. До live нужны настоящий OIDC login/logout/recovery, role matrix, Keycloak deployment hardening и revoke race tests. Source revision acceptance и admission остаются отдельными; документ не утверждает прохождение этих будущих проверок.
