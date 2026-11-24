#!/usr/bin/env bash
set -euo pipefail
umask 077

here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
state=${HITPIPE_SCHOOL_STATE:?Path to the writable school runtime directory is required}
frontend=${HITPIPE_SCHOOL_FRONTEND:?Path to the built Next application is required}
node=${HITPIPE_NODE:?Path to the pinned Node executable is required}
backend=${HITPIPE_SCHOOL_BACKEND:?Path to the persistent backend launcher is required}
go=${HITPIPE_GO:-}
if [[ -n "$go" && ( "$go" != /* || ! -x "$go" ) ]]; then
  printf 'Укажите абсолютный путь к закреплённому Go.\n' >&2
  exit 1
fi

for path in "$state" "$frontend" "$node" "$backend"; do
  [[ "$path" = /* ]] || { printf 'Нужен абсолютный путь: %s\n' "$path" >&2; exit 1; }
done
[[ -x "$node" && -x "$backend" && -f "$frontend/.next/BUILD_ID" ]]
[[ "$("$node" --version)" == 'v24.22.0' ]]

mkdir -p -- "$state"
[[ ! -L "$state" && $(stat -c '%u' -- "$state") = $(id -u) ]] || exit 1
chmod 700 -- "$state"
exec 9>"$state/service.lock"
flock -n 9 || { printf 'Учебный стенд уже запущен.\n' >&2; exit 1; }
run=$(mktemp -d "$state/run.XXXXXX")
share="$run/bridge"
mkdir -- "$share"
ln -s -- "$run" "$state/.current-$$"
mv -Tf -- "$state/.current-$$" "$state/current"

egress_pid=
backend_pid=
browser_pid=
stop() {
  trap - EXIT INT TERM
  if [[ -n "$browser_pid" ]]; then kill -TERM "$browser_pid" 2>/dev/null || true; fi
  if [[ -n "$backend_pid" ]]; then kill -TERM "$backend_pid" 2>/dev/null || true; fi
  if [[ -n "$browser_pid" ]]; then wait "$browser_pid" 2>/dev/null || true; fi
  if [[ -n "$backend_pid" ]]; then wait "$backend_pid" 2>/dev/null || true; fi
  if [[ -n "$egress_pid" ]]; then kill -TERM "$egress_pid" 2>/dev/null || true; wait "$egress_pid" 2>/dev/null || true; fi
  rm -f -- "$run/web-ready.json"
}
trap stop EXIT
trap 'exit 0' TERM INT
printf 'Протоколы учебного стенда в %s\n' "$run"

# A launcher may leave its database volumes mounted between starts. The
# bridge directory and its synthetic credentials belong to just this run.
export HITPIPE_BROWSER_SHARE="$share"

if [[ -e "$state/provider-demo.json" ]]; then
  credentials=${HITPIPE_PROVIDER_DEMO_CREDENTIALS:?Set the owner-only demo provider credentials path}
  python3 -B "$here/provider-demo/provider_egress.py" --binding "$state/provider-demo.json" \
    --credentials "$credentials" --journal "$state/provider-demo.sqlite3" \
    --socket "$share/provider-egress.sock" >"$run/egress.log" 2>&1 &
  egress_pid=$!
  for ((attempt=0; attempt<160; attempt++)); do
    [[ -S "$share/provider-egress.sock" ]] && break
    if ! kill -0 "$egress_pid" 2>/dev/null; then
      tail -5 "$run/egress.log" >&2
      exit 1
    fi
    sleep 0.25
  done
  [[ -S "$share/provider-egress.sock" ]] || exit 1
fi
backend_command=("$backend")
if [[ -n "$go" ]]; then backend_command+=("$go"); fi
"${backend_command[@]}" >"$run/backend.log" 2>&1 &
backend_pid=$!
for ((elapsed=0; elapsed<480; elapsed++)); do
  [[ -s "$share/ready.json" ]] && break
  if ! kill -0 "$backend_pid" 2>/dev/null; then
    tail -40 "$run/backend.log" >&2
    exit 1
  fi
  sleep 0.5
done
if [[ ! -s "$share/ready.json" ]]; then
  tail -40 "$run/backend.log" >&2
  printf 'Бэкенд не опубликовал готовность.\n' >&2
  exit 1
fi

"$node" "$here/browser-host/host.mjs" "$share" "$frontend" "$run" >"$run/browser.log" 2>&1 &
browser_pid=$!
processes=("$backend_pid" "$browser_pid")
if [[ -n "$egress_pid" ]]; then processes+=("$egress_pid"); fi
if wait -n "${processes[@]}"; then
  printf 'Один из процессов стенда завершился.\n' >&2
else
  printf 'Один из процессов стенда завершился с ошибкой.\n' >&2
fi
exit 1
