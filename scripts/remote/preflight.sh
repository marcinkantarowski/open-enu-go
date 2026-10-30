#!/usr/bin/env bash
# =============================================================================
#  Preflight - the gate every remote target passes before touching a server.
# =============================================================================
#  .ai/platform/docs/deployment.md. Read-only, ten seconds, changes nothing. The
#  point of `buildprod` being one command is that it FAILS in the first ten seconds with
#  an actionable message, instead of half-provisioning a box and dying at ACME
#  forty minutes later.
#
#  Hard failures stop the run. Soft ones are collected and printed at the end,
#  so one pass tells you everything rather than one thing at a time.
#
#  Usage: preflight.sh HOST DOMAIN [REPO] [REF]
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"; DOMAIN="${2:-}"; REPO="${3:-}"; REF="${4:-main}"
[ -n "$HOST" ] && [ -n "$DOMAIN" ] || die \
  "usage: preflight.sh HOST DOMAIN [REPO] [REF]" \
  "example: make preflight HOST=deploy@203.0.113.42 DOMAIN=example.com REPO=acme/open-enu-go"

WARNINGS=()
warn() { WARNINGS+=("$1"); log_warn "$1"; }

REMOTE() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" "$@"; }

# ── target IP: the address in HOST, or the A record of its hostname ─────────
HOSTPART="${HOST#*@}"
if [[ "$HOSTPART" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  TARGET="$HOSTPART"
else
  TARGET="$(getent hosts "$HOSTPART" 2>/dev/null | awk '{print $1; exit}')"
  [ -n "$TARGET" ] || die "cannot resolve $HOSTPART to an IP" \
    "preflight compares DNS against the server's address, so it needs one"
fi

log_step "Preflight - $DOMAIN → $TARGET${REPO:+, repo $REPO@$REF}"

# ── 1-4. DNS and CAA ────────────────────────────────────────────────────────
# Delegated wholesale: dns-check writes the importable fixes when it fails, and
# duplicating any of that logic here would give two answers to one question.
if ! "$HERE/dns-check.sh" "$DOMAIN" "$TARGET"; then
  die "preflight stopped at DNS" "fix the records above, then re-run"
fi

# ── 3. proxy / CDN in front of the origin ───────────────────────────────────
# Worth a warning rather than a failure: it is a legitimate setup for the
# marketing site, and a fatal one for `api` (which also serves Mercure's SSE).
if command -v dig >/dev/null 2>&1; then
  ip="$(dig +short A "api.${DOMAIN}" 2>/dev/null | head -1)"
  case "$ip" in
    104.16.*|104.17.*|104.18.*|104.19.*|104.20.*|104.21.*|172.67.*|172.64.*|162.159.*)
      warn "api.${DOMAIN} resolves into a Cloudflare range - turn the orange cloud OFF for it. Proxying breaks Mercure SSE and hides the origin from HTTP-01 validation."
      ;;
  esac
fi

# ── 5. the ports ACME and the apps need ─────────────────────────────────────
log_step "Ports on $TARGET"
for port in 80 443; do
  # bash's /dev/tcp: no netcat dependency, and it is the same TCP connect.
  if timeout 5 bash -c "exec 3<>/dev/tcp/$TARGET/$port" 2>/dev/null; then
    log_ok "$port open"
  elif [ "$port" = 80 ] && ! REMOTE true 2>/dev/null; then
    warn "port 80 is closed on $TARGET and the server is not reachable to ask why - the SSH check below says more"
  elif [ "$port" = 80 ] && ! REMOTE "ss -tln | grep -q ':80 '" 2>/dev/null; then
    # A bare box: nothing listens yet and provisioning opens ufw after this
    # gate. Only something listening and unreachable is a firewall in the way.
    warn "nothing listens on port 80 yet - expected before the first deploy; provisioning opens it, a provider's firewall it cannot"
  elif [ "$port" = 80 ]; then
    die "port 80 is closed on $TARGET" \
      "Let's Encrypt HTTP-01 validation connects here - check ufw and the cloud security group" \
      "this fails at certificate issuance, long after the stack looks up"
  else
    warn "port 443 is closed on $TARGET - expected before the first deploy, a problem after it"
  fi
done

# ── 6. ssh ──────────────────────────────────────────────────────────────────
log_step "SSH"
if ! REMOTE true 2>/dev/null; then
  die "cannot ssh to $HOST" \
    "check: the key is loaded (ssh-add -l), the user exists, the host is reachable" \
    "on a brand-new box only root@ exists: bash scripts/remote/deploy-user.sh $HOST creates the user" \
    "(make buildprod / buildstaging do that themselves before running this)"
fi
log_ok "$HOST reachable, key accepted"

# ── 7. is this server able to run the stack ─────────────────────────────────
log_step "Server"
OS="$(REMOTE 'grep -oP "^ID=\K.*" /etc/os-release 2>/dev/null | tr -d \"' || true)"
case "$OS" in
  debian|ubuntu) log_ok "$OS" ;;
  "") warn "could not read /etc/os-release - provisioning assumes Debian or Ubuntu" ;;
  *) die "unsupported distribution: $OS" \
       "provisioning installs packages with apt and will not guess at another package manager" \
       "Debian 12+ or Ubuntu 22.04+ are what this is tested against" ;;
esac

RAM_MB="$(REMOTE "free -m 2>/dev/null | awk '/^Mem:/{print \$2}'" || echo 0)"
if [ "${RAM_MB:-0}" -lt 1900 ]; then
  warn "${RAM_MB}MB RAM - provisioning will add a swapfile; builds will be slow"
else
  log_ok "${RAM_MB}MB RAM"
fi

DISK_GB="$(REMOTE "df -BG --output=avail / 2>/dev/null | tail -1 | tr -dc '0-9'" || echo 0)"
if [ "${DISK_GB:-0}" -lt 20 ]; then
  die "${DISK_GB}GB free on / - images, volumes and backups need more than that" \
    "20GB is the floor; a build will fail partway and leave the stack down"
fi
log_ok "${DISK_GB}GB free"
log_ok "$(REMOTE 'uname -m' 2>/dev/null || echo 'unknown arch')"

# ── 8-10. the repository ────────────────────────────────────────────────────
if [ -n "$REPO" ]; then
  log_step "Repository"

  # 8: readable from HERE, where your credentials are.
  if command -v gh >/dev/null 2>&1 && gh repo view "$REPO" >/dev/null 2>&1; then
    log_ok "$REPO readable from this machine (gh)"
  elif git ls-remote "git@github.com:$REPO.git" >/dev/null 2>&1; then
    log_ok "$REPO readable from this machine (git)"
  else
    die "cannot read $REPO from this machine" \
      "check the name, and that you are authenticated (gh auth login)" \
      "or use SYNC=rsync to push the working tree instead - see .ai/platform/docs/deployment.md"
  fi

  # 9: the ref actually exists.
  if ! git ls-remote --exit-code "git@github.com:$REPO.git" "$REF" >/dev/null 2>&1; then
    available="$(git ls-remote --heads "git@github.com:$REPO.git" 2>/dev/null | awk '{print $2}' | sed 's#refs/heads/##' | paste -sd' ' -)"
    die "ref \"$REF\" does not exist on $REPO" "branches: ${available:-<none readable>}"
  fi
  log_ok "ref $REF exists"

  # 10: readable from THERE. The one that matters - 8 and 9 pass on your laptop
  # because your laptop holds your GitHub credentials. The server does not, and
  # that is exactly where the first clone fails.
  # GIT_HOST: github-staging for a staging checkout (its own read-write key).
  if REMOTE "git ls-remote git@${GIT_HOST:-github.com}:$REPO.git >/dev/null 2>&1"; then
    log_ok "$HOST can clone $REPO"
  else
    die "$HOST cannot read $REPO" \
      "the server has no deploy key for it yet" \
      "run: make deploy-key HOST=$HOST REPO=$REPO$([ "${GIT_HOST:-github.com}" = github.com ] || echo ' STAGE=staging')"
  fi
fi

# ── summary ─────────────────────────────────────────────────────────────────
echo
if [ ${#WARNINGS[@]} -gt 0 ]; then
  log_warn "${#WARNINGS[@]} warning(s) - none blocking:"
  for w in "${WARNINGS[@]}"; do log_info "  · $w"; done
  echo
fi
log_ok "preflight passed - safe to provision $HOST"
