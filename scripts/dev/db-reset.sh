#!/usr/bin/env bash
# =============================================================================
#  make db-reset - an empty development database, with the e2e accounts.
# =============================================================================
#  The way back from make copyproddata. Refuses anything but a local stack.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

env_value() { sed -n "s/^$1=//p" "$ROOT/.env" | head -1; }
[ "$(env_value APP_STAGE)" = local ] || die "APP_STAGE is not local in .env" "this empties a development database only"

compose_init "$ROOT"
DB_USER="$(env_value DB_USER)"; DB_NAME="$(env_value DB_NAME)"
[ -n "$DB_USER" ] && [ -n "$DB_NAME" ] || die "no DB_USER or DB_NAME in .env"
psql_dev() { "${COMPOSE[@]}" exec -T postgres psql -U "$DB_USER" -v ON_ERROR_STOP=1 -q "$@"; }

log_step "Recreating $DB_NAME"
"${COMPOSE[@]}" stop worker scheduler >/dev/null 2>&1 || die "could not stop the worker and the scheduler"
psql_dev -d postgres -c "DROP DATABASE IF EXISTS \"$DB_NAME\" WITH (FORCE);" -c "CREATE DATABASE \"$DB_NAME\";" </dev/null \
  || die "could not recreate $DB_NAME"
psql_dev -d "$DB_NAME" -v app_role="${DB_USER}_app" < "$ROOT/docker/postgres/init/grant-app.psql" >/dev/null \
  || die "could not grant ${DB_USER}_app"
"${COMPOSE[@]}" exec -T api go run ./cmd/openenu migrate >/dev/null || die "the migrations failed"
"${COMPOSE[@]}" exec -T redis redis-cli -a "$(env_value REDIS_PASSWORD)" --no-auth-warning FLUSHALL >/dev/null \
  || log_warn "could not flush the cache"
"${COMPOSE[@]}" up -d worker scheduler >/dev/null 2>&1 || die "could not start the worker and the scheduler"
log_ok "$DB_NAME is empty and migrated"
bash "$HERE/e2e-fixtures.sh"
