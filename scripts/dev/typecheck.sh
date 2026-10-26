#!/usr/bin/env bash
# =============================================================================
#  Type-check all three Nuxt apps.
# =============================================================================
#  The point is not style. API types are GENERATED from openapi.json (ADR-0006),
#  so a backend DTO that loses a field becomes a type error in the component that
#  reads it - at build time, in CI, rather than as `undefined` in a browser.
#
#  Run in the node container so the Node version and the installed modules are
#  the ones the apps actually run with.
#
#  Expect this line, several times, and ignore it:
#
#    [Vue] Resolve plugin path failed: vue-router/volar/sfc-route-blocks
#
#  Nuxt's generated tsconfig registers a vue-tsc plugin that vue-router 4 does
#  not export. It is upstream, it is harmless, and the check still reports real
#  type errors - left visible rather than filtered, because silencing somebody
#  else's warning is how the next real one gets missed.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

. "$ROOT/scripts/lib/compose.sh"; compose_init "$ROOT"
CO=("${COMPOSE[@]}")
fails=0

log_step "Nuxt type-check"

for app in frontend manager landing; do
  if "${CO[@]}" run --rm --no-deps --entrypoint sh frontend -c "npm run --silent -w $app typecheck"; then
    log_ok "$app"
  else
    log_fail "$app does not type-check" \
      "if the errors are about API shapes, the generated types are stale: make types" || true
    fails=$((fails + 1))
  fi
done

echo
[ "$fails" -eq 0 ] && { log_ok "all three apps type-check"; exit 0; }
log_fail "$fails app(s) failed type-checking"
exit 1
