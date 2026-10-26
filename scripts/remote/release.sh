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
#                 one Nuxt image per app. Nothing running is touched until the
#                 build and the migration have both succeeded.
#    staging      the dev stack: make up (edge + attach), deps, wait, migrate.
#                 air does the rest; this exists for dependencies, migrations
#                 and containers that changed.
#
#  Usage: release.sh [PREVIOUS_COMMIT]   - named in the rollback hint on failure
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"
cd "$ROOT" || die "cannot enter $ROOT"

BEFORE="${1:-}"
[ -f .env ] || die "$ROOT/.env is missing" "provision this stack first: make buildstaging / make buildprod"
env_value() { sed -n "s/^$1=//p" .env | head -1; }
STAGE="$(env_value APP_STAGE)"
DOMAIN="$(env_value DOMAIN)"
STACK="$(env_value STACK)"
compose_init "$ROOT"
AT="$(git rev-parse --short HEAD 2>/dev/null || echo 'a working tree')"

rollback_hint() {
  [ -n "$BEFORE" ] || return 0
  log_info "roll back: cd $ROOT && git checkout --detach $BEFORE && bash scripts/remote/release.sh"
}
fail() { log_fail "$@" || true; rollback_hint; exit 1; }

log_step "Releasing $AT to $STACK ($STAGE)"

# Derived files are regenerated from .env, which is preserved. Skipping this is
# how a variable added upstream is missing on the server.
./scripts/lib/envgen.sh generate >/dev/null || fail "envgen failed"

case "$STAGE" in
  production)
    log_step "Building"
    # --pull: fetch the newest golang, distroless and node base images first.
    # Without it a release rebuilds on whatever the server pulled last time,
    # and a Go security release never reaches it.
    "${COMPOSE[@]}" build --pull --quiet || fail "build failed - the running stack is untouched"

    log_step "Backup, then migrate"
    # A failed copy off the box does not stop a deploy: the dump a rollback
    # needs is the local one. It is said, though, not hidden behind "no database".
    if bash scripts/remote/backup.sh pre-deploy >/dev/null 2>&1; then
      log_ok "pre-deploy dump taken"
    elif ls "$ROOT"/backups/*-pre-deploy.dump* >/dev/null 2>&1; then
      log_warn "pre-deploy dump is on this disk only - the copy off the box failed (bash scripts/remote/backup.sh manual says why)"
    else
      log_skip "no database to dump yet"
    fi
    # The database first, and healthy: on a first deploy nothing is running
    # yet, and `run --no-deps` below would migrate against a host that does
    # not exist. --wait is the healthcheck (pg_isready), not a sleep. On a
    # later deploy postgres is already up and this changes nothing.
    "${COMPOSE[@]}" up -d --wait postgres >/dev/null \
      || fail "postgres did not become healthy - nothing was migrated or restarted" \
              "inspect: ${COMPOSE[*]} logs --tail=80 postgres"
    # The same image that will serve, run once as the schema owner. River's,
    # the kernel's and every module's migrations, in dependency order.
    # --no-deps so a migration never starts or recreates the serving containers.
    "${COMPOSE[@]}" run --rm -T --no-deps api migrate >/dev/null \
      || fail "migrations failed - nothing was restarted, the old containers are still serving" \
              "the pre-deploy dump is in $ROOT/backups"
    log_ok "migrated"

    bash scripts/dev/edge.sh up || fail "the edge is not running"
    log_step "Restarting (brief downtime)"
    "${COMPOSE[@]}" up -d --remove-orphans || fail "restart failed"
    bash scripts/dev/edge.sh attach || fail "could not attach the edge"

    # Through the edge, with the certificate: the question is whether the
    # public URL works, not whether the binary starts. --resolve so a cloud's
    # NAT hairpin cannot make a healthy stack look down.
    log_step "Health - https://api.$DOMAIN/health/deep"
    for _ in $(seq 1 60); do
      if curl -fsS --max-time 5 --resolve "api.$DOMAIN:443:127.0.0.1" "https://api.$DOMAIN/health/deep" 2>/dev/null \
         | grep -q '"status":"ok"'; then
        log_ok "deep health check green"
        log_ok "$DOMAIN is running $AT"
        exit 0
      fi
      sleep 4
    done
    fail "the stack did not come back healthy within 240s" \
      "a first certificate can take a minute - inspect: docker logs enu-edge; ${COMPOSE[*]} logs --tail=80 api"
    ;;

  staging)
    make --no-print-directory up    || fail "the staging stack did not start"
    make --no-print-directory deps  || fail "dependencies did not install"
    make --no-print-directory wait  || fail "the staging stack did not become healthy"
    make --no-print-directory migrate >/dev/null || fail "migrations failed"
    log_ok "migrated"

    # Inside the stack, not through the edge: staging answers only to the
    # allowlist, and this server's own address is not on it. The dev image is
    # alpine, so wget is there.
    if "${COMPOSE[@]}" exec -T api wget -qO- -T 10 http://127.0.0.1:8080/health/deep 2>/dev/null | grep -q '"status":"ok"'; then
      log_ok "staging is running $AT - edit, and it reloads"
      exit 0
    fi
    fail "staging did not pass its deep health check" "inspect: make logs-api"
    ;;

  *) die "APP_STAGE is '$STAGE' in $ROOT/.env" "release.sh runs on a server: staging or production" ;;
esac
