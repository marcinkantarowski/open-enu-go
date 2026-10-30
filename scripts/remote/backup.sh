#!/usr/bin/env bash
# =============================================================================
#  The nightly backup. Runs ON the server (.ai/platform/docs/deployment.md).
# =============================================================================
#  A pg_dump on the same disk as the database is a hope, not a backup. The disk
#  is what fails, and the dump fails with it.
#
#    pg_dump -Fc             →  age --encrypt  →  rclone copy to an off-box bucket
#    the storage volume, tar →  the same, every run (uploads live nowhere else)
#    .env                    →  a secrets bundle - encrypted, or not written at all
#
#  Encrypted before it leaves, so the bucket holds nothing readable. The private
#  half of the age key is deliberately NOT on this server - a backup an attacker
#  with the server can decrypt is a backup that protects against disk failure
#  and nothing else.
#
#  Usage: backup.sh [LABEL]          nightly (default), pre-deploy, pre-restore,
#                                    manual; --pre-migrate means pre-migrate
#         BACKUP_STAMP=<stamp>       the caller's UTC stamp (20260101T031700Z),
#                                    so it can look for THIS run's files and
#                                    never mistake an older run's for them
#
#  Exit codes - a contract; release.sh and the systemd unit read them:
#
#    0  complete: dumped, storage archived, off the box (or no BACKUP_REMOTE)
#    1  could not start: no .env, a malformed stamp or label
#    2  NO DUMP: postgres is down, pg_dump failed, or the dump is empty.
#       Nothing from this run is usable; a deploy must not migrate on it.
#    3  the dump is good and on THIS disk only: BACKUP_REMOTE is set and the
#       copy failed or was refused. A rollback can use it; the unit still fails.
#    4  the dump is good (and off the box when a remote is set) but the
#       storage volume could not be archived. For a deploy, the same as 3.
#
#  One run, one stamp:
#    <db>-<stamp>-<label>.dump[.age]            the database
#    <db>-<stamp>-<label>-storage.tar[.age]     the storage volume
#    <db>-<stamp>-secrets.tar.age               .env, only ever encrypted
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/compose.sh"

cd "$ROOT" || die "cannot enter $ROOT"
[ -f .env ] || die "$ROOT/.env is missing"
set -a; . ./.env; set +a
[ -n "${DB_NAME:-}" ] && [ -n "${DB_USER:-}" ] && [ -n "${STACK:-}" ] || die "DB_NAME, DB_USER and STACK must be set in $ROOT/.env"

compose_init "$ROOT"
STAMP="${BACKUP_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}"
[[ "$STAMP" =~ ^[0-9]{8}T[0-9]{6}Z$ ]] || die "BACKUP_STAMP must look like 20260101T031700Z, got: $STAMP"
LOCAL_DIR="$ROOT/backups"
LABEL="${1:-nightly}"
[ "$LABEL" = "--pre-migrate" ] && LABEL="pre-migrate"
[[ "$LABEL" =~ ^[a-z][a-z-]*$ ]] || die "the label must be lowercase letters and hyphens, got: $LABEL"
PREFIX="${LOCAL_DIR:?}/${DB_NAME:?}-${STAMP:?}"

# Everything written here is the database and the keys to it. Nobody else on
# the host reads it, not even in the seconds before it is encrypted.
umask 077
install -d -m 700 "$LOCAL_DIR" || die "cannot create $LOCAL_DIR"
chmod 700 "$LOCAL_DIR" 2>/dev/null || true   # one made by an older version

# ── 1. dump ─────────────────────────────────────────────────────────────────
DUMP="$PREFIX-${LABEL}.dump"

if ! "${COMPOSE[@]}" exec -T postgres pg_isready -U "$DB_USER" -d "$DB_NAME" >/dev/null 2>&1; then
  # An alarm, not a skip: production's database down at 03:17 is news, and a
  # deploy that read "nothing to dump" as success would migrate with no way
  # back (release.sh brings postgres up before it asks for the dump).
  log_fail "postgres is not up - nothing was dumped" "inspect: ${COMPOSE[*]} logs --tail=80 postgres" || true
  exit 2
fi

# -Fc: the custom format. Compressed, and pg_restore can select individual
# tables from it - which is what you want at 3am, not an all-or-nothing SQL file.
if ! "${COMPOSE[@]}" exec -T postgres pg_dump -Fc -U "$DB_USER" "$DB_NAME" > "$DUMP" 2>/dev/null; then
  rm -f "$DUMP"
  log_fail "pg_dump failed - no dump was written (disk full? df -h $LOCAL_DIR)" || true
  exit 2
fi
# A dump of nothing is not a backup, and an empty file uploaded nightly looks
# exactly like a working backup until the day it is needed.
if [ ! -s "$DUMP" ]; then
  rm -f "$DUMP"
  log_fail "the dump is empty - refusing to treat that as a backup" || true
  exit 2
fi
log_ok "dumped $(du -h "$DUMP" | cut -f1) → $(basename "$DUMP")"

# ── 2. the storage volume ───────────────────────────────────────────────────
# Uploads live on the `storage` volume and nowhere else (no S3 backend): the
# attachments, the scanned agreements, the kept PDFs. A restored database that
# names files the disk no longer has is a broken link in every register.
#
# The volume's real name comes from the labels compose puts on it - the
# project (STACK, the compose file's `name:`) and the volume key - not from
# gluing strings together. tar runs in the image the database already runs:
# it is on the box and pinned, so nothing is pulled at 3am.
STORAGE="$PREFIX-${LABEL}-storage.tar"
STORAGE_OK=1
VOLUME="$(docker volume ls -q \
  --filter "label=com.docker.compose.project=${STACK}" \
  --filter "label=com.docker.compose.volume=storage" 2>/dev/null | head -1)"
PG_IMAGE="$(docker inspect -f '{{.Config.Image}}' "$("${COMPOSE[@]}" ps -q postgres 2>/dev/null | head -1)" 2>/dev/null || true)"
if [ -z "$VOLUME" ]; then
  log_warn "no storage volume for project ${STACK} yet - nothing uploaded to archive"
  STORAGE=""
elif [ -z "$PG_IMAGE" ] \
     || ! docker run --rm --network none -v "$VOLUME:/s:ro" --entrypoint tar "$PG_IMAGE" --numeric-owner -C /s -cf - . > "$STORAGE" 2>/dev/null \
     || ! tar -tf "$STORAGE" >/dev/null 2>&1; then
  rm -f "$STORAGE"; STORAGE=""; STORAGE_OK=0
  log_fail "could not archive the storage volume $VOLUME - this backup has the database but not the uploads" || true
else
  log_ok "archived $VOLUME: $(tar -tf "$STORAGE" | grep -cv '/$') file(s), $(du -h "$STORAGE" | cut -f1) → $(basename "$STORAGE")"
fi

# ── 3. the rest of what a restore needs ─────────────────────────────────────
# A database alone does not bring a dead server back: without APP_SECRET every
# session breaks, and without APP_ENCRYPTION_KEY every encrypted column is lost.
# Both live in .env, so .env is the bundle - written ONLY when it can be
# encrypted. A copy of the encryption key beside the data it protects is the
# key under the mat, not a backup.
BUNDLE=""
CAN_ENCRYPT=0
if [ -z "${BACKUP_AGE_PUBKEY:-}" ]; then
  log_warn "BACKUP_AGE_PUBKEY is not set - keeping the dump in the clear, on this disk only"
elif ! command -v age >/dev/null 2>&1; then
  log_warn "age is not installed - keeping the dump in the clear, on this disk only"
else
  CAN_ENCRYPT=1
fi
if [ "$CAN_ENCRYPT" -eq 1 ]; then
  BUNDLE="$PREFIX-secrets.tar"
  tar -cf "$BUNDLE" -C "$ROOT" .env 2>/dev/null || { rm -f "$BUNDLE"; BUNDLE=""; log_warn "could not bundle .env"; }
else
  log_warn "NOT writing the secrets bundle: .env holds APP_ENCRYPTION_KEY and is never stored unencrypted"
fi

# ── 4. encrypt ──────────────────────────────────────────────────────────────
# A file that is not encrypted never leaves: everything below refuses to copy it.
ENCRYPTED=0
if [ "$CAN_ENCRYPT" -eq 1 ]; then
  ENCRYPTED=1
  for f in "$DUMP" "$STORAGE" "$BUNDLE"; do
    [ -n "$f" ] && [ -f "$f" ] || continue
    if age --encrypt -r "$BACKUP_AGE_PUBKEY" -o "$f.age" "$f"; then
      rm -f "$f"
    else
      rm -f "$f.age"; ENCRYPTED=0
      log_fail "age could not encrypt $(basename "$f")" || true
    fi
  done
  if [ "$ENCRYPTED" -eq 1 ]; then
    DUMP="$DUMP.age"
    [ -z "$STORAGE" ] || STORAGE="$STORAGE.age"
    [ -z "$BUNDLE" ] || BUNDLE="$BUNDLE.age"
    log_ok "encrypted to the backup public key"
  else
    # Partly encrypted is not encrypted, and .env never stays in the clear.
    rm -f "$PREFIX-secrets.tar" "$PREFIX-secrets.tar.age"
    BUNDLE=""
    [ -f "$DUMP" ] || DUMP="$DUMP.age"
    [ -z "$STORAGE" ] || [ -f "$STORAGE" ] || STORAGE="$STORAGE.age"
  fi
fi

# ── 5. off the box ──────────────────────────────────────────────────────────
# A remote that is set and not reached fails the run - exit 3, so the nightly
# unit shows failed and `buildprod` stops. A warning in a log is not an alarm.
OFFBOX=ok
if [ -z "${BACKUP_REMOTE:-}" ]; then
  # Warned every night, deliberately. A silent local-only backup is the failure
  # mode this whole script exists to avoid.
  log_warn "BACKUP_REMOTE is not set - this backup is on the SAME DISK as the database"
  log_info "set BACKUP_REMOTE in .env (e.g. s3:my-bucket/${DB_NAME}) and re-run"
elif [ "$ENCRYPTED" -ne 1 ]; then
  log_fail "refusing to copy an unencrypted backup to $BACKUP_REMOTE" "set BACKUP_AGE_PUBKEY and install age" || true
  OFFBOX=failed
elif ! command -v rclone >/dev/null 2>&1; then
  log_fail "rclone is not installed - cannot copy off the box" || true
  OFFBOX=failed
else
  for f in "$DUMP" "$STORAGE" "$BUNDLE"; do
    [ -n "$f" ] && [ -f "$f" ] || continue
    rclone copy "$f" "$BACKUP_REMOTE/" --quiet \
      || { log_fail "rclone could not copy $(basename "$f") to $BACKUP_REMOTE" || true; OFFBOX=failed; }
  done
  [ "$OFFBOX" = ok ] && log_ok "copied to $BACKUP_REMOTE"

  # ── 6. retention, enforced remotely ───────────────────────────────────────
  # Seven days of nightly dumps, storage archives and the secrets bundles that
  # go with every run. Applied where the backups live: pruning locally while
  # the bucket grows forever is the usual way this goes wrong. Pre-deploy,
  # pre-restore and manual runs stay.
  rclone delete "$BACKUP_REMOTE/" --min-age 7d \
    --include "*-nightly.dump*" --include "*-nightly-storage.tar*" --include "*-secrets.tar*" \
    --quiet 2>/dev/null || true
fi

# Local copies are a convenience, not the backup. Two days is enough to restore
# a mistake without the disk filling.
find "$LOCAL_DIR" -type f -mtime +2 -delete 2>/dev/null || true

# This run's dump, on a line of its own, for a caller that wants the path.
echo "dump: $DUMP"
if [ "$OFFBOX" != ok ]; then
  log_fail "backup incomplete ($LABEL): on this disk only" || true
  exit 3
fi
if [ "$STORAGE_OK" -ne 1 ]; then
  log_fail "backup incomplete ($LABEL): the database without the uploads" || true
  exit 4
fi
log_ok "backup complete ($LABEL)"
