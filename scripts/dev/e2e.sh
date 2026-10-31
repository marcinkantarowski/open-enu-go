#!/usr/bin/env bash
# =============================================================================
#  make e2e - the browser tests, against their own database.
# =============================================================================
#  The tests create tenants and change flags, so they never run on the
#  development database - which may hold a copy of production's. ${DB_NAME}_e2e
#  is made from nothing on every run, and the API, the worker and the scheduler
#  point at it for as long as the tests take. They return to the development
#  database on every way out: a failure, ctrl-c, a kill of this script.
#  Nothing else in the stack moves; `make up` also puts them back.
#
#  Usage: e2e.sh [spec...]
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

env_value() { sed -n "s/^$1=//p" "$ROOT/.env" | head -1; }

# compose prefers the environment to .env, and the tests create tenants: a
# DOMAIN from outside would send them to whatever server carries that name.
[ "${DOMAIN:-$(env_value DOMAIN)}" = "$(env_value DOMAIN)" ] \
  || die "DOMAIN=$DOMAIN in the environment is not this stack's" "the browser tests run on the DOMAIN in .env only: unset it"

compose_init "$ROOT"
DB_USER="$(env_value DB_USER)"; DEV_DB="$(env_value DB_NAME)"
[ -n "$DB_USER" ] && [ -n "$DEV_DB" ] || die "no DB_USER or DB_NAME in .env"
E2E_DB="${DEV_DB}_e2e"
[ -n "$("${COMPOSE[@]}" ps -q --status running api 2>/dev/null)" ] || die "the dev stack is not running" "start it: make up"

psql_dev() { "${COMPOSE[@]}" exec -T postgres psql -U "$DB_USER" -v ON_ERROR_STOP=1 -q "$@"; }
# Up on one database or the other; --no-deps, or Postgres would be recreated
# for the name it was made with. A second argument raises the operator's login
# limit: the suite signs in from one address, and waiting it out costs a minute
# a login. Back on the development database it is empty, so the limit is real.
point_at() { DB_NAME="$1" MANAGER_LOGIN_BURST="${2:-}" "${COMPOSE[@]}" up -d --no-deps --wait api worker scheduler >/dev/null 2>&1; }
flush() {
  "${COMPOSE[@]}" exec -T redis redis-cli -a "$(env_value REDIS_PASSWORD)" --no-auth-warning FLUSHALL >/dev/null \
    || log_warn "could not flush the cache"
}
restore() {
  local rc=$?
  trap - EXIT
  log_step "Back to $DEV_DB"
  if point_at "$DEV_DB"; then flush; log_ok "the stack is on $DEV_DB"; else log_warn "the API is not back on $DEV_DB - run: make up"; fi
  exit "$rc"
}

log_step "A new $E2E_DB"
psql_dev -d postgres -c "DROP DATABASE IF EXISTS \"$E2E_DB\" WITH (FORCE);" -c "CREATE DATABASE \"$E2E_DB\";" </dev/null \
  || die "could not recreate $E2E_DB"
psql_dev -d "$E2E_DB" -v app_role="${DB_USER}_app" < "$ROOT/docker/postgres/init/grant-app.psql" >/dev/null \
  || die "could not grant ${DB_USER}_app"
"${COMPOSE[@]}" exec -T -e DATABASE_ADMIN_URL="postgres://$DB_USER:$(env_value DB_PASSWORD)@postgres:5432/$E2E_DB" \
  api go run ./cmd/openenu migrate >/dev/null || die "the migrations failed"
log_ok "migrated"

trap restore EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

log_step "Pointing the API, the worker and the scheduler at $E2E_DB"
point_at "$E2E_DB" 1000 || die "they did not come up on $E2E_DB"
flush
log_ok "ready"

bash "$HERE/e2e-fixtures.sh" || die "the fixtures failed"

"${COMPOSE[@]}" --profile e2e up -d --no-deps --wait webhook-echo >/dev/null 2>&1 || die "the webhook receiver did not come up"
"${COMPOSE[@]}" --profile e2e run --rm --no-deps e2e npx playwright test "$@"
