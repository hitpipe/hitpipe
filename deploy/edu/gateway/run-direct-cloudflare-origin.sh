#!/usr/bin/env bash
# Launch only the alternate connector-only listener. Domain/Tunnel records are external.
set -euo pipefail
if [[ $# != 4 ]]; then
    echo 'Usage: run-direct-cloudflare-origin.sh STAGED_GATEWAY INCIDENT_DIR CONNECTOR_IPV4 CA_PEM' >&2
    exit 2
fi
gateway=$(realpath "$1")
incident=$(realpath "$2")
connector="$3"
ca=$(realpath "$4")
cd "$gateway"
printf '%s  gateway-direct.sha256\n' 33c8fc37b391bd365101834fe7a5c1c0ae79ce97cb9be1979b3c0205a85b8de3 |
    sha256sum --strict --status --check
sha256sum --strict --quiet --check gateway-direct.sha256
exec /usr/bin/python3 -B "$gateway/school_public_origin.py" \
    --connector-ip "$connector" \
    --port 19485 \
    --app-port 19446 \
    --ca-file "$ca" \
    --require-cloudflare-https \
    --guest-stage "$incident/guest-no-js-cookie-stage" \
    --guest-manifest "$incident/guest-no-js-cookie-manifest.json" \
    --guest-manifest-sha256 97151300b2d8b3b551ea47f6df27cadf0f10e3add31daa3bc79e3a2f35f97615
