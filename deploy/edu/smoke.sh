#!/usr/bin/env bash
set -euo pipefail
umask 077
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
state=${HITPIPE_SCHOOL_STATE:?Set the school runtime directory}
node=${HITPIPE_NODE:?Set the pinned Node executable}
config="$state/current/bridge/browser-config.json"
case ${1:-browser} in
  browser)
    exec "$node" "$here/browser/run-browser.mjs" "$config"
    ;;
  persisted-case)
    exec "$node" "$here/verify-case.mjs" "$config" "${2:?Pass the earlier browser-proof.json}"
    ;;
  stripe)
    command=${2:-status}
    case "$command" in
      paid|replay|unpaid|async-paid|status) ;;
      *) printf 'Допустимо paid, replay, unpaid, async-paid или status.\n' >&2; exit 2 ;;
    esac
    export HITPIPE_BROWSER_SHARE
    HITPIPE_BROWSER_SHARE=$(realpath -- "$state/current/bridge")
    exec "${HITPIPE_SCHOOL_EVENT:?Set the school event launcher}" "$command"
    ;;
  *)
    printf 'Режимы проверки — browser, persisted-case и stripe.\n' >&2
    exit 2
    ;;
esac
