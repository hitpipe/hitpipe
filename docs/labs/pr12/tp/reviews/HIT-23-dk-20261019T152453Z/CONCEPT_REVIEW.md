# Концептуальная сверка IAM

Codex по делегации Тимофея Патрушева; это агентное ревью. Источник — exact R2 contract и HP-BP-001/T/002,003,013 / HP-BP-007/T/005; formal Session/Authorization OAS и iam.sql прочитаны. Frozen sources validated; Candidate/In Review, grant=none не повышены.

| Ситуация | Вывод из нормы | Сверка с кодом/проверками |
|---|---|---|
| trusted idle−1ns, known/not revoked, корректные сроки | valid/session_valid, без business grant | Domain boundary и normal tests; raw canonical probe |
| now=idle | denied/idle_expired | Собственный CLI equality probe + boundary suite |
| revoked, сроки впереди | denied/revoked_session | Domain, оба adapters, CLI regression |
| Lookup success, Clock error | unavailable/clock_unavailable | PortErrors test; положительные сроки не разрешают fallback |
| now=absolute, idle истёк | denied/absolute_expired | AbsoluteBoundary/DecisionTablePrecedence |
| Found=false | denied/unknown_session после пригодного Clock; старый success неприменим | Unknown + contract substitution tests |

По этим шести ситуациям смысл требований, локальный oracle и проверенное поведение согласованы. Это **AGENT_CLARITY_ANALYSIS_NOT_BLIND**: контракт, код и авторский oracle были доступны; личные ответы Тимофея до oracle не получены и не сочиняются. Student clarity остаётся NOT_ATTESTED/NOT_PERFORMED.

Проверены nanosecond equality, precedence, malformed/zero/out-of-range/24h/offset, revoked/unknown, typed-nil/zero dependency, cancel, freshness Lookup→Clock, fail-closed Result/Decision, error redaction, value ownership/concurrency и safe diagnostics. Session validity не означает tenant membership/business grant. Синтетические адаптеры намеренно не выполняют multi-session lookup; tenant/session mismatch и freshness реального authority не объявлены проверенными. Нельзя навязывать Snapshot отсутствующую lastActivity или fixed created+8h renewal cap: это изменило бы заявленный scope и отвергало законное обновление idle.
