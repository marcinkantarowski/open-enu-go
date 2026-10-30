#!/usr/bin/env bash
# =============================================================================
#  systemd: bring the stack back after a reboot, back it up nightly, and say
#  so when either fails.
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
#  A failed unit starts <stack>-notify@<unit>.service (OnFailure=), which runs
#  scripts/remote/notify.sh: the journal at priority err, mail to ALERT_EMAIL
#  through the host's sendmail when there is one, else `wall`. A backup that
#  fails at 03:17 and tells nobody is the same as no backup.
#
#  Usage: install-units.sh HOST STACK APP_DIR STAGE [DEPLOY_USER]
#         DEPLOY_USER defaults to whoever HOST logs in as - asked of the server,
#         not cut out of HOST, which may be a bare address or an ssh alias.
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"; SLUG="${2:-}"; APP_DIR="${3:-}"; STAGE="${4:-production}"; USER_NAME="${5:-}"
[ -n "$HOST" ] && [ -n "$SLUG" ] && [ -n "$APP_DIR" ] || die "usage: install-units.sh HOST STACK APP_DIR STAGE [DEPLOY_USER]"

REMOTE() { ssh -o BatchMode=yes "$HOST" "$@"; }
[ -n "$USER_NAME" ] || USER_NAME="$(REMOTE 'id -un' 2>/dev/null)"
[[ "$USER_NAME" =~ ^[a-z_][a-z0-9_-]*$ ]] || die "cannot tell which user the units run as (got '${USER_NAME}')" \
  "pass it: install-units.sh $HOST $SLUG $APP_DIR $STAGE deploy"
[ "$USER_NAME" = root ] && log_warn "the units run as root - give the stack its own deploy user (deployment.md → The deploy user)"

log_step "systemd units (as $USER_NAME)"

REMOTE "set -e
  SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo 'sudo -n')

  # The notifier runs as root, so it is a root-owned copy - not the checkout's
  # file, which the deploy user can rewrite.
  \$SUDO install -m 0755 -o root -g root $APP_DIR/scripts/remote/notify.sh /usr/local/lib/$SLUG-notify

  \$SUDO tee /etc/systemd/system/$SLUG-notify@.service >/dev/null <<UNIT
[Unit]
Description=$SLUG: tell someone that %i failed

[Service]
Type=oneshot
ExecStart=/usr/local/lib/$SLUG-notify %i $APP_DIR
UNIT

  \$SUDO tee /etc/systemd/system/$SLUG.service >/dev/null <<UNIT
[Unit]
Description=$SLUG application stack
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target
OnFailure=$SLUG-notify@%n.service

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
# Every non-zero exit of backup.sh is a failure here, 3 and 4 included: a
# backup on this disk only, or without the uploads, is what this alarm is for.
OnFailure=$SLUG-notify@%n.service

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
log_ok "$SLUG-notify@.service: a failed unit is logged at err and mailed to ALERT_EMAIL (needs sendmail on the host; else wall)"
if ! REMOTE "command -v sendmail >/dev/null 2>&1 || command -v mail >/dev/null 2>&1"; then
  log_warn "no sendmail or mail on $HOST - a failure reaches the journal and wall, not your inbox"
  log_info "install one pointed at your relay, e.g.: sudo apt-get install msmtp-mta (then /etc/msmtprc)"
fi
exit 0
