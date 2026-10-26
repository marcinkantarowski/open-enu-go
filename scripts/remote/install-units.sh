#!/usr/bin/env bash
# =============================================================================
#  systemd: bring the stack back after a reboot, and back it up nightly.
# =============================================================================
#  Without the first unit, a kernel update at 4am takes the product down until
#  somebody notices. `restart: unless-stopped` covers a crashed container; it
#  does not cover a rebooted host, because nothing starts the compose project.
#
#  Units are named after the STACK (<slug>-staging, <slug>-prod), so two stages
#  on one server get two units instead of the second overwriting the first.
#  Only production gets the backup timer: staging data is disposable, and a
#  nightly upload of it is a bill for nothing.
#
#  Usage: install-units.sh HOST STACK APP_DIR STAGE
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"; SLUG="${2:-}"; APP_DIR="${3:-}"; STAGE="${4:-production}"
[ -n "$HOST" ] && [ -n "$SLUG" ] && [ -n "$APP_DIR" ] || die "usage: install-units.sh HOST STACK APP_DIR STAGE"

REMOTE() { ssh -o BatchMode=yes "$HOST" "$@"; }
USER_NAME="${HOST%@*}"

log_step "systemd units"

REMOTE "set -e
  SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo 'sudo -n')

  \$SUDO tee /etc/systemd/system/$SLUG.service >/dev/null <<UNIT
[Unit]
Description=$SLUG application stack
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
User=$USER_NAME
WorkingDirectory=$APP_DIR
# stack.sh reads COMPOSE_FILE from the stack's .env and re-attaches the edge.
# Not a systemd EnvironmentFile: compose does its own interpolation of that
# file, and two parsers over one file disagree about quoting.
ExecStart=/bin/bash $APP_DIR/scripts/remote/stack.sh up
ExecStop=/bin/bash $APP_DIR/scripts/remote/stack.sh stop
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
UNIT

  \$SUDO systemctl daemon-reload
  \$SUDO systemctl enable --now $SLUG.service >/dev/null 2>&1
  [ '$STAGE' = production ] || exit 0

  \$SUDO tee /etc/systemd/system/$SLUG-backup.service >/dev/null <<UNIT
[Unit]
Description=$SLUG nightly backup
After=docker.service

[Service]
Type=oneshot
User=$USER_NAME
WorkingDirectory=$APP_DIR
ExecStart=/bin/bash $APP_DIR/scripts/remote/backup.sh
UNIT

  \$SUDO tee /etc/systemd/system/$SLUG-backup.timer >/dev/null <<UNIT
[Unit]
Description=$SLUG nightly backup

[Timer]
OnCalendar=*-*-* 03:17:00
# Not 03:00. Every timer in the world fires on the hour, and a bucket that
# throttles at 3am sharp throttles everybody at once.
RandomizedDelaySec=900
Persistent=true

[Install]
WantedBy=timers.target
UNIT

  \$SUDO systemctl daemon-reload
  \$SUDO systemctl enable --now $SLUG.service >/dev/null 2>&1
  \$SUDO systemctl enable --now $SLUG-backup.timer >/dev/null 2>&1
" || { log_warn "could not install systemd units (needs sudo on the server)"; exit 1; }

log_ok "$SLUG.service enabled - the stack returns after a reboot"
[ "$STAGE" = production ] && log_ok "$SLUG-backup.timer enabled - nightly at 03:17 (+ up to 15 min jitter)"
