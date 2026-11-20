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
