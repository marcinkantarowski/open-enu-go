#!/usr/bin/env bash
# =============================================================================
#  Promote: production runs exactly the commit staging runs.
# =============================================================================
#  Not "deploy main to production". Whatever staging is running - a commit you
#  vibe coded on the staging server, or one pulled in from your laptop - is
#  what you looked at, so that SHA is what ships. A branch head may have moved
#  since.
#
#  Code only. Staging's database never goes anywhere near production.
#
#    1. staging is clean        uncommitted work is not a release; commit it
#    2. staging is pushed       the commit reaches GitHub (staging's key writes)
#    3. the gate, on staging    arch + tests + typecheck, against that commit
#    4. production moves        fetch that SHA from GitHub, release.sh:
#                               build → back up → migrate → restart → health
#    5. a tag                   prod-<timestamp>, so "what was live when" is git
#
#  Staging and production on one server or two:
#
#    on the staging server   make promote                    (prod on this box)
#                            make promote PROD=deploy@prod   (prod elsewhere -
#                            this server then needs ssh access to it)
#    from your machine       make promote STAGING=deploy@stg [PROD=deploy@prod]
#                            PROD defaults to STAGING: one box for both
#
#  Usage: STAGING=<host|local> PROD=<host|local> promote.sh
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/stage.sh"

SLUG="$(project_slug "$ROOT")"
SDIR="$(stage_dir "$SLUG" staging)"
PDIR="$(stage_dir "$SLUG" production)"

here_is_staging() { [ "$(sed -n 's/^APP_STAGE=//p' "$ROOT/.env" 2>/dev/null | head -1)" = staging ]; }

STAGING="${STAGING:-}"; PROD="${PROD:-}"
if [ -z "$STAGING" ]; then
  here_is_staging || die "where is staging?" \
    "from your machine:      make promote STAGING=deploy@<staging-ip> [PROD=deploy@<prod-ip>]" \
    "on the staging server:  make promote"
  STAGING=local
fi
if [ -z "$PROD" ]; then
  if [ "$STAGING" = local ]; then
    [ -d "$PDIR" ] || die "production is not on this server ($PDIR does not exist)" \
      "if it is on another one: make promote PROD=deploy@<prod-ip>" \
      "if it does not exist yet: make buildprod HOST=… from your machine"
    PROD=local
  else
    PROD="$STAGING"
  fi
fi

S() { on "$STAGING" "cd $SDIR && $*"; }
P() { on "$PROD" "cd $PDIR && $*"; }

log_step "Promote staging ($STAGING) → production ($PROD)"

# ── 1. staging is a clean checkout of a branch ──────────────────────────────
[ "$(S "sed -n 's/^APP_STAGE=//p' .env")" = staging ] \
  || die "$SDIR on $STAGING is not a staging checkout"

dirty="$(S "git status --porcelain")" || die "cannot read git status in $SDIR on $STAGING"
if [ -n "$dirty" ]; then
  log_fail "staging has uncommitted changes - a promotion ships commits, never a working tree" \
    "commit what you mean to ship (and discard what you do not), then promote again:" || true
  head -15 <<<"$dirty" | sed 's/^/      /' >&2
  exit 1
fi

BRANCH="$(S "git symbolic-ref --quiet --short HEAD")" \
  || die "staging is on a detached HEAD" "check out the branch you work on: cd $SDIR && git switch main"

# ── 2. pushed, and not behind ───────────────────────────────────────────────
S "git fetch --quiet origin" || die "staging cannot reach GitHub" "check its deploy key: make deploy-key HOST=… REPO=… STAGE=staging"

behind="$(S "git rev-list --count HEAD..origin/$BRANCH 2>/dev/null || echo 0")"
if [ "${behind:-0}" -gt 0 ]; then
  die "origin/$BRANCH has $behind commit(s) staging has not seen" \
    "staging must run them before they can be promoted: make deploystaging (it pulls them in)" \
    "then look at staging, then promote"
fi

ahead="$(S "git rev-list --count origin/$BRANCH..HEAD 2>/dev/null || echo 0")"
if [ "${ahead:-0}" -gt 0 ]; then
  S "git push --quiet origin HEAD:$BRANCH" \
    || die "could not push $ahead commit(s) from staging" \
         "staging's deploy key must be allowed to write: make deploy-key HOST=… REPO=… STAGE=staging"
  log_ok "pushed $ahead commit(s) to origin/$BRANCH"
fi

SHA="$(S "git rev-parse HEAD")"
log_ok "staging runs $(S "git log -1 --format='%h %s'")"

# ── 3. the gate, on staging, against this commit ────────────────────────────
if [ "${SKIP_GATE:-0}" = 1 ]; then
  log_warn "SKIP_GATE=1 - promoting without the gate"
else
  log_step "Gate on staging: make promote-gate"
  if ! S "make --no-print-directory promote-gate > /tmp/$SLUG-promote-gate.log 2>&1"; then
    log_fail "the gate is red - production was not touched" || true
    S "tail -40 /tmp/$SLUG-promote-gate.log" | sed 's/^/      /' >&2
    log_info "full log on staging: /tmp/$SLUG-promote-gate.log"
    exit 1
  fi
  log_ok "gate green"
fi

# ── 4. production moves to that SHA ─────────────────────────────────────────
[ "$(P "sed -n 's/^APP_STAGE=//p' .env" 2>/dev/null)" = production ] \
  || die "$PDIR on $PROD is not a production checkout" "provision it first: make buildprod HOST=…"

BEFORE="$(P "git rev-parse HEAD")"
if [ "$BEFORE" = "$SHA" ] && [ "${FORCE:-0}" != 1 ]; then
  log_skip "production already runs ${SHA:0:7} - nothing to promote (FORCE=1 re-releases it)"
  exit 0
fi

P "git fetch --quiet origin && git -c advice.detachedHead=false checkout --quiet --detach $SHA" \
  || die "production could not fetch ${SHA:0:7} from GitHub" "check its deploy key: make deploy-key HOST=… REPO=…"
log_ok "production checkout ${BEFORE:0:7} → ${SHA:0:7}"

if ! P "bash scripts/remote/release.sh $BEFORE"; then
  die "production did not release cleanly" \
    "roll back:  on $PROD: cd $PDIR && git checkout --detach $BEFORE && bash scripts/remote/release.sh" \
    "the pre-deploy dump is in $PDIR/backups - a migration is not undone by rolling back code"
fi

# ── 5. remember it ──────────────────────────────────────────────────────────
TAG="prod-$(date -u +%Y%m%d-%H%M%S)"
if S "git tag $TAG $SHA && git push --quiet origin $TAG" >/dev/null 2>&1; then
  log_ok "tagged $TAG"
else
  log_warn "released, but could not push the tag $TAG"
fi

echo
log_ok "production runs ${SHA:0:7} - what staging runs"
