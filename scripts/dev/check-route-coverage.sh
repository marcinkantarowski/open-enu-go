#!/usr/bin/env bash
# =============================================================================
#  Every operation in the spec was reached by a test - measured, not grepped.
# =============================================================================
#  `make test` runs every suite with ROUTE_TRACE set; the kernel's request
#  chain appends the matched pattern of each request to that file
#  (kernel/httpx/trace.go). Afterwards `openenu route-coverage` subtracts the
#  trace from the spec: spec minus hit must be empty, exemptions aside.
#
#  Grepping the tests for paths would measure whether somebody wrote the path
#  down. This measures what actually ran - and notices a deleted test, which a
#  grep over source cannot.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/go.sh"

TRACE="${ROUTE_TRACE:-$ROOT/backend/.route-trace}"

log_step "Route coverage"
if [ ! -s "$TRACE" ]; then
  # An absent trace is not "nothing to cover": it means the suites did not run
  # with tracing on, and crediting that as coverage would hide every gap.
  log_fail "no trace at $TRACE" "run: make test  (it writes the trace), then make route-coverage"
  exit 1
fi

if out="$(cd "$ROOT/backend" && "$GO" run ./cmd/openenu route-coverage "$TRACE" 2>&1)"; then
  printf '%s\n' "$out" | sed 's/^/  /'
  log_ok "every operation was reached by a test"
else
  printf '%s\n' "$out" | sed 's/^/  /' >&2
  log_fail "operations without a test" "the list above names each one and what to do"
  exit 1
fi
