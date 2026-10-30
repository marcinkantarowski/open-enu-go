#!/usr/bin/env bash
# =============================================================================
#  Smoke test - does every host actually answer, over trusted TLS?
# =============================================================================
#  Uses --resolve rather than /etc/hosts so this proves Traefik's routing and
#  the certificate, independently of whether the hosts file has been written.
#  It does NOT pass -k: an untrusted certificate is a failure here, because a
#  browser would refuse it too.
#
#  Every target also asserts a SUBSTRING of the body, not only the status code.
#  This is not belt-and-braces: a server can render a fatal error as HTML with
#  status 200, and a status-only check then reports it fully healthy while the
#  API is failing to boot
#  (.ai/platform/lessons/status-codes-alone-are-not-a-health-check.md). A
#  check that cannot fail is not a check.
#
#  For the three Nuxt apps the substring is the `app-role` meta tag each one
#  declares in `app.head`. Three reasons it is that and not a phrase from the
#  page:
#    • it is in the HTML shell, so it works for the two apps that render on the
#      client rather than on the server;
#    • it identifies WHICH app answered, so three hosts pointing at one
#      container fails here instead of looking perfectly healthy;
#    • it is a role - "tenant", never the project name - because `make init`
#      rewrites the project name, and a check compared against something the
#      tool rewrites inverts itself
#      (.ai/platform/lessons/sentinel-strings-must-not-be-renameable.md).
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/os.sh"

DOMAIN="$(sed -n 's/^DOMAIN=//p' "$ROOT/.env" | head -1)"
HEALTH_TOKEN="$(sed -n 's/^HEALTH_TOKEN=//p' "$ROOT/.env" | head -1)"
CA="$(mkcert -CAROOT 2>/dev/null)/rootCA.pem"
[ -f "$CA" ] || die "mkcert CA not found" "run: make certs"

# The landing site gives the default language the bare URL and every other one
# a prefix, so which URL is "the other language" follows DEFAULT_LOCALE. Checked
# from both sides: the bare URL must be in the default language, and the
# prefixed one in the other - a site that answers every URL in one language
# passes a check that only looks at one of them. And asked for by a browser
# that prefers the other: the bare URL must still answer, not redirect.
DEFAULT_LOCALE="$(sed -n 's/^DEFAULT_LOCALE=//p' "$ROOT/.env" | head -1)"
DEFAULT_LOCALE="${DEFAULT_LOCALE:-en}"
OTHER_LOCALE=pl; [ "$DEFAULT_LOCALE" = pl ] && OTHER_LOCALE=en
LOCALES="$(sed -n 's/^LOCALES=//p' "$ROOT/.env" | head -1)"
OFFERS_OTHER=0; case ",${LOCALES:-en,pl}," in *",$OTHER_LOCALE,"*) OFFERS_OTHER=1 ;; esac

# host | path | status | body must contain | what it is [| request header]
TARGETS=(
  "$DOMAIN|/|200|content=\"marketing\"|landing"
  "www.$DOMAIN|/|200|content=\"marketing\"|landing (www)"
  "$DOMAIN|/|200|lang=\"$DEFAULT_LOCALE\"|landing, default language ($DEFAULT_LOCALE)"
  "$DOMAIN|/|200|lang=\"$DEFAULT_LOCALE\"|landing, / for a browser preferring $OTHER_LOCALE|Accept-Language: $OTHER_LOCALE"
  "$DOMAIN|/robots.txt|200|User-agent:|landing robots.txt"
  "$DOMAIN|/sitemap.xml|200|hreflang=\"x-default\"|landing sitemap"
  "$DOMAIN|/llms.txt|200|## Pages|landing llms.txt"
  "app.$DOMAIN|/|200|content=\"tenant\"|frontend"
  "manager.$DOMAIN|/|200|content=\"operator\"|manager"
  "api.$DOMAIN|/health|200|\"status\":\"ok\"|API health"
  "api.$DOMAIN|/health/deep|200|\"status\":\"ok\"|API readiness|X-Health-Token: $HEALTH_TOKEN"
  "api.$DOMAIN|/.well-known/mercure|400|missing \"match\"|Mercure hub (on the API host)"
  "traefik.$DOMAIN|/dashboard/|200|<!DOCTYPE html>|Traefik dashboard"
  "mail.$DOMAIN|/|200|Mailpit|Mailpit"
)
[ "$OFFERS_OTHER" = 0 ] || TARGETS+=(
  "$DOMAIN|/$OTHER_LOCALE|200|lang=\"$OTHER_LOCALE\"|landing, /$OTHER_LOCALE"
)
# With a token set, the readiness probe must also refuse a caller without it.
# Without one it answers private addresses, and through the dev edge every
# caller is one, so there is nothing to refuse.
[ -z "$HEALTH_TOKEN" ] || TARGETS+=("api.$DOMAIN|/health/deep|404|\"not_found\"|API readiness refused without the token")
# Two expectations that look wrong and are not:
#   Mercure answers 400 to a subscribe with no matcher - that is the hub replying,
#   which is the thing being tested. A 502 would mean it is not there at all.
#   Traefik redirects / to /dashboard/, so the dashboard is probed at its real path.

TIMEOUT="${SMOKE_TIMEOUT:-10}"
fails=0

log_step "Smoke test - https://*.$DOMAIN"
body="$(mktemp)"; trap 'rm -f "$body"' EXIT

for entry in "${TARGETS[@]}"; do
  IFS='|' read -r host path want needle what header <<<"$entry"
  url="https://$host$path"
  got="$(curl -s -o "$body" -w '%{http_code}' ${header:+-H "$header"} \
          --cacert "$CA" --resolve "$host:443:127.0.0.1" \
          --max-time "$TIMEOUT" "$url" 2>/dev/null)"

  if [ "$got" != "$want" ]; then
    printf '%s  ✗ %-44s %s (expected %s)  %s%s\n' \
      "$C_RED" "$url" "${got:-no response}" "$want" "$what" "$C_RESET" >&2
    fails=$((fails + 1))
    continue
  fi

  if ! grep -qF "$needle" "$body"; then
    printf '%s  ✗ %-44s %s but body lacks %s  %s%s\n' \
      "$C_RED" "$url" "$got" "\"$needle\"" "$what" "$C_RESET" >&2
    printf '      first line: %s\n' "$(head -c 160 "$body" | tr -d '\n')" >&2
    fails=$((fails + 1))
    continue
  fi

  printf '%s  ✓%s %-44s %s  %s\n' "$C_GRN" "$C_RESET" "$url" "$got" "$what"
done

echo
if [ "$fails" -eq 0 ]; then
  log_ok "all ${#TARGETS[@]} hosts answer over trusted HTTPS"
  exit 0
fi
log_fail "$fails of ${#TARGETS[@]} hosts failed" \
  "check container status:  make status" \
  "check a specific log:    make logs-api | make logs-ui"
exit 1
