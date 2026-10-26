#!/usr/bin/env bash
# =============================================================================
#  DNS preflight - and the fixes, in three importable formats
#  (.ai/platform/docs/deployment.md).
# =============================================================================
#  Read-only. It never changes a zone; `dns-apply.sh` does that, and only when
#  you ask it to with a token you supplied. A build command that quietly
#  rewrites a live zone is not a tool anyone should trust near production.
#
#  Two queries per record, deliberately:
#    • the local resolver, because that is what your laptop believes;
#    • the AUTHORITATIVE nameserver, because that is what is actually true.
#  A stale ISP cache can otherwise produce a false pass (record deleted an hour
#  ago, still cached) or a false fail (record added a minute ago, not yet
#  propagated). Only the authoritative answer decides.
#
#  Usage: dns-check.sh DOMAIN TARGET [--ttl N] [--mode explicit|wildcard]
#         TARGET is an IP, or anything an IP can be resolved from - a hostname,
#         or a `user@host` the way the rest of the remote targets take it.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

DOMAIN="${1:-}"
TARGET="${2:-}"
TTL=300
MODE="${DNS_MODE:-explicit}"

shift 2 2>/dev/null || true
while [ $# -gt 0 ]; do
  case "$1" in
    --ttl) TTL="$2"; shift 2 ;;
    --mode) MODE="$2"; shift 2 ;;
    *) shift ;;
  esac
done

[ -n "$DOMAIN" ] && [ -n "$TARGET" ] || die "usage: dns-check.sh DOMAIN TARGET [--ttl N] [--mode explicit|wildcard]" \
  "TARGET is an IP, a hostname, or the same user@host the remote targets take"

# Accept `deploy@1.2.3.4` and `deploy@host.example` as readily as a bare IP: the
# caller already typed the server once, and asking for it again in a second
# format is how a preflight ends up checking the wrong address.
TARGET="${TARGET#*@}"
if ! [[ "$TARGET" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  resolved="$(getent hosts "$TARGET" 2>/dev/null | awk '{print $1; exit}')"
  [ -n "$resolved" ] || die "cannot resolve $TARGET to an IP" \
    "DNS is compared against the server's address, so one is needed"
  TARGET="$resolved"
fi
command -v dig >/dev/null 2>&1 || die "dig is not installed" "install: apt install dnsutils | brew install bind"

OUT="$ROOT/.out/dns/$DOMAIN"
SUBDOMAINS=(www app manager api)

# ── the authoritative nameserver ────────────────────────────────────────────
# Asked once and reused: every subsequent lookup goes straight to the source of
# truth rather than through whatever this machine happens to cache.
NS="$(dig +short NS "$DOMAIN" 2>/dev/null | head -1)"
if [ -z "$NS" ]; then
  # No NS at all usually means the domain is not delegated yet - a different
  # problem from a missing record, and worth saying so plainly.
  log_fail "no nameservers found for $DOMAIN" \
    "the domain is not delegated yet, or the name is wrong" \
    "check it is registered and its nameservers are set at the registrar"
  exit 1
fi

# lookup <name> -> the authoritative answer, or empty
lookup() {
  local name="$1"
  dig +short "@${NS}" A "$name" 2>/dev/null | grep -E '^[0-9.]+$' | head -1
}

# ── build the desired record set ────────────────────────────────────────────
declare -a WANT_NAME WANT_TYPE
if [ "$MODE" = wildcard ]; then
  # For a product that hands tenants their own subdomain. Note the caveat the
  # summary prints: a PROXIED wildcard is Cloudflare Enterprise-only.
  WANT_NAME=("@" "*")
else
  WANT_NAME=("@")
  for sub in "${SUBDOMAINS[@]}"; do WANT_NAME+=("$sub"); done
fi
for _ in "${WANT_NAME[@]}"; do WANT_TYPE+=("A"); done

# ── check each one ──────────────────────────────────────────────────────────
declare -a ROWS=()
missing=0; wrong=0; ok=0

log_step "DNS for $DOMAIN → $TARGET   (mode: $MODE, authoritative: $NS)"

for i in "${!WANT_NAME[@]}"; do
  name="${WANT_NAME[$i]}"
  fqdn="$DOMAIN"
  [ "$name" != "@" ] && fqdn="${name}.${DOMAIN}"

  current="$(lookup "$fqdn")"

  if [ -z "$current" ]; then
    ROWS+=("missing|A|$name|-|$TARGET")
    missing=$((missing + 1))
  elif [ "$current" != "$TARGET" ]; then
    ROWS+=("wrong|A|$name|$current|$TARGET")
    wrong=$((wrong + 1))
  else
    ROWS+=("ok|A|$name|$current|$TARGET")
    ok=$((ok + 1))
  fi
done

# ── CAA: the one that fails silently ────────────────────────────────────────
# A CAA record that names some other CA does not warn anybody. Issuance simply
# never succeeds, and the error surfaces hours later inside Traefik's ACME log.
caa="$(dig +short "@${NS}" CAA "$DOMAIN" 2>/dev/null)"
caa_blocks=0
if [ -n "$caa" ]; then
  if grep -qi 'letsencrypt\.org' <<<"$caa"; then
    ROWS+=("ok|CAA|@|$(tr '\n' ' ' <<<"$caa" | sed 's/ *$//')|includes letsencrypt.org")
  else
    ROWS+=("wrong|CAA|@|$(tr '\n' ' ' <<<"$caa" | sed 's/ *$//')|0 issue \"letsencrypt.org\"")
    caa_blocks=1
  fi
fi

# ── mail records: informational ─────────────────────────────────────────────
# Warn only. They matter once the app sends mail FROM this domain, which is not
# a prerequisite for the stack coming up.
for t in MX TXT; do
  found="$(dig +short "@${NS}" "$t" "$DOMAIN" 2>/dev/null | head -1)"
  [ -z "$found" ] && ROWS+=("warn|$t|@|-|deliverability only, not needed to go live")
done

# ── report ──────────────────────────────────────────────────────────────────
printf '\n  %-9s %-5s %-9s %-24s %s\n' status type name current expected
printf '  %s\n' "────────────────────────────────────────────────────────────────────────────"
for row in "${ROWS[@]}"; do
  IFS='|' read -r status type name current expected <<<"$row"
  case "$status" in
    ok)      colour="$C_GRN" ;;
    warn)    colour="$C_YEL" ;;
    *)       colour="$C_RED" ;;
  esac
  printf '  %s%-9s%s %-5s %-9s %-24s %s\n' "$colour" "$status" "$C_RESET" "$type" "$name" "$current" "$expected"
done
echo

if [ "$missing" -eq 0 ] && [ "$wrong" -eq 0 ] && [ "$caa_blocks" -eq 0 ]; then
  log_ok "every record resolves to $TARGET"
  exit 0
fi

# ── remediation, in three formats ───────────────────────────────────────────
# A diagnosis you have to retype into a DNS panel is half a tool.
mkdir -p "$OUT"
"$HERE/dns-emit.sh" "$DOMAIN" "$TARGET" "$TTL" "$MODE" "$OUT" "${ROWS[@]}"

# Everything from here to stderr, so the diagnosis and its remedy stay in one
# stream and cannot interleave when the caller pipes stdout somewhere.
{
  log_fail "DNS incomplete for $DOMAIN → $TARGET" \
    "$missing missing, $wrong wrong$([ "$caa_blocks" -eq 1 ] && echo ", CAA blocks Let's Encrypt")" || true
  printf '\n    Wrote:\n'
  printf '      %s/zone.txt        → import at Cloudflare / most providers\n' "${OUT#"$ROOT"/}"
  printf '      %s/cloudflare.sh   → applies only the deltas via API\n' "${OUT#"$ROOT"/}"
  printf '      %s/records.json    → machine-readable, for Terraform/Route53\n' "${OUT#"$ROOT"/}"
  printf '\n    Then re-run the same preflight command.\n\n'
} >&2
exit 1
