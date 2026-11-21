# HIT-127. Исходники Health, точная OpenAPI и инструкция

Код `4da48202e77131427730b1827df9ee81940e559a`, база `d450efc7cfc11f8155a476304aced96dad0ebaf6`. Вложенный полный diff исключает только папку отчёта HIT-127 и её JSON результата. Проверенный SHA-256 diff `43448d180cf3884cb22bbd1afa31f758d1edc2adf4806ee2c4c045ca770fed96`. Патч применён в отдельном индексе Git, восстановленное исходное дерево сверено с проверенным коммитом по той же области. После старого проверенного кода `f1ddceeda0a61b7a7fd58a79d9b3616d4d614702` менялись только тесты сбора и runner. Миграция совпала побайтно, SHA-256 `a4f89c9b78a09c6d548422d661d9dcab7171e87397dbb2ebb2f7bc7f9958bb93`. Контракт и копия schema Health совпали, закреплённый OpenAPI SHA `fedbe40bb597e25f66aeb1172af3ff7459c81b40`, SHA-256 `a0c65940c8175de884bd404bc1187ace3be3c009737cc40b047e550e3f2c2b9e`.

Health на порту 9444 проверяет mTLS OPS и существующий технический barrier/DB. Код `READY` наблюдался одновременно с HELD/ATTENTION; после отказа подключения worker зафиксирован `DEGRADED`. `LIVE` остаётся допустимым значением общей схемы; проверяемый обработчик в этих сценариях его не выдавал. Формат обязательных полей и отказ на добавленном поле отдельно проверены на копиях реального ответа. Фактические ответы содержатся в `process-and-http.md`.

## d450efc, полный обработчик Health

~~~go
func (rt *Runtime) health(peer, revision string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !peerOK(r, peer):
			writeError(w, r, 403, "HEALTH_ACCESS_DENIED")
			return
		case !routeOK(r, "/internal/v1/health/processing", http.MethodGet):
			writeError(w, r, 404, "NOT_FOUND")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), protectedTime)
		defer cancel()
		state, e := rt.a.gate.State(ctx)
		status := "CLOSED"
		if state == startgate.Open && !rt.a.stopping.Load() {
			status = "READY"
			if e != nil || len(rt.a.stand) == 0 || rt.pool[0].Ping(ctx) != nil || rt.pool[1].Ping(ctx) != nil {
				status = "DEGRADED"
			}
		}
		writeJSON(w, 200, drafts.Health{BuildRevision: revision, CheckedAt: time.Now().UTC(), ProtocolVersion: "1.0", Service: "PROCESSING", Status: status})
	})
}
~~~

## Проверенный код, полный обработчик Health

~~~go
func (rt *Runtime) health(peer, revision string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case !peerOK(r, peer):
			writeError(w, r, 403, "HEALTH_ACCESS_DENIED")
			return
		case !routeOK(r, "/internal/v1/health/processing", http.MethodGet):
			writeError(w, r, 404, "NOT_FOUND")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), protectedTime)
		defer cancel()
		state, e := rt.a.gate.State(ctx)
		status := "CLOSED"
		if state == startgate.Open && !rt.a.stopping.Load() {
			status = "READY"
			if e != nil || len(rt.a.stand) == 0 || rt.pool[0].Ping(ctx) != nil || rt.pool[1].Ping(ctx) != nil {
				status = "DEGRADED"
			}
		}
		writeJSON(w, 200, drafts.Health{BuildRevision: revision, CheckedAt: time.Now().UTC(), ProtocolVersion: "1.0", Service: "PROCESSING", Status: status})
	})
}
~~~

## OpenAPI, точное извлечение закреплённого маршрута и схемы

~~~json
{
  "path": "/internal/v1/health/processing",
  "operation": {
    "operationId": "healthProcessing",
    "responses": {
      "200": {
        "$ref": "#/components/responses/MvpR011"
      },
      "400": {
        "$ref": "#/components/responses/MvpR004"
      },
      "401": {
        "$ref": "#/components/responses/MvpR009"
      },
      "403": {
        "$ref": "#/components/responses/MvpR002"
      },
      "404": {
        "$ref": "#/components/responses/MvpR005"
      },
      "409": {
        "$ref": "#/components/responses/MvpR008"
      },
      "412": {
        "$ref": "#/components/responses/MvpR007"
      },
      "413": {
        "$ref": "#/components/responses/MvpR006"
      },
      "429": {
        "$ref": "#/components/responses/MvpR003"
      },
      "503": {
        "$ref": "#/components/responses/MvpR010"
      }
    },
    "security": [
      {
        "mtls": []
      }
    ],
    "summary": "healthProcessing",
    "tags": [
      "PROCESSING"
    ],
    "x-hitpipe-behavior": "HP-BP-006",
    "x-hitpipe-callers": [
      "OPS"
    ],
    "x-hitpipe-interface": "HP-IF-013",
    "x-hitpipe-listener-port": 9444,
    "x-hitpipe-owner": "PROCESSING"
  },
  "success_response": {
    "content": {
      "application/json": {
        "schema": {
          "$ref": "#/components/schemas/Health"
        }
      }
    },
    "description": "Successful bounded result"
  },
  "Health": {
    "additionalProperties": false,
    "description": "Liveness is not customer business readiness or achieved SLO.",
    "properties": {
      "build_revision": {
        "pattern": "^[a-f0-9]{40}$",
        "type": "string"
      },
      "checked_at": {
        "format": "date-time",
        "type": "string"
      },
      "protocol_version": {
        "const": "1.0",
        "type": "string"
      },
      "service": {
        "maxLength": 255,
        "minLength": 1,
        "type": "string"
      },
      "status": {
        "enum": [
          "LIVE",
          "READY",
          "DEGRADED",
          "CLOSED"
        ],
        "type": "string"
      }
    },
    "required": [
      "status",
      "service",
      "build_revision",
      "protocol_version",
      "checked_at"
    ],
    "type": "object"
  }
}
~~~

## RUN_RU.md, полный текст

~~~markdown
# Служебная диагностика Processing

На подготовленном изолированном стенде запусти существующий бинарник Processing с настройкой HIT-111. JSON пишется в штатный stderr процесса. Для текущего стенда используй существующий sink журналов процесса и доступ к операторским файлам. Символы `RUN_LOG` и `PURGE_ERR` ниже обозначают разрешённые локальные файлы этих журналов.

```sh
jq -c 'select(.msg=="processing queue sample")' "$RUN_LOG"
jq -c 'select(.msg=="processing barrier diagnostic" or .msg=="processing diagnostic fault")' "$RUN_LOG"
jq -c 'select(.msg=="processing observation committed")|{time,case_id,message_id,effect_id,observation_age_ms,disposition,case_state}' "$RUN_LOG"
jq -c 'select(.state=="HELD")|{time,msg,message_id,request_id,state,reason,disposition}' "$RUN_LOG"
jq -c 'select(.reason|strings|startswith("purge_"))|{time,reason,workspace_id}' "$PURGE_ERR"
```

1. Проверь свежесть строки и `workspace_id`. Если snapshots перестали появляться, проверь `processing barrier diagnostic` и безопасный код `postgres_unavailable` или `barrier_denied`. Последняя сохранённая строка может быть старой.
2. Смотри `outbox.held`, `cases.unknown` и `cases.attention` отдельно. При `cap_reached` выведенные числа ограничены выборкой. `jobs.expired_leases` и пример ID показывают просроченную аренду. Они сами по себе не разрешают повтор внешней записи.
3. Передавай оператору только разрешённые opaque IDs и коды причин. Для очистки смотри stderr отдельной команды, а для подтверждения полного сбора позже используй собственный протокол стенда. Исключи рабочие секреты и тела из выгрузок.
4. Для технической проверки вызови существующий endpoint `GET /internal/v1/health/processing` с назначенным OPS mTLS-доступом. Формат ответа сохранён. Успешный технический ответ может сосуществовать с HELD или UNKNOWN.

Воспроизведение на отдельном одноразовом PostgreSQL 18.7, закреплённом digest, собственных файлах и локальных управляемых mTLS-соседях. Запуск из корня репозитория:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 services/processing/scripts/postgres/verify_diagnostics.py \
  --output /absolute/path/to/new-log-dir --go /absolute/path/to/go1.27.2/bin/go
```

Наблюдаемые JSON-строки сохраняются в `diagnostic-runtime.jsonl`, точные команды, версии, коды выхода и SHA — в протоколах проверки. В этом сценарии намеренные SQL и файловые отказы допускаются только в одноразовом контейнере и его собственной директории.
~~~

## diagnostics-map.md, полный текст

~~~markdown
# Источники операционной диагностики Processing

Основание — HP-MVP-A-028, GLOBAL_NFR, точный контракт HIT-127 редакции 1 и существующие Processing SQL и Health. Версии сохранены в `source-audit.json`.

## Снимок очередей

`processing queue sample` выполняется после проверки существующего допуска в настроенном workspace. Worker открывает короткую read-only транзакцию, задаёт scope через RLS и явно фильтрует `job_index`, у которого нет RLS. Снимок использует единое время SQL-запроса. В каждой из четырёх групп знаменатель — поле `rows`, не более 100 строк на workspace. При `cap_reached=true` показано состояние первых 100 строк, точный размер больше или равен лимиту. Inbox, outbox и дела упорядочены по UUID, задания — по `due_at, job_id`. Результат не представляет всё пространство заказов и не сравнивается с общей пропускной способностью.

Все времена — миллисекунды. Значение `null` означает отсутствие пригодной строки для данного измерения. Интервал задаёт паузу не менее двух секунд между проходами. Бюджет одного workspace, включая авторизацию и SQL, 350 мс. Используется существующий предел не более пяти настроенных workspace. При закрытом или недоступном барьере новый снимок не выдаётся, на ошибке SQL нет фиктивных нулей.

| Поле или событие | Источник и измерение | Граница интерпретации |
| --- | --- | --- |
| `inbox.held`, `inbox.max_age_ms` | `inbox.disposition`, разница SQL now и самого раннего `received_at` из выборки | Возраст сохранённого входа. Из inbox нельзя вывести сквозную задержку до ClickUp или число ещё не обработанных сообщений. |
| `outbox.pending/claimed/held/acked`, `max_age_ms` | `outbox.state`; возраст от `created_at` среди PENDING/CLAIMED/HELD | HELD остаётся видимым. ACKED обозначает квитанцию внутренней доставки, знание фактического внешнего эффекта живёт отдельно. |
| `outbox.max_due_lag_ms`, `expired_leases` | Задержка от прошедшего `next_attempt_at` среди PENDING/CLAIMED; просроченные CLAIMED считаются по `lease_until` | График локального отправщика. В тихом снимке отсутствует измерение просроченного due-time. |
| `jobs.due`, `max_due_lag_ms`, `expired_leases` | `job_index.due_at`, `claim_token`, `lease_until` | Дедлайн планировщика. Истечение аренды не доказывает неуспех или отсутствие внешнего эффекта. `expired_job_id` / `expired_message_id` — один пример из выборки для поиска события. |
| `cases.waiting_data/attention/unknown` | `case_identity.state` из той же scoped транзакции | Текущие сохранённые состояния; ATTENTION и UNKNOWN показываются по отдельности. |
| `processing observation committed` | Время PostgreSQL, с которым транзакция проверила Observation, минус его `ObservedAt`. Публикация только после успешного commit нового сообщения. | Один возраст на свежее подтверждённое изменение. Повтор квитанции и неудачный commit событие не создают. После перезапуска журнал предыдущего процесса читается отдельно; возраст не реконструируется из одного HTTP 202. |
| `processing barrier diagnostic` | Изменения состояния существующего HIT-69 `CLOSED/SYNCING/OPEN`, ошибка чтения `UNAVAILABLE` | Состояние допуска процесса. Локальный отказ конкретного разрешения может дополнительно дать `barrier_denied`. |
| `processing diagnostic fault` | Фактическая ошибка существующего хранилища, snapshot, worker index/claim, stale/expired lease, отправщика, recheck или таймера | Коды фиксированы. `dispatch_held` показывает отказ текущей попытки в отправщике; причина допуска проверяется отдельно. `job_recovery_failed` относится к восстановлению индекса из сохранённых case/inbox после перезапуска, в том числе если их чтение или запись временно недоступны. Процедура восстановления копии относится к HIT-128. |
| `purge_volume_unavailable`, `purge_postgres_unavailable`, `purge_scope_denied`, `purge_incomplete` | Отдельный процесс очистки HIT-126 пишет безопасную причину в stderr; число held/removed остаётся в stdout при успехе | Результат одной служебной команды. Требования к роли и эксклюзивной директории сохранены. |

Ограничения диагностики: максимум одно предупреждение каждого кода за две секунды на процесс и 16 событий новой observation за двухсекундное окно. Следующее принятое окно указывает число пропусков предыдущего. Нет растущей очереди диагностических записей. Значения идентификаторов разрешаются лишь после проверки формата UUID и используются для корреляции в JSON, без разрастания набора имён полей. В события не попадают тела, пути, тексты ошибок из драйвера, session, CSRF, credentials, имена/ID внешних аккаунтов и AI-текст.

Health по существующему OAS остаётся отдельным техническим ответом. Наблюдения на одноразовом стенде измеряют локальные интервалы и отказные сценарии. Сквозной p95/p99, доступность PILOT и другие продуктовые SLO в этих данных не оценены.
~~~
