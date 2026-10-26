#!/usr/bin/env bash
# =============================================================================
#  Start or stop THIS checkout's stack. What the systemd unit runs at boot.
# =============================================================================
#  A script rather than a compose command inline in the unit: the compose files
#  differ per stage (COMPOSE_FILE in .env), and the edge has to be attached
#  again after the stack is up - neither fits in an ExecStart line without
#  systemd's own escaping rules for `$` getting in the way.
#
#  Usage: stack.sh up|stop
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"
compose_init "$ROOT"

case "${1:-}" in
  up)
    bash "$ROOT/scripts/dev/edge.sh" up || exit 1
    "${COMPOSE[@]}" up -d --remove-orphans || exit 1
    bash "$ROOT/scripts/dev/edge.sh" attach
    ;;
  stop)
    "${COMPOSE[@]}" stop
    ;;
  *) die "usage: stack.sh up|stop" ;;
esac
