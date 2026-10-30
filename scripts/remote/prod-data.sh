#!/usr/bin/env bash
# =============================================================================
#  Production's data, read out for a development copy. Runs ON the server.
# =============================================================================
#  scripts/remote/copyproddata.sh drives it over SSH. Read-only: both actions
#  write to stdout and nowhere else - no file on the server, nothing changed.
#
#    dump        pg_dump of the database, custom format
#    encrypted   every encrypted value in the open, one JSON object per line
#                (openenu encrypted export) - so the copy can be sealed with
#                the development key, and this server's key stays here
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

cd "$ROOT" || die "cannot enter $ROOT"
compose_init "$ROOT"

DB_USER="$(sed -n 's/^DB_USER=//p' .env | head -1)"
DB_NAME="$(sed -n 's/^DB_NAME=//p' .env | head -1)"
[ -n "$DB_USER" ] && [ -n "$DB_NAME" ] || die "no DB_USER or DB_NAME in $ROOT/.env"

case "${1:-}" in
  dump)
    exec "${COMPOSE[@]}" exec -T postgres pg_dump -Fc --no-owner --no-privileges -U "$DB_USER" "$DB_NAME"
    ;;
  encrypted)
    exec "${COMPOSE[@]}" exec -T api /openenu encrypted export
    ;;
  *) die "usage: prod-data.sh dump|encrypted" ;;
esac
