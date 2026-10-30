#!/usr/bin/env bash
# =============================================================================
#  make ci - the full gate, one stage at a time.
# =============================================================================
#  At a terminal each stage is a line with its time, the line under it shows
#  what the stage is printing right now, and the rest goes to a log, kept and
#  named when a stage fails. Anywhere else (CI, a pipe, a file) everything is
#  printed. The gate stops at the first red stage, and says which.
#
#  A green gate is recorded with the content it saw (scripts/lib/ci-stamp.sh),
#  unless that content changed while it ran.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/ci-stamp.sh"

STAGES=(
  "check-tools|the toolchain"
  "arch|guardrails, docs, translations, budgets"
  "selftest|every guardrail fails on a broken copy"
  "test|every Go test, with Postgres and Redis"
  "route-coverage|every operation was reached by a test"
  "typecheck|the three Nuxt apps"
  "smoke|every URL over HTTPS"
  "e2e|browser tests on their own database"
)

took() { printf '%dm%02ds' $(($1 / 60)) $(($1 % 60)); }

# live <log> - the current line under the stage, overwritten by the next.
live() {
  local cols line
  cols="$(tput cols 2>/dev/null || echo 100)"
  while IFS= read -r line; do
    printf '%s\n' "$line" >>"$1"
    line="${line//$'\t'/ }"
    printf '\r\033[K%s    %s%s' "$C_DIM" "${line:0:$((cols - 5))}" "$C_RESET"
  done
  printf '\r\033[K'
}

LOG=""
if [ -t 1 ]; then
  LOG="$(mktemp)"
  log_info "details: $LOG"
fi

ci_stamp_clear "$ROOT"
TREE="$(ci_tree "$ROOT")"

n=0
for stage in "${STAGES[@]}"; do
  target="${stage%%|*}"; n=$((n + 1)); began=$SECONDS
  log_step "[$n/${#STAGES[@]}] $target - ${stage#*|}"
  if [ -n "$LOG" ]; then
    "${MAKE:-make}" --no-print-directory -C "$ROOT" "$target" </dev/null 2>&1 | live "$LOG"
    status=${PIPESTATUS[0]}
  else
    "${MAKE:-make}" --no-print-directory -C "$ROOT" "$target"
    status=$?
  fi
  if [ "$status" -ne 0 ]; then
    [ -z "$LOG" ] || tail -n 40 "$LOG"
    log_fail "make ci stopped at [$n/${#STAGES[@]}] $target after $(took $((SECONDS - began)))" \
      ${LOG:+"the whole output: $LOG"} "alone: make $target" || true
    exit 1
  fi
  log_ok "$target $(took $((SECONDS - began)))"
done

[ -z "$LOG" ] || rm -f "$LOG"
[ -z "$TREE" ] || [ "$TREE" != "$(ci_tree "$ROOT")" ] || ci_stamp_write "$ROOT" "$TREE"
log_ok "make ci passed in $(took "$SECONDS")"
