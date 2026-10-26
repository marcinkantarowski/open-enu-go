#!/usr/bin/env bash
# =============================================================================
#  ui-kit/types/api.d.ts - fresh against backend/openapi.json, checked without Node.
# =============================================================================
#  `make types` generates the file with openapi-typescript (in the frontend
#  container) and then runs this script with --stamp, which appends the spec's
#  sha256 as the file's last line. Without --stamp it compares that line with
#  the spec as it is now: a spec that changed since the last `make types` fails
#  here, in `make gen-check`, with no container and no npm.
#
#  Usage: check-types.sh            check
#         check-types.sh --stamp    append the stamp (make types only)
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

SPEC="$ROOT/backend/openapi.json"
TYPES="$ROOT/ui-kit/types/api.d.ts"
[ -f "$SPEC" ] || die "backend/openapi.json is missing" "run: cd backend/cmd/openenu && go generate"
[ -f "$TYPES" ] || die "ui-kit/types/api.d.ts is missing" "run: make types"

sum="$(python3 -c 'import hashlib, sys; print(hashlib.sha256(open(sys.argv[1], "rb").read()).hexdigest())' "$SPEC")" \
  || die "could not hash backend/openapi.json"
[ -n "$sum" ] || die "hashing backend/openapi.json produced nothing"
STAMP="// generated from backend/openapi.json sha256:$sum"

if [ "${1:-}" = "--stamp" ]; then
  printf '%s\n' "$STAMP" >> "$TYPES"
  log_ok "ui-kit/types/api.d.ts stamped with the spec's hash"
  exit 0
fi

last="$(tail -n 1 "$TYPES")"
if [ "$last" = "$STAMP" ]; then
  log_ok "ui-kit/types/api.d.ts matches backend/openapi.json"
  exit 0
fi
case "$last" in
  "// generated from backend/openapi.json sha256:"*)
    die "ui-kit/types/api.d.ts is stale - backend/openapi.json changed since it was generated" "run: make types" ;;
  *)
    die "ui-kit/types/api.d.ts carries no stamp - it was not written by make types" "run: make types (never edit the file by hand)" ;;
esac
