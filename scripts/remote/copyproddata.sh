#!/usr/bin/env bash
# =============================================================================
#  make copyproddata - production's database, mirrored into the dev stack.
# =============================================================================
#  REPLACES the development database with a copy of production's: real people
#  and real tenants, on this machine. Keep the disk encrypted, and the copy off
#  anything shared.
#
#  Production is only read (scripts/remote/prod-data.sh): the dump and the
#  encrypted values come over SSH on stdout, and no file is left there.
#
#    1. refuse unless this is a local stack that cannot reach the outside:
#       APP_STAGE=local, not production's domain, mail going to Mailpit
#    2. stop the worker and the scheduler, so nothing acts on the copy before
#       it is made safe
#    3. dump production, drop and recreate the dev database, restore
#    4. empty the River queue and switch every webhook endpoint off - a copied
#       job or endpoint would otherwise call a customer's server from here
#    5. migrate, in case this checkout is ahead of production, and grant
#    6. re-encrypt: production opens every encrypted value and this stack seals
#       it with ITS key, recomputing the lookup hashes. The production master
#       key never leaves the server - it opens every tenant's data
#    7. flush the cache and start everything again
#
#  Uploaded files are not copied.
#
#  Usage: copyproddata.sh <prod-host>
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"; . "$HERE/../lib/stage.sh"

PROD="${1:-}"
[ -n "$PROD" ] || die "usage: copyproddata.sh <prod-host>"

APP_DIR="$(stage_dir "$(project_slug "$ROOT")" production)"
on_prod() { ssh -o BatchMode=yes "$PROD" "cd $APP_DIR && $*"; }

env_value() { sed -n "s/^$1=//p" "$ROOT/.env" | head -1; }

# ── 1. only into a local stack that cannot reach the outside ─────────────────
[ "$(env_value APP_STAGE)" = local ] || die "APP_STAGE is not local in .env" \
  "the copy goes into a development stack only"
[ "$(env_value DOMAIN)" != "$(env_value PROD_DOMAIN)" ] || die "DOMAIN is production's domain" \
  "this is production's checkout, not a development one"
case "$(env_value MAILER_DSN)" in
  *mailpit*) ;;
  *) die "MAILER_DSN does not point at Mailpit" \
       "with it, the copy would mail real people" \
       "set smtp://mailpit:1025 and run again" ;;
esac

compose_init "$ROOT"
DB_USER="$(env_value DB_USER)"; DB_NAME="$(env_value DB_NAME)"
[ -n "$DB_USER" ] && [ -n "$DB_NAME" ] || die "no DB_USER or DB_NAME in .env"
[ -n "$("${COMPOSE[@]}" ps -q --status running api 2>/dev/null)" ] || die "the dev stack is not running" "start it: make up"

openenu() { "${COMPOSE[@]}" exec -T api go run ./cmd/openenu "$@"; }
psql_dev() { "${COMPOSE[@]}" exec -T postgres psql -U "$DB_USER" -v ON_ERROR_STOP=1 -q "$@"; }

on_prod test -f scripts/remote/prod-data.sh 2>/dev/null || die "production ($PROD) has no scripts/remote/prod-data.sh" \
  "it arrives with this checkout's code: push, then make deployprod"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

log_warn "This REPLACES the development database ($DB_NAME) with production's, real people included."

# ── 2-3. the dump, into a database nothing is working on ─────────────────────
log_step "Dumping production ($PROD)"
on_prod bash scripts/remote/prod-data.sh dump > "$WORK/prod.dump" || die "the dump failed"
[ -s "$WORK/prod.dump" ] || die "the dump is empty"
log_ok "$(du -h "$WORK/prod.dump" | cut -f1)"

log_step "Stopping the worker and the scheduler"
"${COMPOSE[@]}" stop worker scheduler >/dev/null 2>&1 || die "could not stop them"
log_ok "stopped"

log_step "Restoring into $DB_NAME"
psql_dev -d postgres -c "DROP DATABASE IF EXISTS \"$DB_NAME\" WITH (FORCE);" -c "CREATE DATABASE \"$DB_NAME\";" </dev/null \
  || die "could not recreate $DB_NAME"
"${COMPOSE[@]}" exec -T postgres pg_restore -U "$DB_USER" -d "$DB_NAME" --no-owner --no-privileges --single-transaction < "$WORK/prod.dump" \
  || die "the restore failed - $DB_NAME is empty; run again, or start over: make db-reset"
rm -f "$WORK/prod.dump"
log_ok "restored"

# ── 4. nothing copied may call out ───────────────────────────────────────────
log_step "Switching off what would reach the outside"
psql_dev -d "$DB_NAME" <<'SQL' || die "could not switch them off - the worker and the scheduler stay stopped"
DO $$
BEGIN
  IF to_regclass('public.river_job') IS NOT NULL THEN
    DELETE FROM river_job;
  END IF;
  IF to_regclass('public.webhook_endpoint') IS NOT NULL THEN
    UPDATE webhook_endpoint SET active = false;
  END IF;
END $$;
SQL
log_ok "queue emptied, webhook endpoints off"

# ── 5-6. this checkout's schema, this stack's key ────────────────────────────
log_step "Migrating"
openenu migrate >/dev/null || die "the migrations failed"
psql_dev -d "$DB_NAME" -v app_role="${DB_USER}_app" < "$ROOT/docker/postgres/init/grant-app.psql" >/dev/null \
  || die "could not grant ${DB_USER}_app"
log_ok "up to date, ${DB_USER}_app granted"

log_step "Re-encrypting with the development key"
on_prod bash scripts/remote/prod-data.sh encrypted | openenu encrypted import \
  || die "re-encrypting failed - the copy's e-mails cannot be read until it succeeds; run again"

# ── 7. go ────────────────────────────────────────────────────────────────────
log_step "Starting again"
"${COMPOSE[@]}" exec -T redis redis-cli -a "$(env_value REDIS_PASSWORD)" --no-auth-warning FLUSHALL >/dev/null \
  || log_warn "could not flush the cache"
"${COMPOSE[@]}" up -d worker scheduler >/dev/null 2>&1 || die "could not start the worker and the scheduler" "start them: make up"
log_ok "running"

log_ok "the development database is a copy of production"
log_info "sign in as on production: app.$(env_value DOMAIN), manager.$(env_value DOMAIN)"
log_info "mail goes to Mailpit, webhooks nowhere; uploaded files were not copied"
log_info "back to an empty development database: make db-reset"
