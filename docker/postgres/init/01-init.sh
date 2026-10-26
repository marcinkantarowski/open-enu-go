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
#  Default privileges, so a table a later migration creates is readable by the
#  application role without the migration having to remember a GRANT.
# =============================================================================
set -euo pipefail

APP_ROLE="${POSTGRES_USER}_app"
: "${DB_APP_PASSWORD:?DB_APP_PASSWORD is not set - run: make env}"

psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
     -v app_role="$APP_ROLE" -v app_password="$DB_APP_PASSWORD" <<'SQL'
-- Installed but unused by the shipped search, which is tsvector only (ADR-0017):
-- a trigram index or a fuzzy driver then needs no infrastructure change.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

SELECT format('CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE', :'app_role', :'app_password')
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = :'app_role')\gexec

SELECT format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), :'app_role')\gexec
SELECT format('GRANT USAGE ON SCHEMA public TO %I', :'app_role')\gexec
SELECT format('GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO %I', :'app_role')\gexec
SELECT format('GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO %I', :'app_role')\gexec
SELECT format('ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I', :'app_role')\gexec
SELECT format('ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %I', :'app_role')\gexec
SQL
echo "[init] application role $APP_ROLE ready (NOSUPERUSER NOBYPASSRLS)"
