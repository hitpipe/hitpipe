#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

umask 077
printf '%s  %s\n' '50f9fdfa632c7721521e95afd90d8b1fd7a536e1c5845c1062a0d16ef3c0038a' \
  'src/main/resources/META-INF/keycloak-email.sql' | sha256sum -c -

run_dir=$(mktemp -d)
export HIT105_COMPOSE_ENV="$run_dir/compose.env"
export HIT105_COMPOSE_PROJECT=hit105-email-lab
docker=(docker --host unix:///var/run/docker.sock)
compose=("${docker[@]}" compose --env-file "$HIT105_COMPOSE_ENV" --project-name "$HIT105_COMPOSE_PROJECT" -f compose.yaml)
compose_started=0
loopback_pid=

# Compose gives ambient environment variables precedence. Remove only the lab
# variable names so an existing caller's credentials cannot enter the stand.
unset HIT105_DB_PASSWORD HIT105_ADMIN_PASSWORD HITPIPE_EMAIL_CODE_KEY \
  HITPIPE_EMAIL_CODE_KEY_VERSION HITPIPE_EMAIL_RATE_KEY

cleanup() {
  if [[ -n $loopback_pid ]]; then
    kill "$loopback_pid" 2>/dev/null || true
    wait "$loopback_pid" 2>/dev/null || true
  fi
  if [[ $compose_started == 1 ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  if [[ -f $HIT105_COMPOSE_ENV ]]; then unlink "$HIT105_COMPOSE_ENV"; fi
  rmdir "$run_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

{
  printf 'HIT105_DB_PASSWORD=%s\n' "$(openssl rand -hex 24)"
  printf 'HIT105_ADMIN_PASSWORD=%s\n' "$(openssl rand -hex 24)"
  printf 'HITPIPE_EMAIL_CODE_KEY=%s\n' "$(openssl rand -base64 32)"
  printf 'HITPIPE_EMAIL_CODE_KEY_VERSION=1\n'
  printf 'HITPIPE_EMAIL_RATE_KEY=%s\n' "$(openssl rand -base64 32)"
} > "$HIT105_COMPOSE_ENV"

if [[ -n $("${compose[@]}" ps -q --all) ]]; then
  echo 'Lab project already has containers; stop the previous lab before running.' >&2
  exit 1
fi

"${compose[@]}" config --quiet
compose_started=1
"${compose[@]}" up --build -d

# An internal Docker bridge has no published ports. Inspect only addresses in
# this lab's network and forward the two browser-facing services on VM loopback.
lab_address() {
  local id
  id=$("${compose[@]}" ps --all -q "$1")
  [[ -n $id ]] || return 1
  "${docker[@]}" inspect --format \
    '{{with index .NetworkSettings.Networks "hit105-email-lab_lab"}}{{.IPAddress}}{{end}}' "$id"
}

node tests/loopback.mjs "$(lab_address keycloak)" "$(lab_address smtp)" &
loopback_pid=$!
ready=0
for attempt in {1..75}; do
  if ! kill -0 "$loopback_pid" 2>/dev/null; then break; fi
  if curl --silent --fail --max-time 2 -o /dev/null \
      http://127.0.0.1:18080/realms/hit105-test/.well-known/openid-configuration \
    && curl --silent --fail --max-time 2 -o /dev/null http://127.0.0.1:18025/readyz; then
    ready=1
    break
  fi
  sleep 2
done
if [[ $ready == 0 ]]; then
  echo 'Keycloak test realm did not become available.' >&2
  "${compose[@]}" ps --all >&2 || true
  "${compose[@]}" logs --no-color --tail 80 keycloak | while IFS= read -r line; do
    while IFS='=' read -r name value; do
      if [[ ${#value} -gt 8 ]]; then line=${line//"$value"/[redacted]}; fi
    done < "$HIT105_COMPOSE_ENV"
    printf '%s\n' "$line" >&2
  done || true
  exit 1
fi

java_version=$("${compose[@]}" exec -T keycloak java -version 2>&1)
keycloak_version=$("${compose[@]}" exec -T keycloak /opt/keycloak/bin/kc.sh --version 2>&1)
postgres_version=$("${compose[@]}" exec -T postgres psql -X -qAt -U hit105 -d hit105_keycloak -c 'SHOW server_version')
mailpit_version=$("${compose[@]}" exec -T smtp /mailpit version --no-release-check)
printf '%s\n%s\nPostgreSQL %s\n%s\n' "$java_version" "$keycloak_version" "$postgres_version" "$mailpit_version"
[[ $java_version =~ version\ \"21[.\"] ]]
[[ $keycloak_version =~ Keycloak\ 26\.8\.1($|[[:space:]]) ]]
[[ $postgres_version =~ ^18\.7($|[[:space:]]) ]]
[[ $mailpit_version =~ 1\.33\.1($|[[:space:]]) ]]
node --version
node tests/browser.mjs
