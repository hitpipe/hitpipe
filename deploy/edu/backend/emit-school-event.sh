#!/usr/bin/env bash
# Send a signed event through, or read status from, a private Unix socket.
set -euo pipefail
share=${HITPIPE_BROWSER_SHARE:?Set HITPIPE_BROWSER_SHARE to the backend coordination directory}
[[ $# == 1 ]] || { printf 'Use status, paid, replay, unpaid or async-paid.\n' >&2; exit 2; }
case "$1" in
  status)
    exec curl --silent --show-error --fail-with-body --max-time 8 --unix-socket "$share/provider.sock" http://localhost/status
    ;;
  paid|replay|unpaid|async-paid)
    exec curl --silent --show-error --fail-with-body --max-time 10 --unix-socket "$share/provider.sock" -X POST "http://localhost/events/$1"
    ;;
  *)
    printf 'Use status, paid, replay, unpaid or async-paid.\n' >&2
    exit 2
    ;;
esac
