# Карта модулей Processing

Автор записи: Тимофей Патрушев при помощи Codex, HIT-28. Это персональная owner-запись для общей карты, не изменение чужого shared artifact. A → B означает зависимость исходников A от B; runtime вызов через interface не меняет направление imports. Фактическая сверка: go list -f '{{.ImportPath}} => {{join .Imports " "}}' ./... из services/processing; wrapper import guard.

| Модуль и путь относительно services/processing | Ответственность и контракт | Зависимости | Статус |
|---|---|---|---|
| internal/domain/eligibility | Evaluate: финансовые predicates/typed knowledge | stdlib time | Реализован, offline checks; human review pending |
| internal/application/evaluatepaidorder | EvaluatePaidOrder, собственный SourceFactsReader | domain, context/time | Реализован, DI checks; human review pending |
| internal/adapters/fixtures/input | bounded strict local synthetic JSON | domain, stdlib JSON/file | Реализован; не canonical wire adapter |
| internal/adapters/fixtures/sourcefacts | File implements reader | application, input | Реализован в offline supported domain |
| cmd/processing-contractcheck | flags, metadata, composition, безопасный diagnostic | application, adapters, domain validation | Реализован; не business HTTP endpoint |
| Будущий authenticated Intake adapter | scope/OAS/digest/provenance/source read | application port + generated wire | ПЛАН |
| Будущий Processing owner transport/storage | actor/epoch/case/lineage/dispatch guards | application + собственная persistence | ПЛАН |

Исходные стрелки: cmd → application, cmd → adapters, cmd → domain; sourcefacts → application, sourcefacts → input; input → domain; application → domain. Обратных production imports и циклов нет. Test double только в _test.go. IAM/Intake/Delivery/UX принадлежат другим владельцам; эта карта не утверждает их реализацию. Lead может включить запись в общую карту без переавторства чужих частей.
