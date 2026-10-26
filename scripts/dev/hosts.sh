#!/usr/bin/env bash
# =============================================================================
#  Map every ${DOMAIN} host to 127.0.0.1
# =============================================================================
#  On WSL2 there are TWO hosts files that matter and they serve different
#  clients: Linux /etc/hosts resolves for curl and the scripts in this repo, and
#  the Windows hosts file resolves for the browser. Writing only one leaves half
#  the stack apparently broken.
#
#  Both edits are a single marker-delimited block, so re-running replaces it
#  rather than appending a second copy.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/os.sh"

[ -f "$ROOT/.env" ] || die ".env is missing" "run: make env"
DOMAIN="$(sed -n 's/^DOMAIN=//p' "$ROOT/.env" | head -1)"
SLUG="$(sed -n 's/.*"slug"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$ROOT/.project.json" | head -1)"

# webhook-echo is test infrastructure and only runs under the `e2e` profile,
# but it gets a hosts entry anyway: debugging a delivery means opening it in a
# browser, and discovering the name does not resolve at that moment is a
# detour nobody needs.
HOSTS=("$DOMAIN" "www.$DOMAIN" "app.$DOMAIN" "manager.$DOMAIN" "api.$DOMAIN" \
       "traefik.$DOMAIN" "mail.$DOMAIN" "webhook-echo.$DOMAIN")
BEGIN="# >>> ${SLUG} dev hosts >>>"
END="# <<< ${SLUG} dev hosts <<<"
BLOCK="$BEGIN
127.0.0.1 ${HOSTS[*]}
::1 ${HOSTS[*]}
$END"

# up_to_date <file>
up_to_date() {
  [ -f "$1" ] || return 1
  local current
  current="$(sed -n "/$BEGIN/,/$END/p" "$1" 2>/dev/null | tr -d '\r')"
  [ "$current" = "$BLOCK" ]
}

# render <file> -> full desired content on stdout (block replaced or appended)
render() {
  local f="$1"
  if grep -qF "$BEGIN" "$f" 2>/dev/null; then
    awk -v b="$BEGIN" -v e="$END" -v blk="$BLOCK" '
      $0 ~ b {print blk; skip=1; next}
      $0 ~ e {skip=0; next}
      !skip {print}
    ' "$f"
  else
    cat "$f"; echo; echo "$BLOCK"
  fi
}

# ── Linux / macOS ───────────────────────────────────────────────────────────
log_step "Linux hosts file (/etc/hosts) - resolves for curl and this repo's scripts"
if up_to_date /etc/hosts; then
  log_skip "already current"
else
  tmp="$(mktemp)"; render /etc/hosts > "$tmp"
  if sudo cp "$tmp" /etc/hosts 2>/dev/null; then
    log_ok "${#HOSTS[@]} hosts mapped to 127.0.0.1"
  else
    log_warn "could not write /etc/hosts (sudo declined?)"
    log_info "add this block manually:"; echo "$BLOCK" | sed 's/^/      /'
  fi
  rm -f "$tmp"
fi

# ── WSL2: the browser resolves through Windows ──────────────────────────────
if is_wsl2; then
  WHOSTS="$(win_hosts_path)"
  log_step "Windows hosts file - resolves for the browser"
  if [ -z "$WHOSTS" ] || [ ! -f "$WHOSTS" ]; then
    log_warn "not reachable (is /mnt/c mounted?) - the browser will not resolve these names"
  elif up_to_date "$WHOSTS"; then
    log_skip "already current"
  else
    tmp="$(mktemp)"; render "$WHOSTS" > "$tmp"
    if cp "$tmp" "$WHOSTS" 2>/dev/null; then
      log_ok "${#HOSTS[@]} hosts mapped (direct write)"
    else
      # The hosts file needs Administrator. Elevate via PowerShell; Windows
      # shows a UAC prompt, which is the only interactive step in `builddev`.
      log_info "Windows will ask for Administrator - that prompt is this step."
      wtmp="$(wslpath -w "$tmp" 2>/dev/null || true)"
      if [ -n "$wtmp" ] && powershell.exe -NoProfile -Command \
           "Start-Process -Verb RunAs -Wait -FilePath powershell -ArgumentList '-NoProfile','-Command','Copy-Item -LiteralPath ''$wtmp'' -Destination ''C:\\Windows\\System32\\drivers\\etc\\hosts'' -Force'" >/dev/null 2>&1 \
         && up_to_date "$WHOSTS"; then
        log_ok "${#HOSTS[@]} hosts mapped (elevated)"
      else
        log_warn "could not update the Windows hosts file automatically"
        log_info "open Notepad as Administrator, edit"
        log_info "  C:\\Windows\\System32\\drivers\\etc\\hosts"
        log_info "and add:"; echo "$BLOCK" | sed 's/^/      /'
      fi
    fi
    rm -f "$tmp"
  fi
fi

# Never fail the build over a hosts file: the fallback is printed above and
# the stack is perfectly usable via curl --resolve or by editing hosts by hand.
exit 0
