#!/usr/bin/env bash
# The "you are done" screen.
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/os.sh"
D="$(sed -n 's/^DOMAIN=//p' "$ROOT/.env" | head -1)"

printf '\n  %sThe stack is up.%s\n\n' "$C_BOLD$C_GRN" "$C_RESET"
row() { printf '    %-34s %s\n' "$1" "$2"; }
row "https://$D"            "landing"
row "https://app.$D"        "tenant application"
row "https://manager.$D"    "operator console"
row "https://api.$D/health" "API"
row "https://api.$D/.well-known/mercure" "realtime hub"
printf '\n'
row "https://traefik.$D"    "Traefik dashboard (dev only)"
row "https://mail.$D"       "Mailpit - every outbound mail lands here"
printf '\n    %smake logs%s follows everything · %smake status%s shows container health\n\n' \
  "$C_DIM" "$C_RESET" "$C_DIM" "$C_RESET"

if is_wsl2; then
  printf '    %sWSL2:%s the browser resolves via the Windows hosts file and trusts the\n' "$C_DIM" "$C_RESET"
  printf '    mkcert CA through the Windows store - both were set up by this build.\n\n'
fi
