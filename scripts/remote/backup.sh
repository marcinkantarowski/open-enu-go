#!/usr/bin/env bash
# =============================================================================
#  The nightly backup. Runs ON the server (.ai/platform/docs/deployment.md).
# =============================================================================
#  A pg_dump on the same disk as the database is a hope, not a backup. The disk
#  is what fails, and the dump fails with it.
#
#    pg_dump -Fc  →  age --encrypt  →  rclone copy to an off-box bucket
#
#  Encrypted before it leaves, so the bucket holds nothing readable. The private
#  half of the age key is deliberately NOT on this server - a backup an attacker
#  with the server can decrypt is a backup that protects against disk failure
#  and nothing else.
#
#  Usage: backup.sh [--pre-migrate]
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

cd "$ROOT" || die "cannot enter $ROOT"
set -a; . ./.env; set +a

compose_init "$ROOT"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
LOCAL_DIR="$ROOT/backups"
LABEL="${1:-nightly}"
[ "$LABEL" = "--pre-migrate" ] && LABEL="pre-migrate"

mkdir -p "$LOCAL_DIR"

# ── 1. dump ─────────────────────────────────────────────────────────────────
DUMP="$LOCAL_DIR/${DB_NAME}-${STAMP}-${LABEL}.dump"

if ! "${COMPOSE[@]}" exec -T postgres pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
  log_skip "postgres is not up - nothing to dump"
  exit 0
fi

# -Fc: the custom format. Compressed, and pg_restore can select individual
# tables from it - which is what you want at 3am, not an all-or-nothing SQL file.
"${COMPOSE[@]}" exec -T postgres pg_dump -Fc -U "$DB_USER" "$DB_NAME" > "$DUMP" 2>/dev/null \
  || die "pg_dump failed"

SIZE="$(du -h "$DUMP" | cut -f1)"
log_ok "dumped $SIZE → $(basename "$DUMP")"

# A dump of nothing is not a backup, and an empty file uploaded nightly looks
# exactly like a working backup until the day it is needed.
[ -s "$DUMP" ] || die "the dump is empty - refusing to treat that as a backup"

# ── 2. the rest of what a restore needs ─────────────────────────────────────
# A database alone does not bring a dead server back: without APP_SECRET every
# session breaks, and without APP_ENCRYPTION_KEY every encrypted column is lost.
# Both live in .env, so .env is the bundle.
BUNDLE="$LOCAL_DIR/${DB_NAME}-${STAMP}-secrets.tar"
tar -cf "$BUNDLE" -C "$ROOT" .env 2>/dev/null || true

# ── 3. encrypt ──────────────────────────────────────────────────────────────
# A dump that is not encrypted never leaves: everything below refuses to copy it.
ENCRYPTED=0
if [ -z "${BACKUP_AGE_PUBKEY:-}" ]; then
  log_warn "BACKUP_AGE_PUBKEY is not set - keeping the dump in the clear, on this disk only"
elif ! command -v age >/dev/null 2>&1; then
  log_warn "age is not installed - keeping the dump in the clear, on this disk only"
else
  ENCRYPTED=1
  for f in "$DUMP" "$BUNDLE"; do
    [ -f "$f" ] || continue
    age --encrypt -r "$BACKUP_AGE_PUBKEY" -o "$f.age" "$f" && rm -f "$f"
  done
  DUMP="$DUMP.age"; BUNDLE="$BUNDLE.age"
  log_ok "encrypted to the backup public key"
fi

# ── 4. off the box ──────────────────────────────────────────────────────────
# A remote that is set and not reached fails the run - exit 1, so the nightly
# unit shows failed and `buildprod` stops. A warning in a log is not an alarm.
OFFBOX=ok
if [ -z "${BACKUP_REMOTE:-}" ]; then
  # Warned every night, deliberately. A silent local-only backup is the failure
  # mode this whole script exists to avoid.
  log_warn "BACKUP_REMOTE is not set - this backup is on the SAME DISK as the database"
  log_info "set BACKUP_REMOTE in .env (e.g. s3:my-bucket/${DB_NAME}) and re-run"
elif [ "$ENCRYPTED" -ne 1 ]; then
  log_fail "refusing to copy an unencrypted dump to $BACKUP_REMOTE" "set BACKUP_AGE_PUBKEY and install age" || true
  OFFBOX=failed
elif ! command -v rclone >/dev/null 2>&1; then
  log_fail "rclone is not installed - cannot copy off the box" || true
  OFFBOX=failed
else
  for f in "$DUMP" "$BUNDLE"; do
    [ -f "$f" ] || continue
    rclone copy "$f" "$BACKUP_REMOTE/" --quiet \
      || { log_fail "rclone could not copy $(basename "$f") to $BACKUP_REMOTE" || true; OFFBOX=failed; }
  done
  [ "$OFFBOX" = ok ] && log_ok "copied to $BACKUP_REMOTE"

  # ── 5. retention, enforced remotely ───────────────────────────────────────
  # Seven days of nightly dumps and of the secrets bundles that go with every
  # run. Applied where the backups live: pruning locally while the bucket grows
  # forever is the usual way this goes wrong. Pre-deploy and manual dumps stay.
  rclone delete "$BACKUP_REMOTE/" --min-age 7d --include "*-nightly.dump*" --include "*-secrets.tar*" --quiet 2>/dev/null || true
fi

# Local copies are a convenience, not the backup. Two days is enough to restore
# a mistake without the disk filling.
find "$LOCAL_DIR" -type f -mtime +2 -delete 2>/dev/null || true

[ "$OFFBOX" = ok ] || die "backup incomplete ($LABEL): the dump is on this disk only"
log_ok "backup complete ($LABEL)"
