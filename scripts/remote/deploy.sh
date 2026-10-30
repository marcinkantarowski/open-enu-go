#!/usr/bin/env bash
# =============================================================================
#  Move one stage to a git ref, from your machine (.ai/platform/docs/deployment.md).
# =============================================================================
#  `provision.sh` sets a stage up. This moves one that is already running.
#
#    staging      pull REF into the staging checkout, fast-forward only, then
#                 release. This is how work from your laptop reaches staging.
#                 It REFUSES when the staging checkout has uncommitted changes
#                 or commits of its own that are not on REF - that is somebody's
#                 vibe-coded work, and a deploy is not allowed to erase it.
#    production   run `make ci` here (unless it just passed on exactly what REF
#                 holds), move the production checkout to REF, and release.
#                 The usual path to production is `make promote`,
#                 which ships what staging runs; this one ships a ref.
#
#  NOT zero-downtime for production: `compose up -d` recreates changed
#  containers, and a single replica is unavailable for the seconds that takes.
#
#  Usage: deploy.sh HOST DOMAIN STAGE [REF]
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/stage.sh"; . "$HERE/../lib/ci-stamp.sh"

HOST="${1:-}"; DOMAIN="${2:-}"; STAGE="$(stage_normalize "${3:-}")"; REF="${4:-main}"
[ -n "$HOST" ] && [ -n "$STAGE" ] || die "usage: deploy.sh HOST DOMAIN staging|production [REF]"

SLUG="$(project_slug "$ROOT")"
APP_DIR="$(stage_dir "$SLUG" "$STAGE")"
R() { on "$HOST" "cd $APP_DIR && $*"; }

R "test -d .git" 2>/dev/null \
  || die "$APP_DIR on $HOST is not a git checkout" \
       "provision it first (make build$( [ "$STAGE" = production ] && echo prod || echo staging )), or it was provisioned with SYNC=rsync"

BEFORE="$(R "git rev-parse HEAD" 2>/dev/null || echo)"

if [ "$STAGE" = production ]; then
  # Deliberately the whole gate. "It was probably fine" is how a broken
  # migration reaches production on a Friday.
  if [ "${SKIP_CI:-0}" = 1 ]; then
    log_warn "SKIP_CI=1 - deploying without running the gate"
  elif git -C "$ROOT" fetch --quiet origin 2>/dev/null \
    && ci_stamp_fresh "$ROOT" "$(git -C "$ROOT" rev-parse --verify --quiet "origin/$REF^{tree}" \
         || git -C "$ROOT" rev-parse --verify --quiet "$REF^{tree}")"; then
    log_ok "gate green - make ci passed on exactly this code in the last $((CI_FRESH_SECONDS / 60)) min"
  else
    log_step "make ci"
    (cd "$ROOT" && make ci) \
      || die "make ci is red - refusing to deploy" "its output is above" \
             "SKIP_CI=1 exists and is a footgun"
    log_ok "gate green"
  fi

  log_step "Moving production to $REF"
  R "git fetch --quiet origin && { git -c advice.detachedHead=false checkout --quiet --detach origin/$REF 2>/dev/null \
       || git -c advice.detachedHead=false checkout --quiet --detach $REF; }" \
    || die "could not move $APP_DIR to $REF"
else
  log_step "Pulling $REF into staging"
  dirty="$(R "git status --porcelain")"
  if [ -n "$dirty" ]; then
    log_fail "staging has uncommitted changes - refusing to pull over them" \
      "on the server: commit them (then make promote pushes them) or discard them" || true
    head -15 <<<"$dirty" | sed 's/^/      /' >&2
    exit 1
  fi
  R "git fetch --quiet origin && git checkout --quiet $REF && git merge --quiet --ff-only origin/$REF" \
    || die "staging cannot fast-forward to origin/$REF" \
         "it has commits of its own that origin lacks - on the server: git pull --rebase, or push them with make promote"
fi

AFTER="$(R "git rev-parse --short HEAD")"
[ "${BEFORE:0:7}" = "$AFTER" ] && log_skip "already at $AFTER - releasing anyway (dependencies or env may have moved)" \
  || log_ok "${BEFORE:0:7} → $AFTER"

R "bash scripts/remote/release.sh $BEFORE" || die "the release failed on $HOST"

[ -n "$DOMAIN" ] && "$HERE/urls.sh" "$DOMAIN" 2>/dev/null || true
