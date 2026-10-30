#!/bin/bash
# =============================================================================
#  Runs once, on an empty data volume, as the database owner.
# =============================================================================
#  Two roles, deliberately (ADR-0009). ${POSTGRES_USER} owns every table and
#  runs migrations (DATABASE_ADMIN_URL). The application connects as
#  ${POSTGRES_USER}_app, which is NOSUPERUSER and NOBYPASSRLS, so the row-level
#  policies the migrations declare bind every query it runs - the dev stack
#  connects exactly as production will, or the policies are untested where
#  they matter. The kernel's test harness (kernel/testdb) does the same.
#
#  The role is created here. What it may do in the database is grant-app.psql,
#  next to this file, because a restore has to apply the same grants again:
#  they are per database and `DROP DATABASE` takes them with it.
# =============================================================================
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_ROLE="${POSTGRES_USER}_app"
: "${DB_APP_PASSWORD:?DB_APP_PASSWORD is not set - run: make env}"

psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
     -v app_role="$APP_ROLE" -v app_password="$DB_APP_PASSWORD" <<'SQL'
-- Installed but unused by the shipped search, which is tsvector only (ADR-0017):
-- a trigram index or a fuzzy driver then needs no infrastructure change.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE', :'app_role', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'app_role')\gexec
SQL

# Default privileges, so a table a later migration creates is readable by the
# application role without the migration having to remember a GRANT.
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
     -v app_role="$APP_ROLE" -f "$HERE/grant-app.psql"
echo "[init] application role $APP_ROLE ready (NOSUPERUSER NOBYPASSRLS)"
