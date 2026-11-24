#!/usr/bin/env bash
# Run the isolated, stateful education profile with real service executables.
set -euo pipefail
umask 077
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
demo=${HITPIPE_DEMO_SOURCE:?Point to the unpacked, verified school source archive}
state=${HITPIPE_SCHOOL_STATE:?Set an absolute 0700 state directory}
share=${HITPIPE_BROWSER_SHARE:?Set an absolute 0700 browser coordination directory}
instance=${HITPIPE_SCHOOL_INSTANCE:-vps}
workspace_file=${HITPIPE_SCHOOL_WORKSPACE_FILE:-}
default_workspace=11111111-1111-4111-8111-111111111111
[[ "$instance" =~ ^[a-z][a-z0-9-]{0,15}$ ]] || { printf 'Invalid school instance name.\n' >&2; exit 2; }
private_dir() {
  [[ "$1" == /* && -d "$1" && ! -L "$1" && $(stat -c '%a:%u' -- "$1") == "700:$(id -u)" ]]
}
private_dir "$state" && private_dir "$share" ||
  { printf 'School state and share must already exist as private 0700 directories owned by the current user.\n' >&2; exit 2; }
[[ $(realpath -- "$state") != "$(realpath -- "$share")" ]] ||
  { printf 'School state and browser share must be separate directories.\n' >&2; exit 2; }
exec 9>"$state/school.lock"
flock -n 9 || { printf 'This school state already has a supervisor.\n' >&2; exit 3; }
go_bin=$(command -v "${1:-go}") || { printf 'Specify Go 1.27.2.\n' >&2; exit 2; }
[[ $("$go_bin" version) == "go version go1.27.2 "* ]] ||
  { printf 'Go 1.27.2 is required.\n' >&2; exit 2; }
image=docker.io/library/postgres@sha256:75dcac3dfb1cd01ca3536910bbc84128759362f8b118bf8c55b38a94f68f3b73
docker image inspect "$image" >/dev/null ||
  { printf 'The pinned PostgreSQL 18.7 image must be available locally.\n' >&2; exit 3; }
intake="$demo/intake-candidate"
source="$demo/src-processing/services"
delivery="$demo/delivery-stand/services/delivery"
bin="$state/bin"
mkdir -p -m 0700 "$bin"
private_dir "$bin" || exit 2

scratch=$(mktemp -d "$state/build.XXXXXX")
container="hitpipe-school-pg-$instance"
volume="hitpipe-school-pgdata-$instance"
created=
cleanup() {
  if [[ -n "$created" ]]; then
    if [[ -f "$state/backend.pid" ]] && read -r backend_pid <"$state/backend.pid" &&
       [[ "$backend_pid" =~ ^[0-9]+$ && "$backend_pid" -gt 1 ]]; then
      docker exec -u "$(id -u):$(id -g)" "$container" sh -c \
        '[ "$(readlink "/proc/$1/exe")" = /school/state/bin/school-backend ] && kill -TERM "$1"' \
        school "$backend_pid" >/dev/null 2>&1 || true
      if [[ -n "${running:-}" ]]; then
        for ((grace=0; grace<16; grace++)); do
          kill -0 "$running" 2>/dev/null || break
          sleep 0.5
        done
      fi
    fi
    docker stop -t 8 "$container" >/dev/null 2>&1 || true
    if [[ -n "${running:-}" ]]; then wait "$running" || true; fi
  fi
  rm -rf -- "$scratch"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [[ ${HITPIPE_SCHOOL_REUSE_BINARIES:-0} == 1 ]]; then
  (cd "$bin" && sha256sum --status --check binaries.sha256) ||
    { printf 'School binaries are absent or do not match their build receipt.\n' >&2; exit 3; }
else
  mkdir -m 0700 "$scratch/compile" "$scratch/go"
  export GOTOOLCHAIN=local GOMAXPROCS=1 CGO_ENABLED=0 GOFLAGS='-p=1 -buildvcs=false'
  export GOCACHE=${GOCACHE:-"$state/go-cache"} GOMODCACHE=${GOMODCACHE:-"$state/go-mod"}
  export GOTMPDIR="$scratch/compile"
  cp -a -- "$source/processing" "$source/platform" "$scratch/go/"
  patch --batch --forward -s -d "$scratch/go/processing" -p1 <"$here/processing-school-source-facts.patch"
  mkdir -p "$scratch/go/processing/cmd/school-backend"
  cp -- "$here"/cmd/school-backend/*.go "$scratch/go/processing/cmd/school-backend/"
  cp -a -- "$intake" "$scratch/intake-source"
  patch --batch --forward -s -d "$scratch/intake-source" -p1 <"$here/intake-qualification-workspace.patch"
  "$go_bin" -C "$scratch/intake-source" build -ldflags '-s -w' -o "$scratch/intake" ./cmd/intake
  "$go_bin" -C "$scratch/go/processing" build -tags hitpipe_school -ldflags '-s -w -X main.revision=dd1a36f6529f57d7452a27b615b6cad18b1288bd' -o "$scratch/processing" ./cmd/processing
  cp -a -- "$delivery" "$scratch/delivery"
  patch --batch --forward -s -d "$scratch/delivery" -p1 <"$here/persistent-qualification-worker.patch"
  "$go_bin" -C "$scratch/delivery" build -ldflags '-s -w' -o "$scratch/delivery-offline-receipt" ./cmd/delivery-offline-receipt
  "$go_bin" -C "$scratch/go/processing" build -ldflags '-s -w' -o "$scratch/school-backend" ./cmd/school-backend
  for executable in intake processing delivery-offline-receipt school-backend; do
    mv -- "$scratch/$executable" "$bin/$executable"
  done
  (cd "$bin" && sha256sum intake processing delivery-offline-receipt school-backend > binaries.sha256)
fi

workspace=$default_workspace
container_workspace_file=
if [[ -n "$workspace_file" || -e "$state/workspace-id" || -L "$state/workspace-id" ]]; then
  if [[ "$workspace_file" != "$state/workspace-id" ]]; then
    printf 'Specify the owner-provisioned school workspace file in the instance state directory.\n' >&2
    exit 3
  fi
  workspace=$(HITPIPE_SCHOOL_STATE="$state" HITPIPE_SCHOOL_WORKSPACE_FILE="$workspace_file" \
    "$bin/school-backend" --resolve-school-workspace) ||
    { printf 'Workspace provisioning requires the matching school supervisor and private configuration.\n' >&2; exit 3; }
  container_workspace_file=/school/state/workspace-id
elif [[ -e "$state/workspace-bound-id" || -L "$state/workspace-bound-id" ]]; then
  bound="$state/workspace-bound-id"
  [[ -f "$bound" && ! -L "$bound" && $(stat -c '%a:%u' "$bound") == "600:$(id -u)" && $(cat "$bound") == "$default_workspace" ]] ||
    { printf 'The existing school state requires its explicit workspace configuration.\n' >&2; exit 3; }
fi
[[ "$workspace" =~ ^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$ ]] || exit 3

if ! docker volume inspect "$volume" >/dev/null 2>&1; then
  docker volume create --label "dev.hitpipe.school.instance=$instance" \
    --label "dev.hitpipe.school.workspace=$workspace" "$volume" >/dev/null
fi
[[ "$(docker volume inspect -f '{{index .Labels "dev.hitpipe.school.instance"}}' "$volume")" == "$instance" ]] ||
  { printf 'Refusing to use a volume outside the school stack.\n' >&2; exit 3; }
volume_workspace=$(docker volume inspect -f '{{with index .Labels "dev.hitpipe.school.workspace"}}{{.}}{{end}}' "$volume")
[[ "$volume_workspace" == "$workspace" || ( -z "$volume_workspace" && "$workspace" == "$default_workspace" ) ]] ||
  { printf 'Refusing to use a volume provisioned for a different school workspace.\n' >&2; exit 3; }

# The state lock is held. Recover only a container from this exact runtime
# directory and volume, for example after the supervisor was killed mid-stop.
if previous=$(docker container inspect -f '{{.Id}} {{index .Config.Labels "dev.hitpipe.school.instance"}} {{.HostConfig.NetworkMode}} {{.HostConfig.AutoRemove}} {{range .Mounts}}{{if eq .Destination "/school/state"}}{{.Source}}{{end}}{{end}} {{range .Mounts}}{{if eq .Destination "/var/lib/postgresql"}}{{.Name}}{{end}}{{end}}' "$container" 2>/dev/null); then
  previous_id=${previous%% *}
  if [[ "$previous" != "$previous_id $instance none true $state $volume" || ! "$previous_id" =~ ^[0-9a-f]{64}$ ]]; then
    printf 'Refusing to stop a container outside the school state directory.\n' >&2
    exit 3
  fi
  docker stop -t 12 "$previous_id" >/dev/null
  for ((i=0; i<20; i++)); do
    docker container inspect "$previous_id" >/dev/null 2>&1 || break
    sleep 0.5
  done
fi

docker run -d --rm --name "$container" --network none --memory 416m --cpus 1 --pids-limit 96 \
  --label "dev.hitpipe.school.instance=$instance" \
  --mount "type=volume,source=$volume,target=/var/lib/postgresql" \
  --mount "type=bind,source=$state,target=/school/state" \
  --mount "type=bind,source=$share,target=/school/bridge" \
  -e POSTGRES_HOST_AUTH_METHOD=trust "$image" postgres -c listen_addresses=127.0.0.1 \
  -c shared_buffers=24MB -c max_connections=35 >/dev/null
created=1
for ((i=0; i<45; i++)); do
  if docker exec "$container" pg_isready -q -h 127.0.0.1 -U postgres; then break; fi
  sleep 1
done
sql() { docker exec -i "$container" psql -X -v ON_ERROR_STOP=1 "$@"; }
version=$(sql -U postgres -d postgres -Atc 'SHOW server_version; SHOW fsync; SHOW full_page_writes; SHOW synchronous_commit;')
[[ "$version" == $'18.7\non\non\non' ]] ||
  { printf 'The private database must have PostgreSQL 18.7 and durable WAL settings.\n' >&2; exit 3; }
ensure_role() {
  local name=$1 flags=$2
  if [[ $(sql -U postgres -d postgres -Atc "SELECT count(*) FROM pg_roles WHERE rolname='$name'") == 0 ]]; then
    sql -U postgres -d postgres -c "CREATE ROLE $name $flags NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS"
  fi
}
ensure_db() {
  local name=$1 owner=$2
  if [[ $(sql -U postgres -d postgres -Atc "SELECT count(*) FROM pg_database WHERE datname='$name'") == 0 ]]; then
    sql -U postgres -d postgres -c "CREATE DATABASE $name OWNER $owner"
  fi
}
if [[ ! -f "$state/db-bootstrapped-v1" ]]; then
  for role in migrator app worker purge; do ensure_role "hp_intake_$role" LOGIN; done
  ensure_db intake_test hp_intake_migrator
  sql -U postgres -d intake_test -c 'REVOKE ALL ON DATABASE intake_test FROM PUBLIC; GRANT CONNECT ON DATABASE intake_test TO hp_intake_app,hp_intake_worker,hp_intake_purge; REVOKE ALL ON SCHEMA public FROM PUBLIC;'
  if [[ $(sql -U postgres -d intake_test -Atc "SELECT to_regclass('hp_intake.source_connections') IS NULL") == t ]]; then
    sql -U hp_intake_migrator -d intake_test <"$intake/migrations/0001_intake.sql"
  fi

  ensure_role hp_processing_owner NOLOGIN
  ensure_role hp_processing_migrator 'LOGIN NOINHERIT'
  for role in app worker purge; do ensure_role "hp_processing_$role" LOGIN; done
  sql -U postgres -d postgres -c 'GRANT hp_processing_owner TO hp_processing_migrator'
  ensure_db hit78 hp_processing_owner
  sql -U postgres -d hit78 -c 'REVOKE ALL ON DATABASE hit78 FROM PUBLIC; GRANT CONNECT ON DATABASE hit78 TO hp_processing_migrator,hp_processing_app,hp_processing_worker,hp_processing_purge;'
  docker exec "$container" mkdir -p /tmp/processing-migrations
  tar -C "$source/processing/scripts/migrations" -cf - . | docker exec -i "$container" tar -C /tmp/processing-migrations -xf -
  if [[ $(sql -U postgres -d hit78 -Atc "SELECT to_regclass('hp_processing.operations') IS NULL") == t ]]; then
    sql -U hp_processing_migrator -d hit78 -f /tmp/processing-migrations/apply.sql
  fi

  for role in migrator app worker purge; do ensure_role "hp_delivery_$role" LOGIN; done
  ensure_db delivery_demo hp_delivery_migrator
  sql -U postgres -d delivery_demo -c 'REVOKE ALL ON DATABASE delivery_demo FROM PUBLIC; GRANT CONNECT ON DATABASE delivery_demo TO hp_delivery_app,hp_delivery_worker,hp_delivery_purge; REVOKE ALL ON SCHEMA public FROM PUBLIC;'
  if [[ $(sql -U postgres -d delivery_demo -Atc "SELECT to_regclass('hp_delivery.destination_connections') IS NULL") == t ]]; then
    sql -U hp_delivery_migrator -d delivery_demo <"$delivery/migrations/001_delivery.sql"
  fi
  printf '%s\n' "$instance" >"$state/db-bootstrapped-v1"
fi
[[ $(cat "$state/db-bootstrapped-v1") == "$instance" ]] ||
  { printf 'The school state already belongs to another instance.\n' >&2; exit 3; }

docker exec -u "$(id -u):$(id -g)" \
  -e GOMAXPROCS=1 -e HITPIPE_SCHOOL_STATE=/school/state -e HITPIPE_BROWSER_SHARE=/school/bridge \
  -e "HITPIPE_SCHOOL_WORKSPACE_FILE=$container_workspace_file" -e "HITPIPE_SCHOOL_EXPECTED_WORKSPACE_ID=$workspace" \
  -e HITPIPE_SCHOOL_BIN=/school/state/bin "$container" /school/state/bin/school-backend &
running=$!
wait "$running"
