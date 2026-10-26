#!/usr/bin/env bash
# =============================================================================
#  Bare server → every subdomain live, in one command (.ai/platform/docs/deployment.md).
# =============================================================================
#  Runs from YOUR machine and drives the server over SSH. Every step is
#  idempotent: re-running against a live server is the normal case, and the
#  property that makes that safe is that secrets and data are read, never
#  regenerated.
#
#  Usage: provision.sh HOST DOMAIN STAGE [REPO] [REF]
#         STAGE = staging | production
#
#  Staging and production may share a server or have one each. Each gets its
#  own checkout, compose project, database, .env and systemd units
#  (scripts/lib/stage.sh); the only thing they share on one box is the edge.
#
#    production  immutable images, off-box backups
#    staging     the DEV stack - hot reload, Mailpit - so you can vibe code on
#                it; reachable only from STAGING_ALLOW_FROM, and its git key
#                can push, because commits made there are what `make promote`
#                ships
#
#  Optional environment: EMAIL (LETSENCRYPT_EMAIL), ALLOW (STAGING_ALLOW_FROM;
#  defaults to the address you are connecting from).
# =============================================================================
set -uo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
. "$HERE/../lib/log.sh"; . "$HERE/../lib/stage.sh"

HOST="${1:-}"; DOMAIN="${2:-}"; REPO="${4:-}"; REF="${5:-main}"
STAGE="$(stage_normalize "${3:-staging}")" || die "STAGE must be staging or production, got: ${3:-}"
SYNC="${SYNC:-git}"
SLUG="$(project_slug "$ROOT")"
STACK="$(stage_stack "$SLUG" "$STAGE")"
APP_DIR="$(stage_dir "$SLUG" "$STAGE")"
GIT_HOST="$(stage_git_host "$STAGE")"
OTHER_DIR="$(stage_dir "$SLUG" "$([ "$STAGE" = production ] && echo staging || echo production)")"

[ -n "$HOST" ] && [ -n "$DOMAIN" ] || die "usage: provision.sh HOST DOMAIN STAGE [REPO] [REF]"

REMOTE() { ssh -o BatchMode=yes -o ConnectTimeout=15 "$HOST" "$@"; }
DEPLOY_USER="${HOST%@*}"
[ "$DEPLOY_USER" = "$HOST" ] && DEPLOY_USER=root
SERVER="${HOST#*@}"

# ── 0. preflight ────────────────────────────────────────────────────────────
if [ "${SKIP_PREFLIGHT:-0}" = "1" ]; then
  # Documented, and a footgun: it exists for the case where you know DNS is
  # still propagating and want the stack up anyway. It is not a shortcut.
  log_warn "SKIP_PREFLIGHT=1 - provisioning a server without checking DNS, ports or repo access"
else
  GIT_HOST="$GIT_HOST" "$HERE/preflight.sh" "$HOST" "$DOMAIN" "$REPO" "$REF" || die "preflight failed - nothing was touched"
fi

log_step "Provisioning $SERVER for $DOMAIN ($STAGE → $STACK in $APP_DIR)"

# ── 1. Docker and git, if missing ───────────────────────────────────────────
if REMOTE "command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1"; then
  log_skip "docker and compose already installed"
else
  log_step "Installing Docker"
  REMOTE "sudo -n true 2>/dev/null || [ \$(id -u) -eq 0 ]" \
    || die "installing Docker needs root on the server" \
         "run this once as root@, or give $DEPLOY_USER passwordless sudo"
  REMOTE "set -e
    SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo sudo)
    export DEBIAN_FRONTEND=noninteractive
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq ca-certificates curl git make ufw
    \$SUDO install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/\$(. /etc/os-release && echo \$ID)/gpg \
      | \$SUDO tee /etc/apt/keyrings/docker.asc >/dev/null
    \$SUDO chmod a+r /etc/apt/keyrings/docker.asc
    echo \"deb [arch=\$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] https://download.docker.com/linux/\$(. /etc/os-release && echo \$ID) \$(. /etc/os-release && echo \$VERSION_CODENAME) stable\" \
      | \$SUDO tee /etc/apt/sources.list.d/docker.list >/dev/null
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
  " || die "Docker installation failed"
  log_ok "docker installed"
fi

# The nightly backup encrypts with age and leaves with rclone. Without them it
# keeps the dump on the same disk and says so in a log nobody reads - so they are
# installed here, not left for somebody to remember. Separate from Docker: a
# server image with Docker already on it has neither.
if REMOTE "command -v age >/dev/null 2>&1 && command -v rclone >/dev/null 2>&1"; then
  log_skip "age and rclone already installed"
else
  REMOTE "set -e
    SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo sudo)
    export DEBIAN_FRONTEND=noninteractive
    \$SUDO apt-get update -qq
    \$SUDO apt-get install -y -qq age rclone
  " || die "installing age and rclone failed" \
         "they need root on the server, like Docker: run once as root@, or give $DEPLOY_USER passwordless sudo"
  log_ok "age and rclone installed"
fi

# ── 2. the deploy user, created in lockout-safe order ───────────────────────
# The order is the whole point. Create the user, install the key, PROVE a second
# login works - and only then touch sshd_config. Hardening first and verifying
# afterwards is how a box becomes unreachable with no way back in.
if [ "$DEPLOY_USER" != "root" ] && REMOTE "id $DEPLOY_USER >/dev/null 2>&1"; then
  log_skip "user $DEPLOY_USER exists"
elif [ "$DEPLOY_USER" != "root" ]; then
  log_step "Creating $DEPLOY_USER"
  ssh -o BatchMode=yes "root@$SERVER" "set -e
    adduser --disabled-password --gecos '' $DEPLOY_USER
    usermod -aG docker $DEPLOY_USER
    mkdir -p /home/$DEPLOY_USER/.ssh && chmod 700 /home/$DEPLOY_USER/.ssh
    cp /root/.ssh/authorized_keys /home/$DEPLOY_USER/.ssh/authorized_keys
    chown -R $DEPLOY_USER:$DEPLOY_USER /home/$DEPLOY_USER/.ssh
    chmod 600 /home/$DEPLOY_USER/.ssh/authorized_keys
  " || die "could not create $DEPLOY_USER - is root@ reachable?"
  log_ok "$DEPLOY_USER created and added to the docker group"
fi

# ── 3. hardening, gated on a PROVEN second login ────────────────────────────
if [ "$DEPLOY_USER" != "root" ]; then
  log_step "Verifying the deploy login before hardening"
  if ! REMOTE true 2>/dev/null; then
    die "cannot log in as $HOST" \
      "sshd_config was NOT modified - the box is exactly as you found it" \
      "fix the key for $DEPLOY_USER and re-run"
  fi
  log_ok "second login as $DEPLOY_USER works"

  if REMOTE "sudo -n grep -q '^PasswordAuthentication no' /etc/ssh/sshd_config 2>/dev/null"; then
    log_skip "sshd already hardened"
  else
    log_step "Disabling password auth and root login"
    REMOTE "set -e
      sudo -n cp /etc/ssh/sshd_config /etc/ssh/sshd_config.bak.\$(date +%s)
      sudo -n sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config
      sudo -n sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config
      sudo -n sshd -t
      sudo -n systemctl reload ssh 2>/dev/null || sudo -n systemctl reload sshd
    " && log_ok "password auth off, root login key-only (backup kept)" \
      || log_warn "could not harden sshd - the stack still works; do it by hand"
  fi
fi

# ── 4. firewall, log rotation, swap ─────────────────────────────────────────
log_step "Host configuration"
REMOTE "set -e
  SUDO=\$([ \$(id -u) -eq 0 ] && echo '' || echo 'sudo -n')
  \$SUDO ufw allow 22/tcp  >/dev/null 2>&1 || true
  \$SUDO ufw allow 80/tcp  >/dev/null 2>&1 || true
  \$SUDO ufw allow 443/tcp >/dev/null 2>&1 || true
  \$SUDO ufw --force enable >/dev/null 2>&1 || true

  # Docker's default is unbounded. One chatty container fills the disk the
  # database is on, and Postgres stops before anything says why.
  echo '{\"log-driver\":\"json-file\",\"log-opts\":{\"max-size\":\"10m\",\"max-file\":\"3\"}}' \
    | \$SUDO tee /etc/docker/daemon.json >/dev/null
  \$SUDO systemctl restart docker || true

  # Under 2GB, a Nuxt build is the thing that gets OOM-killed. The Go build
  # is light; the three Nuxt images are not.
  if [ \"\$(free -m | awk '/^Mem:/{print \$2}')\" -lt 1900 ] && [ ! -f /swapfile ]; then
    \$SUDO fallocate -l 2G /swapfile && \$SUDO chmod 600 /swapfile && \$SUDO mkswap /swapfile >/dev/null
    \$SUDO swapon /swapfile
    grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' | \$SUDO tee -a /etc/fstab >/dev/null
    echo 'added 2G swap'
  fi
" || log_warn "some host configuration steps did not apply - check ufw and /etc/docker/daemon.json"
log_ok "firewall, log rotation, swap"

# ── 5. the code ─────────────────────────────────────────────────────────────
log_step "Syncing code to $APP_DIR"
REMOTE "sudo -n mkdir -p $APP_DIR /opt/enu-edge && sudo -n chown $DEPLOY_USER:$DEPLOY_USER $APP_DIR /opt/enu-edge" 2>/dev/null \
  || REMOTE "mkdir -p $APP_DIR /opt/enu-edge"

if REMOTE "test -d $OTHER_DIR" 2>/dev/null; then
  RAM_MB="$(REMOTE "free -m | awk '/^Mem:/{print \$2}'" 2>/dev/null || echo 0)"
  if [ "${RAM_MB:-0}" -lt 3800 ]; then
    log_warn "staging and production on one server with ${RAM_MB}MB RAM - the dev stack alone runs three Nuxt dev servers; 4GB is the floor"
  fi
fi

if [ "$SYNC" = rsync ]; then
  # Day one, before the project has a git remote. Documented, not hidden: what
  # lands on the server is your working tree, including anything uncommitted -
  # and with no git history there, `make promote` has nothing to promote.
  log_warn "SYNC=rsync - pushing the local working tree, not a commit; make promote will not work"
  command -v rsync >/dev/null 2>&1 || die "rsync is not installed"
  rsync -az --delete \
    --exclude '.git' --exclude 'node_modules' --exclude 'backend/bin' \
    --exclude '.nuxt' --exclude '.output' --exclude '.out' \
    --exclude 'docker/certs' --exclude 'docker/api/project.env' \
    --exclude '.env' --exclude '.env.*' \
    "$ROOT/" "$HOST:$APP_DIR/" || die "rsync failed"
  log_ok "working tree pushed"
else
  [ -n "$REPO" ] || die "REPO is required unless SYNC=rsync" \
    "example: make build$( [ "$STAGE" = production ] && echo prod || echo staging ) HOST=$HOST DOMAIN=$DOMAIN REPO=acme/$SLUG"
  if REMOTE "test -d $APP_DIR/.git"; then
    if [ "$STAGE" = staging ]; then
      # Staging is where work happens. Re-provisioning must not reset it.
      if [ -n "$(REMOTE "cd $APP_DIR && git status --porcelain")" ]; then
        log_warn "staging has uncommitted changes - leaving its checkout exactly as it is"
      else
        REMOTE "cd $APP_DIR && git fetch --quiet origin && git merge --quiet --ff-only origin/\$(git symbolic-ref --short HEAD)" \
          && log_ok "staging fast-forwarded" \
          || log_warn "staging has commits origin lacks - not touched; make promote pushes them"
      fi
    else
      REMOTE "cd $APP_DIR && git fetch --quiet origin && { git -c advice.detachedHead=false checkout --quiet --detach origin/$REF 2>/dev/null \
                || git -c advice.detachedHead=false checkout --quiet --detach $REF; }" \
        || die "could not move $APP_DIR to $REF"
      log_ok "$REPO@$REF at $APP_DIR"
    fi
  else
    REMOTE "git clone --quiet git@$GIT_HOST:$REPO.git $APP_DIR && cd $APP_DIR && git checkout --quiet $REF" \
      || die "could not clone $REPO@$REF" "run: make deploy-key HOST=$HOST REPO=$REPO STAGE=$STAGE"
    log_ok "$REPO@$REF cloned to $APP_DIR"
  fi

  if [ "$STAGE" = staging ]; then
    # Commits made on staging need an author. Yours, taken from this machine.
    name="$(git config user.name 2>/dev/null || true)"; email="$(git config user.email 2>/dev/null || true)"
    [ -n "$name" ] && REMOTE "cd $APP_DIR && git config user.name >/dev/null || git config user.name '$name'"
    [ -n "$email" ] && REMOTE "cd $APP_DIR && git config user.email >/dev/null || git config user.email '$email'"
  fi
fi

# ── 6. environment: generated once, preserved forever after ─────────────────
# This is the property that makes the command safe to re-run against a live
# server. Regenerating APP_SECRET invalidates every session; regenerating
# APP_ENCRYPTION_KEY makes every encrypted column unreadable, permanently.
log_step "Environment"
# The address this machine reaches the server from, as the server sees it.
CLIENT_IP="$(REMOTE 'echo ${SSH_CLIENT%% *}' 2>/dev/null || true)"

REMOTE "set -e
  cd $APP_DIR
  [ -f .env ] || { cp .env.example .env && chmod 600 .env && echo created; }
  ./scripts/lib/envgen.sh generate >/dev/null     # adds keys a newer .env.example declares
  set_env() { grep -q \"^\$1=\" .env && sed -i \"s|^\$1=.*|\$1=\$2|\" .env || echo \"\$1=\$2\" >> .env; }
  set_env DOMAIN       '$DOMAIN'
  set_env APP_STAGE    '$STAGE'
  set_env STACK        '$STACK'
  set_env COMPOSE_FILE '$(stage_compose_files "$STAGE")'
  set_env HOST_UID     \$(id -u)
  set_env HOST_GID     \$(id -g)
  [ -z '${EMAIL:-}' ] || set_env LETSENCRYPT_EMAIL '${EMAIL:-}'
  if [ '$STAGE' = staging ]; then
    if [ -n '${ALLOW:-}' ]; then set_env STAGING_ALLOW_FROM '${ALLOW:-}'
    elif [ -z \"\$(sed -n 's/^STAGING_ALLOW_FROM=//p' .env)\" ] && [ -n '$CLIENT_IP' ]; then set_env STAGING_ALLOW_FROM '$CLIENT_IP'
    fi
  fi
  ./scripts/lib/envgen.sh generate >/dev/null
" || die "could not prepare the environment on $HOST"

[ -n "$(REMOTE "sed -n 's/^LETSENCRYPT_EMAIL=//p' $APP_DIR/.env")" ] \
  || die "LETSENCRYPT_EMAIL is not set for $STACK" \
       "without it no certificate is ever issued - re-run with EMAIL=you@example.com"
log_ok ".env present, per-app files derived (existing values preserved)"
[ "$STAGE" = staging ] && log_info "staging answers only to: $(REMOTE "sed -n 's/^STAGING_ALLOW_FROM=//p' $APP_DIR/.env")"

# Production refuses to start with Mailpit as its mail provider (compose.prod.yml
# requires MAILER_DSN). Said here, before a forty-minute build fails on it.
if [ "$STAGE" = production ]; then
  dsn="$(REMOTE "sed -n 's/^MAILER_DSN=//p' $APP_DIR/.env" 2>/dev/null || true)"
  case "$dsn" in
    ""|*mailpit*)
      die "MAILER_DSN is '${dsn:-empty}' in $APP_DIR/.env - production needs a real mail provider" \
        "set it on the server (smtp://user:pass@host:587) and re-run" \
        "silently dropping verification mail in production is worse than failing here" ;;
  esac
  log_ok "mail goes through a real provider"
fi

# ── 7. release: build, back up, migrate, start, health ─────────────────────
# The same script `make deploy*` and `make promote` run, so a first release
# and the hundredth cannot differ.
log_step "Releasing (a few minutes on the first run)"
REMOTE "cd $APP_DIR && bash scripts/remote/release.sh" \
  || die "the release failed on $HOST" "inspect: ssh $HOST 'cd $APP_DIR && docker compose logs --tail=80'"

# ── 8. from the outside: DNS, certificate and routing, end to end ───────────
log_step "Waiting for https://api.$DOMAIN/health/deep from here"
healthy=0
for _ in $(seq 1 40); do
  if curl -fsS --max-time 5 "https://api.$DOMAIN/health/deep" 2>/dev/null | grep -q '"status":"ok"'; then
    healthy=1; break
  fi
  sleep 5
done
if [ "$healthy" -ne 1 ]; then
  [ "$STAGE" = staging ] && log_info "staging answers only to STAGING_ALLOW_FROM - is this machine's address on it?"
  die "https://api.$DOMAIN is not healthy from here within 200s" \
    "it is healthy on the server, so look at DNS, the certificate, or the allowlist:" \
    "  ssh $HOST 'docker logs --tail=80 enu-edge'"
fi
log_ok "deep health check green from outside"

# ── 9. survive a reboot, and back up nightly ────────────────────────────────
"$HERE/install-units.sh" "$HOST" "$STACK" "$APP_DIR" "$STAGE" || log_warn "systemd units not installed - the stack will not return after a reboot"

# ── 10. the backup gate - production only ──────────────────────────────────
BACKUP_REMOTE="$(REMOTE "grep '^BACKUP_REMOTE=' $APP_DIR/.env 2>/dev/null | cut -d= -f2-" || true)"
AGE_PUB="$(REMOTE "grep '^BACKUP_AGE_PUBKEY=' $APP_DIR/.env 2>/dev/null | cut -d= -f2-" || true)"

echo
"$HERE/urls.sh" "$DOMAIN" 2>/dev/null || true
[ -n "$AGE_PUB" ] && { echo; log_info "Backup public key (store the PRIVATE half off this server):"; log_info "  $AGE_PUB"; }

if [ -z "$BACKUP_REMOTE" ]; then
  if [ "$STAGE" = production ]; then
    # The stack is up. The command is not done - and saying "done" here is how a
    # production system runs for a year with backups nobody configured.
    echo
    log_fail "BACKUP_REMOTE is not set - this is a production server with no off-box backup" \
      "the stack IS running; this command is refusing to call that finished" \
      "set it on the server and re-run:" \
      "  ssh $HOST 'cd $APP_DIR && nano .env'   # BACKUP_REMOTE=s3:bucket/path" \
      "  make buildprod HOST=$HOST DOMAIN=$DOMAIN REPO=$REPO" \
      "a pg_dump on the same disk as the database is a hope, not a backup"
    exit 1
  fi
  log_warn "BACKUP_REMOTE is not set - local dumps only, and the nightly timer will warn until it is"
elif [ "$STAGE" = production ]; then
  # Set is not working. A remote nobody can log in to, or a dump that left in the
  # clear, looks exactly like a backup until the night it is needed - so one is
  # taken now and looked for where it is supposed to be.
  if [ -z "$AGE_PUB" ]; then
    log_fail "BACKUP_AGE_PUBKEY is not set - the dump would leave this server unencrypted" \
      "generate the pair on YOUR machine:  age-keygen -o ~/.config/age/<project>.txt" \
      "put the public half (age1...) in .env on the server; the private half never goes there"
    exit 1
  fi
  log_step "Proving the off-box backup"
  if ! REMOTE "cd $APP_DIR && bash scripts/remote/backup.sh manual" >/dev/null 2>&1; then
    log_fail "the backup did not reach $BACKUP_REMOTE" \
      "see why:  ssh $HOST 'cd $APP_DIR && bash scripts/remote/backup.sh manual'" \
      "the rclone remote belongs to the deploy user:  ssh $HOST rclone config"
    exit 1
  fi
  FOUND="$(REMOTE "rclone lsf '$BACKUP_REMOTE/' --include '*-manual.dump.age' 2>/dev/null | wc -l" || echo 0)"
  if [ "${FOUND:-0}" -eq 0 ]; then
    log_fail "the backup reported success but nothing is listed at $BACKUP_REMOTE" \
      "look by hand:  ssh $HOST \"rclone lsf '$BACKUP_REMOTE/'\""
    exit 1
  fi
  log_ok "an encrypted backup is at $BACKUP_REMOTE"
fi

echo
log_ok "$DOMAIN is live ($STAGE)"
if [ "$STAGE" = staging ]; then
  echo
  log_info "Vibe code here:   ssh $HOST, then cd $APP_DIR - the stack hot-reloads"
  log_info "Pull your work:   make deploystaging HOST=$HOST DOMAIN=$DOMAIN"
  log_info "Ship to prod:     make promote          (on this server)"
  log_info "                  make promote STAGING=$HOST [PROD=deploy@…]   (from your machine)"
fi
