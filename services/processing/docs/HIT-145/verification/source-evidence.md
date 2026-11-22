# HIT-145. Исходники и зависимость

База [HIT-111](https://github.com/hitpipe/hitpipe/pull/72) — `49229e1db3f371a051a022ea9e31951dd558eecb`. Его принятый результат указывает проверенный исходник `b2e823012854575146595b01a6f56d5ac677fc44`. Оба входят в историю сборки `30385d09962c3fa4aad4808253a5493235559d01`; обе проверки `git merge-base --is-ancestor` вернули 0. PR №84 направлен в рабочую ветку HIT-111. Наблюдавшийся main `ce771d37505e7770003d0796ae932556d06b7bab` не содержит HIT-111. [Результат зависимости](https://github.com/hitpipe/hitpipe/blob/49229e1db3f371a051a022ea9e31951dd558eecb/.project-os/results/HIT-111.json) · [Независимое ревью HIT-111](https://hitpipe.atlassian.net/browse/HIT-111?focusedCommentId=14258)

[Полный diff проверенной сборки](https://github.com/hitpipe/hitpipe/compare/49229e1db3f371a051a022ea9e31951dd558eecb...30385d09962c3fa4aad4808253a5493235559d01). Оригинальный вывод `git diff --name-status`:

```text
A	services/processing/Dockerfile
A	services/processing/Dockerfile.dockerignore
A	services/processing/docs/HIT-145/RUN_RU.md
A	services/processing/docs/HIT-145/processing.example.json
A	services/processing/internal/adapters/postgres/image_process_test.go
M	services/processing/scripts/postgres/verify.py
A	services/processing/scripts/postgres/verify_image.py
```

`git diff --exit-code` от проверенного HIT-111 по cmd, production Go, зависимостям, Platform, формальной миграции и корневому Dockerfile вернул 0 с пустым stdout. Обработчики, mTLS, RLS, TTL и финансовые правила сохранены. [Команды, SHA и коды](source-commands.json)

[Dockerfile](https://github.com/hitpipe/hitpipe/blob/30385d09962c3fa4aad4808253a5493235559d01/services/processing/Dockerfile) · [Контекст сборки](https://github.com/hitpipe/hitpipe/blob/30385d09962c3fa4aad4808253a5493235559d01/services/processing/Dockerfile.dockerignore) · [Runner](https://github.com/hitpipe/hitpipe/blob/30385d09962c3fa4aad4808253a5493235559d01/services/processing/scripts/postgres/verify_image.py) · [Стендовый тест](https://github.com/hitpipe/hitpipe/blob/30385d09962c3fa4aad4808253a5493235559d01/services/processing/internal/adapters/postgres/image_process_test.go)

Локальный `go test -race -count=1 ./...` и `go vet ./...` на Go 1.27.2 Darwin arm64 завершились с кодом 0. Контейнерный TEST выполнен отдельно на Linux arm64. [Go race stdout](source-go-race.stdout) · [Go vet stdout](source-go-vet.stdout)
