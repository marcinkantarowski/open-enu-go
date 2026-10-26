#!/usr/bin/env bash
# The URL table for a remote environment. Same shape as `make urls` in dev, so
# the thing you read after a deploy looks like the thing you read after builddev.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

DOMAIN="${1:-}"
[ -n "$DOMAIN" ] || exit 0

printf '\n'
printf '  %-38s %s\n' "https://$DOMAIN"            "landing"
printf '  %-38s %s\n' "https://app.$DOMAIN"        "tenant application"
printf '  %-38s %s\n' "https://manager.$DOMAIN"    "operator console"
printf '  %-38s %s\n' "https://api.$DOMAIN/health" "API"
printf '  %-38s %s\n' "https://api.$DOMAIN/.well-known/mercure" "realtime hub"
printf '\n'
