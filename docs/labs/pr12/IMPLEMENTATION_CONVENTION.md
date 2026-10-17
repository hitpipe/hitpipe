# Внутренняя структура компонента HitPipe

Это реализационная конвенция ручного учебного среза под HP-DELEG-001 AUTO, не новый
канонический AD и не принятие Candidate фазовых контрактов. Сервисные/data/trust
границы и wire/SQL остаются у exact Architecture источников.

## go

tree:

- cmd/<service> (composition root only)
- cmd/<service>-contractcheck (retained offline diagnostic component)
- internal/domain/<module> (pure policies and invariants)
- internal/application/<usecase> (orchestration; consumer-owned ports)
- internal/adapters/<dependency> (filesystem/http/pg/time implementations as needed)
- internal/transport/<protocol> (future real inbound transport; no fake public business routes)
- internal/bootstrap (wiring only if needed)
- testdata
- docs

dependency_rules:

- domain uses standard-library value types; no DB/framework/net/http/env/clock side effects
- application imports domain and owns narrow interfaces; no concrete adapters
- adapters implement application ports and map external wire ↔ domain; no foreign service internals/DB access
- cmd/bootstrap compose everything; domain/application never import cmd/bootstrap/adapters
- each Go service owns go.mod; no shared business aggregate library; generated wire/transport helper sharing only through integrator-owned approved package
- create layers only when used; do not create empty ceremonial directories

checks:

- go list dependency/import graph checked against layer rules
- go test ./... and go vet ./...
- go test -race ./... where supported; explicit environment limitation otherwise
- service-local ownership paths, no import of another service internal directory

## next

layers:

- src/app: routing/server composition
- src/features/<feature>/domain: pure TS projection/value rules
- src/features/<feature>/application: presenter/consumer-owned ports
- src/features/<feature>/components: React presentation
- src/server: server-only owner clients/authority boundary
- src/generated: generated types/schema artifacts; never hand edit

rules:

- domain imports neither React/Next nor browser/Node globals or fetch
- client components cannot import server-only code or connector secrets
- UI state is view/transport state, not business-state authority
- use exact pinned Next/React/TS/Tailwind/Playwright cohort; no Vite lock mix or silent new major
- test runner may use already pinned TS compiler + native Node test/coverage; browser checks use pinned Playwright; verify exact argv before marking works

## python

services/ai-interaction/src/hitpipe_ai

layers:

- domain
- application
- ports (consumer-owned; may remain within application)
- adapters
- bootstrap

rules:

- strict DTO boundary, no business DB/production connector dependencies
- FastAPI/httpx only adapters/bootstrap, not pure policy
- no Python implementation task required for this bounded PR1/2 tranche

## Рабочая область

Backend: четыре независимых модуля `services/iam`, `services/intake`,
`services/processing`, `services/delivery`, каждый со своим go.mod. Legacy корень
не мигрировать в этой подготовительной работе. Frontend: pure features в
`src/features/case-status`, server-only boundary отдельно. Legacy Vite root lock
не смешивать с новым cohort: `tooling/pr12/frontend` — отдельный воспроизводимый
инструментальный контур. Он не превращает legacy приложение в готовый Next продукт.
Универсальный entrypoint wrappers: `python3 tools/pr12/workspace.py --help`.

Только используемые слои. Application владеет ports; concrete adapter выбирается
в composition root. Один consumer тестируется с двумя объявленными реализациями.
Нет cross-service SQL, shared business aggregate, подмены unknown успешным ответом
или внешнего эффекта из диагностического компонента. Своё решение документировать
implementation ADR с module scope, не вторым HP-ADR-001.
