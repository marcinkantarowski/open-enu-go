#!/usr/bin/env bash
# =============================================================================
#  Prove the backup restores (.ai/platform/docs/deployment.md).
# =============================================================================
#  An untested backup is a belief. This pulls the LATEST REMOTE dump - not a
#  local copy, because the remote one is what exists after the disk dies -
#  and the storage archive of the same run, decrypts both, restores the dump
#  into a throwaway Postgres container, and asks whether the schema it found
#  is the one the application expects.
#
#  It restores the way restore.sh does, and then reads as the APPLICATION
#  ROLE, not as a superuser: a superuser reads every restored table whether or
#  not the grants came back, so a check made as one cannot see the failure
#  that matters (every API query "permission denied"). The role is created
#  NOBYPASSRLS, shown to be refused BEFORE docker/postgres/init/grant-app.psql
#  runs and to read AFTER it - the check proves itself each time.
#
#  Nothing here touches the live database. The scratch container is removed on
#  the way out, including on failure.
#
#  Usage: backup-verify.sh HOST
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"
[ -n "$HOST" ] || die "usage: backup-verify.sh HOST" "example: make backup-verify HOST=deploy@203.0.113.42"

. "$HERE/../lib/stage.sh"
SLUG="$(project_slug "$ROOT")"
# Production's. Staging has no backups: its data is disposable by design.
APP_DIR="$(stage_dir "$SLUG" production)"
SCRATCH="${SLUG}-verify-$$"
WORK="$(mktemp -d)"

cleanup() {
  docker rm -f "$SCRATCH" >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

REMOTE() { ssh -o BatchMode=yes "$HOST" "$@"; }

log_step "Verifying the latest off-box backup"

# ── 1. the latest remote object ─────────────────────────────────────────────
BACKUP_REMOTE="$(REMOTE "grep '^BACKUP_REMOTE=' $APP_DIR/.env 2>/dev/null | cut -d= -f2-" || true)"
[ -n "$BACKUP_REMOTE" ] || die "BACKUP_REMOTE is not set on $HOST" \
  "there is no off-box backup to verify - set it and let one nightly run complete"

LATEST="$(REMOTE "cd $APP_DIR && rclone lsf '$BACKUP_REMOTE/' --include '*.dump*' 2>/dev/null | sort | tail -1")"
[ -n "$LATEST" ] || die "no dump found at $BACKUP_REMOTE" \
  "the nightly timer has not produced one yet: ssh $HOST 'systemctl status ${SLUG}-prod-backup.timer'"

log_ok "latest: $LATEST"

# The storage archive of the same run (backup.sh names them together).
STORAGE_LATEST="${LATEST/.dump/-storage.tar}"

# fetch <remote name> <local file>
fetch() {
  REMOTE "cd $APP_DIR && rclone copy '$BACKUP_REMOTE/$1' /tmp/verify-$$/ --quiet" \
    || die "could not fetch $1 from $BACKUP_REMOTE"
  REMOTE "cat /tmp/verify-$$/$1" > "$2" || die "could not read the fetched $1"
  [ -s "$2" ] || die "the fetched $1 is empty" \
    "a backup job that uploads an empty file looks identical to a working one"
}
fetch "$LATEST" "$WORK/dump"
[ -n "$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '$STORAGE_LATEST' 2>/dev/null")" ] \
  || die "no storage archive ($STORAGE_LATEST) next to $LATEST" \
    "the uploads never left the box with this backup - a backup made before the storage volume was archived, or one whose archive failed (backup.sh exit 4)" \
    "take a fresh one: make backup HOST=$HOST"
fetch "$STORAGE_LATEST" "$WORK/storage"
REMOTE "rm -rf /tmp/verify-$$" || true

# ── 2. decrypt, if it was encrypted ─────────────────────────────────────────
for pair in "$LATEST:dump" "$STORAGE_LATEST:storage"; do
  name="${pair%%:*}"; file="$WORK/${pair##*:}"
  [[ "$name" == *.age ]] || continue
  [ -n "${BACKUP_AGE_KEY_FILE:-}" ] || die "this backup is encrypted and no private key was given" \
    "make backup-verify HOST=$HOST BACKUP_AGE_KEY_FILE=\$HOME/.config/age/${SLUG}.txt   # an argument: the Makefile loads .env, and an empty value there beats the environment" \
    "the private half is deliberately NOT on the server - that is the point of it"
  age --decrypt -i "$BACKUP_AGE_KEY_FILE" -o "$file.plain" "$file" \
    || die "decrypting $name failed - wrong key?"
  mv "$file.plain" "$file"
  log_ok "decrypted $name"
done

# ── 2b. the storage archive is a readable tar ───────────────────────────────
# Listed in full, so a truncated archive fails here and not on the night it is
# unpacked. An empty one is legal (nothing uploaded yet) and said.
ENTRIES="$(tar -tf "$WORK/storage" 2>/dev/null)" || die "the storage archive $STORAGE_LATEST is not a readable tar"
FILES="$(grep -cv '/$' <<<"$ENTRIES" || true)"
if [ "${FILES:-0}" -eq 0 ]; then
  log_warn "the storage archive lists no files - fine before the first upload, wrong after it"
else
  log_ok "storage archive: $FILES file(s), $(du -h "$WORK/storage" | cut -f1)"
fi

# ── 3. restore into a scratch database ──────────────────────────────────────
log_step "Restoring into a scratch container"
docker run -d --name "$SCRATCH" \
  -e POSTGRES_PASSWORD=verify -e POSTGRES_USER=verify -e POSTGRES_DB=verify \
  pgvector/pgvector:pg18 >/dev/null || die "could not start the scratch database"

for _ in $(seq 1 30); do
  docker exec "$SCRATCH" pg_isready -U verify >/dev/null 2>&1 && break
  sleep 1
done
docker exec "$SCRATCH" pg_isready -U verify >/dev/null 2>&1 || die "the scratch database never became ready"

docker exec -i "$SCRATCH" pg_restore -U verify -d verify --no-owner --no-privileges < "$WORK/dump" 2>/dev/null
# pg_restore warns about extensions and ownership it cannot reproduce as a
# different user. Those are expected; an empty database is not, which is what
# the row counts below actually check.

# ── 4. does it look like this application's database ────────────────────────
log_step "Sanity checks"
TABLES="$(docker exec "$SCRATCH" psql -U verify -d verify -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema='public';" 2>/dev/null | tr -d ' ')"

[ "${TABLES:-0}" -ge 10 ] || die "only ${TABLES:-0} tables restored" \
  "a real dump of this application has more than ten; this one is truncated or wrong"
log_ok "$TABLES tables"

# One goose ledger per migration source (kernel/migrate: goose_<source>).
for table in tenant identity_user goose_kernel goose_tenant; do
  docker exec "$SCRATCH" psql -U verify -d verify -tAc "SELECT 1 FROM $table LIMIT 1;" >/dev/null 2>&1 \
    || die "table $table is missing or unreadable in the restored database"
done
log_ok "tenant, identity_user and the migration ledgers are present"

APPLIED="$(docker exec "$SCRATCH" psql -U verify -d verify -tAc \
  "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'goose_%';" 2>/dev/null | tr -d ' ')"
[ "${APPLIED:-0}" -ge 2 ] || die "the restored database has no migration ledgers"
log_ok "$APPLIED migration ledger(s) recorded"

# ── 5. readable by the application role, exactly as restore.sh leaves it ────
log_step "As the application role (NOBYPASSRLS), the way the API connects"
APP_ROLE=verify_app
q_app() { docker exec "$SCRATCH" psql -U "$APP_ROLE" -d verify -tAc "$1" 2>/dev/null; }
docker exec "$SCRATCH" psql -v ON_ERROR_STOP=1 -U verify -d verify -qc \
  "CREATE ROLE $APP_ROLE LOGIN NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;" >/dev/null \
  || die "could not create the application role in the scratch database"
# Before the grants the role must be refused - or this check could not tell a
# restore that forgot them from one that did not.
if q_app "SELECT count(*) FROM goose_kernel;" >/dev/null; then
  die "the application role read goose_kernel before any GRANT" \
    "the scratch database is not reproducing a restore; this check would prove nothing"
fi
log_ok "refused before the grants - as a restore without them would be"
docker exec -i "$SCRATCH" psql -v ON_ERROR_STOP=1 -q -U verify -d verify -v app_role="$APP_ROLE" \
  < "$ROOT/docker/postgres/init/grant-app.psql" >/dev/null \
  || die "docker/postgres/init/grant-app.psql failed against the restored database"
ROWS="$(q_app "SELECT count(*) FROM goose_kernel;" | tr -d ' ')"
[ -n "$ROWS" ] && [ "$ROWS" -ge 1 ] || die "the application role cannot read the restored database after grant-app.psql" \
  "restore.sh applies the same file - a restore today would answer every API query with permission denied"
log_ok "the application role reads it after grant-app.psql ($ROWS goose_kernel rows)"

echo
log_ok "the latest off-box backup restores, with its uploads, and the application role can read it"
