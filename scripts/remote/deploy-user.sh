#!/usr/bin/env bash
# =============================================================================
#  The deploy user, created in lockout-safe order (.ai/platform/docs/deployment.md).
# =============================================================================
#  Runs from YOUR machine. Logs in as root@ once, and leaves a user that the
#  rest of provisioning can work as:
#
#    1. the user, with your root@ authorized_keys and no password
#    2. passwordless sudo - /etc/sudoers.d/90-<user>, checked with
#       `visudo -cf` BEFORE it is put in place: a broken sudoers file is a box
#       nobody can administer
#    3. the docker group. Membership of it is ROOT-EQUIVALENT (any member can
#       mount / into a container); it is granted because every stack command
#       runs as this user, not because it is a lesser privilege than sudo
#    4. a SECOND login as that user, proved, with sudo working - and only
#       after that may anything touch sshd_config (provision.sh step 3)
#
#  Idempotent: a user that exists and has sudo is left alone; one that exists
#  without it (an older provision made those) is given it through root@.
#  sshd is never touched here.
#
#  Usage: deploy-user.sh USER@HOST
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/../lib/log.sh"

HOST="${1:-}"
DEPLOY_USER="${HOST%@*}"; SERVER="${HOST#*@}"
[ -n "$HOST" ] && [ "$DEPLOY_USER" != "$HOST" ] || die "usage: deploy-user.sh USER@HOST" "example: deploy-user.sh deploy@203.0.113.42"
[ "$DEPLOY_USER" != root ] || die "the deploy user cannot be root" "pick a name: deploy-user.sh deploy@$SERVER"
[[ "$DEPLOY_USER" =~ ^[a-z_][a-z0-9_-]{0,31}$ ]] || die "'$DEPLOY_USER' is not a valid user name"

AS_USER() { ssh -o BatchMode=yes -o ConnectTimeout=15 "$HOST" "$@"; }
AS_ROOT() { ssh -o BatchMode=yes -o ConnectTimeout=15 "root@$SERVER" "$@"; }

log_step "Deploy user $DEPLOY_USER on $SERVER"

if AS_USER "sudo -n true" 2>/dev/null; then
  log_skip "$DEPLOY_USER logs in and has passwordless sudo"
  AS_USER "id -nG | tr ' ' '\n' | grep -qx docker" 2>/dev/null \
    || log_warn "$DEPLOY_USER is not in the docker group yet - provisioning adds it after Docker is installed"
  exit 0
fi

AS_ROOT true 2>/dev/null || die "cannot log in as $HOST with sudo, nor as root@$SERVER" \
  "a fresh box: log in as root once with your key (ssh-copy-id root@$SERVER), then re-run" \
  "an existing user without sudo: give it passwordless sudo by hand, then re-run"

# The sudoers line goes to a temporary file, is checked, and only then moves
# into /etc/sudoers.d - sudo refuses to run at all with a broken file in there.
AS_ROOT "set -e
  id $DEPLOY_USER >/dev/null 2>&1 || adduser --disabled-password --gecos '' $DEPLOY_USER >/dev/null
  # Created before Docker when the box is bare; Docker's package adopts it.
  groupadd -f docker
  usermod -aG docker $DEPLOY_USER

  install -d -m 700 -o $DEPLOY_USER -g $DEPLOY_USER /home/$DEPLOY_USER/.ssh
  touch /home/$DEPLOY_USER/.ssh/authorized_keys
  # Your keys, the ones that just logged in as root - appended, never replacing
  # a key the user already has.
  while IFS= read -r key; do
    [ -n \"\$key\" ] || continue
    grep -qxF \"\$key\" /home/$DEPLOY_USER/.ssh/authorized_keys || printf '%s\n' \"\$key\" >> /home/$DEPLOY_USER/.ssh/authorized_keys
  done < /root/.ssh/authorized_keys
  chown $DEPLOY_USER:$DEPLOY_USER /home/$DEPLOY_USER/.ssh/authorized_keys
  chmod 600 /home/$DEPLOY_USER/.ssh/authorized_keys

  tmp=\$(mktemp)
  printf '%s ALL=(ALL) NOPASSWD:ALL\n' $DEPLOY_USER > \"\$tmp\"
  visudo -cf \"\$tmp\" >/dev/null
  install -m 0440 -o root -g root \"\$tmp\" /etc/sudoers.d/90-$DEPLOY_USER
  rm -f \"\$tmp\"
  visudo -c >/dev/null
" || die "could not prepare $DEPLOY_USER as root@$SERVER" "nothing in sshd_config was touched"
log_ok "$DEPLOY_USER: your keys, passwordless sudo (visudo-checked), the docker group"
log_info "the docker group is root-equivalent - this user is an administrator of the box, by design"

# The second login, the whole point of the order: a NEW connection, as the
# user, with sudo. Until it works, nobody touches sshd.
AS_USER "sudo -n true" 2>/dev/null || die "cannot log in as $HOST with sudo after creating it" \
  "sshd_config was NOT modified - root@ still works" \
  "look: ssh root@$SERVER 'ls -l /home/$DEPLOY_USER/.ssh; cat /etc/sudoers.d/90-$DEPLOY_USER'"
log_ok "second login as $DEPLOY_USER works, and so does sudo"
