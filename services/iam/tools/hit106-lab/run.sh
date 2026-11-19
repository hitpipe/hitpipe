#!/usr/bin/env bash
set -euo pipefail
umask 077

if [[ ${HIT106_ISOLATED_LAB:-} != 1 ]]; then
  echo 'Launch only in a disposable guest with HIT106_ISOLATED_LAB=1.' >&2
  exit 1
fi

source_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
iam_dir=$(cd -- "$source_dir/../.." && pwd)
go_bin=${HIT106_GO:-go}
for tool in docker node openssl curl certutil "$go_bin"; do command -v "$tool" >/dev/null; done
test -d "$source_dir/node_modules/playwright"

HIT106_LAB_DIR=$(mktemp -d "${TMPDIR:-/tmp}/hit106-lab.XXXXXXXX")
export HIT106_LAB_DIR
docker=(docker --host unix:///var/run/docker.sock)
compose=("${docker[@]}" compose --env-file /dev/null -p hit106-browser-lab -f "$source_dir/compose.yaml")
compose_started=0
loopback_pid=
cleanup() {
  if [[ -n $loopback_pid ]]; then
    kill "$loopback_pid" 2>/dev/null || true
    wait "$loopback_pid" 2>/dev/null || true
  fi
  if [[ $compose_started == 1 ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  case $HIT106_LAB_DIR in
    "${TMPDIR:-/tmp}"/hit106-lab.????????) rm -rf -- "$HIT106_LAB_DIR" ;;
    *) return 1 ;;
  esac
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

for secret_name in HIT106_IAM_ADMIN_PASSWORD HIT106_IAM_MIGRATOR_PASSWORD HIT106_IAM_APP_PASSWORD \
  HIT106_IDP_DB_PASSWORD HIT106_IDP_ADMIN_PASSWORD HIT106_CLIENT_SECRET; do
  secret_value=$(openssl rand -hex 32)
  export "$secret_name=$secret_value"
done
HIT106_EMAIL_CODE_KEY=$(openssl rand -base64 32)
HIT106_EMAIL_RATE_KEY=$(openssl rand -base64 32)
export HIT106_EMAIL_CODE_KEY HIT106_EMAIL_RATE_KEY
unset secret_name secret_value

prior=$("${compose[@]}" ps --all -q)
if [[ -n $prior ]]; then
  echo 'A previous lab project still has containers; inspect it before starting.' >&2
  exit 1
fi

mkdir -m 0755 "$HIT106_LAB_DIR/realm-import" "$HIT106_LAB_DIR/idp-tls"
node "$source_dir/realm.mjs" "$iam_dir/keycloak/realm/hit105-test-realm.json" \
  "$HIT106_LAB_DIR/realm-import/hit106-test-realm.json"

openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=HitPipe disposable lab CA' \
  -addext 'basicConstraints=critical,CA:TRUE,pathlen:0' -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -keyout "$HIT106_LAB_DIR/ca.key" -out "$HIT106_LAB_DIR/ca.pem" >/dev/null 2>&1
make_leaf() {
  local path=$1 label=$2 ip=$3
  cat > "$HIT106_LAB_DIR/leaf.ext" <<EXT
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=IP:$ip
EXT
  openssl req -new -newkey rsa:2048 -nodes -sha256 -subj "/CN=$label" \
    -keyout "$path.key" -out "$HIT106_LAB_DIR/leaf.csr" >/dev/null 2>&1
  openssl x509 -req -sha256 -days 1 -in "$HIT106_LAB_DIR/leaf.csr" \
    -CA "$HIT106_LAB_DIR/ca.pem" -CAkey "$HIT106_LAB_DIR/ca.key" -CAcreateserial \
    -extfile "$HIT106_LAB_DIR/leaf.ext" -out "$path.pem" >/dev/null 2>&1
  openssl verify -CAfile "$HIT106_LAB_DIR/ca.pem" -verify_ip "$ip" "$path.pem" >/dev/null
}
make_leaf "$HIT106_LAB_DIR/server" 'HitPipe disposable IAM' 127.0.0.2
make_leaf "$HIT106_LAB_DIR/idp-tls/idp" 'HitPipe disposable Keycloak' 127.0.0.1
chmod 0444 "$HIT106_LAB_DIR/idp-tls/"{idp.pem,idp.key}

browser_home="$HIT106_LAB_DIR/browser-home"
browser_cache=${PLAYWRIGHT_BROWSERS_PATH:-$HOME/.cache/ms-playwright}
for database in "$browser_home/.pki/nssdb" "$browser_home/.local/share/pki/nssdb"; do
  mkdir -p "$database"
  certutil -N -d "sql:$database" --empty-password
  certutil -A -d "sql:$database" -n 'HitPipe disposable lab CA' -t 'C,,' -i "$HIT106_LAB_DIR/ca.pem"
done

(cd -- "$iam_dir" && "$go_bin" build -tags hit106lab -o "$HIT106_LAB_DIR/iam-browser" ./tools/hit106-server)
"${compose[@]}" config --quiet
compose_started=1
"${compose[@]}" up --build -d

address() {
  local id
  id=$("${compose[@]}" ps --all -q "$1")
  [[ -n $id ]]
  "${docker[@]}" inspect --format \
    '{{with index .NetworkSettings.Networks "hit106-browser-lab_lab"}}{{.IPAddress}}{{end}}' "$id"
}

node "$source_dir/loopback.mjs" "$(address idp)" "$(address smtp)" "$(address iam-db)" &
loopback_pid=$!
ready=0
for _ in {1..60}; do
  if ! kill -0 "$loopback_pid" 2>/dev/null; then break; fi
  if curl --silent --fail --max-time 2 --cacert "$HIT106_LAB_DIR/ca.pem" -o /dev/null \
    https://127.0.0.1:18443/realms/hit106-test/.well-known/openid-configuration \
    && curl --silent --fail --max-time 2 -o /dev/null http://127.0.0.1:18425/readyz; then
    ready=1
    break
  fi
  sleep 2
done
if [[ $ready != 1 ]]; then
  echo 'The HTTPS realm did not become ready with verified CA and SAN, or the SMTP receiver did not become ready.' >&2
  "${compose[@]}" ps --all >&2
  exit 1
fi

(cd -- "$iam_dir" && "$HIT106_LAB_DIR/iam-browser" migrate)
"${compose[@]}" exec -T idp /opt/keycloak/bin/kc.sh --version
for database in idp-db iam-db; do
  "${compose[@]}" exec -T "$database" /bin/sh -c 'postgres --version'
done
"${compose[@]}" exec -T smtp /mailpit version --no-release-check
node --version

HOME="$browser_home" XDG_CONFIG_HOME="$browser_home/.config" XDG_DATA_HOME="$browser_home/.local/share" \
  PLAYWRIGHT_BROWSERS_PATH="$browser_cache" NODE_EXTRA_CA_CERTS="$HIT106_LAB_DIR/ca.pem" node "$source_dir/browser.mjs"
