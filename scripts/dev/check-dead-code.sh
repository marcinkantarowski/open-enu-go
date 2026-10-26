#!/usr/bin/env bash
# =============================================================================
#  Dead code - blocking for TypeScript and for the application's Go, advisory
#  for the kernel's exported surface.
# =============================================================================
#  The failure mode is ABANDONED SCAFFOLDING: a composable replaced but not
#  deleted, a seam exported for a test that was rewritten, a dependency added
#  for an approach that was dropped. None of it breaks anything, which is
#  precisely why it survives - and why the next agent reads it as an example.
#
#  TypeScript: knip, blocking. The Nuxt conventions it cannot see are declared
#  once in knip.config.ts, each with the reason attached.
#
#  Go: golang.org/x/tools/cmd/deadcode over both modules at once, from the
#  application's roots (cmd/openenu and every test). Deterministic, so a
#  finding in internal/ or cmd/ is a failing build. A finding in the kernel is
#  printed and does not fail it: the kernel is a library with an upgrade
#  contract, and an export this application does not reach is reachable from
#  the next project built on it. Generated files (*.gen.go) are not reported;
#  oapi-codegen emits helpers nobody is expected to call.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/go.sh"
. "$ROOT/scripts/lib/compose.sh"; compose_init "$ROOT"
CO=("${COMPOSE[@]}")
rc=0

# ── Go ──────────────────────────────────────────────────────────────────────
log_step "Dead code - Go (deadcode)"
raw="$(cd "$ROOT/backend" && "$GO" tool deadcode -test -filter '^github.com/open-enu/' ./... 2>&1)"
status=$?
if [ "$status" -ne 0 ] && [ -z "$raw" ]; then
  # "The tool produced no output" is not "the code is all alive".
  log_fail "deadcode could not run (exit $status)" "try: cd backend && go tool deadcode -test ./..."
  exit 1
fi
findings="$(printf '%s\n' "$raw" | grep -v '\.gen\.go:' | grep -v '^$' || true)"
app="$(printf '%s\n' "$findings" | grep -v '^kernel/' || true)"
lib="$(printf '%s\n' "$findings" | grep '^kernel/' || true)"
if [ -n "$app" ]; then
  log_fail "unreachable Go in the application" "delete it - nothing calls it, and the next reader will copy it"
  printf '%s\n' "$app" | sed 's/^/    /' >&2
  rc=1
else
  log_ok "no unreachable Go in internal/ or cmd/"
fi
if [ -n "$lib" ]; then
  log_warn "$(printf '%s\n' "$lib" | wc -l | tr -d ' ') kernel export(s) this application does not reach (advisory - a library's surface)"
  printf '%s\n' "$lib" | sed 's/^/    /'
fi

# ── TypeScript ──────────────────────────────────────────────────────────────
log_step "Dead code - TypeScript (knip)"
out="$(mktemp)"; trap 'rm -f "$out"' EXIT
# `--no-config-hints`: knip suggests tightening patterns that match nothing
# yet, and several of ours describe a convention a module may not use today.
if "${CO[@]}" run --rm --no-deps --entrypoint sh frontend \
     -c 'npx knip --no-progress --no-config-hints' >"$out" 2>&1; then
  log_ok "no unused files, exports or dependencies"
else
  log_fail "knip found dead TypeScript" \
    "delete it, or - if a framework reaches it by convention - declare that convention in knip.config.ts"
  grep -v '^npm notice' "$out" | grep -v '^ Container' | head -40 >&2
  rc=1
fi
exit $rc
