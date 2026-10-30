#!/usr/bin/env bash
# =============================================================================
#  Give a stage on the server access to one repository (.ai/platform/docs/deployment.md).
# =============================================================================
#  A repo-scoped deploy key, generated ON the server and never leaving it -
#  not a copy of your personal SSH key. If the box is compromised the blast
#  radius is read access to one repository, not your GitHub account.
#
#  Idempotent at every step: an existing key is reused, an already-registered
#  key is reported and skipped.
#
#  Two stages, two keys, one ~/.ssh/config:
#
#    production  read-only, used as github.com          (the server only pulls)
#    staging     WRITE, used as the alias github-staging (commits vibe coded on
#                staging are pushed from there, and `make promote` tags)
#
#  A write key is a real widening of the blast radius, so it exists only for the
#  stage that needs it - and GitHub scopes it to this one repository.
#
#  Usage: deploy-key.sh HOST REPO [staging|production]
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"; REPO="${2:-}"; STAGE="${3:-production}"
. "$HERE/../lib/stage.sh"
STAGE="$(stage_normalize "$STAGE")" || die "STAGE must be staging or production"
GIT_HOST="$(stage_git_host "$STAGE")"
if [ "$STAGE" = production ]; then KEY=id_ed25519_deploy; READ_ONLY=true; else KEY=id_ed25519_staging; READ_ONLY=false; fi
[ -n "$HOST" ] && [ -n "$REPO" ] || die "usage: deploy-key.sh HOST REPO [staging|production]" \
  "example: deploy-key.sh deploy@203.0.113.42 acme/open-enu-go"

REMOTE() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" "$@"; }

log_step "Deploy key for $REPO on $HOST ($STAGE, $([ "$READ_ONLY" = true ] && echo read-only || echo read-write))"

REMOTE true 2>/dev/null || die "cannot ssh to $HOST" \
  "check the key is loaded (ssh-add -l), the user exists, and the host is reachable"

# ── 1. the key itself, generated on the server ──────────────────────────────
if REMOTE "test -f ~/.ssh/$KEY"; then
  log_skip "\$HOME/.ssh/$KEY already exists on the server - reused, never overwritten"
else
  REMOTE "mkdir -p ~/.ssh && chmod 700 ~/.ssh && ssh-keygen -t ed25519 -N '' -C 'deploy@$HOST' -f ~/.ssh/$KEY -q" \
    || die "could not generate the key on $HOST"
  log_ok "generated ~/.ssh/$KEY"
fi

PUBKEY="$(REMOTE "cat ~/.ssh/$KEY.pub")"
[ -n "$PUBKEY" ] || die "the public key is empty - generation failed silently"

# ── 2. pin GitHub's host keys, verified against GitHub's own published list ──
# `ssh-keyscan` on its own is trust-on-first-use against an unauthenticated
# network. The fingerprint is fetched AT RUN TIME rather than baked in as a
# constant, which would break the day GitHub rotates its keys.
log_step "Pinning github.com host key"
EXPECTED="$(curl -fsS https://api.github.com/meta 2>/dev/null \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["ssh_key_fingerprints"]["SHA256_ED25519"])' 2>/dev/null)"

if [ -z "$EXPECTED" ]; then
  die "could not fetch GitHub's host-key fingerprints from api.github.com/meta" \
    "without them, pinning the host key would be trust-on-first-use" \
    "check this machine's network, then re-run"
fi

# OpenSSH 10 prints a `# host:port banner` comment on stdout before the key.
SCANNED="$(REMOTE "ssh-keyscan -t ed25519 github.com 2>/dev/null" | grep -v '^#' | head -1)"
[ -n "$SCANNED" ] || die "the server could not reach github.com on port 22" \
  "check its egress firewall - the clone will fail for the same reason"

ACTUAL="$(printf '%s\n' "$SCANNED" | ssh-keygen -lf - 2>/dev/null | awk '{print $2}' | sed 's/^SHA256://')"

if [ "$ACTUAL" != "$EXPECTED" ]; then
  die "github.com host key does not match GitHub's published fingerprint" \
    "expected SHA256:$EXPECTED" \
    "got      SHA256:$ACTUAL" \
    "this is a hard failure, not a warning: something is intercepting the connection"
fi

REMOTE "touch ~/.ssh/known_hosts && chmod 600 ~/.ssh/known_hosts && \
        grep -q 'github.com ssh-ed25519' ~/.ssh/known_hosts || echo '$SCANNED' >> ~/.ssh/known_hosts"
log_ok "verified against api.github.com/meta and pinned"

# ── 3. tell git to use that key for this stage's host name ──────────────────
# github-staging is an alias for github.com: host-key checks still resolve to
# github.com, so the pinned key above covers both.
REMOTE "grep -q '^Host $GIT_HOST\$' ~/.ssh/config 2>/dev/null || {
  printf 'Host %s\n  HostName github.com\n  IdentityFile ~/.ssh/%s\n  IdentitiesOnly yes\n' '$GIT_HOST' '$KEY' >> ~/.ssh/config
  chmod 600 ~/.ssh/config
}"
log_ok "the server's ssh config points $GIT_HOST at $KEY"

# ── 4. register it on GitHub, from here, where the credentials are ──────────
TITLE="$(basename "$REPO") $STAGE $(date +%F)"
FINGERPRINT="$(printf '%s\n' "$PUBKEY" | ssh-keygen -lf - 2>/dev/null | awk '{print $2}')"

registered=0
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  if gh api "repos/$REPO/keys" --jq '.[].key' 2>/dev/null \
     | while read -r k; do printf '%s\n' "$k" | ssh-keygen -lf - 2>/dev/null | awk '{print $2}'; done \
     | grep -qF "$FINGERPRINT"; then
    log_skip "already registered on $REPO - not duplicated"
    registered=1
  elif gh api "repos/$REPO/keys" -f title="$TITLE" -f key="$PUBKEY" -F read_only=$READ_ONLY >/dev/null 2>&1; then
    log_ok "registered on $REPO as \"$TITLE\" ($([ "$READ_ONLY" = true ] && echo read-only || echo read-write))"
    registered=1
  else
    log_warn "gh could not register the key - falling back to the manual path"
  fi
fi

if [ "$registered" -eq 0 ]; then
  # Supported, not degraded: the same verification runs either way.
  echo
  log_info "Add this deploy key by hand:"
  log_info "  1. open   https://github.com/$REPO/settings/keys/new"
  log_info "  2. title  $TITLE"
  log_info "  3. key    (below)"
  if [ "$READ_ONLY" = true ]; then
    log_info "  4. LEAVE \"Allow write access\" UNCHECKED - production only needs to read"
  else
    log_info "  4. CHECK \"Allow write access\" - staging pushes the commits you make there"
  fi
  echo
  printf '%s\n\n' "$PUBKEY"
  printf '  Press enter once it is saved... '
  read -r _ </dev/tty 2>/dev/null || true
fi

# ── 5. prove it end to end ──────────────────────────────────────────────────
# The only step that matters. Everything above can look fine while the clone
# still fails, and this is where that shows up.
log_step "Verifying from the server"

GREETING="$(REMOTE "ssh -o StrictHostKeyChecking=yes -T git@$GIT_HOST 2>&1" || true)"
if ! grep -qi 'successfully authenticated' <<<"$GREETING"; then
  die "the server still cannot authenticate to GitHub" \
    "github said: $(head -1 <<<"$GREETING")" \
    "if you just added the key, give it a few seconds and re-run"
fi
log_ok "$(grep -o "Hi [^!]*" <<<"$GREETING" 2>/dev/null || echo 'authenticated')"

if ! REMOTE "git ls-remote git@$GIT_HOST:$REPO.git >/dev/null 2>&1"; then
  die "authenticated, but cannot read $REPO" \
    "the key is registered on a different repository, or the repo name is wrong"
fi

log_ok "$HOST can clone $REPO"
