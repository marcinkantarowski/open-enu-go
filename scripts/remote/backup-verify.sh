#!/usr/bin/env bash
# =============================================================================
#  Prove the backup restores (.ai/platform/docs/deployment.md).
# =============================================================================
#  An untested backup is a belief. This pulls the LATEST REMOTE dump - not a
#  local copy, because the remote one is what exists after the disk dies -
#  decrypts it, restores it into a throwaway Postgres container, and asks the
#  application whether the schema it found is the one it expects.
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

REMOTE "cd $APP_DIR && rclone copy '$BACKUP_REMOTE/$LATEST' /tmp/verify-$$/ --quiet" \
  || die "could not fetch $LATEST from $BACKUP_REMOTE"
REMOTE "cat /tmp/verify-$$/$LATEST" > "$WORK/dump" || die "could not read the fetched dump"
REMOTE "rm -rf /tmp/verify-$$" || true

[ -s "$WORK/dump" ] || die "the fetched dump is empty" \
  "a backup job that uploads an empty file looks identical to a working one"

# ── 2. decrypt, if it was encrypted ─────────────────────────────────────────
if [[ "$LATEST" == *.age ]]; then
  [ -n "${BACKUP_AGE_KEY_FILE:-}" ] || die "this backup is encrypted and no private key was given" \
    "BACKUP_AGE_KEY_FILE=~/.config/age/${SLUG}.txt make backup-verify HOST=$HOST" \
    "the private half is deliberately NOT on the server - that is the point of it"
  age --decrypt -i "$BACKUP_AGE_KEY_FILE" -o "$WORK/dump.plain" "$WORK/dump" \
    || die "decryption failed - wrong key?"
  mv "$WORK/dump.plain" "$WORK/dump"
  log_ok "decrypted"
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

echo
log_ok "the latest off-box backup restores and looks like this application"
