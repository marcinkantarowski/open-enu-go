#!/usr/bin/env bash
# =============================================================================
#  Put a backup back (.ai/platform/docs/deployment.md).
# =============================================================================
#  Destructive by definition, so it asks. The confirmation is the database name
#  rather than "yes": typing the name means having read which database is about
#  to be replaced.
#
#    1. a safety backup of the current state (backup.sh pre-restore)
#    2. stop api, worker and scheduler - the only writers of the database and
#       of the storage volume; postgres stays up
#    3. the database: drop, create, pg_restore, then the application role's
#       grants (docker/postgres/init/grant-app.psql) - they are per database,
#       and without them every query the API runs is "permission denied"
#    4. the storage volume: emptied, then the archive of the SAME run
#       (<db>-<stamp>-<label>-storage.tar[.age]) unpacked into it
#    5. `openenu migrate` - an older dump may predate the current schema
#    6. start everything again (stack.sh up: the stack and the edge)
#
#  Usage: restore.sh HOST [BACKUP_FILE]
#         BACKUP_FILE is the dump (<name>.dump or <name>.dump.age); it defaults to the
#         latest one at BACKUP_REMOTE. Its storage archive is found by name.
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
BACKUP_REMOTE="$(REMOTE "grep '^BACKUP_REMOTE=' $APP_DIR/.env | cut -d= -f2-" || true)"

if [ -z "$WHICH" ]; then
  [ -n "$BACKUP_REMOTE" ] || die "no BACKUP_REMOTE set and no file given"
  WHICH="$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '*.dump' --include '*.dump.age' | sort | tail -1")"
  [ -n "$WHICH" ] || die "no backups found at $BACKUP_REMOTE"
fi
case "$WHICH" in
  *.dump|*.dump.age) ;;
  *) die "$WHICH is not a dump" "name the .dump or .dump.age file; its storage archive is found from it" ;;
esac

# The storage archive of the same run: <db>-<stamp>-<label>.dump[.age] →
# <db>-<stamp>-<label>-storage.tar[.age]. Looked for locally, then remotely.
STORAGE_WHICH="${WHICH/.dump/-storage.tar}"
if REMOTE "test -f $APP_DIR/backups/$STORAGE_WHICH" 2>/dev/null \
   || { [ -n "$BACKUP_REMOTE" ] && [ -n "$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '$STORAGE_WHICH'" 2>/dev/null)" ]; }; then
  HAS_STORAGE=1
else
  HAS_STORAGE=0; STORAGE_WHICH=""
fi

log_warn "About to REPLACE the contents of $DB_NAME on $HOST with:"
log_info "  $WHICH"
if [ "$HAS_STORAGE" -eq 1 ]; then
  log_warn "and the storage volume (uploads) with:"
  log_info "  $STORAGE_WHICH"
else
  log_warn "there is NO storage archive for this dump - the uploads on the server stay as they are,"
  log_info "  and files uploaded after this dump will be on disk with no row pointing at them"
fi
log_info "Everything written since that backup will be gone."
echo
printf '  Type the database name (%s) to confirm: ' "$DB_NAME"
read -r answer </dev/tty
[ "$answer" = "$DB_NAME" ] || { log_info "aborted - nothing was changed"; exit 1; }

# A backup of the current state first, always. The most common restore mistake
# is restoring the wrong backup, and without this there is no way back from it.
# backup.sh's exit codes (its header): 0 complete, 3/4 local or partial, else none.
log_step "Backing up the current state first"
REMOTE "cd $APP_DIR && bash scripts/remote/backup.sh pre-restore" >/dev/null 2>&1
case $? in
  0)   log_ok "safety backup taken" ;;
  3)   log_warn "safety backup is on the server only - the copy off the box failed" ;;
  4)   log_warn "safety backup has the database but not the uploads" ;;
  *)   log_warn "could not take a safety backup - continuing, but there is no way back from this" ;;
esac

log_step "Stopping the writers (api, worker, scheduler) - the database stays up"
REMOTE_BASH "$APP_DIR" >/dev/null 2>&1 <<'SH' || die "could not stop the application - nothing was restored"
cd "$1" && . scripts/lib/compose.sh && compose_init "$PWD" && "${COMPOSE[@]}" stop api worker scheduler
SH

log_step "Restoring"
# Every command that could read stdin gets /dev/null or a file: stdin is this script.
REMOTE_BASH "$APP_DIR" "$WHICH" "$DB_NAME" "$STORAGE_WHICH" <<'SH' \
  || die "the restore failed - the application is still stopped" \
         "the safety backup is in $APP_DIR/backups (*-pre-restore*)" \
         "start it again as it is: ssh $HOST 'cd $APP_DIR && bash scripts/remote/stack.sh up'"
set -euo pipefail
APP_DIR="$1"; WHICH="$2"; DB_NAME="$3"; STORAGE_WHICH="$4"
cd "$APP_DIR"
. scripts/lib/compose.sh
compose_init "$APP_DIR"
env_of() { grep "^$1=" .env | cut -d= -f2- || true; }
DB_USER="$(env_of DB_USER)"; STACK="$(env_of STACK)"; KEY="$(env_of BACKUP_AGE_KEY_FILE)"
work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
chmod 700 "$work"

# fetch <file> <plain output> - local copy first, then the remote; decrypt.
fetch() {
  if [ -f "backups/$1" ]; then
    cp "backups/$1" "$work/"
  else
    rclone copy "$(env_of BACKUP_REMOTE)/$1" "$work/" --quiet </dev/null
  fi
  [ -s "$work/$1" ] || { echo "could not fetch $1" >&2; exit 1; }
  case "$1" in
    *.age)
      [ -n "$KEY" ] || { echo 'encrypted backup and no BACKUP_AGE_KEY_FILE on this server' >&2; exit 1; }
      age --decrypt -i "$KEY" -o "$2" "$work/$1" </dev/null ;;
    *) cp "$work/$1" "$2" ;;
  esac
}

fetch "$WHICH" "$work/restored.dump"
echo "  · fetched $WHICH"
if [ -n "$STORAGE_WHICH" ]; then
  fetch "$STORAGE_WHICH" "$work/restored-storage.tar"
  tar -tf "$work/restored-storage.tar" >/dev/null || { echo "the storage archive is not a readable tar" >&2; exit 1; }
  echo "  · fetched $STORAGE_WHICH"
fi

# ── the database ──
"${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U "$DB_USER" -d postgres \
  -c "DROP DATABASE IF EXISTS \"$DB_NAME\" WITH (FORCE);" -c "CREATE DATABASE \"$DB_NAME\";" </dev/null >/dev/null
"${COMPOSE[@]}" exec -T postgres pg_restore -U "$DB_USER" -d "$DB_NAME" --no-owner --no-privileges < "$work/restored.dump"
echo "  ✓ database restored"
# The grants 01-init.sh gave the application role went with the dropped
# database. The same file, applied the same way.
"${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -q -U "$DB_USER" -d "$DB_NAME" \
  -v app_role="${DB_USER}_app" < docker/postgres/init/grant-app.psql >/dev/null
echo "  ✓ ${DB_USER}_app granted again (docker/postgres/init/grant-app.psql)"

# ── the storage volume ──
if [ -n "$STORAGE_WHICH" ]; then
  VOLUME="$(docker volume ls -q --filter "label=com.docker.compose.project=$STACK" \
                                --filter "label=com.docker.compose.volume=storage" | head -1)"
  [ -n "$VOLUME" ] || { echo "no storage volume for project $STACK" >&2; exit 1; }
  PG_IMAGE="$(docker inspect -f '{{.Config.Image}}' "$("${COMPOSE[@]}" ps -q postgres | head -1)")"
  # Emptied first: a restore is the state of that night, not that night plus
  # whatever was uploaded since. The numeric owners in the archive are kept,
  # so the API's user still owns its files.
  docker run --rm -i --network none -v "$VOLUME:/s" --entrypoint sh "$PG_IMAGE" \
    -c 'find /s -mindepth 1 -delete && tar --numeric-owner -C /s -xf -' < "$work/restored-storage.tar"
  echo "  ✓ storage volume $VOLUME restored"
fi

# ── the schema the code expects ──
# An older dump may predate migrations the running release has. The same image
# that will serve, as the schema owner, exactly as release.sh runs it.
"${COMPOSE[@]}" run --rm -T --no-deps api migrate </dev/null >/dev/null
echo "  ✓ migrated to the running release's schema"
SH

log_step "Starting the application"
REMOTE "cd $APP_DIR && bash scripts/remote/stack.sh up" >/dev/null 2>&1 \
  || die "the restore is done but the stack did not start" "ssh $HOST 'cd $APP_DIR && bash scripts/remote/stack.sh up'"

log_ok "restored $WHICH into $DB_NAME$([ "$HAS_STORAGE" -eq 1 ] && echo " and $STORAGE_WHICH into the storage volume")"
log_info "check: curl -fsS -H \"X-Health-Token: \$HEALTH_TOKEN\" https://api.\$DOMAIN/health/deep (HEALTH_TOKEN from the server's .env)"
