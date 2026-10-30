#!/usr/bin/env bash
# =============================================================================
#  A unit failed: say so where a person will see it. Run by systemd as
#  <stack>-notify@<unit>.service, through OnFailure= on the stack's units
#  (install-units.sh). Not run from the checkout: install-units.sh copies it to
#  /usr/local/lib/<stack>-notify, root-owned, because it runs as root and the
#  checkout is writable by the deploy user.
# =============================================================================
#  Three channels, in order, and the first two are not either-or:
#
#    the journal   always, at priority err - `journalctl -p err` finds it
#    mail          to ALERT_EMAIL (else LETSENCRYPT_EMAIL) from the stack's
#                  .env, through the host's own sendmail or mail command, when
#                  one is installed (msmtp-mta pointed at the relay is enough).
#                  The application's MAILER_DSN is not used: the API binary
#                  has no mail sub-command, and the alarm must not depend on
#                  the stack that may be what failed.
#    wall          when no mail could be sent - a logged-in operator sees it
#
#  Usage: notify.sh UNIT APP_DIR
# =============================================================================
set -uo pipefail
UNIT="${1:-}"; APP_DIR="${2:-}"
[ -n "$UNIT" ] && [ -n "$APP_DIR" ] || { echo "usage: notify.sh UNIT APP_DIR" >&2; exit 2; }

env_of() { sed -n "s/^$1=//p" "$APP_DIR/.env" 2>/dev/null | head -1; }
TO="$(env_of ALERT_EMAIL)"; [ -n "$TO" ] || TO="$(env_of LETSENCRYPT_EMAIL)"
HOST_NAME="$(hostname -f 2>/dev/null || hostname)"
SUBJECT="[$HOST_NAME] $UNIT failed"
BODY="$(systemctl status --no-pager --lines=0 "$UNIT" 2>&1; echo; journalctl -u "$UNIT" -n 40 --no-pager 2>&1)"

logger -p user.err -t "$(basename "$0")" "$SUBJECT - journalctl -u $UNIT"

sent=0
if [ -n "$TO" ]; then
  if command -v sendmail >/dev/null 2>&1; then
    printf 'To: %s\nSubject: %s\n\n%s\n' "$TO" "$SUBJECT" "$BODY" | sendmail -t && sent=1
  elif command -v mail >/dev/null 2>&1; then
    printf '%s\n' "$BODY" | mail -s "$SUBJECT" "$TO" && sent=1
  fi
fi
if [ "$sent" -eq 1 ]; then
  logger -p user.notice -t "$(basename "$0")" "mailed $TO about $UNIT"
else
  logger -p user.err -t "$(basename "$0")" "no mail sent (ALERT_EMAIL empty, or no sendmail/mail on this host)"
  wall "$SUBJECT - journalctl -u $UNIT" 2>/dev/null || true
fi
exit 0
