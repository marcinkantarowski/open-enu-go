#!/usr/bin/env bash
# =============================================================================
#  make check - the inner loop.
# =============================================================================
#  Budget: 60 seconds. That number is a design constraint, not an aspiration.
#  A check an agent runs after every edit has to be fast enough to actually be
#  run after every edit; past about a minute it gets skipped, and a guardrail
#  nobody runs is a guardrail that does not exist.
#
#  Everything here runs on the host against the native Go toolchain. Nothing
#  needs Docker: tests that need Postgres or Redis start them with
#  testcontainers and are excluded from -short, so they belong to `make ci`.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"
. "$HERE/../lib/go.sh"

[ -n "${GO:-}" ] || die "go is not installed" \
  "install Go 1.27+ from https://go.dev/dl into ~/.local/go or /usr/local/go" \
  "make check-tools lists everything the host needs"

started=$(date +%s)
fails=0
run() { # <label> <command...>
  local label="$1"; shift
  if "$@" >/tmp/check.$$ 2>&1; then
    log_ok "$label"
  else
    log_fail "$label" || true
    sed 's/^/      /' /tmp/check.$$ | tail -25 >&2
    fails=$((fails + 1))
  fi
  rm -f /tmp/check.$$
}

# gofmt -l prints the files it would change and exits 0 either way; the check
# fails on any output, which is the one behaviour `gofmt` does not offer itself.
fmt_check() {
  local out
  out="$("$GOFMT" -l "$@" 2>&1)" || return 1
  [ -z "$out" ] || { printf 'not gofmt-ed:\n%s\n' "$out"; return 1; }
}

log_step "Fast checks"

# Host-side and instant.
run "documentation"     "$HERE/check-docs.sh"
run "translations"      "$HERE/check-i18n.sh"
run "context budgets"   "$HERE/agents-budget.sh"
run "typography"        "$HERE/check-typography.sh"
run "versions"          "$HERE/check-versions.sh"

# The two Go modules, each on its own: the kernel has no dependency on the
# app, so it cannot be reached from the app's `./...` - and that separation is
# the point (ADR-0002).
for mod in backend/kernel backend; do
  (cd "$ROOT/$mod" && run "gofmt       $mod" fmt_check .)
  (cd "$ROOT/$mod" && run "go vet      $mod" "$GO" vet ./...)
  (cd "$ROOT/$mod" && run "go test     $mod" "$GO" test -short ./...)
done 2>&1 | tee /tmp/check-go.$$
# run() counts failures in a subshell above; recount from what it printed.
fails=$(( fails + $(grep -c '✗' /tmp/check-go.$$ || true) ))
rm -f /tmp/check-go.$$

# The architecture guardrails and the freshness of what `go generate` owns.
# Both are cheap once the toolchain cache is warm and both are the kind of
# failure that is cheapest to hear about now.
log_step "Guardrails"
run "analyzers   backend"        bash -c 'cd "$1" && "$2" run github.com/open-enu/kernel/analysis/cmd/arch ./...' _ "$ROOT/backend" "$GO"
run "analyzers   backend/kernel" bash -c 'cd "$1" && "$2" run ./analysis/cmd/arch ./...' _ "$ROOT/backend/kernel" "$GO"
run "generated files are fresh"  bash -c '
  cd "$1/cmd/openenu" && "$2" run ../../tools/genregistry -check && "$2" run ../../tools/genspec -check || exit 1
  cd "$1" && for m in internal/module/*/; do
    [ -f "$m/openapi.json" ] || continue
    (cd "$m" && "$2" run ../../../tools/genapi -check) || exit 1
  done' _ "$ROOT/backend" "$GO"

elapsed=$(( $(date +%s) - started ))
echo
if [ "$fails" -gt 0 ]; then
  log_fail "$fails check(s) failed in ${elapsed}s"
  exit 1
fi

if [ "$elapsed" -gt 60 ]; then
  log_info "a slow inner loop stops being run; treat this as a bug in the harness"
  log_warn "all checks passed, but took ${elapsed}s - over the 60s budget"
else
  log_ok "all checks passed in ${elapsed}s"
fi
