#!/usr/bin/env bash
# =============================================================================
#  Make the running stack match the checkout. Runs ON the server, in the stack's
#  own checkout, AFTER the caller has moved it to the commit to release.
# =============================================================================
#  One script for provisioning, deploying and promoting, so the three cannot
#  drift into three slightly different releases.
#
#  The caller moves the checkout, not this script: bash reads a script as it
#  runs, and a script that `git checkout`s a new version of itself executes a
#  mix of both.
#
#    production   build → back up → migrate → edge → up → health
#                 One static binary image for api, worker and scheduler, and
#                 one Nuxt image per app, each tagged with RELEASE (the short
#                 commit). Nothing running is touched until the build and the
#                 migration have both succeeded.
#    staging      the dev stack: make up (edge + attach), deps, wait, migrate.
#                 air does the rest; this exists for dependencies, migrations
#                 and containers that changed.
#
#  Release tags (production). Every image is built as <stack>-<svc>:<RELEASE>,
#  and the one serving is also tagged `latest` - what the boot unit, a restore
#  and a hand-typed `docker compose` start. .out/releases lists the releases
#  that served, newest last; the last three keep their images, older tags are
#  removed. That is what makes a rollback a restart instead of a rebuild:
#
#    git checkout --detach <previous commit> && bash scripts/remote/release.sh --rollback
#
#  --rollback builds nothing, dumps nothing and migrates nothing (a migration
#  is not undone by older code; the pre-deploy dump is how a schema goes back).
#  It refuses when the images of that commit are no longer on the box.
#
#  Usage: release.sh [PREVIOUS_COMMIT]   - named in the rollback hint on failure
#         release.sh --rollback          - serve the checkout's commit from its kept images
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"
cd "$ROOT" || die "cannot enter $ROOT"

ROLLBACK=0; BEFORE=""
case "${1:-}" in
  --rollback) ROLLBACK=1 ;;
  *) BEFORE="${1:-}" ;;
esac
[ -f .env ] || die "$ROOT/.env is missing" "provision this stack first: make buildstaging / make buildprod"
env_value() { sed -n "s/^$1=//p" .env | head -1; }
STAGE="$(env_value APP_STAGE)"
DOMAIN="$(env_value DOMAIN)"
STACK="$(env_value STACK)"
DB_NAME="$(env_value DB_NAME)"
# /health/deep answers only the token holder (kernel/httpx/health.go).
HEALTH_TOKEN="$(env_value HEALTH_TOKEN)"
compose_init "$ROOT"
AT="$(git rev-parse --short HEAD 2>/dev/null || echo 'a working tree')"

# The tag of this release: the commit, at a fixed length so the same commit is
# always the same tag. A checkout without git (SYNC=rsync) gets a timestamp.
RELEASE="$(git rev-parse --short=12 HEAD 2>/dev/null || date -u +r%Y%m%d%H%M%S)"
export RELEASE
RELEASES_FILE="$ROOT/.out/releases"
KEEP_RELEASES=3

PREVIOUS_RELEASE="$(tail -1 "$RELEASES_FILE" 2>/dev/null || true)"
rollback_hint() {
  if [ -n "$BEFORE" ]; then
    log_info "roll back: cd $ROOT && git checkout --detach $BEFORE && bash scripts/remote/release.sh --rollback"
  elif [ -n "$PREVIOUS_RELEASE" ] && [ "$PREVIOUS_RELEASE" != "$RELEASE" ]; then
    log_info "roll back: cd $ROOT && git checkout --detach $PREVIOUS_RELEASE && bash scripts/remote/release.sh --rollback"
  fi
  return 0
}
fail() { log_fail "$@" || true; rollback_hint; exit 1; }

# The images this checkout's compose files build, at this RELEASE - read from
# compose itself, so a project's own service (docker/project/) is included.
release_images() { "${COMPOSE[@]}" config --images 2>/dev/null | grep -E ":${RELEASE}\$" | sort -u; }

# What serves is also `latest`; the list remembers it; old tags go.
record_release() {
  local img repo tag keep
  while IFS= read -r img; do
    [ -n "$img" ] && docker tag "$img" "${img%:*}:latest" >/dev/null 2>&1
  done < <(release_images)
  mkdir -p "$ROOT/.out"
  { grep -vxF "$RELEASE" "$RELEASES_FILE" 2>/dev/null; echo "$RELEASE"; } > "$RELEASES_FILE.tmp" \
    && mv "$RELEASES_FILE.tmp" "$RELEASES_FILE"
  keep="$(tail -n "$KEEP_RELEASES" "$RELEASES_FILE")"
  tail -n "$KEEP_RELEASES" "$RELEASES_FILE" > "$RELEASES_FILE.tmp" && mv "$RELEASES_FILE.tmp" "$RELEASES_FILE"
  while IFS= read -r img; do
    [ -n "$img" ] || continue
    repo="${img%:*}"
    while IFS= read -r tag; do
      case "$tag" in latest|"<none>"|"") continue ;; esac
      if grep -qxF "$tag" <<<"$keep"; then continue; fi
      docker rmi "$repo:$tag" >/dev/null 2>&1 && log_skip "pruned $repo:$tag"
    done < <(docker image ls --format '{{.Tag}}' "$repo" 2>/dev/null)
  done < <(release_images)
  log_ok "release $RELEASE recorded; images kept for: $(tr '\n' ' ' <<<"$keep")"
}

health() {
  # Through the edge, with the certificate: the question is whether the
  # public URL works, not whether the binary starts. --resolve so a cloud's
  # NAT hairpin cannot make a healthy stack look down. A bounded poll of the
  # public endpoint - there is no healthcheck to --wait on from the outside.
  log_step "Health - https://api.$DOMAIN/health/deep"
  for _ in $(seq 1 60); do
    if curl -fsS --max-time 5 --resolve "api.$DOMAIN:443:127.0.0.1" -H "X-Health-Token: $HEALTH_TOKEN" "https://api.$DOMAIN/health/deep" 2>/dev/null \
       | grep -q '"status":"ok"'; then
      log_ok "deep health check green"
      return 0
    fi
    sleep 4
  done
  return 1
}

log_step "Releasing $AT to $STACK ($STAGE)$([ "$ROLLBACK" -eq 1 ] && echo ' - rollback, no build')"

# Derived files are regenerated from .env, which is preserved. Skipping this is
# how a variable added upstream is missing on the server.
./scripts/lib/envgen.sh generate >/dev/null || fail "envgen failed"

case "$STAGE" in
  production)
    if [ "$ROLLBACK" -eq 1 ]; then
      mapfile -t imgs < <(release_images)
      [ ${#imgs[@]} -gt 0 ] || die "compose names no image tagged $RELEASE"
      for img in "${imgs[@]}"; do
        docker image inspect "$img" >/dev/null 2>&1 \
          || die "$img is not on this server - only the last $KEEP_RELEASES releases keep their images" \
                 "kept: $(tr '\n' ' ' < "$RELEASES_FILE" 2>/dev/null)" \
                 "an older commit needs a full release: bash scripts/remote/release.sh"
      done
      log_ok "images for $RELEASE are on the box - nothing to build"
      log_warn "no migration runs on a rollback - if the newer release changed the schema, the pre-deploy dump in $ROOT/backups is the way back"
      bash scripts/dev/edge.sh up || fail "the edge is not running"
      "${COMPOSE[@]}" up -d --no-build --remove-orphans || fail "restart failed"
      bash scripts/dev/edge.sh attach || fail "could not attach the edge"
      record_release
      health || fail "the rolled-back stack did not come back healthy within 240s" \
        "inspect: ${COMPOSE[*]} logs --tail=80 api"
      log_ok "$DOMAIN is running $AT (rolled back)"
      exit 0
    fi

    log_step "Building $RELEASE"
    # --pull: fetch the newest golang, distroless and node base images first.
    # Without it a release rebuilds on whatever the server pulled last time,
    # and a Go security release never reaches it.
    "${COMPOSE[@]}" build --pull --quiet || fail "build failed - the running stack is untouched"

    # The database first, and healthy: the dump below needs it, and on a first
    # deploy nothing is running yet. --wait is the healthcheck (pg_isready),
    # not a sleep. On a later deploy postgres is already up and this changes
    # nothing.
    "${COMPOSE[@]}" up -d --wait postgres >/dev/null \
      || fail "postgres did not become healthy - nothing was migrated or restarted" \
              "inspect: ${COMPOSE[*]} logs --tail=80 postgres"

    log_step "Backup, then migrate"
    # backup.sh's exit codes are a contract (its header). Only a dump taken BY
    # THIS RUN counts: the stamp is ours, and the file must carry it. An older
    # dump lying in backups/ is exactly what a disk-full failure leaves behind,
    # and migrating on it would make "the way back" a day old.
    STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
    BACKUP_STAMP="$STAMP" bash scripts/remote/backup.sh pre-deploy >/dev/null 2>&1
    rc=$?
    DUMP_FOUND="$(find "$ROOT/backups" -maxdepth 1 -type f -size +0 -name "${DB_NAME}-${STAMP}-pre-deploy.dump*" 2>/dev/null | head -1)"
    case "$rc" in
      0) ;;
      3) log_warn "the pre-deploy dump is on this disk only - the copy off the box failed (bash scripts/remote/backup.sh manual says why)" ;;
      4) log_warn "the pre-deploy dump has no storage archive - the database is covered, the uploads are not (bash scripts/remote/backup.sh manual says why)" ;;
      *) fail "the pre-deploy dump failed (backup.sh exit $rc) - nothing was migrated or restarted" \
              "see why: cd $ROOT && bash scripts/remote/backup.sh manual" ;;
    esac
    [ -n "$DUMP_FOUND" ] || fail "backup.sh reported exit $rc but no dump stamped $STAMP is in $ROOT/backups - refusing to migrate"
    log_ok "pre-deploy dump: $(basename "$DUMP_FOUND")"

    # The same image that will serve, run once as the schema owner. River's,
    # the kernel's and every module's migrations, in dependency order.
    # --no-deps so a migration never starts or recreates the serving containers.
    "${COMPOSE[@]}" run --rm -T --no-deps api migrate >/dev/null \
      || fail "migrations failed - nothing was restarted, the old containers are still serving" \
              "the pre-deploy dump is $DUMP_FOUND"
    log_ok "migrated"

    bash scripts/dev/edge.sh up || fail "the edge is not running"
    log_step "Restarting (brief downtime)"
    "${COMPOSE[@]}" up -d --remove-orphans || fail "restart failed"
    bash scripts/dev/edge.sh attach || fail "could not attach the edge"
    # Recorded once it serves, healthy or not: `latest` is what a reboot
    # starts, and a reboot should start what was running.
    record_release

    health || fail "the stack did not come back healthy within 240s" \
      "a first certificate can take a minute - inspect: docker logs enu-edge; ${COMPOSE[*]} logs --tail=80 api"
    log_ok "$DOMAIN is running $AT"
    exit 0
    ;;

  staging)
    [ "$ROLLBACK" -eq 0 ] || die "--rollback is for production" "staging rebuilds from source: check out the commit and run release.sh"
    make --no-print-directory up    || fail "the staging stack did not start"
    make --no-print-directory deps  || fail "dependencies did not install"
    make --no-print-directory wait  || fail "the staging stack did not become healthy"
    make --no-print-directory migrate >/dev/null || fail "migrations failed"
    log_ok "migrated"

    # Inside the stack, not through the edge: staging answers only to the
    # allowlist, and this server's own address is not on it. The dev image is
    # alpine, so wget is there; the token is the container's own HEALTH_TOKEN.
    if "${COMPOSE[@]}" exec -T api sh -c 'wget -qO- -T 10 --header "X-Health-Token: $HEALTH_TOKEN" http://127.0.0.1:8080/health/deep' 2>/dev/null | grep -q '"status":"ok"'; then
      log_ok "staging is running $AT - edit, and it reloads"
      exit 0
    fi
    fail "staging did not pass its deep health check" "inspect: make logs-api"
    ;;

  *) die "APP_STAGE is '$STAGE' in $ROOT/.env" "release.sh runs on a server: staging or production" ;;
esac
