#!/usr/bin/env bash
# =============================================================================
#  Put a backup back (.ai/platform/docs/deployment.md).
# =============================================================================
#  Destructive by definition, so it asks. The confirmation is the database name
#  rather than "yes": typing the name means having read which database is about
#  to be replaced.
#
#  Usage: restore.sh HOST [BACKUP_FILE]
#         BACKUP_FILE defaults to the latest object at BACKUP_REMOTE.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"; WHICH="${2:-}"
[ -n "$HOST" ] || die "usage: restore.sh HOST [BACKUP_FILE]"

. "$HERE/../lib/stage.sh"
SLUG="$(project_slug "$ROOT")"
# Production's. Staging has no backups: its data is disposable by design.
APP_DIR="$(stage_dir "$SLUG" production)"

REMOTE() { ssh -o BatchMode=yes "$HOST" "$@"; }
# A script piped to bash on the server, arguments after `--`. Compose there is
# what the server's checkout defines - COMPOSE_FILE in its .env, read by
# scripts/lib/compose.sh - never a file named here: naming one is how an
# override gets dropped (make prod-check refuses it).
REMOTE_BASH() { ssh -o BatchMode=yes "$HOST" bash -s -- "$@"; }

DB_NAME="$(REMOTE "grep '^DB_NAME=' $APP_DIR/.env | cut -d= -f2-")"
[ -n "$DB_NAME" ] || die "could not read DB_NAME from $HOST:$APP_DIR/.env"

if [ -z "$WHICH" ]; then
  BACKUP_REMOTE="$(REMOTE "grep '^BACKUP_REMOTE=' $APP_DIR/.env | cut -d= -f2-" || true)"
  [ -n "$BACKUP_REMOTE" ] || die "no BACKUP_REMOTE set and no file given"
  WHICH="$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '*.dump*' | sort | tail -1")"
  [ -n "$WHICH" ] || die "no backups found at $BACKUP_REMOTE"
fi

log_warn "About to REPLACE the contents of $DB_NAME on $HOST with:"
log_info "  $WHICH"
log_info "Everything written since that dump will be gone."
echo
printf '  Type the database name (%s) to confirm: ' "$DB_NAME"
read -r answer </dev/tty
[ "$answer" = "$DB_NAME" ] || { log_info "aborted - nothing was changed"; exit 1; }

# A dump of the current state first, always. The most common restore mistake is
# restoring the wrong backup, and without this there is no way back from it.
log_step "Dumping the current state first"
if REMOTE "cd $APP_DIR && bash scripts/remote/backup.sh pre-restore" >/dev/null 2>&1; then
  log_ok "safety dump taken"
elif REMOTE "ls $APP_DIR/backups/*-pre-restore.dump* >/dev/null 2>&1"; then
  log_warn "safety dump is on the server only - the copy off the box failed"
else
  log_warn "could not take a safety dump - continuing, but there is no way back from this"
fi

log_step "Stopping the application (leaving the database up)"
REMOTE_BASH "$APP_DIR" >/dev/null 2>&1 <<'SH'
cd "$1" && . scripts/lib/compose.sh && compose_init "$PWD" && "${COMPOSE[@]}" stop api worker scheduler
SH

log_step "Restoring"
# Every command that could read stdin gets /dev/null: stdin is this script.
REMOTE_BASH "$APP_DIR" "$WHICH" "$DB_NAME" <<'SH' \
  || die "the restore failed - the application is still stopped; the safety dump is in $APP_DIR/backups"
set -euo pipefail
APP_DIR="$1"; WHICH="$2"; DB_NAME="$3"
cd "$APP_DIR"
. scripts/lib/compose.sh
compose_init "$APP_DIR"
env_of() { grep "^$1=" .env | cut -d= -f2- || true; }
DB_USER="$(env_of DB_USER)"; KEY="$(env_of BACKUP_AGE_KEY_FILE)"
work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
if [ -f "backups/$WHICH" ]; then
  cp "backups/$WHICH" "$work/"
else
  rclone copy "$(env_of BACKUP_REMOTE)/$WHICH" "$work/" --quiet </dev/null
fi
case "$WHICH" in
  *.age)
    [ -n "$KEY" ] || { echo 'encrypted backup and no BACKUP_AGE_KEY_FILE on this server' >&2; exit 1; }
    age --decrypt -i "$KEY" -o "$work/restored.dump" "$work/$WHICH" </dev/null ;;
  *) cp "$work/$WHICH" "$work/restored.dump" ;;
esac
"${COMPOSE[@]}" exec -T postgres psql -U "$DB_USER" -d postgres \
  -c "DROP DATABASE IF EXISTS $DB_NAME WITH (FORCE);" -c "CREATE DATABASE $DB_NAME;" </dev/null
"${COMPOSE[@]}" exec -T postgres pg_restore -U "$DB_USER" -d "$DB_NAME" --no-owner --no-privileges < "$work/restored.dump"
SH

log_step "Starting the application"
REMOTE_BASH "$APP_DIR" >/dev/null 2>&1 <<'SH'
cd "$1" && . scripts/lib/compose.sh && compose_init "$PWD" && "${COMPOSE[@]}" up -d
SH

log_ok "restored $WHICH into $DB_NAME"
log_info "check: curl -fsS https://api.\$DOMAIN/health/deep"
